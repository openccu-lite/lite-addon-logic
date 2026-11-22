package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

// What the CCU does not have: "hält seit" (a line's "for"), the random delay, "ein für".

// held: one line on x above 10 that must have held for ten minutes, with a mode.
func held(trigger string) string {
	return "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n" + gt10 + "              for: 10min\n              trigger: " + trigger + "\n    then:\n      - variable: out\n        to: then\n  - else:\n      - variable: out\n        to: else\n"
}

// A line with "for" becomes true - and triggers - once its comparison has held that long; a
// break in between starts the count again; when the comparison ends, the line is false at once.
func TestHoldsFor(t *testing.T) {
	r := newPrig(t)
	r.add("p", held("change"))
	r.set("x", 12, FromUser) // holds from now; not true yet, nothing flips
	r.expect("p", "")
	r.clock.Advance(9 * time.Minute)
	r.set("x", 15, FromUser) // still above: the count goes on
	r.expect("p", "")
	if h := r.e.HoldLines("p"); len(h) != 1 || h[0].Line != 1 || h[0].True || h[0].Due.Sub(h[0].Since) != 10*time.Minute || h[0].For.Std() != 10*time.Minute {
		t.Errorf("counting: %+v", h)
	}
	if h := r.e.HoldLines("nope"); h != nil {
		t.Errorf("no such program: %+v", h)
	}
	r.clock.Advance(time.Minute)
	r.expect("p", "1")
	if runs := r.e.Runs("p"); len(runs) != 0 {
		t.Fatalf("runs %v", runs)
	}
	if h := r.e.HoldLines("p"); len(h) != 1 || !h[0].True {
		t.Errorf("held: %+v", h)
	}
	r.set("x", 20, FromUser) // true before, true now
	r.expect("p", "")
	r.set("x", 5, FromUser) // over: false at once, the truth flips
	r.expect("p", "0")
	if h := r.e.HoldLines("p"); len(h) != 0 {
		t.Errorf("over: %+v", h)
	}
	r.set("x", 12, FromUser)
	r.clock.Advance(5 * time.Minute)
	r.set("x", 5, FromUser) // broke after five minutes: it was never true, nothing flips
	r.expect("p", "")
	r.set("x", 12, FromUser)
	r.clock.Advance(9 * time.Minute)
	r.expect("p", "") // the five minutes before the break do not count
	r.clock.Advance(time.Minute)
	r.expect("p", "1")
	if r.clock.Pending() != 0 {
		t.Errorf("%d timers left", r.clock.Pending())
	}
}

// The run names what triggered it; "update" triggers at the moment too and at every report;
// "check" never triggers but is true once the time is up.
func TestHoldsForModes(t *testing.T) {
	r := newPrig(t)
	r.add("u", held("update"))
	r.add("c", held("check"))
	r.add("v", held("value"))
	r.set("x", 12, FromUser)
	r.expect("u", "0") // a report: evaluated, the line is not true yet
	r.expect("v", "0")
	r.clock.Advance(10 * time.Minute)
	if runs := r.e.Runs("u"); len(runs) != 1 || runs[0].Trigger != "held:1" || runs[0].Block != 1 || !runs[0].Time.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("u: %+v", runs)
	}
	r.expect("u", "1")
	r.expect("v", "1")
	r.expect("c", "")
	if err := r.e.RunProgramBy("c", model.RunCheck, "user"); err != nil {
		t.Fatal(err)
	}
	r.expect("c", "1")
	// an inactive program's line counts all the same, and is true when the program is switched on
	r2 := newPrig(t)
	r2.add("p", "active: false\n"+held("change"))
	r2.set("x", 12, FromUser)
	r2.clock.Advance(10 * time.Minute)
	r2.expect("p", "")
	if err := r2.e.RunProgramBy("p", model.RunCheck, "user"); err != nil {
		t.Fatal(err)
	}
	r2.expect("p", "1")
}

// Two lines of one program that come true at the same instant run it once.
func TestHoldsForTwoLinesOneRun(t *testing.T) {
	r := newPrig(t)
	line := "            - variable: x\n" + gt10 + "              for: 10min\n              trigger: update\n"
	r.add("p", "rules:\n  - when:\n      any:\n        - all:\n"+line+"        - all:\n"+line+"    then:\n      - variable: out\n        to: then\n")
	r.set("x", 12, FromUser)
	r.expect("p", "-")
	r.clock.Advance(10 * time.Minute)
	r.expect("p", "1")
}

// A device line with "for": the first value seen starts the count; a program saved while the
// comparison holds counts from the save; a changed or removed program stops its timers.
func TestHoldsForOnDevicesAndSaves(t *testing.T) {
	r := newPrig(t)
	dev := "rules:\n  - when:\n      all:\n        - all:\n            - device: " + temp + "\n              op: lt\n              value: 5\n              for: 1h\n              trigger: change\n    then:\n      - variable: out\n        to: frost\n"
	r.add("p", dev)
	ref, _ := model.ParseRef(temp)
	r.e.SeedDevice(ref, 3.0) // known from the system's state: holds from now
	r.clock.Advance(59 * time.Minute)
	r.dev(temp, 2.5)
	r.clock.Advance(time.Minute)
	r.expect("p", "1")
	r.dev(temp, 6.0)
	r.expect("p", "-")
	// saved while x is above 10: counts from the save
	r.set("x", 50, FromUser)
	r.clock.Advance(time.Hour)
	r.add("q", held("change"))
	r.clock.Advance(9 * time.Minute)
	r.expect("q", "")
	// the same program saved again keeps its count; with another "for" it starts anew
	r.e.SetPrograms(r.progs)
	r.clock.Advance(time.Minute)
	r.expect("q", "1")
	r.add("q", strings.Replace(held("change"), "10min", "20min", 1))
	r.clock.Advance(19 * time.Minute)
	r.expect("q", "")
	delete(r.progs, "q")
	delete(r.progs, "p")
	r.e.SetPrograms(r.progs)
	if r.clock.Pending() != 0 {
		t.Errorf("%d timers left after the programs went", r.clock.Pending())
	}
}

// The count survives a restart: a comparison that held when the state was written and holds at
// the start has held since then; when the time ran out while the addon was stopped, the
// program runs at the start.
func TestHoldsForAcrossARestart(t *testing.T) {
	r := newPrig(t)
	r.add("p", held("change"))
	dev := "rules:\n  - when:\n      all:\n        - all:\n            - device: " + temp + "\n              op: lt\n              value: 5\n              for: 10min\n              trigger: change\n    then:\n      - variable: out\n        to: frost\n"
	r.add("d", dev)
	ref, _ := model.ParseRef(temp)
	r.set("x", 12, FromUser)
	r.dev(temp, 1.0)
	r.clock.Advance(4 * time.Minute)
	st, dirty := r.e.Export()
	if !dirty || len(st.Programs["p"].Since) != 1 || !st.Programs["p"].Since[0].Equal(t0) {
		t.Fatalf("state: %+v", st.Programs)
	}
	start := func(at time.Duration) *prig {
		n := newPrig(t)
		n.clock.Set(t0.Add(at))
		n.progs = r.progs
		n.e.SetPrograms(r.progs)
		n.e.Import(st) // the variables' values come with it: x is 12
		return n
	}
	// started after five minutes: five more to go for the variable; the device's value comes
	// from the system a moment later and counts from the stored time as well
	n := start(5 * time.Minute)
	n.e.SeedDevice(ref, 1.5)
	n.clock.Advance(4 * time.Minute)
	n.expect("p", "")
	n.clock.Advance(time.Minute)
	n.expect("p", "1")
	n.expect("d", "1")
	// started after an hour: the time is up, the program runs at once
	n = start(time.Hour)
	n.clock.Advance(0)
	n.expect("p", "1")
	// the device reports another value at the start: it does not hold, the stored time is forgotten
	n.e.SeedDevice(ref, 9.0)
	n.e.SeedDevice(ref, 1.0)
	n.clock.Advance(9 * time.Minute)
	n.expect("d", "")
	n.clock.Advance(time.Minute)
	n.expect("d", "1")
	// a variable that no longer holds at the start
	st.Variables["x"] = VariableState{Value: 1.0}
	n = start(time.Hour)
	n.clock.Advance(time.Hour)
	n.expect("p", "")
}

// The random delay is drawn to the second between "after" and "after_up_to" each time the
// block runs.
func TestRandomDelay(t *testing.T) {
	r := &prig{rig: &rig{t: t, clock: NewFakeClock(t0)}, w: &fakeWriter{}, progs: map[string]*model.Program{}}
	var asked []int64
	next := int64(0)
	r.e = New(Options{Clock: r.clock, Writer: r.w, Random: func(n int64) int64 { asked = append(asked, n); return next }})
	d := defs()
	d["zaehler"] = &model.Variable{Key: "zaehler", Name: "Zähler", Type: model.VarNumber}
	r.e.SetVariables(d)
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: regen\n              is: true\n              trigger: update\n    then:\n      - variable: zaehler\n        to: 1\n        after: 5min\n        after_up_to: 30min\n      - variable: zaehler\n        to: 2\n        after: 0s\n        after_up_to: 10s\n")
	r.set("regen", true, FromUser) // drawn 0: the first in five minutes, the second at once
	if p := r.e.PendingActions(); len(p) != 1 || !p[0].Due.Equal(t0.Add(5*time.Minute)) || r.value("zaehler") != 2.0 {
		t.Fatalf("pending %+v, zaehler %v", p, r.value("zaehler"))
	}
	next = 7
	r.set("regen", true, FromUser)
	p := r.e.PendingActions()
	if len(p) != 3 || !p[0].Due.Equal(t0.Add(7*time.Second)) || !p[2].Due.Equal(t0.Add(5*time.Minute+7*time.Second)) {
		t.Fatalf("pending %+v", p)
	}
	if fmt.Sprint(asked) != "[1500 10 1500 10]" {
		t.Errorf("the dice were asked for %v", asked)
	}
	if runs := r.e.Runs("p"); !strings.Contains(runs[1].Notes()[0], "in 5min7s") {
		t.Errorf("notes %v", runs[1].Notes())
	}
	// the real dice stay inside the span
	e := New(Options{})
	for i := 0; i < 200; i++ {
		if n := e.o.Random(3); n < 0 || n > 3 {
			t.Fatalf("dice %d", n)
		}
	}
}

type setWriter struct {
	fakeWriter
	sets []string
}

func (w *setWriter) WriteSet(channel model.Ref, values []model.ParamValue, done func(error)) {
	w.sets = append(w.sets, fmt.Sprintf("%s %v", channel.Channel(), values))
	w.done = append(w.done, done)
}

const onFor = "rules:\n  - when:\n      all:\n        - all:\n            - variable: regen\n              is: true\n              trigger: update\n    then:\n      - set: BidCos-RF/NEQ0123456:1/STATE\n        to: true\n        on_for: 5min\n"

// "ein für": the on-time and the value in one command; with a writer that cannot, the on-time
// first and the value after it. A refused write counts as the program's failure.
func TestOnFor(t *testing.T) {
	w := &setWriter{}
	r := &prig{rig: &rig{t: t, clock: NewFakeClock(t0)}, w: &w.fakeWriter, progs: map[string]*model.Program{}}
	r.e = New(Options{Clock: r.clock, Writer: w})
	r.e.SetVariables(defs())
	r.add("p", onFor)
	r.set("regen", true, FromUser)
	if len(w.sets) != 1 || w.sets[0] != "BidCos-RF/NEQ0123456:1 [{ON_TIME 300} {STATE true}]" || len(w.writes) != 0 {
		t.Fatalf("sets %v, writes %v", w.sets, w.writes)
	}
	if runs := r.e.Runs("p"); !strings.Contains(runs[0].Notes()[0], "STATE = true for 5min") {
		t.Errorf("notes %v", runs[0].Notes())
	}
	w.done[0](fmt.Errorf("refused"))
	w.done[0](nil)
	if in, _ := r.e.Program("p"); in.Failures != 1 || !strings.Contains(in.LastFailure, "refused") {
		t.Errorf("failures: %+v", in)
	}
	r2 := newPrig(t)
	r2.add("p", onFor)
	r2.set("regen", true, FromUser)
	if len(r2.w.writes) != 2 || r2.w.writes[0] != (write{"BidCos-RF/NEQ0123456:1/ON_TIME", 300.0}) || r2.w.writes[1] != (write{"BidCos-RF/NEQ0123456:1/STATE", true}) {
		t.Fatalf("writes %v", r2.w.writes)
	}
}

// The sun's position as two system values: fed from the clock and the position, an event for a
// program that looks at them; nothing without a position or before the clock is set.
func TestSunValues(t *testing.T) {
	ready := false
	r := &prig{rig: &rig{t: t, clock: NewFakeClock(time.Date(2026, 6, 15, 11, 7, 0, 0, time.UTC))}, w: &fakeWriter{}, progs: map[string]*model.Program{}}
	r.e = New(Options{Clock: r.clock, ClockReady: func() bool { return ready }})
	d := defs()
	d["out"] = &model.Variable{Key: "out", Name: "Out", Type: model.VarString}
	r.e.SetVariables(d)
	r.add("shade", "rules:\n  - when:\n      all:\n        - all:\n            - variable: sun-elevation\n              op: gt\n              value: 30\n              trigger: change\n            - variable: sun-azimuth\n              op: range\n              from: 135\n              below: 225\n              trigger: check\n    then:\n      - variable: out\n        to: shade\n  - else:\n      - variable: out\n        to: open\n")
	r.e.UpdateSun() // no position
	r.e.SetPosition(schedule.Position{Lat: 52.5, Lon: 13.4, Known: true})
	r.e.UpdateSun() // the clock is not set
	if v := r.e.SystemValues(); v[3].Value != 0 || v[4].Value != 0 {
		t.Fatalf("values before the clock is set: %v", v)
	}
	ready = true
	r.e.UpdateSun()
	v := r.e.SystemValues()
	if v[3].Key != model.SystemSunAzimuth || v[3].Value < 178 || v[3].Value > 182 || v[4].Value < 60 || v[4].Value > 61.5 || v[4].Value*10 != float64(int(v[4].Value*10)) {
		t.Fatalf("Berlin at solar noon in June: %v", v)
	}
	r.expect("shade", "1")
	r.e.UpdateSun() // the same minute: nothing moved, no event
	r.expect("shade", "")
	r.clock.Advance(9 * time.Hour) // 22:07 CEST: the sun is down
	r.e.UpdateSun()
	r.expect("shade", "0")
	if v := r.e.SystemValues(); v[4].Value > 0 || v[3].Value < 300 {
		t.Errorf("in the evening: %v", v)
	}
}
