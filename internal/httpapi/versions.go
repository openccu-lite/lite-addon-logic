package httpapi

import (
	"net/http"

	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// versionsShown is how many history entries of one file the page lists.
const versionsShown = 200

// versions answers the history of a program's or a variable's file, the newest entry first,
// and the file as it is now - the text the page compares the versions with.
func (a *pageAPI) versions(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		entries, err := a.svc.Versions(kind, key, versionsShown)
		if err != nil {
			fail(w, err)
			return
		}
		if entries == nil {
			entries = []repo.Entry{}
		}
		now, err := a.svc.Current(kind, key)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Versions []repo.Entry `json:"versions"`
			Current  string       `json:"current"`
		}{entries, string(now)})
	}
}

// version answers the file as one history entry has it.
func (a *pageAPI) version(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		text, err := a.svc.Version(kind, r.PathValue("key"), r.PathValue("hash"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"text": string(text)})
	}
}

// restore saves the file of a history entry as the object's new version.
func (a *pageAPI) restore(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.svc.Restore(kind, r.PathValue("key"), r.PathValue("hash"), author(r)); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// versionRoutes are the three routes, for programs and for variables.
func (a *pageAPI) versionRoutes(api *http.ServeMux) {
	for plural, kind := range map[string]string{"programs": service.KindProgram, "variables": service.KindVariable} {
		handle(api, "GET /api/v1/"+plural+"/{key}/versions", a.versions(kind))
		handle(api, "GET /api/v1/"+plural+"/{key}/versions/{hash}", a.version(kind))
		handle(api, "POST /api/v1/"+plural+"/{key}/versions/{hash}/restore", a.restore(kind))
	}
}
