package engine

import (
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const simulated = `rules:
  - when:
      all:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: change
            - device: HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE
              op: lt
              value: 30
              trigger: check
    cancel_pending: true
    then:
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        to: 0.5
      - variable: out
        to: later
        after: 30s
      - variable: out
        from_variable: text
      - variable: modus
        from_variable: text
      - run: other
        mode: check
      - deactivate: other
      - script: "! kept"
  - when:
      all:
        - all:
            - variable: y
              is: 1
              trigger: value
            - time:
                kind: daily
                at: "06:00"
              trigger: update
    then:
      - variable: out
        to: elseif
  - else:
      - variable: out
        to: else
`

func TestDryRunAndSimulate(t *testing.T) {
	r := newPrig(t)
	r.set("text", "hallo", FromUser)
	r.add("other", "rules:\n  - when:\n      all: []\n    then:\n      - variable: out\n        to: other\n")
	p, err := model.DecodeProgram("p.yaml", "p", []byte("name: p\nactive: true\n"+simulated))
	if err != nil {
		t.Fatal(err)
	}
	r.progs["p"] = p
	r.e.SetPrograms(r.progs)
	r.dev(temp, 20.0)
	// a delay is pending for the target the first block names
	r.add("q", "rules:\n  - when:\n      all: []\n    then:\n      - variable: out\n        to: q\n        after: 1h\n")
	_ = r.e.RunProgramBy("q", model.RunThen, "user")
	r.e.mu.Lock()
	r.e.progs["p"].problems = nil // (the script; the list entry that does not fit)
	r.e.mu.Unlock()
	state := func() string {
		st, _ := r.e.Export()
		b, _ := MarshalState(st)
		return string(b) + "|" + r.texts("p") + "|" + r.texts("other") + "|" + strings.Join(strings.Fields(strings.Repeat("x", len(r.w.writes))), "")
	}
	before, notes := state(), len(r.notes)
	text := func(run Run, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if !run.Dry || run.Seq != 0 {
			t.Errorf("not marked as a simulation: %+v", run)
		}
		return run.Text()
	}
	want := func(got, want string) {
		t.Helper()
		if got != strings.TrimSpace(want) {
			t.Fatalf("got:\n%s\nwant:\n%s", got, strings.TrimSpace(want))
		}
	}
	// now: x is 0, so the chain reaches SONST
	want(text(r.e.DryRun("p")), `
12:00:00 user: block 0
  line 1 (block 1) x gt 10 [change]: 0 is false
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE lt 30 [check]: 20 is true
  line 3 (block 2) y eq 1 [value]: 0 is false
  line 4 (block 2) time module daily [update]: false
  1: variable out = else`)
	// x reports 11: the flip triggers, the first block would run - every kind of step, none done
	want(text(r.e.Simulate("p", 1, 11)), `
12:00:00 variable:x (0 -> 11): block 1
  line 1 (block 1) x gt 10 [change]: 11 is true
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE lt 30 [check]: 20 is true
  line 3 (block 2) y eq 1 [value]: not looked at, a block before ran
  line 4 (block 2) time module daily [update]: not looked at, a block before ran
  cancelled 1 pending
  1: set HmIP-RF/00111BE9AB9012:4/LEVEL = 0.5
  2: variable out scheduled for 12:00:30
  3: variable out = hallo
  4: variable modus: modus takes one of its entries (Sommer, Übergang, Winter)
  5: run other (check)
  6: deactivate other
  7: script: skipped, scripts are never run`)
	// x reports 5: the line's truth stays false - not triggered, and why
	want(text(r.e.Simulate("p", 1, 5.0)), `12:00:00 variable:x (0 -> 5): not triggered, no-change`)
	// the line that only checks, with a value that would make it false
	want(text(r.e.Simulate("p", 2, 35.0)), `12:00:00 device:HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE (20 -> 35): not triggered, check-only`)
	// "at every change of the value": the same value does not trigger, another one does
	want(text(r.e.Simulate("p", 3, 0.0)), `12:00:00 variable:y (0 -> 0): not triggered, same-value`)
	want(text(r.e.Simulate("p", 3, 1.0)), `
12:00:00 variable:y (0 -> 1): block 0
  line 1 (block 1) x gt 10 [change]: 0 is false
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE lt 30 [check]: 20 is true
  line 3 (block 2) y eq 1 [value]: 1 is true
  line 4 (block 2) time module daily [update]: false
  1: variable out = else`)
	// the time module fires: its line is true at that moment
	want(text(r.e.Simulate("p", 4, nil)), `
12:00:00 time:4:point: block 0
  line 1 (block 1) x gt 10 [change]: 0 is false
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE lt 30 [check]: 20 is true
  line 3 (block 2) y eq 1 [value]: 0 is false
  line 4 (block 2) time module daily [update]: true
  1: variable out = else`)
	// nothing happened: no write, no notification, no entry, the state as it was
	if after := state(); after != before || len(r.notes) != notes || len(r.e.PendingActions()) != 1 {
		t.Errorf("a simulation left a trace:\n%s\nbefore:\n%s", after, before)
	}
	// what cannot be simulated says so
	for _, c := range []struct {
		run  func() (Run, error)
		code string
	}{
		{func() (Run, error) { return r.e.DryRun("nothing") }, "unknown-program"},
		{func() (Run, error) { return r.e.Simulate("nothing", 1, 1) }, "unknown-program"},
		{func() (Run, error) { return r.e.Simulate("p", 0, 1) }, "invalid"},
		{func() (Run, error) { return r.e.Simulate("p", 5, 1) }, "invalid"},
		{func() (Run, error) { return r.e.Simulate("p", 1, "text") }, "wrong-type"},
		{func() (Run, error) { return r.e.Simulate("p", 2, []int{1}) }, "invalid"},
	} {
		_, err := c.run()
		if we, ok := err.(*WriteError); !ok || we.Code != c.code {
			t.Errorf("want %s, got %v", c.code, err)
		}
	}
	r.e.mu.Lock()
	r.e.progs["p"].problems = []string{"broken"}
	r.e.mu.Unlock()
	if _, err := r.e.DryRun("p"); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("a program with problems: %v", err)
	}
	// a device nobody heard of yet: the simulated value is its first; without a writer a
	// device write would be dropped
	e := New(Options{Clock: NewFakeClock(t0)})
	only, err := model.DecodeProgram("d.yaml", "d", []byte("name: d\nactive: true\nrules:\n  - when:\n      all:\n        - all:\n            - device: "+temp+"\n              op: gt\n              value: 5\n              trigger: update\n    then:\n      - set: HmIP-RF/00111BE9AB9012:4/LEVEL\n        to: 1\n      - variable: gone\n        from: "+temp+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	e.SetPrograms(map[string]*model.Program{"d": only})
	e.mu.Lock()
	e.progs["d"].problems = nil
	e.mu.Unlock()
	want(text(e.Simulate("d", 1, 7)), `
12:00:00 device:HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE (first value 7): block 1
  line 1 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE gt 5 [update]: 7 is true
  1: set HmIP-RF/00111BE9AB9012:4/LEVEL = 1: dropped, nothing sends device writes
  2: variable gone: there is no variable gone`)
	if _, ok := e.DeviceValue(model.Ref{Interface: "HmIP-RF", Address: "000A1B2C3D4E5F:1", Param: "ACTUAL_TEMPERATURE"}); ok {
		t.Error("the simulated value stayed")
	}
}
