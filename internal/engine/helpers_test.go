package engine

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const (
	psm     = "HmIP-RF/000A1B2C3D4E5F:6"
	swo     = "HmIP-RF/000B1B2C3D4E5F:1"
	energy  = psm + "/ENERGY_COUNTER"
	eflag   = psm + "/ENERGY_COUNTER_OVERFLOW"
	rain    = swo + "/RAIN_COUNTER"
	rflag   = swo + "/RAIN_COUNTER_OVERFLOW"
	sun     = swo + "/SUNSHINEDURATION"
	maxPSM  = 838859.1
	maxRain = 1638.0
)

func helperVar(key, kind, channel, role string) *model.Variable {
	return &model.Variable{Key: key, Name: key, Type: model.VarNumber, Visible: true, Helper: &model.Helper{Kind: kind, Channel: channel, Role: role}}
}

// hrig is an engine in Europe/Berlin with an energy helper (total, today, yesterday), a rain
// helper (the same three) and a sunshine helper (the total only), all imported: they have seen
// their device.
type hrig struct {
	*trig
	defs map[string]*model.Variable
}

func newHrig(t *testing.T, start string) *hrig {
	r := &hrig{trig: newTrig(t, start), defs: map[string]*model.Variable{}}
	for _, in := range r.e.Variables() {
		r.defs[in.Def.Key] = in.Def
	}
	for _, v := range []*model.Variable{
		helperVar("e", model.HelperEnergy, psm, model.RoleTotal), helperVar("e-today", model.HelperEnergy, psm, model.RoleToday), helperVar("e-yesterday", model.HelperEnergy, psm, model.RoleYesterday),
		helperVar("r", model.HelperRain, swo, model.RoleTotal), helperVar("r-today", model.HelperRain, swo, model.RoleToday), helperVar("r-yesterday", model.HelperRain, swo, model.RoleYesterday),
		helperVar("s", model.HelperSunshine, swo, model.RoleTotal),
	} {
		r.defs[v.Key] = v
	}
	r.e.SetVariables(r.defs)
	for _, k := range []string{"e", "r", "s"} {
		if !r.e.SetHelperState(k, model.HelperState{Seen: true}) {
			t.Fatalf("no helper %s", k)
		}
	}
	r.e.SetCounterMaximum(mustRef(t, energy), maxPSM)
	r.e.SetCounterMaximum(mustRef(t, rain), maxRain)
	return r
}

func mustRef(t *testing.T, s string) model.Ref {
	t.Helper()
	ref, err := model.ParseRef(s)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func (r *hrig) want(what string, pairs ...any) {
	r.t.Helper()
	for i := 0; i+1 < len(pairs); i += 2 {
		if got := r.value(pairs[i].(string)); got != pairs[i+1] {
			r.t.Errorf("%s: %s is %v, want %v", what, pairs[i], got, pairs[i+1])
		}
	}
}

func (r *hrig) helper(kind string) HelperInfo {
	r.t.Helper()
	for _, h := range r.e.Helpers() {
		if h.Kind == kind {
			return h
		}
	}
	r.t.Fatalf("no helper %s", kind)
	return HelperInfo{}
}

func (r *hrig) logged(part string) bool {
	for _, l := range r.logs {
		if strings.Contains(l, part) {
			return true
		}
	}
	return false
}

// A helper counts its device's reports into the total and today's amount; its datapoints are
// among those the daemon follows, its counters among those it asks the maximum of.
func TestHelperCounts(t *testing.T) {
	r := newHrig(t, "2026-11-22 12:00:00 CET")
	refs := []string{}
	for _, ref := range r.e.Refs() {
		refs = append(refs, ref.String())
	}
	if !reflect.DeepEqual(refs, []string{energy, eflag, rain, rflag, sun, swo + "/SUNSHINEDURATION_OVERFLOW"}) {
		t.Errorf("refs: %v", refs)
	}
	counters := []string{}
	for _, ref := range r.e.HelperCounters() {
		counters = append(counters, ref.String())
	}
	if !reflect.DeepEqual(counters, []string{energy, rain, sun}) {
		t.Errorf("counters: %v", counters)
	}
	// imported with the CCU's old reading of 0: the first report adds the whole counter
	r.dev(energy, 120.5)
	r.want("first", "e", 120.5, "e-today", 120.5, "e-yesterday", 0.0)
	r.dev(energy, 121.7)
	r.want("second", "e", 121.7, "e-today", 121.7)
	// tenths that do not add up in binary stay tenths
	r.dev(energy, 121.8)
	r.dev(energy, 122.1)
	r.want("fractions", "e", 122.1, "e-today", 122.1)
	// a reading of zero is ignored, a text is nothing to count
	r.dev(energy, 0)
	r.dev(energy, "x")
	r.want("ignored", "e", 122.1)
	// other counters are other helpers
	r.dev(rain, 0.3)
	r.dev(sun, 12)
	r.want("the others", "r", 0.3, "r-today", 0.3, "s", 12.0, "e", 122.1)
	// what the helper remembers
	h := r.helper(model.HelperEnergy)
	if h.State.Old != 122.1 || !h.State.Seen || h.Maximum == nil || *h.Maximum != maxPSM || h.Channel != psm || !reflect.DeepEqual(h.Variables, map[string]string{"total": "e", "today": "e-today", "yesterday": "e-yesterday"}) {
		t.Errorf("helper: %+v", h)
	}
	if h := r.helper(model.HelperSunshine); h.Maximum != nil {
		t.Errorf("a maximum nobody told: %+v", h)
	}
	// every counted report is an update of the variable, as the CCU's script wrote it each time
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: e\n              op: ge\n              value: 0\n              trigger: update\n    then:\n      - variable: out\n        to: counted\n")
	r.dev(energy, 122.1)
	r.expect("p", "1")
}

// Smaller readings: a restart without the overflow flag, a wrap with it - also when the flag
// arrives a moment after the reading, as the interface sends them.
func TestHelperRestartAndOverflow(t *testing.T) {
	r := newHrig(t, "2026-11-22 12:00:00 CET")
	r.dev(energy, 838850.0)
	r.dev(energy, 3.5) // no flag: the device restarted
	r.want("restart", "e", 838853.5, "e-today", 838853.5)
	if !r.logged("the device restarted") {
		t.Errorf("logs: %v", r.logs)
	}
	// the flag first, then the reading
	r.dev(energy, 838855.0)
	r.dev(eflag, true)
	r.dev(energy, 2.0)
	r.want("wrap", "e", 838853.5+838851.5+4.1+2.0)
	if !r.logged("began anew at 2 after its maximum") {
		t.Errorf("logs: %v", r.logs)
	}
	// the reading first, the flag right after it: counted as a restart, then put right
	r.dev(rflag, false)
	r.dev(rain, 1637.4)
	r.dev(rain, 0.3)
	r.want("before the flag", "r", 1637.7, "r-today", 1637.7)
	r.clock.Advance(time.Second)
	r.dev(rflag, true)
	r.want("with the flag", "r", 1638.3, "r-today", 1638.3)
	// a flag that changes long after a reading changes nothing
	r.dev(rain, 0.5)
	r.clock.Advance(time.Minute)
	r.dev(rflag, false)
	r.want("a late flag", "r", 1638.5)
	// a flag that repeats itself changes nothing either
	r.dev(rain, 0.6)
	r.dev(rflag, false)
	r.want("the same flag", "r", 1638.6)
	// the maximum not known: only the new reading
	r.dev(sun, 16380)
	r.dev(swo+"/SUNSHINEDURATION_OVERFLOW", 1) // some interfaces say 1 for true
	r.dev(sun, 4)
	r.want("no maximum", "s", 16384.0)
	if !r.logged("its maximum is not known") {
		t.Errorf("logs: %v", r.logs)
	}
}

// A HomeMatic measuring actuator: the first report sets the total, BOOT after the reading
// turns a wrap into a restart; an HM-ES-TX-WM's BOOT is not corrected when the total was set.
func TestHelperHomeMatic(t *testing.T) {
	r := newHrig(t, "2026-11-22 12:00:00 CET")
	const pm, wm = "BidCos-RF/NEQ0000002:2", "BidCos-RF/NEQ0000003:1"
	r.defs["hm"] = helperVar("hm", model.HelperEnergyHM, pm, model.RoleTotal)
	r.defs["gas"] = helperVar("gas", model.HelperGasWM, wm, model.RoleTotal)
	r.defs["rhm"] = helperVar("rhm", model.HelperRainHM, "BidCos-RF/OEQ0000004:1", model.RoleToday)
	r.e.SetVariables(r.defs)
	r.e.SetHelperState("hm", model.HelperState{Seen: true})
	r.e.SetHelperState("gas", model.HelperState{Seen: true, Old: 0})
	r.e.SetCounterMaximum(mustRef(t, pm+"/ENERGY_COUNTER"), 838860.7)
	r.set("hm", 77.0, FromUser)
	r.dev(pm+"/ENERGY_COUNTER", 420.5)
	r.want("the first report sets the total", "hm", 420.5)
	r.dev(pm+"/BOOT", false)
	r.dev(pm+"/ENERGY_COUNTER", 1.5) // counted as a wrap ...
	r.want("a wrap", "hm", 420.5+838860.7-420.5+1.5)
	r.dev(pm+"/BOOT", true) // ... until BOOT says it was a restart
	r.want("a restart", "hm", 422.0)
	// the HM-ES-TX-WM: BOOT before anything was counted sets the total to zero
	r.set("gas", 5.0, FromUser)
	r.dev(wm+"/BOOT", true)
	r.dev(wm+"/GAS_ENERGY_COUNTER", 0.25)
	r.want("BOOT", "gas", 0.0)
	r.dev(wm+"/BOOT", false) // the step set the total: nothing to correct
	r.want("BOOT gone", "gas", 0.0)
	r.dev(wm+"/GAS_ENERGY_COUNTER", 0.5)
	r.want("after the restart", "gas", 0.5)
	// the HomeMatic rain sensor has today's amount only, and made here it notes its first reading
	r.dev("BidCos-RF/OEQ0000004:1/RAIN_COUNTER", 12.0)
	r.dev("BidCos-RF/OEQ0000004:1/RAIN_COUNTER", 12.295)
	r.want("rain", "rhm", 0.295)
}

// Today's and yesterday's amounts are the helper's alone; the total takes a meter reading, and
// counting goes on from it. A helper that is off adds nothing and counts on from where the
// device stands when it is switched on again.
func TestHelperWritesAndOff(t *testing.T) {
	r := newHrig(t, "2026-11-22 12:00:00 CET")
	r.dev(energy, 100.0)
	for _, src := range []Source{FromUser, FromRPC, FromProgram} {
		r.refused("e-today", 5.0, src, "read-only")
		r.refused("e-yesterday", 5.0, src, "read-only")
	}
	r.set("e-today", 7.0, FromImport) // an import brings values along
	r.set("e", 20000.0, FromUser)
	r.dev(energy, 101.5)
	r.want("meter reading", "e", 20001.5, "e-today", 8.5)
	// off
	off := r.defs["e"].Clone()
	off.Helper.Off = true
	r.defs["e"] = off
	r.e.SetVariables(r.defs)
	if !r.helper(model.HelperEnergy).Off {
		t.Error("not off")
	}
	r.dev(energy, 150.0)
	r.dev(eflag, true)
	r.want("off", "e", 20001.5, "e-today", 8.5)
	on := off.Clone()
	on.Helper.Off = false
	r.defs["e"] = on
	r.e.SetVariables(r.defs)
	r.dev(energy, 151.0)
	r.want("on again", "e", 20002.5, "e-today", 9.5)
	// a variable that leads no helper has no state to set
	if r.e.SetHelperState("e-today", model.HelperState{}) || r.e.SetHelperState("x", model.HelperState{}) || r.e.SetHelperState("gone", model.HelperState{}) {
		t.Error("state for a variable that leads no helper")
	}
}

// A helper made here has not seen its device: the system's stored value, or the first report,
// only says where the counter stands. One that has seen it is not moved by a stored value.
func TestHelperStartsFromNow(t *testing.T) {
	r := newHrig(t, "2026-11-22 12:00:00 CET")
	r.e.SetHelperState("e", model.HelperState{})
	r.e.SetHelperState("r", model.HelperState{})
	r.e.SeedDevice(mustRef(t, energy), 5000.0)
	r.e.SeedDevice(mustRef(t, eflag), false)
	r.e.SeedDevice(mustRef(t, energy), "x")
	r.dev(energy, 5002.5)
	r.want("seeded", "e", 2.5)
	r.dev(rain, 44.0)
	r.dev(rain, 44.3)
	r.want("first report", "r", 0.3)
	r.e.SeedDevice(mustRef(t, energy), 6000.0)
	r.dev(energy, 5003.0)
	r.want("a stored value later", "e", 3.0)
}

// The day's boundary: 07:00 for rain, midnight for energy - today's amount becomes
// yesterday's. The state survives a restart, and a boundary that passed while the addon was
// stopped is made up at the start.
func TestHelperDayRoll(t *testing.T) {
	r := newHrig(t, "2026-11-22 12:00:00 CET")
	r.clock.Advance(time.Minute) // the first look at the clock: the boundary behind is where the helpers stand
	r.dev(energy, 10.0)
	r.dev(rain, 2.5)
	r.clock.Advance(11*time.Hour + 58*time.Minute) // 23:59
	r.want("before midnight", "e-today", 10.0, "e-yesterday", 0.0)
	r.clock.Advance(time.Minute)
	r.want("midnight", "e-today", 0.0, "e-yesterday", 10.0, "e", 10.0, "r-today", 2.5)
	r.dev(energy, 11.0)
	r.clock.Advance(7 * time.Hour)
	r.want("07:00", "r-today", 0.0, "r-yesterday", 2.5, "r", 2.5, "e-today", 1.0)
	if got := r.helper(model.HelperRain).State.Rolled; !got.Equal(berlinTime(t, "2026-11-23 07:00:00 CET")) {
		t.Errorf("rolled: %s", got)
	}
	// the roll is a write like any other: a program on yesterday's amount runs
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: e-yesterday\n              op: ge\n              value: 0\n              trigger: update\n    then:\n      - variable: out\n        to: rolled\n")
	r.dev(rain, 4.0)
	r.clock.Advance(17 * time.Hour)
	r.expect("p", "1")
	r.want("the second midnight", "e-yesterday", 1.0, "e-today", 0.0)

	// stopped for three days: the state is written, the addon starts again on Thursday noon
	state, _ := r.e.Export()
	raw, err := MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalState(raw)
	if err != nil {
		t.Fatal(err)
	}
	n := &hrig{trig: newTrig(t, "2026-11-26 12:00:00 CET"), defs: r.defs}
	n.e.SetVariables(n.defs)
	n.e.Import(back)
	n.want("restored", "r-today", 1.5, "r-yesterday", 2.5, "e", 11.0)
	n.dev(rain, 4.5) // counted before the clock was looked at: still the old day
	n.clock.Advance(time.Minute)
	n.want("made up", "r-today", 0.0, "r-yesterday", 2.0, "r", 4.5, "e-today", 0.0, "e-yesterday", 0.0)
	if !n.logged("3 day boundaries passed") {
		t.Errorf("logs: %v", n.logs)
	}
	// a boundary noted on a wrong clock, ahead of the right one, is forgotten
	n.e.SetHelperState("r", model.HelperState{Seen: true, Old: 4.5, Rolled: berlinTime(t, "2030-01-01 07:00:00 CET")})
	n.dev(rain, 5.0)
	n.clock.Advance(time.Minute)
	n.want("wrong clock", "r-today", 0.5)
	if got := n.helper(model.HelperRain).State.Rolled; !got.Equal(berlinTime(t, "2026-11-26 07:00:00 CET")) {
		t.Errorf("rolled: %s", got)
	}
}

// Without a clock that is set nothing rolls: a system without a battery starts in the past.
func TestHelperWaitsForTheClock(t *testing.T) {
	ready := false
	tr := newTrig(t, "2026-11-22 23:50:00 CET", func(o *Options) { o.ClockReady, o.ClockWait = func() bool { return ready }, time.Hour })
	r := &hrig{trig: tr, defs: map[string]*model.Variable{}}
	for _, v := range []*model.Variable{helperVar("e", model.HelperEnergy, psm, model.RoleTotal), helperVar("e-today", model.HelperEnergy, psm, model.RoleToday), helperVar("e-yesterday", model.HelperEnergy, psm, model.RoleYesterday)} {
		r.defs[v.Key] = v
	}
	r.e.SetVariables(r.defs)
	r.e.SetHelperState("e", model.HelperState{Seen: true, Rolled: berlinTime(t, "2026-11-22 00:00:00 CET")})
	r.dev(energy, 9.0)
	r.clock.Advance(20 * time.Minute)
	r.want("no clock", "e-today", 9.0, "e-yesterday", 0.0)
	ready = true
	r.clock.Advance(time.Minute)
	r.want("the clock is set", "e-today", 0.0, "e-yesterday", 9.0)
	// a helper without a day's amount needs no timer
	delete(r.defs, "e-today")
	delete(r.defs, "e-yesterday")
	r.e.SetVariables(r.defs)
	if n := r.clock.Pending(); n != 0 {
		t.Errorf("%d timers without anything to wait for", n)
	}
	// a helper whose variable went away leaves nothing behind
	r.dev(energy, 10.0)
	delete(r.defs, "e")
	r.e.SetVariables(r.defs)
	r.dev(energy, 11.0)
	if len(r.e.Helpers()) != 0 || len(r.e.HelperCounters()) != 0 {
		t.Error("a helper without variables")
	}
}
