package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

var admin = repo.Author{Name: "Admin"}

func open(t *testing.T, dir string) (*Service, *[]string) {
	t.Helper()
	var logs []string
	// (the push worker logs too: one line at a time)
	var mu sync.Mutex
	s, err := Open(Options{Dir: dir, Clock: engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)), Log: func(f string, a ...any) {
		mu.Lock()
		defer mu.Unlock()
		logs = append(logs, f)
	}})
	if err != nil {
		t.Fatal(err)
	}
	// the history is written in the background and records the files as they are when its
	// turn comes; the tests wait for it after every change, so each change is its own entry
	s.OnChange(s.queue.Flush)
	return s, &logs
}

func codeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err == nil {
		return ""
	}
	return "other: " + err.Error()
}

func list(name string, entries ...string) *model.Variable {
	return &model.Variable{Name: name, Type: model.VarList, Entries: entries, Visible: true}
}

func uses(key, modus string) *model.Program {
	return &model.Program{Name: key, Visible: true, Operable: true, Rules: []model.Rule{{
		When: &model.Condition{Op: model.All, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "modus", Op: model.OpEq, Value: modus, Trigger: model.TriggerChange}}}}},
		Then: []model.Action{{Kind: model.ActionVariable, Variable: "modus", To: modus}},
	}}}
}

func TestVariableLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	changes := 0
	s.OnChange(func() { changes++ })
	v, err := s.CreateVariable(&model.Variable{Name: "Außentemperatur Mittel", Type: model.VarNumber, Unit: "°C", Visible: true}, 12.5, admin)
	if err != nil || v.Key != "aussentemperatur-mittel" {
		t.Fatalf("create: %+v %v", v, err)
	}
	if got, _ := s.Engine().Value(v.Key); got != 12.5 {
		t.Errorf("the initial value: %v", got)
	}
	// the same name again gets another key
	v2, err := s.CreateVariable(&model.Variable{Name: "Außentemperatur Mittel", Type: model.VarBool, Visible: true}, nil, admin)
	if err != nil || v2.Key != "aussentemperatur-mittel-2" {
		t.Fatalf("second: %+v %v", v2, err)
	}
	// a system value's name is not taken as a key
	v3, err := s.CreateVariable(&model.Variable{Name: "Alarm messages", Type: model.VarBool, Visible: true}, nil, admin)
	if err != nil || v3.Key == engine.SystemAlarmMessages {
		t.Fatalf("a system value's key: %+v %v", v3, err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "", Type: model.VarBool}, nil, admin); codeOf(err) != "invalid" {
		t.Errorf("no name: %v", err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "X", Type: model.VarBool}, "ja", admin); codeOf(err) != "invalid" {
		t.Errorf("an initial value of another type: %v", err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "X", Type: model.VarBool, Unit: "V"}, nil, admin); codeOf(err) != "invalid" || strings.Contains(err.Error(), ".yaml") {
		t.Errorf("a refused definition names a file: %v", err)
	}
	if got := s.Variables(); len(got) != 3 || got[0].Name != "Alarm messages" {
		t.Fatalf("variables: %+v", got)
	}
	// update: the name changes, the key stays
	up, err := s.UpdateVariable(v.Key, &model.Variable{Name: "Temperatur außen", Type: model.VarNumber, Visible: true}, nil, admin)
	if err != nil || up.Key != v.Key || up.Name != "Temperatur außen" {
		t.Fatalf("update: %+v %v", up, err)
	}
	if got, _ := s.Engine().Value(v.Key); got != 12.5 {
		t.Errorf("the value after an update: %v", got)
	}
	if _, err := s.UpdateVariable("nothing", up, nil, admin); codeOf(err) != "not-found" {
		t.Errorf("update of nothing: %v", err)
	}
	if _, err := s.UpdateVariable(v.Key, &model.Variable{Name: "", Type: model.VarNumber}, nil, admin); codeOf(err) != "invalid" {
		t.Errorf("update without a name: %v", err)
	}
	if err := s.DeleteVariable(v2.Key, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVariable(v2.Key, admin); codeOf(err) != "not-found" {
		t.Errorf("delete twice: %v", err)
	}
	if changes != 5 {
		t.Errorf("%d change calls, want 5", changes)
	}
	s.Flush()
	h, err := s.History("", 0)
	if err != nil || len(h) != 5 || h[0].Message != "Variable Außentemperatur Mittel removed" || h[0].Author != "Admin" || h[1].Message != "Variable Temperatur außen changed" {
		t.Fatalf("history: %+v %v", h, err)
	}
	if fh, _ := s.History("variables/"+v.Key+".yaml", 0); len(fh) != 2 {
		t.Errorf("the file's history: %+v", fh)
	}
	if n := s.Notices(); n.History != "" || n.State != "" || len(n.BadFiles) != 0 {
		t.Errorf("notices: %+v", n)
	}
	// the state survives a restart; the files are the definitions
	if err := s.Engine().SetVariable(v.Key, 99.0, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, _ := open(t, dir)
	if got, _ := s2.Engine().Value(v.Key); got != 99.0 {
		t.Errorf("after a restart: %v", got)
	}
	if got := s2.Variables(); len(got) != 2 {
		t.Errorf("after a restart: %d variables", len(got))
	}
	if s2.Tree() == nil {
		t.Error("Tree")
	}
	_ = s2.Close()
}

// "Wert beim Systemstart" is applied once per boot of the system, not at every start of the addon.
func TestSystemStart(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	if _, err := s.CreateVariable(&model.Variable{Name: "Anwesenheit", Type: model.VarBool, Visible: true, StartValue: true}, false, admin); err != nil {
		t.Fatal(err)
	}
	if !s.SystemStart("boot-1") {
		t.Fatal("the first start after a boot was not taken for one")
	}
	if got, _ := s.Engine().Value("anwesenheit"); got != true {
		t.Errorf("the start value: %v", got)
	}
	_ = s.Engine().SetVariable("anwesenheit", false, engine.FromUser)
	_ = s.Close()
	s, _ = open(t, dir)
	if s.SystemStart("boot-1") {
		t.Error("a restart of the addon was taken for a boot")
	}
	if got, _ := s.Engine().Value("anwesenheit"); got != false {
		t.Errorf("a restart of the addon set the start value: %v", got)
	}
	if s.SystemStart("") {
		t.Error("an unknown boot id was taken for a boot")
	}
	if !s.SystemStart("boot-2") {
		t.Error("the next boot was not taken for one")
	}
	if got, _ := s.Engine().Value("anwesenheit"); got != true {
		t.Errorf("the start value after the next boot: %v", got)
	}
	_ = s.Close()
}

// The evaluation after a boot of the system: once per boot id, when everything is ready or the
// wait is over; only programs whose own switch says so; a restart of the addon does not repeat
// it, one that was stopped while it waited does it at its next start.
func TestStartEvaluation(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	empty := func(name string, atStart bool) *model.Program {
		return &model.Program{Name: name, Active: true, Visible: true, Operable: true, AtStart: atStart, Rules: []model.Rule{{When: &model.Condition{Op: model.All, Groups: []model.Group{}}, Then: []model.Action{}}}}
	}
	for _, p := range []*model.Program{empty("Bei Start", true), empty("Nicht bei Start", false)} {
		if _, err := s.CreateProgram(p, admin); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if keys, done := s.StartEvaluation(ctx, "", true, func() bool { return true }, time.Second, time.Millisecond); done || keys != nil {
		t.Error("an unknown boot id")
	}
	// stopped while it waits: nothing is evaluated, and nothing is noted as done
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	if _, done := s.StartEvaluation(stopped, "boot-1", true, func() bool { return false }, time.Minute, time.Millisecond); done || len(s.Engine().Runs("bei-start")) != 0 {
		t.Error("evaluated although the addon was stopped while waiting")
	}
	// ready after a few looks
	looks := 0
	keys, done := s.StartEvaluation(ctx, "boot-1", true, func() bool { looks++; return looks > 3 }, time.Minute, time.Millisecond)
	if !done || !reflect.DeepEqual(keys, []string{"bei-start"}) || looks != 4 {
		t.Fatalf("the evaluation: %v %v after %d looks", keys, done, looks)
	}
	if runs := s.Engine().Runs("bei-start"); len(runs) != 1 || runs[0].Trigger != engine.StartTrigger || runs[0].Block != 1 {
		t.Errorf("the run: %+v", runs)
	}
	if runs := s.Engine().Runs("nicht-bei-start"); len(runs) != 0 {
		t.Errorf("a program that is not evaluated at start ran: %+v", runs)
	}
	// the page's switch
	if p, err := s.SetAtStart("nicht-bei-start", true, admin); err != nil || !p.AtStart {
		t.Fatalf("switched on: %+v %v", p, err)
	}
	if p, err := s.SetAtStart("nicht-bei-start", true, admin); err != nil || !p.AtStart {
		t.Errorf("the same again: %+v %v", p, err)
	}
	if _, err := s.SetAtStart("nothing", true, admin); codeOf(err) != "not-found" {
		t.Errorf("a program that is not there: %v", err)
	}
	// the same boot again - the addon was restarted or updated: not again
	_ = s.Close()
	s, logs := open(t, dir)
	if _, done := s.StartEvaluation(ctx, "boot-1", false, func() bool { return true }, time.Minute, time.Millisecond); done {
		t.Error("a restart of the addon evaluated again")
	}
	// the next boot, and what it waits for never comes: after the wait it goes on with what is known
	keys, done = s.StartEvaluation(ctx, "boot-2", true, func() bool { return false }, 20*time.Millisecond, time.Millisecond)
	if !done || !reflect.DeepEqual(keys, []string{"bei-start", "nicht-bei-start"}) {
		t.Errorf("the next boot: %v %v", keys, done)
	}
	if all := strings.Join(*logs, "|"); !strings.Contains(all, "not everything was ready") || !strings.Contains(all, "programs evaluated once") {
		t.Errorf("log: %s", all)
	}
	// a boot whose first start was stopped while it waited: the next start, not the first one
	// after that boot, still evaluates - an evaluation was noted before, this boot's is owed
	if keys, done := s.StartEvaluation(ctx, "boot-3", false, func() bool { return true }, time.Minute, time.Millisecond); !done || len(keys) != 2 {
		t.Errorf("an owed evaluation: %v %v", keys, done)
	}
	_ = s.Close()

	// an update from a version that noted no evaluation, on a system that did not start: nothing
	// is evaluated, now or at the next restart
	dir = t.TempDir()
	s, _ = open(t, dir)
	if _, err := s.CreateProgram(empty("Bei Start", true), admin); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, done := s.StartEvaluation(ctx, "boot-9", false, func() bool { return true }, time.Minute, time.Millisecond); done || len(s.Engine().Runs("bei-start")) != 0 {
			t.Errorf("an update of the addon evaluated the programs (%d)", i)
		}
	}
	_ = s.Close()
}

// A delayed activity that is scheduled, removed or run asks for the state to be written soon:
// a delay must survive a power cut, not only a clean stop.
func TestSaveSoonForDelays(t *testing.T) {
	dir := t.TempDir()
	clock := engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	s, err := Open(Options{Dir: dir, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "Licht", Type: model.VarBool, Visible: true}, false, admin); err != nil {
		t.Fatal(err)
	}
	p := &model.Program{Name: "Später", Active: true, Visible: true, Operable: true, Rules: []model.Rule{{
		When: &model.Condition{Op: model.All, Groups: []model.Group{}},
		Then: []model.Action{{Kind: model.ActionVariable, Variable: "licht", To: true, After: model.Duration(time.Minute)}}}}}
	if _, err := s.CreateProgram(p, admin); err != nil {
		t.Fatal(err)
	}
	asked := func() bool {
		select {
		case <-s.SaveSoon():
			return true
		default:
			return false
		}
	}
	if asked() {
		t.Error("asked to save without a delay")
	}
	if err := s.Engine().RunProgram("spaeter", model.RunThen); err != nil {
		t.Fatal(err)
	}
	if !asked() || len(s.Engine().PendingActions()) != 1 {
		t.Fatal("a scheduled delay did not ask for the state to be written")
	}
	// written then, a power cut (no Close) keeps it: the next start arms it again
	if err := s.SaveState(false); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(Options{Dir: dir, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Engine().PendingActions(); len(got) != 1 || got[0].Program != "spaeter" {
		t.Errorf("after the power cut: %+v", got)
	}
	asked() // (loading it armed it)
	clock.Advance(2 * time.Minute)
	if !asked() {
		t.Error("a delay that ran did not ask for the state to be written")
	}
	_ = s2.Close()
	_ = s.Close()
}

// Q-35 and Q-36: a value list or a type that programs use does not silently change under them.
func TestVariableInUse(t *testing.T) {
	s, _ := open(t, t.TempDir())
	if _, err := s.CreateVariable(list("Modus", "Sommer", "Übergang", "Winter"), "Winter", admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProgram(uses("heizen", "Winter"), admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProgram(uses("lueften", "Sommer"), admin); err != nil {
		t.Fatal(err)
	}
	if got := s.VariableUsers("modus"); !reflect.DeepEqual(got, []string{"heizen", "lueften"}) {
		t.Fatalf("users: %v", got)
	}
	// the type cannot change while in use
	_, err := s.UpdateVariable("modus", &model.Variable{Name: "Modus", Type: model.VarString}, nil, admin)
	var e *Error
	if !errors.As(err, &e) || e.Code != "in-use" || !reflect.DeepEqual(e.Users, []string{"heizen", "lueften"}) {
		t.Fatalf("type change: %v", err)
	}
	// an entry in use cannot be removed; one that is not, can
	if _, err = s.UpdateVariable("modus", list("Modus", "Sommer", "Übergang"), nil, admin); !errors.As(err, &e) || e.Code != "in-use" || !reflect.DeepEqual(e.Users, []string{"heizen"}) {
		t.Fatalf("removing Winter: %v", err)
	}
	if _, err = s.UpdateVariable("modus", list("Modus", "Sommer", "Winter"), nil, admin); err != nil {
		t.Fatalf("removing Übergang: %v", err)
	}
	// a rename rewrites the programs and the value
	if _, err = s.UpdateVariable("modus", list("Modus", "Warm", "Kalt"), map[string]string{"Winter": "Kalt", "Sommer": "Warm"}, admin); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got, _ := s.Engine().Value("modus"); got != "Kalt" {
		t.Errorf("the value after the rename: %v", got)
	}
	p, _ := s.Program("heizen")
	if p.Program.Rules[0].When.Groups[0].Lines[0].Value != "Kalt" || p.Program.Rules[0].Then[0].To != "Kalt" || len(p.Problems) != 0 {
		t.Errorf("the program after the rename: %+v", p)
	}
	data, _ := s.Tree().Read("programs/heizen.yaml")
	if !strings.Contains(string(data), "is: Kalt") {
		t.Errorf("the file after the rename:\n%s", data)
	}
	for _, r := range []map[string]string{{"Nichts": "Kalt"}, {"Kalt": "Nirgends"}, {"Kalt": "Warm"}} {
		entries := []string{"Warm", "Kalt"}
		if _, err := s.UpdateVariable("modus", list("Modus", entries...), r, admin); codeOf(err) != "invalid" {
			t.Errorf("rename %v: %v", r, err)
		}
	}
	if err := s.DeleteVariable("modus", admin); !errors.As(err, &e) || e.Code != "in-use" || len(e.Users) != 2 {
		t.Fatalf("delete in use: %v", err)
	}
	s.Flush()
	h, _ := s.History("programs/heizen.yaml", 0)
	if len(h) != 2 || h[0].Message != "Variable Modus changed" {
		t.Errorf("the program's history: %+v", h)
	}
	_ = s.Close()
}

func TestProgramLifecycle(t *testing.T) {
	s, _ := open(t, t.TempDir())
	if _, err := s.CreateVariable(list("Modus", "Sommer", "Winter"), nil, admin); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProgram(uses("Heizen im Winter", "Winter"), admin)
	if err != nil || p.Key != "heizen-im-winter" || p.Active {
		t.Fatalf("create: %+v %v", p, err)
	}
	if _, err := s.CreateProgram(&model.Program{Name: ""}, admin); codeOf(err) != "invalid" {
		t.Errorf("no name: %v", err)
	}
	// an active program must be sound
	bad := uses("Kaputt", "Herbst")
	bad.Active = true
	if _, err := s.CreateProgram(bad, admin); codeOf(err) != "invalid" || !strings.Contains(err.Error(), "cannot be active") {
		t.Errorf("an active program with a broken reference: %v", err)
	}
	bad.Active = false
	stored, err := s.CreateProgram(bad, admin)
	if err != nil {
		t.Fatalf("the same program inactive: %v", err)
	}
	if info, _ := s.Program(stored.Key); len(info.Problems) != 2 {
		t.Errorf("problems: %+v", info.Problems)
	}
	if _, err := s.SetActive(stored.Key, true, admin); codeOf(err) != "invalid" {
		t.Errorf("activating it: %v", err)
	}
	on, err := s.SetActive(p.Key, true, admin)
	if err != nil || !on.Active {
		t.Fatalf("activate: %+v %v", on, err)
	}
	if again, err := s.SetActive(p.Key, true, admin); err != nil || !again.Active {
		t.Errorf("activate again: %v", err)
	}
	if _, err := s.SetActive("nothing", true, admin); codeOf(err) != "not-found" {
		t.Errorf("activate nothing: %v", err)
	}
	renamed := on.Clone()
	renamed.Name = "Heizen"
	up, err := s.UpdateProgram(p.Key, renamed, admin)
	if err != nil || up.Key != p.Key || up.Name != "Heizen" {
		t.Fatalf("update: %+v %v", up, err)
	}
	if _, err := s.UpdateProgram("nothing", renamed, admin); codeOf(err) != "not-found" {
		t.Errorf("update nothing: %v", err)
	}
	renamed.Rules = append(renamed.Rules, model.Rule{Else: true}, model.Rule{Else: true})
	if _, err := s.UpdateProgram(p.Key, renamed, admin); codeOf(err) != "invalid" {
		t.Errorf("two else rules: %v", err)
	}
	// a program another one runs cannot be removed
	runner := &model.Program{Name: "Starter", Visible: true, Operable: true, Rules: []model.Rule{{Else: true, Then: []model.Action{{Kind: model.ActionRun, Program: p.Key, Mode: model.RunThen}}}}}
	if _, err := s.CreateProgram(runner, admin); err != nil {
		t.Fatal(err)
	}
	if got := s.ProgramUsers(p.Key); !reflect.DeepEqual(got, []string{"starter"}) {
		t.Errorf("users: %v", got)
	}
	var e *Error
	if err := s.DeleteProgram(p.Key, admin); !errors.As(err, &e) || e.Code != "in-use" || !reflect.DeepEqual(e.Users, []string{"starter"}) {
		t.Fatalf("delete in use: %v", err)
	}
	if err := s.DeleteProgram("starter", admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProgram(p.Key, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProgram(p.Key, admin); codeOf(err) != "not-found" {
		t.Errorf("delete twice: %v", err)
	}
	if got := s.Programs(); len(got) != 1 || got[0].Program.Key != "kaputt" {
		t.Errorf("programs: %+v", got)
	}
	if _, ok := s.Program("nothing"); ok {
		t.Error("a program that is not there")
	}
	_ = s.Close()
}

// A program's activity that switches another program is stored: the file says what the engine
// did, and the program stays switched after a restart.
func TestProgramSwitchedByAProgram(t *testing.T) {
	dir := t.TempDir()
	s, logs := open(t, dir)
	if _, err := s.CreateVariable(&model.Variable{Name: "Regen", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	target := &model.Program{Name: "Ziel", Visible: true, Operable: true, Rules: []model.Rule{{Else: true}}}
	if _, err := s.CreateProgram(target, admin); err != nil {
		t.Fatal(err)
	}
	when := func(v bool) *model.Condition {
		return &model.Condition{Op: model.All, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "regen", Op: model.OpEq, Value: v, Trigger: model.TriggerChange}}}}}
	}
	sw := &model.Program{Name: "Schalter", Active: true, Visible: true, Operable: true, Rules: []model.Rule{
		{When: when(true), Then: []model.Action{{Kind: model.ActionActivate, Program: "ziel"}}},
		{Else: true, Then: []model.Action{{Kind: model.ActionDeactivate, Program: "ziel"}}},
	}}
	if _, err := s.CreateProgram(sw, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Engine().SetVariable("regen", true, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	if p, _ := s.Program("ziel"); !p.Program.Active {
		t.Fatal("the program was not switched on")
	}
	data, _ := s.Tree().Read("programs/ziel.yaml")
	if !strings.Contains(string(data), "active: true") {
		t.Errorf("the file:\n%s", data)
	}
	h, _ := s.History("programs/ziel.yaml", 1)
	if len(h) != 1 || h[0].Author != "program" {
		t.Errorf("history: %+v", h)
	}
	_ = s.Engine().SetVariable("regen", false, engine.FromUser)
	s.Flush()
	if p, _ := s.Program("ziel"); p.Program.Active {
		t.Fatal("the program was not switched off")
	}
	// a switch that cannot be stored is logged, nothing more
	n := len(*logs)
	on := true
	s.fanOut(engine.Notification{Kind: "program", Key: "nirgends", Active: &on})
	s.Flush()
	if len(*logs) != n+1 {
		t.Errorf("logs: %v", (*logs)[n:])
	}
	_ = s.Close()
}

func TestApplyImport(t *testing.T) {
	s, _ := open(t, t.TempDir())
	active := uses("Heizen", "Winter")
	active.Key, active.Active = "heizen", true
	broken := uses("Kaputt", "Herbst")
	broken.Key, broken.Active = "kaputt", true
	modus := list("Modus", "Sommer", "Winter")
	modus.Key = "modus"
	zahl := &model.Variable{Key: "zaehler", Name: "Zähler", Type: model.VarNumber, Max: func() *float64 { x := 100.0; return &x }(), Visible: true}
	res, err := s.ApplyImport(Import{Programs: []*model.Program{active, broken}, Variables: []*model.Variable{modus, zahl},
		Values: map[string]any{"modus": "Winter", "zaehler": 70000.0, "nirgends": 1.0, "zaehler-x": true}}, admin)
	if err != nil {
		t.Fatal(err)
	}
	if res.Programs != 2 || res.Variables != 2 || len(res.Deactivated) != 1 || len(res.Deactivated["kaputt"]) != 2 || len(res.Values) != 2 {
		t.Fatalf("result: %+v", res)
	}
	if got, _ := s.Engine().Value("zaehler"); got != 70000.0 {
		t.Errorf("a counter beyond its maximum: %v", got)
	}
	if got, _ := s.Engine().Value("modus"); got != "Winter" {
		t.Errorf("modus: %v", got)
	}
	h, _ := s.Program("heizen")
	k, _ := s.Program("kaputt")
	if !h.Program.Active || k.Program.Active {
		t.Errorf("active: heizen %v, kaputt %v", h.Program.Active, k.Program.Active)
	}
	// a program on a system value needs no variable; the import's own files go with it
	var notes []engine.Notification
	s.Subscribe(func(n engine.Notification) { notes = append(notes, n) })
	onAlarms := &model.Program{Key: "alarme-zeigen", Name: "Alarme zeigen", Active: true, Visible: true, Operable: true, Rules: []model.Rule{{
		When: &model.Condition{Op: model.All, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: model.SystemAlarmMessages, Op: model.OpGt, Value: 0.0, Trigger: model.TriggerChange}}}}},
		Then: []model.Action{},
	}}}
	alarm := &model.Variable{Key: "alarm", Name: "Alarm", Type: model.VarAlarm, Visible: true}
	res, err = s.ApplyImport(Import{Programs: []*model.Program{onAlarms}, Variables: []*model.Variable{alarm},
		Files: map[string][]byte{"imports/ccu-keller.yaml": []byte("serial: X\n"), "devices-seen.yaml": []byte("devices: []\n")}}, admin)
	if err != nil || len(res.Deactivated) != 0 {
		t.Fatalf("a program on a system value: %+v %v", res, err)
	}
	if b, _ := s.Tree().Read("imports/ccu-keller.yaml"); string(b) != "serial: X\n" {
		t.Errorf("the ledger: %q", b)
	}
	if lh, _ := s.History("imports/ccu-keller.yaml", 0); len(lh) != 1 || lh[0].Message != "Import: 1 programs, 1 variables" {
		t.Errorf("the ledger's history: %+v", lh)
	}
	if err := s.Engine().SetVariable("alarm", true, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Engine().Value(model.SystemAlarmMessages); got != 1.0 || len(notes) != 3 || notes[1].Key != model.SystemAlarmMessages || notes[1].Value != 1.0 || notes[2].Kind != "program" || notes[2].Key != "alarme-zeigen" || notes[2].Block != 1 {
		t.Errorf("the number of alarm messages: %v, %+v", got, notes)
	}
	// a variable an importer brings for a built-in counter is left out, not refused
	res, err = s.ApplyImport(Import{Variables: []*model.Variable{{Key: model.SystemDutyCycle, Name: "DutyCycle", Type: model.VarNumber, Visible: true}}, Values: map[string]any{model.SystemDutyCycle: 12.0}}, admin)
	if err != nil || res.Variables != 0 || len(res.Values) != 1 {
		t.Errorf("a variable under a system value's key: %+v %v", res, err)
	}
	if _, err := s.Tree().Read("variables/duty-cycle.yaml"); err == nil {
		t.Error("the system value got a file")
	}
	if _, err := s.ApplyImport(Import{Files: map[string][]byte{"programs/x.yaml": []byte("x")}}, admin); codeOf(err) != "invalid" {
		t.Errorf("a file below programs/: %v", err)
	}
	// again: the same keys are replaced, with their own message
	active.Name = "Heizen neu"
	if _, err := s.ApplyImport(Import{Programs: []*model.Program{active}, Message: "Import CCU Keller"}, admin); err != nil {
		t.Fatal(err)
	}
	if h, _ = s.Program("heizen"); h.Program.Name != "Heizen neu" || len(s.Programs()) != 3 {
		t.Errorf("after the second import: %+v", h.Program)
	}
	// one refused object and nothing is written
	for _, in := range []Import{
		{Variables: []*model.Variable{{Key: "ok", Name: "OK", Type: model.VarBool}, {Key: "bad", Name: "", Type: model.VarBool}}},
		{Variables: []*model.Variable{{Key: "ok", Name: "OK", Type: model.VarBool}}, Programs: []*model.Program{{Key: "bad"}}},
	} {
		if _, err := s.ApplyImport(in, admin); codeOf(err) != "invalid" {
			t.Errorf("a refused import: %v", err)
		}
	}
	if len(s.Variables()) != 3 {
		t.Errorf("a refused import left %d variables", len(s.Variables()))
	}
	if _, err = s.ApplyImport(Import{}, admin); err != nil {
		t.Errorf("an empty import: %v", err)
	}
	s.Flush()
	hist, _ := s.History("", 0)
	if len(hist) != 3 || hist[0].Message != "Import CCU Keller" || hist[2].Message != "Import: 2 programs, 2 variables" {
		t.Errorf("history: %+v", hist)
	}
	_ = s.Close()
}

// Files changed by hand: a reload takes them, reports what it cannot read, and the history
// records them.
func TestReloadAndBadFiles(t *testing.T) {
	dir := t.TempDir()
	s, logs := open(t, dir)
	if _, err := s.CreateVariable(&model.Variable{Name: "Regen", Type: model.VarBool, Visible: true}, true, admin); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(dir, "tree")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(tree, "variables", "wasser.yaml"), []byte("name: Wasser\ntype: bool\n"), 0o644))
	must(os.WriteFile(filepath.Join(tree, "programs", "kaputt.yaml"), []byte("name: Kaputt\nrules: nein\n"), 0o644))
	must(s.Reload())
	if len(s.Variables()) != 2 {
		t.Errorf("after the reload: %d variables", len(s.Variables()))
	}
	if got, _ := s.Engine().Value("regen"); got != true {
		t.Errorf("a reload lost a value: %v", got)
	}
	n := s.Notices()
	if len(n.BadFiles) != 1 || n.BadFiles[0].Path != "programs/kaputt.yaml" || !strings.Contains(n.BadFiles[0].Error, "kaputt.yaml:2") || len(*logs) == 0 {
		t.Errorf("bad files: %+v", n.BadFiles)
	}
	s.Flush()
	h, _ := s.History("variables/wasser.yaml", 0)
	if len(h) != 1 || h[0].Message != "Changed outside the page" {
		t.Errorf("the hand edit's history: %+v", h)
	}
	// the tree becomes unreadable: the reload says so and the engine keeps what it has
	must(os.Rename(filepath.Join(tree, "programs"), filepath.Join(tree, "programs.away")))
	must(os.WriteFile(filepath.Join(tree, "programs"), []byte("x"), 0o644))
	if err := s.Reload(); err == nil {
		t.Error("a reload of an unreadable tree")
	}
	if len(s.Variables()) != 2 {
		t.Error("the failed reload emptied the set")
	}
	_ = s.Close()
}

// A damaged state file: the generation before is used and the user is told; both damaged, the
// defaults and a notice. A history that does not open is moved aside and started anew.
func TestDamagedState(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	if _, err := s.CreateVariable(&model.Variable{Name: "Zahl", Type: model.VarNumber, Visible: true}, 1.0, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveState(false); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveState(false); err != nil { // nothing changed: nothing written
		t.Fatal(err)
	}
	_ = s.Engine().SetVariable("zahl", 2.0, engine.FromUser)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state", "state.json")
	if b, _ := os.ReadFile(state + ".1"); !strings.Contains(string(b), `"value":1`) {
		t.Fatalf("the generation before: %s", b)
	}
	_ = os.WriteFile(state, []byte(`{"variables":`), 0o600)
	s, logs := open(t, dir)
	if got, _ := s.Engine().Value("zahl"); got != 1.0 || !strings.Contains(s.Notices().State, "the one before") || len(*logs) == 0 {
		t.Errorf("a torn state file: %v, %q", got, s.Notices().State)
	}
	_ = s.Close()
	_ = os.WriteFile(state, []byte(`{`), 0o600)
	_ = os.WriteFile(state+".1", []byte(`{`), 0o600)
	s, _ = open(t, dir)
	if got, _ := s.Engine().Value("zahl"); got != 0.0 || !strings.Contains(s.Notices().State, "default values") {
		t.Errorf("both damaged: %v, %q", got, s.Notices().State)
	}
	_ = s.Close()
	// the history is damaged
	git := filepath.Join(dir, "tree", ".git")
	if err := os.RemoveAll(git); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(git, []byte("nonsense"), 0o644)
	s, logs = open(t, dir)
	if !strings.Contains(s.Notices().History, "started anew") || len(*logs) == 0 {
		t.Errorf("a damaged history: %q", s.Notices().History)
	}
	if h, _ := s.History("", 0); len(h) != 1 || h[0].Message != outside {
		t.Errorf("the new history: %+v", h)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "Neu", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	if n := s.Notices(); n.History != "" {
		t.Errorf("the notice after a commit that worked: %q", n.History)
	}
	moved, _ := filepath.Glob(filepath.Join(dir, "history-broken-*"))
	if len(moved) != 1 {
		t.Errorf("the old history was not kept: %v", moved)
	}
	_ = s.Close()
}

func TestOpenErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	_ = os.WriteFile(file, []byte("x"), 0o644)
	if _, err := Open(Options{Dir: file}); err == nil {
		t.Error("a file as the data directory")
	}
	// state is a file
	d2 := filepath.Join(dir, "d2")
	_ = os.MkdirAll(d2, 0o755)
	_ = os.WriteFile(filepath.Join(d2, "state"), []byte("x"), 0o644)
	if _, err := Open(Options{Dir: d2}); err == nil {
		t.Error("a file where the state directory should be")
	}
	// a history that fails while the service runs is a notice, and a save has happened anyway
	if os.Getuid() == 0 {
		t.Skip("root writes into a read-only directory")
	}
	d3 := filepath.Join(dir, "d3")
	s, _ := open(t, d3)
	s.Flush()
	objects := filepath.Join(d3, "tree", ".git", "objects")
	_ = os.Chmod(objects, 0o555)
	v, err := s.CreateVariable(&model.Variable{Name: "X", Type: model.VarBool, Visible: true}, nil, admin)
	if err != nil {
		t.Fatalf("a save failed with the history: %v", err)
	}
	s.Flush()
	if n := s.Notices(); !strings.Contains(n.History, "could not be written") {
		t.Errorf("the history's notice: %q", n.History)
	}
	_ = os.Chmod(objects, 0o755)
	if _, err := s.UpdateVariable(v.Key, &model.Variable{Name: "Y", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	s.Flush()
	if n := s.Notices(); n.History != "" {
		t.Errorf("the notice stays: %q", n.History)
	}
	_ = s.Close()
}

// The settings are a file of the tree; the location reaches the engine, an import brings one
// only where none is set, and a damaged file is not used.
func TestSettings(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	if s.Settings().Location != nil || s.Engine().Position().Known {
		t.Error("a location out of nothing")
	}
	if err := s.SetSettings(model.Settings{Location: &model.Location{Latitude: 91}}, repo.Author{Name: "t"}); codeOf(err) != "invalid" {
		t.Errorf("an impossible location: %v", err)
	}
	// an import brings the CCU's location
	res, err := s.ApplyImport(Import{Location: &model.Location{Latitude: 52.52, Longitude: 13.405}}, repo.Author{Name: "t"})
	if err != nil || !res.Location || s.Settings().Location.Latitude != 52.52 || !s.Engine().Position().Known {
		t.Fatalf("an import's location: %+v %v", res, err)
	}
	// the next one does not replace it, and an impossible one is left out
	res, err = s.ApplyImport(Import{Location: &model.Location{Latitude: 1, Longitude: 1}}, repo.Author{Name: "t"})
	if err != nil || res.Location || s.Settings().Location.Latitude != 52.52 {
		t.Fatalf("a second import's location: %+v %v", res, err)
	}
	if err := s.SetSettings(model.Settings{Location: &model.Location{Latitude: -33.87, Longitude: 151.21}}, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	// the same again changes nothing and writes no history
	if err := s.SetSettings(model.Settings{Location: &model.Location{Latitude: -33.87, Longitude: 151.21}}, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	s.queue.Flush()
	if h, err := s.History(model.SettingsPath, 10); err != nil || len(h) != 2 || h[0].Message != "Settings changed" {
		t.Errorf("the history of the settings: %+v %v", h, err)
	}
	s.Close()

	// they are there after a restart
	s, _ = open(t, dir)
	if l := s.Settings().Location; l == nil || l.Longitude != 151.21 || s.Engine().Position().Lat != -33.87 {
		t.Errorf("after a restart: %+v", l)
	}
	if err := s.SetSettings(model.Settings{}, repo.Author{Name: "t"}); err != nil || s.Engine().Position().Known {
		t.Errorf("no location: %v", err)
	}
	res, err = s.ApplyImport(Import{Location: &model.Location{Latitude: 99}}, repo.Author{Name: "t"})
	if err != nil || res.Location || s.Settings().Location != nil {
		t.Errorf("an import's impossible location: %+v %v", res, err)
	}
	s.Close()

	// the loop guard and the pause between write commands reach the engine and the writer
	var gaps []time.Duration
	s, err = Open(Options{Dir: dir, Pacer: func(d time.Duration) { gaps = append(gaps, d) }})
	if err != nil {
		t.Fatal(err)
	}
	if g := s.Engine().LoopGuard(); g != engine.DefaultLoopGuard || len(gaps) != 1 || gaps[0] != 300*time.Millisecond {
		t.Errorf("the defaults: %+v %v", g, gaps)
	}
	if err := s.SetSettings(model.Settings{PacingMS: 50}, repo.Author{Name: "t"}); codeOf(err) != "invalid" {
		t.Errorf("a pause that is too short: %v", err)
	}
	if err := s.SetSettings(model.Settings{PacingMS: 500, LoopGuard: &model.LoopGuard{Runs: 5, WindowSeconds: 3}}, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	if g := s.Engine().LoopGuard(); g != (engine.LoopGuard{Runs: 5, Window: 3 * time.Second, Pause: time.Minute}) || gaps[len(gaps)-1] != 500*time.Millisecond {
		t.Errorf("set: %+v %v", g, gaps)
	}
	s.Close()
	s, err = Open(Options{Dir: dir, Pacer: func(d time.Duration) { gaps = append(gaps, d) }})
	if err != nil {
		t.Fatal(err)
	}
	if g := s.Engine().LoopGuard(); g.Runs != 5 || gaps[len(gaps)-1] != 500*time.Millisecond || s.Settings().PacingMS != 500 {
		t.Errorf("after a restart: %+v %v", g, gaps)
	}
	s.Close()

	// a damaged file is said and not used
	if err := os.WriteFile(filepath.Join(dir, "tree", model.SettingsPath), []byte("location: here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, logs := open(t, dir)
	defer s.Close()
	if s.Settings().Location != nil || !strings.Contains(strings.Join(*logs, "|"), "the settings are not used") {
		t.Errorf("a damaged file: %+v %v", s.Settings(), *logs)
	}
}

// Saves from several sides while the history is recorded in the background: no save waits for a
// commit, no commit fails over a file that changes under it, and when all is quiet the history
// has every file as the tree has it.
func TestSavesWhileTheHistoryIsRecorded(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Options{Dir: dir, Clock: engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// no commit may fail, not even one that the next would make up for
	var failed []string
	var failedMu sync.Mutex
	s.queue.OnError = func(err error) {
		failedMu.Lock()
		failed = append(failed, err.Error())
		failedMu.Unlock()
	}
	if _, err := s.CreateVariable(list("Modus", "Sommer", "Winter"), nil, admin); err != nil {
		t.Fatal(err)
	}
	rounds := 150
	if testing.Short() {
		rounds = 30
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			name := fmt.Sprintf("Programm %d", w)
			key := fmt.Sprintf("programm-%d", w)
			for i := 0; i < rounds; i++ {
				var err error
				switch i % 4 {
				case 0:
					_, err = s.CreateProgram(uses(name, "Winter"), admin)
				case 1, 2:
					p := uses(name, []string{"Sommer", "Winter"}[i%3%2])
					p.Note = strings.Repeat("a line of text ", 1+(i*31+w)%200)
					_, err = s.UpdateProgram(key, p, admin)
				default:
					err = s.DeleteProgram(key, admin)
				}
				if err != nil {
					errs <- fmt.Errorf("worker %d, round %d: %w", w, i, err)
					return
				}
				if i%10 == 0 {
					if _, err := s.UpdateVariable("modus", list("Modus", "Sommer", "Winter", fmt.Sprintf("Runde %d-%d", w, i)), nil, admin); err != nil {
						errs <- err
						return
					}
				}
			}
			_, err := s.CreateProgram(uses(name, "Winter"), admin)
			if err != nil {
				errs <- err
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	s.Flush()
	failedMu.Lock()
	defer failedMu.Unlock()
	if len(failed) > 0 || s.queue.Err() != nil {
		t.Fatalf("%d commits failed while the tree was written, the first: %v", len(failed), failed)
	}
	if n := s.Notices(); n.History != "" {
		t.Errorf("the history's notice: %q", n.History)
	}
	head, err := s.History("", 1)
	if err != nil || len(head) != 1 {
		t.Fatalf("the history: %v %v", head, err)
	}
	paths := []string{store.VariablePath("modus")}
	for w := 0; w < 4; w++ {
		paths = append(paths, store.ProgramPath(fmt.Sprintf("programm-%d", w)))
	}
	for _, p := range paths {
		want, err := s.Tree().Read(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.queue.Repo().FileAt(head[0].Hash, p)
		if err != nil || string(got) != string(want) {
			t.Errorf("%s: the history has %d bytes (%v), the tree %d", p, len(got), err, len(want))
		}
	}
}

func TestTheRunTraceSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	clock := engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	var logs []string
	start := func() *Service {
		s, err := Open(Options{Dir: dir, Clock: clock, Log: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := start()
	if _, err := s.CreateVariable(&model.Variable{Name: "Licht", Type: model.VarBool, Visible: true}, false, admin); err != nil {
		t.Fatal(err)
	}
	p := &model.Program{Name: "Später", Active: true, Visible: true, Operable: true, Rules: []model.Rule{{
		When: &model.Condition{Op: model.All, Groups: []model.Group{}},
		Then: []model.Action{{Kind: model.ActionVariable, Variable: "licht", To: true, After: model.Duration(time.Minute)}}}}}
	if _, err := s.CreateProgram(p, admin); err != nil {
		t.Fatal(err)
	}
	run := func(s *Service) {
		t.Helper()
		if err := s.Engine().RunProgram("spaeter", model.RunThen); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, "state", "trace.jsonl")
	lines := func() int {
		b, err := os.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(b), "\n")
	}
	// an entry goes to disk once it is settled: the interface has answered by then
	run(s)
	if err := s.SaveState(false); err != nil || lines() != 0 {
		t.Fatalf("a fresh entry was written (%d lines), %v", lines(), err)
	}
	clock.Advance(11 * time.Second)
	run(s)
	if err := s.SaveState(false); err != nil || lines() != 1 {
		t.Fatalf("%d lines, %v", lines(), err)
	}
	if err := s.SaveState(false); err != nil || lines() != 1 {
		t.Fatalf("written twice: %d lines, %v", lines(), err)
	}
	// the stop writes the rest; the next start has all of it and counts on
	if err := s.Close(); err != nil || lines() != 2 {
		t.Fatalf("after the stop: %d lines, %v", lines(), err)
	}
	// (a line cut short by a power cut is left out)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"seq":3,"program":"spaet`)
	f.Close()
	s = start()
	back := s.Engine().Trace(0)
	if len(back) != 2 || back[0].Seq != 1 || back[1].Steps[0].What != engine.StepScheduled || len(s.Engine().Runs("spaeter")) != 2 {
		t.Fatalf("after the restart: %+v", back)
	}
	// the delays came back with the state: the one that comes due notes it on the step that
	// scheduled it, before the restart
	clock.Advance(50 * time.Second)
	back = s.Engine().Trace(0)
	if len(back) != 3 || !back[2].Delayed || back[2].Seq != 3 || back[0].Steps[0].Ran.IsZero() {
		t.Fatalf("the delay that came due: %+v", back)
	}
	if err := s.Close(); err != nil || lines() != 4 { // the cut line stays, closed off
		t.Fatalf("after the second stop: %d lines, %v", lines(), err)
	}
	// a ring that cannot be read or written: said, and the addon runs without it
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0o755); err != nil {
		t.Fatal(err)
	}
	logs = nil
	s = start()
	if len(logs) == 0 || !strings.Contains(strings.Join(logs, "\n"), "the run trace could not be read") {
		t.Errorf("logs: %v", logs)
	}
	run(s)
	if err := s.Close(); err == nil {
		t.Error("a ring that cannot be written is no error")
	}
}

// Computed values through the service: made with their inputs checked, kept free of circles,
// their inputs protected from removal and from another type, and calculated by the engine.
func TestComputedValues(t *testing.T) {
	dir := t.TempDir()
	s, logs := open(t, dir)
	num := func(name string) *model.Variable {
		return &model.Variable{Name: name, Type: model.VarNumber, Visible: true}
	}
	computed := func(name, fn string, inputs ...string) *model.Variable {
		v := num(name)
		v.Computed = &model.Computed{Function: fn}
		for _, in := range inputs {
			v.Computed.Inputs = append(v.Computed.Inputs, model.Input{Variable: in})
		}
		return v
	}
	for _, name := range []string{"A", "B"} {
		if _, err := s.CreateVariable(num(name), 2.0, admin); err != nil {
			t.Fatal(err)
		}
	}
	// an initial value is not taken: the value is the inputs'
	sum, err := s.CreateVariable(computed("Summe", model.FnSum, "a", "b"), 99.0, admin)
	if err != nil || sum.Key != "summe" || sum.Computed == nil {
		t.Fatalf("create: %+v %v", sum, err)
	}
	if got, _ := s.Engine().Value("summe"); got != 4.0 {
		t.Errorf("the sum at once: %v", got)
	}
	if err := s.Engine().SetVariable("a", 5.5, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Engine().Value("summe"); got != 7.5 {
		t.Errorf("the sum follows: %v", got)
	}
	if err := s.Engine().SetVariable("summe", 1.0, engine.FromUser); err == nil {
		t.Error("a computed value was written")
	}
	// refused: an input that does not exist, one that is no number, a definition the reader refuses
	if _, err := s.CreateVariable(computed("Weg", model.FnSum, "a", "nope"), nil, admin); codeOf(err) != "invalid" || !strings.Contains(err.Error(), "nope does not exist") {
		t.Errorf("a missing input: %v", err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "Flag", Type: model.VarBool, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVariable(computed("Falsch", model.FnSum, "a", "flag"), nil, admin); codeOf(err) != "invalid" || !strings.Contains(err.Error(), "not a number") {
		t.Errorf("an input that is no number: %v", err)
	}
	if _, err := s.CreateVariable(computed("Eins", model.FnDewPoint, "a"), nil, admin); codeOf(err) != "invalid" {
		t.Errorf("one input for a dew point: %v", err)
	}
	// a chain is fine, a circle is not
	if _, err := s.CreateVariable(computed("Kette", model.FnMax, "summe", "b"), nil, admin); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Engine().Value("kette"); got != 7.5 {
		t.Errorf("the chain: %v", got)
	}
	var e *Error
	if _, err := s.UpdateVariable("summe", computed("Summe", model.FnSum, "a", "kette"), nil, admin); !errors.As(err, &e) || e.Code != "invalid" || !strings.Contains(e.Msg, "lead back to itself") {
		t.Errorf("a circle: %v", err)
	}
	// an input cannot be removed, and cannot become another type
	if err := s.DeleteVariable("a", admin); !errors.As(err, &e) || e.Code != "in-use" || !reflect.DeepEqual(e.Users, []string{"Summe"}) {
		t.Errorf("removing an input: %v", err)
	}
	if _, err := s.UpdateVariable("a", &model.Variable{Name: "A", Type: model.VarString, Visible: true}, nil, admin); !errors.As(err, &e) || e.Code != "in-use" || !reflect.DeepEqual(e.Users, []string{"Summe"}) {
		t.Errorf("another type for an input: %v", err)
	}
	// renamed and given a unit, it stays what it is; made a plain number, it can be written
	if v, err := s.UpdateVariable("kette", func() *model.Variable { v := computed("Größter", model.FnMax, "summe", "b"); v.Unit = "W"; return v }(), nil, admin); err != nil || v.Name != "Größter" || v.Computed == nil {
		t.Errorf("update: %+v %v", v, err)
	}
	if _, err := s.UpdateVariable("kette", num("Größter"), nil, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Engine().SetVariable("kette", 1.0, engine.FromUser); err != nil {
		t.Errorf("no longer computed: %v", err)
	}
	if err := s.DeleteVariable("kette", admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVariable("summe", admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVariable("a", admin); err != nil {
		t.Errorf("an input nobody uses any more: %v", err)
	}
	// a file edited by hand into a circle is said in the log, and nothing hangs
	for key, in := range map[string]string{"eins": "zwei", "zwei": "eins"} {
		text := "name: " + key + "\ntype: number\ncomputed:\n  function: sum\n  inputs:\n    - variable: " + in + "\n    - variable: b\n"
		if err := os.WriteFile(filepath.Join(dir, "tree", "variables", key+".yaml"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(*logs, "\n"); !strings.Contains(joined, "variable %s is not computed: %s") {
		t.Errorf("the log: %s", joined)
	}
	if err := s.Engine().SetVariable("b", 3.0, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Engine().Value("eins"); got != 0.0 {
		t.Errorf("a value in a circle: %v", got)
	}
}
