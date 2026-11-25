package repo

import (
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/fakesys"
)

// host starts a git host with one empty repository and answers the remote that reaches it
// over HTTP with a token.
func host(t *testing.T) (*fakesys.GitRemote, Remote) {
	t.Helper()
	g := fakesys.NewGitRemote(t.TempDir())
	g.Token = "geheim"
	if err := g.Create("haus.git"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	t.Cleanup(g.Close)
	return g, Remote{URL: srv.URL + "/haus.git", Token: "geheim"}
}

func put2(t *testing.T, r *Repo, path, text, message string) string {
	t.Helper()
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := r.Commit([]string{path}, Author{Name: "t"}, message)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func code(err error) string {
	var re *RemoteError
	if errors.As(err, &re) {
		return re.Code
	}
	if err == nil {
		return ""
	}
	return "other: " + err.Error()
}

func TestPushFetchFastForward(t *testing.T) {
	_, rem := host(t)
	a, _ := Open(t.TempDir())
	// nothing to push, nothing to fetch
	if pushed, err := a.Push(rem); pushed || err != nil {
		t.Fatalf("a push of nothing: %v %v", pushed, err)
	}
	f, err := a.Fetch(rem)
	if err != nil || f.Relation != RelationEmpty || f.Local != "" {
		t.Fatalf("an empty remote: %+v %v", f, err)
	}
	h1 := put2(t, a, "programs/a.yaml", "name: A\n", "first")
	if f, err = a.Fetch(rem); err != nil || f.Relation != RelationEmpty || f.Local != h1 {
		t.Fatalf("an empty remote, a local entry: %+v %v", f, err)
	}
	if pushed, err := a.Push(rem); !pushed || err != nil {
		t.Fatalf("the first push: %v %v", pushed, err)
	}
	if pushed, err := a.Push(rem); pushed || err != nil {
		t.Fatalf("a push of what is there: %v %v", pushed, err)
	}
	if f, err = a.Fetch(rem); err != nil || f.Relation != RelationSame || f.Remote != h1 {
		t.Fatalf("after the push: %+v %v", f, err)
	}

	// a second system takes the history over from nothing
	b, _ := Open(t.TempDir())
	f, err = b.Fetch(rem)
	if err != nil || f.Relation != RelationNew || f.Remote != h1 || len(f.Entries) != 1 || f.Entries[0].Message != "first" ||
		len(f.Changes) != 1 || f.Changes[0].Path != "programs/a.yaml" || f.Changes[0].Kind != "added" || string(f.Changes[0].Data) != "name: A\n" {
		t.Fatalf("a new system: %+v %v", f, err)
	}
	if _, err := b.FastForward(h1[:10]); !errors.Is(err, ErrMoved) {
		t.Fatalf("another entry than the fetched one: %v", err)
	}
	done, err := b.FastForward(h1)
	if err != nil || len(done) != 1 {
		t.Fatalf("take over: %v %v", done, err)
	}
	if data, _ := os.ReadFile(filepath.Join(b.dir, "programs", "a.yaml")); string(data) != "name: A\n" {
		t.Fatalf("the file: %q", data)
	}
	if h, _ := b.History("", 0); len(h) != 1 || h[0].Hash != h1 {
		t.Fatalf("the history after taking over: %v", h)
	}
	// the tree is clean: nothing to record
	if h, err := b.Commit(nil, Author{}, "nothing"); h != "" || err != nil {
		t.Fatalf("a commit after taking over: %q %v", h, err)
	}
	if _, err := b.FastForward(h1); !errors.Is(err, ErrMoved) {
		t.Fatalf("taking over what is there: %v", err)
	}

	// b goes on: change, add, remove; a is behind and takes it over
	put2(t, b, "programs/a.yaml", "name: A2\n", "changed")
	put2(t, b, "variables/v.yaml", "name: V\n", "added")
	h4 := put2(t, b, "programs/b.yaml", "name: B\n", "added b")
	if f, err = b.Fetch(rem); err != nil || f.Relation != RelationAhead || len(f.Changes) != 0 {
		t.Fatalf("ahead: %+v %v", f, err)
	}
	if pushed, err := b.Push(rem); !pushed || err != nil {
		t.Fatal(pushed, err)
	}
	f, err = a.Fetch(rem)
	if err != nil || f.Relation != RelationBehind || f.Remote != h4 || len(f.Entries) != 3 || f.Entries[0].Message != "added b" || len(f.Changes) != 3 {
		t.Fatalf("behind: %+v %v", f, err)
	}
	kinds := map[string]string{}
	for _, c := range f.Changes {
		kinds[c.Path] = c.Kind
	}
	if kinds["programs/a.yaml"] != "changed" || kinds["variables/v.yaml"] != "added" || kinds["programs/b.yaml"] != "added" {
		t.Fatalf("changes: %v", kinds)
	}
	// a file changed and not recorded stops it, before anything is written
	if err := os.WriteFile(filepath.Join(a.dir, "programs", "a.yaml"), []byte("name: by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.FastForward(h4); !errors.Is(err, ErrMoved) || !strings.Contains(err.Error(), "programs/a.yaml") {
		t.Fatalf("an unrecorded change: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.dir, "variables", "v.yaml")); err == nil {
		t.Fatal("something was written all the same")
	}
	// recorded, the two have diverged: nothing is taken over, nothing is pushed
	if _, err := a.Commit(nil, Author{}, "by hand"); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Fetch(rem); err != nil || f.Relation != RelationDiverged || len(f.Entries) != 3 || len(f.Changes) != 0 {
		t.Fatalf("diverged: %+v %v", f, err)
	}
	if _, err := a.FastForward(h4); !errors.Is(err, ErrMoved) {
		t.Fatalf("diverged: %v", err)
	}
	if _, err := a.Push(rem); code(err) != "rejected" {
		t.Fatalf("a push onto a remote that went on: %v", err)
	}

	// a third system: behind with a removal
	c, _ := Open(t.TempDir())
	if f, err = c.Fetch(rem); err != nil || f.Relation != RelationNew {
		t.Fatal(f, err)
	}
	if _, err := c.FastForward(f.Remote); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(b.dir, "programs", "b.yaml")); err != nil {
		t.Fatal(err)
	}
	h5, _ := b.Commit([]string{"programs/b.yaml"}, Author{}, "removed b")
	if _, err := b.Push(rem); err != nil {
		t.Fatal(err)
	}
	if f, err = c.Fetch(rem); err != nil || f.Relation != RelationBehind || len(f.Changes) != 1 || f.Changes[0].Kind != "removed" {
		t.Fatalf("a removal: %+v %v", f, err)
	}
	if _, err := c.FastForward(h5); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.dir, "programs", "b.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the removed file: %v", err)
	}
	if h, err := c.Commit(nil, Author{}, "nothing"); h != "" || err != nil {
		t.Fatalf("a commit after taking over: %q %v", h, err)
	}
	// another branch of the same remote is empty until pushed
	other := rem
	other.Branch = "zweit"
	if f, err = c.Fetch(other); err != nil || f.Relation != RelationEmpty {
		t.Fatalf("a branch the remote lacks: %+v %v", f, err)
	}
	if pushed, err := c.Push(other); !pushed || err != nil {
		t.Fatal(pushed, err)
	}
	if f, err = c.Fetch(other); err != nil || f.Relation != RelationSame {
		t.Fatalf("the second branch: %+v %v", f, err)
	}
}

func TestRemoteRefusals(t *testing.T) {
	g, rem := host(t)
	a, _ := Open(t.TempDir())
	put2(t, a, "x.yaml", "x\n", "x")
	wrong := rem
	wrong.Token = "falsch"
	if _, err := a.Push(wrong); code(err) != "auth" {
		t.Errorf("a wrong token: %v", err)
	}
	if _, err := a.Fetch(wrong); code(err) != "auth" {
		t.Errorf("a wrong token: %v", err)
	}
	none := rem
	none.Token = ""
	if _, err := a.Fetch(none); code(err) != "auth" {
		t.Errorf("no token: %v", err)
	}
	gone := rem
	gone.URL = strings.Replace(rem.URL, "haus.git", "weg.git", 1)
	if _, err := a.Fetch(gone); code(err) != "not-found" {
		t.Errorf("no such repository: %v", err)
	}
	if _, err := a.Push(gone); code(err) != "not-found" {
		t.Errorf("no such repository: %v", err)
	}
	down := rem
	// (a port nobody listens on any more)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down.URL = "http://" + ln.Addr().String() + "/haus.git"
	ln.Close()
	if _, err := a.Fetch(down); code(err) != "unreachable" {
		t.Errorf("nobody there: %v", err)
	}
	if _, err := a.Push(down); code(err) != "unreachable" {
		t.Errorf("nobody there: %v", err)
	}
	for url, want := range map[string]string{
		"":                                   "invalid",
		" https://example.org/a.git":         "invalid",
		"http://example.org/a.git":           "invalid",
		"https://user:pw@example.org/a.git":  "invalid",
		"https://example.org":                "invalid",
		"/srv/git/a.git":                     "invalid",
		"file:///srv/git/a.git":              "invalid",
		"git://example.org/a.git":            "invalid",
		"https://example.org/a.git":          "",
		"http://localhost:3000/a.git":        "",
		"http://[::1]:3000/a.git":            "",
		"ssh://git@example.org:2222/a/b.git": "",
		"git@example.org:a/b.git":            "",
	} {
		kind, err := Kind(url)
		if code(err) != want {
			t.Errorf("Kind(%q): %q %v", url, kind, err)
		}
		if err == nil && (kind == "ssh") != (strings.HasPrefix(url, "ssh") || strings.HasPrefix(url, "git@")) {
			t.Errorf("Kind(%q) = %q", url, kind)
		}
	}
	bad := rem
	bad.URL = "git://example.org/a.git"
	if _, err := a.Push(bad); code(err) != "invalid" {
		t.Errorf("push: %v", err)
	}
	if _, err := a.Fetch(bad); code(err) != "invalid" {
		t.Errorf("fetch: %v", err)
	}
	if _, err := a.FastForward("0000000000000000000000000000000000000000"); code(err) != "invalid" {
		t.Errorf("take over before any fetch: %v", err)
	}
	// what a remote must never write into the tree
	for path, want := range map[string]string{".git/config": "not one inside", "a/.GIT/hooks/x": "not one inside", "programs/.tmp-x": "being written", "programs/a.yaml": ""} {
		if got := pathProblem(path, 0o100644); (want == "") != (got == "") || !strings.Contains(got, want) {
			t.Errorf("pathProblem(%q) = %q", path, got)
		}
	}
	if pathProblem("a", 0o120000) == "" || pathProblem("a", 0o160000) == "" || pathProblem("a", 0o100755) != "" {
		t.Error("a symbolic link and a submodule are refused, an executable file is taken")
	}
	_ = g
}

func TestSSHRemote(t *testing.T) {
	g, _ := host(t)
	addr, hostKey, err := g.ListenSSH("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	private, public, err := GenerateKey("logic@test")
	if err != nil || !strings.HasPrefix(public, "ssh-ed25519 ") || !strings.HasSuffix(public, " logic@test") {
		t.Fatalf("a key: %q %v", public, err)
	}
	if again, err := PublicKey(private); err != nil || again+" logic@test" != public {
		t.Fatalf("the public key of the private one: %q %v", again, err)
	}
	if fp := Fingerprint(public); !strings.HasPrefix(fp, "SHA256:") || Fingerprint("nonsense") != "" {
		t.Fatalf("fingerprint: %q", fp)
	}
	if _, err := PublicKey([]byte("no key")); err == nil {
		t.Fatal("a private key that is none")
	}
	a, _ := Open(t.TempDir())
	h := put2(t, a, "x.yaml", "x\n", "x")
	var seen string
	rem := Remote{URL: "ssh://git@" + addr + "/haus.git", Key: private, OnHostKey: func(k string) { seen = k }}
	// the key is not a deploy key yet
	if _, err := a.Push(rem); code(err) != "auth" {
		t.Fatalf("an unknown key: %v", err)
	}
	g.Allow(public)
	if pushed, err := a.Push(rem); !pushed || err != nil {
		t.Fatalf("push over ssh: %v %v", pushed, err)
	}
	if seen != hostKey {
		t.Fatalf("the host key at first contact: %q, the host's is %q", seen, hostKey)
	}
	rem.HostKey = hostKey
	f, err := a.Fetch(rem)
	if err != nil || f.Relation != RelationSame || f.Remote != h {
		t.Fatalf("fetch over ssh: %+v %v", f, err)
	}
	b, _ := Open(t.TempDir())
	if f, err = b.Fetch(rem); err != nil || f.Relation != RelationNew || len(f.Changes) != 1 {
		t.Fatalf("a new system over ssh: %+v %v", f, err)
	}
	// another host key than the remembered one
	_, otherPublic, _ := GenerateKey("")
	rem.HostKey = otherPublic
	if _, err := a.Fetch(rem); code(err) != "host-key" {
		t.Fatalf("a changed host key: %v", err)
	}
	if _, err := a.Push(rem); code(err) != "host-key" {
		t.Fatalf("a changed host key: %v", err)
	}
	rem.HostKey = hostKey
	// no key, a key that is none, no such repository
	rem.Key = nil
	if _, err := a.Fetch(rem); code(err) != "auth" {
		t.Fatalf("no key: %v", err)
	}
	rem.Key = []byte("nonsense")
	if _, err := a.Fetch(rem); code(err) != "auth" {
		t.Fatalf("a key that is none: %v", err)
	}
	rem.Key = private
	rem.URL = "ssh://git@" + addr + "/weg.git"
	if _, err := a.Fetch(rem); code(err) != "not-found" {
		t.Fatalf("no such repository over ssh: %v", err)
	}
}

// Somebody else's push with what must never be written into the tree stops the take-over, and
// a long history is listed up to a bound.
func TestRemoteLongAndHostile(t *testing.T) {
	g, rem := host(t)
	for i := 0; i < MaxFetchedEntries+3; i++ {
		if err := g.Commit("haus.git", "programs/a.yaml", "name: A"+strings.Repeat("x", i+1)+"\n", "entry"); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := Open(t.TempDir())
	f, err := a.Fetch(rem)
	if err != nil || f.Relation != RelationNew || len(f.Entries) != MaxFetchedEntries || !f.More || len(f.Changes) != 1 {
		t.Fatalf("a long history: %d entries, more %v, %v", len(f.Entries), f.More, err)
	}
	if again, err := a.LastFetch(); err != nil || again.Remote != f.Remote || again.Relation != RelationNew {
		t.Fatalf("the last fetch: %+v %v", again, err)
	}
	if _, err := a.FastForward(f.Remote); err != nil {
		t.Fatal(err)
	}
	// a file under a name the addon keeps for files being written, and its removal
	if err := g.Commit("haus.git", "programs/.tmp-evil", "boese\n", "hostile"); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Fetch(rem); err != nil || f.Relation != RelationBehind || len(f.Changes) != 1 || f.Changes[0].Problem == "" || f.Changes[0].Data != nil {
		t.Fatalf("a hostile path: %+v %v", f, err)
	}
	if _, err := a.FastForward(f.Remote); code(err) != "invalid" {
		t.Fatalf("taking a hostile path over: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.dir, "programs", ".tmp-evil")); err == nil {
		t.Fatal("the hostile file was written")
	}
	if err := g.Commit("haus.git", "programs/.tmp-evil", "", "gone again"); err != nil {
		t.Fatal(err)
	}
	if f, err = a.Fetch(rem); err != nil || len(f.Changes) != 0 || f.Relation != RelationBehind {
		t.Fatalf("after the hostile file went: %+v %v", f, err)
	}
	if _, err := a.FastForward(f.Remote); err != nil {
		t.Fatal(err)
	}
	if got := a.Dir(); got != a.dir || a.Time().IsZero() {
		t.Error("Dir and Time")
	}
}

// B-11: a push and a fetch do not wait for a commit (they hold a lock of their own), and the
// library rewrote a reference in place - truncated, then written - so that a push reading the
// branch while a commit moved it now and then found the file empty ("ref file is empty"). A
// reference is now written to a file of its own and renamed over the old one: a push beside a
// stream of commits always reads a whole reference.
func TestPushBesideCommits(t *testing.T) {
	_, rem := host(t)
	r, _ := Open(t.TempDir())
	put2(t, r, "programs/a.yaml", "name: A\n", "first")
	full := filepath.Join(r.dir, "programs", "a.yaml")
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 150; i++ {
			if err := os.WriteFile(full, []byte("name: A\nn: "+strconv.Itoa(i)+"\n"), 0o644); err != nil {
				done <- err
				return
			}
			if _, err := r.Commit([]string{"programs/a.yaml"}, Author{Name: "t"}, "next"); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("the commits: %v", err)
			}
			return
		default:
		}
		if _, err := r.Push(rem); err != nil && code(err) != "rejected" {
			t.Errorf("a push beside the commits: %v", err)
		}
		if _, err := r.Fetch(rem); err != nil {
			t.Errorf("a fetch beside the commits: %v", err)
		}
		if t.Failed() {
			<-done
			return
		}
	}
}
