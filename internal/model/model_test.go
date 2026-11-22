package model

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func names(r Ref) string {
	if r.Address == "00111BE9AB9012:4" {
		return "Markise\n Terrasse"
	}
	return ""
}

// The file with everything in it is canonical: reading and writing it gives the same bytes, and
// the page's JSON form carries the same program.
func TestProgramRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/full.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := DecodeProgram("programs/markise.yaml", "markise", data)
	if err != nil {
		t.Fatal(err)
	}
	out := EncodeProgram(p, names)
	if !bytes.Equal(out, data) {
		t.Fatalf("not canonical:\n%s", diff(string(data), string(out)))
	}
	if p.Key != "markise" || !p.Active || p.Visible || p.Operable || !p.Internal || !p.AtStart || len(p.Rules) != 3 {
		t.Fatalf("program %+v", p)
	}
	r := p.Rules[0]
	if r.When.Op != Any || len(r.When.Groups) != 2 || r.When.Groups[0].Op != All || !r.CancelPending || r.Note != "der erste Block" {
		t.Fatalf("rule %+v", r)
	}
	l := r.When.Groups[0].Lines[1]
	if l.Device.String() != "HmIP-RF/00111BE9AB9012:4/LEVEL" || l.Device.Channel() != "HmIP-RF/00111BE9AB9012:4" || l.Op != OpGt || l.Value != 0.0 || l.Trigger != TriggerCheck {
		t.Fatalf("line %+v", l)
	}
	g := r.When.Groups[1].Lines
	if !g[0].Not || g[0].Value != "Winter" || g[1].From != -5.0 || g[1].Below != 15.5 || g[2].To != 35.0 || g[3].Value != "on" {
		t.Fatalf("lines %+v", g)
	}
	if tm := g[4].Time; tm.Kind != "daily" || tm.At != "06:30" || tm.Duration.Std() != 90*time.Minute || tm.Every != 2 || tm.Count != 10 || tm.Until != "2027-03-31 12:00" {
		t.Fatalf("time %+v", tm)
	}
	if tm := g[5].Time; tm.Astro != "before_sunset" || tm.Offset.Std() != 30*time.Minute || !reflect.DeepEqual(tm.Weekdays, []string{"mon", "wed"}) {
		t.Fatalf("time %+v", tm)
	}
	a := r.Then
	if a[1].After.Std() != 30*time.Second || a[1].Note != "zwei Zeilen\nNotiz" || a[2].To != "" || a[3].FromVariable != "aussentemperatur" || a[4].FromDevice == nil || a[5].Special != "OLD_LEVEL" {
		t.Fatalf("actions %+v", a)
	}
	if a[7].Kind != ActionRun || a[7].Mode != RunThen || a[9].Kind != ActionActivate || a[10].Kind != ActionDeactivate || !strings.Contains(a[11].Script, "\n\nWriteLine") {
		t.Fatalf("actions %+v", a)
	}
	if a[13].OnFor.Std() != 5*time.Minute || a[13].After != 0 || a[13].AfterUpTo.Std() != 30*time.Minute || a[14].After.Std() != 10*time.Minute || a[14].AfterUpTo.Std() != time.Hour {
		t.Fatalf("actions %+v", a[13:])
	}
	if g[1].For.Std() != 10*time.Minute || !g[6].Time.Point || g[6].Time.Twilight != "civil" {
		t.Fatalf("for, point, twilight: %+v %+v", g[1], g[6].Time)
	}
	if !p.Rules[1].When.empty() || !p.Rules[2].Else || p.Rules[2].When != nil {
		t.Fatalf("rules %+v", p.Rules[1:])
	}
	// without names no comments; and the comment is never read back
	plainOut := EncodeProgram(p, nil)
	if bytes.Contains(plainOut, []byte("#")) {
		t.Errorf("a comment without a namer:\n%s", plainOut)
	}
	p2, err := DecodeProgram("x.yaml", "markise", plainOut)
	if err != nil || !reflect.DeepEqual(p, p2) {
		t.Fatalf("the comments changed the program: %v", err)
	}
	// JSON for the page and back
	js, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var p3 Program
	if err := json.Unmarshal(js, &p3); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(EncodeProgram(&p3, names), data) {
		t.Errorf("JSON changed the program:\n%s", diff(string(data), string(EncodeProgram(&p3, names))))
	}
	// Check is the reader's validation; Clone is deep
	c := p.Clone()
	if checked, err := c.Check(); err != nil || !reflect.DeepEqual(checked, p) {
		t.Fatalf("check: %v", err)
	}
	c.Rules[0].When.Groups[0].Lines[1].Device.Param = "X"
	c.Rules[0].Then[0].Device.Param = "X"
	c.Rules[0].Then[4].FromDevice.Param = "X"
	c.Rules[0].When.Groups[1].Lines[5].Time.Weekdays[0] = "sun"
	if !bytes.Equal(EncodeProgram(p, names), data) {
		t.Error("Clone shares memory with its source")
	}
}

func (c *Condition) empty() bool { return c != nil && len(c.Groups) == 0 }

func diff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) && i < len(g); i++ {
		if w[i] != g[i] {
			return "line " + itoa(i+1) + "\nwant: " + w[i] + "\ngot:  " + g[i]
		}
	}
	return "lengths differ: want " + itoa(len(w)) + " lines, got " + itoa(len(g))
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestMinimalProgram(t *testing.T) {
	p, err := DecodeProgram("p.yaml", "p", []byte("name: P\nrules: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Active || !p.Visible || !p.Operable || p.Internal || p.AtStart || len(p.Rules) != 0 {
		t.Fatalf("defaults: %+v", p)
	}
	if got := string(EncodeProgram(p, nil)); got != "name: P\nrules: []\n" {
		t.Errorf("got %q", got)
	}
	// a rule without a condition object is written as an always-true one
	q := &Program{Key: "q", Name: "Q", Visible: true, Operable: true, Rules: []Rule{{Then: []Action{{Kind: ActionSet}}}, {When: &Condition{Op: Any, Groups: []Group{{Op: All}}}}}}
	if got := string(EncodeProgram(q, nil)); !strings.Contains(got, "  - when:\n      all: []\n    then:\n      - set: //\n") || !strings.Contains(got, "        - all: []\n") {
		t.Errorf("got %s", got)
	}
}

// Strings that YAML would read as something else are written in quotes, and every string comes
// back as it was.
func TestStrings(t *testing.T) {
	for _, s := range []string{"x", "", " lead", "trail ", "true", "False", "yes", "No", "on", "OFF", "null", "~", "123", "1.5", "0x1F", "1e3", "06:30", "2026-12-01",
		"a: b", "a #b", "#x", "- x", "[x]", "{x}", "x\ty", "x\ny", "x\r", "Übergang", "\"q\"", "'q'", "back\\slash", "&a", "*a", "!t", "|", ">", "%x", "@x", "`x", "a,b", "?", "k:",
		"\x01", "\x7f", "\u0085", "\u2028", "\ufeff", "end:", ".inf", ".nan", "-", "--", "---", "...", "=", "<<", "wegen Regen eingefahren", "bad\xffutf"} {
		p := &Program{Key: "p", Name: "n", Note: s, Visible: true, Operable: true, Rules: []Rule{{When: &Condition{Op: All}, Then: []Action{{Kind: ActionVariable, Variable: "v", To: s}, {Kind: ActionScript, Script: s}}}}}
		out := EncodeProgram(p, nil)
		got, err := DecodeProgram("p.yaml", "p", out)
		if strings.Contains(s, "\xff") {
			// not UTF-8: written with the replacement character, never an unreadable file
			if err != nil {
				t.Errorf("%q: %v", s, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v\n%s", s, err, out)
			continue
		}
		if got.Note != s || got.Rules[0].Then[0].To != any(s) || got.Rules[0].Then[1].Script != s {
			t.Errorf("%q came back as %q / %q / %q\n%s", s, got.Note, got.Rules[0].Then[0].To, got.Rules[0].Then[1].Script, out)
		}
		if !bytes.Equal(EncodeProgram(got, nil), out) {
			t.Errorf("%q is not stable", s)
		}
	}
	// multi-line texts that cannot be a block literal are quoted
	for _, s := range []string{"a\nb\n", "a \nb", " a\nb", "\na", "a\n\x01b", "a\n\u2028"} {
		if literal(s) {
			t.Errorf("%q as a literal", s)
		}
	}
	if scalar(struct{}{}) != `"{}"` {
		t.Errorf("an unknown value: %s", scalar(struct{}{}))
	}
}

func TestNumbers(t *testing.T) {
	for in, want := range map[float64]string{0: "0", -40: "-40", 12.5: "12.5", 65000: "65000", 1e300: "1e+300", 0.1: "0.1", -0.000001: "-1e-06", 1e15: "1e+15"} {
		if got := number(in); got != want {
			t.Errorf("%v: %s, want %s", in, got, want)
		}
		v := &Variable{Key: "v", Name: "V", Type: VarNumber, Visible: true, Max: &in}
		got, err := DecodeVariable("v.yaml", "v", EncodeVariable(v))
		if err != nil || *got.Max != in {
			t.Errorf("%v did not come back: %v", in, err)
		}
	}
}

func TestVariableRoundTrip(t *testing.T) {
	min, max := -40.0, 60.0
	for _, v := range []*Variable{
		{Key: "regen", Name: "Regen", Note: "Regensensor Terrasse", Type: VarBool, Labels: &Labels{False: "trocken", True: "es regnet"}, Visible: true, Logged: true, StartValue: true},
		{Key: "heizperiode", Name: "Heizperiode", Type: VarList, Entries: []string{"Sommer", "Übergang", "Winter", "on"}, Visible: true, StartValue: "Winter"},
		{Key: "temp", Name: "Außentemperatur Mittel", Type: VarNumber, Unit: "°C", Min: &min, Max: &max, Visible: true, Channel: "HmIP-RF/000A1B2C3D4E5F:1", StartValue: 1.5},
		{Key: "hinweis", Name: "Markise-Hinweis", Type: VarString, Visible: true, StartValue: ""},
		{Key: "merker", Name: "intern Merker", Type: VarBool, Internal: true, Labels: &Labels{True: "ist wahr"}},
		{Key: "alarm-wasser", Name: "Alarm Wasser", Type: VarAlarm, Labels: &Labels{False: "nicht ausgelöst", True: "ausgelöst"}, Visible: true},
	} {
		out := EncodeVariable(v)
		got, err := DecodeVariable("variables/"+v.Key+".yaml", v.Key, out)
		if err != nil {
			t.Fatalf("%s: %v\n%s", v.Key, err, out)
		}
		if !reflect.DeepEqual(got, v) {
			t.Errorf("%s: %+v, want %+v\n%s", v.Key, got, v, out)
		}
		if !bytes.Equal(EncodeVariable(got), out) {
			t.Errorf("%s is not stable", v.Key)
		}
		if c, err := v.Clone().Check(); err != nil || !reflect.DeepEqual(c, v) {
			t.Errorf("%s: check %v", v.Key, err)
		}
	}
	want := "name: Regen\nnote: Regensensor Terrasse\ntype: bool\nlabels:\n  \"false\": trocken\n  \"true\": es regnet\nlogged: true\nstart_value: true\n"
	if got := string(EncodeVariable(&Variable{Key: "regen", Name: "Regen", Note: "Regensensor Terrasse", Type: VarBool, Labels: &Labels{False: "trocken", True: "es regnet"}, Visible: true, Logged: true, StartValue: true})); got != want {
		t.Errorf("got\n%s", got)
	}
	// empty labels are no labels; a clone is deep
	v, err := DecodeVariable("v.yaml", "v", []byte("name: V\ntype: bool\nlabels: {}\n"))
	if err != nil || v.Labels != nil {
		t.Errorf("empty labels: %+v %v", v, err)
	}
	src := &Variable{Key: "l", Name: "L", Type: VarList, Entries: []string{"a"}, Labels: &Labels{True: "x"}, Min: &min, Max: &max}
	c := src.Clone()
	c.Entries[0], c.Labels.True, *c.Min, *c.Max = "b", "y", 1, 2
	if src.Entries[0] != "a" || src.Labels.True != "x" || *src.Min != -40 || *src.Max != 60 {
		t.Error("Clone shares memory with its source")
	}
}

func TestDefaultsAndValues(t *testing.T) {
	five := 5.0
	neg := -5.0
	for _, c := range []struct {
		v    Variable
		want any
	}{
		{Variable{Type: VarBool}, false}, {Variable{Type: VarAlarm}, false},
		{Variable{Type: VarList, Entries: []string{"a", "b"}}, "a"}, {Variable{Type: VarList}, ""},
		{Variable{Type: VarNumber}, 0.0}, {Variable{Type: VarNumber, Min: &five}, 5.0}, {Variable{Type: VarNumber, Min: &neg}, 0.0},
		{Variable{Type: VarString}, ""},
	} {
		if got := c.v.Default(); got != c.want {
			t.Errorf("%+v: default %v, want %v", c.v, got, c.want)
		}
	}
	list := &Variable{Key: "l", Type: VarList, Entries: []string{"a", "b"}}
	if list.EntryIndex("b") != 1 || list.EntryIndex("c") != -1 {
		t.Error("EntryIndex")
	}
	for _, c := range []struct {
		t  VarType
		x  any
		ok bool
	}{
		{VarBool, true, true}, {VarBool, 1.0, false}, {VarAlarm, false, true}, {VarAlarm, "x", false},
		{VarNumber, 1.5, true}, {VarNumber, "1", false}, {VarString, "", true}, {VarString, 1.0, false},
		{VarList, "a", true}, {VarList, "c", false}, {VarList, 0.0, false}, {VarType(""), "a", false},
	} {
		v := &Variable{Key: "l", Type: c.t, Entries: []string{"a", "b"}}
		if err := v.CheckValue(c.x); (err == nil) != c.ok {
			t.Errorf("%s %v: %v", c.t, c.x, err)
		}
	}
}

func TestRefsDurationsKeys(t *testing.T) {
	for _, s := range []string{"HmIP-RF/000A1B2C3D4E5F:1/STATE", "BidCos-RF/NEQ0123456:12/LEVEL", "VirtualDevices/INT0000001:3/PRESS_SHORT", "Logic/var-regen:1/VALUE", "CUxD/CUX2801001:1/CMD_EXEC"} {
		r, err := ParseRef(s)
		if err != nil || r.String() != s || r.IsZero() {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"", "a/b", "HmIP-RF/000A1B2C3D4E5F/STATE", "HmIP-RF/000A:1/", "/a:1/b", "1x/a:1/b", "a/b:1/c/d", "a/b:x/c", "a/b c:1/d"} {
		if _, err := ParseRef(s); err == nil {
			t.Errorf("%q is taken for a datapoint", s)
		}
	}
	if !(Ref{}).IsZero() {
		t.Error("IsZero")
	}
	if ParseChannel("HmIP-RF/000A1B2C3D4E5F:1") != nil || ParseChannel("HmIP-RF/000A1B2C3D4E5F") == nil || ParseChannel("x") == nil {
		t.Error("ParseChannel")
	}
	for s, want := range map[string]time.Duration{"30s": 30 * time.Second, "5min": 5 * time.Minute, "2h": 2 * time.Hour, "1h30min": 90 * time.Minute, "1h1min1s": 3661 * time.Second, "90s": 90 * time.Second, "25h": 25 * time.Hour, "0s": 0} {
		d, err := ParseDuration(s)
		if err != nil || d.Std() != want {
			t.Errorf("%s: %v %v", s, d, err)
		}
	}
	for _, s := range []string{"", "5", "5m", "1.5h", "30s5min", "-5s", "5 min", "99999999999999999999s", "9999999999h"} {
		if _, err := ParseDuration(s); err == nil {
			t.Errorf("%q is taken for a duration", s)
		}
	}
	for d, want := range map[time.Duration]string{0: "0s", -time.Second: "0s", 90 * time.Second: "1min30s", 3600 * time.Second: "1h", 3661 * time.Second: "1h1min1s", 500 * time.Millisecond: "0s", 26 * time.Hour: "26h"} {
		if got := Duration(d).String(); got != want {
			t.Errorf("%v: %s, want %s", d, got, want)
		}
	}
	var d Duration
	if err := json.Unmarshal([]byte(`"1h30min"`), &d); err != nil || d.Std() != 90*time.Minute {
		t.Errorf("duration from JSON: %v %v", d, err)
	}
	if err := json.Unmarshal([]byte(`"soon"`), &d); err == nil {
		t.Error("\"soon\" is taken for a duration")
	}
	if b, _ := json.Marshal(Duration(30 * time.Second)); string(b) != `"30s"` {
		t.Errorf("duration to JSON: %s", b)
	}
	for name, want := range map[string]string{
		"Markise bei Regen einfahren": "markise-bei-regen-einfahren", "Rollläden abends": "rolllaeden-abends", "Außentemperatur Mittel": "aussentemperatur-mittel",
		"  -- ": "x", "": "x", "${sysVarPresence}": "sysvarpresence", "svEnergyCounter_4711_000A1B2C3D4E5F:6": "svenergycounter-4711-000a1b2c3d4e5f-6",
		"Über Ärger Öl Çé": "ueber-aerger-oel-ce", "日本語 x": "x",
		"ein sehr langer Name der mehr als achtundvierzig Zeichen hat und abgeschnitten wird": "ein-sehr-langer-name-der-mehr-als-achtundvierzig",
	} {
		got := Slug(name)
		if got != want || !ValidKey(got) {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
	for _, k := range []string{"", "-a", "a-", "A", "a_b", "a--b", "a b", strings.Repeat("a", 49)} {
		if ValidKey(k) {
			t.Errorf("%q is taken for a key", k)
		}
	}
	taken := map[string]bool{"regen": true, "regen-2": true, strings.Repeat("a", 48): true}
	if got := UniqueKey("Regen", func(k string) bool { return taken[k] }); got != "regen-3" {
		t.Errorf("unique: %s", got)
	}
	if got := UniqueKey("Wasser", func(k string) bool { return taken[k] }); got != "wasser" {
		t.Errorf("unique: %s", got)
	}
	if got := UniqueKey(strings.Repeat("a", 60), func(k string) bool { return taken[k] }); got != strings.Repeat("a", 46)+"-2" || !ValidKey(got) {
		t.Errorf("unique: %s", got)
	}
}

func TestTargets(t *testing.T) {
	for want, a := range map[string]Action{
		"dp:HmIP-RF/A:1/STATE": {Kind: ActionSet, Device: &Ref{"HmIP-RF", "A:1", "STATE"}},
		"var:regen":            {Kind: ActionVariable, Variable: "regen"},
		"prg:p":                {Kind: ActionRun, Program: "p"},
		"":                     {Kind: ActionScript, Script: "x"},
	} {
		if got := a.Target(); got != want {
			t.Errorf("%+v: %q", a, got)
		}
	}
	if (Action{Kind: ActionSet}).Target() != "" || (Action{Kind: ActionActivate, Program: "p"}).Target() != "prg:p" || (Action{Kind: ActionDeactivate, Program: "p"}).Target() != "prg:p" {
		t.Error("targets")
	}
}

// What a time module's fields must be, taken together.
func TestTimeModuleCheck(t *testing.T) {
	for _, c := range []struct {
		name string
		def  TimeModule
		msg  string // "" for a module that is fine
	}{
		{"a daily point", TimeModule{Kind: "daily", At: "06:30"}, ""},
		{"all day", TimeModule{Kind: "daily", AllDay: true}, ""},
		{"astro with offset and duration", TimeModule{Kind: "daily", Astro: "before_sunset", Offset: Duration(30 * time.Minute), Duration: Duration(time.Hour)}, ""},
		{"workdays", TimeModule{Kind: "daily", At: "06:30", Weekdays: []string{"mon", "tue"}}, ""},
		{"an interval", TimeModule{Kind: "interval", AllDay: true, Interval: Duration(time.Minute)}, ""},
		{"once", TimeModule{Kind: "once", At: "12:00", Date: "2027-01-01"}, ""},
		{"weekly, every second week", TimeModule{Kind: "weekly", At: "12:00", Weekdays: []string{"mon"}, Every: 2, From: "2026-11-02"}, ""},
		{"monthly on a day", TimeModule{Kind: "monthly", At: "12:00", Day: 31}, ""},
		{"monthly on the n-th weekday", TimeModule{Kind: "monthly", At: "12:00", Nth: 5, Weekdays: []string{"fri"}}, ""},
		{"yearly", TimeModule{Kind: "yearly", At: "12:00", Day: 24, Month: 12}, ""},
		{"yearly on the n-th weekday", TimeModule{Kind: "yearly", At: "12:00", Nth: 4, Weekdays: []string{"thu"}, Month: 11}, ""},
		{"a window", TimeModule{Kind: "daily", At: "12:00", From: "2026-12-01", Until: "2026-12-01 00:01", Count: 3}, ""},

		{"nothing says when", TimeModule{Kind: "daily"}, "exactly one of"},
		{"at and astro", TimeModule{Kind: "daily", At: "06:30", Astro: "day"}, "exactly one of"},
		{"at and all day", TimeModule{Kind: "daily", At: "06:30", AllDay: true}, "exactly one of"},
		{"an offset without astro", TimeModule{Kind: "daily", At: "06:30", Offset: Duration(time.Minute)}, "belongs to"},
		{"an offset on sunrise", TimeModule{Kind: "daily", Astro: "sunrise", Offset: Duration(time.Minute)}, "has no \"offset\""},
		{"an offset on sunset", TimeModule{Kind: "daily", Astro: "sunset", Offset: Duration(time.Minute)}, "has no \"offset\""},
		{"an offset of a day", TimeModule{Kind: "daily", Astro: "day", Offset: Duration(24 * time.Hour)}, "less than a day"},
		{"a negative offset", TimeModule{Kind: "daily", Astro: "day", Offset: Duration(-time.Minute)}, "less than a day"},
		{"a range of eight days", TimeModule{Kind: "daily", At: "06:30", Duration: MaxTimeRange + Duration(time.Second)}, "at most"},
		{"a negative range", TimeModule{Kind: "daily", At: "06:30", Duration: Duration(-time.Second)}, "at most"},
		{"all day with a duration", TimeModule{Kind: "daily", AllDay: true, Duration: Duration(time.Hour)}, "no \"duration\""},
		{"an interval without one", TimeModule{Kind: "interval", AllDay: true}, "only that"},
		{"a daily with an interval", TimeModule{Kind: "daily", AllDay: true, Interval: Duration(time.Minute)}, "only that"},
		{"every on yearly", TimeModule{Kind: "yearly", At: "12:00", Day: 1, Month: 1, Every: 2, From: "2026-01-01"}, "has no \"every\""},
		{"weekdays on once", TimeModule{Kind: "once", At: "12:00", Date: "2027-01-01", Weekdays: []string{"mon"}}, "has no \"weekdays\""},
		{"weekdays on an interval", TimeModule{Kind: "interval", AllDay: true, Interval: Duration(time.Minute), Weekdays: []string{"mon"}}, "has no \"weekdays\""},
		{"a day on daily", TimeModule{Kind: "daily", At: "12:00", Day: 3}, "has no \"day\""},
		{"nth on weekly", TimeModule{Kind: "weekly", At: "12:00", Weekdays: []string{"mon"}, Nth: 2}, "has no \"nth\""},
		{"a month on monthly", TimeModule{Kind: "monthly", At: "12:00", Day: 3, Month: 2}, "has no \"month\""},
		{"a date on daily", TimeModule{Kind: "daily", At: "12:00", Date: "2027-01-01"}, "has no \"date\""},
		{"once without a date", TimeModule{Kind: "once", At: "12:00"}, "needs a \"date\""},
		{"once with a time in the date", TimeModule{Kind: "once", At: "12:00", Date: "2027-01-01 12:00"}, "the day only"},
		{"weekly without weekdays", TimeModule{Kind: "weekly", At: "12:00"}, "needs \"weekdays\""},
		{"yearly without a month", TimeModule{Kind: "yearly", At: "12:00", Day: 1}, "needs a \"month\""},
		{"monthly without a day", TimeModule{Kind: "monthly", At: "12:00"}, "or on the \"nth\""},
		{"monthly with a day and a weekday", TimeModule{Kind: "monthly", At: "12:00", Day: 1, Weekdays: []string{"mon"}}, "or on the \"nth\""},
		{"nth without a weekday", TimeModule{Kind: "monthly", At: "12:00", Nth: 2}, "exactly one weekday"},
		{"nth with two weekdays", TimeModule{Kind: "monthly", At: "12:00", Nth: 2, Weekdays: []string{"mon", "tue"}}, "exactly one weekday"},
		{"a weekday without nth", TimeModule{Kind: "yearly", At: "12:00", Month: 3, Weekdays: []string{"mon"}}, "exactly one weekday"},
		{"every without from", TimeModule{Kind: "daily", At: "12:00", Every: 2}, "counted from"},
		{"count without from", TimeModule{Kind: "daily", At: "12:00", Count: 2}, "counted from"},
		{"until before from", TimeModule{Kind: "daily", At: "12:00", From: "2026-12-02 00:01", Until: "2026-12-01"}, "not after"},
		{"until with a time, at from", TimeModule{Kind: "daily", At: "12:00", From: "2026-12-01", Until: "2026-12-01 00:00"}, "not after"},
	} {
		err := c.def.Check()
		switch {
		case c.msg == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.msg == "":
			// and it reads back as it was written
			def := c.def
			p := &Program{Key: "p", Name: "P", Visible: true, Operable: true, Rules: []Rule{{When: &Condition{Op: All, Groups: []Group{{Op: All, Lines: []Line{{Time: &def, Trigger: TriggerUpdate}}}}}}}}
			back, err := DecodeProgram("programs/p.yaml", "p", EncodeProgram(p, nil))
			if err != nil || !reflect.DeepEqual(back.Rules[0].When.Groups[0].Lines[0].Time, &def) {
				t.Errorf("%s: read back as %+v (%v)\n%s", c.name, back, err, EncodeProgram(p, nil))
			}
		case c.msg != "" && (err == nil || !strings.Contains(err.Error(), c.msg)):
			t.Errorf("%s: %v, want %q", c.name, err, c.msg)
		}
	}
}

func TestSettings(t *testing.T) {
	s := &Settings{Location: &Location{Latitude: 52.52, Longitude: 13.405}}
	text := string(EncodeSettings(s))
	if !strings.Contains(text, "location:\n  latitude: 52.52\n  longitude: 13.405\n") {
		t.Errorf("written as:\n%s", text)
	}
	back, err := DecodeSettings([]byte(text))
	if err != nil || !reflect.DeepEqual(back, s) {
		t.Errorf("read back: %+v %v", back, err)
	}
	for _, empty := range []string{"", "\n", "# nothing\n", string(EncodeSettings(&Settings{}))} {
		if got, err := DecodeSettings([]byte(empty)); err != nil || got.Location != nil {
			t.Errorf("%q: %+v %v", empty, got, err)
		}
	}
	for text, msg := range map[string]string{
		"location:\n  latitude: 91\n  longitude: 0\n":   "latitude",
		"location:\n  latitude: 0\n  longitude: -181\n": "longitude",
		"location:\n  latitude: .nan\n  longitude: 0\n": "latitude",
		"location:\n  latitude: north\n":                "settings.yaml",
		"position: 1\n":                                 "settings.yaml",
		"location: [\n":                                 "settings.yaml",
		"pacing_ms: 50\n":                               "between 100 and 10000",
		"pacing_ms: 20000\n":                            "between 100 and 10000",
		"loop_guard:\n  runs: 1\n":                      "runs",
		"loop_guard:\n  window_seconds: 601\n":          "window_seconds",
		"loop_guard:\n  pause_seconds: -1\n":            "pause_seconds",
	} {
		if _, err := DecodeSettings([]byte(text)); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%q: %v", text, err)
		}
	}
}

// The pause between write commands and the loop guard: unset they are the defaults, and only
// what is set is in the file.
func TestSettingsPacingAndLoopGuard(t *testing.T) {
	var s Settings
	if s.Pacing() != 300 || s.Guard() != (LoopGuard{Runs: 20, WindowSeconds: 10, PauseSeconds: 60}) {
		t.Errorf("the defaults: %d %+v", s.Pacing(), s.Guard())
	}
	s = Settings{PacingMS: 500, LoopGuard: &LoopGuard{Runs: 5, PauseSeconds: 120}}
	if s.Pacing() != 500 || s.Guard() != (LoopGuard{Runs: 5, WindowSeconds: 10, PauseSeconds: 120}) {
		t.Errorf("set: %d %+v", s.Pacing(), s.Guard())
	}
	text := string(EncodeSettings(&s))
	if !strings.Contains(text, "\npacing_ms: 500\n") || !strings.Contains(text, "\nloop_guard:\n  runs: 5\n  pause_seconds: 120\n") || strings.Contains(text, "window_seconds") {
		t.Errorf("written as:\n%s", text)
	}
	back, err := DecodeSettings([]byte(text))
	if err != nil || !reflect.DeepEqual(back, &s) {
		t.Errorf("read back: %+v %v", back, err)
	}
	// a loop guard without a field is not written
	if text := string(EncodeSettings(&Settings{LoopGuard: &LoopGuard{}})); strings.Contains(text, "loop_guard") {
		t.Errorf("an empty loop guard:\n%s", text)
	}
	all := Settings{LoopGuard: &LoopGuard{Runs: 1000, WindowSeconds: 600, PauseSeconds: 86400}, PacingMS: 100}
	if err := all.Check(); err != nil {
		t.Errorf("the limits themselves: %v", err)
	}
}
