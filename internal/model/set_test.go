package model

import (
	"reflect"
	"strings"
	"testing"
)

func testSet() *Set {
	s := NewSet()
	s.Variables["regen"] = &Variable{Key: "regen", Name: "Regen", Type: VarBool}
	s.Variables["modus"] = &Variable{Key: "modus", Name: "Modus", Type: VarList, Entries: []string{"Sommer", "Winter"}}
	s.Variables["temp"] = &Variable{Key: "temp", Name: "Temp", Type: VarNumber}
	s.Variables["text"] = &Variable{Key: "text", Name: "Text", Type: VarString}
	s.Variables["alarm"] = &Variable{Key: "alarm", Name: "Alarm", Type: VarAlarm}
	s.Programs["other"] = &Program{Key: "other", Name: "Other"}
	return s
}

func line(variable string, op Op, value any) Line {
	return Line{Variable: variable, Op: op, Value: value, Trigger: TriggerChange}
}

func program(lines []Line, actions []Action) *Program {
	return &Program{Key: "p", Name: "P", Rules: []Rule{
		{When: &Condition{Op: All, Groups: []Group{{Op: All, Lines: lines}}}, Then: actions},
		{Else: true},
	}}
}

func TestProgramProblems(t *testing.T) {
	s := testSet()
	ok := program([]Line{
		line("regen", OpEq, true), line("modus", OpNe, "Winter"), line("temp", OpGt, 5.0), line("text", OpEq, "x"), line("alarm", OpEq, false),
		{Variable: "temp", Op: OpRange, From: 1.0, Below: 2.0, Trigger: TriggerCheck},
		{Device: &Ref{"HmIP-RF", "A:1", "STATE"}, Op: OpEq, Value: true, Trigger: TriggerChange},
		{Time: &TimeModule{Kind: "daily"}, Trigger: TriggerUpdate},
	}, []Action{
		{Kind: ActionVariable, Variable: "modus", To: "Sommer"},
		{Kind: ActionVariable, Variable: "temp", FromVariable: "temp"},
		{Kind: ActionVariable, Variable: "temp", FromDevice: &Ref{"HmIP-RF", "A:1", "LEVEL"}},
		{Kind: ActionVariable, Variable: "temp", Special: "X"},
		{Kind: ActionSet, Device: &Ref{"HmIP-RF", "A:1", "STATE"}, To: true},
		{Kind: ActionSet, Device: &Ref{"HmIP-RF", "A:1", "STATE"}, FromVariable: "regen"},
		{Kind: ActionRun, Program: "other", Mode: RunThen},
		{Kind: ActionScript, Script: "x"},
		{Kind: ActionVariable, Variable: "temp", FromVariable: SystemDutyCycle},
		{Kind: ActionSet, Device: &Ref{"HmIP-RF", "A:1", "LEVEL"}, FromVariable: SystemServiceMessages},
	})
	ok.Rules[0].When.Groups[0].Lines = append(ok.Rules[0].When.Groups[0].Lines, line(SystemAlarmMessages, OpGt, 0.0))
	s.Programs["p"] = ok
	if got := s.ProgramProblems(ok); len(got) != 0 {
		t.Fatalf("problems of a sound program: %v", got)
	}
	if len(s.Problems()) != 0 {
		t.Fatalf("problems: %v", s.Problems())
	}
	if !ok.HasScript() || !ok.HasTime() || s.Programs["other"].HasScript() || s.Programs["other"].HasTime() {
		t.Error("HasScript / HasTime")
	}
	for want, p := range map[string]*Program{
		"the variable gone does not exist":       program([]Line{line("gone", OpEq, true)}, nil),
		"the variable temp is a number":          program([]Line{line("temp", OpEq, "warm")}, nil),
		"regen (bool) can only be compared":      program([]Line{line("regen", OpGt, 1.0)}, nil),
		"text is text and can only be compared":  program([]Line{line("text", OpLt, 1.0)}, nil),
		"regen takes true or false":              program([]Line{line("regen", OpEq, "ja")}, nil),
		"modus takes one of its entries":         program([]Line{line("modus", OpEq, "Herbst")}, nil),
		"a range on regen":                       program([]Line{{Variable: "regen", Op: OpRange, From: 1.0, Below: 2.0}}, nil),
		"rule 1: the variable gone2 does not":    program(nil, []Action{{Kind: ActionVariable, Variable: "gone2", To: 1.0}}),
		"rule 1: temp takes a number":            program(nil, []Action{{Kind: ActionVariable, Variable: "temp", To: "x"}}),
		"rule 1: the variable gone3 does not":    program(nil, []Action{{Kind: ActionVariable, Variable: "temp", FromVariable: "gone3"}}),
		"rule 1: the variable gone4 does not":    program(nil, []Action{{Kind: ActionSet, Device: &Ref{"a", "b:1", "c"}, FromVariable: "gone4"}}),
		"the program nowhere does not exist":     program(nil, []Action{{Kind: ActionActivate, Program: "nowhere"}}),
		"is a system value and cannot be writte": program(nil, []Action{{Kind: ActionVariable, Variable: SystemAlarmMessages, To: 1.0}}),
		"the variable duty-cycle is a number":    program([]Line{line(SystemDutyCycle, OpEq, "hoch")}, nil),
		"a program with a script cannot be acti": {Key: "p", Active: true, Rules: []Rule{{Else: true, Then: []Action{{Kind: ActionScript, Script: "x"}}}}},
	} {
		got := s.ProgramProblems(p)
		if len(got) != 1 || !strings.Contains(got[0], want) {
			t.Errorf("want %q, got %v", want, got)
		}
	}
	s.Programs["bad"] = program([]Line{line("gone", OpEq, true)}, nil)
	if pr := s.Problems(); len(pr) != 1 || len(pr["bad"]) != 1 {
		t.Errorf("problems: %v", pr)
	}
}

func TestSystemValueKeys(t *testing.T) {
	for _, k := range SystemValues {
		if !IsSystemValue(k) || !ValidKey(k) {
			t.Errorf("%s", k)
		}
	}
	if IsSystemValue("regen") {
		t.Error("regen")
	}
}

func TestUsersAndRename(t *testing.T) {
	s := testSet()
	s.Programs["a"] = program([]Line{line("modus", OpEq, "Winter")}, []Action{{Kind: ActionVariable, Variable: "modus", To: "Sommer"}, {Kind: ActionRun, Program: "other", Mode: RunThen}})
	s.Programs["a"].Key = "a"
	s.Programs["b"] = program([]Line{line("regen", OpEq, true)}, []Action{{Kind: ActionVariable, Variable: "temp", FromVariable: "modus"}, {Kind: ActionVariable, Variable: "modus", To: "Winter"}})
	s.Programs["b"].Key = "b"
	s.Programs["other"].Rules = []Rule{{Else: true, Then: []Action{{Kind: ActionActivate, Program: "other"}}}}
	if got := s.VariableUsers("modus"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("users of modus: %v", got)
	}
	if got := s.VariableUsers("regen"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("users of regen: %v", got)
	}
	if got := s.VariableUsers("temp"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("users of temp: %v", got)
	}
	if got := s.VariableUsers("alarm"); got != nil {
		t.Errorf("users of alarm: %v", got)
	}
	if got := s.EntryUsers("modus", "Winter"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("users of Winter: %v", got)
	}
	if got := s.EntryUsers("modus", "Sommer"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("users of Sommer: %v", got)
	}
	if got := s.ProgramUsers("other"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("users of other: %v", got)
	}
	changed := s.RenameEntry("modus", "Winter", "Kalt")
	if len(changed) != 2 || changed["a"].Rules[0].When.Groups[0].Lines[0].Value != "Kalt" || changed["b"].Rules[0].Then[1].To != "Kalt" {
		t.Errorf("rename: %+v", changed)
	}
	if s.Programs["a"].Rules[0].When.Groups[0].Lines[0].Value != "Winter" || changed["a"].Rules[0].Then[0].To != "Sommer" {
		t.Error("rename changed the set itself, or another entry")
	}
}
