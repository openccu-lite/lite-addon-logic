package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/importer"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// "Gerät ersetzen": the devices programs and variables name, with what the system and the
// import know about each, and the replacement of one by another of its kind.

// Named is an object by key and name.
type Named struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// UsedDevice is a device programs or variables name.
type UsedDevice struct {
	service.Device
	// Name and Type are the device's on this system or, when it is not here, what the import
	// noted of it; Present says the system has it.
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Present bool   `json:"present"`
	// Datapoints are what is named of it ("CHANNEL/DATAPOINT", or "CHANNEL" for a whole channel).
	Datapoints []string `json:"datapoints"`
	Programs   []Named  `json:"programs"`
	Variables  []Named  `json:"variables"`
}

// seen answers what the import noted about devices that were not here, by channel.
func (a *pageAPI) seen() []importer.SeenDevice {
	data, err := a.svc.Tree().Read(importer.SeenPath)
	if err != nil {
		return nil
	}
	list, _ := importer.DecodeSeen(data)
	return list
}

// seenDevice answers the name and the type the import noted for a device.
func seenDevice(seen []importer.SeenDevice, d service.Device) (name, typ string) {
	for _, sd := range seen {
		if strings.HasPrefix(sd.Channel, d.String()+":") {
			if name == "" {
				name = sd.Device
			}
			if typ == "" {
				typ = sd.Type
			}
		}
	}
	return name, typ
}

func find(devs []system.Device, d service.Device) *system.Device {
	for i := range devs {
		if devs[i].Interface == d.Interface && devs[i].Address == d.Address {
			return &devs[i]
		}
	}
	return nil
}

func (a *pageAPI) named(programs, variables []string) (ps, vs []Named) {
	ps, vs = []Named{}, []Named{}
	for _, k := range programs {
		if p, ok := a.svc.Program(k); ok {
			ps = append(ps, Named{k, p.Program.Name})
		}
	}
	names := map[string]string{}
	for _, v := range a.svc.Variables() {
		names[v.Key] = v.Name
	}
	for _, k := range variables {
		vs = append(vs, Named{k, names[k]})
	}
	return ps, vs
}

// systemDevices lists the system's devices; nil when there is no system to ask or it does not
// answer (ok says which).
func (a *pageAPI) systemDevices(ctx context.Context, lang string) (devs []system.Device, ok bool) {
	if a.sys == nil {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	devs, err := a.sys.Devices(ctx, lang)
	if err != nil {
		return nil, false
	}
	return devs, true
}

// usedDevices answers the devices programs and variables name: for the dialog's first list.
func (a *pageAPI) usedDevices(w http.ResponseWriter, r *http.Request) {
	devs, known := a.systemDevices(r.Context(), r.URL.Query().Get("lang"))
	seen := a.seen()
	out := []UsedDevice{}
	for _, u := range a.svc.DeviceUses() {
		d := UsedDevice{Device: u.Device, Datapoints: u.Datapoints}
		d.Programs, d.Variables = a.named(u.Programs, u.Variables)
		if here := find(devs, u.Device); here != nil {
			d.Name, d.Type, d.Present = here.Name, here.Type, true
		} else {
			d.Name, d.Type = seenDevice(seen, u.Device)
		}
		out = append(out, d)
	}
	// system says whether the system's device list could be asked: without it nothing is "present"
	writeJSON(w, http.StatusOK, map[string]any{"devices": out, "system": known})
}

// Replacement is what replacing one device by another would do, or did.
type Replacement struct {
	Programs   []Named `json:"programs"`
	Variables  []Named `json:"variables"`
	References int     `json:"references"`
	// Problems are what stands against it: another type, a channel or a datapoint the new
	// device does not have.
	Problems []string `json:"problems"`
	CanApply bool     `json:"can_apply"`
}

// replaceDevice makes everything that names one device name another one - or, with dry_run,
// says what that would change and what stands against it. The new device must be on the
// system, of the old one's type where that is known, and have every channel and datapoint that
// is named.
func (a *pageAPI) replaceDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From   service.Device `json:"from"`
		To     service.Device `json:"to"`
		DryRun bool           `json:"dry_run"`
	}
	if !body(w, r, &in) {
		return
	}
	// what the service itself refuses comes first: it needs no system
	res, err := a.svc.ReplaceDevice(in.From, in.To, nil, author(r), true)
	if err != nil {
		fail(w, err)
		return
	}
	devs, ok := a.systemDevices(r.Context(), "")
	if !ok {
		addonkit.WriteError(w, http.StatusBadGateway, "system", "the system did not list its devices: a device can only be replaced by one the system has")
		return
	}
	out := Replacement{References: res.References, Problems: []string{}}
	out.Programs, out.Variables = a.named(res.Programs, res.Variables)
	seen := a.seen()
	target := find(devs, in.To)
	if target == nil {
		out.Problems = append(out.Problems, fmt.Sprintf("the system has no device %s", in.To))
	} else {
		_, oldType := seenDevice(seen, in.From)
		if old := find(devs, in.From); old != nil {
			oldType = old.Type
		}
		if oldType != "" && oldType != target.Type {
			out.Problems = append(out.Problems, fmt.Sprintf("%s is a %s, the device it would replace a %s: only a device of the same type can take its place", in.To, target.Type, oldType))
		}
		// every channel and datapoint that is named must be there
		var used []string
		for _, u := range a.svc.DeviceUses() {
			if u.Device == in.From {
				used = u.Datapoints
			}
		}
		has := map[string]bool{}
		var channels []string
		for _, ch := range target.Channels {
			has[ch.Address] = true
			channels = append(channels, in.To.Interface+"/"+ch.Address)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		params, perr := a.sys.Params(ctx, channels)
		cancel()
		if perr != nil {
			addonkit.WriteError(w, http.StatusBadGateway, "system", "the system did not describe the new device's datapoints: "+perr.Error())
			return
		}
		var missing []string
		for _, dp := range used {
			channel, param, _ := strings.Cut(dp, "/")
			addr := in.To.Address + ":" + channel
			switch {
			case !has[addr]:
				missing = append(missing, "channel "+channel)
			case param != "":
				found := false
				for _, p := range params[in.To.Interface+"/"+addr] {
					found = found || p.ID == param
				}
				if !found {
					missing = append(missing, channel+"/"+param)
				}
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			out.Problems = append(out.Problems, fmt.Sprintf("%s lacks what programs name: %s", in.To, strings.Join(dedupe(missing), ", ")))
		}
	}
	out.CanApply = len(out.Problems) == 0
	if in.DryRun {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if !out.CanApply {
		fail(w, &service.Error{Code: "invalid", Msg: strings.Join(out.Problems, "; ")})
		return
	}
	// the import's note about the old device is of no use any more
	var extra map[string][]byte
	if len(seen) > 0 {
		kept := []importer.SeenDevice{}
		for _, sd := range seen {
			if !strings.HasPrefix(sd.Channel, in.From.String()+":") {
				kept = append(kept, sd)
			}
		}
		if len(kept) != len(seen) {
			extra = map[string][]byte{importer.SeenPath: importer.EncodeSeen(kept)}
		}
	}
	if _, err := a.svc.ReplaceDevice(in.From, in.To, extra, author(r), false); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func dedupe(sorted []string) []string {
	out := sorted[:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
