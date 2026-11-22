package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// The history's remote: another git repository the history is pushed to after every entry,
// in the background, and from which it can be taken over on request - only as a continuation
// of what is here (a fast-forward), and only after every changed file was read and checked
// like a save. A push that fails is a notice, never an error of the save.
//
// What reaches the remote is the tree: programs, variables, the settings, the import's
// records. The remote's own settings - the address, the token, the key - are kept beside the
// tree (remote.json, readable by the addon only) and never go anywhere.

// remoteFile is what remote.json holds.
type remoteFile struct {
	URL      string `json:"url"`
	Branch   string `json:"branch,omitempty"`
	Username string `json:"username,omitempty"`
	Token    string `json:"token,omitempty"`
	// Key is the SSH private key (PEM), HostKey the host key remembered at first contact.
	Key     string `json:"key,omitempty"`
	HostKey string `json:"host_key,omitempty"`
}

// RemoteInfo is the remote as the page sees it: never the token, never the private key.
type RemoteInfo struct {
	URL      string `json:"url"`
	Branch   string `json:"branch"`
	Username string `json:"username"`
	// Kind is "https" or "ssh" ("" when there is no remote).
	Kind     string `json:"kind"`
	HasToken bool   `json:"has_token"`
	// PublicKey is the deploy key to enter at the host (ssh), HostKey the fingerprint of the
	// host key remembered ("" before the first contact).
	PublicKey string `json:"public_key,omitempty"`
	HostKey   string `json:"host_key,omitempty"`
	// Pending says a push is due or under way; LastPush is the last one that worked, Error
	// what the last attempt (a push or a fetch) answered, with its code.
	Pending   bool       `json:"pending"`
	LastPush  *time.Time `json:"last_push,omitempty"`
	Error     string     `json:"error,omitempty"`
	ErrorCode string     `json:"error_code,omitempty"`
}

// RemoteInput is what the page sets. A nil Token keeps the one there is.
type RemoteInput struct {
	URL      string  `json:"url"`
	Branch   string  `json:"branch"`
	Username string  `json:"username"`
	Token    *string `json:"token"`
}

// MaxRemoteField bounds the address, the branch, the user name and the token.
const MaxRemoteField = 2048

// RemoteRetry is how long after a push that failed the next one is tried by itself.
const RemoteRetry = 5 * time.Minute

func (s *Service) remotePath() string { return filepath.Join(s.o.Dir, "remote.json") }

// loadRemote reads remote.json; a file that cannot be read is said and counts as no remote.
func (s *Service) loadRemote() {
	data, err := os.ReadFile(s.remotePath())
	if err != nil {
		if !os.IsNotExist(err) {
			s.logf("the remote's settings could not be read: %v", err)
		}
		return
	}
	var f remoteFile
	if err := json.Unmarshal(data, &f); err != nil {
		s.logf("the remote's settings are damaged: there is no remote until it is entered again")
		return
	}
	if f.URL != "" {
		s.remote = &f
	}
}

func (s *Service) saveRemote() error {
	if s.remote == nil {
		err := os.Remove(s.remotePath())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data, _ := json.MarshalIndent(s.remote, "", "  ")
	return store.WriteFile(s.remotePath(), append(data, '\n'), 0o600)
}

// Remote answers the remote's settings and state.
func (s *Service) Remote() RemoteInfo {
	s.remMu.Lock()
	defer s.remMu.Unlock()
	return s.remoteInfo()
}

func (s *Service) remoteInfo() RemoteInfo {
	info := RemoteInfo{Pending: s.remotePending, Error: s.remoteErr, ErrorCode: s.remoteErrCode}
	if !s.remotePushed.IsZero() {
		t := s.remotePushed
		info.LastPush = &t
	}
	f := s.remote
	if f == nil {
		info.Pending = false
		return info
	}
	info.URL, info.Branch, info.Username, info.HasToken = f.URL, f.Branch, f.Username, f.Token != ""
	if info.Branch == "" {
		info.Branch = repo.DefaultBranch
	}
	info.Kind, _ = repo.Kind(f.URL)
	if info.Kind == "ssh" {
		if f.Key != "" {
			info.PublicKey, _ = repo.PublicKey([]byte(f.Key))
		}
		info.HostKey = repo.Fingerprint(f.HostKey)
	}
	return info
}

// host answers the machine an address names: when it changes, the remembered host key goes.
func host(url string) string {
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return ""
	}
	return ep.Host
}

// branchOK says whether a text can be a branch's name.
func branchOK(b string) bool {
	if b == "" || strings.HasPrefix(b, "-") || strings.HasPrefix(b, "/") || strings.HasSuffix(b, "/") || strings.HasSuffix(b, ".lock") || strings.Contains(b, "..") || strings.Contains(b, "//") || strings.Contains(b, "@{") {
		return false
	}
	for _, r := range b {
		if r <= ' ' || r == 0x7f || strings.ContainsRune("~^:?*[\\", r) {
			return false
		}
	}
	return true
}

// SetRemote sets the remote, or removes it with an empty address. An SSH remote gets a key of
// its own the first time; a remote on another host forgets the host key of the one before.
// The history is pushed at once.
func (s *Service) SetRemote(in RemoteInput) error {
	in.URL, in.Branch, in.Username = strings.TrimSpace(in.URL), strings.TrimSpace(in.Branch), strings.TrimSpace(in.Username)
	s.remMu.Lock()
	if in.URL == "" {
		s.remote, s.remoteErr, s.remoteErrCode, s.remotePending = nil, "", "", false
		s.remotePushed = time.Time{}
		err := s.saveRemote()
		s.remMu.Unlock()
		return err
	}
	fail := func(format string, args ...any) error {
		s.remMu.Unlock()
		return refuse("invalid", format, args...)
	}
	kind, err := repo.Kind(in.URL)
	if err != nil {
		return fail("%s", err.Error())
	}
	if in.Branch != "" && !branchOK(in.Branch) {
		return fail("%q cannot be a branch's name", in.Branch)
	}
	if len(in.URL) > MaxRemoteField || len(in.Branch) > MaxRemoteField || len(in.Username) > MaxRemoteField || (in.Token != nil && len(*in.Token) > MaxRemoteField) {
		return fail("a field is longer than %d characters", MaxRemoteField)
	}
	if strings.ContainsAny(in.Username, "\r\n:") || (in.Token != nil && strings.ContainsAny(*in.Token, "\r\n")) {
		return fail("the user name or the token holds a character that cannot be sent")
	}
	f := remoteFile{URL: in.URL, Branch: in.Branch, Username: in.Username}
	if old := s.remote; old != nil {
		f.Token, f.Key = old.Token, old.Key
		if host(old.URL) == host(in.URL) {
			f.HostKey = old.HostKey
		}
	}
	if in.Token != nil {
		f.Token = *in.Token
	}
	if kind == "ssh" {
		f.Username, f.Token = "", ""
		if f.Key == "" {
			private, _, err := repo.GenerateKey("logic")
			if err != nil {
				s.remMu.Unlock()
				return err
			}
			f.Key = string(private)
		}
	} else {
		f.HostKey = ""
	}
	was := s.remote
	s.remote = &f
	if err := s.saveRemote(); err != nil {
		s.remote = was
		s.remMu.Unlock()
		return err
	}
	s.remoteErr, s.remoteErrCode = "", ""
	s.remMu.Unlock()
	s.pushSoon()
	return nil
}

// NewRemoteKey replaces the SSH key by a new one: the old one stops working once the host has
// the new one instead.
func (s *Service) NewRemoteKey() error {
	s.remMu.Lock()
	defer s.remMu.Unlock()
	if s.remote == nil {
		return refuse("not-found", "there is no remote")
	}
	if kind, _ := repo.Kind(s.remote.URL); kind != "ssh" {
		return refuse("invalid", "only a remote over ssh has a key")
	}
	private, _, err := repo.GenerateKey("logic")
	if err != nil {
		return err
	}
	was := s.remote.Key
	s.remote.Key = string(private)
	if err := s.saveRemote(); err != nil {
		s.remote.Key = was
		return err
	}
	return nil
}

// ForgetHostKey forgets the host key remembered: the next contact remembers the one the host
// presents then. For a host that was set up anew.
func (s *Service) ForgetHostKey() error {
	s.remMu.Lock()
	defer s.remMu.Unlock()
	if s.remote == nil {
		return refuse("not-found", "there is no remote")
	}
	was := s.remote.HostKey
	s.remote.HostKey = ""
	if err := s.saveRemote(); err != nil {
		s.remote.HostKey = was
		return err
	}
	if s.remoteErrCode == "host-key" {
		s.remoteErr, s.remoteErrCode = "", ""
	}
	return nil
}

// remoteFor answers the remote for the repository's calls; nil when there is none. A host key
// seen at first contact is remembered.
func (s *Service) remoteFor() *repo.Remote {
	s.remMu.Lock()
	defer s.remMu.Unlock()
	f := s.remote
	if f == nil {
		return nil
	}
	url := f.URL
	return &repo.Remote{URL: f.URL, Branch: f.Branch, Username: f.Username, Token: f.Token, Key: []byte(f.Key), HostKey: f.HostKey,
		OnHostKey: func(key string) {
			s.remMu.Lock()
			defer s.remMu.Unlock()
			if s.remote != nil && s.remote.URL == url && s.remote.HostKey == "" {
				s.remote.HostKey = key
				if err := s.saveRemote(); err != nil {
					s.logf("remote: the host key could not be stored: %v", err)
				}
				s.logf("remote: host key remembered at first contact: %s", repo.Fingerprint(key))
			}
		}}
}

// noteRemote keeps what an attempt answered.
func (s *Service) noteRemote(err error) {
	s.remMu.Lock()
	defer s.remMu.Unlock()
	if err == nil {
		s.remoteErr, s.remoteErrCode = "", ""
		return
	}
	var re *repo.RemoteError
	s.remoteErr, s.remoteErrCode = err.Error(), "failed"
	if errors.As(err, &re) {
		s.remoteErrCode = re.Code
	}
}

// remoteRefusal says a remote's error as a refusal with its code ("remote-auth", …).
func remoteRefusal(err error) error {
	var re *repo.RemoteError
	if errors.As(err, &re) {
		return &Error{Code: "remote-" + re.Code, Msg: re.Msg}
	}
	return err
}

// pushSoon asks the worker for a push.
func (s *Service) pushSoon() {
	s.remMu.Lock()
	on := s.remote != nil
	if on {
		s.remotePending = true
		s.remoteWanted++
	}
	s.remMu.Unlock()
	if !on {
		return
	}
	select {
	case s.pushWake <- struct{}{}:
	default:
	}
}

// pushWorker pushes when asked, one push at a time; after one that failed it tries again by
// itself.
func (s *Service) pushWorker() {
	defer close(s.pushDone)
	retry := time.NewTimer(time.Hour)
	retry.Stop()
	defer retry.Stop()
	for {
		select {
		case _, ok := <-s.pushWake:
			if !ok {
				return
			}
		case <-retry.C:
		}
		if err := s.PushRemote(); err != nil && code(err) != "not-found" {
			s.remMu.Lock()
			after := s.remoteRetry
			s.remMu.Unlock()
			retry.Reset(after)
		}
	}
}

// PushRemote pushes the history now and says whether it worked.
func (s *Service) PushRemote() error {
	rem := s.remoteFor()
	if rem == nil {
		return refuse("not-found", "there is no remote")
	}
	s.queue.Flush()
	s.remMu.Lock()
	wanted := s.remoteWanted
	s.remMu.Unlock()
	pushed, err := s.queue.Repo().Push(*rem)
	s.noteRemote(err)
	s.remMu.Lock()
	if err == nil {
		// (what was asked for while this push ran is the next push's)
		if s.remoteWanted == wanted {
			s.remotePending = false
		}
		if pushed {
			s.remotePushed = s.queue.Repo().Time()
		}
	}
	s.remMu.Unlock()
	if err != nil {
		s.logf("remote: push: %v", err)
		return remoteRefusal(err)
	}
	if pushed {
		s.logf("remote: pushed")
	}
	return nil
}

// RemoteChange is a file a fetched history would change, with what the reader says about it.
type RemoteChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	// What is "program", "variable", "settings" or "" (another file), Name the object's name
	// (the new one; of a removed object the one it has).
	What string `json:"what,omitempty"`
	Name string `json:"name,omitempty"`
	// Problem says why the file cannot be taken over.
	Problem string `json:"problem,omitempty"`
}

// RemotePreview is what a fetch found, checked: how the two histories relate, what taking the
// remote's over would change, and whether that can be done.
type RemotePreview struct {
	Relation string         `json:"relation"`
	Local    string         `json:"local"`
	Remote   string         `json:"remote"`
	Entries  []repo.Entry   `json:"entries"`
	More     bool           `json:"more,omitempty"`
	Changes  []RemoteChange `json:"changes"`
	// Problems are what stands against taking it over, beyond the files' own problems: a
	// program that would name a variable which is gone.
	Problems []string `json:"problems"`
	// CanApply says "take over" would work.
	CanApply bool `json:"can_apply"`
}

// FetchRemote gets the remote's branch and answers the preview. Nothing changes.
func (s *Service) FetchRemote() (*RemotePreview, error) {
	rem := s.remoteFor()
	if rem == nil {
		return nil, refuse("not-found", "there is no remote")
	}
	// what was put into the folder by hand belongs to the local history first
	s.queue.Enqueue(nil, repo.Author{}, outside)
	s.queue.Flush()
	f, err := s.queue.Repo().Fetch(*rem)
	s.noteRemote(err)
	if err != nil {
		s.logf("remote: fetch: %v", err)
		return nil, remoteRefusal(err)
	}
	return s.preview(f), nil
}

// preview checks what a fetch brought against the programs and variables there are.
func (s *Service) preview(f *repo.Fetched) *RemotePreview {
	p := &RemotePreview{Relation: f.Relation, Local: f.Local, Remote: f.Remote, Entries: f.Entries, More: f.More, Changes: []RemoteChange{}, Problems: []string{}}
	if f.Relation != repo.RelationBehind && f.Relation != repo.RelationNew {
		return p
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// the set as it would be
	set := model.NewSet()
	for k, v := range s.set.Programs {
		set.Programs[k] = v
	}
	for k, v := range s.set.Variables {
		set.Variables[k] = v
	}
	before := s.set.Problems()
	ok := true
	for _, c := range f.Changes {
		rc := RemoteChange{Path: c.Path, Kind: c.Kind, Problem: c.Problem}
		dir, file, _ := strings.Cut(c.Path, "/")
		key, isYAML := strings.CutSuffix(file, ".yaml")
		switch {
		case rc.Problem != "":
		case dir == store.ProgramsDir && isYAML && !strings.Contains(file, "/") && !strings.HasPrefix(file, "."):
			rc.What = "program"
			if c.Kind == "removed" {
				if was := set.Programs[key]; was != nil {
					rc.Name = was.Name
				}
				delete(set.Programs, key)
			} else if def, err := model.DecodeProgram(c.Path, key, c.Data); err != nil {
				rc.Problem = plainError(err)
			} else {
				rc.Name = def.Name
				set.Programs[key] = def
			}
		case dir == store.VariablesDir && isYAML && !strings.Contains(file, "/") && !strings.HasPrefix(file, "."):
			rc.What = "variable"
			if c.Kind == "removed" {
				if was := set.Variables[key]; was != nil {
					rc.Name = was.Name
				}
				delete(set.Variables, key)
			} else if def, err := model.DecodeVariable(c.Path, key, c.Data); err != nil {
				rc.Problem = plainError(err)
			} else {
				rc.Name = def.Name
				set.Variables[key] = def
			}
		case c.Path == model.SettingsPath:
			rc.What = "settings"
			if c.Kind != "removed" {
				if st, err := model.DecodeSettings(c.Data); err != nil {
					rc.Problem = plainError(err)
				} else if err := st.Check(); err != nil {
					rc.Problem = err.Error()
				}
			}
		}
		if rc.Problem != "" {
			ok = false
		}
		p.Changes = append(p.Changes, rc)
	}
	// what the whole would not hold together: only what is new counts - a program that is
	// faulty here already stays as it is
	after := set.Problems()
	keys := make([]string, 0, len(after))
	for k := range after {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Join(before[k], "\n") == strings.Join(after[k], "\n") {
			continue
		}
		name := k
		if def := set.Programs[k]; def != nil {
			name = def.Name
		}
		p.Problems = append(p.Problems, name+": "+strings.Join(after[k], "; "))
		ok = false
	}
	for _, k := range sortedVariables(set.Variables) {
		if problem := set.ComputedProblem(set.Variables[k]); problem != "" && s.set.Variables[k] != set.Variables[k] {
			p.Problems = append(p.Problems, set.Variables[k].Name+": "+problem)
			ok = false
		}
	}
	p.CanApply = ok
	return p
}

// PullRemote takes over what the last fetch brought, when it is still the entry the page was
// shown (remote) and continues the local history: the files are written, the history then
// ends in the remote's entry, and the programs and variables are read anew - nothing runs
// because of it, values and running delays stay.
func (s *Service) PullRemote(remote string) (*RemotePreview, error) {
	if s.remoteFor() == nil {
		return nil, refuse("not-found", "there is no remote")
	}
	s.queue.Enqueue(nil, repo.Author{}, outside)
	s.queue.Flush()
	rp := s.queue.Repo()
	f, err := rp.LastFetch()
	if err != nil {
		return nil, remoteRefusal(err)
	}
	p := s.preview(f)
	switch {
	case f.Remote != remote || (f.Relation != repo.RelationBehind && f.Relation != repo.RelationNew):
		return nil, refuse("moved", "the history has moved since it was fetched: fetch again")
	case !p.CanApply:
		return nil, refuse("invalid", "what was fetched cannot be taken over as it is")
	}
	if _, err := rp.FastForward(remote); err != nil {
		if errors.Is(err, repo.ErrMoved) {
			return nil, refuse("moved", "the history has moved since it was fetched: fetch again")
		}
		return nil, remoteRefusal(err)
	}
	s.logf("remote: took over %d changed files (now at %s)", len(p.Changes), remote[:7])
	if err := s.Reload(); err != nil {
		return nil, err
	}
	s.noteRemote(nil)
	return p, nil
}

// code answers a refusal's code ("" for another error).
func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
