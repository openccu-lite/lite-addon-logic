package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// --- what the program editor needs: writing programs, the devices to pick from, a time module's next times ---

// programBody is a program as the editor sends it.
type programBody struct {
	Program *model.Program `json:"program"`
}

func (a *pageAPI) createProgram(w http.ResponseWriter, r *http.Request) {
	var in programBody
	if !body(w, r, &in) {
		return
	}
	if in.Program == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "program: the program")
		return
	}
	if err := a.checkValues(r.Context(), in.Program); err != nil {
		fail(w, err)
		return
	}
	p, err := a.svc.CreateProgram(in.Program, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"program": p})
}

func (a *pageAPI) updateProgram(w http.ResponseWriter, r *http.Request) {
	var in programBody
	if !body(w, r, &in) {
		return
	}
	if in.Program == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "program: the program")
		return
	}
	if err := a.checkValues(r.Context(), in.Program); err != nil {
		fail(w, err)
		return
	}
	p, err := a.svc.UpdateProgram(r.PathValue("key"), in.Program, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"program": p})
}

// checkValues checks what a program's "channel" activities write against the channels'
// datapoints as the system describes them now: a datapoint the channel does not have or that
// cannot be written, a value that does not fit. A "display" activity's channel must take the
// combined text, and its lists must have the icons and the sound at the numbers that are sent.
// A channel the system does not know - a device that is not there (yet) - is not checked; the
// check is made again before every send.
func (a *pageAPI) checkValues(ctx context.Context, p *model.Program) error {
	if a.sys == nil {
		return nil
	}
	var channels []string
	p.Actions(func(_ int, act *model.Action) {
		if (act.Kind == model.ActionChannel || act.Kind == model.ActionDisplay) && model.ParseChannel(act.Channel) == nil {
			channels = append(channels, act.Channel)
		}
	})
	if len(channels) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	params, err := a.sys.Params(ctx, channels)
	if err != nil {
		return nil
	}
	var problem error
	p.Actions(func(rule int, act *model.Action) {
		known, ok := params[act.Channel]
		if !ok || problem != nil {
			return
		}
		if act.Kind == model.ActionDisplay {
			if err := checkDisplay(known, act.Display); err != nil {
				problem = &service.Error{Code: "invalid", Msg: fmt.Sprintf("rule %d: %s: %v", rule+1, act.Channel, err)}
			}
			return
		}
		if act.Kind != model.ActionChannel {
			return
		}
		values := map[string]any{}
		for _, v := range act.Values {
			values[v.Param] = v.Value
		}
		if err := system.FitAll(known, values); err != nil {
			problem = &service.Error{Code: "invalid", Msg: fmt.Sprintf("rule %d: %s: %v", rule+1, act.Channel, err)}
		}
	})
	return problem
}

// checkDisplay says whether a channel is a display that takes what d shows.
func checkDisplay(known []system.Param, d *model.Display) error {
	if err := system.FitAll(known, map[string]any{"COMBINED_PARAMETER": ""}); err != nil {
		return fmt.Errorf("this channel is no display (%v)", err)
	}
	has := func(param string, table []string, entry string) bool {
		for _, p := range known {
			if p.ID == param {
				for i, name := range table {
					if name == entry {
						return i < len(p.Values) && p.Values[i] == entry
					}
				}
			}
		}
		return true
	}
	for i, l := range d.Lines {
		if l.Icon != "" && !has("DISPLAY_DATA_ICON", model.DisplayIcons, l.Icon) {
			return fmt.Errorf("line %d: the display has no icon %s", i+1, l.Icon)
		}
	}
	if d.Sound != "" && !has("ACOUSTIC_NOTIFICATION_SELECTION", model.DisplaySounds, d.Sound) {
		return fmt.Errorf("the display has no signal tone %s", d.Sound)
	}
	return nil
}

func (a *pageAPI) deleteProgram(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteProgram(r.PathValue("key"), author(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// devices answers the system's devices with their channels, names and rooms (lang: de or en),
// for the editor's picker.
func (a *pageAPI) devices(w http.ResponseWriter, r *http.Request) {
	if a.sys == nil {
		writeJSON(w, http.StatusOK, map[string]any{"devices": []any{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	devs, err := a.sys.Devices(ctx, r.URL.Query().Get("lang"))
	if err != nil {
		addonkit.WriteError(w, http.StatusBadGateway, "system", "the system did not list its devices: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devs})
}

// MaxDatapointChannels bounds one question for datapoints.
const MaxDatapointChannels = 500

// datapoints answers the datapoints of the channels named by ?channel=Interface/ADDRESS:CH
// (repeatable); a channel the system does not know is left out.
func (a *pageAPI) datapoints(w http.ResponseWriter, r *http.Request) {
	channels := r.URL.Query()["channel"]
	if len(channels) == 0 || len(channels) > MaxDatapointChannels {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-channel", "channel: one to 500 channels, each Interface/ADDRESS:CHANNEL")
		return
	}
	for _, ch := range channels {
		if err := model.ParseChannel(ch); err != nil {
			addonkit.WriteError(w, http.StatusBadRequest, "invalid-channel", err.Error())
			return
		}
	}
	if a.sys == nil {
		writeJSON(w, http.StatusOK, map[string]any{"channels": map[string]any{}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	params, err := a.sys.Params(ctx, channels)
	if err != nil {
		addonkit.WriteError(w, http.StatusBadGateway, "system", "the system did not describe the datapoints: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": params})
}

// timePreview answers when a time module would fire next (the next five times) - the module is
// the body's, nothing is stored. A module the scheduler cannot run is refused with the reason.
func (a *pageAPI) timePreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Time *model.TimeModule `json:"time"`
	}
	if !body(w, r, &in) {
		return
	}
	if in.Time == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "time: the time module")
		return
	}
	// checked as a file's would be: in a program of one line
	probe := &model.Program{Key: "x", Name: "x", Rules: []model.Rule{{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All,
		Lines: []model.Line{{Time: in.Time, Trigger: model.TriggerUpdate}}}}}, Then: []model.Action{}}}}
	if _, err := probe.Check(); err != nil {
		msg := err.Error()
		if me, ok := err.(*model.Error); ok {
			msg = me.Msg
		}
		addonkit.WriteError(w, http.StatusUnprocessableEntity, "invalid", msg)
		return
	}
	eng := a.svc.Engine()
	now := eng.Now()
	mod, err := schedule.Compile(in.Time, now.Location(), eng.Position())
	if err != nil {
		addonkit.WriteError(w, http.StatusUnprocessableEntity, "invalid", err.Error())
		return
	}
	next := []engine.TimeEdge{}
	for _, edge := range mod.Upcoming(now, 5) {
		next = append(next, engine.TimeEdge{At: edge.At, Kind: edge.Kind.String()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"next": next, "no_position": mod.NeedsPosition()})
}
