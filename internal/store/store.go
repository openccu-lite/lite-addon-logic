// Package store is the tree of files the programs and variables live in: programs/<key>.yaml and
// variables/<key>.yaml below one directory, which is also a git working tree (package repo). The
// files are the truth; nothing else is.
package store

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// The two folders of the tree.
const (
	ProgramsDir  = "programs"
	VariablesDir = "variables"
	ext          = ".yaml"
)

// Tree is the directory.
type Tree struct {
	Dir string
}

// Open makes the directory and its two folders.
func Open(dir string) (*Tree, error) {
	for _, d := range []string{dir, filepath.Join(dir, ProgramsDir), filepath.Join(dir, VariablesDir)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	return &Tree{Dir: dir}, nil
}

// ProgramPath and VariablePath are an object's path inside the tree, with forward slashes (as
// git names it).
func ProgramPath(key string) string  { return ProgramsDir + "/" + key + ext }
func VariablePath(key string) string { return VariablesDir + "/" + key + ext }

// FileError is a file that was not taken: the object it holds does not exist for the engine
// until the file is put right; everything else is loaded.
type FileError struct {
	// Path is the file inside the tree.
	Path string
	Err  error
}

func (e FileError) Error() string { return e.Err.Error() }

// Load reads every file. A file the reader refuses is reported and left out; the rest is the set.
func (t *Tree) Load() (*model.Set, []FileError, error) {
	set := model.NewSet()
	var bad []FileError
	keys, err := t.keys(VariablesDir)
	if err != nil {
		return nil, nil, err
	}
	for _, k := range keys {
		p := VariablePath(k)
		data, err := os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(p)))
		if err != nil {
			bad = append(bad, FileError{Path: p, Err: err})
			continue
		}
		v, err := model.DecodeVariable(p, k, data)
		if err != nil {
			bad = append(bad, FileError{Path: p, Err: err})
			continue
		}
		set.Variables[k] = v
	}
	if keys, err = t.keys(ProgramsDir); err != nil {
		return nil, nil, err
	}
	for _, k := range keys {
		p := ProgramPath(k)
		data, err := os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(p)))
		if err != nil {
			bad = append(bad, FileError{Path: p, Err: err})
			continue
		}
		prg, err := model.DecodeProgram(p, k, data)
		if err != nil {
			bad = append(bad, FileError{Path: p, Err: err})
			continue
		}
		set.Programs[k] = prg
	}
	return set, bad, nil
}

// keys lists the keys of a folder's .yaml files, sorted; a file whose name cannot be a key is
// still listed, so the reader reports it.
func (t *Tree) keys(folder string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(t.Dir, folder))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ext))
	}
	sort.Strings(out)
	return out, nil
}

// SaveProgram writes a program's file and answers its path and whether the bytes changed. The
// program is validated first, as the reader would.
func (t *Tree) SaveProgram(p *model.Program, names model.Namer) (string, bool, error) {
	if _, err := p.Check(); err != nil {
		return "", false, err
	}
	path := ProgramPath(p.Key)
	changed, err := t.write(path, model.EncodeProgram(p, names))
	return path, changed, err
}

// SaveVariable writes a variable's file.
func (t *Tree) SaveVariable(v *model.Variable) (string, bool, error) {
	if _, err := v.Check(); err != nil {
		return "", false, err
	}
	path := VariablePath(v.Key)
	changed, err := t.write(path, model.EncodeVariable(v))
	return path, changed, err
}

// DeleteProgram and DeleteVariable remove a file; a file that is not there is no error.
func (t *Tree) DeleteProgram(key string) (string, error) { return t.remove(ProgramPath(key), key) }

// DeleteVariable removes a variable's file.
func (t *Tree) DeleteVariable(key string) (string, error) { return t.remove(VariablePath(key), key) }

func (t *Tree) remove(path, key string) (string, error) {
	if !model.ValidKey(key) {
		return "", fmt.Errorf("%q is not a key", key)
	}
	err := os.Remove(filepath.Join(t.Dir, filepath.FromSlash(path)))
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return path, err
}

// CheckExtraPath says whether path can be another file of the tree (an import's ledger, the
// devices it saw): a relative path with forward slashes, no dot segment, nothing hidden, and
// not below programs/ or variables/, which hold only what the reader takes.
func CheckExtraPath(path string) error {
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "\\") {
		return fmt.Errorf("%q is not a path inside the tree", path)
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "" || strings.HasPrefix(seg, ".") {
			return fmt.Errorf("%q is not a path inside the tree", path)
		}
	}
	if first := strings.SplitN(path, "/", 2)[0]; first == ProgramsDir || first == VariablesDir {
		return fmt.Errorf("%q is below %s/, which holds only its own files", path, first)
	}
	return nil
}

// WriteExtra writes another file of the tree atomically and answers whether its bytes changed.
func (t *Tree) WriteExtra(path string, data []byte) (bool, error) {
	if err := CheckExtraPath(path); err != nil {
		return false, err
	}
	return t.write(path, data)
}

// Read answers a file of the tree as it is.
func (t *Tree) Read(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(path)))
}

// write replaces a file atomically (a temporary file beside it, synced, renamed over it), unless
// it holds these bytes already.
func (t *Tree) write(path string, data []byte) (bool, error) {
	full := filepath.Join(t.Dir, filepath.FromSlash(path))
	if old, err := os.ReadFile(full); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return false, err
	}
	return true, WriteFile(full, data, 0o644)
}

// WriteFile replaces a file atomically: a temporary file beside it, synced, then renamed over it.
func WriteFile(full string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, full); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// RemoveStale removes what a write that was cut off - a power cut between writing the
// temporary file and renaming it over the real one - left below root: the files WriteFile
// makes (".tmp-" and digits) in root and its folders, not below a ".git". Such a file is never
// the newest whole state of anything. It answers the files removed, relative to root; a root
// that is not there has none.
func RemoveStale(root string) ([]string, error) {
	var removed []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !staleName.MatchString(d.Name()) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		removed = append(removed, filepath.ToSlash(rel))
		return nil
	})
	return removed, err
}

// staleName is the name os.CreateTemp gives WriteFile's temporary file.
var staleName = regexp.MustCompile(`^\.tmp-[0-9]+$`)
