package engine

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// The scenarios: a program as its file has it, a script of events, what must have run. The
// numbers (Q-n) are the quirk catalogue's (the concept's Appendix A); "measured" is what the
// CCU's ReGaHss did when the same program and the same writes were tried on it.

type write struct {
	ref   string
	value any
}

type fakeWriter struct {
	writes []write
	done   []func(error)
}

func (w *fakeWriter) Write(ref model.Ref, value any, done func(error)) {
	w.writes = append(w.writes, write{ref.String(), value})
	w.done = append(w.done, done)
}

type prig struct {
	*rig
	w     *fakeWriter
	progs map[string]*model.Program
}

// newPrig makes an engine with the usual variables and a marker variable per program letter:
// a program writes "then"/"else" markers into the string variable out.
func newPrig(t *testing.T) *prig {
	r := &prig{rig: &rig{t: t, clock: NewFakeClock(t0)}, w: &fakeWriter{}, progs: map[string]*model.Program{}}
	r.e = New(Options{Clock: r.clock, Writer: r.w, Notify: func(n Notification) { r.notes = append(r.notes, n) }, Log: func(format string, args ...any) { r.logs = append(r.logs, fmt.Sprintf(format, args...)) }})
	d := defs()
	d["out"] = &model.Variable{Key: "out", Name: "Out", Type: model.VarString}
	d["x"] = &model.Variable{Key: "x", Name: "X", Type: model.VarNumber}
	d["y"] = &model.Variable{Key: "y", Name: "Y", Type: model.VarNumber}
	r.e.SetVariables(d)
	return r
}

// add loads a program from YAML (active unless the text says otherwise).
func (r *prig) add(key, yaml string) {
	r.t.Helper()
	if !strings.Contains(yaml, "active:") {
		yaml = "active: true\n" + yaml
	}
	p, err := model.DecodeProgram(key+".yaml", key, []byte("name: "+key+"\n"+yaml))
	if err != nil {
		r.t.Fatalf("%s: %v", key, err)
	}
	r.progs[key] = p
	r.e.SetPrograms(r.progs)
	if in, _ := r.e.Program(key); len(in.Problems) > 0 {
		r.t.Fatalf("%s: %v", key, in.Problems)
	}
}

// blocks answers the blocks the program ran since the last call: "1", "2", "0" (SONST), "-"
// (evaluated, nothing ran).
func (r *prig) blocks(key string) string {
	var out []string
	for _, run := range r.e.Runs(key) {
		switch {
		case run.Delayed, run.Untriggered:
		case run.None:
			out = append(out, "-")
		default:
			out = append(out, fmt.Sprint(run.Block))
		}
	}
	// forget them
	r.e.mu.Lock()
	r.e.progs[key].runs = nil
	r.e.mu.Unlock()
	return strings.Join(out, " ")
}

func (r *prig) expect(key, want string) {
	r.t.Helper()
	if got := r.blocks(key); got != want {
		r.t.Fatalf("%s ran [%s], want [%s]", key, got, want)
	}
}

func (r *prig) dev(ref string, v any) {
	r.t.Helper()
	x, err := model.ParseRef(ref)
	if err != nil {
		r.t.Fatal(err)
	}
	r.e.DeviceEvent(x, v)
}

const temp = "HmIP-RF/000A1B2C3D4E5F:1/ACTUAL_TEMPERATURE"

// one line on x with a mode, DANN and SONST
func onX(cond, trigger string) string {
	return "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n" + cond + "              trigger: " + trigger + "\n    then:\n      - variable: out\n        to: then\n  - else:\n      - variable: out\n        to: else\n"
}

const gt10 = "              op: gt\n              value: 10\n"

// Q-2: "bei Änderung" fires when the truth of the line flips - in both directions - not when
// the value changes. 6 -> 12 over a threshold of 10 triggers, 12 -> 15 does not, 15 -> 8
// triggers and runs SONST (Q-4).
func TestChangeTriggersOnTheFlipOfTheTruth(t *testing.T) {
	r := newPrig(t)
	r.set("x", 6, FromUser)
	r.add("p", onX(gt10, "change"))
	r.expect("p", "") // saving a program runs nothing (Q-10, fixed)
	r.set("x", 12, FromUser)
	r.expect("p", "1")
	r.set("x", 15, FromUser)
	r.expect("p", "")
	r.set("x", 15, FromUser) // the same value: an update, no change (Q-37)
	r.expect("p", "")
	r.set("x", 8, FromUser)
	r.expect("p", "0")
	r.set("x", 9, FromUser)
	r.expect("p", "")
	if r.value("out") != "else" {
		t.Errorf("out: %v", r.value("out"))
	}
}

// Q-3: "bei Aktualisierung" fires on every report, the same value included; with a SONST the
// SONST runs at every report while the condition is false.
func TestUpdateTriggersOnEveryReport(t *testing.T) {
	r := newPrig(t)
	r.add("p", onX(gt10, "update"))
	for _, v := range []float64{5, 5, 12, 12, 3} {
		r.set("x", v, FromUser)
	}
	r.expect("p", "0 0 1 1 0")
}

// Q-5: "nur prüfen" never triggers; a program with only such lines never runs by itself - but
// the line counts when something else triggers the program (Q-1: the trigger mode only decides
// when the program is evaluated, it is not part of the condition).
func TestCheckOnlyNeverTriggers(t *testing.T) {
	r := newPrig(t)
	r.add("p", onX(gt10, "check"))
	r.set("x", 50, FromUser)
	r.set("x", 1, FromUser)
	r.expect("p", "")
	r.add("q", `rules:
  - when:
      all:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: check
            - variable: regen
              is: true
              trigger: change
    then: []
  - else: []
`)
	r.set("regen", true, FromUser) // x is 1: the check line is false, SONST
	r.expect("q", "0")
	r.set("x", 50, FromUser) // nur prüfen: nothing runs
	r.expect("q", "")
	r.set("regen", false, FromUser)
	r.expect("q", "0")
	r.set("regen", true, FromUser)
	r.expect("q", "1")
}

// A-4: "bei jeder Wertänderung" fires when the value differs from the one before.
func TestValueTriggersOnEveryChangeOfTheValue(t *testing.T) {
	r := newPrig(t)
	r.add("p", onX(gt10, "value"))
	for _, v := range []float64{12, 15, 15, 8, 9} {
		r.set("x", v, FromUser)
	}
	r.expect("p", "1 1 0 0")
}

// Q-1: any trigger evaluates the whole chain top-down against the current values; the first
// true block runs, later ones are not looked at; if none is true SONST runs - and a program
// without a SONST then runs nothing.
func TestTheChain(t *testing.T) {
	r := newPrig(t)
	r.add("p", `rules:
  - when:
      all:
        - all:
            - variable: x
              op: lt
              value: 5
              trigger: change
    then: []
  - when:
      all:
        - all:
            - variable: x
              op: range
              from: 5
              below: 15
              trigger: change
    then: []
  - when:
      all:
        - all:
            - variable: x
              op: ge
              value: 15
              trigger: change
    then: []
`)
	r.set("x", 3, FromUser) // x was 0: the first line stays true, the others stay false
	r.expect("p", "")
	r.set("x", 7, FromUser)
	r.expect("p", "2")
	r.set("x", 20, FromUser)
	r.expect("p", "3")
	r.set("x", 1, FromUser)
	r.expect("p", "1")
	// a program with one rule and no SONST: evaluated and nothing runs; "zuletzt" stays (Q-6)
	r.add("q", "rules:\n  - when:\n      all:\n        - all:\n            - variable: y\n              op: gt\n              value: 10\n              trigger: update\n    then: []\n")
	r.set("y", 1, FromUser)
	r.expect("q", "-")
	if in, _ := r.e.Program("q"); in.LastRun != nil {
		t.Errorf("a run in which nothing ran moved the last run: %+v", in.LastRun)
	}
	r.set("y", 11, FromUser)
	r.clock.Advance(time.Minute)
	r.set("y", 1, FromUser)
	if in, _ := r.e.Program("q"); in.LastRun == nil || !in.LastRun.Time.Equal(t0) || in.LastRun.Block != 1 || in.LastRun.Trigger != "variable:y" {
		t.Errorf("the last run: %+v", in.LastRun)
	}
}

// Two levels: groups of lines, AND inside and OR between, or the dual; and "not".
func TestGroupsAndNot(t *testing.T) {
	r := newPrig(t)
	r.add("p", `rules:
  - when:
      any:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: update
            - variable: regen
              is: true
              trigger: check
        - all:
            - variable: modus
              is: Winter
              not: true
              trigger: check
            - variable: text
              is: go
              trigger: check
    then: []
  - else: []
`)
	step := func(x float64, regen bool, modus, text, want string) {
		t.Helper()
		r.set("regen", regen, FromUser)
		r.set("modus", modus, FromUser)
		r.set("text", text, FromUser)
		r.set("x", x, FromUser)
		r.expect("p", want)
	}
	step(11, true, "Winter", "", "1")    // first group
	step(11, false, "Winter", "go", "0") // neither: not Winter is false
	step(1, false, "Sommer", "go", "1")  // second group
	step(1, false, "Sommer", "stop", "0")
	r.add("q", `rules:
  - when:
      all:
        - any:
            - variable: x
              op: gt
              value: 10
              trigger: update
            - variable: regen
              is: true
              trigger: check
        - any:
            - variable: text
              op: ne
              value: go
              trigger: check
    then: []
  - else: []
`)
	r.blocks("p")
	r.set("text", "go", FromUser)
	r.set("regen", true, FromUser)
	r.set("x", 1, FromUser)
	r.expect("q", "0") // the second group is false
	r.set("text", "x", FromUser)
	r.set("x", 1, FromUser)
	r.expect("q", "1")
	r.set("regen", false, FromUser)
	r.set("x", 1, FromUser)
	r.expect("q", "0") // the first group is false
}

// Q-7 (fixed, and as measured for two lines on one variable): triggers of one program from the
// same event are one evaluation.
func TestOneEvaluationPerEvent(t *testing.T) {
	r := newPrig(t)
	r.add("p", `rules:
  - when:
      any:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: update
        - all:
            - variable: x
              op: lt
              value: 0
              trigger: update
        - all:
            - variable: x
              is: 5
              trigger: change
    then: []
  - else: []
`)
	r.set("x", 5, FromUser)
	r.expect("p", "1")
	r.set("x", 20, FromUser)
	r.expect("p", "1")
}

// Q-37 and Q-38 (measured): on a value list and a string, "bei Änderung" fires only when the
// line starts or stops matching; from one non-matching value to another nothing runs.
func TestListsAndStringsUnderChange(t *testing.T) {
	r := newPrig(t)
	r.add("l", "rules:\n  - when:\n      all:\n        - all:\n            - variable: modus\n              is: Übergang\n              trigger: change\n    then: []\n  - else: []\n")
	for _, v := range []string{"Übergang", "Übergang", "Winter", "Winter", "Sommer", "Übergang"} {
		r.set("modus", v, FromUser)
	}
	r.expect("l", "1 0 1") // the 1st, 3rd and 6th write
	r.add("s", "rules:\n  - when:\n      all:\n        - all:\n            - variable: text\n              is: x\n              trigger: change\n    then: []\n  - else: []\n")
	for _, v := range []string{"x", "x", "y", "z", "x"} {
		r.set("text", v, FromUser)
	}
	r.expect("s", "1 0 1") // the 1st, 3rd and 5th write
}

// Q-48 (measured): a "bei Änderung" line in a SONST WENN that was not reached fires on every
// write - the same value included - while an earlier block is true; as soon as the earlier
// block is false the line behaves normally again.
func TestUnreachedChangeLineFiresOnEveryWrite(t *testing.T) {
	r := newPrig(t)
	r.add("p", `rules:
  - when:
      all:
        - all:
            - variable: regen
              is: true
              trigger: check
    then: []
  - when:
      all:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: change
    then: []
  - else: []
`)
	// regen is false: the second rule is reached, the line behaves as usual
	for _, v := range []float64{1, 2, 20, 30, 3} {
		r.set("x", v, FromUser)
	}
	r.expect("p", "2 0")
	// regen is true: the chain stops at the first block, the line below is never looked at
	r.set("regen", true, FromUser)
	r.set("x", 4, FromUser) // false stays false: nothing
	r.expect("p", "")
	r.set("x", 50, FromUser) // a flip: the program runs its first block and forgets the line
	for _, v := range []float64{50, 51, 1} {
		r.set("x", v, FromUser) // from then on every write runs it, the same value included
	}
	r.expect("p", "1 1 1 1")
	// regen false again: one more run to look at the line, then normal
	r.set("regen", false, FromUser)
	r.set("x", 51, FromUser) // once more, and now the line is looked at again
	r.set("x", 52, FromUser)
	r.set("x", 1, FromUser)
	r.expect("p", "2 0")
}

// The ranges, as measured: "von a bis kleiner b" takes a and not b, and is never true when a is
// above b; the comparisons on numbers; a bool against a number.
func TestComparisons(t *testing.T) {
	line := func(op model.Op, a, b any) *model.Line {
		l := &model.Line{Variable: "x", Op: op, Trigger: model.TriggerCheck}
		switch op {
		case model.OpRange:
			l.From, l.Below = a, b
		case model.OpOutside:
			l.From, l.To = a, b
		default:
			l.Value = a
		}
		return l
	}
	for _, c := range []struct {
		l    *model.Line
		v    any
		want bool
	}{
		{line(model.OpRange, 20.0, 50.0), 20.0, true}, {line(model.OpRange, 20.0, 50.0), 30.0, true}, {line(model.OpRange, 20.0, 50.0), 50.0, false}, {line(model.OpRange, 20.0, 50.0), 10.0, false},
		{line(model.OpRange, 50.0, 20.0), 10.0, false}, {line(model.OpRange, 50.0, 20.0), 20.0, false}, {line(model.OpRange, 50.0, 20.0), 30.0, false}, {line(model.OpRange, 50.0, 20.0), 50.0, false}, {line(model.OpRange, 50.0, 20.0), 60.0, false},
		{line(model.OpOutside, 30.0, 35.0), 29.0, true}, {line(model.OpOutside, 30.0, 35.0), 30.0, false}, {line(model.OpOutside, 30.0, 35.0), 35.0, false}, {line(model.OpOutside, 30.0, 35.0), 36.0, true},
		{line(model.OpGt, 10.0, nil), 10.0, false}, {line(model.OpGt, 10.0, nil), 10.1, true}, {line(model.OpGe, 10.0, nil), 10.0, true}, {line(model.OpGe, 10.0, nil), 9.9, false},
		{line(model.OpLt, 10.0, nil), 9.9, true}, {line(model.OpLt, 10.0, nil), 10.0, false}, {line(model.OpLe, 10.0, nil), 10.0, true}, {line(model.OpLe, 10.0, nil), 10.1, false},
		{line(model.OpEq, 3.0, nil), 3.0, true}, {line(model.OpEq, 3.0, nil), 3.5, false}, {line(model.OpNe, 3.0, nil), 3.5, true}, {line(model.OpNe, 3.0, nil), 3.0, false},
		{line(model.OpEq, true, nil), 1.0, true}, {line(model.OpEq, true, nil), 0.0, false}, {line(model.OpEq, false, nil), 0.0, true}, {line(model.OpEq, 1.0, nil), true, true},
		{line(model.OpEq, "a", nil), "a", true}, {line(model.OpEq, "a", nil), "b", false}, {line(model.OpEq, "1", nil), 1.0, false}, {line(model.OpNe, "1", nil), 1.0, true},
		{line(model.OpGt, 0.0, nil), true, true}, {line(model.OpGt, 0.0, nil), "x", false}, {line(model.Op("like"), 0.0, nil), 1.0, false}, {line("", 2.0, nil), 2.0, true},
	} {
		if got := compare(c.l, c.v); got != c.want {
			t.Errorf("%v %s %v/%v/%v/%v: %v", c.v, c.l.Op, c.l.Value, c.l.From, c.l.Below, c.l.To, got)
		}
	}
}

// Device datapoints: every report is an update, a different value a change; an unknown value
// makes the line false, and the first value a line sees is not something that happened (Q-14).
func TestDeviceLines(t *testing.T) {
	r := newPrig(t)
	r.add("c", "rules:\n  - when:\n      all:\n        - all:\n            - device: "+temp+"\n              op: gt\n              value: 21\n              trigger: change\n    then: []\n  - else: []\n")
	r.add("u", "rules:\n  - when:\n      all:\n        - all:\n            - device: "+temp+"\n              op: gt\n              value: 21\n              trigger: update\n    then: []\n  - else: []\n")
	r.add("n", "rules:\n  - when:\n      all:\n        - all:\n            - device: "+temp+"\n              op: gt\n              value: 21\n              not: true\n              trigger: check\n            - variable: regen\n              is: true\n              trigger: update\n    then: []\n  - else: []\n")
	// nothing known yet: a line on an unknown value is false, also with "not"
	r.set("regen", true, FromUser)
	r.expect("n", "0")
	if _, ok := r.e.DeviceValue(model.Ref{Interface: "HmIP-RF", Address: "000A1B2C3D4E5F:1", Param: "ACTUAL_TEMPERATURE"}); ok {
		t.Error("a value before any report")
	}
	r.dev(temp, 22.5) // the first report: the change line learns where it stands
	r.expect("c", "")
	r.expect("u", "1")
	r.dev(temp, 22.5)
	r.expect("c", "")
	r.expect("u", "1")
	r.dev(temp, 20)
	r.expect("c", "0")
	r.expect("u", "0")
	r.dev(temp, 23) // an int is a number
	r.expect("c", "1")
	r.set("regen", true, FromUser)
	r.expect("n", "0") // 23 > 21, not: false
	r.dev(temp, 20.0)
	r.set("regen", true, FromUser)
	r.expect("n", "1")
	if v, _ := r.e.DeviceValue(model.Ref{Interface: "HmIP-RF", Address: "000A1B2C3D4E5F:1", Param: "ACTUAL_TEMPERATURE"}); v != 20.0 {
		t.Errorf("the device value: %v", v)
	}
	want := []model.Ref{{Interface: "HmIP-RF", Address: "000A1B2C3D4E5F:1", Param: "ACTUAL_TEMPERATURE"}}
	if got := r.e.Refs(); !reflect.DeepEqual(got, want) {
		t.Errorf("refs: %v", got)
	}
}

// A seed (the state store at the start, a resync) is never an event, and a line that was loaded
// before its value was known takes its truth from it silently.
func TestSeed(t *testing.T) {
	r := newPrig(t)
	ref := model.Ref{Interface: "HmIP-RF", Address: "000A1B2C3D4E5F:1", Param: "ACTUAL_TEMPERATURE"}
	r.add("c", "rules:\n  - when:\n      all:\n        - all:\n            - device: "+temp+"\n              op: gt\n              value: 21\n              trigger: change\n    then: []\n  - else: []\n")
	r.e.SeedDevice(ref, 25)
	r.expect("c", "")
	r.dev(temp, 25.0) // the same value as the seed: no flip
	r.expect("c", "")
	r.dev(temp, 20.0)
	r.expect("c", "0")
	r.e.SeedDevice(ref, 30.0) // a resync that moved the value: still no event
	r.expect("c", "")
	r.dev(temp, 30.0) // the report after it flips against what the program last saw
	r.expect("c", "1")
}

// Activities: in their listed order (Q-23, fixed); a value copied from a datapoint or a
// variable; a special value and a script are skipped; device writes go to the writer.
func TestActivities(t *testing.T) {
	r := newPrig(t)
	r.dev(temp, 19.5)
	r.set("y", 7, FromUser)
	r.add("p", `active: false
rules:
  - when:
      all: []
    then:
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        to: 0.5
      - variable: out
        to: eins
      - set: BidCos-RF/NEQ0123456:1/STATE
        to: true
      - variable: x
        from_variable: y
      - variable: out
        to: zwei
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        from: `+temp+`
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        from_variable: alarm-messages
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        special: OLD_LEVEL
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        from: HmIP-RF/00111BE9AB9012:9/UNKNOWN
      - variable: y
        from: HmIP-RF/00111BE9AB9012:9/UNKNOWN
      - script: WriteLine("x");
      - variable: modus
        to: Winter
`)
	if err := r.e.RunProgramBy("p", model.RunCheck, "user"); err != nil {
		t.Fatal(err)
	}
	wantWrites := []write{{"HmIP-RF/00111BE9AB9012:4/LEVEL", 0.5}, {"BidCos-RF/NEQ0123456:1/STATE", true}, {"HmIP-RF/00111BE9AB9012:4/LEVEL", 19.5}, {"HmIP-RF/00111BE9AB9012:4/LEVEL", 0.0}}
	if !reflect.DeepEqual(r.w.writes, wantWrites) {
		t.Errorf("writes: %+v", r.w.writes)
	}
	if r.value("out") != "zwei" || r.value("x") != 7.0 || r.value("modus") != "Winter" {
		t.Errorf("variables: out %v x %v modus %v", r.value("out"), r.value("x"), r.value("modus"))
	}
	runs := r.e.Runs("p")
	notes := strings.Join(runs[0].Notes(), "\n")
	for _, want := range []string{"1: set HmIP-RF/00111BE9AB9012:4/LEVEL = 0.5", "2: variable out = eins", "8: set HmIP-RF/00111BE9AB9012:4/LEVEL: skipped, the special value OLD_LEVEL",
		"9: set HmIP-RF/00111BE9AB9012:4/LEVEL: skipped, the value of HmIP-RF/00111BE9AB9012:9/UNKNOWN is not known", "10: variable y: skipped", "11: script: skipped, scripts are never run", "12: variable modus = Winter"} {
		if !strings.Contains(notes, want) {
			t.Errorf("no note %q in\n%s", want, notes)
		}
	}
	if runs[0].Trigger != "user" || runs[0].Block != 1 {
		t.Errorf("run: %+v", runs[0])
	}
	// the system refuses a write: counted on the program, never retried
	r.w.done[0](nil)
	r.w.done[1](errors.New("-1 Failure"))
	if in, _ := r.e.Program("p"); in.Failures != 1 || !strings.Contains(in.LastFailure, "BidCos-RF/NEQ0123456:1/STATE: -1 Failure") || len(r.logs) == 0 {
		t.Errorf("failures: %+v", in)
	}
	// a write that fits no variable is a note, not a crash; without a writer a device write is dropped
	r.add("bad", "rules:\n  - when:\n      all: []\n    then:\n      - variable: modus\n        from_variable: y\n")
	_ = r.e.RunProgramBy("bad", model.RunThen, "user")
	if n := r.e.Runs("bad")[0].Notes()[0]; !strings.Contains(n, "takes one of its entries") {
		t.Errorf("note: %s", n)
	}
	e := New(Options{Clock: r.clock})
	only, err := model.DecodeProgram("p.yaml", "p", []byte("name: p\nrules:\n  - else:\n      - set: HmIP-RF/00111BE9AB9012:4/LEVEL\n        to: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	e.SetPrograms(map[string]*model.Program{"p": only})
	if err := e.RunProgramBy("p", model.RunThen, "user"); err != nil {
		t.Fatal(err)
	}
	if n := e.Runs("p")[0].Notes()[0]; !strings.Contains(n, "dropped") {
		t.Errorf("note: %s", n)
	}
	// a failure reported for a program that is gone
	r.e.SetPrograms(map[string]*model.Program{})
	r.w.done[2](errors.New("late"))
}

const delayed = `rules:
  - when:
      all:
        - all:
            - variable: regen
              is: true
              trigger: change
    %s
    then:
      - variable: out
        to: sofort
      - variable: x
        to: 1
        after: 30s
      - set: HmIP-RF/00111BE9AB9012:4/LEVEL
        to: 0
        after: 1min
  - else:
      - variable: x
        to: 0
    %s
`

// Q-15, Q-16, Q-18: a delay counts from the trigger; without the checkbox a pending delayed
// activity stays when the program triggers again - two triggers, two executions; and it is not
// checked again when it is due.
func TestDelaysWithoutCancel(t *testing.T) {
	r := newPrig(t)
	r.add("p", fmt.Sprintf(delayed, "", ""))
	r.set("regen", true, FromUser)
	if r.value("out") != "sofort" || r.value("x") != 0.0 || len(r.e.PendingActions()) != 2 {
		t.Fatalf("after the trigger: out %v x %v, %d pending", r.value("out"), r.value("x"), len(r.e.PendingActions()))
	}
	r.clock.Advance(10 * time.Second)
	r.set("regen", false, FromUser) // SONST: x = 0; the condition is false now
	r.set("regen", true, FromUser)  // triggered again: two more pending
	pend := r.e.PendingActions()
	if len(pend) != 4 || !pend[0].Due.Equal(t0.Add(30*time.Second)) || !pend[1].Due.Equal(t0.Add(40*time.Second)) || pend[0].Program != "p" || pend[0].By != "variable:regen" {
		t.Fatalf("pending: %+v", pend)
	}
	r.set("regen", false, FromUser)
	r.clock.Advance(20 * time.Second) // 30 s after the first trigger: it fires although the condition is false
	if r.value("x") != 1.0 {
		t.Fatalf("the delayed write did not happen: %v", r.value("x"))
	}
	r.set("x", 5, FromUser)
	r.clock.Advance(10 * time.Second) // the second one
	if r.value("x") != 1.0 {
		t.Fatalf("the second delayed write did not happen: %v", r.value("x"))
	}
	r.clock.Advance(time.Minute)
	if len(r.w.writes) != 2 || len(r.e.PendingActions()) != 0 || r.clock.Pending() != 0 {
		t.Errorf("device writes %d, pending %d", len(r.w.writes), len(r.e.PendingActions()))
	}
	runs := r.e.Runs("p")
	if last := runs[len(runs)-1]; !last.Delayed || !strings.Contains(last.Notes()[0], "delayed: set HmIP-RF/00111BE9AB9012:4/LEVEL") {
		t.Errorf("the delayed activity's entry: %+v", last)
	}
}

// Q-17: the checkbox cancels the pending activities for the targets named in that block -
// whoever scheduled them, also another program; a block that names another target cancels
// nothing. That is why an immediate opposite command in SONST cancels DANN's delay.
func TestCancelPendingAcrossPrograms(t *testing.T) {
	r := newPrig(t)
	r.add("p", fmt.Sprintf(delayed, "", "cancel_pending: true"))
	r.set("regen", true, FromUser)
	r.clock.Advance(10 * time.Second)
	r.set("regen", false, FromUser) // SONST names x: the delayed x = 1 goes, the LEVEL write stays
	pend := r.e.PendingActions()
	if len(pend) != 1 || pend[0].Action.Kind != model.ActionSet {
		t.Fatalf("pending: %+v", pend)
	}
	if !strings.Contains(strings.Join(r.e.Runs("p")[1].Notes(), " "), "cancelled 1 pending") {
		t.Errorf("notes: %v", r.e.Runs("p")[1].Notes())
	}
	// another program whose block names the datapoint cancels p's delayed write
	r.add("q", "rules:\n  - when:\n      all:\n        - all:\n            - variable: y\n              op: gt\n              value: 0\n              trigger: change\n    cancel_pending: true\n    then:\n      - set: HmIP-RF/00111BE9AB9012:4/LEVEL\n        to: 1\n")
	r.set("y", 1, FromUser)
	if len(r.e.PendingActions()) != 0 {
		t.Fatalf("pending: %+v", r.e.PendingActions())
	}
	r.clock.Advance(time.Hour)
	if len(r.w.writes) != 1 || r.w.writes[0].value != 1.0 || r.value("x") != 0.0 {
		t.Errorf("writes %+v, x %v", r.w.writes, r.value("x"))
	}
	// a block with the checkbox that names other targets cancels nothing
	r.add("other", "rules:\n  - when:\n      all:\n        - all:\n            - variable: text\n              is: go\n              trigger: change\n    cancel_pending: true\n    then:\n      - variable: out\n        to: x\n")
	r.set("regen", true, FromUser)
	r.set("text", "go", FromUser)
	if len(r.e.PendingActions()) != 2 {
		t.Errorf("pending: %+v", r.e.PendingActions())
	}
}

// Q-19: with the checkbox on and a trigger at every report the delay starts again each time and
// never fires while the reports keep coming. Q-20: switching a program off does not cancel its
// pending activities; removing it does.
func TestDelayRestartsAndSurvivesTheSwitch(t *testing.T) {
	r := newPrig(t)
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: gt\n              value: 0\n              trigger: update\n    cancel_pending: true\n    then:\n      - variable: out\n        to: fertig\n        after: 3min\n")
	for i := 0; i < 5; i++ {
		r.set("x", 1, FromUser)
		r.clock.Advance(2 * time.Minute)
	}
	if r.value("out") != "" || len(r.e.PendingActions()) != 1 {
		t.Fatalf("out %v, pending %d", r.value("out"), len(r.e.PendingActions()))
	}
	off := r.progs["p"].Clone()
	off.Active = false
	r.progs["p"] = off
	r.e.SetPrograms(r.progs)
	r.set("x", 1, FromUser) // inactive: no trigger
	r.clock.Advance(time.Minute)
	if r.value("out") != "fertig" {
		t.Fatalf("switching the program off cancelled its delay: %v", r.value("out"))
	}
	on := off.Clone()
	on.Active = true
	r.progs["p"] = on
	r.e.SetPrograms(r.progs)
	r.set("x", 1, FromUser)
	id := r.e.PendingActions()[0].ID
	if !r.e.CancelPending(id) || r.e.CancelPending(id) || len(r.e.PendingActions()) != 0 {
		t.Error("CancelPending")
	}
	r.set("x", 1, FromUser)
	delete(r.progs, "p")
	r.e.SetPrograms(r.progs)
	if len(r.e.PendingActions()) != 0 || r.clock.Pending() != 0 {
		t.Errorf("a removed program's delay stays: %+v", r.e.PendingActions())
	}
}

// Program activities, and the manual start: "then" runs the first block without looking at the
// conditions (Q-8, Q-9) - also of an inactive program -, "check" evaluates it. Activating and
// deactivating is told to the listener, which stores it.
func TestProgramActivitiesAndManualRuns(t *testing.T) {
	r := newPrig(t)
	r.add("ziel", "active: false\n"+onX(gt10, "change"))
	r.add("p", `rules:
  - when:
      all:
        - all:
            - variable: regen
              is: true
              trigger: change
    then:
      - run: ziel
        mode: then
      - run: ziel
        mode: check
      - activate: ziel
      - deactivate: ziel
      - run: ziel
        mode: then
        after: 10s
`)
	r.set("x", 1, FromUser)
	r.notes = nil
	r.set("regen", true, FromUser)
	r.expect("ziel", "1 0") // then: the first block; check: x is 1, SONST
	var switched []bool
	for _, n := range r.notes {
		if n.Kind == "program" && n.Active != nil && n.Key == "ziel" {
			switched = append(switched, *n.Active)
		}
	}
	if !reflect.DeepEqual(switched, []bool{true, false}) {
		t.Errorf("switches: %v (%+v)", switched, r.notes)
	}
	if run := r.e.Runs("p")[0]; run.Block != 1 || !strings.Contains(strings.Join(run.Notes(), "\n"), "1: run ziel (then)") {
		t.Errorf("run: %+v", run)
	}
	r.clock.Advance(10 * time.Second)
	r.expect("ziel", "1")
	if err := r.e.RunProgram("ziel", model.RunThen); err != nil {
		t.Fatal(err)
	}
	if r.e.LastRun("ziel") != 1 || r.e.LastRun("nothing") != 0 || r.e.Runs("ziel")[len(r.e.Runs("ziel"))-1].Trigger != "outside" {
		t.Errorf("LastRun: %d", r.e.LastRun("ziel"))
	}
	if err := r.e.RunProgramBy("ziel", model.RunCheck, "user"); err != nil {
		t.Fatal(err)
	}
	if in, _ := r.e.Program("ziel"); in.LastRun == nil || in.LastRun.Trigger != "user" || in.LastRun.Block != 0 {
		t.Errorf("last run: %+v", in.LastRun)
	}
	r.expect("ziel", "1 0")
	// the notification of a run carries the block
	last := r.notes[len(r.notes)-1]
	if last.Kind != "program" || last.Key != "ziel" || last.Block != 0 || last.Active != nil {
		t.Errorf("notification: %+v", last)
	}
	for _, c := range []struct{ key, mode, code string }{{"nothing", model.RunThen, "unknown-program"}, {"ziel", "fast", "invalid"}} {
		err := r.e.RunProgramBy(c.key, c.mode, "user")
		var we *WriteError
		if !errors.As(err, &we) || we.Code != c.code {
			t.Errorf("run %s %s: %v", c.key, c.mode, err)
		}
	}
	// a program without rules, started by hand: nothing to run
	r.add("leer", "rules: []\n")
	_ = r.e.RunProgramBy("leer", model.RunThen, "user")
	r.expect("leer", "-")
	// a program that is removed while an activity names it
	r.progs["p"].Rules[0].Then = r.progs["p"].Rules[0].Then[:3]
	delete(r.progs, "ziel")
	r.e.SetPrograms(r.progs)
	r.set("regen", false, FromUser)
	r.set("regen", true, FromUser)
	if in, _ := r.e.Program("p"); len(in.Problems) == 0 {
		t.Error("a program whose target is gone has no problem")
	}
	if err := r.e.RunProgramBy("p", model.RunThen, "user"); err == nil {
		t.Error("a program with a problem ran")
	}
	if got := r.blocks("p"); got != "1" {
		t.Errorf("a program with a problem was triggered: %s", got)
	}
}

// An inactive program does not react; switched on, it starts with the truth its lines have then
// and does not run (OQ-8: no run on activation or save).
func TestInactiveAndActivation(t *testing.T) {
	r := newPrig(t)
	r.add("p", "active: false\n"+onX(gt10, "change"))
	r.set("x", 20, FromUser)
	r.expect("p", "")
	on := r.progs["p"].Clone()
	on.Active = true
	r.progs["p"] = on
	r.e.SetPrograms(r.progs)
	r.expect("p", "")
	r.set("x", 21, FromUser) // true stays true: nothing
	r.expect("p", "")
	r.set("x", 1, FromUser)
	r.expect("p", "0")
	// an edit that leaves the conditions alone keeps what the lines remember; one that changes
	// them starts them anew, silently
	renamed := on.Clone()
	renamed.Name, renamed.Rules[0].Then = "anders", nil
	r.progs["p"] = renamed
	r.e.SetPrograms(r.progs)
	r.set("x", 2, FromUser)
	r.expect("p", "")
	edited := renamed.Clone()
	edited.Rules[0].When.Groups[0].Lines[0].Value = 1.0
	r.progs["p"] = edited
	r.e.SetPrograms(r.progs)
	r.expect("p", "")
	r.set("x", 3, FromUser) // 3 > 1 was true at the save already
	r.expect("p", "")
	r.set("x", 0, FromUser)
	r.expect("p", "0")
}

// What the engine says about its programs.
func TestProgramStates(t *testing.T) {
	r := newPrig(t)
	r.add("p", "rules:\n  - when:\n      all:\n        - all:\n            - variable: regen\n              is: true\n              trigger: change\n    then: []\n  - else: []\n")
	if st := r.e.ProgramStates(); len(st) != 1 || st[0].Key != "p" {
		t.Errorf("states: %+v", st)
	}
	if _, ok := r.e.Program("nothing"); ok || r.e.Runs("nothing") != nil {
		t.Error("a program that is not there")
	}
}

// Q-26 (fixed): a program that writes what it triggers on runs in a loop on the CCU; here it is
// paused after 20 runs in 10 s, for a minute, and the listener is told.
func TestLoopGuard(t *testing.T) {
	r := newPrig(t)
	r.add("loop", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: ge\n              value: 0\n              trigger: update\n    then:\n      - variable: x\n        to: 1\n")
	r.set("x", 1, FromUser)
	runs := r.e.Runs("loop")
	if len(runs) != 21 || !runs[20].None || !strings.Contains(runs[20].Notes()[0], "paused") {
		t.Fatalf("%d runs, last %+v", len(runs), runs[len(runs)-1])
	}
	in, _ := r.e.Program("loop")
	if !in.PausedUntil.Equal(t0.Add(time.Minute)) || len(r.logs) == 0 {
		t.Errorf("paused until %v", in.PausedUntil)
	}
	paused := 0
	for _, n := range r.notes {
		if n.Kind == "program" && n.Paused {
			paused++
		}
	}
	if paused != 1 {
		t.Errorf("%d pause notifications", paused)
	}
	r.blocks("loop")
	r.set("x", 1, FromUser) // paused: nothing
	r.expect("loop", "")
	r.clock.Advance(61 * time.Second)
	if in, _ = r.e.Program("loop"); !in.PausedUntil.IsZero() {
		t.Error("still paused")
	}
	// runs spread over more than the window are no loop
	r.add("ok", "rules:\n  - when:\n      all:\n        - all:\n            - variable: y\n              op: ge\n              value: 0\n              trigger: update\n    then: []\n")
	for i := 0; i < RunsKept+10; i++ {
		r.set("y", 1, FromUser)
		r.clock.Advance(time.Second)
	}
	if in, _ = r.e.Program("ok"); !in.PausedUntil.IsZero() {
		t.Error("a program that runs once a second was paused")
	}
	if len(r.e.Runs("ok")) != RunsKept {
		t.Errorf("%d runs kept", len(r.e.Runs("ok")))
	}
	// the thresholds are settings
	e := New(Options{Clock: r.clock, LoopRuns: 2, LoopWindow: time.Second, LoopPause: time.Hour})
	e.SetVariables(map[string]*model.Variable{"x": {Key: "x", Name: "X", Type: model.VarNumber}})
	e.SetPrograms(map[string]*model.Program{"loop": r.progs["loop"]})
	_ = e.SetVariable("x", 1, FromUser)
	if in, _ := e.Program("loop"); !in.PausedUntil.Equal(r.clock.Now().Add(time.Hour)) || len(e.Runs("loop")) != 3 {
		t.Errorf("with settings: %+v, %d runs", in, len(e.Runs("loop")))
	}
}

// What the state file keeps for the programs: the truth each line had (so the first write of
// an unchanged value after a restart does not trigger, Q-14) and the delayed activities - one
// still in the future is armed again, one that came due while the addon was stopped is dropped
// (Q-21, fixed: the CCU loses them all).
func TestProgramStateSurvivesARestart(t *testing.T) {
	r := newPrig(t)
	r.add("p", fmt.Sprintf(delayed, "", ""))
	r.add("q", onX(gt10, "change"))
	r.set("x", 50, FromUser)
	r.blocks("q")
	r.set("regen", true, FromUser) // out = sofort; x = 1 in 30 s; LEVEL in 1 min
	st, _ := r.e.Export()
	b, err := MarshalState(st)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalState(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Pending) != 2 || len(back.Programs) != 2 || !reflect.DeepEqual(back.Programs["q"].Truth, []int8{1}) {
		t.Fatalf("state: %+v", back)
	}
	// a restart 45 s later: the first delay is overdue, the second still to come
	r2 := newPrig(t)
	r2.clock.Set(t0.Add(45 * time.Second))
	r2.progs = r.progs
	r2.e.SetPrograms(r.progs)
	back.Programs["gone"] = ProgramState{Conditions: "x", Truth: []int8{1}}
	back.Pending = append(back.Pending, Pending{Due: t0.Add(time.Hour), Program: "gone"})
	r2.e.Import(back)
	if got := r2.e.PendingActions(); len(got) != 1 || got[0].Action.Kind != model.ActionSet || len(r2.logs) != 1 || !strings.Contains(r2.logs[0], "dropped") {
		t.Fatalf("pending after the restart: %+v, %v", got, r2.logs)
	}
	r2.clock.Advance(15 * time.Second)
	if len(r2.w.writes) != 1 || r2.value("x") != 50.0 {
		t.Errorf("the delay that survived: %+v, x %v", r2.w.writes, r2.value("x"))
	}
	// q's line was true before the restart: the same value written again is no flip
	r2.set("x", 60, FromUser)
	r2.expect("q", "")
	r2.set("x", 1, FromUser)
	r2.expect("q", "0")
	// a state written for other conditions is not applied: the lines start with what is now
	r3 := newPrig(t)
	r3.set("x", 1, FromUser)
	r3.add("q", onX("              op: gt\n              value: 0\n", "change"))
	r3.e.Import(back)
	r3.set("x", 2, FromUser)
	r3.expect("q", "")
	// a line the program had not looked at keeps the truth it has at the start
	r4 := newPrig(t)
	r4.add("q", onX(gt10, "change"))
	back.Programs["q"] = ProgramState{Conditions: fingerprint(r4.progs["q"]), Truth: []int8{-1}}
	r4.e.Import(back)
	r4.set("x", 1, FromUser)
	r4.expect("q", "")
}

// A delayed activity can be carried out before it is due: it runs once, is gone from the
// registry with its timer, and the program's runs note it like one that came due.
func TestRunPending(t *testing.T) {
	r := newPrig(t)
	r.add("p", fmt.Sprintf(delayed, "", ""))
	r.set("regen", true, FromUser)
	pend := r.e.PendingActions()
	if len(pend) == 0 || pend[0].Set.IsZero() || !pend[0].Due.After(pend[0].Set) {
		t.Fatalf("pending: %+v", pend)
	}
	n := len(pend)
	timers := r.clock.Pending()
	if !r.e.RunPending(pend[0].ID) || r.e.RunPending(pend[0].ID) || r.e.RunPending(987654) {
		t.Fatal("RunPending: once, and only what is there")
	}
	if len(r.e.PendingActions()) != n-1 || r.clock.Pending() != timers-1 {
		t.Errorf("pending %d (was %d), timers %d (was %d)", len(r.e.PendingActions()), n, r.clock.Pending(), timers)
	}
	runs := r.e.Runs("p")
	if last := runs[len(runs)-1]; !last.Delayed || !strings.HasPrefix(last.Notes()[0], "delayed: ") {
		t.Errorf("the run's note: %+v", last)
	}
	// when its time comes nothing happens a second time
	before := len(r.e.Runs("p"))
	r.clock.Advance(time.Hour)
	if got := len(r.e.Runs("p")); got != before+n-1 {
		t.Errorf("runs after the delays: %d, want %d", got, before+n-1)
	}
}

// What the engine knows of a datapoint: a seeded value has no report; reports are timed, and
// from the third on they say how often the datapoint reports.
func TestDeviceStates(t *testing.T) {
	r := newPrig(t)
	a, b := model.Ref{Interface: "HmIP-RF", Address: "A:1", Param: "TEMP"}, model.Ref{Interface: "HmIP-RF", Address: "B:1", Param: "STATE"}
	r.e.SeedDevice(b, true)
	r.e.DeviceEvent(a, 20.5)
	r.clock.Advance(3 * time.Minute)
	r.e.DeviceEvent(a, 21)
	st := r.e.DeviceStates()
	if len(st) != 2 || st[b.String()].Value != true || !st[b.String()].Time.IsZero() || st[b.String()].EverySeconds != 0 {
		t.Errorf("seeded: %+v", st)
	}
	if got := st[a.String()]; got.Value != 21.0 || !got.Time.Equal(r.clock.Now()) || got.EverySeconds != 0 {
		t.Errorf("two reports: %+v", got)
	}
	r.clock.Advance(5 * time.Minute)
	r.e.DeviceEvent(a, 21)
	if got := r.e.DeviceStates()[a.String()]; got.EverySeconds != 240 {
		t.Errorf("three reports in eight minutes: %+v", got)
	}
}
