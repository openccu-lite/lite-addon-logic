// Package repo keeps the history of the tree of program and variable files: a git repository in
// the tree's directory, through go-git inside the daemon. The files are the truth and the
// repository a convenience - a save never depends on a commit, and the engine never reads from
// here.
//
// Four rules, each from a measurement on a Raspberry Pi 3 with 1,100 files:
//
//  1. The repository is opened per operation with a small object cache and the memory handed
//     back afterwards; kept open it grows from 12 MB to 26 MB and more.
//  2. A file's history is an own first-parent walk comparing the path's blob hash, never
//     go-git's log by file name, which is four times slower.
//  3. Repacking is rare and the caller's to schedule.
//  4. A commit that fails (a full disk, a read-only file system, a torn index) is reported and
//     the next one picks the change up; a torn index is rebuilt from HEAD.
package repo

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/openccu-lite/lite-addon-logic/internal/gitfs"
)

// Repo is the repository in a directory. Its methods may be called from several goroutines; one
// runs at a time.
type Repo struct {
	dir string
	mu  sync.Mutex
	// net is held by a push or a fetch (see Push)
	net sync.Mutex
	// Now is the clock for commit times (time.Now when nil).
	Now func() time.Time
	// beforeCommit is a test's hook, called just before the library builds the commit.
	beforeCommit func()
}

// Author is who a commit is by.
type Author struct {
	Name  string
	Email string
}

// Entry is one commit of a history.
type Entry struct {
	Hash    string    `json:"hash"`
	Time    time.Time `json:"time"`
	Author  string    `json:"author"`
	Message string    `json:"message"`
}

// ErrNotFound is FileAt's answer for a commit or a path that is not there.
var ErrNotFound = errors.New("not in the history")

const defaultAuthor = "lite-addon-logic"

// Open opens the repository in dir, making it when there is none, and puts right what a power
// cut may have left: a torn index and unfinished object files.
func Open(dir string) (*Repo, error) {
	r := &Repo{dir: dir}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		if _, err := git.PlainInit(dir, false); err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	r.sweep()
	if _, err := r.open(); err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return r, nil
}

// Dir is the directory of the tree the repository is in.
func (r *Repo) Dir() string { return r.dir }

// Time is the repository's clock: the time a commit made now would carry.
func (r *Repo) Time() time.Time { return r.now() }

func (r *Repo) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// sweep removes the temporary object files a failed commit leaves behind: in the repository's
// folder (gitfs puts them there) and, from versions before it, in objects/pack.
func (r *Repo) sweep() {
	matches, _ := filepath.Glob(filepath.Join(r.dir, ".git", "objects", "pack", "tmp_*"))
	more, _ := filepath.Glob(filepath.Join(r.dir, ".git", "tmp_*"))
	for _, m := range append(matches, more...) {
		_ = os.Remove(m)
	}
}

// open opens the repository with a 1 MiB object cache.
func (r *Repo) open() (*git.Repository, error) {
	wt := osfs.New(r.dir)
	dot, err := wt.Chroot(".git")
	if err != nil {
		return nil, err
	}
	// references written whole: a push reads the branch while a commit may move it (B-11)
	return git.Open(filesystem.NewStorage(gitfs.New(dot), cache.NewObjectLRU(1*cache.MiByte)), wt)
}

// tracked is a file of a commit's tree: its blob and its mode.
type tracked struct {
	hash plumbing.Hash
	mode filemode.FileMode
}

// headTree reads the last commit: its hash (zero when there is none), the branch HEAD names,
// its files and the hashes of its folders' trees.
func headTree(repo *git.Repository) (parent plumbing.Hash, branch plumbing.ReferenceName, files map[string]tracked, folders map[string]plumbing.Hash, err error) {
	files, folders = map[string]tracked{}, map[string]plumbing.Hash{}
	head, err := repo.Storer.Reference(plumbing.HEAD)
	if err != nil {
		return parent, "", nil, nil, err
	}
	branch = head.Target()
	if head.Type() != plumbing.SymbolicReference {
		// a detached HEAD (somebody checked a commit out by hand): the commit goes onto it
		branch = plumbing.HEAD
	}
	ref, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return parent, branch, files, folders, nil
	}
	if err != nil {
		return parent, "", nil, nil, err
	}
	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return parent, "", nil, nil, err
	}
	tree, err := c.Tree()
	if err != nil {
		return parent, "", nil, nil, err
	}
	w := object.NewTreeWalker(tree, true, nil)
	defer w.Close()
	for {
		name, e, err := w.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return parent, "", nil, nil, err
		}
		if e.Mode == filemode.Dir {
			folders[name] = e.Hash
		} else {
			files[name] = tracked{e.Hash, e.Mode}
		}
	}
	return ref.Hash(), branch, files, folders, nil
}

// version is a file of the tree as it was read: one whole version of it.
type version struct {
	data []byte
	mode filemode.FileMode
	info os.FileInfo
}

// read takes a file of the tree as it is at this moment; nil when it is not there. The tree's
// files are replaced by renaming a finished file over them, so what one read returns is always
// one whole version - which is why a commit reads every file exactly once and looks at the
// working tree in no other way.
func (r *Repo) read(path string) (*version, error) {
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(full)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return &version{data: []byte(target), mode: filemode.Symlink, info: info}, err
	case info.IsDir():
		return nil, fmt.Errorf("%q is a folder, not a file", path)
	case !info.Mode().IsRegular():
		return nil, fmt.Errorf("%q is not a file", path)
	}
	data, err := os.ReadFile(full)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	mode := filemode.Regular
	if info.Mode()&0o100 != 0 {
		mode = filemode.Executable
	}
	return &version{data: data, mode: mode, info: info}, err
}

// everything lists the files in the tree's directory: not the repository's own, not the
// temporary files of a save in flight, not what a .gitignore names.
func (r *Repo) everything() ([]string, error) {
	var patterns []gitignore.Pattern
	var out []string
	err := filepath.WalkDir(r.dir, func(full string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil // removed while the walk ran
			}
			return err
		}
		rel, _ := filepath.Rel(r.dir, full) // full is below dir
		var parts []string
		if rel != "." {
			parts = strings.Split(filepath.ToSlash(rel), "/")
			if rel == ".git" || gitignore.NewMatcher(patterns).Match(parts, d.IsDir()) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() {
			if !strings.HasPrefix(d.Name(), TempPrefix) {
				out = append(out, strings.Join(parts, "/"))
			}
			return nil
		}
		// a folder's .gitignore holds for what is below it
		if text, err := os.ReadFile(filepath.Join(full, ".gitignore")); err == nil {
			for _, line := range strings.Split(string(text), "\n") {
				if line = strings.TrimRight(line, "\r "); line != "" && !strings.HasPrefix(line, "#") {
					patterns = append(patterns, gitignore.ParsePattern(line, parts))
				}
			}
		}
		return nil
	})
	return out, err
}

// TempPrefix begins the name of a file that is being written: the store writes a file's new
// version beside it under such a name and renames it over the file. No commit takes one.
const TempPrefix = ".tmp-"

// put stores an object and answers its hash.
func put(repo *git.Repository, encode func(plumbing.EncodedObject) error) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	if err := encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return repo.Storer.SetEncodedObject(obj)
}

// writeTree stores the tree of a folder ("" is the root) and answers its hash; a folder
// nothing changed in keeps the tree it has.
func writeTree(repo *git.Repository, folder string, files map[string]tracked, folders map[string]plumbing.Hash, changed map[string]bool) (plumbing.Hash, error) {
	if h, ok := folders[folder]; ok && !changed[folder] {
		return h, nil
	}
	prefix := ""
	if folder != "" {
		prefix = folder + "/"
	}
	var entries []object.TreeEntry
	below := map[string]bool{}
	for path, f := range files {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		name, _, deeper := strings.Cut(path[len(prefix):], "/")
		if !deeper {
			entries = append(entries, object.TreeEntry{Name: name, Mode: f.mode, Hash: f.hash})
		} else if !below[name] {
			below[name] = true
			h, err := writeTree(repo, prefix+name, files, folders, changed)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			entries = append(entries, object.TreeEntry{Name: name, Mode: filemode.Dir, Hash: h})
		}
	}
	sort.Sort(object.TreeEntrySorter(entries))
	tree := &object.Tree{Entries: entries}
	return put(repo, tree.Encode)
}

// writeIndex leaves the repository's index saying what the commit says, so that git on the
// command line finds the tree clean. An entry keeps its file's size and time when the file is
// the commit's; every other entry has none, which makes git read that file again. An index
// that cannot be read (torn by a power cut) is written anew.
func writeIndex(repo *git.Repository, files map[string]tracked, read map[string]*version) error {
	old, err := repo.Storer.Index()
	if err != nil {
		old = &index.Index{}
	}
	idx := &index.Index{Version: 2}
	for path, f := range files {
		e := &index.Entry{Name: path, Hash: f.hash, Mode: f.mode}
		if v := read[path]; v != nil {
			e.ModifiedAt, e.Size = v.info.ModTime(), uint32(v.info.Size())
		} else if was, err := old.Entry(path); err == nil && was.Hash == f.hash && was.Mode == f.mode {
			e = was
		}
		idx.Entries = append(idx.Entries, e)
	}
	sort.Slice(idx.Entries, func(i, j int) bool { return idx.Entries[i].Name < idx.Entries[j].Name })
	return repo.Storer.SetIndex(idx)
}

// Commit records the given paths (inside the tree, forward slashes) as they are now - changed,
// new or removed - and answers the commit's hash, or "" when nothing differs from the last
// commit. No paths means everything in the tree.
//
// The tree may be written while this runs - the history is recorded in the background, a save
// never waits for it. The commit is therefore built without the library's working tree (which
// looks at a file several times and has been seen to fail, and to panic, when the file changed
// in between): every file is read once, as one whole version, and the commit is made of what
// was read. What is saved after that read is the next commit's.
func (r *Repo) Commit(paths []string, author Author, message string) (hash string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer debug.FreeOSMemory()
	// the library must never end the daemon: whatever breaks off inside it is an error like
	// any other, and the paths go with the next commit
	defer func() {
		if p := recover(); p != nil {
			r.sweep()
			hash, err = "", fmt.Errorf("history: the commit broke off (%v); it is made up with the next one", p)
		}
	}()
	repo, err := r.open()
	if err != nil {
		return "", fmt.Errorf("history: %w", err)
	}
	parent, branch, files, folders, err := headTree(repo)
	if err != nil {
		return "", fmt.Errorf("history: %w", err)
	}
	for _, p := range paths {
		if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.HasPrefix(p, ".git") {
			return "", fmt.Errorf("history: %q is not a path inside the tree", p)
		}
	}
	if len(paths) == 0 {
		if paths, err = r.everything(); err != nil {
			return "", fmt.Errorf("history: %w", err)
		}
		// and what the last commit has and the tree no longer
		seen := map[string]bool{}
		for _, p := range paths {
			seen[p] = true
		}
		for p := range files {
			if !seen[p] {
				paths = append(paths, p)
			}
		}
	}
	changed := map[string]bool{}
	touch := func(path string) {
		for dir := path; dir != ""; {
			if i := strings.LastIndex(dir, "/"); i >= 0 {
				dir = dir[:i]
			} else {
				dir = ""
			}
			changed[dir] = true
		}
	}
	read := map[string]*version{}
	for _, p := range paths {
		v, err := r.read(p)
		if err != nil {
			return "", fmt.Errorf("history: %w", err)
		}
		was, had := files[p]
		if v == nil {
			if had {
				delete(files, p)
				touch(p)
			}
			continue
		}
		read[p] = v
		h := plumbing.ComputeHash(plumbing.BlobObject, v.data)
		if had && was.hash == h && was.mode == v.mode {
			continue
		}
		if _, err := put(repo, func(o plumbing.EncodedObject) error {
			o.SetType(plumbing.BlobObject)
			w, _ := o.Writer() // an object in memory: its writer cannot fail
			_, _ = w.Write(v.data)
			return w.Close()
		}); err != nil {
			return "", fmt.Errorf("history: %w", err)
		}
		files[p] = tracked{h, v.mode}
		touch(p)
	}
	if len(changed) == 0 {
		return "", nil
	}
	if author.Name == "" {
		author.Name = defaultAuthor
	}
	if author.Email == "" {
		author.Email = defaultAuthor + "@localhost"
	}
	sig := object.Signature{Name: author.Name, Email: author.Email, When: r.now()}
	if r.beforeCommit != nil {
		r.beforeCommit()
	}
	root, err := writeTree(repo, "", files, folders, changed)
	if err != nil {
		r.sweep()
		return "", fmt.Errorf("history: %w", err)
	}
	c := &object.Commit{Author: sig, Committer: sig, Message: message, TreeHash: root}
	if !parent.IsZero() {
		c.ParentHashes = []plumbing.Hash{parent}
	}
	h, err := put(repo, c.Encode)
	if err != nil {
		r.sweep()
		return "", fmt.Errorf("history: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branch, h)); err != nil {
		return "", fmt.Errorf("history: %w", err)
	}
	// the commit is made; an index that cannot be written only costs git on the command line
	// a wrong "changed" until the next commit writes it
	_ = writeIndex(repo, files, read)
	return h.String(), nil
}

func entry(c *object.Commit) Entry {
	return Entry{Hash: c.Hash.String(), Time: c.Author.When, Author: c.Author.Name, Message: strings.TrimRight(c.Message, "\n")}
}

// blobAt answers the hash of the path's blob in a commit; the zero hash when the commit has no
// such file.
func blobAt(c *object.Commit, path string) (plumbing.Hash, error) {
	tree, err := c.Tree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	e, err := tree.FindEntry(path)
	if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
		return plumbing.ZeroHash, nil
	}
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return e.Hash, nil
}

// History lists the commits, newest first, at most limit (0: all). With a path, only the
// commits that changed that file - made, changed or removed it.
func (r *Repo) History(path string, limit int) ([]Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer debug.FreeOSMemory()
	repo, err := r.open()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	head, err := repo.Head()
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	c, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	var out []Entry
	for c != nil && (limit <= 0 || len(out) < limit) {
		var parent *object.Commit
		if c.NumParents() > 0 {
			if parent, err = c.Parent(0); err != nil {
				return nil, fmt.Errorf("history: %w", err)
			}
		}
		take := path == ""
		if !take {
			mine, err := blobAt(c, path)
			if err != nil {
				return nil, fmt.Errorf("history: %w", err)
			}
			before := plumbing.ZeroHash
			if parent != nil {
				if before, err = blobAt(parent, path); err != nil {
					return nil, fmt.Errorf("history: %w", err)
				}
			}
			take = mine != before
		}
		if take {
			out = append(out, entry(c))
		}
		c = parent
	}
	return out, nil
}

// FileAt answers a file's content as a commit has it; ErrNotFound when the commit is not there
// or does not have the file.
func (r *Repo) FileAt(hash, path string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer debug.FreeOSMemory()
	repo, err := r.open()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	if !plumbing.IsHash(hash) {
		return nil, ErrNotFound
	}
	c, err := repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, ErrNotFound
	}
	f, err := c.File(path)
	if err != nil {
		return nil, ErrNotFound
	}
	s, err := f.Contents()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return []byte(s), nil
}
