package service

import (
	"os"
	"path/filepath"
)

// renameGit moves a tree's .git aside.
func renameGit(dir, to string) error {
	return os.Rename(filepath.Join(dir, ".git"), to)
}
