// Package httpapi is the addon's own HTTP server: the Svelte page's files and its JSON API under
// one base path (/addons/logic/), which lighttpd proxies to the loopback port. Every request
// carries the shell's session, confirmed through the kit (administrators only); writes carry the
// kit's CSRF header. The API is the page's own, not a contract for other software: that is the
// interface "Logic" behind lite-rpc.
package httpapi

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/docs"
	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// Options configure the server.
type Options struct {
	// BasePath is where the page is served, /addons/logic (no trailing slash).
	BasePath string
	// WWW is the directory with the built page.
	WWW string
	// Wrap is the session check; nil: none (development only).
	Wrap    func(http.Handler) http.Handler
	Version string
	Started time.Time
	// Service is the addon's state: programs, variables, the engine.
	Service *service.Service
	// System answers what the system knows (nil: nothing - no device list, no kept database).
	System System
	// Now is the time (time.Now when nil).
	Now func() time.Time
	// DevClock is the engine's clock when it is a fake one (development and the page's tests
	// only): then /api/v1/dev/clock moves it.
	DevClock *engine.FakeClock
}

// Routes lists every "METHOD /path" the API serves.
var Routes []string

type server struct {
	o Options
}

func handle(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	Routes = append(Routes, pattern)
	mux.HandleFunc(pattern, h)
}

// New builds the handler.
func New(o Options) http.Handler {
	o.BasePath = strings.TrimRight(o.BasePath, "/")
	s := &server{o: o}
	Routes = nil
	api := http.NewServeMux()
	handle(api, "GET /api/v1/health", s.health)
	if o.DevClock != nil {
		handle(api, "GET /api/v1/dev/clock", s.devClock)
		handle(api, "POST /api/v1/dev/clock", s.devClock)
	}
	if o.Service != nil {
		a := &pageAPI{svc: o.Service, sys: o.System, now: o.Now}
		if a.now == nil {
			a.now = time.Now
		}
		handle(api, "GET /api/v1/overview", a.overview)
		handle(api, "POST /api/v1/programs", a.createProgram)
		handle(api, "PUT /api/v1/programs/{key}", a.updateProgram)
		handle(api, "DELETE /api/v1/programs/{key}", a.deleteProgram)
		handle(api, "GET /api/v1/devices", a.devices)
		handle(api, "GET /api/v1/datapoints", a.datapoints)
		handle(api, "GET /api/v1/devices/used", a.usedDevices)
		handle(api, "POST /api/v1/devices/replace", a.replaceDevice)
		handle(api, "POST /api/v1/time/preview", a.timePreview)
		handle(api, "POST /api/v1/programs/{key}/active", a.setActive)
		handle(api, "POST /api/v1/programs/{key}/at-start", a.setAtStart)
		handle(api, "POST /api/v1/programs/{key}/run", a.run)
		handle(api, "POST /api/v1/programs/{key}/dry-run", a.dryRun)
		handle(api, "GET /api/v1/programs/{key}/times", a.times)
		handle(api, "GET /api/v1/sources", a.sources)
		handle(api, "GET /api/v1/runs", a.runs)
		a.versionRoutes(api)
		handle(api, "GET /api/v1/schedule", a.schedule)
		handle(api, "POST /api/v1/pending/{id}/run", a.runPending)
		handle(api, "DELETE /api/v1/pending/{id}", a.cancelPending)
		handle(api, "GET /api/v1/settings", a.settings)
		handle(api, "PUT /api/v1/settings", a.setSettings)
		handle(api, "GET /api/v1/secrets", a.secrets)
		handle(api, "PUT /api/v1/secrets/{name}", a.setSecret)
		handle(api, "DELETE /api/v1/secrets/{name}", a.deleteSecret)
		handle(api, "GET /api/v1/remote", a.remote)
		handle(api, "PUT /api/v1/remote", a.setRemote)
		handle(api, "DELETE /api/v1/remote", a.deleteRemote)
		handle(api, "POST /api/v1/remote/key", a.newRemoteKey)
		handle(api, "DELETE /api/v1/remote/host-key", a.forgetHostKey)
		handle(api, "POST /api/v1/remote/push", a.pushRemote)
		handle(api, "POST /api/v1/remote/fetch", a.fetchRemote)
		handle(api, "POST /api/v1/remote/pull", a.pullRemote)
		handle(api, "POST /api/v1/variables", a.createVariable)
		handle(api, "PUT /api/v1/variables/{key}", a.updateVariable)
		handle(api, "DELETE /api/v1/variables/{key}", a.deleteVariable)
		handle(api, "POST /api/v1/variables/{key}/value", a.setValue)
		handle(api, "POST /api/v1/variables/{key}/acknowledge", a.acknowledge)
		handle(api, "GET /api/v1/variables/{key}/log", a.variableLog)
		handle(api, "POST /api/v1/alarms/acknowledge", a.acknowledgeAll)
		handle(api, "GET /api/v1/helpers", a.helpers)
		handle(api, "POST /api/v1/helpers", a.createHelper)
		handle(api, "DELETE /api/v1/helpers/{key}", a.deleteHelper)
		handle(api, "GET /api/v1/import", a.importState)
		handle(api, "GET /api/v1/import/codes", a.importCodes)
		handle(api, "POST /api/v1/import/preview", a.importPreview)
		handle(api, "GET /api/v1/import/{id}/report.md", a.importReport)
		handle(api, "POST /api/v1/import/{id}/apply", a.importApply)
		handle(api, "DELETE /api/v1/import/{id}", a.importDiscard)
	}
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		addonkit.WriteError(w, http.StatusNotFound, "not-found", r.URL.Path)
	})

	page := http.NewServeMux()
	page.Handle("/api/", addonkit.CSRF(api))
	page.HandleFunc("GET /docs/{lang}/{$}", s.guide)
	page.HandleFunc("GET /docs/screenshots/{lang}/{name}", s.picture)
	page.Handle("/", addonkit.AppFiles(o.WWW))

	var h http.Handler = http.StripPrefix(o.BasePath, page)
	if o.Wrap != nil {
		h = o.Wrap(h)
	}
	root := http.NewServeMux()
	root.Handle(o.BasePath+"/", guard(h))
	return root
}

// guard refuses a path that is not clean (a dot segment), before anything looks at it.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := r.URL.Path; p != "/" && (path.Clean(p) != strings.TrimSuffix(p, "/") && path.Clean(p) != p) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.o.Version, "started": s.o.Started.UTC().Format(time.RFC3339)})
}

// picture serves a picture of the guide.
func (s *server) picture(w http.ResponseWriter, r *http.Request) {
	b, err := docs.Picture(r.PathValue("lang"), r.PathValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(b)
}

// guide serves the user guide of a language as a page; the page's ?-icons link its sections.
func (s *server) guide(w http.ResponseWriter, r *http.Request) {
	page, err := docs.HTML(r.PathValue("lang"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}
