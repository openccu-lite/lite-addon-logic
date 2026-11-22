package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"

	"github.com/openccu-lite/lite-addon-logic/internal/importer"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// fakeSystem is the system as the API asks it.
type fakeSystem struct {
	channels map[string]bool
	names    map[string]string
	regadom  []byte
	fail     error
	devices  []system.Device
	params   map[string][]system.Param
}

func (f *fakeSystem) Devices(context.Context, string) ([]system.Device, error) {
	return f.devices, f.fail
}
func (f *fakeSystem) Params(_ context.Context, channels []string) (map[string][]system.Param, error) {
	out := map[string][]system.Param{}
	for _, ch := range channels {
		if p, ok := f.params[ch]; ok {
			out[ch] = p
		}
	}
	return out, f.fail
}

func (f *fakeSystem) Channels(context.Context) (map[string]bool, error) { return f.channels, f.fail }
func (f *fakeSystem) Names(_ context.Context, channels []string) (map[string]string, error) {
	out := map[string]string{}
	for _, ch := range channels {
		if n := f.names[ch]; n != "" {
			out[ch] = n
		}
	}
	return out, f.fail
}
func (f *fakeSystem) HasRegadom(context.Context) bool { return f.regadom != nil }
func (f *fakeSystem) Regadom(context.Context) (*regadom.Source, error) {
	if f.regadom == nil {
		return nil, importer.ErrNoSystemDatabase
	}
	return regadom.Read(bytes.NewReader(f.regadom))
}

type env struct {
	t   *testing.T
	svc *service.Service
	sys *fakeSystem
	srv *httptest.Server
	now time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	svc, err := service.Open(service.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e := &env{t: t, svc: svc, sys: &fakeSystem{}, now: time.Unix(1800000000, 0)}
	e.srv = newServer(t, Options{Service: svc, System: e.sys, Now: func() time.Time { return e.now }})
	return e
}

// do sends a request with the kit's header and answers the status and the decoded JSON.
func (e *env) do(method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	status, raw, _ := e.raw(method, path, body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			e.t.Fatalf("%s %s: %d, not JSON: %s", method, path, status, raw)
		}
	}
	return status, out
}

func (e *env) raw(method, path string, body any) (int, []byte, http.Header) {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	case string:
		rd = strings.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		rd = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, e.srv.URL+"/addons/logic/api/v1/"+path, rd)
	req.Header.Set(addonkit.HeaderRequest, "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw, res.Header
}

func (e *env) want(status int, code string, method, path string, body any) map[string]any {
	e.t.Helper()
	got, out := e.do(method, path, body)
	if got != status || (code != "" && out["error"] != code) {
		e.t.Errorf("%s %s: %d %v, want %d %s", method, path, got, out, status, code)
	}
	return out
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../regadom/testdata/" + name + ".regadom")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestVariablesAPI(t *testing.T) {
	e := newEnv(t)
	out := e.want(200, "", "GET", "overview", nil)
	if len(out["programs"].([]any)) != 0 || len(out["variables"].([]any)) != 5 || len(out["alarms"].([]any)) != 0 {
		t.Fatalf("an empty addon: %v", out)
	}
	sys := out["variables"].([]any)[0].(map[string]any)
	if sys["system"] != true || sys["def"].(map[string]any)["key"] != model.SystemAlarmMessages || sys["value"] != 0.0 || len(sys["users"].([]any)) != 0 {
		t.Errorf("the system values: %v", sys)
	}
	// create: a number with limits and a first value, an alarm, a list
	out = e.want(201, "", "POST", "variables", map[string]any{"variable": map[string]any{"name": "Temperatur", "type": "number", "unit": "°C", "min": -40, "max": 60, "visible": true}, "value": 21.5})
	if out["variable"].(map[string]any)["key"] != "temperatur" {
		t.Fatalf("created: %v", out)
	}
	e.want(201, "", "POST", "variables", map[string]any{"variable": map[string]any{"name": "Alarm Wasser", "type": "alarm", "visible": true}})
	e.want(201, "", "POST", "variables", map[string]any{"variable": map[string]any{"name": "Modus", "type": "list", "entries": []string{"Auto", "Manuell"}, "visible": true}})
	// a second variable of the same name gets a key of its own
	if out := e.want(201, "", "POST", "variables", map[string]any{"variable": map[string]any{"name": "Temperatur", "type": "number", "visible": true}}); out["variable"].(map[string]any)["key"] != "temperatur-2" {
		t.Errorf("a second Temperatur: %v", out)
	}
	e.want(200, "", "DELETE", "variables/temperatur-2", nil)
	e.want(422, "invalid", "POST", "variables", map[string]any{"variable": map[string]any{"name": "", "type": "number"}})
	e.want(400, "invalid-body", "POST", "variables", map[string]any{})
	e.want(400, "invalid-body", "POST", "variables", "{")
	e.want(400, "invalid-body", "POST", "variables", map[string]any{"unknown": 1})
	// set a value by hand: the limits hold (OQ-23)
	e.want(200, "", "POST", "variables/temperatur/value", map[string]any{"value": 19})
	e.want(422, "out-of-range", "POST", "variables/temperatur/value", map[string]any{"value": 99})
	e.want(422, "wrong-type", "POST", "variables/temperatur/value", map[string]any{"value": "warm"})
	e.want(404, "unknown-variable", "POST", "variables/nichts/value", map[string]any{"value": 1})
	e.want(422, "read-only", "POST", "variables/"+model.SystemDutyCycle+"/value", map[string]any{"value": 1})
	e.want(400, "invalid-body", "POST", "variables/temperatur/value", "nope")
	e.want(200, "", "POST", "variables/modus/value", map[string]any{"value": "Manuell"})
	// an alarm: triggered, listed, acknowledged
	e.want(200, "", "POST", "variables/alarm-wasser/value", map[string]any{"value": true})
	out = e.want(200, "", "GET", "overview", nil)
	alarms := out["alarms"].([]any)
	if len(alarms) != 1 || alarms[0].(map[string]any)["def"].(map[string]any)["key"] != "alarm-wasser" || alarms[0].(map[string]any)["alarm"].(map[string]any)["count"] != 1.0 {
		t.Errorf("alarm messages: %v", alarms)
	}
	var temp map[string]any
	for _, v := range out["variables"].([]any) {
		m := v.(map[string]any)
		if m["def"].(map[string]any)["key"] == "temperatur" {
			temp = m
		}
		if m["def"].(map[string]any)["key"] == model.SystemAlarmMessages && m["value"] != 1.0 {
			t.Errorf("the alarm message count: %v", m["value"])
		}
	}
	if temp == nil || temp["value"] != 19.0 || temp["def"].(map[string]any)["unit"] != "°C" || temp["time"] == nil {
		t.Errorf("temperatur: %v", temp)
	}
	// a logged variable's kept changes
	e.want(201, "", "POST", "variables", map[string]any{"variable": map[string]any{"name": "Zähler", "type": "number", "visible": true, "logged": true}})
	e.want(200, "", "POST", "variables/zaehler/value", map[string]any{"value": 1})
	e.want(200, "", "POST", "variables/zaehler/value", map[string]any{"value": 2})
	if log := e.want(200, "", "GET", "variables/zaehler/log", nil)["log"].([]any); len(log) != 2 || log[1].(map[string]any)["value"] != 2.0 || log[1].(map[string]any)["time"] == nil {
		t.Errorf("the log: %v", log)
	}
	if log := e.want(200, "", "GET", "variables/temperatur/log", nil)["log"].([]any); len(log) != 0 {
		t.Errorf("the log of a variable that is not logged: %v", log)
	}
	e.want(404, "unknown-variable", "GET", "variables/nichts/log", nil)
	e.want(200, "", "DELETE", "variables/zaehler", nil)
	e.want(200, "", "POST", "variables/alarm-wasser/acknowledge", nil)
	e.want(422, "wrong-type", "POST", "variables/temperatur/acknowledge", nil)
	e.want(404, "unknown-variable", "POST", "variables/nichts/acknowledge", nil)
	e.want(200, "", "POST", "variables/alarm-wasser/value", map[string]any{"value": true})
	if out := e.want(200, "", "POST", "alarms/acknowledge", nil); out["acknowledged"] != 1.0 {
		t.Errorf("acknowledge all: %v", out)
	}
	if out := e.want(200, "", "GET", "overview", nil); len(out["alarms"].([]any)) != 0 {
		t.Errorf("after acknowledging: %v", out["alarms"])
	}
	// update: a renamed entry is renamed in the value too; a type in use cannot change
	out = e.want(200, "", "PUT", "variables/modus", map[string]any{"variable": map[string]any{"name": "Betriebsart", "type": "list", "entries": []string{"Auto", "Hand"}, "visible": true}, "renames": map[string]string{"Manuell": "Hand"}})
	if out["variable"].(map[string]any)["name"] != "Betriebsart" {
		t.Errorf("updated: %v", out)
	}
	if v, _ := e.svc.Engine().Value("modus"); v != "Hand" {
		t.Errorf("the value after the rename: %v", v)
	}
	e.want(404, "not-found", "PUT", "variables/nichts", map[string]any{"variable": map[string]any{"name": "x", "type": "bool"}})
	e.want(400, "invalid-body", "PUT", "variables/modus", map[string]any{})
	e.want(400, "invalid-body", "PUT", "variables/modus", "[")
	// a variable a program uses cannot be removed; the answer names the program
	prg := &model.Program{Key: "heizen", Name: "Heizen", Visible: true, Operable: true, Rules: []model.Rule{
		{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{
			{Variable: "temperatur", Op: model.OpLt, Value: 18.0, Trigger: model.TriggerChange},
			{Variable: "temperatur", Op: model.OpLt, Value: 30.0, Trigger: model.TriggerChange},
			{Device: &model.Ref{Interface: "HmIP-RF", Address: "0001:1", Param: "STATE"}, Op: model.OpEq, Value: true, Trigger: model.TriggerUpdate},
			{Variable: "modus", Op: model.OpEq, Value: "Auto", Trigger: model.TriggerCheck},
			{Time: &model.TimeModule{Kind: "daily", At: "06:30"}, Trigger: model.TriggerUpdate}}}}},
			Then: []model.Action{{Kind: model.ActionSet, Device: &model.Ref{Interface: "HmIP-RF", Address: "0002:4", Param: "LEVEL"}, To: 1.0}}}}}
	if _, err := e.svc.CreateProgram(prg, repo.Author{Name: "Test"}); err != nil {
		t.Fatal(err)
	}
	out = e.want(409, "in-use", "DELETE", "variables/temperatur", nil)
	if users := out["users"].([]any); len(users) != 1 || users[0] != "heizen" {
		t.Errorf("users: %v", out)
	}
	e.want(200, "", "DELETE", "variables/alarm-wasser", nil)
	e.want(404, "not-found", "DELETE", "variables/alarm-wasser", nil)
	// the overview of the program: its triggers once each, the names of the channels it uses
	e.sys.names = map[string]string{"HmIP-RF/0001:1": "Fenster Küche", "HmIP-RF/9999:1": "nicht verwendet"}
	out = e.want(200, "", "GET", "overview", nil)
	p := out["programs"].([]any)[0].(map[string]any)
	tr := p["triggers"].([]any)
	if len(tr) != 3 || tr[0].(map[string]any)["ref"] != "temperatur" || tr[1].(map[string]any)["kind"] != "device" || tr[1].(map[string]any)["ref"] != "HmIP-RF/0001:1/STATE" ||
		tr[2].(map[string]any)["kind"] != "time" || tr[2].(map[string]any)["ref"] != "daily" || p["time"] != true || p["script"] != false || len(p["problems"].([]any)) != 0 {
		t.Errorf("program view: %v", p)
	}
	if names := out["names"].(map[string]any); len(names) != 1 || names["HmIP-RF/0001:1"] != "Fenster Küche" {
		t.Errorf("names: %v", names)
	}
	for _, v := range out["variables"].([]any) {
		m := v.(map[string]any)
		if m["def"].(map[string]any)["key"] == "temperatur" && (len(m["users"].([]any)) != 1 || m["users"].([]any)[0] != "heizen") {
			t.Errorf("users in the overview: %v", m)
		}
	}
	// the system does not answer for the names: the overview comes without them
	e.sys.fail = errors.New("down")
	if out := e.want(200, "", "GET", "overview", nil); len(out["names"].(map[string]any)) != 0 {
		t.Errorf("names without the system: %v", out["names"])
	}
}

func TestProgramsAPI(t *testing.T) {
	e := newEnv(t)
	who := repo.Author{Name: "Test"}
	if _, err := e.svc.CreateVariable(&model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	prg := &model.Program{Key: "markise", Name: "Markise", Visible: true, Operable: true, Rules: []model.Rule{
		{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "regen", Op: model.OpEq, Value: true, Trigger: model.TriggerChange}}}}},
			Then: []model.Action{{Kind: model.ActionVariable, Variable: "regen", To: false}}}}}
	if _, err := e.svc.CreateProgram(prg, who); err != nil {
		t.Fatal(err)
	}
	out := e.want(200, "", "POST", "programs/markise/active", map[string]any{"active": true})
	if out["program"].(map[string]any)["active"] != true {
		t.Errorf("switched: %v", out)
	}
	if p, _ := e.svc.Program("markise"); !p.Program.Active {
		t.Error("the program is not active")
	}
	e.want(200, "", "POST", "programs/markise/active", map[string]any{"active": false})
	e.want(404, "not-found", "POST", "programs/nichts/active", map[string]any{"active": true})
	// "beim Systemstart prüfen"
	out = e.want(200, "", "POST", "programs/markise/at-start", map[string]any{"at_start": true})
	if out["program"].(map[string]any)["at_start"] != true {
		t.Errorf("switched on: %v", out)
	}
	out = e.want(200, "", "POST", "programs/markise/at-start", map[string]any{"at_start": false})
	if out["program"].(map[string]any)["at_start"] != nil {
		t.Errorf("switched off: %v", out)
	}
	e.want(400, "invalid-body", "POST", "programs/markise/at-start", map[string]any{})
	e.want(404, "not-found", "POST", "programs/nichts/at-start", map[string]any{"at_start": true})
	e.want(400, "invalid-body", "POST", "programs/markise/active", map[string]any{})
	e.want(400, "invalid-body", "POST", "programs/markise/active", "x")
	// run: the mode is checked, the program must exist
	e.want(400, "invalid-body", "POST", "programs/markise/run", map[string]any{"mode": "sideways"})
	e.want(400, "invalid-body", "POST", "programs/markise/run", "x")
	e.want(404, "unknown-program", "POST", "programs/nichts/run", map[string]any{"mode": "check"})
	// "then" runs the first block whatever the condition says; the overview shows the run
	if _, err := e.svc.Engine().Value("regen"); !err {
		t.Fatal("regen")
	}
	_ = e.svc.Engine().SetVariable("regen", true, 1)
	e.want(200, "", "POST", "programs/markise/run", map[string]any{"mode": "then"})
	if v, _ := e.svc.Engine().Value("regen"); v != false {
		t.Errorf("the run did not act: %v", v)
	}
	out = e.want(200, "", "GET", "overview", nil)
	st := out["programs"].([]any)[0].(map[string]any)["state"].(map[string]any)
	if last := st["last_run"].(map[string]any); last["block"] != 1.0 || last["trigger"] != "user" || st["key"] != "markise" {
		t.Errorf("the state after a run: %v", st)
	}
	// "check" with a condition that does not hold runs nothing
	e.want(200, "", "POST", "programs/markise/run", map[string]any{"mode": "check"})
	// the history names the session's user, a token, or the addon
	for _, c := range []struct {
		info *addonkit.SessionInfo
		want string
	}{{&addonkit.SessionInfo{User: "anna"}, "anna"}, {&addonkit.SessionInfo{Token: "ci"}, "API token ci"}, {&addonkit.SessionInfo{}, "Logic"}, {nil, "Logic"}} {
		r := httptest.NewRequest("POST", "/", nil)
		if c.info != nil {
			r = addonkit.WithSession(r, c.info)
		}
		if got := author(r).Name; got != c.want {
			t.Errorf("author: %q, want %q", got, c.want)
		}
	}
}

func TestImportAPI(t *testing.T) {
	e := newEnv(t)
	out := e.want(200, "", "GET", "import", nil)
	if out["system"] != false || out["pending"] != nil {
		t.Fatalf("nothing to import yet: %v", out)
	}
	e.want(404, "no-regadom", "POST", "import/preview?from=system", nil)
	e.want(400, "invalid-from", "POST", "import/preview?from=elsewhere", nil)
	e.want(400, "invalid-source", "POST", "import/preview?source=..%2Fetc", fixture(t, "fixture"))
	e.want(422, "unreadable", "POST", "import/preview", "this is neither a backup nor a database, however long it may be ....................")
	e.want(422, "unreadable", "POST", "import/preview", "<html/>")

	// a dry run of the uploaded fixture: the report, nothing written
	out = e.want(200, "", "POST", "import/preview?lang=en", fixture(t, "fixture"))
	id, _ := out["id"].(string)
	rep := out["report"].(map[string]any)
	if len(id) != 16 || rep["source"] != "ccu" || rep["kind"] != "regadom" || len(rep["programs"].([]any)) != 20 || len(rep["variables"].([]any)) != 44 {
		t.Fatalf("preview: %v %v", id, rep["source"])
	}
	if p, _ := e.svc.Program("kalender"); p.Program != nil {
		t.Fatal("a dry run wrote a program")
	}
	if out := e.want(200, "", "GET", "import", nil); out["pending"].(map[string]any)["id"] != id {
		t.Errorf("the pending dry run: %v", out["pending"])
	}
	status, md, hdr := e.raw("GET", "import/"+id+"/report.md?lang=de", nil)
	if status != 200 || !strings.HasPrefix(string(md), "# Importbericht") || !strings.HasPrefix(hdr.Get("Content-Type"), "text/markdown") || !strings.Contains(hdr.Get("Content-Disposition"), "import-ccu.md") {
		t.Errorf("report: %d %.40s %v", status, md, hdr)
	}
	e.want(404, "no-preview", "GET", "import/0000000000000000/report.md", nil)
	e.want(404, "no-preview", "POST", "import/0000000000000000/apply", nil)
	e.want(404, "no-preview", "DELETE", "import/0000000000000000", nil)
	// the words of the report's codes, both languages, with the placeholder left in
	codes := e.want(200, "", "GET", "import/codes", nil)
	if c := codes[importer.ReasonMissing].(map[string]any); !strings.Contains(c["de"].(string), "%s") || c["de"] == c["en"] || len(codes) != len(importer.Codes()) {
		t.Errorf("codes: %v", c)
	}

	// Übernehmen: one step; the programs, the variables with their values, the ledger
	out = e.want(200, "", "POST", "import/"+id+"/apply", nil)
	res := out["result"].(map[string]any)
	if res["programs"] != 18.0 || res["variables"] != 38.0 || out["report"] == nil {
		t.Fatalf("applied: %v", res)
	}
	if p, ok := e.svc.Program("markise-bei-regen-einfahren"); !ok || !p.Program.Active || len(p.Problems) != 0 {
		t.Errorf("an imported program: %+v", p)
	}
	if v, _ := e.svc.Engine().Value("aussentemperatur-mittel"); v != 12.5 {
		t.Errorf("an imported value: %v", v)
	}
	if l, err := importer.LoadLedger(e.svc.Tree(), "ccu"); err != nil || len(l.Programs) != 18 {
		t.Errorf("the ledger: %+v %v", l, err)
	}
	e.want(404, "no-preview", "POST", "import/"+id+"/apply", nil) // it is used up
	e.svc.Flush()
	if h, err := e.svc.History("programs/kalender.yaml", 5); err != nil || len(h) != 1 || h[0].Message != "Import from ccu: 18 programs, 38 variables" {
		t.Errorf("history: %+v %v", h, err)
	}

	// again, from the database the system kept, everything inactive: the same files are updated
	e.sys.regadom = fixture(t, "fixture")
	if out := e.want(200, "", "GET", "import", nil); out["system"] != true {
		t.Errorf("the system keeps one: %v", out)
	}
	out = e.want(200, "", "POST", "import/preview?from=system&all_inactive=1", nil)
	id = out["id"].(string)
	rep = out["report"].(map[string]any)
	first := rep["programs"].([]any)[6].(map[string]any)
	if rep["all_inactive"] != true || first["key"] != "markise-bei-regen-einfahren" || first["updated"] != true || first["active"] != false {
		t.Errorf("the second dry run: %v", first)
	}
	// a newer dry run replaces the older; discarding ends it
	out = e.want(200, "", "POST", "import/preview?from=system&source=ferienhaus", nil)
	if out["id"] == id || out["report"].(map[string]any)["programs"].([]any)[6].(map[string]any)["key"] != "markise-bei-regen-einfahren-2" {
		t.Errorf("another source: %v", out["id"])
	}
	e.want(404, "no-preview", "POST", "import/"+id+"/apply", nil)
	id = out["id"].(string)
	e.want(200, "", "DELETE", "import/"+id, nil)
	e.want(404, "no-preview", "DELETE", "import/"+id, nil)
	// a dry run nobody decided on is dropped after an hour
	out = e.want(200, "", "POST", "import/preview", fixture(t, "devices"))
	id = out["id"].(string)
	e.now = e.now.Add(previewTTL + time.Second)
	e.want(404, "no-preview", "POST", "import/"+id+"/apply", nil)
	if out := e.want(200, "", "GET", "import", nil); out["pending"] != nil {
		t.Errorf("an old dry run is still offered: %v", out["pending"])
	}
	// the devices this system has decide what stays active
	e.sys.channels = map[string]bool{"HmIP-RF/00111BE9AB9012:4": true}
	out = e.want(200, "", "POST", "import/preview", fixture(t, "devices"))
	devs := out["report"].(map[string]any)["devices"].([]any)
	if len(devs) != 4 || devs[3].(map[string]any)["known"] != true || devs[0].(map[string]any)["known"] != false {
		t.Errorf("devices: %v", devs)
	}
	// what the service refuses to store is said, and the dry run stays for another try
	if os.Getuid() != 0 {
		id = out["id"].(string)
		dir := e.svc.Tree().Dir + "/variables"
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		e.want(500, "failed", "POST", "import/"+id+"/apply", nil)
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		e.want(200, "", "POST", "import/"+id+"/apply", nil)
	}
	// a damaged record of the earlier import is said, not ignored
	if _, err := e.svc.Tree().WriteExtra(importer.LedgerPath("ccu"), []byte("source: [")); err != nil {
		t.Fatal(err)
	}
	e.want(409, "ledger", "POST", "import/preview", fixture(t, "devices"))
	if plural(1, "program") != "1 program" || plural(0, "program") != "0 programs" {
		t.Error("plural")
	}
}

// Without the system (no device list, no kept database) the page still works.
func TestWithoutSystem(t *testing.T) {
	svc, err := service.Open(service.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	e := &env{t: t, svc: svc}
	e.srv = newServer(t, Options{Service: svc})
	if out := e.want(200, "", "GET", "import", nil); out["system"] != false {
		t.Errorf("%v", out)
	}
	e.want(404, "no-regadom", "POST", "import/preview?from=system", nil)
	out := e.want(200, "", "POST", "import/preview", fixture(t, "devices"))
	if devs := out["report"].(map[string]any)["devices"].([]any); devs[3].(map[string]any)["known"] != false {
		t.Errorf("without a device list every device is missing: %v", devs[3])
	}
	e.want(200, "", "GET", "overview", nil)
}

// What is neither the service's nor the engine's refusal is a 500 with the text.
func TestFail(t *testing.T) {
	w := httptest.NewRecorder()
	fail(w, errors.New("the disk is full"))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "the disk is full") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	fail(w, &service.Error{Code: "read-only", Msg: "no"})
	if w.Code != 422 {
		t.Errorf("%d", w.Code)
	}
	w = httptest.NewRecorder()
	fail(w, &service.Error{Code: "exists", Msg: "no"})
	if w.Code != 409 {
		t.Errorf("%d", w.Code)
	}
}
