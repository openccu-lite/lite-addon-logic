package httpapi

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

func counterParam(id, unit string, max float64) system.Param {
	return system.Param{ID: id, Type: "FLOAT", Read: true, Unit: unit, Max: &max}
}

// The helpers there are and the channels that could have one; making and removing one.
func TestHelpersAPI(t *testing.T) {
	e := newEnv(t)
	e.sys.devices = []system.Device{
		{Interface: "HmIP-RF", Address: "000A", Name: "Waschmaschine", Type: "HmIP-PSM", Channels: []system.Channel{
			{Address: "000A:0", Type: "MAINTENANCE"}, {Address: "000A:6", Name: "Waschmaschine Messung", Type: "ENERGIE_METER_TRANSMITTER"}}},
		{Interface: "HmIP-RF", Address: "000B", Name: "Wetter", Type: "HmIP-SWO-B", Channels: []system.Channel{{Address: "000B:1", Type: "WEATHER_TRANSMIT"}}},
		{Interface: "HmIP-RF", Address: "000E", Type: "HmIP-ESI", Channels: []system.Channel{{Address: "000E:1", Type: "ENERGIE_METER_TRANSMITTER"}, {Address: "000E:2", Type: "ENERGIE_METER_TRANSMITTER"}}},
		{Interface: "BidCos-RF", Address: "OEQ1", Name: "Thermometer", Type: "HM-WDS40-TH-I", Channels: []system.Channel{{Address: "OEQ1:1", Type: "WEATHER"}}},
	}
	e.sys.params = map[string][]system.Param{
		"HmIP-RF/000A:6": {counterParam("ENERGY_COUNTER", "Wh", 838859.1), {ID: "ENERGY_COUNTER_OVERFLOW", Type: "BOOL", Read: true}},
		// a weather sensor without a rain gauge: sunshine only
		"HmIP-RF/000B:1": {counterParam("SUNSHINEDURATION", "min", 16383)},
		// the HmIP-ESI's first channel has no counter; the second reports gas, without a unit here
		"HmIP-RF/000E:1":   {{ID: "CHANNEL_OPERATION_MODE", Type: "ENUM", Read: true}},
		"HmIP-RF/000E:2":   {counterParam("GAS_VOLUME", "", 10995116277.59), {ID: "ENERGY_COUNTER", Type: "FLOAT"}},
		"BidCos-RF/OEQ1:1": {{ID: "TEMPERATURE", Type: "FLOAT", Read: true}},
	}
	e.sys.names = map[string]string{"HmIP-RF/000A:6": "Waschmaschine Messung"}
	out := e.want(http.StatusOK, "", "GET", "helpers", nil)
	if out["system"] != true || len(out["helpers"].([]any)) != 0 {
		t.Errorf("empty: %v", out)
	}
	want := []any{
		map[string]any{"kind": "energy", "channel": "HmIP-RF/000A:6", "name": "Waschmaschine Messung", "type": "HmIP-PSM", "unit": "Wh", "roles": []any{"total", "today", "yesterday"}},
		map[string]any{"kind": "sunshine", "channel": "HmIP-RF/000B:1", "name": "Wetter", "type": "HmIP-SWO-B", "unit": "min", "roles": []any{"total", "today", "yesterday"}},
		map[string]any{"kind": "esi", "channel": "HmIP-RF/000E:2", "type": "HmIP-ESI", "roles": []any{"total", "today", "yesterday"}},
	}
	if !reflect.DeepEqual(out["candidates"], want) {
		t.Errorf("candidates: %v", out["candidates"])
	}
	// making one: the unit is the device's
	body := map[string]any{"kind": "energy", "channel": "HmIP-RF/000A:6", "names": map[string]string{"total": "Energiezähler Waschmaschine", "today": "Energie heute Waschmaschine"}}
	made := e.want(http.StatusCreated, "", "POST", "helpers", body)
	vars := made["variables"].([]any)
	if len(vars) != 2 || vars[0].(map[string]any)["unit"] != "Wh" || vars[0].(map[string]any)["key"] != "energiezaehler-waschmaschine" {
		t.Errorf("made: %v", made)
	}
	out = e.want(http.StatusOK, "", "GET", "helpers", nil)
	helpers := out["helpers"].([]any)
	if len(helpers) != 1 || len(out["candidates"].([]any)) != 2 {
		t.Fatalf("after: %v", out)
	}
	h := helpers[0].(map[string]any)
	if h["kind"] != "energy" || h["channel"] != "HmIP-RF/000A:6" || h["name"] != "Waschmaschine Messung" ||
		!reflect.DeepEqual(h["variables"], map[string]any{"total": "energiezaehler-waschmaschine", "today": "energie-heute-waschmaschine"}) {
		t.Errorf("helper: %v", h)
	}
	// the overview names the helper's channel
	over := e.want(http.StatusOK, "", "GET", "overview", nil)
	if over["names"].(map[string]any)["HmIP-RF/000A:6"] != "Waschmaschine Messung" {
		t.Errorf("names: %v", over["names"])
	}
	// the day's amount is read-only, the total takes a meter reading
	e.want(http.StatusUnprocessableEntity, "read-only", "POST", "variables/energie-heute-waschmaschine/value", map[string]any{"value": 5})
	e.want(http.StatusOK, "", "POST", "variables/energiezaehler-waschmaschine/value", map[string]any{"value": 20000})
	// refused
	e.want(http.StatusConflict, "exists", "POST", "helpers", body)
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "helpers", map[string]any{"kind": "rain", "channel": "HmIP-RF/000B:1", "names": map[string]string{"total": "Regen"}})
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "helpers", map[string]any{"kind": "energy", "channel": "HmIP-RF/000F:6", "names": map[string]string{"total": "Weg"}})
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "helpers", map[string]any{"kind": "heat", "channel": "HmIP-RF/000A:6", "names": map[string]string{"total": "X"}})
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "helpers", map[string]any{"kind": "energy", "channel": "x", "names": map[string]string{"total": "X"}})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "helpers", `{"kind": 5}`)
	// the kind's own unit when the device names none
	gas := e.want(http.StatusCreated, "", "POST", "helpers", map[string]any{"kind": "esi", "channel": "HmIP-RF/000E:2", "names": map[string]string{"total": "Gas"}})
	if u, has := gas["variables"].([]any)[0].(map[string]any)["unit"]; has {
		t.Errorf("unit: %v", u)
	}
	// removing
	e.want(http.StatusNotFound, "not-found", "DELETE", "helpers/gone", nil)
	e.want(http.StatusOK, "", "DELETE", "helpers/energie-heute-waschmaschine", nil)
	if n := len(e.want(http.StatusOK, "", "GET", "helpers", nil)["helpers"].([]any)); n != 1 {
		t.Errorf("%d helpers left", n)
	}
	// the system does not answer: the helpers are listed, the candidates are not; none is made
	e.sys.fail = errors.New("down")
	out = e.want(http.StatusOK, "", "GET", "helpers?lang=en", nil)
	if out["system"] != false || len(out["helpers"].([]any)) != 1 || len(out["candidates"].([]any)) != 0 {
		t.Errorf("system down: %v", out)
	}
	e.want(http.StatusBadGateway, "system", "POST", "helpers", body)
	e.sys.fail = nil
	// no device fits: no candidates, and the datapoints are not asked for
	e.sys.devices = e.sys.devices[3:]
	if out = e.want(http.StatusOK, "", "GET", "helpers", nil); out["system"] != true || len(out["candidates"].([]any)) != 0 {
		t.Errorf("nothing fits: %v", out)
	}
	if model.HelperKindOf("energy") == nil {
		t.Fatal("kinds")
	}
}

// Without a system the helpers are listed and none can be made.
func TestHelpersWithoutSystem(t *testing.T) {
	e := newEnv(t)
	e.srv.Close()
	e.srv = newServer(t, Options{Service: e.svc})
	if out := e.want(http.StatusOK, "", "GET", "helpers", nil); out["system"] != false {
		t.Errorf("%v", out)
	}
	e.want(http.StatusBadGateway, "system", "POST", "helpers", map[string]any{"kind": "energy", "channel": "HmIP-RF/000A:6", "names": map[string]string{"total": "X"}})
}

// The CCU's counters, imported: the helpers count on from the readings the database holds -
// from the upload through the dry run and the apply to the device's next report.
func TestImportedCountersCountOn(t *testing.T) {
	e := newEnv(t)
	out := e.want(http.StatusOK, "", "POST", "import/preview?lang=en", fixture(t, "counters"))
	id, _ := out["id"].(string)
	rep := out["report"].(map[string]any)
	notes := map[string]string{}
	for _, v := range rep["variables"].([]any) {
		row := v.(map[string]any)
		var codes []string
		for _, n := range row["notes"].([]any) {
			codes = append(codes, n.(map[string]any)["code"].(string))
		}
		notes[row["name"].(string)] = row["status"].(string) + " " + strings.Join(codes, ",")
	}
	for name, want := range map[string]string{
		"svEnergyCounter_5001_000A1B2C3D4E12:1": "imported counter,helper",
		"svEnergyCounterOldVal_5001":            "skipped counter,helper-state",
		"svEnergyCounterToday_5001":             "imported helper-day",
		"svHmIPRainCounterToday_5101":           "imported counter,helper",
	} {
		if notes[name] != want {
			t.Errorf("%s: %q, want %q", name, notes[name], want)
		}
	}
	for _, p := range rep["programs"].([]any) {
		if row := p.(map[string]any); row["status"] != "skipped" {
			t.Errorf("a counter program was imported: %v", row)
		}
	}
	e.want(http.StatusOK, "", "POST", "import/"+id+"/apply", nil)
	value := func(key string) any {
		t.Helper()
		v, ok := e.svc.Engine().Value(key)
		if !ok {
			t.Fatalf("no variable %s", key)
		}
		return v
	}
	const total, today, yesterday = "svenergycounter-5001-000a1b2c3d4e12-1", "svenergycountertoday-5001", "svenergycounteryesterday-5001"
	if value(total) != 20100.5 || value(today) != 12.5 || value(yesterday) != 340.2 {
		t.Errorf("imported: %v %v %v", value(total), value(today), value(yesterday))
	}
	list := e.want(http.StatusOK, "", "GET", "helpers", nil)
	if n := len(list["helpers"].([]any)); n != 3 {
		t.Fatalf("%d helpers", n)
	}
	// the device reports: the CCU counted up to 211.75, the device is at 213.25 now
	ref, _ := model.ParseRef("HmIP-RF/000A1B2C3D4E12:1/ENERGY_COUNTER")
	e.svc.Engine().DeviceEvent(ref, 213.25)
	if value(total) != 20102.0 || value(today) != 14.0 {
		t.Errorf("counted on: %v %v", value(total), value(today))
	}
	rain, _ := model.ParseRef("HmIP-RF/000A1B2C3D4E13:1/RAIN_COUNTER")
	e.svc.Engine().DeviceEvent(rain, 102.0)
	if v := value("svhmipraincounter-5101-000a1b2c3d4e13-1"); v != 990.8 {
		t.Errorf("rain: %v", v)
	}
	if v := value("svhmipraincountertoday-5101"); v != 2.8 {
		t.Errorf("rain today: %v", v)
	}
	// a second import of the same database changes nothing about the helpers' variables
	out = e.want(http.StatusOK, "", "POST", "import/preview", fixture(t, "counters"))
	e.want(http.StatusOK, "", "POST", "import/"+out["id"].(string)+"/apply", nil)
	if n := len(e.want(http.StatusOK, "", "GET", "helpers", nil)["helpers"].([]any)); n != 3 {
		t.Errorf("%d helpers after the second import", n)
	}
	if n := len(e.svc.Variables()); n != 9 {
		t.Errorf("%d variables after the second import", n)
	}
}
