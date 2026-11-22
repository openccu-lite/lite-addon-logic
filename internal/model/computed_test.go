package model

import (
	"reflect"
	"strings"
	"testing"
)

const dewPoint = `name: Taupunkt Bad
type: number
unit: °C
computed:
  function: dew-point
  inputs:
    - device: HmIP-RF/000A:1/ACTUAL_TEMPERATURE
    - variable: feuchte
`

// A computed value is written the one canonical way and read back as it was.
func TestComputedRoundTrip(t *testing.T) {
	v, err := DecodeVariable("variables/taupunkt-bad.yaml", "taupunkt-bad", []byte(dewPoint))
	if err != nil {
		t.Fatal(err)
	}
	want := &Computed{Function: FnDewPoint, Inputs: []Input{{Device: &Ref{Interface: "HmIP-RF", Address: "000A:1", Param: "ACTUAL_TEMPERATURE"}}, {Variable: "feuchte"}}}
	if !reflect.DeepEqual(v.Computed, want) {
		t.Fatalf("computed: %+v", v.Computed)
	}
	if got := string(EncodeVariable(v)); got != dewPoint {
		t.Errorf("written:\n%s", got)
	}
	if v.Computed.Inputs[0].Source() != "dp:HmIP-RF/000A:1/ACTUAL_TEMPERATURE" || v.Computed.Inputs[1].Source() != "var:feuchte" {
		t.Errorf("sources: %v %v", v.Computed.Inputs[0].Source(), v.Computed.Inputs[1].Source())
	}
	// a copy shares nothing
	c := v.Clone()
	c.Computed.Inputs[0].Device.Param = "X"
	c.Computed.Inputs[1].Variable = "y"
	if !reflect.DeepEqual(v.Computed, want) {
		t.Errorf("the copy changed the original: %+v", v.Computed)
	}
	if checked, err := v.Check(); err != nil || !reflect.DeepEqual(checked.Computed, want) {
		t.Errorf("check: %v %+v", err, checked)
	}
}

// Every refusal of a computed value names its line.
func TestComputedErrors(t *testing.T) {
	head := "name: C\ntype: number\n"
	two := "  inputs:\n    - variable: a\n    - variable: b\n"
	for _, c := range []struct {
		name, yaml string
		line       int
		msg        string
	}{
		{"not a mapping", head + "computed: sum\n", 3, "must be a mapping"},
		{"no function", head + "computed:\n" + two, 4, "needs a \"function\""},
		{"unknown function", head + "computed:\n  function: median\n" + two, 4, "not one of"},
		{"no inputs", head + "computed:\n  function: sum\n", 4, "takes 2 to 8 inputs"},
		{"one input", head + "computed:\n  function: dew-point\n  inputs:\n    - variable: a\n", 4, "takes exactly 2 inputs"},
		{"three for a difference", head + "computed:\n  function: difference\n" + two + "    - variable: c\n", 4, "takes exactly 2 inputs"},
		{"nine", head + "computed:\n  function: max\n  inputs:\n" + strings.Repeat("    - variable: a\n", 9), 4, "takes 2 to 8 inputs"},
		{"inputs not a list", head + "computed:\n  function: sum\n  inputs: a\n", 5, "list"},
		{"an input that is text", head + "computed:\n  function: sum\n  inputs:\n    - a\n    - variable: b\n", 6, "must be a mapping"},
		{"an input on both", head + "computed:\n  function: sum\n  inputs:\n    - variable: a\n      device: HmIP-RF/A:1/X\n    - variable: b\n", 6, "exactly one of"},
		{"an input on neither", head + "computed:\n  function: sum\n  inputs:\n    - {}\n    - variable: b\n", 6, "exactly one of"},
		{"a bad datapoint", head + "computed:\n  function: sum\n  inputs:\n    - device: nope\n    - variable: b\n", 6, "is not a datapoint"},
		{"a bad key", head + "computed:\n  function: sum\n  inputs:\n    - variable: Nope\n    - variable: b\n", 6, "not a variable's key"},
		{"a key that is a list", head + "computed:\n  function: sum\n  inputs:\n    - variable: [a]\n    - variable: b\n", 6, "flow style"},
		{"an unknown field of an input", head + "computed:\n  function: sum\n  inputs:\n    - variable: a\n      factor: 2\n    - variable: b\n", 7, "factor"},
		{"an unknown field", head + "computed:\n  function: sum\n" + two + "  round: 1\n", 8, "round"},
		{"not a number", "name: C\ntype: bool\ncomputed:\n  function: sum\n" + two, 4, "only a number can be computed"},
		{"with min", head + "min: 0\ncomputed:\n  function: sum\n" + two, 3, "has no \"min\""},
		{"with max", head + "max: 9\ncomputed:\n  function: sum\n" + two, 3, "has no \"max\""},
		{"with a start value", head + "start_value: 1\ncomputed:\n  function: sum\n" + two, 3, "has no \"start_value\""},
	} {
		_, err := DecodeVariable("variables/c.yaml", "c", []byte(c.yaml))
		e, ok := err.(*Error)
		if !ok {
			t.Errorf("%s: no refusal (%v)", c.name, err)
			continue
		}
		if e.Line != c.line || !strings.Contains(e.Msg, c.msg) {
			t.Errorf("%s: line %d %q, want line %d with %q", c.name, e.Line, e.Msg, c.line, c.msg)
		}
	}
}

// The functions, against values from tables: the dew point and the absolute humidity by the
// Magnus formula, the others plain arithmetic without a float's tail.
func TestCompute(t *testing.T) {
	for _, c := range []struct {
		fn   string
		in   []float64
		want float64
		ok   bool
	}{
		{FnDewPoint, []float64{20, 50}, 9.3, true},
		{FnDewPoint, []float64{25, 60}, 16.7, true},
		{FnDewPoint, []float64{0, 100}, 0, true},
		{FnDewPoint, []float64{-10, 80}, -12.8, true},
		{FnDewPoint, []float64{30, 100}, 30, true},
		{FnDewPoint, []float64{20, 0}, 0, false},
		{FnDewPoint, []float64{20, -5}, 0, false},
		{FnDewPoint, []float64{-300, 50}, 0, false},
		{FnAbsoluteHumidity, []float64{20, 50}, 8.6, true},
		{FnAbsoluteHumidity, []float64{25, 60}, 13.8, true},
		{FnAbsoluteHumidity, []float64{0, 100}, 4.8, true},
		{FnAbsoluteHumidity, []float64{20, 0}, 0, false},
		{FnDifference, []float64{21.3, 19.8}, 1.5, true},
		{FnDifference, []float64{1, 3}, -2, true},
		{FnSum, []float64{0.1, 0.2}, 0.3, true},
		{FnSum, []float64{1, 2, 3, 4, 5, 6, 7, 8}, 36, true},
		{FnAverage, []float64{20.5, 21.5, 19}, 20.333333, true},
		{FnMin, []float64{3, -1, 2}, -1, true},
		{FnMax, []float64{3, -1, 7.5}, 7.5, true},
		{FnSum, []float64{1}, 0, false},
		{FnSum, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}, 0, false},
		{FnDifference, []float64{1, 2, 3}, 0, false},
		{"median", []float64{1, 2}, 0, false},
		{FnSum, []float64{1.7e308, 1.7e308}, 0, false},
	} {
		got, ok := Compute(c.fn, c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%s%v = %v, %v; want %v, %v", c.fn, c.in, got, ok, c.want, c.ok)
		}
	}
	for _, fn := range ComputedFunctions {
		if lo, hi := InputCount(fn); lo != 2 || hi < 2 {
			t.Errorf("%s: %d to %d inputs", fn, lo, hi)
		}
	}
}

// What the set can tell about a computed value's inputs, and who uses a variable as one.
func TestComputedProblem(t *testing.T) {
	num := func(key string, inputs ...string) *Variable {
		v := &Variable{Key: key, Name: strings.ToUpper(key), Type: VarNumber}
		if len(inputs) > 0 {
			v.Computed = &Computed{Function: FnSum}
			for _, in := range inputs {
				if strings.Contains(in, "/") {
					r, _ := ParseRef(in)
					v.Computed.Inputs = append(v.Computed.Inputs, Input{Device: &r})
				} else {
					v.Computed.Inputs = append(v.Computed.Inputs, Input{Variable: in})
				}
			}
		}
		return v
	}
	s := NewSet()
	for _, v := range []*Variable{num("a"), num("b"), {Key: "flag", Name: "Flag", Type: VarBool},
		num("ok", "a", "HmIP-RF/A:1/X", SystemDutyCycle), num("chain", "ok", "b"),
		num("self", "self", "a"), num("gone", "a", "nope"), num("bool", "a", "flag"),
		num("one", "two", "a"), num("two", "three", "a"), num("three", "one", "a"), num("twice", "chain", "chain")} {
		s.Variables[v.Key] = v
	}
	for key, want := range map[string]string{
		"a": "", "ok": "", "chain": "", "twice": "",
		"self":  "self cannot be its own input",
		"gone":  "the input variable nope does not exist",
		"bool":  "the input variable flag is not a number",
		"one":   "the inputs of one lead back to itself through two",
		"three": "the inputs of three lead back to itself through one",
	} {
		if got := s.ComputedProblem(s.Variables[key]); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
	if got := s.ComputedUsers("a"); !reflect.DeepEqual(got, []string{"BOOL", "GONE", "OK", "ONE", "SELF", "THREE", "TWO"}) {
		t.Errorf("users of a: %v", got)
	}
	if got := s.ComputedUsers("flag"); !reflect.DeepEqual(got, []string{"BOOL"}) {
		t.Errorf("users of flag: %v", got)
	}
	if got := s.ComputedUsers("twice"); len(got) != 0 {
		t.Errorf("users of twice: %v", got)
	}
}
