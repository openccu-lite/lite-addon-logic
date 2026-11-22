package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

func program(name string) map[string]any {
	return map[string]any{"program": map[string]any{"name": name, "active": false, "visible": true, "operable": true, "rules": []any{
		map[string]any{"when": map[string]any{"op": "any", "groups": []any{map[string]any{"op": "all", "lines": []any{
			map[string]any{"variable": "regen", "op": "eq", "value": true, "trigger": "change"},
			map[string]any{"device": map[string]any{"interface": "HmIP-RF", "address": "A:1", "param": "LEVEL"}, "op": "gt", "value": 0.5, "not": true, "trigger": "check"},
		}}}}, "cancel_pending": true, "then": []any{
			map[string]any{"kind": "set", "device": map[string]any{"interface": "HmIP-RF", "address": "A:1", "param": "LEVEL"}, "to": 0, "after": "1h30min"},
		}},
		map[string]any{"else": true, "then": []any{}},
	}}}
}

// A program is created, changed and removed through the API; what the model refuses is refused
// with its reason, and a program others run cannot be removed.
func TestProgramRoutes(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.CreateVariable(&model.Variable{Name: "Regen", Type: model.VarBool, Visible: true}, nil, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	out := e.want(http.StatusCreated, "", "POST", "programs", program("Markise bei Regen"))
	p := out["program"].(map[string]any)
	if p["key"] != "markise-bei-regen" {
		t.Fatalf("created: %v", p)
	}
	stored, _ := e.svc.Program("markise-bei-regen")
	line := stored.Program.Rules[0].When.Groups[0].Lines[1]
	if !line.Not || line.Op != model.OpGt || stored.Program.Rules[0].Then[0].After.String() != "1h30min" || !stored.Program.Rules[0].CancelPending || !stored.Program.Rules[1].Else {
		t.Errorf("stored: %+v", stored.Program.Rules)
	}
	// the same name again: a key of its own
	out = e.want(http.StatusCreated, "", "POST", "programs", program("Markise bei Regen"))
	if out["program"].(map[string]any)["key"] != "markise-bei-regen-2" {
		t.Errorf("second: %v", out)
	}

	// changed: the name, active
	body := program("Markise einfahren")
	body["program"].(map[string]any)["active"] = true
	out = e.want(http.StatusOK, "", "PUT", "programs/markise-bei-regen", body)
	if p := out["program"].(map[string]any); p["name"] != "Markise einfahren" || p["active"] != true || p["key"] != "markise-bei-regen" {
		t.Errorf("changed: %v", p)
	}

	// refused: no name, a variable that does not exist on an active program, a body without a program, an unknown one
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs", program(" "))
	bad := program("X")
	bad["program"].(map[string]any)["active"] = true
	bad["program"].(map[string]any)["rules"].([]any)[0].(map[string]any)["when"].(map[string]any)["groups"].([]any)[0].(map[string]any)["lines"].([]any)[0].(map[string]any)["variable"] = "gibt-es-nicht"
	if out := e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs", bad); !strings.Contains(out["message"].(string), "gibt-es-nicht") {
		t.Errorf("message: %v", out)
	}
	e.want(http.StatusBadRequest, "invalid-body", "POST", "programs", map[string]any{})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "programs", "{")
	e.want(http.StatusBadRequest, "invalid-body", "PUT", "programs/markise-bei-regen", map[string]any{})
	e.want(http.StatusBadRequest, "invalid-body", "PUT", "programs/markise-bei-regen", "{")
	e.want(http.StatusNotFound, "not-found", "PUT", "programs/nichts", program("X"))
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "programs/markise-bei-regen", program(""))

	// a program another one runs cannot be removed
	user := program("Aufrufer")
	user["program"].(map[string]any)["rules"] = []any{map[string]any{"when": map[string]any{"op": "any", "groups": []any{}}, "then": []any{
		map[string]any{"kind": "run", "program": "markise-bei-regen-2", "mode": "check"}}}}
	e.want(http.StatusCreated, "", "POST", "programs", user)
	if out := e.want(http.StatusConflict, "in-use", "DELETE", "programs/markise-bei-regen-2", nil); len(out["users"].([]any)) != 1 {
		t.Errorf("users: %v", out)
	}
	e.want(http.StatusOK, "", "DELETE", "programs/aufrufer", nil)
	e.want(http.StatusOK, "", "DELETE", "programs/markise-bei-regen-2", nil)
	e.want(http.StatusNotFound, "not-found", "DELETE", "programs/markise-bei-regen-2", nil)
	if _, ok := e.svc.Program("markise-bei-regen-2"); ok {
		t.Error("still there")
	}
}

// The picker's routes hand on what the system says.
func TestDevicesAndDatapoints(t *testing.T) {
	e := newEnv(t)
	min, max := 0.0, 1.0
	e.sys.devices = []system.Device{{Interface: "HmIP-RF", Address: "A", Name: "Rollladen", Type: "HmIP-BROLL", Channels: []system.Channel{{Address: "A:1", Name: "Rollladen", Type: "SHUTTER", Rooms: []string{"Bad"}}}}}
	e.sys.params = map[string][]system.Param{"HmIP-RF/A:1": {{ID: "LEVEL", Type: "FLOAT", Read: true, Write: true, Min: &min, Max: &max, Unit: "100%"}}}
	out := e.want(http.StatusOK, "", "GET", "devices?lang=de", nil)
	devs := out["devices"].([]any)
	if len(devs) != 1 || devs[0].(map[string]any)["channels"].([]any)[0].(map[string]any)["rooms"].([]any)[0] != "Bad" {
		t.Errorf("devices: %v", out)
	}
	out = e.want(http.StatusOK, "", "GET", "datapoints?channel=HmIP-RF/A:1&channel=HmIP-RF/B:1", nil)
	chs := out["channels"].(map[string]any)
	if len(chs) != 1 || chs["HmIP-RF/A:1"].([]any)[0].(map[string]any)["unit"] != "100%" {
		t.Errorf("datapoints: %v", out)
	}
	e.want(http.StatusBadRequest, "invalid-channel", "GET", "datapoints", nil)
	e.want(http.StatusBadRequest, "invalid-channel", "GET", "datapoints?channel=nonsense", nil)
	e.want(http.StatusBadRequest, "invalid-channel", "GET", "datapoints?"+strings.Repeat("channel=HmIP-RF/A:1&", MaxDatapointChannels+1), nil)
	e.sys.fail = errors.New("down")
	e.want(http.StatusBadGateway, "system", "GET", "devices", nil)
	e.want(http.StatusBadGateway, "system", "GET", "datapoints?channel=HmIP-RF/A:1", nil)

	// without a system: nothing to pick from, and no error
	svc, err := service.Open(service.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	bare := &env{t: t, svc: svc}
	bare.srv = newServer(t, Options{Service: svc})
	if out := bare.want(http.StatusOK, "", "GET", "devices", nil); len(out["devices"].([]any)) != 0 {
		t.Errorf("no system: %v", out)
	}
	if out := bare.want(http.StatusOK, "", "GET", "datapoints?channel=HmIP-RF/A:1", nil); len(out["channels"].(map[string]any)) != 0 {
		t.Errorf("no system: %v", out)
	}
}

// A time module's next times, asked before it is stored.
func TestTimePreview(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	clock := engine.NewFakeClock(time.Date(2026, 11, 21, 6, 0, 0, 0, loc))
	svc, err := service.Open(service.Options{Dir: t.TempDir(), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e := &env{t: t, svc: svc}
	e.srv = newServer(t, Options{Service: svc})
	out := e.want(http.StatusOK, "", "POST", "time/preview", map[string]any{"time": map[string]any{"kind": "daily", "at": "22:00", "duration": "8h"}})
	next := out["next"].([]any)
	if len(next) != 5 || next[0].(map[string]any)["at"] != "2026-11-21T22:00:00+01:00" || next[0].(map[string]any)["kind"] != "begin" || next[1].(map[string]any)["kind"] != "end" || out["no_position"] != false {
		t.Errorf("daily range: %v", out)
	}
	// sunrise without a location: said, and no times
	out = e.want(http.StatusOK, "", "POST", "time/preview", map[string]any{"time": map[string]any{"kind": "daily", "astro": "sunrise"}})
	if out["no_position"] != true || len(out["next"].([]any)) != 0 {
		t.Errorf("astro: %v", out)
	}
	// what the scheduler cannot run is refused with the reason, without a file's name and line
	out = e.want(http.StatusUnprocessableEntity, "invalid", "POST", "time/preview", map[string]any{"time": map[string]any{"kind": "weekly", "at": "07:00"}})
	if out["message"] != `a weekly module needs "weekdays"` {
		t.Errorf("message: %v", out)
	}
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "time/preview", map[string]any{"time": map[string]any{"kind": "daily", "at": "25:00"}})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "time/preview", map[string]any{})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "time/preview", "{")
}

// The triggers page's device side: values, last reports, how often.
func TestSources(t *testing.T) {
	e := newEnv(t)
	ref := model.Ref{Interface: "HmIP-RF", Address: "A:1", Param: "LEVEL"}
	e.svc.Engine().DeviceEvent(ref, 0.5)
	out := e.want(http.StatusOK, "", "GET", "sources", nil)
	dev := out["devices"].(map[string]any)["HmIP-RF/A:1/LEVEL"].(map[string]any)
	if dev["value"] != 0.5 || dev["time"] == nil || out["now"] == nil {
		t.Errorf("sources: %v", out)
	}
}

// The webhooks' secrets through the API: names only, never a value.
func TestSecretRoutes(t *testing.T) {
	e := newEnv(t)
	out := e.want(http.StatusOK, "", "GET", "secrets", nil)
	if len(out["secrets"].([]any)) != 0 || len(out["missing"].([]any)) != 0 {
		t.Fatalf("at first: %v", out)
	}
	out = e.want(http.StatusOK, "", "PUT", "secrets/token", map[string]any{"value": "s3cret"})
	if got := out["secrets"].([]any); len(got) != 1 || got[0].(map[string]any)["name"] != "token" || strings.Contains(fmt.Sprint(out), "s3cret") {
		t.Errorf("entered: %v", out)
	}
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "secrets/Token", map[string]any{"value": "x"})
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "secrets/empty", map[string]any{"value": ""})
	e.want(http.StatusBadRequest, "invalid-body", "PUT", "secrets/token", map[string]any{"wert": "x"})
	// a program that names it, and one more that is not there
	hook := map[string]any{"program": map[string]any{"name": "Melden", "visible": true, "operable": true, "rules": []any{map[string]any{
		"when": map[string]any{"op": "all", "groups": []any{}},
		"then": []any{
			map[string]any{"kind": "webhook", "webhook": map[string]any{"method": "POST", "url": "http://127.0.0.1:1/x?k={secret:token}", "headers": []any{map[string]any{"name": "X-Key", "value": "{secret:other}"}}, "body_type": "json", "body": "{}"}},
			map[string]any{"kind": "wake", "wake": map[string]any{"mac": "AA:BB:CC:00:11:22", "broadcast": "192.168.1.255"}},
		}}}}}
	made := e.want(http.StatusCreated, "", "POST", "programs", hook)
	then := made["program"].(map[string]any)["rules"].([]any)[0].(map[string]any)["then"].([]any)
	if w := then[0].(map[string]any)["webhook"].(map[string]any); w["method"] != "POST" || w["body_type"] != "json" || then[1].(map[string]any)["wake"].(map[string]any)["mac"] != "AA:BB:CC:00:11:22" {
		t.Errorf("stored: %v", then)
	}
	out = e.want(http.StatusOK, "", "GET", "secrets", nil)
	if fmt.Sprint(out["missing"]) != "[other]" || fmt.Sprint(out["secrets"].([]any)[0].(map[string]any)["users"]) != "[melden]" {
		t.Errorf("used: %v", out)
	}
	out = e.want(http.StatusConflict, "in-use", "DELETE", "secrets/token", nil)
	if fmt.Sprint(out["users"]) != "[melden]" {
		t.Errorf("in use: %v", out)
	}
	e.want(http.StatusNotFound, "not-found", "DELETE", "secrets/nope", nil)
	// what the model refuses, with its reason
	bad := map[string]any{"program": map[string]any{"name": "Falsch", "rules": []any{map[string]any{"when": map[string]any{"op": "all", "groups": []any{}},
		"then": []any{map[string]any{"kind": "webhook", "webhook": map[string]any{"method": "DELETE", "url": "http://h/"}}}}}}}
	if out := e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs", bad); !strings.Contains(fmt.Sprint(out["message"]), "is not one of GET, POST, PUT") {
		t.Errorf("refused: %v", out)
	}
	e.want(http.StatusOK, "", "DELETE", "programs/melden", nil)
	out = e.want(http.StatusOK, "", "DELETE", "secrets/token", nil)
	if len(out["secrets"].([]any)) != 0 || len(out["missing"].([]any)) != 0 {
		t.Errorf("removed: %v", out)
	}
}

// What a "channel" activity writes is checked against the channel's datapoints when the program
// is saved; a channel the system does not know is not.
func TestChannelValuesAreCheckedAtSave(t *testing.T) {
	e := newEnv(t)
	lo, hi := 0.0, 1.01
	e.sys.params = map[string][]system.Param{"HmIP-RF/A:8": {
		{ID: "COLOR", Type: "ENUM", Write: true, Values: []string{"BLACK", "BLUE", "RED"}},
		{ID: "LEVEL", Type: "FLOAT", Write: true, Min: &lo, Max: &hi},
		{ID: "STATE", Type: "BOOL", Read: true},
	}}
	body := func(channel string, values ...any) map[string]any {
		list := []any{}
		for i := 0; i+1 < len(values); i += 2 {
			list = append(list, map[string]any{"param": values[i], "value": values[i+1]})
		}
		return map[string]any{"program": map[string]any{"name": "Licht", "visible": true, "operable": true, "rules": []any{
			map[string]any{"when": map[string]any{"op": "all", "groups": []any{}}, "then": []any{
				map[string]any{"kind": "variable", "variable": "nope", "to": 1},
				map[string]any{"kind": "channel", "channel": channel, "values": list},
			}}}}}
	}
	for _, c := range []struct {
		values []any
		msg    string
	}{
		{[]any{"COLOR", "GREEN"}, `rule 1: HmIP-RF/A:8: COLOR has no entry "GREEN"`},
		{[]any{"COLOR", "RED", "LEVEL", 2}, "rule 1: HmIP-RF/A:8: LEVEL: 2 is outside 0 to 1.01"},
		{[]any{"STATE", true}, "rule 1: HmIP-RF/A:8: STATE cannot be written"},
		{[]any{"COLOR_BEHAVIOUR", 1}, "rule 1: HmIP-RF/A:8: the channel has no datapoint COLOR_BEHAVIOUR"},
	} {
		out := e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs", body("HmIP-RF/A:8", c.values...))
		if out["message"] != c.msg {
			t.Errorf("%v: %v", c.values, out["message"])
		}
	}
	made := e.want(http.StatusCreated, "", "POST", "programs", body("HmIP-RF/A:8", "COLOR", "RED", "LEVEL", 0.5))
	then := made["program"].(map[string]any)["rules"].([]any)[0].(map[string]any)["then"].([]any)[1].(map[string]any)
	if then["channel"] != "HmIP-RF/A:8" || fmt.Sprint(then["values"]) != "[map[param:COLOR value:RED] map[param:LEVEL value:0.5]]" {
		t.Errorf("stored: %v", then)
	}
	// the overview carries the channel's name
	e.sys.names = map[string]string{"HmIP-RF/A:8": "Taster Flur"}
	if out := e.want(http.StatusOK, "", "GET", "overview", nil); out["names"].(map[string]any)["HmIP-RF/A:8"] != "Taster Flur" {
		t.Errorf("names: %v", out["names"])
	}
	// changed the same way; a channel nobody knows, and a system that does not answer, pass
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "programs/licht", body("HmIP-RF/A:8", "LEVEL", 5))
	e.want(http.StatusOK, "", "PUT", "programs/licht", body("HmIP-RF/NEW:1", "ANYTHING", 5))
	e.sys.fail = errors.New("down")
	e.want(http.StatusOK, "", "PUT", "programs/licht", body("HmIP-RF/A:8", "LEVEL", 5))
	// what the model refuses is refused before the system is asked
	e.sys.fail = nil
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "programs/licht", body("nope", "LEVEL", 5))
	e.want(http.StatusUnprocessableEntity, "invalid", "PUT", "programs/licht", body("HmIP-RF/A:8"))
	// a daemon without a system to ask (the tests' own) takes what the model takes
	bare := &env{t: t, svc: e.svc}
	bare.srv = newServer(t, Options{Service: e.svc})
	bare.want(http.StatusOK, "", "PUT", "programs/licht", body("HmIP-RF/A:8", "LEVEL", 5))
}

// A display activity is checked against the channel at save: it must take the combined text,
// and its lists must have the icon and the sound where they are sent.
func TestDisplayIsCheckedAtSave(t *testing.T) {
	e := newEnv(t)
	e.sys.params = map[string][]system.Param{
		"HmIP-RF/W:3": {
			{ID: "COMBINED_PARAMETER", Type: "STRING", Write: true},
			{ID: "DISPLAY_DATA_ICON", Type: "ENUM", Write: true, Values: model.DisplayIcons[:31]},
			{ID: "ACOUSTIC_NOTIFICATION_SELECTION", Type: "ENUM", Write: true, Values: []string{"LOW_BATTERY", "DISARMED", "EXTERNALLY_ARMED"}},
		},
		"HmIP-RF/W:1": {{ID: "PRESS_SHORT", Type: "ACTION", Read: true}},
		"HmIP-RF/W:4": {{ID: "COMBINED_PARAMETER", Type: "STRING", Write: true}},
	}
	body := func(channel string, display map[string]any) map[string]any {
		return map[string]any{"program": map[string]any{"name": "Anzeige", "visible": true, "operable": true, "rules": []any{
			map[string]any{"when": map[string]any{"op": "all", "groups": []any{}}, "then": []any{
				map[string]any{"kind": "display", "channel": channel, "display": display},
			}}}}}
	}
	line := func(text, icon string) map[string]any {
		return map[string]any{"lines": []any{map[string]any{"text": text, "icon": icon}}}
	}
	for _, c := range []struct {
		channel string
		display map[string]any
		msg     string
	}{
		{"HmIP-RF/W:1", line("a", ""), "rule 1: HmIP-RF/W:1: this channel is no display (the channel has no datapoint COMBINED_PARAMETER)"},
		{"HmIP-RF/W:3", line("a", "CLOCK"), "rule 1: HmIP-RF/W:3: line 1: the display has no icon CLOCK"},
		{"HmIP-RF/W:3", map[string]any{"sound": "INTERNALLY_ARMED"}, "rule 1: HmIP-RF/W:3: the display has no signal tone INTERNALLY_ARMED"},
		{"HmIP-RF/W:3", map[string]any{"sound": "EVENT"}, "rule 1: HmIP-RF/W:3: the display has no signal tone EVENT"},
	} {
		out := e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs", body(c.channel, c.display))
		if out["message"] != c.msg {
			t.Errorf("%v: %v", c.display, out["message"])
		}
	}
	// what the model refuses is refused first
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs", body("HmIP-RF/W:3", line("a, b", "")))
	made := e.want(http.StatusCreated, "", "POST", "programs", body("HmIP-RF/W:3", map[string]any{"lines": []any{map[string]any{"text": "Tür {value}", "icon": "BELL"}}, "sound": "DISARMED", "repetitions": 2, "interval": 10}))
	then := made["program"].(map[string]any)["rules"].([]any)[0].(map[string]any)["then"].([]any)[0].(map[string]any)
	if then["channel"] != "HmIP-RF/W:3" || fmt.Sprint(then["display"]) != "map[interval:10 lines:[map[icon:BELL text:Tür {value}]] repetitions:2 sound:DISARMED]" {
		t.Errorf("stored: %v", then)
	}
	// the overview carries the channel's name
	e.sys.names = map[string]string{"HmIP-RF/W:3": "Wandtaster"}
	if out := e.want(http.StatusOK, "", "GET", "overview", nil); out["names"].(map[string]any)["HmIP-RF/W:3"] != "Wandtaster" {
		t.Errorf("names: %v", out["names"])
	}
	// a display whose description has no lists, a channel nobody knows, a system that does not answer: taken
	e.want(http.StatusOK, "", "PUT", "programs/anzeige", body("HmIP-RF/W:4", line("a", "CLOCK")))
	e.want(http.StatusOK, "", "PUT", "programs/anzeige", body("HmIP-RF/NEW:3", line("a", "CLOCK")))
	e.sys.fail = errors.New("down")
	e.want(http.StatusOK, "", "PUT", "programs/anzeige", body("HmIP-RF/W:1", line("a", "")))
}
