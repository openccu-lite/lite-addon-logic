package fakesys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"golang.org/x/crypto/ssh"
)

// GitRemote is a git host for the tests: bare repositories in a folder, served over HTTP (the
// "smart" protocol, with a token as the password) and over SSH (with deploy keys), by the git
// library itself - the test hosts have no git program either.
type GitRemote struct {
	// Root holds the repositories, one bare repository per name.
	Root string
	// Token is the password HTTP asks for ("" asks for none).
	Token string

	mu     sync.Mutex
	keys   map[string]bool // the public keys SSH lets in (authorized_keys lines without a comment)
	server transport.Transport
	signer ssh.Signer
	ln     net.Listener
}

// NewGitRemote makes a host over a folder.
func NewGitRemote(root string) *GitRemote {
	return &GitRemote{Root: root, keys: map[string]bool{}, server: server.NewServer(server.NewFilesystemLoader(osfs.New(root)))}
}

// Create makes an empty repository whose branch is "main".
func (g *GitRemote) Create(name string) error {
	repo, err := git.PlainInit(filepath.Join(g.Root, name), true)
	if err != nil {
		return err
	}
	return repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")))
}

// Commit puts a file into a repository's branch "main" as somebody else's push would: the
// file with the text (removed when the text is empty), one new entry on top.
func (g *GitRemote) Commit(name, path, text, message string) error {
	dot := osfs.New(filepath.Join(g.Root, name))
	repo, err := git.Open(filesystem.NewStorage(dot, cache.NewObjectLRUDefault()), memfs.New())
	if err != nil {
		return err
	}
	w, err := repo.Worktree()
	if err != nil {
		return err
	}
	if _, err := repo.Head(); err == nil {
		if err := w.Reset(&git.ResetOptions{Mode: git.HardReset}); err != nil {
			return err
		}
	}
	if text == "" {
		if _, err := w.Remove(path); err != nil {
			return err
		}
	} else {
		f, err := w.Filesystem.Create(path)
		if err != nil {
			return err
		}
		_, _ = f.Write([]byte(text))
		_ = f.Close()
		if _, err := w.Add(path); err != nil {
			return err
		}
	}
	_, err = w.Commit(message, &git.CommitOptions{Author: &object.Signature{Name: "Somebody else", Email: "else@example.org", When: time.Now()}})
	return err
}

// Remove removes a repository.
func (g *GitRemote) Remove(name string) error { return os.RemoveAll(filepath.Join(g.Root, name)) }

// Allow lets a public key in over SSH (the line a host takes as a deploy key).
func (g *GitRemote) Allow(public string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	f := strings.Fields(public)
	if len(f) >= 2 {
		g.keys[f[0]+" "+f[1]] = true
	}
}

func (g *GitRemote) endpoint(name string) (*transport.Endpoint, error) {
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "/") || strings.HasPrefix(name, ".") {
		return nil, transport.ErrRepositoryNotFound
	}
	return &transport.Endpoint{Protocol: "file", Path: name}, nil
}

// advertise writes the references of a service.
func (g *GitRemote) advertise(ctx context.Context, name, service string, w io.Writer) error {
	ep, err := g.endpoint(name)
	if err != nil {
		return err
	}
	var adv *packp.AdvRefs
	if service == transport.UploadPackServiceName {
		s, err := g.server.NewUploadPackSession(ep, nil)
		if err != nil {
			return err
		}
		defer s.Close()
		adv, err = s.AdvertisedReferencesContext(ctx)
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			// an empty repository advertises nothing
			return pktline.NewEncoder(w).Flush()
		}
		if err != nil {
			return err
		}
	} else {
		s, err := g.server.NewReceivePackSession(ep, nil)
		if err != nil {
			return err
		}
		defer s.Close()
		if adv, err = s.AdvertisedReferencesContext(ctx); err != nil {
			return err
		}
		if adv.IsEmpty() {
			// an empty repository still says what it can do, as git does: the null id, the
			// name "capabilities^{}" and the capabilities
			e := pktline.NewEncoder(w)
			if err := e.Encodef("%s capabilities^{}\x00%s\n", plumbing.ZeroHash, adv.Capabilities.String()); err != nil {
				return err
			}
			return e.Flush()
		}
	}
	return adv.Encode(w)
}

// serve answers a service's request.
func (g *GitRemote) serve(ctx context.Context, name, service string, r io.Reader, w io.Writer) error {
	ep, err := g.endpoint(name)
	if err != nil {
		return err
	}
	if service == transport.UploadPackServiceName {
		s, err := g.server.NewUploadPackSession(ep, nil)
		if err != nil {
			return err
		}
		defer s.Close()
		req := packp.NewUploadPackRequest()
		if err := req.Decode(r); err != nil {
			return err
		}
		resp, err := s.UploadPack(ctx, req)
		if err != nil {
			return err
		}
		defer resp.Close()
		return resp.Encode(w)
	}
	s, err := g.server.NewReceivePackSession(ep, nil)
	if err != nil {
		return err
	}
	defer s.Close()
	// (the library closes the reader it takes the pack from: over SSH that reader is the
	// channel the answer goes out on, so it gets one that only reads)
	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(struct{ io.Reader }{r}); err != nil {
		return err
	}
	status, err := s.ReceivePack(ctx, req)
	if status != nil {
		return status.Encode(w)
	}
	return err
}

// ServeHTTP is the host over HTTP: /<name>/info/refs?service=…, /<name>/git-upload-pack and
// /<name>/git-receive-pack, below whatever prefix the caller stripped.
func (g *GitRemote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.Token != "" {
		if _, pass, ok := r.BasicAuth(); !ok || pass != g.Token {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
	}
	path := strings.Trim(r.URL.Path, "/")
	name, rest, _ := strings.Cut(path, "/")
	if _, err := os.Stat(filepath.Join(g.Root, name)); err != nil || name == "" || strings.HasPrefix(name, ".") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	switch {
	case r.Method == http.MethodGet && rest == "info/refs":
		service := r.URL.Query().Get("service")
		if service != transport.UploadPackServiceName && service != transport.ReceivePackServiceName {
			http.Error(w, "the smart protocol only", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
		e := pktline.NewEncoder(w)
		_ = e.Encodef("# service=%s\n", service)
		_ = e.Flush()
		if err := g.advertise(r.Context(), name, service, w); err != nil {
			fmt.Fprintf(os.Stderr, "git remote: %s %s: %v\n", name, service, err)
		}
	case r.Method == http.MethodPost && (rest == transport.UploadPackServiceName || rest == transport.ReceivePackServiceName):
		w.Header().Set("Content-Type", "application/x-"+rest+"-result")
		if err := g.serve(r.Context(), name, rest, r.Body, w); err != nil {
			fmt.Fprintf(os.Stderr, "git remote: %s %s: %v\n", name, rest, err)
		}
	default:
		http.NotFound(w, r)
	}
}

// ListenSSH starts the host over SSH on an address ("127.0.0.1:0") and answers the address it
// listens on and its host key line. Close ends it.
func (g *GitRemote) ListenSSH(addr string) (string, string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return "", "", err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", "", err
	}
	g.signer, g.ln = signer, ln
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		g.mu.Lock()
		defer g.mu.Unlock()
		if g.keys[strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))] {
			return nil, nil
		}
		return nil, errors.New("this key is not a deploy key here")
	}}
	cfg.AddHostKey(signer)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go g.sshConn(conn, cfg)
		}
	}()
	return ln.Addr().String(), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))), nil
}

// Close ends the SSH listener.
func (g *GitRemote) Close() {
	if g.ln != nil {
		_ = g.ln.Close()
	}
}

func (g *GitRemote) sshConn(conn net.Conn, cfg *ssh.ServerConfig) {
	defer conn.Close()
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "sessions only")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			defer ch.Close()
			for req := range creqs {
				if req.Type != "exec" {
					_ = req.Reply(req.Type == "env", nil)
					continue
				}
				var cmd struct{ Line string }
				_ = ssh.Unmarshal(req.Payload, &cmd)
				service, arg, _ := strings.Cut(cmd.Line, " ")
				name := strings.Trim(arg, "'\"/ ")
				_ = req.Reply(true, nil)
				code := 0
				if service != transport.UploadPackServiceName && service != transport.ReceivePackServiceName {
					code = 1
				} else if _, serr := os.Stat(filepath.Join(g.Root, name)); serr != nil || name == "" {
					fmt.Fprintf(ch.Stderr(), "ERROR: Repository not found.\n")
					code = 1
				} else if err := g.advertise(context.Background(), name, service, ch); err != nil {
					fmt.Fprintf(os.Stderr, "git remote (ssh): %s %s: %v\n", name, service, err)
					code = 1
				} else if err := g.serve(context.Background(), name, service, ch, ch); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, packp.ErrEmpty) {
					// (a client that wants nothing just hangs up: that is no error)
					fmt.Fprintf(os.Stderr, "git remote (ssh): %s %s: %v\n", name, service, err)
				}
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{uint32(code)}))
				return
			}
		}()
	}
}
