package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newServer(t *testing.T, o Options) *httptest.Server {
	t.Helper()
	if o.WWW == "" {
		o.WWW = t.TempDir()
		_ = os.WriteFile(filepath.Join(o.WWW, "index.html"), []byte("<title>page</title>"), 0o644)
	}
	if o.BasePath == "" {
		o.BasePath = "/addons/logic/"
	}
	srv := httptest.NewServer(New(o))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return res.StatusCode, b.String(), res.Header
}

func TestHealthPageAndGuide(t *testing.T) {
	srv := newServer(t, Options{Version: "1.2.3", Started: time.Unix(1700000000, 0)})
	code, body, hdr := get(t, srv.URL+"/addons/logic/api/v1/health")
	var h struct {
		OK      bool   `json:"ok"`
		Version string `json:"version"`
		Started string `json:"started"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &h) != nil || !h.OK || h.Version != "1.2.3" || h.Started != "2023-11-14T22:13:20Z" {
		t.Fatalf("health: %d %s", code, body)
	}
	if hdr.Get("Cache-Control") != "no-store" {
		t.Errorf("health may be cached: %v", hdr)
	}
	if code, body, _ := get(t, srv.URL+"/addons/logic/"); code != 200 || !strings.Contains(body, "<title>page</title>") {
		t.Errorf("page: %d %s", code, body)
	}
	if code, body, _ := get(t, srv.URL+"/addons/logic/api/v1/nothing"); code != 404 || !strings.Contains(body, "not-found") {
		t.Errorf("unknown route: %d %s", code, body)
	}
	if code, _, _ := get(t, srv.URL+"/elsewhere"); code != 404 {
		t.Errorf("outside the base path: %d", code)
	}
	for lang, heading := range map[string]string{"de": "Überblick", "en": "Overview"} {
		code, body, hdr := get(t, srv.URL+"/addons/logic/docs/"+lang+"/")
		if code != 200 || !strings.Contains(body, `<h2 id="overview">`+heading+`</h2>`) || !strings.HasPrefix(hdr.Get("Content-Type"), "text/html") {
			t.Errorf("guide %s: %d %s", lang, code, body)
		}
	}
	if code, body, hdr := get(t, srv.URL+"/addons/logic/docs/screenshots/de/01-programs.png"); code != 200 || !strings.HasPrefix(body, "\x89PNG") || hdr.Get("Content-Type") != "image/png" {
		t.Errorf("a picture of the guide: %d %s", code, hdr.Get("Content-Type"))
	}
	for _, p := range []string{"fr/01-programs.png", "de/nothing.png", "de/01-programs.svg"} {
		if code, _, _ := get(t, srv.URL+"/addons/logic/docs/screenshots/"+p); code != 404 {
			t.Errorf("picture %s: %d", p, code)
		}
	}
	if code, _, _ := get(t, srv.URL+"/addons/logic/docs/fr/"); code != 404 {
		t.Errorf("a guide in French: %d", code)
	}
	if len(Routes) == 0 || Routes[0] != "GET /api/v1/health" {
		t.Errorf("routes %v", Routes)
	}
}

// A path with a dot segment is refused before anything looks at it, and the session check wraps
// everything under the base path.
func TestGuardAndWrap(t *testing.T) {
	wrapped := 0
	srv := newServer(t, Options{Wrap: func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrapped++; next.ServeHTTP(w, r) })
	}})
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/addons/logic/x", nil)
	req.URL.Path = "/addons/logic/../logic/api/v1/health"
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode == 200 || wrapped != 0 {
		t.Errorf("a dot segment got through: %d, wrapped %d", res.StatusCode, wrapped)
	}
	if code, _, _ := get(t, srv.URL+"/addons/logic/api/v1/health"); code != 200 || wrapped != 1 {
		t.Errorf("wrap: %d, %d", code, wrapped)
	}
	// a writing request without the kit's header is refused
	res, err = http.Post(srv.URL+"/addons/logic/api/v1/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("a write without the CSRF header: %d", res.StatusCode)
	}
}
