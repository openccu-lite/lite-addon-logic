package httpapi

import (
	"net/http"
	"strconv"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
)

// tracePage is how many entries of the run trace one answer holds, unless asked otherwise.
const tracePage = 100

// runs answers a page of the run trace, the newest entry first: the global log, or with
// ?program=<key> that program's entries (with the notes that it was not triggered). ?before=
// <seq> continues below an entry, ?limit= sets the page's size (1 to 500).
func (a *pageAPI) runs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, before := tracePage, uint64(0)
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 500 {
			addonkit.WriteError(w, http.StatusBadRequest, "invalid", "limit is a number from 1 to 500")
			return
		}
		limit = n
	}
	if s := q.Get("before"); s != "" {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			addonkit.WriteError(w, http.StatusBadRequest, "invalid", "before is the number of an entry")
			return
		}
		before = n
	}
	eng := a.svc.Engine()
	runs, more := eng.TracePage(q.Get("program"), before, limit)
	writeJSON(w, http.StatusOK, struct {
		Runs []engine.Run `json:"runs"`
		More bool         `json:"more"`
	}{runs, more})
}

// dryRun answers what a program would do, without an effect: evaluated now ({}), or if one of
// its condition lines' source reported a value ({"line": n, "value": v}; a time module line
// takes no value: it is tried as firing now). The answer is the entry the run trace would hold.
func (a *pageAPI) dryRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Line  int `json:"line"`
		Value any `json:"value"`
	}
	if !body(w, r, &in) {
		return
	}
	eng := a.svc.Engine()
	var run engine.Run
	var err error
	if in.Line == 0 {
		run, err = eng.DryRun(r.PathValue("key"))
	} else {
		run, err = eng.Simulate(r.PathValue("key"), in.Line, in.Value)
	}
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
