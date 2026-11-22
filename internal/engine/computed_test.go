package engine

import (
	"reflect"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const hum = "HmIP-RF/000A1B2C3D4E5F:1/HUMIDITY"

func input(s string) model.Input {
	if r, err := model.ParseRef(s); err == nil {
		return model.Input{Device: &r}
	}
	return model.Input{Variable: s}
}

func computedVar(key, fn string, inputs ...string) *model.Variable {
	v := &model.Variable{Key: key, Name: key, Type: model.VarNumber, Computed: &model.Computed{Function: fn}}
	for _, in := range inputs {
		v.Computed.Inputs = append(v.Computed.Inputs, input(in))
	}
	return v
}

// newCrig is a rig with computed values: the dew point of a device, a difference of it and a
// variable, and a maximum of two variables.
func newCrig(t *testing.T) (*prig, map[string]*model.Variable) {
	r := newPrig(t)
	d := map[string]*model.Variable{}
	for _, in := range r.e.Variables() {
		d[in.Def.Key] = in.Def
	}
	d["tau"] = computedVar("tau", model.FnDewPoint, temp, hum)
	d["diff"] = computedVar("diff", model.FnDifference, "tau", "x")
	d["top"] = computedVar("top", model.FnMax, "x", "y")
	r.e.SetVariables(d)
	return r, d
}

func (r *prig) value(key string) any {
	r.t.Helper()
	v, ok := r.e.Value(key)
	if !ok {
		r.t.Fatalf("no variable %s", key)
	}
	return v
}

// A computed value follows its inputs: nothing while one is unknown, then with every report,
// and a value computed from it follows in turn. Its datapoints are among those the daemon asks for.
func TestComputedFollowsItsInputs(t *testing.T) {
	r, _ := newCrig(t)
	refs := []string{}
	for _, ref := range r.e.Refs() {
		refs = append(refs, ref.String())
	}
	if !reflect.DeepEqual(refs, []string{temp, hum}) {
		t.Errorf("refs: %v", refs)
	}
	if v := r.value("tau"); v != 0.0 {
		t.Errorf("before anything: %v", v)
	}
	r.dev(temp, 20.0)
	if v := r.value("tau"); v != 0.0 {
		t.Errorf("with one input: %v", v)
	}
	r.dev(hum, 50) // an integer, as the interface processes send whole numbers
	if v := r.value("tau"); v != 9.3 {
		t.Errorf("dew point: %v", v)
	}
	if v := r.value("diff"); v != 9.3 {
		t.Errorf("the difference follows: %v", v)
	}
	r.set("x", 4.3, FromUser)
	if v := r.value("diff"); v != 5.0 {
		t.Errorf("difference after x: %v", v)
	}
	if v := r.value("top"); v != 4.3 {
		t.Errorf("max: %v", v)
	}
	r.set("y", 7, FromRPC)
	if v := r.value("top"); v != 7.0 {
		t.Errorf("max after y: %v", v)
	}
	// a humidity of nothing cannot be computed: the value stays
	r.dev(hum, 0.0)
	if v := r.value("tau"); v != 9.3 {
		t.Errorf("after a humidity of 0: %v", v)
	}
	// an input that is no number is no input
	r.dev(hum, true)
	if v := r.value("tau"); v != 9.3 {
		t.Errorf("after a humidity that is no number: %v", v)
	}
	in, _ := r.e.Variable("tau")
	if in.Time.IsZero() || in.Changed.IsZero() {
		t.Errorf("times: %+v", in)
	}
}

// A computed value triggers programs like any variable: an update with every report of an
// input, a change when its value differs - and the program that is triggered by the input
// itself already sees the new computed value.
func TestComputedTriggersPrograms(t *testing.T) {
	r, _ := newCrig(t)
	line := func(subject, trigger string) string {
		return "rules:\n  - when:\n      all:\n        - all:\n            - " + subject + "\n              op: gt\n              value: 10\n              trigger: " + trigger + "\n    then:\n      - variable: out\n        to: then\n  - else:\n      - variable: out\n        to: else\n"
	}
	r.add("onchange", line("variable: tau", "change"))
	r.add("onupdate", line("variable: tau", "update"))
	r.dev(temp, 20.0)
	r.dev(hum, 50.0) // 9.3: below 10
	r.expect("onchange", "")
	r.expect("onupdate", "0")
	r.dev(hum, 60.0) // 12.0
	r.expect("onchange", "1")
	r.expect("onupdate", "1")
	r.dev(hum, 60.0) // the same again: an update, no change
	r.expect("onchange", "")
	r.expect("onupdate", "1")
	r.dev(temp, 20.0) // the other input reports: an update as well
	r.expect("onupdate", "1")
	r.dev(hum, 40.0) // 6.0
	r.expect("onchange", "0")

	// a program on the input that also looks at the computed value sees the new one
	r.add("both", "rules:\n  - when:\n      all:\n        - all:\n            - device: "+hum+"\n              op: gt\n              value: 0\n              trigger: update\n            - variable: tau\n              op: gt\n              value: 10\n              trigger: check\n    then:\n      - variable: out\n        to: then\n  - else:\n      - variable: out\n        to: else\n")
	r.dev(hum, 70.0) // 14.4 now
	r.expect("both", "1")
	r.dev(hum, 30.0) // 1.9 now
	r.expect("both", "0")
}

// Nobody writes a computed value: not a person, not the interface, not a program, not an import.
func TestComputedIsReadOnly(t *testing.T) {
	r, _ := newCrig(t)
	for _, src := range []Source{FromUser, FromRPC, FromProgram, FromImport} {
		err := r.e.SetVariable("tau", 5.0, src)
		if we, ok := err.(*WriteError); !ok || we.Code != "read-only" || we.Msg != "tau is a computed value and cannot be written" {
			t.Errorf("source %v: %v", src, err)
		}
	}
	r.add("writer", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: gt\n              value: 10\n              trigger: change\n    then:\n      - variable: tau\n        to: 99\n")
	r.set("x", 12, FromUser)
	if v := r.value("tau"); v != 0.0 {
		t.Errorf("after a program's write: %v", v)
	}
}

// Values that are told without being events - the system's state at a start, an import - are
// taken into the computed values without events, through a chain of them; and new definitions
// are calculated from what is known.
func TestComputedSilently(t *testing.T) {
	r, d := newCrig(t)
	r.add("watch", "rules:\n  - when:\n      all:\n        - all:\n            - variable: diff\n              op: gt\n              value: 0\n              trigger: update\n    then:\n      - variable: out\n        to: then\n")
	tRef, _ := model.ParseRef(temp)
	hRef, _ := model.ParseRef(hum)
	r.e.SeedDevice(tRef, 25.0)
	r.e.SeedDevice(hRef, 60)
	if r.value("tau") != 16.7 || r.value("diff") != 16.7 {
		t.Errorf("seeded: tau %v diff %v", r.value("tau"), r.value("diff"))
	}
	if err := r.e.SetVariable("x", 6.7, FromImport); err != nil {
		t.Fatal(err)
	}
	if r.value("diff") != 10.0 || r.value("top") != 6.7 {
		t.Errorf("imported: diff %v top %v", r.value("diff"), r.value("top"))
	}
	r.expect("watch", "")
	// the same seed again changes nothing
	r.e.SeedDevice(hRef, 60)
	// a new computed value is there at once; one whose function changed is calculated anew
	d["sum"] = computedVar("sum", model.FnSum, "tau", "diff")
	d["top"] = computedVar("top", model.FnMin, "x", "y")
	r.e.SetVariables(d)
	if r.value("sum") != 26.7 || r.value("top") != 0.0 {
		t.Errorf("new definitions: sum %v top %v", r.value("sum"), r.value("top"))
	}
	r.expect("watch", "")
	// a variable that stops being computed keeps its value and can be written again
	d["sum"] = &model.Variable{Key: "sum", Name: "sum", Type: model.VarNumber}
	r.e.SetVariables(d)
	if r.value("sum") != 26.7 {
		t.Errorf("no longer computed: %v", r.value("sum"))
	}
	r.set("sum", 1, FromUser)
}

// Inputs in a circle (files edited by hand; the page refuses them) end: nothing hangs.
func TestComputedCircle(t *testing.T) {
	r, d := newCrig(t)
	d["one"] = computedVar("one", model.FnSum, "two", "x")
	d["two"] = computedVar("two", model.FnSum, "one", "x")
	r.e.SetVariables(d)
	d["lost"] = computedVar("lost", model.FnSum, "nope", "x")
	r.e.SetVariables(d)
	if err := r.e.SetVariable("x", 1.0, FromImport); err != nil {
		t.Fatal(err)
	}
	r.set("x", 2, FromUser)
	if r.value("one") != 0.0 || r.value("two") != 0.0 || r.value("lost") != 0.0 {
		t.Errorf("one %v two %v lost %v", r.value("one"), r.value("two"), r.value("lost"))
	}
	// the sound ones beside them still follow
	if r.value("top") != 2.0 {
		t.Errorf("top %v", r.value("top"))
	}
}
