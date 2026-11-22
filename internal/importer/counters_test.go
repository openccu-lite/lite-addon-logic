package importer

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// channel adds a device with one channel of a type on an interface, and answers the channel's id.
func (b *build) channel(iface, device, address, channelType string, meta map[string]string) int {
	var ifc *regadom.Interface
	for _, i := range b.db.Interfaces {
		if i.Name == iface {
			ifc = i
		}
	}
	if ifc == nil {
		ifc = &regadom.Interface{Object: b.obj(iface)}
		b.db.Interfaces[ifc.ID] = ifc
	}
	dev := &regadom.Device{Object: b.obj("Gerät " + address), InterfaceID: ifc.ID, Label: device, Address: strings.Split(address, ":")[0]}
	b.db.Devices[dev.ID] = dev
	ch := &regadom.Channel{Object: b.obj("Kanal " + address), Address: address, DeviceID: dev.ID, Label: channelType}
	for k, v := range meta {
		ch.Meta[k] = v
	}
	b.db.Channels[ch.ID] = ch
	return ch.ID
}

func (b *build) counter(name, value string) *regadom.Variable {
	v := b.variable(name, flt, 0, value, map[string]string{"MIN": "0", "MAX": "1" + strings.Repeat("0", 305)})
	v.Flags = regadom.FlagVisible | regadom.FlagInternal
	return v
}

// eQ-3's counter variables become device helpers: each family by its names, with the values,
// the device's last reading and the day's amounts of the channel's metadata.
func TestCounterHelpers(t *testing.T) {
	b := newBuild()
	n := strconv.Itoa
	psm := b.channel("HmIP-RF", "HmIP-PSMCO", "000A1B2C3D4E5F:6", "ENERGIE_METER_TRANSMITTER", map[string]string{"energy0": "12.5", "energy1": "340.2", "energy0FeedIn": "1.5", "energyCounter30Days": "0,0"})
	esi := b.channel("HmIP-RF", "HmIP-ESI", "000E1B2C3D4E5F:2", "ENERGIE_METER_TRANSMITTER", nil)
	hm := b.channel("BidCos-RF", "HM-ES-PMSw1-Pl", "NEQ0000001:2", "POWERMETER", nil)
	wm := b.channel("BidCos-RF", "HM-ES-TX-WM", "NEQ0000002:1", "POWERMETER_IEC1", nil)
	swo := b.channel("HmIP-RF", "HmIP-SWO-PR", "000B1B2C3D4E5F:1", "WEATHER_TRANSMIT", nil)
	wsm := b.channel("HmIP-RF", "ELV-SH-WSM", "000C1B2C3D4E5F:2", "FLOW_METER_TRANSMITTER", map[string]string{"counter0": "33"})
	wds := b.channel("BidCos-RF", "HM-WDS100-C6-O", "OEQ0000003:1", "WEATHER", nil)
	th := b.channel("BidCos-RF", "HM-WDS40-TH-I", "OEQ0000004:1", "WEATHER_TH", nil)
	key := b.channel("HmIP-RF", "HmIP-WRC2", "000D1B2C3D4E5F:1", "KEY_TRANSCEIVER", nil)

	b.counter("svEnergyCounter_"+n(psm)+"_000A1B2C3D4E5F:6", "100.5")
	b.counter("svEnergyCounterOldVal_"+n(psm), "211.75")
	b.counter("svEnergyCounterFeedIn_"+n(psm)+"_000A1B2C3D4E5F:6", "323")
	b.counter("svEnergyCounterOldValFeedIn_"+n(psm), "434.25")
	b.counter("svEnergyCounter_"+n(esi)+"_000E1B2C3D4E5F:2", "5000")
	b.counter("svEnergyCounterOldVal_"+n(esi), "4000")
	b.counter("svEnergyCounter_"+n(hm)+"_NEQ0000001:2", "72651.7")
	b.counter("svEnergyCounterOldVal_"+n(hm), "6122.4")
	b.counter("svEnergyCounter_"+n(wm)+"_NEQ0000002:1", "10")
	b.counter("svEnergyCounterOldVal_"+n(wm), "9")
	latch := b.variable("svEnergyCounter_"+n(wm)+"_NEQ0000002:1_DEVICE_RESET", bin, regadom.SubBool, "true", nil)
	b.variable("svEnergyCounter_"+n(wm)+"_NEQ0000002:1_RESET", bin, regadom.SubBool, "false", nil)
	b.counter("svEnergyCounter_"+n(wm)+"_NEQ0000002:1_TMP_OLDVAL", "9")
	b.counter("svEnergyCounterGas_"+n(wm)+"_NEQ0000002:1", "545.5")
	b.counter("svEnergyCounterGasOldVal_"+n(wm), "656.75")
	b.variable("svEnergyCounterGas_"+n(wm)+"_NEQ0000002:1_DEVICE_RESET", bin, regadom.SubBool, "false", nil)
	b.counter("svEnergyCounterIEC_"+n(wm)+"_NEQ0000002:1", "768")
	b.counter("svEnergyCounterIECOldVal_"+n(wm), "879.25")
	b.counter("svHmIPRainCounter_"+n(swo)+"_000B1B2C3D4E5F:1", "990.5")
	b.counter("svHmIPRainCounterOldVal_"+n(swo), "1101.75")
	today := b.counter("svHmIPRainCounterToday_"+n(swo), "2.5")
	today.ChannelID = swo
	b.counter("svHmIPRainCounterYesterday_"+n(swo), "1.25")
	b.counter("svHmIPSunshineCounter_"+n(swo)+"_000B1B2C3D4E5F:1", "1435")
	old := b.counter("svHmIPSunshineCounterOldVal_"+n(swo), "1546")
	b.counter("svCounter_"+n(wsm)+"_000C1B2C3D4E5F:2", "777")
	b.counter("svCounterOldVal_"+n(wsm), "666")
	rt := b.variable(rainToday, flt, 0, "2.36", map[string]string{"UNIT": "mm", "MIN": "0", "MAX": "1000"})
	rt.ChannelID = wds
	ry := b.variable(rainYesterday, flt, 0, "1.18", map[string]string{"UNIT": "mm"})
	ry.ChannelID = wds
	// what is no helper: a channel that is gone, one of another type, a total that is missing,
	// rain variables on something that is no weather sensor, a name with another type
	b.counter("svEnergyCounter_31337_000F:6", "1")
	b.counter("svEnergyCounterOldVal_31337", "2")
	b.counter("svEnergyCounter_"+n(key)+"_000D1B2C3D4E5F:1", "3")
	b.counter("svHmIPRainCounterToday_"+n(key), "4")
	b.counter("svHmIPRainCounterTomorrow_"+n(swo), "5")
	b.variable("svCounter_"+n(wsm)+"_Text", str, regadom.SubString, "x", nil)
	other := b.variable(rainToday, flt, 0, "9", nil)
	other.ChannelID = th
	b.variable(rainYesterday, flt, 0, "9", nil)
	// a program reads the sunshine helper's old reading: it stays a variable as well
	b.program("Liest", true, ruleSpec{blocks: [][]lineSpec{{on(old.ID, regadom.CmpGreater, regadom.TrigUpdate, val("5", flt))}}, actions: []actionSpec{set(latch.ID, val("1", bin))}})
	b.program("Regentagesmenge aktualisieren", true)
	b.program("prgFlowCounter_"+n(wsm)+"_000C1B2C3D4E5F:2", true)

	res := b.run(t, Options{})
	type h struct{ kind, channel, role, unit string }
	got := map[string]h{}
	for _, v := range res.Variables {
		if _, err := v.Check(); err != nil {
			t.Errorf("%s does not fit the format: %v", v.Key, err)
		}
		if v.Helper != nil {
			got[v.Name] = h{v.Helper.Kind, v.Helper.Channel, v.Helper.Role, v.Unit}
			if v.Max != nil && *v.Max > 1e300 {
				t.Errorf("%s keeps the CCU's maximum", v.Name)
			}
		}
	}
	const (
		cPSM = "HmIP-RF/000A1B2C3D4E5F:6"
		cESI = "HmIP-RF/000E1B2C3D4E5F:2"
		cHM  = "BidCos-RF/NEQ0000001:2"
		cWM  = "BidCos-RF/NEQ0000002:1"
		cSWO = "HmIP-RF/000B1B2C3D4E5F:1"
		cWSM = "HmIP-RF/000C1B2C3D4E5F:2"
		cWDS = "BidCos-RF/OEQ0000003:1"
	)
	want := map[string]h{
		"svEnergyCounter_" + n(psm) + "_000A1B2C3D4E5F:6":       {"energy", cPSM, "total", "Wh"},
		"svEnergyCounterToday_" + n(psm):                        {"energy", cPSM, "today", "Wh"},
		"svEnergyCounterYesterday_" + n(psm):                    {"energy", cPSM, "yesterday", "Wh"},
		"svEnergyCounterFeedIn_" + n(psm) + "_000A1B2C3D4E5F:6": {"feed-in", cPSM, "total", "Wh"},
		"svEnergyCounterTodayFeedIn_" + n(psm):                  {"feed-in", cPSM, "today", "Wh"},
		"svEnergyCounterYesterdayFeedIn_" + n(psm):              {"feed-in", cPSM, "yesterday", "Wh"},
		"svEnergyCounter_" + n(esi) + "_000E1B2C3D4E5F:2":       {"esi", cESI, "total", ""},
		"svEnergyCounterToday_" + n(esi):                        {"esi", cESI, "today", ""},
		"svEnergyCounterYesterday_" + n(esi):                    {"esi", cESI, "yesterday", ""},
		"svEnergyCounter_" + n(hm) + "_NEQ0000001:2":            {"energy-hm", cHM, "total", "Wh"},
		"svEnergyCounter_" + n(wm) + "_NEQ0000002:1":            {"energy-wm", cWM, "total", "Wh"},
		"svEnergyCounterGas_" + n(wm) + "_NEQ0000002:1":         {"gas-wm", cWM, "total", "m³"},
		"svEnergyCounterIEC_" + n(wm) + "_NEQ0000002:1":         {"iec-wm", cWM, "total", ""},
		"svHmIPRainCounter_" + n(swo) + "_000B1B2C3D4E5F:1":     {"rain", cSWO, "total", "mm"},
		"svHmIPRainCounterToday_" + n(swo):                      {"rain", cSWO, "today", "mm"},
		"svHmIPRainCounterYesterday_" + n(swo):                  {"rain", cSWO, "yesterday", "mm"},
		"svHmIPSunshineCounter_" + n(swo) + "_000B1B2C3D4E5F:1": {"sunshine", cSWO, "total", "min"},
		"svCounter_" + n(wsm) + "_000C1B2C3D4E5F:2":             {"water", cWSM, "total", "l"},
		"svCounterToday_" + n(wsm):                              {"water", cWSM, "today", "l"},
		"svCounterYesterday_" + n(wsm):                          {"water", cWSM, "yesterday", "l"},
		"Regen heute":                                           {"rain-hm", cWDS, "today", "mm"},
		"Regen gestern":                                         {"rain-hm", cWDS, "yesterday", "mm"},
	}
	if !reflect.DeepEqual(got, want) {
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: %+v, want %+v", k, got[k], v)
			}
		}
		for k, v := range got {
			if _, ok := want[k]; !ok {
				t.Errorf("%s: %+v is no helper's", k, v)
			}
		}
	}
	// what each helper remembers, by its leading variable
	states := map[string]model.HelperState{}
	for key, st := range res.Helpers {
		states[variable(res, key).Name] = st
	}
	seen := func(old float64, latch bool) model.HelperState {
		return model.HelperState{Seen: true, Old: old, Latch: latch}
	}
	wantStates := map[string]model.HelperState{
		"svEnergyCounter_" + n(psm) + "_000A1B2C3D4E5F:6":       seen(211.75, false),
		"svEnergyCounterFeedIn_" + n(psm) + "_000A1B2C3D4E5F:6": seen(434.25, false),
		"svEnergyCounter_" + n(esi) + "_000E1B2C3D4E5F:2":       seen(4000, false),
		"svEnergyCounter_" + n(hm) + "_NEQ0000001:2":            seen(6122.4, false),
		"svEnergyCounter_" + n(wm) + "_NEQ0000002:1":            seen(9, true),
		"svEnergyCounterGas_" + n(wm) + "_NEQ0000002:1":         seen(656.75, false),
		"svEnergyCounterIEC_" + n(wm) + "_NEQ0000002:1":         seen(879.25, false),
		"svHmIPRainCounter_" + n(swo) + "_000B1B2C3D4E5F:1":     seen(1101.75, false),
		"svHmIPSunshineCounter_" + n(swo) + "_000B1B2C3D4E5F:1": seen(1546, false),
		"svCounter_" + n(wsm) + "_000C1B2C3D4E5F:2":             seen(666, false),
		"Regen heute": seen(0, false),
	}
	if !reflect.DeepEqual(states, wantStates) {
		t.Errorf("states: %+v", states)
	}
	// the values: the totals, the day's amounts from the channel's metadata
	value := func(name string) any {
		for _, v := range res.Variables {
			if v.Name == name {
				return res.Values[v.Key]
			}
		}
		t.Errorf("no variable %s", name)
		return nil
	}
	for name, want := range map[string]float64{
		"svEnergyCounter_" + n(psm) + "_000A1B2C3D4E5F:6": 100.5, "svEnergyCounterToday_" + n(psm): 12.5, "svEnergyCounterYesterday_" + n(psm): 340.2,
		"svEnergyCounterTodayFeedIn_" + n(psm): 1.5, "svEnergyCounterYesterdayFeedIn_" + n(psm): 0, "svCounterToday_" + n(wsm): 33,
		"svHmIPRainCounterToday_" + n(swo): 2.5, "Regen heute": 2.36, "svEnergyCounterToday_" + n(esi): 0,
	} {
		if got := value(name); got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	if v := variable(res, model.Slug("svHmIPRainCounterToday_"+n(swo))); v.Channel != cSWO || !v.Internal {
		t.Errorf("the channel binding and the flags stay: %+v", v)
	}
	// the report: what became of each
	for name, want := range map[string]string{
		"svEnergyCounter_" + n(psm) + "_000A1B2C3D4E5F:6":            "imported counter,helper",
		"svEnergyCounterOldVal_" + n(psm):                            "skipped counter,helper-state",
		"svEnergyCounter_" + n(wm) + "_NEQ0000002:1_DEVICE_RESET":    "imported counter,helper-state",
		"svEnergyCounter_" + n(wm) + "_NEQ0000002:1_RESET":           "skipped counter,helper-unused",
		"svEnergyCounter_" + n(wm) + "_NEQ0000002:1_TMP_OLDVAL":      "skipped counter,helper-unused",
		"svEnergyCounterGas_" + n(wm) + "_NEQ0000002:1_DEVICE_RESET": "skipped counter,helper-state",
		"svHmIPSunshineCounterOldVal_" + n(swo):                      "imported counter,helper-state",
		"svEnergyCounterToday_" + n(psm):                             "imported helper-day",
		"svEnergyCounter_31337_000F:6":                               "imported counter,counter-plain",
		"svEnergyCounterOldVal_31337":                                "imported counter,counter-plain",
		"svEnergyCounter_" + n(key) + "_000D1B2C3D4E5F:1":            "imported counter,counter-plain",
		"svHmIPRainCounterToday_" + n(key):                           "imported counter,counter-plain",
		"svHmIPRainCounterTomorrow_" + n(swo):                        "imported counter,counter-plain",
		"svCounter_" + n(wsm) + "_Text":                              "imported counter,counter-address,counter-plain",
	} {
		r := variableReport(res, name)
		if r == nil {
			t.Errorf("%s is not in the report", name)
			continue
		}
		if got := string(r.Status) + " " + codes(r.Notes); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	// the variables a program uses are variables, and no helper's
	if v := variable(res, model.Slug("svHmIPSunshineCounterOldVal_"+n(swo))); v == nil || v.Helper != nil {
		t.Errorf("the old reading a program reads: %+v", v)
	}
	// the counter programs are not imported, the classic rain sensor's among them
	for _, name := range []string{"Regentagesmenge aktualisieren", "prgFlowCounter_" + n(wsm) + "_000C1B2C3D4E5F:2"} {
		if r := programReport(res, name); r.Status != Skipped {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if !IsCounterProgram("Regentagesmenge zurücksetzen") || IsCounterProgram("Regentagesmenge zurücksetzen 2") || IsCounterProgram("Regen") {
		t.Error("the classic rain programs by name")
	}
	// the report's words
	md := res.Report.Markdown("de")
	for _, part := range []string{"ein Gerätehelfer zählt sie weiter (energy, " + cPSM + ")", "im Gerätehelfer aufgehoben", "nicht mehr nötig", "Tageswert des Gerätehelfers svEnergyCounter_" + n(psm)} {
		if !strings.Contains(md, part) {
			t.Errorf("the report lacks %q", part)
		}
	}

	// a second import keeps the keys, also those of the day variables the import made
	ledger, err := DecodeLedger(res.Ledger.Encode())
	if err != nil {
		t.Fatal(err)
	}
	set := model.NewSet()
	for _, v := range res.Variables {
		set.Variables[v.Key] = v
	}
	again := b.run(t, Options{Previous: ledger, Taken: InStore(set)})
	if len(again.Variables) != len(res.Variables) {
		t.Fatalf("%d variables, then %d", len(res.Variables), len(again.Variables))
	}
	for i, v := range again.Variables {
		if v.Key != res.Variables[i].Key {
			t.Errorf("%s became %s", res.Variables[i].Key, v.Key)
		}
	}
	if r := variableReport(again, "svEnergyCounterToday_"+n(psm)); !r.Updated {
		t.Errorf("the day variable is not an update: %+v", r)
	}
	// the set the import gives holds together: every helper's variable has its place
	for _, v := range set.Variables {
		if p := set.HelperProblem(v); p != "" {
			t.Errorf("%s: %s", v.Key, p)
		}
	}
	if len(set.Helpers()) != len(res.Helpers) {
		t.Errorf("%d helpers in the set, %d states", len(set.Helpers()), len(res.Helpers))
	}
}
