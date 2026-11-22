package service

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/fakesys"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
)

func gitHost(t *testing.T) (*fakesys.GitRemote, string) {
	t.Helper()
	g := fakesys.NewGitRemote(t.TempDir())
	g.Token = "geheim"
	if err := g.Create("haus.git"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	t.Cleanup(g.Close)
	return g, srv.URL + "/haus.git"
}

func str(s string) *string { return &s }

// settled waits for the push worker: nothing pending, or an error noted.
func settled(t *testing.T, s *Service) RemoteInfo {
	t.Helper()
	for i := 0; i < 2000; i++ {
		s.queue.Flush()
		if info := s.Remote(); !info.Pending || info.Error != "" {
			return info
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the push never settled: %+v", s.Remote())
	return RemoteInfo{}
}

// The remote is set, checked and removed; its settings are the addon's alone and outside the
// tree; the page never sees the token.
func TestRemoteSettings(t *testing.T) {
	_, url := gitHost(t)
	dir := t.TempDir()
	s, _ := open(t, dir)
	if info := s.Remote(); info.URL != "" || info.Kind != "" || info.Pending {
		t.Fatalf("at first: %+v", info)
	}
	for _, f := range []func() error{s.PushRemote, s.NewRemoteKey, s.ForgetHostKey, func() error { _, err := s.FetchRemote(); return err }, func() error { _, err := s.PullRemote("x"); return err }} {
		if got := codeOf(f()); got != "not-found" {
			t.Errorf("without a remote: %s", got)
		}
	}
	long := strings.Repeat("x", MaxRemoteField+1)
	for _, in := range []RemoteInput{
		{URL: "ftp://example.org/x.git"},
		{URL: "http://example.org/x.git"},
		{URL: "https://example.org/x.git", Branch: "a b"},
		{URL: "https://example.org/x.git", Branch: "-x"},
		{URL: "https://example.org/x.git", Branch: "a..b"},
		{URL: "https://example.org/x.git", Branch: "x.lock"},
		{URL: "https://example.org/x.git", Username: "a:b"},
		{URL: "https://example.org/x.git", Token: str("a\nb")},
		{URL: "https://example.org/x.git", Token: &long},
		{URL: "https://example.org/" + long},
	} {
		if got := codeOf(s.SetRemote(in)); got != "invalid" {
			t.Errorf("%+v: %s", in, got)
		}
	}
	if s.Remote().URL != "" {
		t.Fatal("a refused remote was kept")
	}
	if err := s.SetRemote(RemoteInput{URL: " " + url + " ", Username: "haus", Token: str("geheim")}); err != nil {
		t.Fatal(err)
	}
	info := settled(t, s)
	if info.URL != url || info.Kind != "https" || info.Branch != "main" || info.Username != "haus" || !info.HasToken || info.PublicKey != "" || info.Error != "" {
		t.Fatalf("the remote: %+v", info)
	}
	path := filepath.Join(dir, "remote.json")
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the file: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tree", "remote.json")); err == nil {
		t.Error("the remote's settings are in the tree")
	}
	// a change that names no token keeps it; another branch
	if err := s.SetRemote(RemoteInput{URL: url, Branch: "zweit"}); err != nil {
		t.Fatal(err)
	}
	if info = settled(t, s); !info.HasToken || info.Branch != "zweit" || info.Username != "" || info.Error != "" {
		t.Fatalf("after a change: %+v", info)
	}
	// a wrong token: the push fails, the page is told, the notice names it; a fetch says so too
	if err := s.SetRemote(RemoteInput{URL: url, Token: str("falsch")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "Regen", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	if info = settled(t, s); info.ErrorCode != "auth" || !info.Pending || s.Notices().Remote == "" {
		t.Fatalf("a wrong token: %+v, notice %q", info, s.Notices().Remote)
	}
	if _, err := s.FetchRemote(); codeOf(err) != "remote-auth" {
		t.Errorf("a fetch with a wrong token: %v", err)
	}
	if got := codeOf(s.PushRemote()); got != "remote-auth" {
		t.Errorf("a push with a wrong token: %s", got)
	}
	// the right one again: pushed, the notice is gone
	if err := s.SetRemote(RemoteInput{URL: url, Token: str("geheim")}); err != nil {
		t.Fatal(err)
	}
	if info = settled(t, s); info.Error != "" || info.Pending || info.LastPush == nil || s.Notices().Remote != "" {
		t.Fatalf("the right token: %+v", info)
	}
	// the settings survive a restart
	s.Close()
	s, _ = open(t, dir)
	if info = settled(t, s); info.URL != url || !info.HasToken {
		t.Fatalf("after a restart: %+v", info)
	}
	// removed
	if err := s.SetRemote(RemoteInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file after the removal: %v", err)
	}
	if info = s.Remote(); info.URL != "" || info.Pending || info.LastPush != nil {
		t.Fatalf("removed: %+v", info)
	}
	if err := s.SetRemote(RemoteInput{}); err != nil {
		t.Errorf("removing what is not there: %v", err)
	}
	// a file that is damaged counts as no remote
	s.Close()
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, logs := open(t, dir)
	defer s2.Close()
	if s2.Remote().URL != "" || !strings.Contains(strings.Join(*logs, "\n"), "damaged") {
		t.Errorf("a damaged file: %+v %v", s2.Remote(), *logs)
	}
}

// Two systems share a remote: what one saves reaches the other when it fetches and takes it
// over; values stay; a remote that went another way is refused; a file the reader refuses
// stops the take-over.
func TestRemoteBetweenTwoSystems(t *testing.T) {
	_, url := gitHost(t)
	in := RemoteInput{URL: url, Token: str("geheim")}
	a, _ := open(t, t.TempDir())
	defer a.Close()
	if err := a.SetRemote(in); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateVariable(list("Modus", "aus", "ein"), "ein", admin); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateProgram(uses("schalten", "ein"), admin); err != nil {
		t.Fatal(err)
	}
	if info := settled(t, a); info.Error != "" || info.LastPush == nil {
		t.Fatalf("a: %+v", info)
	}
	p, err := a.FetchRemote()
	if err != nil || p.Relation != repo.RelationSame || p.CanApply || len(p.Changes) != 0 {
		t.Fatalf("a after its push: %+v %v", p, err)
	}

	// b is new: it takes everything over
	b, _ := open(t, t.TempDir())
	defer b.Close()
	if err := b.SetRemote(in); err != nil {
		t.Fatal(err)
	}
	settled(t, b)
	p, err = b.FetchRemote()
	if err != nil || p.Relation != repo.RelationNew || !p.CanApply || len(p.Changes) != 2 || len(p.Entries) != 2 || len(p.Problems) != 0 {
		t.Fatalf("b fetches: %+v %v", p, err)
	}
	if c := p.Changes[0]; c.Path != "programs/schalten.yaml" || c.What != "program" || c.Name != "schalten" || c.Kind != "added" {
		t.Errorf("the program's change: %+v", c)
	}
	if c := p.Changes[1]; c.What != "variable" || c.Name != "Modus" {
		t.Errorf("the variable's change: %+v", c)
	}
	if _, err := b.PullRemote("0123"); codeOf(err) != "moved" {
		t.Errorf("another entry than the fetched one: %v", err)
	}
	if _, err := b.PullRemote(p.Remote); err != nil {
		t.Fatal(err)
	}
	if len(b.Variables()) != 1 || len(b.Programs()) != 1 || b.Programs()[0].Program.Name != "schalten" {
		t.Fatalf("b after taking over: %d variables, %d programs", len(b.Variables()), len(b.Programs()))
	}
	if _, err := b.PullRemote(p.Remote); codeOf(err) != "moved" {
		t.Errorf("taking over twice: %v", err)
	}
	if p, err = b.FetchRemote(); err != nil || p.Relation != repo.RelationSame {
		t.Fatalf("b afterwards: %+v %v", p, err)
	}

	// b renames the variable and adds one; a is behind, takes it over and keeps its value
	modus := list("Betriebsart", "aus", "ein")
	if _, err := b.UpdateVariable("modus", modus, nil, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateVariable(&model.Variable{Name: "Neu", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	if err := b.SetSettings(model.Settings{PacingMS: 500}, admin); err != nil {
		t.Fatal(err)
	}
	if info := settled(t, b); info.Error != "" {
		t.Fatalf("b pushes: %+v", info)
	}
	if p, err = a.FetchRemote(); err != nil || p.Relation != repo.RelationBehind || !p.CanApply || len(p.Changes) != 3 || len(p.Entries) != 3 {
		t.Fatalf("a is behind: %+v %v", p, err)
	}
	what := map[string]string{}
	for _, c := range p.Changes {
		what[c.Path] = c.Kind + " " + c.What + " " + c.Name
	}
	if what["variables/modus.yaml"] != "changed variable Betriebsart" || what["variables/neu.yaml"] != "added variable Neu" || what["settings.yaml"] != "added settings " {
		t.Errorf("the changes: %v", what)
	}
	if _, err := a.PullRemote(p.Remote); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.Engine().Value("modus"); got != "ein" {
		t.Errorf("taking over lost a value: %v", got)
	}
	if len(a.Variables()) != 2 || a.Settings().PacingMS != 500 {
		t.Errorf("a after taking over: %d variables, pacing %d", len(a.Variables()), a.Settings().PacingMS)
	}
	// nothing new was recorded by taking over: a has nothing to push
	if info := settled(t, a); info.Error != "" || info.Pending {
		t.Fatalf("a after taking over: %+v", info)
	}
	if p, err = a.FetchRemote(); err != nil || p.Relation != repo.RelationSame {
		t.Fatalf("a afterwards: %+v %v", p, err)
	}

	// b removes the program; a, behind, would lose it
	if err := b.DeleteProgram("schalten", admin); err != nil {
		t.Fatal(err)
	}
	settled(t, b)
	if p, err = a.FetchRemote(); err != nil || !p.CanApply || len(p.Changes) != 1 || p.Changes[0].Kind != "removed" || p.Changes[0].Name != "schalten" {
		t.Fatalf("a removal: %+v %v", p, err)
	}
	// a goes its own way instead: the two have diverged
	if _, err := a.CreateVariable(&model.Variable{Name: "Eigen", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	if info := settled(t, a); info.ErrorCode != "rejected" || !info.Pending || !strings.Contains(a.Notices().Remote, "did not take the push") {
		t.Fatalf("a push onto a remote that went on: %+v", info)
	}
	if p, err = a.FetchRemote(); err != nil || p.Relation != repo.RelationDiverged || p.CanApply || len(p.Entries) != 1 {
		t.Fatalf("diverged: %+v %v", p, err)
	}
	if _, err := a.PullRemote(p.Remote); codeOf(err) != "moved" {
		t.Errorf("taking over a diverged remote: %v", err)
	}

	// somebody pushes files the reader refuses: b sees why and takes nothing
	c, _ := repo.Open(t.TempDir())
	rem := repo.Remote{URL: url, Token: "geheim"}
	f, err := c.Fetch(rem)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.FastForward(f.Remote); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{
		"programs/kaputt.yaml":       "name: Kaputt\nrules: nein\n",
		"programs/verwaist.yaml":     "name: Verwaist\nrules:\n  - when:\n      any:\n        - all:\n            - variable: gibt-es-nicht\n              is: true\n              trigger: change\n    then: []\n",
		"variables/schief.yaml":      "name: Schief\ntype: wurst\n",
		"settings.yaml":              "pacing_ms: -5\n",
		"notes/lies-mich.txt":        "frei\n",
		"variables/betriebsart.yaml": "name: Kopie\ntype: bool\n",
	} {
		full := filepath.Join(c.Dir(), filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Commit(nil, repo.Author{Name: "Hand"}, "by hand"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Push(rem); err != nil {
		t.Fatal(err)
	}
	p, err = b.FetchRemote()
	if err != nil || p.Relation != repo.RelationBehind || p.CanApply {
		t.Fatalf("b and the broken files: %+v %v", p, err)
	}
	problems := map[string]string{}
	for _, ch := range p.Changes {
		problems[ch.Path] = ch.Problem
	}
	if problems["programs/kaputt.yaml"] == "" || problems["variables/schief.yaml"] == "" || problems["settings.yaml"] == "" || problems["notes/lies-mich.txt"] != "" || problems["programs/verwaist.yaml"] != "" {
		t.Errorf("the files' problems: %v", problems)
	}
	if len(p.Problems) != 1 || !strings.HasPrefix(p.Problems[0], "Verwaist: ") {
		t.Errorf("the whole's problems: %v", p.Problems)
	}
	if _, err := b.PullRemote(p.Remote); codeOf(err) != "invalid" {
		t.Errorf("taking over broken files: %v", err)
	}
	if len(b.Programs()) != 0 || len(b.Variables()) != 2 {
		t.Error("something was taken over all the same")
	}
}

// A remote over SSH gets a key of its own; the host's key is remembered at first contact and
// a host that presents another one is refused until the key is forgotten.
func TestRemoteOverSSH(t *testing.T) {
	g, _ := gitHost(t)
	addr, hostKey, err := g.ListenSSH("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s, _ := open(t, dir)
	defer s.Close()
	s.remMu.Lock()
	s.remoteRetry = 20 * time.Millisecond
	s.remMu.Unlock()
	if _, err := s.CreateVariable(&model.Variable{Name: "Regen", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	url := "ssh://git@" + addr + "/haus.git"
	if err := s.SetRemote(RemoteInput{URL: url, Username: "ignored", Token: str("ignored")}); err != nil {
		t.Fatal(err)
	}
	// the host does not know the key yet: the push fails and is tried again by itself
	info := settled(t, s)
	if info.Kind != "ssh" || !strings.HasPrefix(info.PublicKey, "ssh-ed25519 ") || info.HasToken || info.Username != "" || info.ErrorCode != "auth" {
		t.Fatalf("before the key is entered: %+v", info)
	}
	g.Allow(info.PublicKey)
	for i := 0; i < 1000 && s.Remote().Pending; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	info = s.Remote()
	if info.Pending || info.Error != "" || info.HostKey != repo.Fingerprint(hostKey) {
		t.Fatalf("after the key is entered: %+v", info)
	}
	// the same address again keeps key and host key; a new key is another one
	if err := s.SetRemote(RemoteInput{URL: url}); err != nil {
		t.Fatal(err)
	}
	if again := settled(t, s); again.PublicKey != info.PublicKey || again.HostKey != info.HostKey {
		t.Fatalf("the same remote again: %+v", again)
	}
	if err := s.NewRemoteKey(); err != nil {
		t.Fatal(err)
	}
	if again := s.Remote(); again.PublicKey == info.PublicKey || again.PublicKey == "" {
		t.Fatal("the new key is the old one")
	}
	if _, err := s.FetchRemote(); codeOf(err) != "remote-auth" {
		t.Errorf("a fetch with a key the host does not know: %v", err)
	}
	g.Allow(s.Remote().PublicKey)
	// the host presents another key
	g.Close()
	g2 := fakesys.NewGitRemote(g.Root)
	defer g2.Close()
	g2.Allow(s.Remote().PublicKey)
	addr2, hostKey2, err := g2.ListenSSH(addr)
	if err != nil || addr2 != addr {
		t.Skipf("the address could not be taken again: %v", err)
	}
	if _, err := s.FetchRemote(); codeOf(err) != "remote-host-key" || s.Remote().ErrorCode != "host-key" {
		t.Fatalf("another host key: %v", err)
	}
	if err := s.ForgetHostKey(); err != nil {
		t.Fatal(err)
	}
	if info = s.Remote(); info.HostKey != "" || info.Error != "" {
		t.Fatalf("forgotten: %+v", info)
	}
	if p, err := s.FetchRemote(); err != nil || p.Relation != repo.RelationSame || s.Remote().HostKey != repo.Fingerprint(hostKey2) {
		t.Fatalf("after forgetting: %+v %v", p, err)
	}
	// a remote on another host forgets the host key; over https there is no key to make
	if err := s.SetRemote(RemoteInput{URL: "ssh://git@127.0.0.2:1/haus.git"}); err != nil {
		t.Fatal(err)
	}
	if info = s.Remote(); info.HostKey != "" || info.PublicKey == "" {
		t.Fatalf("another host: %+v", info)
	}
	if err := s.SetRemote(RemoteInput{URL: "https://example.invalid/x.git"}); err != nil {
		t.Fatal(err)
	}
	if got := codeOf(s.NewRemoteKey()); got != "invalid" {
		t.Errorf("a new key for https: %s", got)
	}
	if err := s.SetRemote(RemoteInput{}); err != nil {
		t.Fatal(err)
	}
}
