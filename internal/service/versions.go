package service

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// The kinds of object that have versions.
const (
	KindProgram  = "program"
	KindVariable = "variable"
)

// versionPath answers the file of a program or a variable that exists.
func (s *Service) versionPath(kind, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch kind {
	case KindProgram:
		if s.set.Programs[key] != nil {
			return store.ProgramPath(key), nil
		}
	case KindVariable:
		if s.set.Variables[key] != nil {
			return store.VariablePath(key), nil
		}
	}
	return "", refuse("not-found", "there is no %s %s", kind, key)
}

// Versions lists the history entries that changed a program's or a variable's file, the newest
// first, at most limit (0: all). What was saved a moment ago is in it: the history's worker is
// waited for.
func (s *Service) Versions(kind, key string, limit int) ([]repo.Entry, error) {
	path, err := s.versionPath(kind, key)
	if err != nil {
		return nil, err
	}
	s.queue.Flush()
	return s.queue.Repo().History(path, limit)
}

// Version answers the file of a program or a variable as a history entry has it - the text
// that is compared. An entry that does not hold the file (the wrong hash, or the entry that
// removed it) is not found.
func (s *Service) Version(kind, key, hash string) ([]byte, error) {
	path, err := s.versionPath(kind, key)
	if err != nil {
		return nil, err
	}
	data, err := s.queue.Repo().FileAt(hash, path)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, refuse("not-found", "the history has no version %s of %s", hash, key)
	}
	return data, err
}

// Restore makes a program or a variable what a history entry has: the file of then is read and
// saved as a change of now - checked like any save (a program that names a variable which is
// gone since is refused), and with an entry of its own in the history. Nothing runs.
func (s *Service) Restore(kind, key, hash string, author repo.Author) error {
	data, err := s.Version(kind, key, hash)
	if err != nil {
		return err
	}
	what := "restored to its version " + hash[:7] // (a hash the history has is a whole one)
	if kind == KindProgram {
		def, err := model.DecodeProgram(store.ProgramPath(key), key, data)
		if err != nil {
			return refuse("invalid", "that version cannot be read any more: %v", err)
		}
		_, err = s.updateProgram(key, def, author, what)
		return err
	}
	def, err := model.DecodeVariable(store.VariablePath(key), key, data)
	if err != nil {
		return refuse("invalid", "that version cannot be read any more: %v", err)
	}
	_, err = s.updateVariable(key, def, nil, author, what)
	return err
}

// Current answers the file of a program or a variable as it is now.
func (s *Service) Current(kind, key string) ([]byte, error) {
	path, err := s.versionPath(kind, key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(s.tree.Dir, filepath.FromSlash(path)))
}
