package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/fakesys"
)

// The remote through the API: set, pushed, fetched, taken over on a second system, removed;
// the token never comes back.
func TestRemoteRoutes(t *testing.T) {
	g := fakesys.NewGitRemote(t.TempDir())
	g.Token = "geheim"
	if err := g.Create("haus.git"); err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(g)
	defer host.Close()
	url := host.URL + "/haus.git"

	e := newEnv(t)
	out := e.want(http.StatusOK, "", "GET", "remote", nil)
	if out["url"] != "" || out["kind"] != "" {
		t.Fatalf("at first: %v", out)
	}
	e.want(http.StatusNotFound, "not-found", "POST", "remote/fetch", nil)
	e.want(http.StatusNotFound, "not-found", "POST", "remote/push", nil)
	e.want(http.StatusNotFound, "not-found", "POST", "remote/pull", map[string]any{"remote": "x"})
	e.want(http.StatusNotFound, "not-found", "POST", "remote/key", nil)
	e.want(http.StatusNotFound, "not-found", "DELETE", "remote/host-key", nil)
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "remote", map[string]any{"url": ""})
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "remote", map[string]any{"url": "ftp://x/y"})
	e.want(http.StatusBadRequest, "invalid-body", "PUT", "remote", map[string]any{"adresse": url})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "remote/pull", map[string]any{"hash": "x"})

	// a wrong token first: the fetch says whose fault it is
	out = e.want(http.StatusOK, "", "PUT", "remote", map[string]any{"url": url, "token": "falsch"})
	if out["kind"] != "https" || out["has_token"] != true || strings.Contains(fmt.Sprint(out), "falsch") {
		t.Fatalf("set: %v", out)
	}
	e.want(http.StatusBadGateway, "remote-auth", "POST", "remote/fetch", nil)
	e.want(http.StatusCreated, "", "POST", "variables", map[string]any{"variable": map[string]any{"name": "Regen", "type": "bool", "visible": true}})
	out = e.want(http.StatusOK, "", "POST", "remote/push", nil)
	if out["error_code"] != "auth" {
		t.Errorf("a push with a wrong token: %v", out)
	}
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "remote/key", nil)
	e.want(http.StatusOK, "", "DELETE", "remote/host-key", nil)
	out = e.want(http.StatusOK, "", "PUT", "remote", map[string]any{"url": url, "token": "geheim"})
	if strings.Contains(fmt.Sprint(out), "geheim") {
		t.Fatalf("the token came back: %v", out)
	}
	out = e.want(http.StatusOK, "", "POST", "remote/push", nil)
	if out["error"] != nil || out["pending"] != false || out["last_push"] == nil {
		t.Errorf("pushed: %v", out)
	}
	out = e.want(http.StatusOK, "", "POST", "remote/fetch", nil)
	if out["relation"] != "same" || out["can_apply"] != false {
		t.Errorf("fetched: %v", out)
	}

	// a second system takes it over
	b := newEnv(t)
	b.want(http.StatusOK, "", "PUT", "remote", map[string]any{"url": url, "token": "geheim"})
	out = b.want(http.StatusOK, "", "POST", "remote/fetch", nil)
	changes := out["changes"].([]any)
	if out["relation"] != "new" || out["can_apply"] != true || len(changes) != 1 || changes[0].(map[string]any)["name"] != "Regen" || len(out["entries"].([]any)) != 1 {
		t.Fatalf("the second system fetches: %v", out)
	}
	b.want(http.StatusConflict, "moved", "POST", "remote/pull", map[string]any{"remote": "0000"})
	b.want(http.StatusOK, "", "POST", "remote/pull", map[string]any{"remote": out["remote"]})
	if vars := b.want(http.StatusOK, "", "GET", "overview", nil)["variables"].([]any); len(vars) == 0 {
		t.Errorf("nothing was taken over")
	}
	found := false
	for _, v := range b.svc.Variables() {
		found = found || v.Name == "Regen"
	}
	if !found {
		t.Error("the variable is not there")
	}
	// removed: nothing left
	out = b.want(http.StatusOK, "", "DELETE", "remote", nil)
	if out["url"] != "" {
		t.Errorf("removed: %v", out)
	}
}
