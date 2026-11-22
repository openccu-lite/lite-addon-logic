package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// TimerView is one time module line of a program, for the schedule page.
type TimerView struct {
	Program string `json:"program"`
	// Line is the line's number, counted over the program's condition lines from 1.
	Line int               `json:"line"`
	Time *model.TimeModule `json:"time"`
	// Armed: the line fires. Off says why not: inactive, faulty, no-clock, no-location.
	Armed bool   `json:"armed"`
	Off   string `json:"off,omitempty"`
	// Active: a range that is under way.
	Active bool `json:"active"`
	// Next is when it fires next (nil: never again, or not armed); Last when it last fired
	// since the addon started.
	Next *engine.TimeEdge `json:"next,omitempty"`
	Last *engine.TimeEdge `json:"last,omitempty"`
}

// HoldView is a condition line with "for" whose comparison holds: it is counting towards the
// moment it becomes true, or it has reached it.
type HoldView struct {
	Program string `json:"program"`
	engine.HoldLine
	// Of is the line itself, to say it in words.
	Of *model.Line `json:"of"`
	// Inactive: the count goes on, but reaching its end will not run the program.
	Inactive bool `json:"inactive,omitempty"`
}

// Schedule is what is armed: the time modules, the delayed activities that are not due yet and
// the lines that count how long their comparison has held.
type Schedule struct {
	Timers  []TimerView      `json:"timers"`
	Pending []engine.Pending `json:"pending"`
	Holds   []HoldView       `json:"holds"`
	// Now is the engine's time, with the system's own offset.
	Now time.Time `json:"now"`
}

// schedule answers the time modules of every program with their next and last firing, and the
// pending delayed activities, the next one first.
func (a *pageAPI) schedule(w http.ResponseWriter, r *http.Request) {
	eng := a.svc.Engine()
	out := Schedule{Timers: []TimerView{}, Pending: eng.PendingActions(), Holds: []HoldView{}, Now: eng.Now()}
	for _, p := range a.svc.Programs() {
		holds := eng.HoldLines(p.Program.Key)
		if len(holds) > 0 {
			lines := []*model.Line{}
			p.Program.Lines(func(_ int, l *model.Line) { lines = append(lines, l) })
			for _, h := range holds {
				out.Holds = append(out.Holds, HoldView{Program: p.Program.Key, HoldLine: h, Of: lines[h.Line-1], Inactive: !p.Program.Active})
			}
		}
		if !p.Program.HasTime() {
			continue
		}
		defs := map[int]*model.TimeModule{}
		n := 0
		p.Program.Lines(func(_ int, l *model.Line) {
			n++
			if l.Time != nil {
				defs[n] = l.Time
			}
		})
		state, _ := eng.Program(p.Program.Key)
		for _, tl := range eng.TimeLines(p.Program.Key, 1) {
			v := TimerView{Program: p.Program.Key, Line: tl.Line, Time: defs[tl.Line], Active: tl.Active, Last: tl.Last}
			switch {
			case len(p.Problems) > 0:
				v.Off = "faulty"
			case !p.Program.Active:
				v.Off = "inactive"
			case state.NoClock:
				v.Off = "no-clock"
			case tl.Position:
				v.Off = "no-location"
			}
			v.Armed = v.Off == ""
			if v.Armed && len(tl.Next) > 0 {
				v.Next = &tl.Next[0]
			}
			if !v.Armed {
				v.Active = false
			}
			out.Timers = append(out.Timers, v)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// pendingID reads the path's id; false means the answer is written.
func pendingID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-id", "the id of a delayed activity is a number")
		return 0, false
	}
	return id, true
}

// runPending carries out a delayed activity now.
func (a *pageAPI) runPending(w http.ResponseWriter, r *http.Request) {
	id, ok := pendingID(w, r)
	if !ok {
		return
	}
	if !a.svc.Engine().RunPending(id) {
		addonkit.WriteError(w, http.StatusNotFound, "not-found", "this delayed activity is not pending any more")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// cancelPending removes a delayed activity: it will not happen.
func (a *pageAPI) cancelPending(w http.ResponseWriter, r *http.Request) {
	id, ok := pendingID(w, r)
	if !ok {
		return
	}
	if !a.svc.Engine().CancelPending(id) {
		addonkit.WriteError(w, http.StatusNotFound, "not-found", "this delayed activity is not pending any more")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// sources answers what the engine knows of the device datapoints the programs look at: the
// value, the last report and how often the datapoint reports - for the triggers page. (The
// variables' values and times are in the overview.)
func (a *pageAPI) sources(w http.ResponseWriter, r *http.Request) {
	eng := a.svc.Engine()
	writeJSON(w, http.StatusOK, map[string]any{"devices": eng.DeviceStates(), "now": eng.Now()})
}
