package soak

import (
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// The tree has the numbers asked for, every file is canonical and reads back, no program has a
// problem, and nothing a program writes is something the load writes.
func TestNew(t *testing.T) {
	for _, o := range []Options{
		{},
		{Programs: 24, Variables: 10},
		{Programs: 36, Variables: 300, Read: "HmIP-RF/000A1B2C3D4E02:1/STATE", ReadValue: "1", Write: "HmIP-RF/HmIP-RCV-1:1/PRESS_SHORT", WriteValue: "true"},
	} {
		tree, err := New(o)
		if err != nil {
			t.Fatal(err)
		}
		wantP, wantV := o.Programs, o.Variables
		if wantP == 0 {
			wantP, wantV = 300, 800
		}
		if len(tree.Programs) != wantP {
			t.Errorf("%d programs, want %d", len(tree.Programs), wantP)
		}
		if min := 150 + 2*wantP + 22; len(tree.Variables) != max(wantV, min) {
			t.Errorf("%d variables, want %d (or the %d the programs need)", len(tree.Variables), wantV, min)
		}
		if len(tree.Inputs) != 150 || tree.Inputs[0].Key != "soak-in-b-000" {
			t.Errorf("inputs: %d, first %v", len(tree.Inputs), tree.Inputs[0])
		}
		inputs := map[string]bool{}
		for _, in := range tree.Inputs {
			inputs[in.Key] = true
			if _, ok := tree.Variables[in.Key]; !ok {
				t.Errorf("input %s has no file", in.Key)
			}
		}
		set := model.NewSet()
		for key, data := range tree.Variables {
			v, err := model.DecodeVariable(key+".yaml", key, data)
			if err != nil {
				t.Fatal(err)
			}
			if string(model.EncodeVariable(v)) != string(data) {
				t.Errorf("variable %s is not canonical", key)
			}
			set.Variables[key] = v
		}
		shapes := map[string]int{}
		for key, data := range tree.Programs {
			p, err := model.DecodeProgram(key+".yaml", key, data)
			if err != nil {
				t.Fatal(err)
			}
			if string(model.EncodeProgram(p, nil)) != string(data) {
				t.Errorf("program %s is not canonical", key)
			}
			set.Programs[key] = p
			shapes[p.Name[9:]]++
			p.Actions(func(_ int, a *model.Action) {
				if inputs[a.Variable] {
					t.Errorf("program %s writes the input %s", key, a.Variable)
				}
			})
		}
		if problems := set.Problems(); len(problems) != 0 {
			t.Errorf("problems: %v", problems)
		}
		want := 10
		if o.Read != "" {
			want = 12
		}
		if len(shapes) != want {
			t.Errorf("%d shapes of programs, want %d: %v", len(shapes), want, shapes)
		}
	}
}

// A text the model refuses is an error that names the object.
func TestNewRefuses(t *testing.T) {
	if _, err := New(Options{Programs: 12, Read: "no datapoint", ReadValue: "1", Write: "HmIP-RF/X:1/STATE", WriteValue: "true"}); err == nil || !strings.Contains(err.Error(), "program soak-010") {
		t.Errorf("a device line without a datapoint: %v", err)
	}
}
