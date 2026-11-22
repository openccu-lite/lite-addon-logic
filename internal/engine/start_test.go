package engine

import (
	"reflect"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Q-12, as measured on the CCU at a restart of ReGaHss (concept Appendix B-1): every active
// program is evaluated once, with its conditions. An empty condition runs THEN; a program that
// is true runs THEN whatever its trigger mode - one with only a "nur prüfen" line too; a
// program with only time modules runs ELSE; a false program without ELSE runs nothing; an
// inactive program is not evaluated. Here the program's own switch decides whether it takes
// part at all, and afterwards every "bei Änderung" line knows where it stands: writing the
// unchanged value triggers nothing.
func TestEvaluateAtStart(t *testing.T) {
	r := newTrig(t, "2026-11-21 21:00:00 CET")
	line := func(mode string) string {
		return "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: gt\n              value: 5\n              trigger: " + mode + "\n    then: []\n"
	}
	r.set("x", 9, FromUser)
	r.add("empty", "at_start: true\nrules:\n  - when:\n      all: []\n    then: []\n  - else: []\n")
	r.add("checked", "at_start: true\n"+line("check"))
	r.add("changes", "at_start: true\n"+line("change")+"  - else: []\n")
	r.add("timer", "at_start: true\n"+timed(true, `kind: daily, at: "06:30"`))
	r.add("interval", "at_start: true\n"+timed(true, `kind: interval, all_day: true, interval: 15min`))
	r.add("night", "at_start: true\n"+timed(true, `kind: daily, at: "20:00", duration: 8h`))
	r.add("untrue", "at_start: true\nrules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: lt\n              value: 5\n              trigger: update\n    then: []\n")
	r.add("idle", "active: false\nat_start: true\n"+line("update"))
	r.add("notatstart", line("update"))
	for _, k := range []string{"empty", "checked", "changes", "timer", "interval", "night", "untrue", "idle", "notatstart"} {
		r.ran(k) // what adding and the write did is not the matter here
	}
	got := r.e.EvaluateAtStart()
	if want := []string{"changes", "checked", "empty", "interval", "night", "timer", "untrue"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("evaluated: %v", got)
	}
	r.want("empty", "2026-11-21 21:00:00 CET start 1")
	r.want("checked", "2026-11-21 21:00:00 CET start 1")
	r.want("changes", "2026-11-21 21:00:00 CET start 1")
	r.want("timer", "2026-11-21 21:00:00 CET start 0")
	r.want("interval", "2026-11-21 21:00:00 CET start 0") // it has not fired yet: false
	r.want("night", "2026-11-21 21:00:00 CET start 1")    // inside its range
	r.want("untrue", "2026-11-21 21:00:00 CET start -")
	r.want("idle")
	r.want("notatstart")
	// the start evaluation is where a "bei Änderung" line starts from
	r.set("x", 8, FromUser) // still greater than 5: no flip
	r.want("changes")
	r.set("x", 1, FromUser)
	r.want("changes", "2026-11-21 21:00:00 CET variable:x 0")
}

// What the evaluation waits for: a value for every device datapoint its programs look at.
func TestStartUnknown(t *testing.T) {
	r := newTrig(t, "2026-11-21 21:00:00 CET")
	dev := func(ref string) string {
		return "rules:\n  - when:\n      all:\n        - all:\n            - device: " + ref + "\n              op: gt\n              value: 0\n              trigger: change\n    then: []\n"
	}
	r.add("window", "at_start: true\n"+dev("HmIP-RF/A:1/STATE"))
	r.add("door", "at_start: true\n"+dev("HmIP-RF/B:1/STATE"))
	r.add("key", "at_start: true\n"+dev("HmIP-RF/C:1/PRESS_SHORT"))
	r.add("later", dev("HmIP-RF/D:1/STATE"))
	r.add("idle", "active: false\nat_start: true\n"+dev("HmIP-RF/E:1/STATE"))
	if got := r.e.StartUnknown(); !reflect.DeepEqual(got, []string{"HmIP-RF/A:1/STATE", "HmIP-RF/B:1/STATE"}) {
		t.Fatalf("unknown: %v", got)
	}
	r.e.SeedDevice(model.Ref{Interface: "HmIP-RF", Address: "A:1", Param: "STATE"}, 1.0)
	r.e.DeviceEvent(model.Ref{Interface: "HmIP-RF", Address: "B:1", Param: "STATE"}, 0.0)
	if got := r.e.StartUnknown(); len(got) != 0 {
		t.Errorf("all known: %v", got)
	}
}

func TestClockSetAndLoopGuard(t *testing.T) {
	ready := false
	r := newTrig(t, "2026-11-21 21:00:00 CET", func(o *Options) { o.ClockReady = func() bool { return ready } })
	if r.e.ClockSet() {
		t.Error("the clock is not set yet")
	}
	ready = true
	if !r.e.ClockSet() {
		t.Error("the clock is set")
	}
	if g := r.e.LoopGuard(); g != DefaultLoopGuard {
		t.Errorf("the default: %+v", g)
	}
	r.e.SetLoopGuard(LoopGuard{Runs: 2, Window: time.Second, Pause: 5 * time.Second})
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: ge\n              value: 0\n              trigger: update\n    then: []\n")
	r.ran("p")
	for i := 0; i < 3; i++ {
		r.set("x", 1, FromUser)
	}
	r.want("p", "2026-11-21 21:00:00 CET variable:x 1", "2026-11-21 21:00:00 CET variable:x 1", "2026-11-21 21:00:00 CET variable:x - paused")
	r.clock.Advance(4 * time.Second)
	r.set("x", 1, FromUser)
	r.want("p")
	r.clock.Advance(2 * time.Second)
	r.set("x", 1, FromUser)
	r.want("p", "2026-11-21 21:00:06 CET variable:x 1")
	// a zero field keeps its default
	r.e.SetLoopGuard(LoopGuard{})
	if g := r.e.LoopGuard(); g != DefaultLoopGuard {
		t.Errorf("back to the default: %+v", g)
	}
}
