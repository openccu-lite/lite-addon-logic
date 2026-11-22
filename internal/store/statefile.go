package store

import (
	"errors"
	"io/fs"
	"os"
)

// StateFile is the file the daemon keeps what it remembers in (the variables' values, what is
// pending), in two generations: a write keeps the file before it as <path>.1, so a file torn by
// a power cut or damaged on the card leaves the one before.
type StateFile struct {
	Path string
}

// Save replaces the file atomically and keeps the one before.
func (s StateFile) Save(data []byte) error {
	if _, err := os.Stat(s.Path); err == nil {
		// a link, not a rename: the current file stays in place until the new one is over it
		_ = os.Remove(s.Path + ".1")
		if err := os.Link(s.Path, s.Path+".1"); err != nil {
			return err
		}
	}
	return WriteFile(s.Path, data, 0o600)
}

// Load answers the newest generation that ok takes (ok is the caller's check that the bytes
// are a state), and which one it was: 0 the file, 1 the one before; -1 with nil bytes when there
// is none - a first start, or both are damaged (damaged then says so).
func (s StateFile) Load(ok func([]byte) bool) (data []byte, generation int, damaged bool) {
	for gen, p := range []string{s.Path, s.Path + ".1"} {
		b, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !ok(b) {
			damaged = true
			continue
		}
		return b, gen, damaged
	}
	return nil, -1, damaged
}
