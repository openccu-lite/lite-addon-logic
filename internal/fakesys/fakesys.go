// Package fakesys is the slice of openccu-lite's API the addon talks to, for the tests and the
// page's development: the kit's fake of lite-rpc, the state store and the metadata, set by the
// tests or over /dev/… by the e2e specs.
package fakesys

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource/fakelite"
)

// Server is the fake system.
type Server struct {
	// Lite is the kit's fake behind the routes this one does not answer itself.
	Lite *fakelite.Server

	mu sync.Mutex
	// names are the device names the addon wrote for its interface, by reference.
	names map[string]string
	// regadom is the ReGa database the system "kept" (nil: none).
	regadom []byte
	// writes are the setValue calls the addon sent, oldest first.
	writes []Write
	// hooks are the webhooks that arrived at /dev/hook/…, wakes the Wake-on-LAN packets'
	// hardware addresses that arrived at ListenWake's port; both oldest first.
	hooks []Hook
	wakes []string
	// gitHost is the git host of /dev/git/, made when first asked for.
	gitHost *GitRemote
}

// GitToken is the password the git host of /dev/git/ asks for.
const GitToken = "geheim"

// git answers the git host, making it in a temporary folder the first time.
func (s *Server) git() (*GitRemote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gitHost == nil {
		dir, err := os.MkdirTemp("", "fakesys-git-")
		if err != nil {
			return nil, err
		}
		s.gitHost = NewGitRemote(dir)
		s.gitHost.Token = GitToken
	}
	return s.gitHost, nil
}

// Hook is one request a webhook sent to /dev/hook/….
type Hook struct {
	Method string            `json:"method"`
	URL    string            `json:"url"`
	Header map[string]string `json:"header"`
	Body   string            `json:"body"`
}

// ListenWake takes Wake-on-LAN packets on a loopback UDP port (the specs' daemon sends them
// there instead of to port 9) and keeps the hardware address of each; /dev/wakes lists them.
func (s *Server) ListenWake(port int) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return err
	}
	go func() {
		buf := make([]byte, 256)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n == 102 {
				s.mu.Lock()
				s.wakes = append(s.wakes, net.HardwareAddr(buf[6:12]).String())
				s.mu.Unlock()
			}
		}
	}()
	return nil
}

// Write is one setValue the addon sent through lite-rpc - or one putParamset on VALUES, which
// has Values in place of Param and Value.
type Write struct {
	Interface string         `json:"interface"`
	Address   string         `json:"address"`
	Param     string         `json:"param,omitempty"`
	Value     any            `json:"value,omitempty"`
	Values    map[string]any `json:"values,omitempty"`
}

// plain takes a value out of the JSON form lite-rpc has for a double.
func plain(value any) any {
	if m, ok := value.(map[string]any); ok && len(m) == 1 {
		if d, ok := m["double"]; ok {
			return d
		}
	}
	return value
}

// Writes answers the setValue calls the addon sent.
func (s *Server) Writes() []Write {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Write(nil), s.writes...)
}

// Names answers the names the addon wrote through the interface's names route.
func (s *Server) Names() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.names {
		out[k] = v
	}
	return out
}

// SetRegadom sets the database GET /api/system/v1/regadom/file answers (nil: 404).
func (s *Server) SetRegadom(b []byte) {
	s.mu.Lock()
	s.regadom = b
	s.mu.Unlock()
}

// New makes a fake system.
func New() *Server {
	s := &Server{Lite: fakelite.New()}
	s.Lite.EventID = "boot1-0"
	return s
}

// Close stops the fake.
func (s *Server) Close() {
	s.Lite.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gitHost != nil {
		s.gitHost.Close()
		_ = os.RemoveAll(s.gitHost.Root)
	}
}

// Handler serves the fake system's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", s.Lite.Config.Handler)
	// lite-rpc's JSON-RPC: a setValue is recorded and answered; everything else is the kit's fake's
	mux.HandleFunc("POST /api/rpc/v1/json/{interface}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var req struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
			ID     any    `json:"id"`
		}
		set, isSet := map[string]any(nil), false
		if json.Unmarshal(body, &req) == nil && req.Method == "putParamset" && len(req.Params) == 3 && req.Params[1] == "VALUES" {
			set, isSet = req.Params[2].(map[string]any)
		}
		if isSet || (req.Method == "setValue" && len(req.Params) == 3) {
			wr := Write{Interface: r.PathValue("interface"), Address: fmt.Sprint(req.Params[0])}
			if isSet {
				wr.Values = map[string]any{}
				for k, v := range set {
					wr.Values[k] = plain(v)
				}
			} else {
				wr.Param, wr.Value = fmt.Sprint(req.Params[1]), plain(req.Params[2])
			}
			s.mu.Lock()
			s.writes = append(s.writes, wr)
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": nil})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		s.Lite.Config.Handler.ServeHTTP(w, r)
	})
	// the specs read what the addon wrote, and let a device report a value
	mux.HandleFunc("GET /dev/writes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Writes())
	})
	// a webhook's target: whatever arrives below /dev/hook/ is kept and answered with ?status=
	// (200 without)
	mux.HandleFunc("/dev/hook/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		h := Hook{Method: r.Method, URL: r.URL.RequestURI(), Header: map[string]string{}, Body: string(body)}
		for k := range r.Header {
			h.Header[k] = r.Header.Get(k)
		}
		s.mu.Lock()
		s.hooks = append(s.hooks, h)
		s.mu.Unlock()
		if status, err := strconv.Atoi(r.URL.Query().Get("status")); err == nil && status >= 200 && status <= 599 {
			w.WriteHeader(status)
		}
	})
	mux.HandleFunc("GET /dev/hooks", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(append([]Hook{}, s.hooks...))
	})
	mux.HandleFunc("GET /dev/wakes", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(append([]string{}, s.wakes...))
	})
	mux.HandleFunc("POST /dev/event", func(w http.ResponseWriter, r *http.Request) {
		var ev struct {
			Interface string `json:"interface"`
			Address   string `json:"address"`
			Param     string `json:"param"`
			Value     any    `json:"value"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&ev); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.Lite.PushEvent(ev.Interface, ev.Address, ev.Param, ev.Value)
	})
	// an addon names the devices of its own interface (scope meta:interface)
	mux.HandleFunc("POST /api/meta/v1/interfaces/{interface}/objects:bulk", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Set    map[string]map[string]string `json:"set"`
			Delete []string                     `json:"delete"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body); err != nil {
			http.Error(w, `{"error":{"code":"invalid-body"}}`, http.StatusUnprocessableEntity)
			return
		}
		s.mu.Lock()
		if s.names == nil {
			s.names = map[string]string{}
		}
		for ref, patch := range body.Set {
			s.names[ref] = patch["name"]
		}
		for _, ref := range body.Delete {
			delete(s.names, ref)
		}
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"revision":1}`))
	})
	// the names the metadata holds (scope meta:read): here only what the addon wrote, or a spec planted
	mux.HandleFunc("GET /api/meta/v1/objects", func(w http.ResponseWriter, r *http.Request) {
		objects := map[string]map[string]string{}
		for ref, name := range s.Names() {
			objects[ref] = map[string]string{"name": name}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"revision": 1, "objects": objects})
	})
	mux.HandleFunc("GET /dev/names", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.Names())
	})
	// the ReGa database kept after the switch (scope regadom:read); the specs set it over /dev/regadom
	mux.HandleFunc("GET /api/system/v1/regadom/file", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		b := s.regadom
		s.mu.Unlock()
		if b == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"no-regadom","message":"no ReGa database is kept"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("PUT /dev/regadom", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 64<<20))
		s.SetRegadom(b)
	})
	mux.HandleFunc("DELETE /dev/regadom", func(w http.ResponseWriter, r *http.Request) { s.SetRegadom(nil) })
	// a git host for the history's remote: PUT /dev/git/<name> makes an empty repository (anew
	// when there is one), POST /dev/git/<name>/commit puts a file into it as somebody else's
	// push would, and below /dev/git/<name>/ git's HTTP protocol answers, with the token
	// "geheim" as the password
	mux.HandleFunc("PUT /dev/git/{name}", func(w http.ResponseWriter, r *http.Request) {
		g, err := s.git()
		if err == nil {
			_ = g.Remove(r.PathValue("name"))
			err = g.Create(r.PathValue("name"))
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("POST /dev/git/{name}/commit", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Path, Text, Message string }
		g, err := s.git()
		if err == nil {
			err = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in)
		}
		if err == nil {
			err = g.Commit(r.PathValue("name"), in.Path, in.Text, in.Message)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/dev/git/", func(w http.ResponseWriter, r *http.Request) {
		g, err := s.git()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.StripPrefix("/dev/git", g).ServeHTTP(w, r)
	})
	return mux
}
