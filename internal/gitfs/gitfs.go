// Package gitfs is a repository's folder as the git library sees it, made safe for a reader beside
// a writer (B-11). Two things in the library break a reader that runs while another goroutine
// writes to the same repository:
//
//   - It rewrites a reference in place: it opens the file, truncates it and writes the new hash.
//     Its lock keeps two writers apart, not a reader: one that reads in between finds the file
//     empty and fails with "ref file is empty". Here a reference is written to a file of its own
//     and renamed over the old one when it is closed, as git does with its lock files: a reader
//     sees the old reference or the new one.
//   - It writes a new object (and a fetched pack) to a temporary file in objects/pack and renames
//     it away when done. A reader that lists objects/pack in that moment gets an error for the
//     file that is gone, and the library's object store then keeps an empty pack index: every
//     object in a pack is "object not found" for the rest of that reader's work. Here those
//     temporary files lie in the repository's folder itself, which the library never lists, so
//     objects/pack only ever gains files.
//
// A push and a fetch read the history without waiting for a commit (they talk to another
// machine and hold a lock of their own), so a push beside a commit now and then failed with one
// of the two.
package gitfs

import (
	"os"
	"path"
	"strings"

	"github.com/go-git/go-billy/v5"
)

// New wraps a repository's folder (.git, or a bare repository's folder; a folder of such
// repositories works too: Chroot wraps the repository's folder it hands out).
func New(fs billy.Filesystem) billy.Filesystem { return refFS{fs} }

type refFS struct{ billy.Filesystem }

// Capabilities leaves out reading and writing one file: the library then writes a reference
// with Create (checking the old one first) instead of truncating the file it reads.
func (f refFS) Capabilities() billy.Capability {
	return billy.Capabilities(f.Filesystem) &^ billy.ReadAndWriteCapability
}

// Chroot keeps the wrapper for the folder below.
func (f refFS) Chroot(p string) (billy.Filesystem, error) {
	fs, err := f.Filesystem.Chroot(p)
	if err != nil {
		return nil, err
	}
	return refFS{fs}, nil
}

// isRef says whether a path in a repository's folder is a reference git rewrites.
func isRef(name string) bool {
	name = path.Clean(strings.ReplaceAll(name, "\\", "/"))
	return name == "HEAD" || strings.HasPrefix(name, "refs/")
}

// Create writes a reference to a file of its own and renames it into place at Close; other
// files are created as before.
func (f refFS) Create(name string) (billy.File, error) {
	if !isRef(name) {
		return f.Filesystem.Create(name)
	}
	if dir := path.Dir(path.Clean(strings.ReplaceAll(name, "\\", "/"))); dir != "." {
		if err := f.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	// the temporary file lies in the repository's folder itself, not beside the reference: the
	// library reads every file below refs/ as a reference when it lists them
	tmp, err := f.Filesystem.TempFile(".", "ref-tmp-")
	if err != nil {
		return nil, err
	}
	return &whole{File: tmp, fs: f.Filesystem, to: name}, nil
}

// packDir is where the library puts the temporary files of new objects and packs.
const packDir = "objects/pack"

// TempFile puts the library's temporary object and pack files into the repository's folder
// instead of objects/pack (they are renamed to their place there all the same); others stay
// where they are asked for.
func (f refFS) TempFile(dir, prefix string) (billy.File, error) {
	if path.Clean(strings.ReplaceAll(dir, "\\", "/")) == packDir {
		dir = "."
	}
	return f.Filesystem.TempFile(dir, prefix)
}

// OpenFile with O_TRUNC on a reference is a Create: without ReadAndWriteCapability the library
// does not open references so, but a caller that does gets the same guarantee.
func (f refFS) OpenFile(name string, flag int, perm os.FileMode) (billy.File, error) {
	if isRef(name) && flag&os.O_TRUNC != 0 && flag&(os.O_WRONLY|os.O_RDWR) != 0 {
		return f.Create(name)
	}
	return f.Filesystem.OpenFile(name, flag, perm)
}

// whole is a reference being written: it goes into place when closed.
type whole struct {
	billy.File
	fs     billy.Filesystem
	to     string
	closed bool
}

// Name is the reference's name, not the temporary file's.
func (w *whole) Name() string { return w.to }

func (w *whole) Close() error {
	if w.closed {
		return os.ErrClosed
	}
	w.closed = true
	tmp := w.File.Name()
	if err := w.File.Close(); err != nil {
		_ = w.fs.Remove(tmp)
		return err
	}
	if err := w.fs.Rename(tmp, w.to); err != nil {
		_ = w.fs.Remove(tmp)
		return err
	}
	return nil
}
