package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const helperPSM = "HmIP-RF/000A1B2C3D4E5F:6"

func helperKeys(s *Service) []string {
	var out []string
	for _, v := range s.Variables() {
		if v.Helper != nil {
			out = append(out, v.Key+"="+v.Helper.Role)
		}
	}
	return out
}

// A device helper is made in one step - its variables, one history entry - counts from the
// device's next report, survives a restart with what it remembers, and goes in one step.
func TestHelperLifecycle(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	in := HelperInput{Kind: model.HelperEnergy, Channel: helperPSM, Unit: "Wh",
		Names: map[string]string{"total": "Energiezähler Waschmaschine", "today": "Energie heute Waschmaschine", "yesterday": "Energie gestern Waschmaschine"}}
	vars, err := s.CreateHelper(in, admin)
	if err != nil || len(vars) != 3 {
		t.Fatalf("create: %v %v", vars, err)
	}
	if v := vars[0]; v.Key != "energiezaehler-waschmaschine" || v.Unit != "Wh" || v.Type != model.VarNumber || !v.Visible || *v.Helper != (model.Helper{Kind: model.HelperEnergy, Channel: helperPSM, Role: model.RoleTotal}) {
		t.Errorf("the total: %+v %+v", v, v.Helper)
	}
	if got := helperKeys(s); !reflect.DeepEqual(got, []string{"energie-gestern-waschmaschine=yesterday", "energie-heute-waschmaschine=today", "energiezaehler-waschmaschine=total"}) {
		t.Errorf("variables: %v", got)
	}
	entries, _ := s.History("", 0)
	if len(entries) == 0 || entries[0].Message != "Device helper Energiezähler Waschmaschine created" {
		t.Errorf("history: %+v", entries)
	}
	// it counts from the device's next report
	ref, _ := model.ParseRef(helperPSM + "/ENERGY_COUNTER")
	s.Engine().DeviceEvent(ref, 5000.0)
	s.Engine().DeviceEvent(ref, 5001.5)
	if got, _ := s.Engine().Value("energiezaehler-waschmaschine"); got != 1.5 {
		t.Errorf("total: %v", got)
	}
	// refused: a second helper of the kind on the channel, a kind, a role, a channel, a lead that is missing
	for name, c := range map[string]struct {
		in   HelperInput
		code string
	}{
		"twice":      {in, "exists"},
		"no kind":    {HelperInput{Kind: "heat", Channel: helperPSM, Names: map[string]string{"total": "X"}}, "invalid"},
		"no channel": {HelperInput{Kind: model.HelperRain, Channel: "x", Names: map[string]string{"total": "X"}}, "invalid"},
		"no role":    {HelperInput{Kind: model.HelperRainHM, Channel: "BidCos-RF/OEQ1:1", Names: map[string]string{"total": "X", "today": "Y"}}, "invalid"},
		"no lead":    {HelperInput{Kind: model.HelperRain, Channel: "HmIP-RF/000B:1", Names: map[string]string{"today": "X"}}, "invalid"},
		"bad name":   {HelperInput{Kind: model.HelperRain, Channel: "HmIP-RF/000B:1", Names: map[string]string{"total": "a\nb"}}, "invalid"},
	} {
		if _, err := s.CreateHelper(c.in, admin); codeOf(err) != c.code {
			t.Errorf("%s: %v", name, err)
		}
	}
	// the kind's own unit when none is given, and only the variables that are named
	rain, err := s.CreateHelper(HelperInput{Kind: model.HelperRain, Channel: "HmIP-RF/000B1B2C3D4E5F:1", Names: map[string]string{"total": "Regen"}}, admin)
	if err != nil || len(rain) != 1 || rain[0].Unit != "mm" {
		t.Fatalf("rain: %+v %v", rain, err)
	}

	// the total alone cannot go while the helper has other variables; a day's amount can
	if err := s.DeleteVariable("energiezaehler-waschmaschine", admin); codeOf(err) != "in-use" || !reflect.DeepEqual(err.(*Error).Users, []string{"Energie gestern Waschmaschine", "Energie heute Waschmaschine"}) {
		t.Errorf("delete the total: %v", err)
	}
	if err := s.DeleteVariable("energie-gestern-waschmaschine", admin); err != nil {
		t.Errorf("delete yesterday: %v", err)
	}
	// a variable cannot take a place in a helper that is taken, or join one that has no total
	twice := &model.Variable{Name: "Zweiter", Type: model.VarNumber, Visible: true, Helper: &model.Helper{Kind: model.HelperEnergy, Channel: helperPSM, Role: model.RoleTotal}}
	if _, err := s.CreateVariable(twice, nil, admin); codeOf(err) != "invalid" || !strings.Contains(err.Error(), "already holds") {
		t.Errorf("a second total: %v", err)
	}
	orphan := &model.Variable{Name: "Waise", Type: model.VarNumber, Visible: true, Helper: &model.Helper{Kind: model.HelperWater, Channel: "HmIP-RF/000C:2", Role: model.RoleToday}}
	if _, err := s.CreateVariable(orphan, nil, admin); codeOf(err) != "invalid" || !strings.Contains(err.Error(), "no variable for its total") {
		t.Errorf("a day's amount without a total: %v", err)
	}
	// ... but a day's amount of a helper that is there is made like any variable
	back := &model.Variable{Name: "Energie gestern", Type: model.VarNumber, Visible: true, Helper: &model.Helper{Kind: model.HelperEnergy, Channel: helperPSM, Role: model.RoleYesterday}}
	if _, err := s.CreateVariable(back, nil, admin); err != nil {
		t.Errorf("yesterday again: %v", err)
	}
	// changing: off and a new name are fine; moving the total away from its day's amounts is not
	total := s.Variables()[2]
	if total.Key != "energiezaehler-waschmaschine" {
		t.Fatalf("order: %s", total.Key)
	}
	off := total.Clone()
	off.Helper.Off, off.Name = true, "Zähler Waschmaschine"
	if _, err := s.UpdateVariable(off.Key, off, nil, admin); err != nil {
		t.Errorf("off: %v", err)
	}
	if h := s.Engine().Helpers(); len(h) != 2 || !h[0].Off {
		t.Errorf("helpers: %+v", h)
	}
	moved := total.Clone()
	moved.Helper.Channel = "HmIP-RF/000F1B2C3D4E5F:6"
	if _, err := s.UpdateVariable(moved.Key, moved, nil, admin); codeOf(err) != "invalid" {
		t.Errorf("moved away: %v", err)
	}
	dup := s.Variables()[0].Clone() // "Energie gestern": yesterday
	dup.Helper.Role = model.RoleToday
	if _, err := s.UpdateVariable(dup.Key, dup, nil, admin); codeOf(err) != "invalid" {
		t.Errorf("a role that is taken: %v", err)
	}

	// what the helper remembers survives a restart
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, _ = open(t, dir)
	var energy engine.HelperInfo
	for _, h := range s.Engine().Helpers() {
		if h.Kind == model.HelperEnergy {
			energy = h
		}
	}
	if energy.State.Old != 5001.5 || !energy.State.Seen || !energy.Off {
		t.Errorf("after the restart: %+v", energy)
	}

	// removing: refused while a program uses a variable, then everything in one step
	p := uses("liest", "x")
	p.Rules[0].When.Groups[0].Lines[0] = model.Line{Variable: "energie-heute-waschmaschine", Op: model.OpGt, Value: 5.0, Trigger: model.TriggerChange}
	p.Rules[0].Then = []model.Action{{Kind: model.ActionVariable, Variable: "energiezaehler-waschmaschine", To: 0.0}}
	if _, err := s.CreateProgram(p, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHelper("energie-gestern", admin); codeOf(err) != "in-use" || len(err.(*Error).Users) == 0 {
		t.Errorf("in use: %v", err)
	}
	if err := s.DeleteProgram("liest", admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteHelper("gone", admin); codeOf(err) != "not-found" {
		t.Errorf("no helper: %v", err)
	}
	if err := s.DeleteHelper("energie-gestern", admin); err != nil { // by any of its variables
		t.Fatal(err)
	}
	if got := helperKeys(s); !reflect.DeepEqual(got, []string{"regen=total"}) {
		t.Errorf("left: %v", got)
	}
	entries, _ = s.History("", 0)
	if entries[0].Message != "Device helper Zähler Waschmaschine removed" {
		t.Errorf("history: %+v", entries[0])
	}
	if err := s.DeleteHelper("regen", admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// An import brings helpers with what they remember; a hand-made file that takes a place twice
// is said so in the log and left out.
func TestHelperImportAndBadFiles(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	hv := func(key, role string) *model.Variable {
		return &model.Variable{Key: key, Name: key, Type: model.VarNumber, Visible: true, Helper: &model.Helper{Kind: model.HelperEnergy, Channel: helperPSM, Role: role}}
	}
	_, err := s.ApplyImport(Import{Variables: []*model.Variable{hv("sv-total", model.RoleTotal), hv("sv-today", model.RoleToday)},
		Values:  map[string]any{"sv-total": 100.5, "sv-today": 12.5},
		Helpers: map[string]model.HelperState{"sv-total": {Seen: true, Old: 211.75}, "sv-today": {Seen: true, Old: 1}}}, admin)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := model.ParseRef(helperPSM + "/ENERGY_COUNTER")
	s.Engine().DeviceEvent(ref, 212.25)
	if total, _ := s.Engine().Value("sv-total"); total != 101.0 {
		t.Errorf("the total continues: %v", total)
	}
	if today, _ := s.Engine().Value("sv-today"); today != 13.0 {
		t.Errorf("today continues: %v", today)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tree", "variables", "aa-twice.yaml"), []byte("name: Zweimal\ntype: number\nhelper:\n  kind: energy\n  channel: "+helperPSM+"\n  role: today\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, logs := open(t, dir)
	found := false
	for _, l := range *logs {
		found = found || strings.Contains(l, "not kept by its device helper")
	}
	if !found {
		t.Errorf("logs: %v", *logs)
	}
	if h := s.Engine().Helpers(); len(h) != 1 || h[0].Variables["today"] != "aa-twice" {
		t.Errorf("helpers: %+v", h)
	}
	_ = s.Close()
}

// A power cut in the middle of a write leaves the write's temporary file: the next start
// removes it and says so, and the state it reads is the last whole one.
func TestOpenRemovesWhatACutOffWriteLeft(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	if _, err := s.CreateVariable(&model.Variable{Name: "Zahl", Type: model.VarNumber, Visible: true}, 5.0, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	left := []string{filepath.Join(dir, "state", ".tmp-2350104642"), filepath.Join(dir, "tree", "variables", ".tmp-17")}
	for _, f := range left {
		if err := os.WriteFile(f, []byte(`{"variables":`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s, logs := open(t, dir)
	for _, f := range left {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s is still there", f)
		}
	}
	said := false
	for _, l := range *logs {
		said = said || strings.Contains(l, "temporary files a cut-off write left")
	}
	if !said {
		t.Errorf("logs: %v", *logs)
	}
	if v, _ := s.Engine().Value("zahl"); v != 5.0 {
		t.Errorf("the value: %v", v)
	}
	for _, l := range *logs {
		if strings.Contains(l, ".tmp-") && !strings.Contains(l, "cut-off write") {
			t.Errorf("the leftover was read as a file: %s", l)
		}
	}
	_ = s.Close()
}
