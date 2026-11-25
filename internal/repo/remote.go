package repo

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

// The history can have a remote: another git repository it is pushed to after every entry and
// fetched from on request. The transports are the two the library speaks by itself, HTTPS and
// SSH; a remote by path would need a git program, which the system does not have. Nothing is
// ever merged: what is fetched is taken over only when it continues the local history (a
// fast-forward), and a push the remote refuses is reported, never forced.

// Remote says where the remote is and how to get in.
type Remote struct {
	// URL is https://host/path, ssh://[user@]host[:port]/path or user@host:path (http:// only
	// for a host on the system itself).
	URL string
	// Branch is the remote's branch (empty: "main").
	Branch string
	// Username and Token are the HTTPS credentials (a token, never a login password; the user
	// name is "git" when empty - the hosts that take tokens ignore it).
	Username string
	Token    string
	// Key is the SSH private key (PEM, as GenerateKey makes it).
	Key []byte
	// HostKey is the SSH host key the remote must present ("ssh-ed25519 AAAA…"). Empty means
	// first contact: the key the host presents is accepted and handed to OnHostKey, to be
	// remembered.
	HostKey   string
	OnHostKey func(key string)
}

// DefaultBranch is the remote's branch when none is named.
const DefaultBranch = "main"

// RemoteTimeout bounds one push or fetch.
const RemoteTimeout = 2 * time.Minute

// remoteRef is where a fetch keeps the remote's branch.
const remoteRef = plumbing.ReferenceName("refs/remotes/origin/fetched")

// RemoteError is a push or a fetch that did not work, with a code the page can act on:
// "auth", "not-found", "host-key", "rejected", "unreachable", "invalid".
type RemoteError struct {
	Code string
	Msg  string
}

func (e *RemoteError) Error() string { return e.Msg }

// Kind answers a remote URL's transport, "https" or "ssh", or why the URL cannot be used.
func Kind(url string) (string, error) {
	if strings.TrimSpace(url) != url || url == "" {
		return "", &RemoteError{"invalid", "the address is empty or has spaces around it"}
	}
	ep, err := transport.NewEndpoint(url)
	if err != nil {
		return "", &RemoteError{"invalid", "the address cannot be read: " + err.Error()}
	}
	switch ep.Protocol {
	case "https":
	case "http":
		if ip := net.ParseIP(strings.Trim(ep.Host, "[]")); ep.Host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", &RemoteError{"invalid", "http:// would send the token unprotected: use https:// or ssh"}
		}
	case "ssh":
		if ep.Host == "" || strings.Trim(ep.Path, "/") == "" {
			return "", &RemoteError{"invalid", "the address names no host or no repository"}
		}
		return "ssh", nil
	default:
		return "", &RemoteError{"invalid", "the address must be https://…, ssh://… or user@host:path"}
	}
	if ep.Host == "" || strings.Trim(ep.Path, "/") == "" {
		return "", &RemoteError{"invalid", "the address names no host or no repository"}
	}
	if ep.User != "" || ep.Password != "" {
		return "", &RemoteError{"invalid", "the address must not hold a user name or a password: they have their own fields"}
	}
	return "https", nil
}

// GenerateKey makes a new SSH key pair: the private key as PEM and the public key as the line
// a host takes as a deploy key.
func GenerateKey(comment string) (private []byte, public string, err error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return nil, "", err
	}
	line, err := PublicKey(pem.EncodeToMemory(block))
	if err != nil {
		return nil, "", err
	}
	if comment != "" {
		line += " " + comment
	}
	return pem.EncodeToMemory(block), line, nil
}

// PublicKey answers the public key line of a private key.
func PublicKey(private []byte) (string, error) {
	signer, err := ssh.ParsePrivateKey(private)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

// Fingerprint answers the SHA256 fingerprint of a public or host key line, as ssh prints it.
func Fingerprint(line string) string {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		return ""
	}
	return ssh.FingerprintSHA256(key)
}

func (rem Remote) branch() plumbing.ReferenceName {
	b := rem.Branch
	if b == "" {
		b = DefaultBranch
	}
	return plumbing.NewBranchReferenceName(b)
}

// auth builds the credentials for the remote's transport.
func (rem Remote) auth() (transport.AuthMethod, error) {
	kind, err := Kind(rem.URL)
	if err != nil {
		return nil, err
	}
	if kind == "https" {
		if rem.Token == "" {
			return nil, nil
		}
		user := rem.Username
		if user == "" {
			user = "git"
		}
		return &githttp.BasicAuth{Username: user, Password: rem.Token}, nil
	}
	if len(rem.Key) == 0 {
		return nil, &RemoteError{"auth", "there is no key for the remote yet"}
	}
	signer, err := ssh.ParsePrivateKey(rem.Key)
	if err != nil {
		return nil, &RemoteError{"auth", "the key for the remote cannot be read: " + err.Error()}
	}
	ep, _ := transport.NewEndpoint(rem.URL) // Kind read it already
	user := ep.User
	if user == "" {
		user = "git"
	}
	a := &gitssh.PublicKeys{User: user, Signer: signer}
	a.HostKeyCallback = func(_ string, _ net.Addr, key ssh.PublicKey) error {
		line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		if rem.HostKey == "" {
			if rem.OnHostKey != nil {
				rem.OnHostKey(line)
			}
			return nil
		}
		if line != strings.TrimSpace(rem.HostKey) {
			return &RemoteError{"host-key", "the host presents another key (" + ssh.FingerprintSHA256(key) + ") than the one remembered (" + Fingerprint(rem.HostKey) + ")"}
		}
		return nil
	}
	return a, nil
}

// remoteError says a transport's error in the page's terms.
func remoteError(err error) error {
	var re *RemoteError
	var netErr net.Error
	switch {
	case errors.As(err, &re):
		return re
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed), strings.Contains(err.Error(), "unable to authenticate"):
		return &RemoteError{"auth", "the remote refuses the credentials: " + err.Error()}
	case errors.Is(err, transport.ErrRepositoryNotFound):
		return &RemoteError{"not-found", "the remote has no such repository (or the credentials may not see it)"}
	case strings.Contains(err.Error(), "non-fast-forward"), errors.Is(err, git.ErrForceNeeded), strings.Contains(err.Error(), "failed to update ref"), strings.Contains(err.Error(), "command error on"):
		return &RemoteError{"rejected", "the remote did not take the push: it has entries this system does not have, or it protects the branch (" + err.Error() + ")"}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr):
		return &RemoteError{"unreachable", "the remote cannot be reached: " + err.Error()}
	}
	return &RemoteError{"unreachable", "the remote did not answer as expected: " + err.Error()}
}

// Push sends the local history to the remote's branch; pushed says whether there was anything
// to send. A history without an entry has nothing to push.
//
// A push and a fetch talk to another machine and may take as long as RemoteTimeout: they do
// not hold the lock a commit and a look at the history wait for (one of the two runs at a
// time, behind a lock of their own). The library's storage takes that - objects are written as
// whole files, and a reference is written to a file of its own and renamed into place (gitfs,
// B-11: the library itself rewrites it in place, and a push then read it empty) - the way two
// git programs share a repository.
func (r *Repo) Push(rem Remote) (pushed bool, err error) {
	r.net.Lock()
	defer r.net.Unlock()
	defer debug.FreeOSMemory()
	defer func() {
		if p := recover(); p != nil {
			pushed, err = false, &RemoteError{"unreachable", fmt.Sprintf("the push broke off (%v)", p)}
		}
	}()
	auth, err := rem.auth()
	if err != nil {
		return false, err
	}
	repo, err := r.open()
	if err != nil {
		return false, fmt.Errorf("history: %w", err)
	}
	parent, branch, _, _, err := headTree(repo)
	if err != nil {
		return false, fmt.Errorf("history: %w", err)
	}
	if parent.IsZero() {
		return false, nil
	}
	if branch == plumbing.HEAD {
		return false, &RemoteError{"invalid", "the history's HEAD names no branch (somebody checked an entry out by hand)"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), RemoteTimeout)
	defer cancel()
	remote := git.NewRemote(repo.Storer, &config.RemoteConfig{Name: "origin", URLs: []string{rem.URL}})
	err = remote.PushContext(ctx, &git.PushOptions{RemoteName: "origin", Auth: auth,
		RefSpecs: []config.RefSpec{config.RefSpec(branch.String() + ":" + rem.branch().String())}})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return false, nil
	}
	if err != nil {
		return false, remoteError(err)
	}
	return true, nil
}

// The relations between the local history and the remote's branch.
const (
	RelationEmpty    = "empty"    // the remote has no such branch yet: a push makes it
	RelationSame     = "same"     // both end in the same entry
	RelationAhead    = "ahead"    // the local history continues the remote's: a push sends it
	RelationBehind   = "behind"   // the remote continues the local history: it can be taken over
	RelationNew      = "new"      // the local history has no entry: the remote's can be taken over
	RelationDiverged = "diverged" // each has entries the other lacks: nothing is taken over
)

// Change is one file a fetched history would change.
type Change struct {
	Path string `json:"path"`
	// Kind is "added", "changed" or "removed".
	Kind string `json:"kind"`
	// Data is the file as the remote has it; nil when removed.
	Data []byte `json:"-"`
	// Problem says why the file cannot be taken (a symbolic link, a path outside the tree).
	Problem string `json:"problem,omitempty"`
}

// Fetched is what a fetch found.
type Fetched struct {
	Relation string `json:"relation"`
	// Local and Remote are the last entries' hashes ("" when there is none).
	Local  string `json:"local"`
	Remote string `json:"remote"`
	// Entries are the remote's entries the local history lacks, newest first, at most
	// MaxFetchedEntries; More says there are more.
	Entries []Entry `json:"entries"`
	More    bool    `json:"more,omitempty"`
	// Changes are the files that differ between the local history's last entry and the
	// remote's, when the remote's can be taken over (behind, new).
	Changes []Change `json:"changes"`
}

// MaxFetchedEntries bounds the entries a fetch lists.
const MaxFetchedEntries = 50

// Fetch gets the remote's branch and says how it relates to the local history. Nothing in the
// tree changes.
func (r *Repo) Fetch(rem Remote) (f *Fetched, err error) {
	r.net.Lock()
	defer r.net.Unlock()
	defer debug.FreeOSMemory()
	defer func() {
		if p := recover(); p != nil {
			f, err = nil, &RemoteError{"unreachable", fmt.Sprintf("the fetch broke off (%v)", p)}
		}
	}()
	auth, err := rem.auth()
	if err != nil {
		return nil, err
	}
	repo, err := r.open()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), RemoteTimeout)
	defer cancel()
	remote := git.NewRemote(repo.Storer, &config.RemoteConfig{Name: "origin", URLs: []string{rem.URL}})
	err = remote.FetchContext(ctx, &git.FetchOptions{RemoteName: "origin", Auth: auth, Tags: git.NoTags, Force: true,
		RefSpecs: []config.RefSpec{config.RefSpec("+" + rem.branch().String() + ":" + remoteRef.String())}})
	var noMatch git.NoMatchingRefSpecError
	switch {
	case err == nil, errors.Is(err, git.NoErrAlreadyUpToDate):
	case errors.Is(err, transport.ErrEmptyRemoteRepository), errors.As(err, &noMatch):
		_ = repo.Storer.RemoveReference(remoteRef)
		local, _, _, _, err := headTree(repo)
		if err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		return &Fetched{Relation: RelationEmpty, Local: hashText(local), Entries: []Entry{}, Changes: []Change{}}, nil
	default:
		return nil, remoteError(err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.relate(repo)
}

// LastFetch says how the local history relates to what the last fetch brought, as it is now,
// without asking the remote again.
func (r *Repo) LastFetch() (*Fetched, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer debug.FreeOSMemory()
	repo, err := r.open()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return r.relate(repo)
}

func hashText(h plumbing.Hash) string {
	if h.IsZero() {
		return ""
	}
	return h.String()
}

// relate compares the local history with what the last fetch brought.
func (r *Repo) relate(repo *git.Repository) (*Fetched, error) {
	local, _, files, _, err := headTree(repo)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	ref, err := repo.Storer.Reference(remoteRef)
	if err != nil {
		return nil, &RemoteError{"invalid", "nothing was fetched yet"}
	}
	f := &Fetched{Local: hashText(local), Remote: ref.Hash().String(), Entries: []Entry{}, Changes: []Change{}}
	theirs, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	switch {
	case local.IsZero():
		f.Relation = RelationNew
	case local == ref.Hash():
		f.Relation = RelationSame
		return f, nil
	default:
		ours, err := repo.CommitObject(local)
		if err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		if behind, err := ours.IsAncestor(theirs); err != nil {
			return nil, fmt.Errorf("history: %w", err)
		} else if behind {
			f.Relation = RelationBehind
		} else if ahead, err := theirs.IsAncestor(ours); err != nil {
			return nil, fmt.Errorf("history: %w", err)
		} else if ahead {
			f.Relation = RelationAhead
			return f, nil
		} else {
			f.Relation = RelationDiverged
		}
	}
	// the remote's entries the local history lacks
	seen := map[plumbing.Hash]bool{}
	if !local.IsZero() {
		ours, _ := repo.CommitObject(local) // read above
		_ = object.NewCommitPreorderIter(ours, nil, nil).ForEach(func(c *object.Commit) error {
			seen[c.Hash] = true
			return nil
		})
	}
	_ = object.NewCommitPreorderIter(theirs, seen, nil).ForEach(func(c *object.Commit) error {
		if len(f.Entries) == MaxFetchedEntries {
			f.More = true
			return io.EOF
		}
		f.Entries = append(f.Entries, entry(c))
		return nil
	})
	sort.SliceStable(f.Entries, func(a, b int) bool { return f.Entries[a].Time.After(f.Entries[b].Time) })
	if f.Relation == RelationDiverged {
		return f, nil
	}
	f.Changes, err = changes(repo, files, theirs)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	return f, nil
}

// pathProblem says why a path of a fetched tree must not be written into the tree.
func pathProblem(path string, mode filemode.FileMode) string {
	for _, part := range strings.Split(path, "/") {
		if strings.EqualFold(part, ".git") || part == ".." || part == "." || part == "" {
			return "the path is not one inside the tree"
		}
		if strings.HasPrefix(part, TempPrefix) {
			return "the name is one the addon keeps for files being written"
		}
	}
	switch mode {
	case filemode.Regular, filemode.Executable, filemode.Deprecated:
		return ""
	case filemode.Symlink:
		return "a symbolic link is not taken over"
	}
	return "not a file"
}

// changes lists what differs between the local files and a commit's.
func changes(repo *git.Repository, ours map[string]tracked, theirs *object.Commit) ([]Change, error) {
	tree, err := theirs.Tree()
	if err != nil {
		return nil, err
	}
	out := []Change{}
	have := map[string]bool{}
	w := object.NewTreeWalker(tree, true, nil)
	defer w.Close()
	for {
		name, e, err := w.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if e.Mode == filemode.Dir {
			continue
		}
		have[name] = true
		was, had := ours[name]
		if had && was.hash == e.Hash && was.mode == e.Mode {
			continue
		}
		c := Change{Path: name, Kind: "added", Problem: pathProblem(name, e.Mode)}
		if had {
			c.Kind = "changed"
		}
		if c.Problem == "" {
			blob, err := repo.BlobObject(e.Hash)
			if err != nil {
				return nil, err
			}
			rd, err := blob.Reader()
			if err != nil {
				return nil, err
			}
			c.Data, err = io.ReadAll(rd)
			_ = rd.Close()
			if err != nil {
				return nil, err
			}
		}
		out = append(out, c)
	}
	for name := range ours {
		if !have[name] {
			out = append(out, Change{Path: name, Kind: "removed"})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Path < out[b].Path })
	return out, nil
}

// ErrMoved is FastForward's answer when the local history or the fetched one is no longer what
// the caller looked at.
var ErrMoved = errors.New("the history has moved since the fetch")

// FastForward takes over what the last fetch brought: the remote's entry must be the given
// one and must continue the local history. The changed files are written into the tree (each
// as a whole, by renaming a finished file over it), removed ones removed, and the history
// then ends in the remote's entry. A file of the tree that is not what the local history's
// last entry says - changed and not recorded yet - stops everything before anything is
// written. The answer lists what changed.
func (r *Repo) FastForward(remote string) (done []Change, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer debug.FreeOSMemory()
	defer func() {
		if p := recover(); p != nil {
			done, err = nil, fmt.Errorf("history: taking over broke off (%v)", p)
		}
	}()
	repo, err := r.open()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	f, err := r.relate(repo)
	if err != nil {
		return nil, err
	}
	if f.Remote != remote || (f.Relation != RelationBehind && f.Relation != RelationNew) {
		return nil, ErrMoved
	}
	_, branch, files, _, err := headTree(repo)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	if branch == plumbing.HEAD {
		return nil, &RemoteError{"invalid", "the history's HEAD names no branch (somebody checked an entry out by hand)"}
	}
	// nothing unrecorded may be overwritten
	for _, c := range f.Changes {
		if c.Problem != "" {
			return nil, &RemoteError{"invalid", c.Path + ": " + c.Problem}
		}
		v, err := r.read(c.Path)
		if err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		was, had := files[c.Path]
		switch {
		case v == nil && !had:
		case v == nil || !had, plumbing.ComputeHash(plumbing.BlobObject, v.data) != was.hash:
			return nil, fmt.Errorf("history: %s was changed and is not in the history yet: %w", c.Path, ErrMoved)
		}
	}
	theirs, err := repo.CommitObject(plumbing.NewHash(remote))
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	tree, err := theirs.Tree()
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	read := map[string]*version{}
	for _, c := range f.Changes {
		full := filepath.Join(r.dir, filepath.FromSlash(c.Path))
		if c.Kind == "removed" {
			if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("history: %w", err)
			}
			delete(files, c.Path)
			continue
		}
		e, err := tree.FindEntry(c.Path)
		if err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		perm := os.FileMode(0o644)
		if e.Mode == filemode.Executable {
			perm = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		tmp := filepath.Join(filepath.Dir(full), TempPrefix+filepath.Base(full))
		if err := os.WriteFile(tmp, c.Data, perm); err != nil {
			return nil, fmt.Errorf("history: %w", err)
		}
		if err := os.Rename(tmp, full); err != nil {
			_ = os.Remove(tmp)
			return nil, fmt.Errorf("history: %w", err)
		}
		files[c.Path] = tracked{e.Hash, e.Mode}
		if v, err := r.read(c.Path); err == nil && v != nil {
			read[c.Path] = v
		}
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(branch, theirs.Hash)); err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	_ = writeIndex(repo, files, read)
	return f.Changes, nil
}
