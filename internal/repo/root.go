package repo

import (
	"os"
	"path/filepath"
)

const StateDir = ".seraph"

type Root struct {
	Path   string
	Marker string
}

func (r Root) Found() bool { return r.Marker != "" }

func (r Root) StatePath() string { return filepath.Join(r.Path, StateDir) }

func (r Root) DBPath() string { return filepath.Join(r.StatePath(), "state.db") }

func Resolve(dir string) (Root, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Root{}, err
	}
	for cur := abs; ; {
		for _, marker := range []string{StateDir, ".git"} {
			if info, err := os.Stat(filepath.Join(cur, marker)); err == nil && info.IsDir() {
				return Root{Path: cur, Marker: marker}, nil
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return Root{Path: abs}, nil
		}
		cur = parent
	}
}
