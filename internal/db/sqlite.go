package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"seraph/internal/repo"

	_ "modernc.org/sqlite"
)

func Open(r repo.Root) (*sql.DB, error) {
	if err := os.MkdirAll(r.StatePath(), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", r.StatePath(), err)
	}

	handle, err := sql.Open("sqlite", dsnFor(r.DBPath()))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", r.DBPath(), err)
	}

	handle.SetMaxOpenConns(4)
	handle.SetMaxIdleConns(4)

	if err := handle.Ping(); err != nil {
		handle.Close()
		return nil, fmt.Errorf("open %s: %w", r.DBPath(), err)
	}
	if err := ensureWAL(handle, r.DBPath()); err != nil {
		handle.Close()
		return nil, err
	}
	if err := Migrate(handle); err != nil {
		handle.Close()
		return nil, err
	}
	return handle, nil
}

func ensureWAL(handle *sql.DB, path string) error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		var mode string
		if err := handle.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			return fmt.Errorf("read journal mode of %s: %w", path, err)
		}
		if strings.EqualFold(mode, "wal") {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("could not put %s into WAL mode (still %s)", path, mode)
		}
		handle.Exec("PRAGMA journal_mode = WAL")
		time.Sleep(50 * time.Millisecond)
	}
}

func dsnFor(path string) string {
	params := url.Values{
		"_pragma": []string{
			"busy_timeout(10000)",
			"foreign_keys(1)",
		},
		"_txlock": []string{"immediate"},
	}
	escaped := strings.NewReplacer("?", "%3F", "#", "%23").Replace(filepath.ToSlash(path))
	return "file:" + escaped + "?" + params.Encode()
}
