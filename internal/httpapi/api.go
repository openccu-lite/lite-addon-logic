package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/importer"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// System is what the page's API asks the system for: the devices that exist here (an import
// keeps a program active only when its devices do), their names, and the ReGa database the
// system kept after the switch from a CCU.
type System interface {
	// Channels answers every channel of every interface as "Interface/ADDRESS:CH".
	Channels(ctx context.Context) (map[string]bool, error)
	// Names answers the names of the given channels, by the same reference (one without a
	// name is left out).
	Names(ctx context.Context, channels []string) (map[string]string, error)
	// Devices lists the devices with their channels, names and rooms (in lang), and Params
	// the datapoints of channels, for the editor's picker.
	Devices(ctx context.Context, lang string) ([]system.Device, error)
	Params(ctx context.Context, channels []string) (map[string][]system.Param, error)
	// Regadom reads the kept database (importer.ErrNoSystemDatabase when there is none).
	Regadom(ctx context.Context) (*regadom.Source, error)
	// HasRegadom says whether one is kept.
	HasRegadom(ctx context.Context) bool
}

// MaxUpload bounds an uploaded backup: a backup carries the addons' data too.
const MaxUpload = 4 << 30

// previewTTL is how long a dry run waits for its "Übernehmen".
const previewTTL = time.Hour

// preview is a dry run kept until it is applied or discarded.
type preview struct {
	id      string
	created time.Time
	res     *importer.Result
}

// pageAPI is the page's API on the service.
type pageAPI struct {
	svc *service.Service
	sys System
	now func() time.Time

	mu      sync.Mutex
	pending *preview
}

func author(r *http.Request) repo.Author {
	if s := addonkit.FromRequest(r); s != nil {
		switch {
		case s.User != "":
			return repo.Author{Name: s.User}
		case s.Token != "":
			return repo.Author{Name: "API token " + s.Token}
		}
	}
	return repo.Author{Name: "Logic"}
}

// fail answers an error of the service or the engine with its code.
func fail(w http.ResponseWriter, err error) {
	var se *service.Error
	var we *engine.WriteError
	switch {
	case errors.As(err, &se):
		status := http.StatusUnprocessableEntity
		switch se.Code {
		case "not-found":
			status = http.StatusNotFound
		case "exists", "in-use", "moved":
			status = http.StatusConflict
		}
		if strings.HasPrefix(se.Code, "remote-") {
			// the history's remote did not play along: not this request's fault
			status = http.StatusBadGateway
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": se.Code, "message": se.Msg, "users": se.Users})
	case errors.As(err, &we):
		status := http.StatusUnprocessableEntity
		if we.Code == "unknown-variable" || we.Code == "unknown-program" {
			status = http.StatusNotFound
		}
		addonkit.WriteError(w, status, we.Code, we.Msg)
	default:
		addonkit.WriteError(w, http.StatusInternalServerError, "failed", err.Error())
	}
}

// body reads a JSON body into v; false means the answer is written.
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "the request is not the JSON this route takes: "+err.Error())
		return false
	}
	return true
}

// VariableView is a variable with its value and who uses it.
type VariableView struct {
	engine.VariableInfo
	// Users are the programs that use the variable (it cannot be removed while there are any).
	Users []string `json:"users"`
	// System marks a system value: read-only, no file.
	System bool `json:"system,omitempty"`
}

// TriggerView is one triggering line of a program, for the list.
type TriggerView struct {
	// Kind: variable, device, time.
	Kind string `json:"kind"`
	// Ref: the variable's key, the datapoint, or the time module's kind.
	Ref  string `json:"ref"`
	Mode string `json:"mode"`
}

// ProgramView is a program with what the list shows.
type ProgramView struct {
	Program  *model.Program `json:"program"`
	Problems []string       `json:"problems"`
	Triggers []TriggerView  `json:"triggers"`
	// Script: the program holds a kept script and cannot be active. Time: it has time modules,
	// which do not trigger in this version.
	Script bool `json:"script"`
	Time   bool `json:"time"`
	// State is what the engine remembers: the last run, a pause of the loop guard, refused writes.
	State engine.ProgramInfo `json:"state"`
}

// Overview is everything the page shows, in one answer.
type Overview struct {
	Programs  []ProgramView  `json:"programs"`
	Variables []VariableView `json:"variables"`
	// Alarms are the alarm messages: the alarm variables triggered and not acknowledged.
	Alarms []engine.VariableInfo `json:"alarms"`
	// Names are the system's names of the channels the programs use, by Interface/ADDRESS:CH.
	Names   map[string]string `json:"names"`
	Notices service.Notices   `json:"notices"`
	// Settings are the addon's settings (the location); Now is the engine's time, in the
	// system's time zone.
	Settings model.Settings `json:"settings"`
	Now      time.Time      `json:"now"`
}

func (a *pageAPI) overview(w http.ResponseWriter, r *http.Request) {
	eng := a.svc.Engine()
	out := Overview{Programs: []ProgramView{}, Variables: []VariableView{}, Alarms: eng.AlarmMessages(), Names: map[string]string{}, Notices: a.svc.Notices(), Settings: a.svc.Settings(), Now: eng.Now()}
	if out.Alarms == nil {
		out.Alarms = []engine.VariableInfo{}
	}
	states := map[string]engine.ProgramInfo{}
	for _, st := range eng.ProgramStates() {
		states[st.Key] = st
	}
	used := map[string]bool{}
	for _, p := range a.svc.Programs() {
		v := ProgramView{Program: p.Program, Problems: p.Problems, Triggers: []TriggerView{}, Script: p.Program.HasScript(), Time: p.Program.HasTime()}
		if v.Problems == nil {
			v.Problems = []string{}
		}
		p.Program.Lines(func(_ int, l *model.Line) {
			if l.Device != nil {
				used[l.Device.Channel()] = true
			}
			// (a time module triggers whatever its line's mode says, as on the CCU)
			if l.Trigger == model.TriggerCheck && l.Time == nil {
				return
			}
			t := TriggerView{Mode: string(l.Trigger)}
			switch {
			case l.Device != nil:
				t.Kind, t.Ref = "device", l.Device.String()
			case l.Time != nil:
				t.Kind, t.Ref = "time", l.Time.Kind
			default:
				t.Kind, t.Ref = "variable", l.Variable
			}
			for _, have := range v.Triggers {
				if have == t {
					return
				}
			}
			v.Triggers = append(v.Triggers, t)
		})
		p.Program.Actions(func(_ int, act *model.Action) {
			if act.Device != nil {
				used[act.Device.Channel()] = true
			}
			if act.FromDevice != nil {
				used[act.FromDevice.Channel()] = true
			}
			if act.Kind == model.ActionChannel || act.Kind == model.ActionDisplay {
				used[act.Channel] = true
			}
		})
		v.State = states[p.Program.Key]
		v.State.Key = p.Program.Key
		out.Programs = append(out.Programs, v)
	}
	infos := map[string]engine.VariableInfo{}
	for _, in := range eng.Variables() {
		infos[in.Def.Key] = in
	}
	for _, def := range a.svc.Variables() {
		users := a.svc.VariableUsers(def.Key)
		if users == nil {
			users = []string{}
		}
		in := infos[def.Key]
		in.Def = def
		if def.Helper != nil {
			used[def.Helper.Channel] = true
		}
		out.Variables = append(out.Variables, VariableView{VariableInfo: in, Users: users})
	}
	for _, sv := range eng.SystemValues() {
		users := a.svc.VariableUsers(sv.Key)
		if users == nil {
			users = []string{}
		}
		out.Variables = append(out.Variables, VariableView{System: true, Users: users,
			VariableInfo: engine.VariableInfo{Def: &model.Variable{Key: sv.Key, Type: model.VarNumber, Visible: true}, Value: sv.Value}})
	}
	if len(used) > 0 && a.sys != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		channels := make([]string, 0, len(used))
		for ch := range used {
			channels = append(channels, ch)
		}
		sort.Strings(channels)
		if names, err := a.sys.Names(ctx, channels); err == nil && names != nil {
			out.Names = names
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// setAtStart switches "beim Systemstart prüfen" of a program.
func (a *pageAPI) setAtStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AtStart *bool `json:"at_start"`
	}
	if !body(w, r, &in) {
		return
	}
	if in.AtStart == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "at_start: true or false")
		return
	}
	p, err := a.svc.SetAtStart(r.PathValue("key"), *in.AtStart, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"program": p})
}

func (a *pageAPI) setActive(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Active *bool `json:"active"`
	}
	if !body(w, r, &in) {
		return
	}
	if in.Active == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "active: true or false")
		return
	}
	p, err := a.svc.SetActive(r.PathValue("key"), *in.Active, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"program": p})
}

func (a *pageAPI) run(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
	}
	if !body(w, r, &in) {
		return
	}
	if in.Mode != model.RunCheck && in.Mode != model.RunThen {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "mode: check or then")
		return
	}
	// the page may run any program, operable or not: "bedienbar" is for other software
	if err := a.svc.Engine().RunProgramBy(r.PathValue("key"), in.Mode, "user"); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// variableBody is a variable's definition as the page sends it, and with a new variable its
// first value, with a changed one the entries that were renamed (old label to new).
type variableBody struct {
	Variable *model.Variable   `json:"variable"`
	Value    any               `json:"value"`
	Renames  map[string]string `json:"renames"`
}

func (a *pageAPI) createVariable(w http.ResponseWriter, r *http.Request) {
	var in variableBody
	if !body(w, r, &in) {
		return
	}
	if in.Variable == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "variable: the definition")
		return
	}
	v, err := a.svc.CreateVariable(in.Variable, in.Value, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"variable": v})
}

func (a *pageAPI) updateVariable(w http.ResponseWriter, r *http.Request) {
	var in variableBody
	if !body(w, r, &in) {
		return
	}
	if in.Variable == nil {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-body", "variable: the definition")
		return
	}
	v, err := a.svc.UpdateVariable(r.PathValue("key"), in.Variable, in.Renames, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"variable": v})
}

func (a *pageAPI) deleteVariable(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.DeleteVariable(r.PathValue("key"), author(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *pageAPI) setValue(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value any `json:"value"`
	}
	if !body(w, r, &in) {
		return
	}
	// manual input: the limits of a number hold, as in the CCU's dialog (OQ-23)
	if err := a.svc.Engine().SetVariable(r.PathValue("key"), in.Value, engine.FromUser); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *pageAPI) acknowledge(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Engine().Acknowledge(r.PathValue("key")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// variableLog answers the kept changes of a logged variable, oldest first.
func (a *pageAPI) variableLog(w http.ResponseWriter, r *http.Request) {
	log, err := a.svc.VariableLog(r.PathValue("key"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"log": log})
}

// acknowledgeAll acknowledges every alarm message ("alle bestätigen").
func (a *pageAPI) acknowledgeAll(w http.ResponseWriter, r *http.Request) {
	n := 0
	for _, al := range a.svc.Engine().AlarmMessages() {
		if err := a.svc.Engine().Acknowledge(al.Def.Key); err == nil {
			n++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": n})
}

// --- the import ---

// ImportState says what the wizard can offer.
type ImportState struct {
	// System: the system keeps a CCU's ReGa database.
	System bool `json:"system"`
	// Pending is a dry run waiting for its decision.
	Pending *PreviewView `json:"pending,omitempty"`
}

// PreviewView is a dry run as the page gets it.
type PreviewView struct {
	ID      string           `json:"id"`
	Created time.Time        `json:"created"`
	Report  *importer.Report `json:"report"`
}

func (p *preview) view() *PreviewView {
	return &PreviewView{ID: p.id, Created: p.created.UTC(), Report: p.res.Report}
}

// current answers the pending dry run, dropping one that waited too long.
func (a *pageAPI) current() *preview {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending != nil && a.now().Sub(a.pending.created) > previewTTL {
		a.pending = nil
	}
	return a.pending
}

func (a *pageAPI) importState(w http.ResponseWriter, r *http.Request) {
	out := ImportState{}
	if a.sys != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		out.System = a.sys.HasRegadom(ctx)
	}
	if p := a.current(); p != nil {
		out.Pending = p.view()
	}
	writeJSON(w, http.StatusOK, out)
}

// importPreview reads a source and answers the dry run: nothing is written. The source is the
// request's body (a .sbk or a homematic.regadom), or with from=system the database the system
// kept. Query: source (the ledger's name, "ccu" by default), all_inactive=1, lang (de|en: the
// names of the CCU's built-in variables).
func (a *pageAPI) importPreview(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("source")
	if name == "" {
		name = importer.DefaultSource
	}
	if !importer.ValidSource(name) {
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-source", "the source's name: lower-case letters, digits and hyphens")
		return
	}
	var src *regadom.Source
	var err error
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	switch q.Get("from") {
	case "system":
		if a.sys == nil {
			err = importer.ErrNoSystemDatabase
		} else {
			src, err = a.sys.Regadom(ctx)
		}
		if errors.Is(err, importer.ErrNoSystemDatabase) {
			addonkit.WriteError(w, http.StatusNotFound, "no-regadom", "the system keeps no ReGa database of a CCU")
			return
		}
	case "", "upload":
		src, err = regadom.Read(http.MaxBytesReader(w, r.Body, MaxUpload))
	default:
		addonkit.WriteError(w, http.StatusBadRequest, "invalid-from", "from: upload or system")
		return
	}
	if err != nil {
		addonkit.WriteError(w, http.StatusUnprocessableEntity, "unreadable", err.Error())
		return
	}
	opt := importer.Options{Source: name, AllInactive: q.Get("all_inactive") == "1", Lang: q.Get("lang")}
	if a.sys != nil {
		if channels, err := a.sys.Channels(ctx); err == nil {
			opt.Known = func(ch string) bool { return channels[ch] }
		}
	}
	prev, err := importer.LoadLedger(a.svc.Tree(), name)
	if err != nil {
		addonkit.WriteError(w, http.StatusConflict, "ledger", "the record of the earlier import of this source cannot be read: "+err.Error())
		return
	}
	opt.Previous = prev
	have := map[string]bool{}
	for _, p := range a.svc.Programs() {
		have["program/"+p.Program.Key] = true
	}
	for _, v := range a.svc.Variables() {
		have["variable/"+v.Key] = true
	}
	opt.Taken = func(kind, key string) bool { return have[kind+"/"+key] }
	res := importer.Import(src, opt)
	p := &preview{id: newID(), created: a.now(), res: res}
	a.mu.Lock()
	a.pending = p
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, p.view())
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// find answers the pending dry run of that id; false means the answer is written.
func (a *pageAPI) find(w http.ResponseWriter, r *http.Request) (*preview, bool) {
	p := a.current()
	if p == nil || p.id != r.PathValue("id") {
		addonkit.WriteError(w, http.StatusNotFound, "no-preview", "this dry run is gone: read the source again")
		return nil, false
	}
	return p, true
}

// importReport answers the dry run's report as Markdown, for keeping.
func (a *pageAPI) importReport(w http.ResponseWriter, r *http.Request) {
	p, ok := a.find(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="import-`+p.res.Report.Source+`.md"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, p.res.Report.Markdown(r.URL.Query().Get("lang")))
}

// importApply stores the dry run: one step, one history entry.
func (a *pageAPI) importApply(w http.ResponseWriter, r *http.Request) {
	p, ok := a.find(w, r)
	if !ok {
		return
	}
	res := p.res
	seen, _ := a.svc.Tree().Read(importer.SeenPath)
	imported, inactive, _, variables := res.Report.Totals()
	out, err := a.svc.ApplyImport(service.Import{Programs: res.Programs, Variables: res.Variables, Values: res.Values, Helpers: res.Helpers, Files: res.Files(seen),
		Location: location(res.Report.Location.Latitude, res.Report.Location.Longitude),
		Message:  "Import from " + res.Report.Source + ": " + plural(imported+inactive, "program") + ", " + plural(variables, "variable")}, author(r))
	if err != nil {
		fail(w, err)
		return
	}
	a.mu.Lock()
	if a.pending == p {
		a.pending = nil
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"result": out, "report": res.Report})
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

func (a *pageAPI) importDiscard(w http.ResponseWriter, r *http.Request) {
	p, ok := a.find(w, r)
	if !ok {
		return
	}
	a.mu.Lock()
	if a.pending == p {
		a.pending = nil
	}
	a.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// importCodes answers the words of every code a report carries, for the page: de and en.
func (a *pageAPI) importCodes(w http.ResponseWriter, r *http.Request) {
	out := map[string]map[string]string{}
	codes := importer.Codes()
	sort.Strings(codes)
	for _, c := range codes {
		out[c] = map[string]string{"de": importer.Template("de", c), "en": importer.Template("en", c)}
	}
	writeJSON(w, http.StatusOK, out)
}
