package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// Device helpers: the ones there are, with the names of their channels, and the channels of
// this system that could have one and have none.

// HelperView is a device helper with its channel's name.
type HelperView struct {
	engine.HelperInfo
	// Name is the channel's name on this system ("" when it has none, or is not here).
	Name string `json:"name,omitempty"`
}

// HelperCandidate is a channel that can have a helper of a kind and has none.
type HelperCandidate struct {
	Kind    string `json:"kind"`
	Channel string `json:"channel"`
	// Name is the channel's name (its device's, when the channel has none; "" without both),
	// Type the device's type.
	Name string `json:"name,omitempty"`
	Type string `json:"type"`
	// Unit is the counter's unit as the device describes it; Roles are the variables a helper
	// of the kind can have.
	Unit  string   `json:"unit,omitempty"`
	Roles []string `json:"roles"`
}

// candidates asks the system which channels could have a helper of which kind: the channel's
// and the device's type fit a kind, the channel has one of the kind's counters, and no helper
// of the kind is there yet.
func (a *pageAPI) candidates(ctx context.Context, lang string, have map[string]bool) ([]HelperCandidate, error) {
	devices, err := a.sys.Devices(ctx, lang)
	if err != nil {
		return nil, err
	}
	out := []HelperCandidate{}
	var channels []string
	for _, d := range devices {
		for _, ch := range d.Channels {
			ref := d.Interface + "/" + ch.Address
			fits := false
			for _, k := range model.HelperKinds {
				if k.Fits(d.Type, ch.Type) && !have[k.Kind+"@"+ref] {
					name := ch.Name
					if name == "" {
						name = d.Name
					}
					out = append(out, HelperCandidate{Kind: k.Kind, Channel: ref, Name: name, Type: d.Type, Roles: k.Roles})
					fits = true
				}
			}
			if fits {
				channels = append(channels, ref)
			}
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	params, err := a.sys.Params(ctx, channels)
	if err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, c := range out {
		if unit, ok := counterUnit(model.HelperKindOf(c.Kind), params[c.Channel]); ok {
			c.Unit = unit
			kept = append(kept, c)
		}
	}
	return kept, nil
}

// counterUnit looks for one of a kind's counters among a channel's datapoints and answers its
// unit (the kind's own when the description names none); false when the channel has none.
func counterUnit(k *model.HelperKind, params []system.Param) (string, bool) {
	for _, counter := range k.Counters {
		for _, p := range params {
			if p.ID == counter && p.Read {
				if p.Unit != "" {
					return p.Unit, true
				}
				return k.Unit, true
			}
		}
	}
	return "", false
}

func (a *pageAPI) helpers(w http.ResponseWriter, r *http.Request) {
	infos := a.svc.Engine().Helpers()
	out := struct {
		Helpers    []HelperView      `json:"helpers"`
		Candidates []HelperCandidate `json:"candidates"`
		// System says whether the system's devices could be asked for the candidates.
		System bool `json:"system"`
	}{Helpers: []HelperView{}, Candidates: []HelperCandidate{}}
	have := map[string]bool{}
	channels := []string{}
	for _, v := range a.svc.Variables() {
		if v.Helper != nil {
			// (also a helper that has a problem: its channel is not offered again)
			have[v.Helper.ID()] = true
		}
	}
	for _, in := range infos {
		channels = append(channels, in.Channel)
	}
	names := map[string]string{}
	if a.sys != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if n, err := a.sys.Names(ctx, channels); err == nil && n != nil {
			names = n
		}
		lang := "de"
		if r.URL.Query().Get("lang") == "en" {
			lang = "en"
		}
		if c, err := a.candidates(ctx, lang, have); err == nil {
			out.Candidates, out.System = c, true
		}
	}
	for _, in := range infos {
		out.Helpers = append(out.Helpers, HelperView{HelperInfo: in, Name: names[in.Channel]})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *pageAPI) createHelper(w http.ResponseWriter, r *http.Request) {
	var in service.HelperInput
	if !body(w, r, &in) {
		return
	}
	kind := model.HelperKindOf(in.Kind)
	if kind != nil && model.ParseChannel(in.Channel) == nil {
		// the channel must be one of this system's, with the kind's counter: a helper for a
		// device that is not here comes with an import only
		if a.sys == nil {
			addonkit.WriteError(w, http.StatusBadGateway, "system", "the system's devices cannot be asked")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		params, err := a.sys.Params(ctx, []string{in.Channel})
		if err != nil {
			addonkit.WriteError(w, http.StatusBadGateway, "system", "the system's devices cannot be asked: "+err.Error())
			return
		}
		unit, ok := counterUnit(kind, params[in.Channel])
		if !ok {
			fail(w, &service.Error{Code: "invalid", Msg: in.Channel + " has no counter a helper of the kind " + kind.Kind + " follows"})
			return
		}
		in.Unit = unit
	}
	vars, err := a.svc.CreateHelper(in, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"variables": vars})
}

func (a *pageAPI) deleteHelper(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteHelper(r.PathValue("key"), author(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
