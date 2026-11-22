package httpapi

import (
	"net/http"

	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// The history's remote: its settings (never the token, never the private key), a push now,
// and the two steps of taking the remote's history over - fetch shows what would change,
// pull does it when it is still what was shown.

func (a *pageAPI) remote(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.Remote())
}

func (a *pageAPI) setRemote(w http.ResponseWriter, r *http.Request) {
	var in service.RemoteInput
	if !body(w, r, &in) {
		return
	}
	if in.URL == "" {
		fail(w, &service.Error{Code: "invalid", Msg: "the address is empty; DELETE removes the remote"})
		return
	}
	if err := a.svc.SetRemote(in); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.svc.Remote())
}

func (a *pageAPI) deleteRemote(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.SetRemote(service.RemoteInput{}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.svc.Remote())
}

func (a *pageAPI) newRemoteKey(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.NewRemoteKey(); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.svc.Remote())
}

func (a *pageAPI) forgetHostKey(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.ForgetHostKey(); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.svc.Remote())
}

// pushRemote pushes now. A push the remote refuses is not this request's failure: the answer
// is the remote's state, with the error in it.
func (a *pageAPI) pushRemote(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.PushRemote(); err != nil && a.svc.Remote().URL == "" {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.svc.Remote())
}

func (a *pageAPI) fetchRemote(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.FetchRemote()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *pageAPI) pullRemote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Remote string `json:"remote"`
	}
	if !body(w, r, &in) {
		return
	}
	p, err := a.svc.PullRemote(in.Remote)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
