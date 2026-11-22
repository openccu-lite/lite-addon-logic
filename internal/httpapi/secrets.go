package httpapi

import (
	"net/http"

	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// SecretsView is what the page knows of the webhooks' secrets: their names and users, and the
// names programs use that are not entered. A value is taken and never given back.
type SecretsView struct {
	Secrets []service.Secret `json:"secrets"`
	Missing []string         `json:"missing"`
}

func (a *pageAPI) secretsView() SecretsView {
	list, missing := a.svc.Secrets()
	return SecretsView{Secrets: list, Missing: missing}
}

func (a *pageAPI) secrets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.secretsView())
}

func (a *pageAPI) setSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value string `json:"value"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := a.svc.SetSecret(r.PathValue("name"), in.Value); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.secretsView())
}

func (a *pageAPI) deleteSecret(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteSecret(r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.secretsView())
}
