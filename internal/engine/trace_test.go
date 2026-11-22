package engine

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// texts answers a program's entries as the trace says them in English.
func (r *prig) texts(key string) string {
	var out []string
	for _, run := range r.e.Runs(key) {
		out = append(out, run.Text())
	}
	return strings.Join(out, "\n")
}

func (r *prig) wantTrace(key, want string) {
	r.t.Helper()
	if got := r.texts(key); got != strings.TrimSpace(want) {
		r.t.Fatalf("the trace of %s:\n%s\nwant:\n%s", key, got, strings.TrimSpace(want))
	}
}

const traced = `rules:
  - when:
      any:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: change
            - device: HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE
              op: range
              from: 5
              below: 30
              trigger: check
        - any:
            - variable: y
              not: true
              op: outside
              from: 1
              to: 2
              for: 10min
              trigger: value
    cancel_pending: true
    then:
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        to: 0.5
      - set: BidCos-RF/NEQ0123456:1/STATE
        to: true
        on_for: 5min
      - variable: out
        to: later
        after: 30s
      - variable: modus
        to: Nirgends
      - run: ziel
        mode: check
      - run: missing
        mode: then
      - deactivate: ziel
      - activate: missing
      - variable: out
        special: OLD
      - variable: out
        from: HmIP-RF/00111BE9AB9012:9/UNKNOWN
      - variable: out
        from_variable: gone
      - variable: out
        from_variable: text
      - variable: offen
        from: HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE
      - script: "! kept"
  - when:
      all:
        - all:
            - variable: x
              is: 3
              trigger: check
    then: []
  - else:
      - variable: out
        to: else
        after: 1min
`

func TestTheRunTrace(t *testing.T) {
	r := newPrig(t)
	r.set("text", "hallo", FromUser)
	r.add("ziel", "rules:\n  - when:\n      all: []\n    then: []\n")
	// a program with broken references does not run: load it past its problems, to see every
	// kind of step
	p, err := model.DecodeProgram("p.yaml", "p", []byte("name: p\nactive: true\n"+traced))
	if err != nil {
		t.Fatal(err)
	}
	r.progs["p"] = p
	r.e.SetPrograms(r.progs)
	r.e.mu.Lock()
	r.e.progs["p"].problems = nil
	r.e.mu.Unlock()
	// a line that only checks does not trigger; the first value of a device neither
	r.dev(temp, 20.0)
	r.dev(temp, 21.0)
	r.dev(temp, 21.0)
	r.wantTrace("p", `
12:00:00 device:HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE (21 -> 21): not triggered, check-only (3 times since 12:00:00)`)
	if e := r.e.Runs("p")[0]; !e.Untriggered || e.Count != 3 || e.Event.Value != 21.0 || !e.None {
		t.Errorf("the note: %+v", e)
	}
	// the same value on a "value" line, the same truth on a "change" line
	r.clock.Advance(time.Second)
	r.set("y", 0, FromUser)
	r.set("x", 5, FromUser)
	r.set("x", 6, FromUser)
	r.wantTrace("p", `
12:00:00 device:HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE (21 -> 21): not triggered, check-only (3 times since 12:00:00)
12:00:01 variable:y (0 -> 0): not triggered, same-value
12:00:01 variable:x (5 -> 6): not triggered, no-change (2 times since 12:00:01)`)
	if got := r.e.Trace(0); len(got) != 0 {
		t.Errorf("notes in the global log: %+v", got)
	}
	// SONST: every line with its value, the delay scheduled
	r.set("y", 1, FromUser)
	r.clock.Advance(time.Second)
	r.wantTraceTail("p", 1, `
12:00:01 variable:y (0 -> 1): block 0
  line 1 (block 1) x gt 10 [change]: 6 is false
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE from 5 to less than 30 [check]: 21 is true
  line 3 (block 1) y not outside 1 to 2 for 10min [value]: 1 is false, holds since 12:00:01
  line 4 (block 2) x eq 3 [check]: 6 is false
  1: variable out scheduled for 12:01:01`)
	// the first block: it ends the delay, and does one of everything
	r.set("x", 11, FromUser)
	r.wantTraceTail("p", 1, `
12:00:02 variable:x (6 -> 11): block 1
  line 1 (block 1) x gt 10 [change]: 11 is true
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE from 5 to less than 30 [check]: 21 is true
  line 3 (block 1) y not outside 1 to 2 for 10min [value]: 1 is false, holds since 12:00:01
  line 4 (block 2) x eq 3 [check]: not looked at, a block before ran
  cancelled 1 pending
  1: set HmIP-RF/00111BE9AB9012:4/LEVEL = 0.5
  2: set BidCos-RF/NEQ0123456:1/STATE = true for 5min
  3: variable out scheduled for 12:00:32
  4: variable modus: modus takes one of its entries (Sommer, Übergang, Winter)
  5: run ziel (check)
  6: run missing: there is no such program
  7: deactivate ziel
  8: activate missing: there is no such program
  9: variable out: skipped, the special value OLD is not supported
  10: variable out: skipped, the value of HmIP-RF/00111BE9AB9012:9/UNKNOWN is not known
  11: variable out: skipped, there is no variable gone
  12: variable out = hallo
  13: variable offen = 21
  14: script: skipped, scripts are never run`)
	runs := r.e.Runs("p")
	run := runs[len(runs)-1]
	if s := run.Steps[4]; s.What != StepRefused || s.Reason == "" {
		t.Errorf("the refused write: %+v", s)
	}
	if s := run.Steps[14]; s.Def.Script != "" || s.Reason != ReasonScript {
		t.Errorf("the script's text went into the trace: %+v", s.Def)
	}
	if run.Lines[2].Since.IsZero() || !run.Lines[3].Skipped || run.Lines[0].Def.Variable != "x" {
		t.Errorf("lines: %+v", run.Lines)
	}
	// the SONST's delay says who ended it; the interface's answers arrive on the steps
	sonst := runs[len(runs)-2]
	if s := sonst.Steps[0]; s.Ended.IsZero() || s.EndedBy != "program:p" {
		t.Errorf("the ended delay: %+v", s)
	}
	r.w.done[0](nil)
	r.w.done[1](errors.New("no answer"))
	r.w.done[1](nil) // a late yes does not take the failure back
	r.w.done[0](errors.New("after all"))
	r.wantTraceTail("p", 1, "")
	text := r.e.Runs("p")[len(runs)-1].Text()
	for _, want := range []string{"1: set HmIP-RF/00111BE9AB9012:4/LEVEL = 0.5, failed: after all", "= true for 5min, failed: no answer"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	// the program it ran has an entry that names it
	if z := r.e.Runs("ziel"); len(z) != 1 || z[0].Trigger != "program:p" || z[0].Text() != "12:00:02 program:p: block 1" {
		t.Errorf("ziel: %+v", z)
	}
	// the delayed activity comes due: an entry of its own, and the step that scheduled it says so
	r.clock.Advance(30 * time.Second)
	r.wantTraceTail("p", 1, `
12:00:32 variable:x: a delayed activity of block 1 is due, nothing is checked again
  3: variable out = later`)
	if s := r.e.Runs("p")[len(runs)-1].Steps[3]; !s.Ran.Equal(t0.Add(32*time.Second)) || !strings.Contains(r.e.Runs("p")[len(runs)-1].Text(), "scheduled for 12:00:32, ran 12:00:32") {
		t.Errorf("the step that scheduled it: %+v", s)
	}
	// a confirmed write
	r.add("q", "rules:\n  - when:\n      all: []\n    then:\n      - set: HmIP-RF/00111BE9AB9012:4/LEVEL\n        to: 1\n      - variable: out\n        to: q\n        after: 1min\n  - else:\n      - variable: out\n        to: q-else\n        after: 1min\n")
	_ = r.e.RunProgramBy("q", model.RunThen, "user")
	r.clock.Advance(2 * time.Second)
	r.w.done[len(r.w.done)-1](nil)
	r.wantTrace("q", `
12:00:32 user: block 1 without a look at the conditions
  1: set HmIP-RF/00111BE9AB9012:4/LEVEL = 1, confirmed 12:00:34
  2: variable out scheduled for 12:01:32`)
	// ended by hand, and by the program's removal
	id := r.e.PendingActions()[0].ID
	if !r.e.CancelPending(id) {
		t.Fatal("not cancelled")
	}
	if text := r.texts("q"); !strings.Contains(text, "scheduled for 12:01:32, ended 12:00:34 by user") {
		t.Errorf("ended by hand:\n%s", text)
	}
	_ = r.e.RunProgramBy("q", model.RunThen, "outside")
	delete(r.progs, "q")
	r.e.SetPrograms(r.progs)
	all := r.e.Trace(0)
	if last := all[len(all)-1]; last.Program != "q" || last.Steps[1].EndedBy != "removed" {
		t.Errorf("ended by the removal: %+v", last)
	}
	// the global log: every entry in order, a page of it the newest first
	for i := 1; i < len(all); i++ {
		if all[i].Seq <= all[i-1].Seq {
			t.Fatalf("not in order at %d", i)
		}
	}
	if rest := r.e.Trace(all[1].Seq); len(rest) != len(all)-2 || rest[0].Seq != all[2].Seq {
		t.Errorf("after an entry: %d of %d", len(rest), len(all))
	}
	page, more := r.e.TracePage("", 0, 2)
	if len(page) != 2 || !more || page[0].Seq != all[len(all)-1].Seq {
		t.Errorf("the first page: %+v, more %t", page, more)
	}
	page, more = r.e.TracePage("", all[1].Seq, 5)
	if len(page) != 1 || more || page[0].Seq != all[0].Seq {
		t.Errorf("the last page: %+v, more %t", page, more)
	}
	if page, more = r.e.TracePage("p", 0, 500); len(page) != len(r.e.Runs("p")) || more || !page[len(page)-1].Untriggered {
		t.Errorf("a program's page: %d entries, more %t", len(page), more)
	}
	if page, more = r.e.TracePage("nothing", 0, 5); len(page) != 0 || more {
		t.Errorf("an unknown program's page: %+v", page)
	}
	// what Runs answers is the caller's: a later answer of the interface does not reach it
	if r.e.Runs("nothing") != nil {
		t.Error("runs of an unknown program")
	}
}

// wantTraceTail compares the last n entries of a program ("" only checks that there are any).
func (r *prig) wantTraceTail(key string, n int, want string) {
	r.t.Helper()
	runs := r.e.Runs(key)
	if len(runs) < n {
		r.t.Fatalf("%s has %d entries", key, len(runs))
	}
	if want == "" {
		return
	}
	var out []string
	for _, run := range runs[len(runs)-n:] {
		out = append(out, run.Text())
	}
	if got := strings.Join(out, "\n"); got != strings.TrimSpace(want) {
		r.t.Fatalf("the trace of %s:\n%s\nwant:\n%s", key, got, strings.TrimSpace(want))
	}
}

func TestTraceWordsForTheRest(t *testing.T) {
	r := newPrig(t)
	// a first value, a value that is not known, a time module, the loop guard, a missed time
	r.add("d", "rules:\n  - when:\n      all:\n        - all:\n            - device: "+temp+"\n              op: gt\n              value: 5\n              trigger: change\n            - device: HmIP-RF/000A1B2C3D4E5F:2/STATE\n              is: true\n              trigger: update\n            - time:\n                kind: daily\n                at: \"06:00\"\n              trigger: check\n    then: []\n")
	r.dev(temp, 20.0)
	r.dev("HmIP-RF/000A1B2C3D4E5F:2/STATE", true)
	r.e.mu.Lock()
	r.e.progs["d"].lines[1].def = &model.Line{Device: &model.Ref{Interface: "X", Address: "Y:1", Param: "Z"}, Trigger: model.TriggerUpdate}
	r.e.mu.Unlock()
	_ = r.e.RunProgramBy("d", model.RunCheck, "user")
	r.wantTrace("d", `
12:00:00 device:HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE (first value 20): not triggered, first-value
12:00:00 device:HmIP-RF/000A1B2C3D4E5F:2/STATE (first value true): evaluated, no block ran
  line 1 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE gt 5 [change]: 20 is true
  line 2 (block 1) HmIP-RF/000A1B2C3D4E5F:2/STATE eq true [update]: true is true
  line 3 (block 1) time module daily [check]: false
12:00:00 user: evaluated, no block ran
  line 1 (block 1) HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE gt 5 [change]: 20 is true
  line 2 (block 1) X/Y:1/Z eq <nil> [update]: the value is not known: false
  line 3 (block 1) time module daily [check]: false`)
	for _, c := range []struct {
		run  Run
		want string
	}{
		{Run{Time: t0, Trigger: "variable:x", Info: InfoPaused, None: true}, "12:00:00 variable:x: paused: the program ran too often (the loop guard)"},
		{Run{Time: t0, Trigger: "time", Info: InfoMissed, Due: t0.Add(-time.Hour), None: true}, "12:00:00 time: missed: due at 2026-11-22T11:00:00Z, the clock was set or the system stood still"},
		{Run{Time: t0, Trigger: "start", Delayed: true, Block: 2, Steps: []Step{{Action: 1, Def: &model.Action{Kind: model.ActionVariable, Variable: "x"}, What: StepDropped, Due: t0.Add(-time.Minute)}}},
			"12:00:00 start: a delayed activity of block 2 is due, nothing is checked again\n  1: variable x: dropped, it was due at 2026-11-22T11:59:00Z while the addon was stopped"},
		{Run{Time: t0, Trigger: "user", Block: 1, Steps: []Step{{Action: 1, Def: &model.Action{Kind: model.ActionSet, Device: &model.Ref{Interface: "A", Address: "B:1", Param: "C"}}, What: StepSkipped, Reason: ReasonNoWriter, Value: 1.0}}},
			"12:00:00 user: block 1\n  1: set A/B:1/C = 1: dropped, nothing sends device writes"},
	} {
		if got := c.run.Text(); got != c.want {
			t.Errorf("got  %s\nwant %s", got, c.want)
		}
	}
	if n := (Run{Info: InfoNoChange, Untriggered: true}).Notes(); len(n) != 1 || n[0] != "not triggered: no-change" {
		t.Errorf("notes: %v", n)
	}
}

func TestTraceRingsAndImport(t *testing.T) {
	r := newPrig(t)
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: y\n              op: ge\n              value: 0\n              trigger: update\n    then:\n      - variable: out\n        to: a\n        after: 1h\n")
	for i := 0; i < TraceKept+TraceKept/10+5; i++ {
		r.set("y", 1, FromUser)
		r.clock.Advance(time.Second)
	}
	all := r.e.Trace(0)
	if len(all) < TraceKept || len(all) > TraceKept+TraceKept/10 || len(r.e.Runs("p")) != RunsKept {
		t.Fatalf("%d in the global log, %d with the program", len(all), len(r.e.Runs("p")))
	}
	// the step of an entry the log dropped is not found any more: ending its delay is no error
	first := r.e.PendingActions()[0]
	if !r.e.CancelPending(first.ID) {
		t.Fatal("not cancelled")
	}
	// another start: the log comes back, the engine counts on, the program has its entries
	r2 := newPrig(t)
	r2.clock.Set(r.clock.Now())
	r2.progs = r.progs
	r2.e.SetPrograms(r.progs)
	extra := append(append([]Run{}, all...), Run{Seq: 3, Program: "p"}, Run{Seq: all[len(all)-1].Seq + 1, Program: "gone", Time: r.clock.Now()})
	r2.e.ImportTrace(extra)
	back := r2.e.Trace(0)
	if len(back) != TraceKept-1 || back[len(back)-1].Program != "gone" || len(r2.e.Runs("p")) != RunsKept {
		t.Fatalf("after the import: %d entries, the last %+v, %d with the program", len(back), back[len(back)-1], len(r2.e.Runs("p")))
	}
	r2.set("y", 2, FromUser)
	if got := r2.e.Trace(back[len(back)-1].Seq); len(got) != 1 || got[0].Seq != back[len(back)-1].Seq+1 {
		t.Errorf("the next entry: %+v", got)
	}
	// a delay of the state that names a step: found when program, step and time fit
	st, _ := r.e.Export()
	last := all[len(all)-1]
	for i := len(all) - 1; last.Delayed; i-- {
		last = all[i]
	}
	for _, p := range []Pending{
		{Run: last.Seq, Step: 0, Program: "p", Due: last.Steps[0].Due},
		{Run: last.Seq, Step: 0, Program: "p", Due: last.Steps[0].Due.Add(time.Second)},
		{Run: last.Seq, Step: 4, Program: "p", Due: last.Steps[0].Due},
		{Run: last.Seq, Step: 0, Program: "other", Due: last.Steps[0].Due},
		{Run: 1, Program: "p"},
		{Program: "p"},
	} {
		r2.e.mu.Lock()
		s := r2.e.findStep(&p)
		r2.e.mu.Unlock()
		if (s != nil) != (p.Step == 0 && p.Program == "p" && p.Run == last.Seq && p.Due.Equal(last.Steps[0].Due)) {
			t.Errorf("findStep(%+v) = %v", p, s)
		}
	}
	// what was due while the addon was stopped is an entry that says so
	r3 := newPrig(t)
	r3.clock.Set(r.clock.Now().Add(2 * time.Hour))
	r3.progs = r.progs
	r3.e.SetPrograms(r.progs)
	st.Pending = st.Pending[:1]
	r3.e.Import(st)
	if got := r3.texts("p"); !strings.Contains(got, "a delayed activity of block 1 is due") || !strings.Contains(got, "1: variable out: dropped, it was due at") {
		t.Errorf("the dropped delay:\n%s", got)
	}
}
