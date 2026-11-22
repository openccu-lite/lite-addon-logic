package repo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T) (*Repo, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tree")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	r.Now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	return r, dir
}

func TestCommitHistoryRestore(t *testing.T) {
	r, dir := open(t)
	if h, err := r.History("", 0); err != nil || h != nil {
		t.Fatalf("an empty repository's history: %v %v", h, err)
	}
	if h, err := r.Commit(nil, Author{}, "nothing"); err != nil || h != "" {
		t.Fatalf("a commit of nothing: %q %v", h, err)
	}
	write(t, dir, "programs/a.yaml", "name: A\n")
	write(t, dir, "programs/b.yaml", "name: B\n")
	write(t, dir, "variables/v.yaml", "name: V\n")
	first, err := r.Commit(nil, Author{Name: "Admin", Email: "admin@localhost"}, "Import")
	if err != nil || len(first) != 40 {
		t.Fatalf("import: %q %v", first, err)
	}
	write(t, dir, "programs/a.yaml", "name: A2\n")
	write(t, dir, "programs/b.yaml", "name: B2\n")
	second, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "Programm A geändert")
	if err != nil || second == "" || second == first {
		t.Fatalf("second: %q %v", second, err)
	}
	// only the named path went in: b is still changed in the tree and not in the commit
	if got, _ := r.FileAt(second, "programs/b.yaml"); string(got) != "name: B\n" {
		t.Errorf("b in the second commit: %q", got)
	}
	if h, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "again"); err != nil || h != "" {
		t.Errorf("an unchanged path: %q %v", h, err)
	}
	if err := os.Remove(filepath.Join(dir, "programs", "a.yaml")); err != nil {
		t.Fatal(err)
	}
	third, err := r.Commit([]string{"programs/a.yaml", "programs/b.yaml", "programs/never-there.yaml"}, Author{}, "A gelöscht, B geändert")
	if err != nil || third == "" {
		t.Fatalf("third: %q %v", third, err)
	}
	all, err := r.History("", 0)
	if err != nil || len(all) != 3 || all[0].Hash != third || all[2].Hash != first {
		t.Fatalf("history: %+v %v", all, err)
	}
	if all[2].Author != "Admin" || all[2].Message != "Import" || all[1].Author != defaultAuthor || !all[0].Time.After(all[1].Time) {
		t.Errorf("entries: %+v", all)
	}
	if two, _ := r.History("", 2); len(two) != 2 || two[0].Hash != third {
		t.Errorf("limit: %+v", two)
	}
	a, err := r.History("programs/a.yaml", 0)
	if err != nil || len(a) != 3 || a[0].Message != "A gelöscht, B geändert" || a[1].Message != "Programm A geändert" || a[2].Message != "Import" {
		t.Fatalf("a's history: %+v %v", a, err)
	}
	b, _ := r.History("programs/b.yaml", 0)
	if len(b) != 2 || b[0].Hash != third || b[1].Hash != first {
		t.Errorf("b's history: %+v", b)
	}
	if v, _ := r.History("variables/v.yaml", 0); len(v) != 1 || v[0].Hash != first {
		t.Errorf("v's history: %+v", v)
	}
	if n, _ := r.History("programs/never.yaml", 0); len(n) != 0 {
		t.Errorf("the history of a file that never was: %+v", n)
	}
	if n, _ := r.History("nofolder/x.yaml", 0); len(n) != 0 {
		t.Errorf("the history of a folder that never was: %+v", n)
	}
	if got, err := r.FileAt(first, "programs/a.yaml"); err != nil || string(got) != "name: A\n" {
		t.Errorf("a at the first commit: %q %v", got, err)
	}
	if got, err := r.FileAt(second, "programs/a.yaml"); err != nil || string(got) != "name: A2\n" {
		t.Errorf("a at the second commit: %q %v", got, err)
	}
	for _, c := range [][2]string{{third, "programs/a.yaml"}, {"nonsense", "programs/a.yaml"}, {strings.Repeat("0", 40), "programs/a.yaml"}} {
		if _, err := r.FileAt(c[0], c[1]); !errors.Is(err, ErrNotFound) {
			t.Errorf("FileAt(%s, %s): %v", c[0], c[1], err)
		}
	}
	for _, p := range []string{"", "/etc/passwd", "../x", ".git/config"} {
		if _, err := r.Commit([]string{p}, Author{}, "x"); err == nil {
			t.Errorf("the path %q was taken", p)
		}
	}
	// opened again, the history is there
	r2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := r2.History("", 0); len(h) != 3 {
		t.Errorf("after reopening: %+v", h)
	}
	if r2.now().IsZero() {
		t.Error("the clock")
	}
}

// A torn index (a power cut while it was rewritten) is rebuilt from HEAD; the files stay as they
// are and the next commit records them. Unfinished object files are removed.
func TestTornIndexAndLeftovers(t *testing.T) {
	r, dir := open(t)
	write(t, dir, "programs/a.yaml", "name: A\n")
	if _, err := r.Commit(nil, Author{}, "first"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "programs/a.yaml", "name: A2\n")
	if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("DIRC\x00\x00torn"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "after the power cut")
	if err != nil || h == "" {
		t.Fatalf("commit over a torn index: %q %v", h, err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "programs", "a.yaml")); string(got) != "name: A2\n" {
		t.Errorf("the repair touched the file: %q", got)
	}
	if got, _ := r.FileAt(h, "programs/a.yaml"); string(got) != "name: A2\n" {
		t.Errorf("the commit: %q", got)
	}
	// a torn index in a repository without a commit
	r2, dir2 := open(t)
	if err := os.WriteFile(filepath.Join(dir2, ".git", "index"), []byte("torn"), 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, dir2, "programs/a.yaml", "name: A\n")
	if h, err := r2.Commit(nil, Author{}, "first"); err != nil || h == "" {
		t.Fatalf("first commit over a torn index: %q %v", h, err)
	}
	pack := filepath.Join(dir, ".git", "objects", "pack")
	_ = os.MkdirAll(pack, 0o755)
	stale := filepath.Join(pack, "tmp_obj_123")
	_ = os.WriteFile(stale, []byte("x"), 0o644)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("the unfinished object file is still there")
	}
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if _, err := Open(filepath.Join(file, "below")); err == nil {
		t.Error("Open below a file")
	}
	// .git is a file with nonsense: not a repository
	broken := filepath.Join(dir, "broken")
	_ = os.MkdirAll(broken, 0o755)
	_ = os.WriteFile(filepath.Join(broken, ".git"), []byte("nonsense"), 0o644)
	if _, err := Open(broken); err == nil {
		t.Error("Open with a .git that is no repository")
	}
	// a repository that goes away under an open Repo
	r, tree := open(t)
	write(t, tree, "a.yaml", "x\n")
	if _, err := r.Commit(nil, Author{}, "first"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(tree, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(nil, Author{}, "x"); err == nil {
		t.Error("a commit without a repository")
	}
	if _, err := r.History("", 0); err == nil {
		t.Error("a history without a repository")
	}
	if _, err := r.FileAt(strings.Repeat("0", 40), "a.yaml"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("FileAt without a repository: %v", err)
	}
}

// A save never depends on the commit: a commit that fails keeps its paths, and the next commit
// records them as well.
func TestQueue(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	r, dir := open(t)
	q := NewQueue(r)
	var mu sync.Mutex
	var commits, failures []string
	q.OnCommit = func(hash, message string) { mu.Lock(); commits = append(commits, message); mu.Unlock() }
	q.OnError = func(err error) { mu.Lock(); failures = append(failures, err.Error()); mu.Unlock() }
	write(t, dir, "programs/a.yaml", "name: A\n")
	q.Enqueue([]string{"programs/a.yaml"}, Author{Name: "Admin"}, "A angelegt")
	q.Flush()
	if q.Err() != nil || len(commits) != 1 {
		t.Fatalf("first: %v %v", q.Err(), commits)
	}
	// nothing changed: no commit, no error
	q.Enqueue([]string{"programs/a.yaml"}, Author{}, "nichts")
	q.Flush()
	if q.Err() != nil || len(commits) != 1 {
		t.Fatalf("an unchanged path: %v %v", q.Err(), commits)
	}
	// the repository cannot be written: the commit fails, the save has happened anyway
	objects := filepath.Join(dir, ".git", "objects")
	if err := os.Chmod(objects, 0o555); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "programs/a.yaml", "name: A2\n")
	q.Enqueue([]string{"programs/a.yaml"}, Author{}, "A geändert")
	q.Flush()
	if q.Err() == nil || len(failures) != 1 || len(commits) != 1 {
		t.Fatalf("a commit into a read-only repository: %v %v %v", q.Err(), failures, commits)
	}
	if err := os.Chmod(objects, 0o755); err != nil {
		t.Fatal(err)
	}
	// the next commit, for another file, picks the first one up
	write(t, dir, "programs/b.yaml", "name: B\n")
	q.Enqueue([]string{"programs/b.yaml"}, Author{}, "B angelegt")
	q.Close()
	if q.Err() != nil || len(commits) != 2 || commits[1] != "B angelegt" {
		t.Fatalf("after the repair: %v %v", q.Err(), commits)
	}
	a, _ := r.History("programs/a.yaml", 0)
	if len(a) != 2 || a[0].Message != "B angelegt" {
		t.Errorf("a's history: %+v", a)
	}
	if got, _ := r.FileAt(a[0].Hash, "programs/a.yaml"); string(got) != "name: A2\n" {
		t.Errorf("a in the last commit: %q", got)
	}
}

// The library panicking inside a commit (it has, when the tree changed under it) is an error,
// not the end of the process: the queue reports it and records the files with the next commit.
func TestCommitThatPanics(t *testing.T) {
	r, dir := open(t)
	q := NewQueue(r)
	defer q.Close()
	var mu sync.Mutex
	var failures, commits []string
	q.OnError = func(err error) { mu.Lock(); failures = append(failures, err.Error()); mu.Unlock() }
	q.OnCommit = func(_, message string) { mu.Lock(); commits = append(commits, message); mu.Unlock() }
	write(t, dir, "programs/a.yaml", "name: A\n")
	r.beforeCommit = func() { panic("the tree changed") }
	q.Enqueue([]string{"programs/a.yaml"}, Author{Name: "Admin"}, "A angelegt")
	q.Flush()
	// Flush returns after the listeners were told
	mu.Lock()
	if len(failures) != 1 || !strings.Contains(failures[0], "broke off (the tree changed)") || len(commits) != 0 {
		t.Fatalf("failures %v, commits %v", failures, commits)
	}
	mu.Unlock()
	if q.Err() == nil {
		t.Error("the queue has no error")
	}
	r.beforeCommit = nil
	write(t, dir, "programs/b.yaml", "name: B\n")
	q.Enqueue([]string{"programs/b.yaml"}, Author{Name: "Admin"}, "B angelegt")
	q.Flush()
	mu.Lock()
	defer mu.Unlock()
	if q.Err() != nil || len(commits) != 1 {
		t.Fatalf("the next commit: %v %v", q.Err(), commits)
	}
	for _, p := range []string{"programs/a.yaml", "programs/b.yaml"} {
		if h, err := r.History(p, 5); err != nil || len(h) != 1 || h[0].Message != "B angelegt" {
			t.Errorf("%s is recorded with the next commit: %+v %v", p, h, err)
		}
	}
}

// atomic replaces a file the way the store does: a temporary file beside it, renamed over it.
func atomic(dir, path, content string) error {
	full := filepath.Join(dir, filepath.FromSlash(path))
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(content); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), full)
}

// The files of the tree are written while the history is recorded in the background. A commit
// takes each file as one whole version, whatever happens to it meanwhile: saves that hammer the
// tree - files replaced with other lengths, removed, made again - never make a commit fail, never
// put half a file or a temporary file into the history, and when the saves have ended one more
// commit leaves the history equal to the tree.
func TestCommitWhileTheTreeIsWritten(t *testing.T) {
	r, dir := open(t)
	paths := []string{"programs/a.yaml", "programs/b.yaml", "variables/v.yaml", "settings.yaml"}
	for _, p := range paths {
		write(t, dir, p, "0\n")
	}
	if _, err := r.Commit(nil, Author{}, "first"); err != nil {
		t.Fatal(err)
	}
	rounds := 200
	if testing.Short() {
		rounds = 60
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var writeErr error
	var mu sync.Mutex
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				p := paths[(i+w)%len(paths)]
				var err error
				if i%7 == 3 {
					if err = os.Remove(filepath.Join(dir, filepath.FromSlash(p))); errors.Is(err, os.ErrNotExist) {
						err = nil
					}
				} else {
					// a whole version is the same line again and again: half a file would show
					err = atomic(dir, p, strings.Repeat("version\n", 1+(i*37+w)%900))
				}
				if err != nil {
					mu.Lock()
					writeErr = err
					mu.Unlock()
					return
				}
			}
		}(w)
	}
	var commits []string
	for i := 0; i < rounds; i++ {
		named := paths
		if i%5 == 4 {
			named = nil // everything in the tree
		}
		h, err := r.Commit(named, Author{}, "while written")
		if err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("commit %d while the tree was written: %v", i, err)
		}
		if h != "" {
			commits = append(commits, h)
		}
	}
	close(stop)
	wg.Wait()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if len(commits) == 0 {
		t.Fatal("no commit was made")
	}
	for _, h := range commits {
		for _, p := range paths {
			got, err := r.FileAt(h, p)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if rest := strings.ReplaceAll(string(got), "version\n", ""); rest != "" && string(got) != "0\n" {
				t.Fatalf("commit %s has half a version of %s: %q", h, p, rest)
			}
		}
	}
	last, err := r.Commit(nil, Author{}, "after")
	if err != nil {
		t.Fatal(err)
	}
	if last == "" {
		last = commits[len(commits)-1]
	}
	for _, p := range paths {
		want, werr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		got, gerr := r.FileAt(last, p)
		if errors.Is(werr, os.ErrNotExist) != errors.Is(gerr, ErrNotFound) || string(got) != string(want) {
			t.Errorf("%s: the history has %d bytes (%v), the tree %d (%v)", p, len(got), gerr, len(want), werr)
		}
	}
	if h, err := r.Commit(nil, Author{}, "nothing more"); err != nil || h != "" {
		t.Errorf("a commit of a tree at rest: %q %v", h, err)
	}
	names, err := filesAt(r, last)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if strings.Contains(n, ".tmp-") {
			t.Errorf("the history has the temporary file %s", n)
		}
	}
}

// filesAt lists the files a commit has.
func filesAt(r *Repo, hash string) ([]string, error) {
	repo, err := r.open()
	if err != nil {
		return nil, err
	}
	c, err := repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, err
	}
	tree, err := c.Tree()
	if err != nil {
		return nil, err
	}
	var out []string
	w := object.NewTreeWalker(tree, true, nil)
	defer w.Close()
	for {
		name, e, err := w.Next()
		if err != nil {
			break
		}
		if e.Mode.IsFile() {
			out = append(out, name)
		}
	}
	return out, nil
}

// What a commit takes from the tree: files with their mode, links as links, folders below
// folders; not what a .gitignore names, not the temporary file of a save in flight; and it
// refuses what is no file.
func TestCommitTakesTheTreeAsItIs(t *testing.T) {
	r, dir := open(t)
	write(t, dir, "programs/a.yaml", "name: A\n")
	write(t, dir, "programs/deep/er/b.yaml", "name: B\n")
	write(t, dir, "programs/.tmp-123", "half a file")
	write(t, dir, ".gitignore", "# what is not history\n\n*.bak\r\n/private/\n")
	write(t, dir, "programs/a.bak", "old\n")
	write(t, dir, "private/secret.yaml", "x\n")
	write(t, dir, "notes/.gitignore", "draft-*\n")
	write(t, dir, "notes/draft-1.txt", "x\n")
	write(t, dir, "notes/kept.txt", "x\n")
	write(t, dir, "run.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("programs/a.yaml", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	first, err := r.Commit(nil, Author{}, "everything")
	if err != nil || first == "" {
		t.Fatalf("everything: %q %v", first, err)
	}
	names, err := filesAt(r, first)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(names, " "), ".gitignore link notes/.gitignore notes/kept.txt programs/a.yaml programs/deep/er/b.yaml run.sh"; got != want {
		t.Errorf("the commit has\n%s, want\n%s", got, want)
	}
	if got, _ := r.FileAt(first, "link"); string(got) != "programs/a.yaml" {
		t.Errorf("the link: %q", got)
	}
	// a named path is taken even when a .gitignore names it; a folder and a pipe are refused
	second, err := r.Commit([]string{"private/secret.yaml", "programs/deep/er/b.yaml"}, Author{}, "named")
	if err != nil || second == "" {
		t.Fatalf("named: %q %v", second, err)
	}
	if got, _ := r.FileAt(second, "private/secret.yaml"); string(got) != "x\n" {
		t.Errorf("the named file: %q", got)
	}
	if _, err := r.Commit([]string{"programs"}, Author{}, "a folder"); err == nil || !strings.Contains(err.Error(), "is a folder") {
		t.Errorf("a folder: %v", err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit([]string{"pipe"}, Author{}, "a pipe"); err == nil || !strings.Contains(err.Error(), "is not a file") {
		t.Errorf("a pipe: %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "pipe"))
	// a path below what is a file now, and a link that went away, are "not there"
	if h, err := r.Commit([]string{"run.sh/below.yaml"}, Author{}, "below a file"); err != nil || h != "" {
		t.Errorf("below a file: %q %v", h, err)
	}
	// a folder that empties leaves the history; the others keep their trees
	if err := os.RemoveAll(filepath.Join(dir, "programs", "deep")); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(dir, "link"))
	third, err := r.Commit(nil, Author{}, "removed")
	if err != nil || third == "" {
		t.Fatalf("removed: %q %v", third, err)
	}
	names, _ = filesAt(r, third)
	if got, want := strings.Join(names, " "), ".gitignore notes/.gitignore notes/kept.txt private/secret.yaml programs/a.yaml run.sh"; got != want {
		t.Errorf("after the removal the commit has\n%s, want\n%s", got, want)
	}
	// git itself finds the repository sound and the tree clean
	_ = os.Remove(filepath.Join(dir, "programs", ".tmp-123"))
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	git("fsck", "--strict")
	if out := git("status", "--porcelain"); out != "" {
		t.Errorf("git finds the tree changed:\n%s", out)
	}
	if out := git("ls-files", "-s", "run.sh", "private/secret.yaml"); !strings.Contains(out, "100755") || !strings.Contains(out, "100644") {
		t.Errorf("the modes in the index:\n%s", out)
	}
	// a commit onto a HEAD somebody detached by hand
	git("checkout", "-q", "--detach")
	write(t, dir, "programs/a.yaml", "name: A3\n")
	fourth, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "detached")
	if err != nil || fourth == "" {
		t.Fatalf("detached: %q %v", fourth, err)
	}
	if out := strings.TrimSpace(git("rev-parse", "HEAD")); out != fourth {
		t.Errorf("HEAD is %s, the commit %s", out, fourth)
	}
	if out := git("status", "--porcelain"); out != "" {
		t.Errorf("git finds the tree changed after the detached commit:\n%s", out)
	}
}

// A repository that cannot be written (a full disk, a read-only file system): the commit fails,
// and when it can be written again the same paths are recorded.
func TestCommitWithoutSpace(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root writes everywhere")
	}
	r, dir := open(t)
	write(t, dir, "programs/a.yaml", "name: A\n")
	if _, err := r.Commit(nil, Author{}, "first"); err != nil {
		t.Fatal(err)
	}
	objects := filepath.Join(dir, ".git", "objects")
	lock := func(mode os.FileMode) {
		_ = filepath.WalkDir(objects, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(p, mode)
			}
			return nil
		})
	}
	// the blob cannot be written
	write(t, dir, "programs/a.yaml", "name: A2\n")
	lock(0o555)
	t.Cleanup(func() { lock(0o755) })
	if h, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "no space"); err == nil {
		t.Fatalf("a commit into a repository that cannot be written: %q", h)
	}
	// the blob is there, the tree cannot be written
	lock(0o755)
	h := plumbing.ComputeHash(plumbing.BlobObject, []byte("name: A2\n")).String()
	_ = os.MkdirAll(filepath.Join(objects, h[:2]), 0o755)
	r.beforeCommit = func() { lock(0o555) }
	if h, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "no space for the tree"); err == nil {
		t.Fatalf("a commit whose tree cannot be written: %q", h)
	}
	lock(0o755)
	r.beforeCommit = nil
	done, err := r.Commit([]string{"programs/a.yaml"}, Author{}, "space again")
	if err != nil || done == "" {
		t.Fatalf("the commit after: %q %v", done, err)
	}
	if got, _ := r.FileAt(done, "programs/a.yaml"); string(got) != "name: A2\n" {
		t.Errorf("the commit after: %q", got)
	}
	if hist, _ := r.History("", 0); len(hist) != 2 {
		t.Errorf("the history has %d commits, want 2", len(hist))
	}
}
