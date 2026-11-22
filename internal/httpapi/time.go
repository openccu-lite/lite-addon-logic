package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// times answers a program's time module lines: whether each is true now, when it fires next
// (the next five times) and when it last fired.
func (a *pageAPI) times(w http.ResponseWriter, r *http.Request) {
	lines := a.svc.Engine().TimeLines(r.PathValue("key"), 5)
	if lines == nil {
		addonkit.WriteError(w, http.StatusNotFound, "not-found", "there is no program "+r.PathValue("key"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (a *pageAPI) settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.svc.Settings())
}

func (a *pageAPI) setSettings(w http.ResponseWriter, r *http.Request) {
	var in model.Settings
	if !body(w, r, &in) {
		return
	}
	if err := a.svc.SetSettings(in, author(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.svc.Settings())
}

// location reads the position an import's source names ("52.52", "13.405"); nil when it names
// none, or nothing that is a position.
func location(latitude, longitude string) *model.Location {
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(latitude), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(longitude), 64)
	l := &model.Location{Latitude: lat, Longitude: lon}
	if err1 != nil || err2 != nil || (&model.Settings{Location: l}).Check() != nil {
		return nil
	}
	return l
}

// devClock is the development clock (Options.DevClock): the page's tests move the time with
// it. POST {"advance": "1h30min"} lets that much time pass, running what comes due;
// {"jump_seconds": -60} sets the clock forward or back the way a correction does. Both answer
// the time the clock shows then.
func (s *server) devClock(w http.ResponseWriter, r *http.Request) {
	c := s.o.DevClock
	if r.Method == http.MethodPost {
		var in struct {
			Advance     model.Duration `json:"advance"`
			JumpSeconds int            `json:"jump_seconds"`
		}
		if !body(w, r, &in) {
			return
		}
		if in.JumpSeconds != 0 {
			c.Jump(time.Duration(in.JumpSeconds) * time.Second)
		}
		c.Advance(in.Advance.Std())
	}
	writeJSON(w, http.StatusOK, map[string]any{"now": c.Now()})
}
