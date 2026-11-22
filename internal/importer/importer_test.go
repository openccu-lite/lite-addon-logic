package importer

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func berlin(t testing.TB) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func source(t testing.TB, name string) *regadom.Source {
	t.Helper()
	f, err := os.Open("../regadom/testdata/" + name + ".regadom")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, err := regadom.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run the tests with -update to write it)", err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("%s differs from the golden file:\n%s", name, got)
	}
}

func program(res *Result, key string) *model.Program {
	for _, p := range res.Programs {
		if p.Key == key {
			return p
		}
	}
	return nil
}

func variable(res *Result, key string) *model.Variable {
	for _, v := range res.Variables {
		if v.Key == key {
			return v
		}
	}
	return nil
}

func programReport(res *Result, name string) *ProgramReport {
	for i := range res.Report.Programs {
		if res.Report.Programs[i].Name == name {
			return &res.Report.Programs[i]
		}
	}
	return nil
}

func variableReport(res *Result, name string) *VariableReport {
	for i := range res.Report.Variables {
		if res.Report.Variables[i].Name == name {
			return &res.Report.Variables[i]
		}
	}
	return nil
}

func codes(notes []Note) string {
	var out []string
	for _, n := range notes {
		out = append(out, n.Code)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

// values writes the imported values one per line, sorted.
func values(res *Result) []byte {
	keys := make([]string, 0, len(res.Values))
	for k := range res.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %#v\n", k, res.Values[k])
	}
	return []byte(b.String())
}

// The fixture, whole: every file the import would write, the values, the ledger and the report in
// both languages are golden files. What the fixture's README promises is spelled out below.
func TestFixtureGolden(t *testing.T) {
	res := Import(source(t, "fixture"), Options{Location: berlin(t)})
	for _, p := range res.Programs {
		golden(t, "fixture/programs/"+p.Key+".yaml", model.EncodeProgram(p, nil))
	}
	for _, v := range res.Variables {
		golden(t, "fixture/variables/"+v.Key+".yaml", model.EncodeVariable(v))
	}
	golden(t, "fixture/values.txt", values(res))
	golden(t, "fixture/imports/ccu.yaml", res.Ledger.Encode())
	golden(t, "fixture/devices-seen.yaml", EncodeSeen(res.Seen))
	golden(t, "fixture/report.de.md", []byte(res.Report.Markdown("de")))
	golden(t, "fixture/report.en.md", []byte(res.Report.Markdown("en")))
	if !*update {
		// nothing in the golden tree that the import no longer writes
		for _, dir := range []string{"programs", "variables"} {
			files, _ := os.ReadDir(filepath.Join("testdata", "golden", "fixture", dir))
			n := len(res.Programs)
			if dir == "variables" {
				n = len(res.Variables)
			}
			if len(files) != n {
				t.Errorf("%s: %d golden files, %d written", dir, len(files), n)
			}
		}
	}
}

func TestDevicesGolden(t *testing.T) {
	res := Import(source(t, "devices"), Options{Location: berlin(t), Known: func(ch string) bool { return ch == "HmIP-RF/00111BE9AB9012:4" }})
	for _, p := range res.Programs {
		golden(t, "devices/programs/"+p.Key+".yaml", model.EncodeProgram(p, nil))
	}
	golden(t, "devices/devices-seen.yaml", EncodeSeen(res.Seen))
	golden(t, "devices/report.de.md", []byte(res.Report.Markdown("de")))
	golden(t, "devices/report.en.md", []byte(res.Report.Markdown("en")))
}

// import -> files -> reload gives the same objects, and writing them again the same bytes; the
// set has no problem; the CCU's own trigger table agrees with the triggers found.
func TestRoundTripStable(t *testing.T) {
	for _, name := range []string{"fixture", "devices"} {
		res := Import(source(t, name), Options{Location: berlin(t), Known: func(string) bool { return true }})
		set := model.NewSet()
		for _, v := range res.Variables {
			data := model.EncodeVariable(v)
			back, err := model.DecodeVariable(v.Key+".yaml", v.Key, data)
			if err != nil {
				t.Fatalf("%s: variable %s: %v", name, v.Key, err)
			}
			if !reflect.DeepEqual(back, v) || !bytes.Equal(model.EncodeVariable(back), data) {
				t.Errorf("%s: variable %s is not stable:\n%+v\n%+v", name, v.Key, v, back)
			}
			set.Variables[v.Key] = back
			if x, ok := res.Values[v.Key]; ok {
				if err := v.CheckValue(x); err != nil {
					t.Errorf("%s: the value of %s: %v", name, v.Key, err)
				}
			} else if v.Type != model.VarAlarm {
				t.Errorf("%s: %s has no value", name, v.Key)
			}
		}
		for _, p := range res.Programs {
			data := model.EncodeProgram(p, nil)
			back, err := model.DecodeProgram(p.Key+".yaml", p.Key, data)
			if err != nil {
				t.Fatalf("%s: program %s: %v\n%s", name, p.Key, err, data)
			}
			if !bytes.Equal(model.EncodeProgram(back, nil), data) {
				t.Errorf("%s: program %s is not stable", name, p.Key)
			}
			set.Programs[p.Key] = back
		}
		if problems := set.Problems(); len(problems) != 0 {
			t.Errorf("%s: %v", name, problems)
		}
		if len(res.Report.Events) != 0 {
			t.Errorf("%s: the trigger table disagrees: %v", name, res.Report.Events)
		}
		// the ledger reads back, and names every file
		back, err := DecodeLedger(res.Ledger.Encode())
		if err != nil || !reflect.DeepEqual(back, res.Ledger) {
			t.Errorf("%s: ledger: %v\n%+v\n%+v", name, err, back, res.Ledger)
		}
		if len(back.Programs) != len(res.Programs) || len(back.Variables) != len(res.Variables) {
			t.Errorf("%s: ledger sizes %d/%d", name, len(back.Programs), len(back.Variables))
		}
	}
}

// What the fixture's README says about each program.
func TestFixturePrograms(t *testing.T) {
	res := Import(source(t, "fixture"), Options{Location: berlin(t)})
	imported, inactive, skipped, vars := res.Report.Totals()
	if imported != 11 || inactive != 7 || skipped != 2 || vars != 38 || len(res.Programs) != 18 || len(res.Variables) != 38 {
		t.Fatalf("totals: %d imported, %d inactive, %d skipped, %d variables", imported, inactive, skipped, vars)
	}
	for name, want := range map[string]struct {
		status  Status
		active  bool
		reasons string
	}{
		"Markise bei Regen einfahren":            {Imported, true, ""},
		"Heizperiode schalten":                   {Imported, true, ""},
		"Vergleiche alle":                        {Imported, true, ""},
		"Alarm bei Wasser":                       {Imported, true, ""},
		"Rollläden morgens":                      {Imported, true, ""},
		"Rollläden abends":                       {Imported, true, ""},
		"Zisterne prüfen":                        {Imported, true, ""},
		"Kalender":                               {Imported, true, ""},
		"Szene Abend":                            {Imported, true, ""},  // its scripts are the two recognised one-liners
		"Nachtabsenkung":                         {Imported, false, ""}, // switched off on the CCU by Szene Abend
		"Inaktiv und unsichtbar":                 {Imported, false, ""},
		"Wetter holen":                           {Inactive, false, ReasonCommand},
		"zzL_Ia5":                                {Inactive, false, ReasonScript},
		"prgEnergyCounter_4711_000A1B2C3D4E5F:6": {Skipped, false, SkipCounter},
		"prgDailySunshineRainCounter_4730":       {Skipped, false, SkipCounter},
	} {
		r := programReport(res, name)
		if r == nil || r.Status != want.status || r.Active != want.active || codes(r.Reasons) != want.reasons {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// the first program of the README, line by line
	p := program(res, "markise-bei-regen-einfahren")
	want := &model.Program{Key: "markise-bei-regen-einfahren", Name: "Markise bei Regen einfahren", Active: true, Visible: true, Operable: true, AtStart: true,
		Rules: []model.Rule{
			{When: &model.Condition{Op: model.Any, Groups: []model.Group{
				{Op: model.All, Lines: []model.Line{
					{Variable: "regen", Op: model.OpEq, Value: true, Trigger: model.TriggerChange},
					{Variable: "aussentemperatur-mittel", Op: model.OpGt, Value: 5.0, Trigger: model.TriggerCheck}}},
				{Op: model.All, Lines: []model.Line{
					{Variable: "heizperiode", Op: model.OpEq, Value: "Sommer", Trigger: model.TriggerCheck},
					{Variable: "wassermelder-keller", Op: model.OpEq, Value: false, Trigger: model.TriggerUpdate}}}}},
				CancelPending: true,
				Then: []model.Action{
					{Kind: model.ActionVariable, Variable: "rollladen-status", To: "Beschattung"},
					{Kind: model.ActionVariable, Variable: "markise-hinweis", To: "wegen Regen eingefahren", After: model.Duration(30 * time.Second)}}},
			{Else: true, CancelPending: true, Then: []model.Action{{Kind: model.ActionVariable, Variable: "markise-hinweis", To: ""}}},
		}}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("Markise bei Regen einfahren:\n%s", model.EncodeProgram(p, nil))
	}
	if r := programReport(res, "Markise bei Regen einfahren"); codes(r.Notes) != "at-start,cancels,else-update,order" {
		t.Errorf("notes: %s", codes(r.Notes))
	}
	// WENN / SONST WENN / SONST WENN / empty SONST; AND between blocks; range and type 7; 2 h
	p = program(res, "heizperiode-schalten")
	if len(p.Rules) != 4 || !p.Rules[3].Else || len(p.Rules[3].Then) != 0 || p.Rules[2].When.Op != model.All || len(p.Rules[2].When.Groups) != 2 {
		t.Fatalf("Heizperiode schalten:\n%s", model.EncodeProgram(p, nil))
	}
	if l := p.Rules[1].When.Groups[0].Lines[0]; l.Op != model.OpRange || l.From != 5.0 || l.Below != 15.0 {
		t.Errorf("range: %+v", l)
	}
	if l := p.Rules[2].When.Groups[1].Lines[0]; l.Op != model.OpOutside || l.From != 30.0 || l.To != 35.0 || l.Trigger != model.TriggerCheck {
		t.Errorf("type 7: %+v", l)
	}
	if a := p.Rules[2].Then[0]; a.After != model.Duration(2*time.Hour) || a.To != "Sommer" {
		t.Errorf("delay: %+v", a)
	}
	if r := programReport(res, "Heizperiode schalten"); codes(r.Notes) != "at-start,change-in-elseif,outside-range" {
		t.Errorf("notes: %s", codes(r.Notes))
	}
	// one line per comparison type; a string, a list entry by its label, an alarm variable
	p = program(res, "vergleiche-alle")
	var ops []string
	p.Lines(func(_ int, l *model.Line) { ops = append(ops, fmt.Sprintf("%s %s %v", l.Variable, l.Op, l.Value)) })
	if got := strings.Join(ops, "; "); got != "zaehler-tuerklingel eq 3; zaehler-tuerklingel gt 10; zaehler-tuerklingel ge 20; zaehler-tuerklingel lt 1; zaehler-tuerklingel le 0; markise-hinweis eq Sturm; heizperiode eq Winter; alarm-wasser eq true" {
		t.Errorf("Vergleiche alle: %s", got)
	}
	// the two recognised one-line scripts in their three spellings
	p = program(res, "szene-abend")
	if p.Operable || p.Note != "ruft andere Programme per Skript auf" {
		t.Errorf("Szene Abend: %+v", p)
	}
	wantThen := []model.Action{
		{Kind: model.ActionRun, Program: "rolllaeden-abends", Mode: model.RunThen},
		{Kind: model.ActionDeactivate, Program: "nachtabsenkung"},
		{Kind: model.ActionActivate, Program: "markise-bei-regen-einfahren", After: model.Duration(10 * time.Second)}}
	if !reflect.DeepEqual(p.Rules[0].Then, wantThen) || !reflect.DeepEqual(p.Rules[1].Then, []model.Action{{Kind: model.ActionActivate, Program: "nachtabsenkung"}}) {
		t.Errorf("Szene Abend:\n%s", model.EncodeProgram(p, nil))
	}
	// a script that is one system.Exec with its output into a variable: a command the block
	// waits for, the program inactive
	p = program(res, "wetter-holen")
	if a := p.Rules[0].Then[0]; p.Active || a.Kind != model.ActionCommand || !a.Command.Shell || !a.Command.Wait || a.Command.Line != "wget -q -O - http://example.invalid/wetter" || a.Command.Output != "markise-hinweis" {
		t.Errorf("Wetter holen: %+v %+v", a, a.Command)
	}
	// a free script: kept, the program inactive
	p = program(res, "zzl-ia5")
	if a := p.Rules[0].Then[0]; p.Active || a.Kind != model.ActionScript || a.Script == "" {
		t.Errorf("zzL_Ia5: %+v", a)
	}
	// an empty condition, inactive, invisible, a description with umlauts
	p = program(res, "inaktiv-und-unsichtbar")
	if p.Active || p.Visible || p.Note != "Beschreibung mit Umlauten: äöüß" || len(p.Rules) != 1 || p.Rules[0].When == nil || len(p.Rules[0].When.Groups) != 0 {
		t.Errorf("Inaktiv und unsichtbar:\n%s", model.EncodeProgram(p, nil))
	}
	// time modules, as stored
	times := func(key string) []model.TimeModule {
		var out []model.TimeModule
		program(res, key).Lines(func(_ int, l *model.Line) {
			if l.Time != nil {
				out = append(out, *l.Time)
			}
		})
		return out
	}
	if got := times("rolllaeden-morgens"); !reflect.DeepEqual(got, []model.TimeModule{
		{Kind: "daily", At: "06:30", From: "2026-11-21"},
		// (the fixture's script set the period to 2, which the WebUI never does for a daily
		// module - "alle N Tage" is the repetition value - so this one is simply daily)
		{Kind: "daily", At: "07:00", From: "2026-12-01", Until: "2027-03-31"},
		{Kind: "daily", At: "07:15", From: "2026-11-21", Count: 10}}) {
		t.Errorf("Rollläden morgens: %+v", got)
	}
	if got := times("rolllaeden-abends"); !reflect.DeepEqual(got, []model.TimeModule{
		{Kind: "daily", Astro: "before_sunset", Offset: model.Duration(30 * time.Minute), Duration: model.Duration(time.Hour), From: "2026-11-21"},
		{Kind: "daily", Astro: "night", From: "2026-11-21"}}) {
		t.Errorf("Rollläden abends: %+v", got)
	}
	if got := times("nachtabsenkung"); !reflect.DeepEqual(got, []model.TimeModule{
		{Kind: "weekly", At: "22:00", Duration: model.Duration(8 * time.Hour), Weekdays: []string{"mon", "wed"}, From: "2026-11-21"}}) {
		t.Errorf("Nachtabsenkung: %+v", got)
	}
	if got := times("zisterne-pruefen"); !reflect.DeepEqual(got, []model.TimeModule{
		{Kind: "interval", AllDay: true, Interval: model.Duration(15 * time.Minute), From: "2026-11-21"},
		{Kind: "interval", At: "08:00", Duration: model.Duration(10 * time.Hour), Interval: model.Duration(30 * time.Minute), From: "2026-11-21"}}) {
		t.Errorf("Zisterne prüfen: %+v", got)
	}
	if got := times("kalender"); !reflect.DeepEqual(got, []model.TimeModule{
		{Kind: "monthly", At: "03:00", Day: 1, From: "2026-11-21"},
		{Kind: "yearly", At: "00:00", Day: 24, Month: 12, From: "2026-11-21"},
		{Kind: "once", At: "12:00", Date: "2027-01-01", From: "2026-11-21"},
		{Kind: "daily", Astro: "day", From: "2026-11-21"}}) {
		t.Errorf("Kalender: %+v", got)
	}
	// (its astro line is "nur prüfen": the module triggers all the same, and the report says so)
	if r := programReport(res, "Kalender"); codes(r.Notes) != "astro,at-start,time-check" {
		t.Errorf("Kalender notes: %s", codes(r.Notes))
	}
	if r := programReport(res, "Inaktiv und unsichtbar"); codes(r.Notes) != "no-trigger" {
		t.Errorf("empty condition: %s", codes(r.Notes))
	}
}

// What the fixture's README says about the variables.
func TestFixtureVariables(t *testing.T) {
	res := Import(source(t, "fixture"), Options{Location: berlin(t)})
	min, max := -40.0, 60.0
	for key, want := range map[string]*model.Variable{
		"regen":                   {Name: "Regen", Note: "Regensensor Terrasse", Type: model.VarBool, Labels: &model.Labels{False: "trocken", True: "es regnet"}, Visible: true, Logged: true},
		"heizperiode":             {Name: "Heizperiode", Note: "wird vom Programm Heizperiode schalten gesetzt", Type: model.VarList, Entries: []string{"Sommer", "Übergang", "Winter"}, Visible: true},
		"aussentemperatur-mittel": {Name: "Außentemperatur Mittel", Note: "Mittelwert, von außen geschrieben", Type: model.VarNumber, Unit: "°C", Min: &min, Max: &max, Visible: true},
		"markise-hinweis":         {Name: "Markise-Hinweis", Type: model.VarString, Visible: true},
		"intern-merker":           {Name: "intern Merker", Type: model.VarBool, Labels: &model.Labels{False: "ist falsch", True: "ist wahr"}, Internal: true},
		"alarm-einbruch":          {Name: "Alarm Einbruch", Note: "Alarmzone Erdgeschoss", Type: model.VarAlarm, Labels: &model.Labels{False: "nicht ausgelöst", True: "ausgelöst"}, Visible: true},
		"alarmzone-1":             {Name: "Alarmzone 1", Note: "Alarmmeldung Alarmzone 1", Type: model.VarAlarm, Labels: &model.Labels{False: "nicht ausgelöst", True: "ausgelöst"}, Visible: true},
		"anwesenheit":             {Name: "Anwesenheit", Note: "Anwesenheit", Type: model.VarBool, Labels: &model.Labels{False: "nicht anwesend", True: "anwesend"}, Visible: true, StartValue: true},
	} {
		want.Key = key
		if got := variable(res, key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n%s", key, model.EncodeVariable(got))
		}
	}
	for key, want := range map[string]any{
		"regen": false, "heizperiode": "Übergang", "aussentemperatur-mittel": 12.5, "markise-hinweis": "", "zzl-t": "x3:$src$@22:17:20 ",
		"zaehler-tuerklingel": 3.0, "svenergycounter-4711-000a1b2c3d4e5f-6": 100.5, "svhmipsunshinecountertoday-4730": 1546.75, "anwesenheit": false,
	} {
		if got, ok := res.Values[key]; !ok || got != want {
			t.Errorf("value of %s: %#v", key, got)
		}
	}
	if _, ok := res.Values["alarm-einbruch"]; ok {
		t.Error("an alarm variable got a value: the database holds none")
	}
	// eQ-3's counter variables: internal numbers with their values, recognised by name
	v := variable(res, "svenergycounteroldval-4711")
	if v == nil || !v.Internal || !v.Visible || v.Type != model.VarNumber || *v.Min != 0 || *v.Max < 1e299 {
		t.Errorf("counter variable: %+v", v)
	}
	counters := 0
	for _, r := range res.Report.Variables {
		if has(r.Notes, VarCounter) {
			counters++
		}
	}
	if counters != 16 {
		t.Errorf("%d counter variables recognised", counters)
	}
	for name, want := range map[string]string{
		"WatchDog: no-internet": SkipWatchdog, "DutyCycle": VarSystemValue, "Alarmmeldungen": VarSystemValue, "Servicemeldungen": VarSystemValue, "Gateway-DP": SkipSystem,
	} {
		if r := variableReport(res, name); r == nil || r.Status != Skipped || codes(r.Notes) != want {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if r := variableReport(res, "zzL_K"); codes(r.Notes) != VarAction || r.Value != "true" {
		t.Errorf("the key stand-in: %+v", r)
	}
	// the built-ins in English
	en := Import(source(t, "fixture"), Options{Location: berlin(t), Lang: "en"})
	if v := variable(en, "presence"); v == nil || v.Labels.True != "present" {
		t.Errorf("presence in English: %+v", v)
	}
}

// OQ-13: "alle inaktiv" switches off what would have been on, and says so.
func TestAllInactive(t *testing.T) {
	res := Import(source(t, "fixture"), Options{Location: berlin(t), AllInactive: true})
	for _, p := range res.Programs {
		if p.Active {
			t.Errorf("%s is active", p.Key)
		}
	}
	if r := programReport(res, "Alarm bei Wasser"); r.Status != Inactive || codes(r.Reasons) != ReasonAllInactive || !r.WasActive || has(r.Notes, NoteAtStart) {
		t.Errorf("%+v", r)
	}
	if r := programReport(res, "Nachtabsenkung"); r.Status != Imported || len(r.Reasons) != 0 {
		t.Errorf("a program that was off on the CCU needs no reason: %+v", r)
	}
	if r := programReport(res, "Wetter holen"); codes(r.Reasons) != ReasonCommand {
		t.Errorf("%+v", r)
	}
	if !strings.Contains(res.Report.Markdown("de"), "alle Programme inaktiv") {
		t.Error("the report does not say what was chosen")
	}
}

// A second import of the same source updates the same files; another source's keys are left alone.
func TestReimport(t *testing.T) {
	first := Import(source(t, "fixture"), Options{Location: berlin(t)})
	prev, err := DecodeLedger(first.Ledger.Encode())
	if err != nil {
		t.Fatal(err)
	}
	// renamed in the store since: the ledger's key wins over the name's slug
	prev.Programs[1233] = "markise"
	prev.Variables[1202] = "regen-terrasse"
	prev.Programs[9999] = "gone-on-the-ccu"
	taken := func(kind, key string) bool { return true } // everything is in the store already
	second := Import(source(t, "fixture"), Options{Location: berlin(t), Previous: prev, Taken: taken})
	if program(second, "markise") == nil || variable(second, "regen-terrasse") == nil {
		t.Fatal("the ledger's keys were not kept")
	}
	if l := program(second, "markise").Rules[0].When.Groups[0].Lines[0]; l.Variable != "regen-terrasse" {
		t.Errorf("the line names %q", l.Variable)
	}
	if a := program(second, "szene-abend").Rules[0].Then[2]; a.Program != "markise" {
		t.Errorf("the activity names %q", a.Program)
	}
	for _, r := range second.Report.Programs {
		if r.Status != Skipped && !r.Updated {
			t.Errorf("%s is not an update", r.Name)
		}
	}
	for _, r := range second.Report.Variables {
		if r.Status != Skipped && !r.Updated {
			t.Errorf("%s is not an update", r.Name)
		}
	}
	if len(second.Programs) != len(first.Programs) || second.Ledger.Programs[9999] != "gone-on-the-ccu" {
		t.Errorf("ledger: %+v", second.Ledger.Programs)
	}
	if !strings.Contains(second.Report.Markdown("en"), "updates the file of an earlier import") {
		t.Error("the report does not name the updates")
	}
	// another source with the same names: its own keys beside the first one's
	own := map[string]bool{}
	for _, p := range first.Programs {
		own["program/"+p.Key] = true
	}
	for _, v := range first.Variables {
		own["variable/"+v.Key] = true
	}
	other := Import(source(t, "fixture"), Options{Location: berlin(t), Source: "ferienhaus", Taken: func(kind, key string) bool { return own[kind+"/"+key] }})
	if program(other, "kalender-2") == nil || variable(other, "regen-2") == nil || program(other, "kalender") != nil {
		t.Error("the second source did not get keys of its own")
	}
	if l := program(other, "markise-bei-regen-einfahren-2").Rules[0].When.Groups[0].Lines[0]; l.Variable != "regen-2" {
		t.Errorf("the second source's line names %q", l.Variable)
	}
	if other.Ledger.Source != "ferienhaus" || other.Report.Source != "ferienhaus" {
		t.Errorf("source: %q", other.Ledger.Source)
	}
	// a ledger whose key is no key any more is ignored
	prev.Programs[1233] = "Not A Key"
	third := Import(source(t, "fixture"), Options{Location: berlin(t), Previous: prev})
	if program(third, "markise-bei-regen-einfahren") == nil {
		t.Error("an invalid ledger key was used")
	}
}

// Devices: references by address, what this system has and what it lacks, CUxD, a key line,
// negated lines, a special value, 3.89.14's start switch, unknown fields, dangling references.
func TestDevices(t *testing.T) {
	known := func(ch string) bool { return ch != "BidCos-RF/LEQ0123456:1" }
	res := Import(source(t, "devices"), Options{Location: berlin(t), Known: known})
	p := program(res, "markise-bei-wind")
	lines := p.Rules[0].When.Groups
	if l := lines[0].Lines[0]; l.Device.String() != "HmIP-RF/00111BE9AB9012:4/LEVEL" || l.Op != model.OpGt || l.Value != 0.5 || l.Trigger != model.TriggerChange {
		t.Errorf("device line: %+v", l)
	}
	if l := lines[0].Lines[1]; l.Device.Param != "PRESS_SHORT" || l.Trigger != model.TriggerUpdate || l.Value != true {
		t.Errorf("key line: %+v", l)
	}
	if l := lines[1].Lines[0]; !l.Not || l.Variable != "regen" || l.Trigger != model.TriggerCheck {
		t.Errorf("negated variable line: %+v", l)
	}
	if l := lines[1].Lines[1]; !l.Not || l.Op != model.OpRange || l.From != 0.2 || l.Below != 0.8 {
		t.Errorf("negated range: %+v", l)
	}
	then := p.Rules[0].Then
	if then[0].Kind != model.ActionSet || then[0].To != 0.0 || then[1].To != true || then[1].After != model.Duration(90*time.Second) || then[2].To != "Manuell" {
		t.Errorf("activities: %+v", then)
	}
	if a := p.Rules[1].Then[0]; a.Special != "[SV]lastValue" || a.To != nil {
		t.Errorf("special value: %+v", a)
	}
	if p.AtStart || p.Active {
		t.Errorf("startupexec 0 and a missing device: at_start %v, active %v", p.AtStart, p.Active)
	}
	r := programReport(res, "Markise bei Wind")
	if r.Status != Inactive || codes(r.Reasons) != "missing-device,special-value" || !reflect.DeepEqual(r.Reasons[1].Args, []string{"BidCos-RF/LEQ0123456:1"}) {
		t.Errorf("reasons: %+v", r.Reasons)
	}
	if codes(r.Notes) != "cancels,else-update,key-line,negated,not-at-start,order" {
		t.Errorf("notes: %s", codes(r.Notes))
	}
	// startupexec 1, CUxD
	p = program(res, "frostwarnung")
	if !p.AtStart || p.Active {
		t.Errorf("Frostwarnung: %+v", p)
	}
	if r := programReport(res, "Frostwarnung"); codes(r.Reasons) != ReasonCUxD {
		t.Errorf("%+v", r)
	}
	// an element a newer ReGa added: inactive, each named
	r = programReport(res, "Neues Feld")
	if r.Status != Inactive || len(r.Reasons) != 3 || r.Reasons[0].Code != ReasonUnknownField || r.Reasons[0].Args[0] != "program/runlimit" || r.Reasons[2].Args[0] != "s-cond/hysteresis" {
		t.Errorf("%+v", r)
	}
	// a reference into nothing: not imported, and not in the ledger
	r = programReport(res, "Verwaist")
	if r.Status != Skipped || codes(r.Reasons) != SkipDangling || r.Key != "" || program(res, "verwaist") != nil || len(res.Ledger.Programs) != 3 {
		t.Errorf("%+v", r)
	}
	// the device list, and what is noted about the missing ones
	if len(res.Report.Devices) != 4 || len(res.Seen) != 2 {
		t.Fatalf("devices: %+v", res.Report.Devices)
	}
	d := res.Report.Devices[3]
	if d.Channel != "HmIP-RF/00111BE9AB9012:4" || !d.Known || d.Name != "Markise Behang" || d.Device != "Markise Terrasse" || d.Type != "HmIP-BROLL" ||
		d.ChannelType != "BLIND_VIRTUAL_RECEIVER" || !reflect.DeepEqual(d.Rooms, []string{"Terrasse", "Flur"}) || !reflect.DeepEqual(d.Params, []string{"LEVEL", "STOP"}) ||
		!reflect.DeepEqual(d.Programs, []string{"Markise bei Wind"}) {
		t.Errorf("%+v", d)
	}
	if res.Seen[0].Channel != "BidCos-RF/LEQ0123456:1" || res.Seen[1].Channel != "CUxD/CUX2801001:1" || res.Report.Devices[1].Known {
		t.Errorf("seen: %+v", res.Seen)
	}
	// a variable bound to a channel, a repaired name, the location of the database
	if v := variable(res, "regen"); v.Channel != "CUxD/CUX2801001:1" {
		t.Errorf("channel binding: %q", v.Channel)
	}
	if v := variable(res, "kueche-licht"); v == nil || v.Name != "Küche Licht" || len(res.Report.Repairs) != 1 {
		t.Errorf("repaired name: %+v", v)
	}
	if res.Report.Location.Latitude != "52.52" {
		t.Errorf("location: %+v", res.Report.Location)
	}
	// without a device list every device counts as missing
	none := Import(source(t, "devices"), Options{Location: berlin(t)})
	if r := programReport(none, "Markise bei Wind"); len(r.Reasons[1].Args) != 3 || len(none.Seen) != 4 {
		t.Errorf("no device list: %+v", r.Reasons)
	}
}

func TestLedger(t *testing.T) {
	l := &Ledger{Source: "ccu", Programs: map[int]string{}, Variables: map[int]string{7: "a"}}
	if got := string(l.Encode()); got != "source: ccu\nprograms: {}\nvariables:\n  7: a\n" {
		t.Errorf("%q", got)
	}
	l = &Ledger{Source: "ccu", Serial: "12345678", Firmware: "3.89.8", Programs: map[int]string{20: "b", 3: "a"}, Variables: map[int]string{}}
	if got := string(l.Encode()); got != "source: ccu\nserial: \"12345678\"\nfirmware: \"3.89.8\"\nprograms:\n  3: a\n  20: b\nvariables: {}\n" {
		t.Errorf("%q", got)
	}
	back, err := DecodeLedger(l.Encode())
	if err != nil || !reflect.DeepEqual(back, l) {
		t.Errorf("%+v %v", back, err)
	}
	if back, err := DecodeLedger([]byte("source: x\n")); err != nil || back.Programs == nil || back.Variables == nil {
		t.Errorf("%+v %v", back, err)
	}
	for _, bad := range []string{"source: Not A Key\n", "source: x\nprograms:\n  1: Not A Key\n", "source: x\nextra: 1\n", "source: [\n", ""} {
		if _, err := DecodeLedger([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "import ledger: ") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	l.Merge(nil)
	l.Merge(&Ledger{Programs: map[int]string{3: "other", 4: "c"}, Variables: map[int]string{9: "v"}})
	if l.Programs[3] != "a" || l.Programs[4] != "c" || l.Variables[9] != "v" {
		t.Errorf("merge: %+v", l)
	}
	if !ValidSource("ferienhaus") || ValidSource("../x") {
		t.Error("ValidSource")
	}
}

func TestSeen(t *testing.T) {
	list := []SeenDevice{
		{Channel: "HmIP-RF/B:2", Name: "b", Rooms: []string{"Bad"}, Params: []string{"STATE"}},
		{Channel: "BidCos-RF/A:1", Name: "a", Device: "A", Type: "HM-X", ChannelType: "KEY"}}
	data := EncodeSeen(list)
	back, err := DecodeSeen(data)
	if err != nil || len(back) != 2 || back[0].Channel != "BidCos-RF/A:1" || !reflect.DeepEqual(back[1], list[0]) {
		t.Errorf("%+v %v\n%s", back, err, data)
	}
	if got := string(EncodeSeen(nil)); got != "devices: []\n" {
		t.Errorf("%q", got)
	}
	if back, err := DecodeSeen(EncodeSeen(nil)); err != nil || len(back) != 0 {
		t.Errorf("%+v %v", back, err)
	}
	for _, bad := range []string{"devices:\n  - channel: nope\n", "devices: 3\n", "other: 1\n"} {
		if _, err := DecodeSeen([]byte(bad)); err == nil || !strings.HasPrefix(err.Error(), "devices-seen: ") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	merged := MergeSeen(list, []SeenDevice{{Channel: "HmIP-RF/B:2", Name: "neu"}, {Channel: "HmIP-RF/C:3"}})
	if len(merged) != 3 || merged[1].Name != "neu" || merged[2].Channel != "HmIP-RF/C:3" {
		t.Errorf("%+v", merged)
	}
}

// Every code a report can carry has words in both languages, and they differ.
func TestTexts(t *testing.T) {
	for _, code := range Codes() {
		de, en := Text("de", code, "X"), Text("en", code, "X")
		if de == "" || en == "" || de == en {
			t.Errorf("%s: %q / %q", code, de, en)
		}
		if strings.Contains(de+en, "Box") || strings.Contains(de+en, " box") {
			t.Errorf("%s says box", code)
		}
	}
	for _, code := range []string{ReasonScript, ReasonCUxD, ReasonMissing, ReasonSpecial, ReasonUnknownField, ReasonComparison, ReasonTrigger, ReasonValue,
		ReasonOperators, ReasonDelay, ReasonAllInactive, ReasonListIndex, SkipCounter, SkipLeftover, SkipUnreadable, SkipDangling, SkipInvalid,
		NoteAtStart, NoteNotAtStart, NoteElseUpdate, NoteCancels, NoteOrder, NoteDST, NoteAstro, NoteKeyLine, NoteOutside, NoteZeroDelay, NoteElseIfChange,
		NoteNoTrigger, NoteNegated, NotePattern, VarSystemValue, VarPresence, VarCounter, VarCounterAddr, VarCounterPlain, VarHelper, VarHelperState, VarHelperUnused, VarHelperDay, VarAlarm, VarEntries, VarListValue,
		VarAction, VarLimits, SkipWatchdog, SkipSystem, SkipUnknownType, EventMissing, EventExtra} {
		if _, ok := texts[code]; !ok {
			t.Errorf("%s has no words", code)
		}
	}
	if got := Text("de", ReasonMissing, "a", "", "b"); got != "Geräte fehlen auf diesem System: a, b" {
		t.Errorf("%q", got)
	}
	if got := Text("en", "no-such-code", "x"); got != "no-such-code x" {
		t.Errorf("%q", got)
	}
	// an empty report still has every section
	md := (&Report{Source: "ccu", Kind: "sbk", Firmware: "3.89.8", Location: Location{Timezone: "Europe/Berlin"}, Services: 3}).Markdown("xx")
	for _, want := range []string{"# Importbericht", "Systemsicherung (.sbk)", "Firmware der Zentrale: 3.89.8", "Zeitzone: Europe/Berlin", "3 Servicemeldungen", "## Programme\n\nkeine", "## Systemvariablen\n\nkeine"} {
		if !strings.Contains(md, want) {
			t.Errorf("the empty report lacks %q:\n%s", want, md)
		}
	}
	if got := cell("a|b\nc"); got != `a\|b c` {
		t.Errorf("%q", got)
	}
}
