package gitfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A reference goes into place whole at Close; nothing is left beside it, and other files are
// created as before.
func TestCreate(t *testing.T) {
	dir := t.TempDir()
	fs := New(osfs.New(dir))
	if billy.CapabilityCheck(fs, billy.ReadAndWriteCapability) {
		t.Error("the library would truncate a reference in place")
	}
	if !billy.CapabilityCheck(fs, billy.WriteCapability) {
		t.Error("writing at all is left out too")
	}
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"HEAD", "refs/heads/main", "refs/remotes/origin/main"} {
		f, err := fs.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if f.Name() != name {
			t.Errorf("the file's name %q, want %q", f.Name(), name)
		}
		_, _ = f.Write([]byte("new\n"))
		if name == "HEAD" && read(t, dir, "HEAD") != "old\n" {
			t.Error("the reference changed before the file was closed")
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if got := read(t, dir, name); got != "new\n" {
			t.Errorf("%s: %q", name, got)
		}
		if err := f.Close(); !errors.Is(err, os.ErrClosed) {
			t.Errorf("closed twice: %v", err)
		}
	}
	// the temporary files are gone, and none ever lay below refs/
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "HEAD" && e.Name() != "refs" {
			t.Errorf("left behind: %s", e.Name())
		}
	}
	// another file: created in place
	f, err := fs.Create("config")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte("x"))
	if read(t, dir, "config") != "x" {
		t.Error("config is written as before")
	}
	_ = f.Close()
	// OpenFile truncating a reference is a Create; reading it, or another file, is not
	f, err = fs.OpenFile("refs/heads/main", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.(*whole); !ok {
		t.Error("a truncating OpenFile of a reference is written in place")
	}
	_, _ = f.Write([]byte("third\n"))
	_ = f.Close()
	if read(t, dir, "refs/heads/main") != "third\n" {
		t.Error("OpenFile with O_TRUNC")
	}
	f, err = fs.OpenFile("refs/heads/main", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(f); string(b) != "third\n" {
		t.Errorf("read: %q", b)
	}
	_ = f.Close()
}

// Chroot keeps the wrapper: a folder of bare repositories, as the test host serves it.
func TestChroot(t *testing.T) {
	dir := t.TempDir()
	fs := New(osfs.New(dir))
	sub, err := fs.Chroot("haus.git")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sub.(refFS); !ok {
		t.Fatal("the folder below is not wrapped")
	}
	f, err := sub.Create("refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.(*whole); !ok {
		t.Error("a reference below is written in place")
	}
	_ = f.Close()
	if _, err := os.Stat(filepath.Join(dir, "haus.git", "refs", "heads", "main")); err != nil {
		t.Error(err)
	}
	if _, err := New(failing{osfs.New(dir)}).Chroot("x"); err == nil {
		t.Error("a Chroot that fails")
	}
}

// failing is a folder that refuses what the test names.
type failing struct{ billy.Filesystem }

var errRefused = errors.New("refused")

func (failing) Chroot(string) (billy.Filesystem, error)     { return nil, errRefused }
func (failing) MkdirAll(string, os.FileMode) error          { return errRefused }
func (failing) TempFile(string, string) (billy.File, error) { return nil, errRefused }

type noTemp struct{ billy.Filesystem }

func (noTemp) TempFile(string, string) (billy.File, error) { return nil, errRefused }

type noRename struct{ billy.Filesystem }

func (noRename) Rename(string, string) error { return errRefused }

type badClose struct{ billy.File }

func (badClose) Close() error { return errRefused }

type closeFails struct{ billy.Filesystem }

func (c closeFails) TempFile(dir, prefix string) (billy.File, error) {
	f, err := c.Filesystem.TempFile(dir, prefix)
	return badClose{f}, err
}

// What fails on the way says so and leaves no temporary file.
func TestCreateFails(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(failing{osfs.New(dir)}).Create("refs/heads/main"); err == nil {
		t.Error("a folder that cannot be made")
	}
	if _, err := New(noTemp{osfs.New(dir)}).Create("HEAD"); err == nil {
		t.Error("a temporary file that cannot be made")
	}
	for _, fs := range []billy.Filesystem{New(noRename{osfs.New(dir)}), New(closeFails{osfs.New(dir)})} {
		f, err := fs.Create("refs/heads/main")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); !errors.Is(err, errRefused) {
			t.Errorf("Close: %v", err)
		}
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "refs" {
			t.Errorf("left behind: %s", e.Name())
		}
	}
}

// The library's storage over the wrapper: a reader beside a writer of the same reference always
// reads a whole one (without the wrapper this fails within the first few rounds).
func TestReaderBesideWriter(t *testing.T) {
	st := filesystem.NewStorage(New(osfs.New(t.TempDir())), cache.NewObjectLRUDefault())
	name := plumbing.NewBranchReferenceName("main")
	ref := func(i int) *plumbing.Reference {
		var h plumbing.Hash
		h[0], h[1] = byte(i), byte(i>>8)
		return plumbing.NewHashReference(name, h)
	}
	if err := st.SetReference(ref(1)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 2; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if err := st.SetReference(ref(i)); err != nil {
				t.Errorf("write: %v", err)
				return
			}
		}
	}()
	for i := 0; i < 2000 && !t.Failed(); i++ {
		if _, err := st.Reference(name); err != nil {
			t.Errorf("read %d: %v", i, err)
		}
		if _, err := st.IterReferences(); err != nil {
			t.Errorf("list %d: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
	// the old check still holds: a reference that moved meanwhile is not overwritten
	if err := st.CheckAndSetReference(ref(9999), ref(1)); err == nil {
		t.Error("CheckAndSetReference over a reference that moved")
	}
}

// The library's temporary object files lie in the repository's folder, never in objects/pack;
// other temporary files stay where they are asked for.
func TestTempFile(t *testing.T) {
	dir := t.TempDir()
	fs := New(osfs.New(dir))
	if err := fs.MkdirAll("objects/pack", 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]string{"objects/pack": ".", "objects/pack/": ".", "objects": "objects", "": "."} {
		f, err := fs.TempFile(dir, "tmp_obj_")
		if err != nil {
			t.Fatal(err)
		}
		if got := filepath.Dir(f.Name()); got != want {
			t.Errorf("TempFile(%q): in %q, want %q", dir, got, want)
		}
		_ = f.Close()
	}
}

// B-11's second race: a reader that lists objects/pack while a writer's temporary object file
// is renamed away kept an empty pack index and found no object of a pack. Readers over a
// repository whose objects are in a pack, beside a writer of new objects, always find them
// (without the wrapper this fails within the first rounds).
func TestPackReadsBesideObjectWrites(t *testing.T) {
	src := t.TempDir()
	r, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := r.Worktree()
	for i := 0; i < 10; i++ {
		name := "f" + strconv.Itoa(i)
		f, _ := w.Filesystem.Create(name)
		_, _ = f.Write([]byte("content " + name))
		_ = f.Close()
		_, _ = w.Add(name)
		if _, err := w.Commit(name, &git.CommitOptions{Author: &object.Signature{Name: "t", When: time.Now()}}); err != nil {
			t.Fatal(err)
		}
	}
	dst := t.TempDir()
	if _, err := git.PlainClone(dst, true, &git.CloneOptions{URL: src}); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(dst, "objects", "pack", "pack-*.idx")); len(m) != 1 {
		t.Fatalf("the clone has no pack: %v", m)
	}
	storage := func() *filesystem.Storage {
		return filesystem.NewStorage(New(osfs.New(dst)), cache.NewObjectLRU(1<<20))
	}
	head, err := storage().Reference(plumbing.NewBranchReferenceName("master"))
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stop:
				written <- nil
				return
			default:
			}
			st := storage()
			o := st.NewEncodedObject()
			o.SetType(plumbing.BlobObject)
			ow, _ := o.Writer()
			_, _ = ow.Write([]byte("new " + strconv.Itoa(i)))
			_ = ow.Close()
			if _, err := st.SetEncodedObject(o); err != nil {
				written <- err
				return
			}
		}
	}()
	for i := 0; i < 300 && !t.Failed(); i++ {
		c, err := object.GetCommit(storage(), head.Hash())
		if err != nil {
			t.Errorf("round %d: the head: %v", i, err)
			break
		}
		if err := object.NewCommitPreorderIter(c, nil, nil).ForEach(func(c *object.Commit) error {
			_, err := c.Tree()
			return err
		}); err != nil {
			t.Errorf("round %d: the history: %v", i, err)
		}
	}
	close(stop)
	if err := <-written; err != nil {
		t.Fatalf("the writer: %v", err)
	}
}
