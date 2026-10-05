package webui

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"

	"seraph/internal/board"
	"seraph/internal/dashboard"
	"seraph/internal/repo"
	"seraph/internal/vocab"
)

//go:embed index.html
var indexHTML string

//go:embed static
var staticFS embed.FS

type pageData struct {
	Token      string
	Root       string
	Note       string
	Err        string
	View       dashboard.View
	Statuses   []vocab.Option
	Priorities []vocab.Option
	Triages    []vocab.Option
}

type Server struct {
	board *dashboard.Board
	token string
	tmpl  *template.Template
}

func Listen(root repo.Root, port int) (string, error) {
	b, err := dashboard.Open(root)
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("index.html").Funcs(funcs()).Parse(indexHTML)
	if err != nil {
		b.Close()
		return "", fmt.Errorf("parse template: %w", err)
	}

	token, err := mintToken()
	if err != nil {
		b.Close()
		return "", err
	}

	s := &Server{board: b, token: token, tmpl: tmpl}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		b.Close()
		return "", fmt.Errorf("listen: %w", err)
	}
	url := fmt.Sprintf("http://%s/?t=%s", listener.Addr().String(), token)

	go func() { http.Serve(listener, s.routes()) }()
	return url, nil
}

func mintToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("mint token: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.auth(s.page))
	mux.HandleFunc("/board", s.auth(s.fragment))
	mux.HandleFunc("/act", s.auth(s.act))

	// Assets are embedded in the binary, so a running server has exactly one copy and
	// it cannot change until the binary is rebuilt. The browser, however, will happily
	// hold the previous one — which showed up as a fix that "did not apply". No-store
	// costs nothing here: nothing is fetched twice.
	static := http.FileServer(http.FS(staticFS))
	mux.Handle("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		static.ServeHTTP(w, r)
	}))
	return mux
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/static/" || strings.HasPrefix(r.URL.Path, "/static/") {
			next(w, r)
			return
		}
		if r.URL.Query().Get("t") != s.token {
			http.Error(w, "unauthorized: open the URL printed by `seraph ui`", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) data(r *http.Request) (pageData, error) {
	view, err := s.board.View()
	if err != nil {
		return pageData{}, err
	}
	return pageData{
		Token:      s.token,
		Root:       view.Root,
		Note:       r.URL.Query().Get("note"),
		Err:        r.URL.Query().Get("err"),
		View:       view,
		Statuses:   vocab.Statuses,
		Priorities: vocab.Priorities,
		Triages:    vocab.Triages,
	}, nil
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	data, err := s.data(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) fragment(w http.ResponseWriter, r *http.Request) {
	data, err := s.data(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, "board", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) act(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirect(w, r, "", err.Error())
		return
	}

	action := r.FormValue("action")
	id := r.FormValue("id")

	var err error
	var note string
	// A note is set only when the action actually succeeded. Reporting the intended
	// outcome alongside the error put a green "Completed TASK-101" next to the refusal
	// that stopped it, and a reader takes the green line for the answer.
	switch action {
	case "claim":
		if err = s.board.Claim(id); err == nil {
			note = "Claimed " + id
		}
	case "release":
		if err = s.board.Release(id); err == nil {
			note = "Released " + id
		}
	case "done":
		if err = s.board.SetStatus(id, "done"); err == nil {
			note = "Completed " + id
		}
	case "status":
		status := r.FormValue("status")
		if err = s.board.SetStatus(id, status); err == nil {
			note = id + " → " + dashboard.StatusLabel(status)
		}
	case "triage":
		if err = s.board.SetTriage(id, r.FormValue("triage")); err == nil {
			note = "Retriaged " + id
		}
	case "create":
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			err = fmt.Errorf("a task needs a title")
			break
		}
		var created string
		created, err = s.board.Create(title,
			strings.TrimSpace(r.FormValue("goal")),
			strings.TrimSpace(r.FormValue("description")),
			strings.TrimSpace(r.FormValue("acceptance")),
			r.FormValue("priority"),
			r.FormValue("triage"))
		if err == nil {
			note = "Created " + created
		}
	default:
		err = fmt.Errorf("unknown action %q", action)
	}

	s.redirect(w, r, note, errText(err))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	if typed, ok := board.AsError(err); ok {
		return typed.Reason + " — " + typed.Message
	}
	return err.Error()
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request, note, errMsg string) {
	url := "/?t=" + s.token
	if note != "" {
		url += "&note=" + template.URLQueryEscaper(note)
	}
	if errMsg != "" {
		url += "&err=" + template.URLQueryEscaper(errMsg)
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}
