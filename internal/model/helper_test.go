package model

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// The maxima of the counters, from the devices' descriptions (HmIP-PSM, HmIP-SWO-PR,
// HM-ES-PMSw1-Pl, HM-ES-TX-WM, HM-WDS100-C6-O: node-red-contrib-ccu's paramsets.json and
// pydevccu).
const (
	maxPSM      = 838859.1
	maxRain     = 1638.0
	maxSunshine = 16383.0
	maxHM       = 838860.7
	maxGasWM    = 2147483.647
	maxRainHM   = 9666.265
)

type report struct {
	dev   float64
	flag  bool
	delta float64
	note  string
	// set: the total is set to this instead (-1: it is not)
	set float64
	old float64
}

func rp(dev float64, flag bool, delta float64, note string, old float64) report {
	return report{dev: dev, flag: flag, delta: delta, note: note, set: -1, old: old}
}

// Each family's arithmetic, row by row of the concept's table: a recorded sequence of the
// device's readings, and what each adds.
func TestHelperStep(t *testing.T) {
	seen := HelperState{Seen: true}
	cases := []struct {
		name  string
		kind  string
		max   float64
		known bool
		start HelperState
		seq   []report
	}{
		{"HmIP: the first report of an imported helper adds the whole counter, then the differences", HelperEnergy, maxPSM, true, seen, []report{
			rp(120.5, false, 120.5, "", 120.5), rp(121.7, false, 1.2, "", 121.7), rp(121.7, false, 0, "", 121.7),
		}},
		{"HmIP: a helper made here only notes its first reading", HelperEnergy, maxPSM, true, HelperState{}, []report{
			rp(5000, false, 0, "first", 5000), rp(5002.5, false, 2.5, "", 5002.5),
		}},
		{"HmIP: smaller without the overflow flag is a restart - the new reading counts whole", HelperEnergy, maxPSM, true, HelperState{Seen: true, Old: 700}, []report{
			rp(3.2, false, 3.2, "restart", 3.2), rp(4, false, 0.8, "", 4),
		}},
		{"HmIP: smaller with the overflow flag is a wrap - up to the maximum and on", HelperEnergy, maxPSM, true, HelperState{Seen: true, Old: 838850}, []report{
			rp(4.4, true, 13.5, "overflow", 4.4), rp(6, true, 1.6, "", 6),
			// the flag stays set until the device restarts: after that a smaller reading is a restart again
			rp(1, false, 1, "restart", 1),
		}},
		{"HmIP: a wrap while the maximum is not known counts the new reading only", HelperEnergy, 0, false, HelperState{Seen: true, Old: 838850}, []report{
			rp(4.4, true, 4.4, "overflow-unknown", 4.4),
		}},
		{"HmIP: a reading of zero is ignored, the old reading kept", HelperEnergy, maxPSM, true, HelperState{Seen: true, Old: 50}, []report{
			rp(0, false, 0, "ignored", 50), rp(0.0004, true, 0, "ignored", 50), rp(51, false, 1, "", 51),
		}},
		{"feed-in counts like energy", HelperFeedIn, maxPSM, true, HelperState{Seen: true, Old: 10}, []report{rp(12, false, 2, "", 12)}},
		{"rain: the wrap of the rain gauge", HelperRain, maxRain, true, HelperState{Seen: true, Old: 1637.4}, []report{
			rp(0.3, true, 0.9, "overflow", 0.3),
		}},
		{"sunshine: minutes, the wrap at 16383", HelperSunshine, maxSunshine, true, HelperState{Seen: true, Old: 16380}, []report{
			rp(16383, false, 3, "", 16383), rp(2, true, 2, "overflow", 2),
		}},
		{"water counts like energy", HelperWater, maxHM, true, HelperState{Seen: true, Old: 100}, []report{rp(100.25, false, 0.25, "", 100.25)}},
		{"ESI: a smaller reading adds nothing (the CCU's overflow line is commented out)", HelperESI, 109951162775.9, true, HelperState{Seen: true, Old: 500}, []report{
			rp(20, false, 0, "restart", 20), rp(25, false, 5, "", 25), rp(0, false, 0, "ignored", 25),
		}},
		{"ESI: the first report adds the whole counter", HelperESI, 0, false, seen, []report{rp(1234.5, false, 1234.5, "", 1234.5)}},
		{"HomeMatic: the first report sets the total to the device's counter", HelperEnergyHM, maxHM, true, seen, []report{
			{dev: 420.5, set: 420.5, note: "first", old: 420.5}, rp(421, false, 0.5, "", 421),
		}},
		{"HomeMatic: smaller with BOOT is a restart, smaller without is a wrap, zero keeps the old reading", HelperEnergyHM, maxHM, true, HelperState{Seen: true, Old: 838860}, []report{
			rp(1.5, false, 2.2, "overflow", 1.5), rp(0.2, true, 0.2, "restart", 0.2), rp(0, false, 0, "ignored", 0.2), rp(0.7, true, 0.5, "", 0.7),
		}},
		{"HomeMatic: a wrap without the maximum", HelperEnergyHM, 0, false, HelperState{Seen: true, Old: 838860}, []report{rp(1.5, false, 1.5, "overflow-unknown", 1.5)}},
		{"HM-ES-TX-WM: the first report adds the whole counter; a wrap; zero moves the old reading", HelperEnergyWM, maxHM, true, seen, []report{
			rp(10, false, 10, "", 10), rp(12, false, 2, "", 12), rp(0, false, 0, "ignored", 0), rp(3, false, 3, "", 3),
			rp(1, false, 838858.7, "overflow", 1),
		}},
		{"HM-ES-TX-WM: a report with BOOT counts nothing and makes the next reading count whole", HelperGasWM, maxGasWM, true, HelperState{Seen: true, Old: 77}, []report{
			rp(0, true, 0, "restart", 77), rp(0, false, 0, "restart", 0), rp(0.5, false, 0.5, "restart", 0.5), rp(0.75, false, 0.25, "", 0.75),
		}},
		{"HM-ES-TX-WM: a restart before anything was counted sets the total to zero", HelperEnergyWM, maxHM, true, seen, []report{
			{dev: 3, flag: true, set: 0, note: "restart", old: 0}, rp(4, false, 4, "restart", 4),
		}},
		{"HM-ES-TX-WM, IEC: every smaller reading is a wrap", HelperIECWM, maxHM, true, HelperState{Seen: true, Old: 838860}, []report{
			rp(2, false, 2.7, "overflow", 2), rp(0, false, 0, "ignored", 2), rp(2.5, false, 0.5, "", 2.5),
		}},
		{"HomeMatic rain: the difference to the reading before, a wrap adds the maximum", HelperRainHM, maxRainHM, true, HelperState{}, []report{
			rp(9665.97, false, 0, "first", 9665.97), rp(9666.265, false, 0.295, "", 9666.265), rp(0.295, false, 0.295, "overflow", 0.295),
		}},
		{"HomeMatic rain: a wrap without the maximum", HelperRainHM, 0, false, HelperState{Seen: true, Old: 9000}, []report{rp(1, false, 1, "overflow-unknown", 1)}},
	}
	for _, c := range cases {
		k := HelperKindOf(c.kind)
		st := c.start
		for i, r := range c.seq {
			step, next := k.Step(st, r.dev, r.flag, c.max, c.known)
			want := HelperStep{Delta: r.delta, Note: r.note}
			if r.set >= 0 {
				want = HelperStep{Set: true, Total: r.set, Note: r.note}
			}
			if step != want || next.Old != r.old || !next.Seen {
				t.Errorf("%s, report %d (%v): %+v, old %v; want %+v, old %v", c.name, i+1, r.dev, step, next.Old, want, r.old)
			}
			st = next
		}
	}
	// the latch of the HM-ES-TX-WM, by itself
	k := HelperKindOf(HelperGasWM)
	_, st := k.Step(HelperState{Seen: true, Old: 5}, 0, true, maxGasWM, true)
	if !st.Latch {
		t.Error("BOOT does not latch")
	}
	if _, st = k.Step(st, 0, false, maxGasWM, true); !st.Latch {
		t.Error("a reading of zero releases the latch")
	}
	if _, st = k.Step(st, 2, false, maxGasWM, true); st.Latch {
		t.Error("a reading releases the latch")
	}
	if _, st = k.Step(HelperState{Latch: true}, 2, false, maxGasWM, true); st.Latch || !st.Seen {
		t.Errorf("a helper's first reading: %+v", st)
	}
}

func TestHelperKinds(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range HelperKinds {
		if seen[k.Kind] || HelperKindOf(k.Kind) != k || len(k.Counters) == 0 || len(k.ChannelTypes) == 0 || !k.HasRole(k.Lead()) || k.HasRole("sum") {
			t.Errorf("%+v", k)
		}
		seen[k.Kind] = true
	}
	if HelperKindOf("heat") != nil {
		t.Error("a kind nobody has")
	}
	fits := func(device, channel string) string {
		var out []string
		for _, k := range HelperKinds {
			if k.Fits(device, channel) {
				out = append(out, k.Kind)
			}
		}
		return strings.Join(out, " ")
	}
	for in, want := range map[string]string{
		"HMIP-PSM ENERGIE_METER_TRANSMITTER":     "energy",
		"HmIP-PSMCO ENERGIE_METER_TRANSMITTER":   "energy feed-in",
		"ELV-SH-PSMCI ENERGIE_METER_TRANSMITTER": "energy feed-in",
		"HmIP-ESI ENERGIE_METER_TRANSMITTER":     "esi",
		"HM-ES-PMSw1-Pl POWERMETER":              "energy-hm",
		"HM-ES-TX-WM POWERMETER_IGL":             "energy-wm gas-wm",
		"HM-ES-TX-WM POWERMETER_IEC1":            "energy-wm gas-wm iec-wm",
		"HmIP-SWO-PR WEATHER_TRANSMIT":           "rain sunshine",
		"ELV-SH-WSM FLOW_METER_TRANSMITTER":      "water",
		"HM-WDS100-C6-O WEATHER":                 "rain-hm",
		"HmIP-PS SWITCH_TRANSMITTER":             "",
		"HM-ES-TX-WM POWERMETER_IEC2":            "",
		"HmIP-ESI MAINTENANCE":                   "",
		"ESI ENERGIE_METER_TRANSMITTER":          "energy",
		"hmip-esi-ind ENERGIE_METER_TRANSMITTER": "esi",
	} {
		device, channel, _ := strings.Cut(in, " ")
		if got := fits(device, channel); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
	h := &Helper{Kind: HelperEnergy, Channel: "HmIP-RF/000A:6", Role: RoleTotal}
	if refs := h.Refs(); len(refs) != 2 || refs[0].String() != "HmIP-RF/000A:6/ENERGY_COUNTER" || refs[1].Param != "ENERGY_COUNTER_OVERFLOW" {
		t.Errorf("%v", refs)
	}
	if refs := (&Helper{Kind: HelperESI, Channel: "HmIP-RF/000A:2"}).Refs(); len(refs) != 2 || refs[1].Param != "GAS_VOLUME" {
		t.Errorf("%v", refs)
	}
	if (&Helper{Kind: "x", Channel: "HmIP-RF/000A:2"}).Refs() != nil || (&Helper{Kind: HelperESI, Channel: "x"}).Refs() != nil {
		t.Error("a helper that names nothing")
	}
}

// The day's boundary: 07:00 for rain and sunshine, 07:30 for the HomeMatic rain sensor,
// midnight for the others - on the system's clock, through both clock changes.
func TestHelperBoundary(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, berlin)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range []struct{ kind, now, last, next string }{
		{HelperRain, "2026-11-22 12:00", "2026-11-22 07:00", "2026-11-23 07:00"},
		{HelperRain, "2026-11-22 06:59", "2026-11-21 07:00", "2026-11-22 07:00"},
		{HelperRain, "2026-11-22 07:00", "2026-11-22 07:00", "2026-11-23 07:00"},
		{HelperRainHM, "2026-11-22 07:15", "2026-11-21 07:30", "2026-11-22 07:30"},
		{HelperEnergy, "2026-11-22 00:00", "2026-11-22 00:00", "2026-11-23 00:00"},
		{HelperEnergy, "2026-12-31 23:59", "2026-12-31 00:00", "2027-01-01 00:00"},
		// the night the clock goes forward (23 hours) and the one it goes back (25)
		{HelperSunshine, "2026-03-29 06:00", "2026-03-28 07:00", "2026-03-29 07:00"},
		{HelperWater, "2026-03-29 03:30", "2026-03-29 00:00", "2026-03-30 00:00"},
		{HelperSunshine, "2026-10-25 08:00", "2026-10-25 07:00", "2026-10-26 07:00"},
		// the leap day
		{HelperEnergy, "2028-02-29 10:00", "2028-02-29 00:00", "2028-03-01 00:00"},
		{HelperRain, "2028-03-01 05:00", "2028-02-29 07:00", "2028-03-01 07:00"},
	} {
		k := HelperKindOf(c.kind)
		if last, next := k.Boundary(at(c.now)), k.NextBoundary(at(c.now)); !last.Equal(at(c.last)) || !next.Equal(at(c.next)) {
			t.Errorf("%s at %s: %s and %s", c.kind, c.now, last, next)
		}
	}
	if d := HelperKindOf(HelperRain).NextBoundary(at("2026-03-28 07:00")).Sub(at("2026-03-28 07:00")); d != 23*time.Hour {
		t.Errorf("the short day: %v", d)
	}
}

func helperVar(key, kind, channel, role string) *Variable {
	return &Variable{Key: key, Name: key, Type: VarNumber, Visible: true, Helper: &Helper{Kind: kind, Channel: channel, Role: role}}
}

// A helper's variable in its file, and what a file may not say.
func TestHelperFile(t *testing.T) {
	v := helperVar("zaehler", HelperEnergy, "HmIP-RF/000A1B2C3D4E5F:6", RoleTotal)
	v.Name, v.Unit, v.Helper.Off = "Energiezähler Waschmaschine", "Wh", true
	text := string(EncodeVariable(v))
	want := "name: Energiezähler Waschmaschine\ntype: number\nunit: Wh\nhelper:\n  kind: energy\n  channel: HmIP-RF/000A1B2C3D4E5F:6\n  role: total\n  off: true\n"
	if text != want {
		t.Errorf("written:\n%s", text)
	}
	back, err := DecodeVariable("zaehler.yaml", "zaehler", []byte(text))
	if err != nil || !reflect.DeepEqual(back, v) {
		t.Errorf("read back: %+v, %v", back, err)
	}
	if c := v.Clone(); c.Helper == v.Helper || !reflect.DeepEqual(c, v) {
		t.Error("the copy shares the helper")
	}
	head := "name: Z\ntype: number\n"
	for text, want := range map[string]string{
		head + "helper: energy\n":                                                                                                                                 "helper",
		head + "helper:\n  channel: HmIP-RF/000A:6\n  role: total\n":                                                                                              `a helper needs a "kind"`,
		head + "helper:\n  kind: energy\n  role: total\n":                                                                                                         `a helper needs a "channel"`,
		head + "helper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n":                                                                                             `a helper needs a "role"`,
		head + "helper:\n  kind: heat\n  channel: HmIP-RF/000A:6\n  role: total\n":                                                                                "is not one of energy, feed-in",
		head + "helper:\n  kind:\n    a: b\n  channel: HmIP-RF/000A:6\n  role: total\n":                                                                           "kind",
		head + "helper:\n  kind: energy\n  channel: 000A:6\n  role: total\n":                                                                                      "is not a channel",
		head + "helper:\n  kind: energy\n  channel:\n    a: b\n  role: total\n":                                                                                   "channel",
		head + "helper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: sum\n":                                                                                "is not one of total, today, yesterday",
		head + "helper:\n  kind: rain-hm\n  channel: BidCos-RF/OEQ1:1\n  role: total\n":                                                                           "is not one of today, yesterday",
		head + "helper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: total\n  off: 1\n":                                                                    "off",
		head + "helper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: today\n  off: true\n":                                                                 `"off" is said on the helper's total`,
		head + "helper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: total\n  max: 5\n":                                                                    "max",
		head + "start_value: 5\nhelper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: total\n":                                                              `has no "start_value"`,
		"name: Z\ntype: string\nhelper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: total\n":                                                              "only a plain number",
		head + "computed:\n  function: sum\n  inputs:\n    - variable: a\n    - variable: b\nhelper:\n  kind: energy\n  channel: HmIP-RF/000A:6\n  role: total\n": "only a plain number",
	} {
		if _, err := DecodeVariable("z.yaml", "z", []byte(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", text, err, want)
		}
	}
}

// The variables with the same kind and channel are one helper; what keeps one out of it.
func TestHelperGroups(t *testing.T) {
	s := NewSet()
	add := func(v *Variable) { s.Variables[v.Key] = v }
	const psm, swo = "HmIP-RF/000A:6", "HmIP-RF/000B:1"
	add(helperVar("e", HelperEnergy, psm, RoleTotal))
	add(helperVar("e-today", HelperEnergy, psm, RoleToday))
	add(helperVar("e-twice", HelperEnergy, psm, RoleTotal))
	add(helperVar("rain-today", HelperRain, swo, RoleToday)) // no total
	add(helperVar("sun", HelperSunshine, swo, RoleTotal))
	add(helperVar("sun-yesterday", HelperSunshine, swo, RoleYesterday))
	add(helperVar("hm-today", HelperRainHM, "BidCos-RF/OEQ1:1", RoleToday))
	add(helperVar("f", HelperEnergy, "HmIP-RF/000C:6", RoleTotal))
	add(&Variable{Key: "plain", Name: "Plain", Type: VarNumber})
	s.Variables["sun"].Helper.Off = true
	problems := map[string]string{}
	for k, v := range s.Variables {
		if p := s.HelperProblem(v); p != "" {
			problems[k] = p
		}
	}
	if !reflect.DeepEqual(problems, map[string]string{"e-twice": "e already holds this helper's total", "rain-today": "the helper has no variable for its total"}) {
		t.Errorf("problems: %v", problems)
	}
	var got []string
	for _, g := range s.Helpers() {
		roles := []string{}
		for _, r := range HelperRoles {
			if g.Vars[r] != "" {
				roles = append(roles, r+"="+g.Vars[r])
			}
		}
		line := g.Kind.Kind + "@" + g.Channel + " " + strings.Join(roles, ",") + " lead=" + g.Lead()
		if g.Off {
			line += " off"
		}
		got = append(got, line)
	}
	want := []string{
		"energy@" + psm + " total=e,today=e-today lead=e",
		"energy@HmIP-RF/000C:6 total=f lead=f",
		"rain-hm@BidCos-RF/OEQ1:1 today=hm-today lead=hm-today",
		"sunshine@" + swo + " total=sun,yesterday=sun-yesterday lead=sun off",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("helpers:\n%s", strings.Join(got, "\n"))
	}
	for key, want := range map[string][]string{"e": {"e-today", "e-twice"}, "e-today": {}, "plain": {}, "gone": {}, "hm-today": {}, "f": {}} {
		if m := s.HelperMembers(key); !reflect.DeepEqual(m, want) {
			t.Errorf("members of %s: %v", key, m)
		}
	}
}
