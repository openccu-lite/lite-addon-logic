package harness

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestScenarios is the comparison itself: every scenario's engine side against the recording of
// the reference. A scenario without a recording fails - record it with cmd/regaharness.
func TestScenarios(t *testing.T) {
	seen := map[string]bool{}
	for _, sc := range Scenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			if err := sc.Check(); err != nil {
				t.Fatal(err)
			}
			if seen[sc.Name] {
				t.Fatalf("the name %s is used twice", sc.Name)
			}
			seen[sc.Name] = true
			if sc.Quirk == "" || sc.Title == "" {
				t.Fatal("a scenario names its quirk and says what it shows")
			}
			dir := filepath.Join("testdata", sc.Name)
			raw, err := os.ReadFile(filepath.Join(dir, "rega.trace"))
			if err != nil {
				t.Fatalf("no recording of the reference (go run ./cmd/regaharness %s): %v", sc.Name, err)
			}
			rec, err := DecodeRecording(raw)
			if err != nil {
				t.Fatal(err)
			}
			packed, err := os.ReadFile(filepath.Join(dir, "homematic.regadom.gz"))
			if err != nil {
				t.Fatal(err)
			}
			dom, err := Gunzip(packed)
			if err != nil {
				t.Fatal(err)
			}
			im, err := Import(sc, dom)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := os.ReadFile(filepath.Join(dir, "programs.yaml"))
			if got := im.YAML(sc); string(got) != string(want) {
				if os.Getenv("HARNESS_UPDATE") != "" {
					if err := os.WriteFile(filepath.Join(dir, "programs.yaml"), got, 0o644); err != nil {
						t.Fatal(err)
					}
				} else {
					t.Errorf("the import stores the programs differently now (HARNESS_UPDATE=1 rewrites programs.yaml):\n%s", got)
				}
			}
			if sc.Needs != "" && !Capabilities[sc.Needs] {
				t.Skipf("recorded; the engine side cannot play %q yet", sc.Needs)
			}
			got, err := RunEngine(sc, im, rec)
			if err != nil {
				t.Fatal(err)
			}
			err = Verdict(sc, rec, got)
			if errors.Is(err, ErrOpen) {
				t.Skip(err)
			}
			if err != nil {
				t.Error(err)
			}
		})
	}
}

// a scenario with one of everything the script generator writes
func everything() *Scenario {
	tm := &TimeModule{Type: Daily, Time: "2007-01-01 22:00:00", Caldur: 28800, Wd: Workdays}
	return &Scenario{
		Name: "everything", Quirk: "Q-0", Title: "one of everything", Start: day, Mark: Fix, Note: "differs", Until: sec(30),
		Vars: []Var{
			boolVar("B", true), numVar("N", 6), {Name: "Wide", Type: "number", Value: 7},
			{Name: "L", Type: "list", Entries: []string{"a", "b"}, Value: 1}, {Name: "S", Type: "string", Value: "x"},
			{Name: "AL", Type: "alarm", Value: false}, {Name: "K", Type: "action", Value: false},
		},
		Programs: []Program{
			{Name: "P1", Rules: []Rule{
				{Groups: [][]Line{{is("B", Change), {Var: "N", Cmp: InRange, V1: 2.5, V2: 9, Mode: Check}}, {{Var: "L", V1: 1, Mode: Update}, {Var: "S", Cmp: Equal, V1: "x", Mode: Check}}},
					GroupsAnd: true, LinesOr: true, Cancel: true,
					Acts: []Act{{Mark: "p1-then"}, {Var: "N", Value: 3, Delay: 3725}, {Script: `dom.GetObject("P2").ProgramExecute();`}}},
				{Groups: [][]Line{{{Time: tm}, {Time: tm, Mode: Check}, {Var: "K", V1: true}}}, Acts: mark("p1-elseif")},
				{Acts: mark("p1-else")},
			}},
			{Name: "P2", Inactive: true, Rules: []Rule{{Acts: mark("p2")}}},
		},
		Steps: []Step{set(1, "B", false), {At: sec(2), Run: "P2"}, {At: sec(3), Activate: "P2"}, {At: sec(4), Deactivate: "P2"}, {At: sec(5), Restart: true}},
	}
}

func TestScripts(t *testing.T) {
	sc := everything()
	if err := sc.Check(); err != nil {
		t.Fatal(err)
	}
	setup := sc.SetupScript()
	for _, want := range []string{
		`dom.CreateObject(OT_VARDP, "B")`, `sv.ValueSubType(istBool)`, `sv.State(true);`,
		`sv.ValueMin(0); sv.ValueMax(100); sv.State(6.0);`, `sv.ValueMin(0); sv.ValueMax(65000); sv.State(7.0);`,
		`sv.ValueList("a;b"); sv.State(1);`, `sv.ValueSubType(istChar8859); sv.State("x");`,
		`dom.CreateObject(OT_ALARMDP); sv.Name("AL"); sv.AlSetBinaryCondition();`, `sv.ValueSubType(istAction); sv.State(false);`,
		`dom.CreateObject(OT_VARDP, "zzH_T")`, `dom.CreateObject(OT_VARDP, "zzH_beat")`, `LeftVal(dom.GetObject("zzH_beat").ID()); s3.ConditionType(5); s3.ConditionType2(13);`, `dom.CreateObject(OT_VARDP, "zzM_p1-else")`, `p.Name("zzH_tracer")`,
		`LeftVal(dom.GetObject("zzM_p2").ID()); s11.ConditionType(5); s11.ConditionType2(13);`, `WriteLine("setup ok");`,
	} {
		if !strings.Contains(setup, want) {
			t.Errorf("the setup script lacks %s\n%s", want, setup)
		}
	}
	p1 := sc.ProgramScript(&sc.Programs[0])
	for _, want := range []string{
		// one time module for the two lines that use it, created before the program
		`object tm1 = dom.CreateObject(OT_CALENDARDP, "Zeitmodul"); dom.GetObject(ID_CALENDARDPS).Add(tm1);`,
		`tm1.TimerType(9); tm1.Time("2007-01-01 22:00:00"); tm1.CalDuration(28800); tm1.CalRepeatTime("0"); tm1.Weekdays(31); tm1.Period(0); tm1.CalRepetitionValue(0); tm1.Begin("2026-11-09"); tm1.End("0"); tm1.CalRepetitionCount(0); tm1.SunOffsetType(0);`,
		`c3.CndOperatorType(1);`, `s4.OperatorType(2);`,
		`s5.ConditionType(6); s5.ConditionType2(15); s5.RightVal1ValType(ivtFloat); s5.RightVal1(2.5); s5.RightVal2ValType(ivtFloat); s5.RightVal2(9.0);`,
		`s7.ConditionType(1); s7.ConditionType2(13); s7.RightVal1ValType(ivtInteger); s7.RightVal1(1);`,
		`s8.ConditionType(5); s8.ConditionType2(15); s8.RightVal1ValType(ivtString); s8.RightVal1("x");`,
		`d9.BreakOnRestart(true);`,
		`a10.DestinationDP(dom.GetObject("zzM_p1-then").ID()); a10.DestinationValueType(ivtString); a10.DestinationValue("p1-then");`,
		`a11.DestinationValueType(ivtFloat); a11.DestinationValue(3.0); a11.DestinationValueParamType(ivtDelay); a11.DestinationValueParam("01:02:05");`,
		`a12.DestinationValue('dom.GetObject("P2").ProgramExecute();');`,
		`object r13 = r2.RuleCreateSubRule(); r13.ElseIfFlag(true);`,
		`s15.ConditionType(3); s15.ConditionType2(13); s15.LeftValType(ivtCurrentDate); s15.RightVal1ValType(ivtObjectId); s15.RightVal1(tm1.ID());`,
		`s16.ConditionType(3); s16.ConditionType2(15);`,
		// a line without a mode keeps ReGa's own (a key line)
		`s17.ConditionType(1); s17.RightVal1ValType(ivtBinary); s17.RightVal1(true);`,
		"object r20 = r13.RuleCreateSubRule();\n", `p.ProgramUpdate(); p.Active(true);`, `WriteLine("created P1 " # system.Date("%F %T"));`,
	} {
		if !strings.Contains(p1, want) {
			t.Errorf("the program script lacks %s\n%s", want, p1)
		}
	}
	if strings.Count(p1, "OT_CALENDARDP") != 1 {
		t.Error("a time module used twice is created once")
	}
	if !sc.Timed() || Scenarios()[0].Timed() {
		t.Error("which scenario has a time module")
	}
	if p2 := sc.ProgramScript(&sc.Programs[1]); !strings.Contains(p2, "p.Active(false);") {
		t.Errorf("an inactive program: %s", p2)
	}
	begin := (&Scenario{Start: day}).timeModuleScript("x", &TimeModule{Type: Once, Caltime: "2027-01-01", Begin: "2026-01-02", End: "2027-03-04", Calcount: 2, Sot: 3, Period: 5, Calrep: 6})
	if !strings.Contains(begin, `x.CalRepeatTime("2027-01-01"); x.Weekdays(0); x.Period(5); x.CalRepetitionValue(6); x.Begin("2026-01-02"); x.End("2027-03-04"); x.CalRepetitionCount(2); x.SunOffsetType(3);`) {
		t.Error(begin)
	}
	steps := []string{`dom.GetObject("B").State(false); Write(`, `dom.GetObject("P2").ProgramExecute(); Write(`, `dom.GetObject("P2").Active(true); Write(`, `dom.GetObject("P2").Active(false); Write(`, ` Write(system.Date`}
	for i := range sc.Steps {
		if got := sc.StepScript(&sc.Steps[i]); !strings.HasPrefix(got, steps[i]) {
			t.Errorf("step %d: %s", i+1, got)
		}
	}
	if got := ClockOffset(time.Unix(1000, 0), time.Unix(4600, 0), 60); got != "-3600s x60\n" {
		t.Errorf("clock offset %q", got)
	}
	if got := ClockOffset(time.Unix(4600, 0), time.Unix(1000, 0), 1); got != "+3600s x1\n" {
		t.Errorf("clock offset %q", got)
	}
}

// An astro module: the position goes to the reference first, the offset where the dialog puts it.
func TestAstroScripts(t *testing.T) {
	sc := everything()
	sc.Position = &Position{Lat: 48.78, Lon: 9.18}
	sc.Programs[0].Rules[1].Groups[0][0].Time = &TimeModule{Type: Daily, Sot: BeforeSunset, Offset: 2400, Caldur: 3600}
	if err := sc.Check(); err != nil {
		t.Fatal(err)
	}
	if setup := sc.SetupScript(); !strings.Contains(setup, "system.Longitude(9.18); system.Latitude(48.78);\n") {
		t.Errorf("the setup script does not set the position:\n%s", setup)
	}
	if p := sc.ProgramScript(&sc.Programs[0]); !strings.Contains(p, `tm1.TimerType(9); tm1.Time("2400"); tm1.CalDuration(3600);`) || !strings.Contains(p, `tm1.SunOffsetType(5); tm1.TimeSeconds(2400);`) {
		t.Errorf("the astro module:\n%s", p)
	}
}

func TestCheck(t *testing.T) {
	bad := map[string]func(*Scenario){
		"the name must be":         func(s *Scenario) { s.Name = "Not OK" },
		"is not a time":            func(s *Scenario) { s.Start = "noon" },
		"the mark must be":         func(s *Scenario) { s.Mark = "" },
		"a fix needs a note":       func(s *Scenario) { s.Note = "" },
		"no end":                   func(s *Scenario) { s.Until = 0 },
		"needs a position":         func(s *Scenario) { s.Programs[0].Rules[1].Groups[0][0].Time.Sot = Sunset },
		`variable name "B"`:        func(s *Scenario) { s.Vars = append(s.Vars, boolVar("B", true)) },
		`variable name "ä"`:        func(s *Scenario) { s.Vars[0].Name = "ä" },
		`variable name ""`:         func(s *Scenario) { s.Vars[0].Name = "" },
		"is no value of a bool":    func(s *Scenario) { s.Vars[0].Value = 3 },
		"is no value of a number":  func(s *Scenario) { s.Vars[1].Value = "x" },
		"is no value of a list":    func(s *Scenario) { s.Vars[3].Value = "a" },
		"is no value of a string":  func(s *Scenario) { s.Vars[4].Value = `"` },
		`unknown type "x"`:         func(s *Scenario) { s.Vars[0].Type = "x" },
		`program name "P1"`:        func(s *Scenario) { s.Programs[1].Name = "P1" },
		`program P1: no variable`:  func(s *Scenario) { s.Programs[0].Rules[0].Groups[0][0].Var = "nope" },
		`P1: no variable "nope"`:   func(s *Scenario) { s.Programs[0].Rules[0].Acts[1].Var = "nope" },
		`marker "Bad Mark"`:        func(s *Scenario) { s.Programs[0].Rules[0].Acts[0].Mark = "Bad Mark" },
		"must be in order":         func(s *Scenario) { s.Steps[1].At = 0 },
		"before the end":           func(s *Scenario) { s.Steps[4].At = time.Hour },
		`step: no variable "nope"`: func(s *Scenario) { s.Steps[0].Set = "nope" },
		"step: B: 7":               func(s *Scenario) { s.Steps[0].Value = 7 },
		`step: no program "nope"`:  func(s *Scenario) { s.Steps[1].Run = "nope" },
		"a step that does nothing": func(s *Scenario) { s.Steps[4] = Step{At: sec(5)} },
	}
	for want, change := range bad {
		sc := everything()
		change(sc)
		if err := sc.Check(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("a start that is no time must not pass silently")
		}
	}()
	(&Scenario{Start: "x"}).StartTime()
}

func TestZone(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unknown zone must not pass silently")
		}
	}()
	mustZone("Nowhere/None")
}

func at(s string) time.Time {
	t, err := parseWall(s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestTrace(t *testing.T) {
	tr, err := ParseTracer("zzH_beat@2026-11-10 12:00:01 zzM_a@2026-11-10 12:00:02 zzM_b@2026-11-10 12:00:02 zzH_beat@2026-11-10 12:00:03 ")
	if err != nil || len(tr) != 2 || tr[1].Mark != "b" || !tr[0].Time.Equal(at("2026-11-10 12:00:02")) {
		t.Fatalf("%v %v", tr, err)
	}
	// the repeated hour of the autumn night: 02:50 then 02:10 is CEST then CET
	tr, err = ParseTracer("zzM_a@2026-10-25 02:50:00 zzM_a@2026-10-25 02:10:00 zzM_a@2026-10-25 02:30:00 ")
	if err != nil || tr[1].Time.Sub(tr[0].Time) != 20*time.Minute || tr[2].Time.Sub(tr[1].Time) != 20*time.Minute {
		t.Fatalf("%v %v", tr.Lines(), err)
	}
	for _, bad := range []string{"junk", "other@2026-11-10 12:00:02 ", "zzM_a@2026-13-40 12:00:02 "} {
		if _, err := ParseTracer(bad); err == nil {
			t.Errorf("%q is no tracer note", bad)
		}
	}
	lines := []string{"2026-11-10 12:00:02 a", "2026-11-10 12:00:02 b"}
	tr, err = ParseLines(lines)
	if err != nil || strings.Join(tr.Lines(), "|") != strings.Join(lines, "|") {
		t.Fatalf("%v %v", tr, err)
	}
	for _, bad := range []string{"short", "2026-13-40 12:00:02 a"} {
		if _, err := ParseLines([]string{bad}); err == nil {
			t.Errorf("%q is no trace line", bad)
		}
	}
	a := Trace{{at("2026-11-10 12:00:00"), "x"}, {at("2026-11-10 12:00:01"), "y"}, {at("2026-11-10 12:00:30"), "z"}}
	b := Trace{{at("2026-11-10 12:00:01"), "y"}, {at("2026-11-10 12:00:02"), "x"}, {at("2026-11-10 12:00:28"), "z"}}
	if err := Compare(a, b, 2*time.Second); err != nil {
		t.Errorf("the same instant in another order, a time within the slack: %v", err)
	}
	if err := Compare(a, b, 0); err == nil {
		t.Error("without slack the two differ")
	}
	if err := Compare(a, b[:2], time.Second); err == nil || !strings.Contains(err.Error(), "2 entries, want 3") {
		t.Errorf("%v", err)
	}
	c := Trace{{at("2026-11-10 12:00:00"), "x"}, {at("2026-11-10 12:00:01"), "y"}, {at("2026-11-10 12:00:25"), "z"}}
	if err := Compare(a, c, 2*time.Second); err == nil || !strings.Contains(err.Error(), `entry 3 is "2026-11-10 12:00:25 z"`) {
		t.Errorf("%v", err)
	}
}

func TestRecordingText(t *testing.T) {
	rec := &Recording{Reference: "ReGaHss R1", Rate: 60,
		Created: []Created{{"P", at("2026-10-25 01:59:50")}},
		Steps:   []time.Time{at("2026-10-25 02:30:00"), at("2026-10-25 02:30:00").Add(time.Hour)},
		Trace:   Trace{{at("2026-10-25 01:59:50"), "else"}, {at("2026-10-25 02:30:00"), "then"}}}
	text := string(rec.Encode())
	for _, want := range []string{"reference ReGaHss R1\nrate 60\n", "2026-10-25 01:59:50 +0200 created P\n2026-10-25 01:59:50 +0200 ran else\n",
		"2026-10-25 02:30:00 +0200 step 1\n2026-10-25 02:30:00 +0200 ran then\n2026-10-25 02:30:00 +0100 step 2\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q:\n%s", want, text)
		}
	}
	back, err := DecodeRecording([]byte(text))
	if err != nil || string(back.Encode()) != text {
		t.Fatalf("%v\n%s", err, back.Encode())
	}
	for _, bad := range []string{"rate x", "short", "2026-13-40 01:59:50 +0200 ran x", "2026-10-25 01:59:50 +0200 step 2", "2026-10-25 01:59:50 +0200 step x", "2026-10-25 01:59:50 +0200 what x"} {
		if _, err := DecodeRecording([]byte(bad)); err == nil {
			t.Errorf("%q is no recording", bad)
		}
	}
	if _, err := Gunzip([]byte("no gzip")); err == nil {
		t.Error("no gzip")
	}
	if back, err := Gunzip(Gzip([]byte("abc"))); err != nil || string(back) != "abc" {
		t.Error(err)
	}
}

// fakeRef is a reference for the recorder's tests: a clock that runs rate times as fast as the
// fake real clock, and answers by the kind of script.
type fakeRef struct {
	real     time.Time
	refBase  time.Time
	realBase time.Time
	rate     int
	notes    string
	scripts  []string
	fail     string // a script containing this fails
	wrong    string // a script containing this answers nonsense
	restarts int
	stepLag  time.Duration
	dead     bool // programs no longer run
}

func (f *fakeRef) ref() time.Time {
	return f.refBase.Add(time.Duration(f.rate) * f.real.Sub(f.realBase))
}
func (f *fakeRef) now() time.Time        { return f.real }
func (f *fakeRef) sleep(d time.Duration) { f.real = f.real.Add(d) }

var createdName = regexp.MustCompile(`WriteLine\("created (\S+) "`)

func (f *fakeRef) Script(text string) (string, error) {
	f.scripts = append(f.scripts, text)
	f.real = f.real.Add(3 * time.Millisecond)
	if f.fail != "" && strings.Contains(text, f.fail) {
		return "", errors.New("the reference is gone")
	}
	if f.wrong != "" && strings.Contains(text, f.wrong) {
		return "nonsense", nil
	}
	stamp := f.ref().Format(TimeLayout)
	switch {
	case strings.Contains(text, "BuildLabel"):
		return "R1.00.0388.0254", nil
	case strings.Contains(text, `WriteLine("setup ok")`):
		return "setup ok\n", nil
	case createdName.MatchString(text):
		f.notes += MarkerPrefix + "else@" + stamp + " "
		return "created " + createdName.FindStringSubmatch(text)[1] + " " + stamp + "\n", nil
	case text == SaveScript:
		return "saved", nil
	case text == TimeScript:
		return stamp, nil
	case text == BeatScript:
		if !f.dead {
			f.notes += BeatVar + "@" + stamp + " "
		}
		return "beat", nil
	case text == TraceScript:
		out := f.notes
		f.notes = ""
		return out, nil
	}
	// a step: the program runs
	f.real = f.real.Add(f.stepLag)
	stamp = f.ref().Format(TimeLayout)
	f.notes += MarkerPrefix + "then@" + stamp + " "
	return stamp, nil
}
func (f *fakeRef) Regadom() ([]byte, error) {
	if f.fail == "regadom" {
		return nil, errors.New("no database")
	}
	return []byte("<dom/>"), nil
}
func (f *fakeRef) Restart(at time.Time, rate int) error {
	if f.fail == "restart" {
		return errors.New("no restart")
	}
	f.restarts++
	f.refBase, f.realBase = at, f.real
	return nil
}

func recorderFor(sc *Scenario, lead time.Duration) (*Recorder, *fakeRef) {
	real := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	f := &fakeRef{real: real, realBase: real, refBase: sc.StartTime().Add(-lead), rate: sc.SpeedRate()}
	return &Recorder{Target: f, Now: f.now, Sleep: f.sleep, Log: func(string, ...any) {}}, f
}

func TestRecorder(t *testing.T) {
	sc := everything()
	sc.Rate = 60
	sc.Steps = []Step{set(120, "B", false), {At: sec(3600), Run: "P2"}, {At: sec(7200), Restart: true}, set(7300, "B", true)}
	sc.Until = sec(9000)
	r, f := recorderFor(sc, 10*time.Minute)
	rec, dom, err := r.Record(sc)
	if err != nil {
		t.Fatal(err)
	}
	if string(dom) != "<dom/>" || rec.Reference != "ReGaHss R1.00.0388.0254" || rec.Rate != 60 || f.restarts != 1 {
		t.Errorf("%s %+v", dom, rec)
	}
	if len(rec.Created) != 2 || rec.Created[0].Program != "P1" || !rec.Created[0].Time.Before(sc.StartTime()) {
		t.Errorf("created %+v", rec.Created)
	}
	if len(rec.Steps) != 4 {
		t.Fatalf("steps %v", rec.Steps)
	}
	for i, st := range sc.Steps {
		if d := rec.Steps[i].Sub(sc.StartTime().Add(st.At)); d < 0 || d > sc.SlackTime() {
			t.Errorf("step %d was done %v off", i+1, d)
		}
	}
	// two creations and three steps that run a program
	if got := strings.Join(rec.Trace.Lines(), "|"); len(rec.Trace) != 5 || !strings.HasSuffix(got, " then") || !strings.Contains(got, "12:02:0") {
		t.Errorf("trace %s", got)
	}
	// the trace was read while waiting, not only at the end
	reads := 0
	for _, s := range f.scripts {
		if s == TraceScript {
			reads++
		}
	}
	if reads < 5 {
		t.Errorf("the trace was read %d times", reads)
	}
	// the real clock on its own, a default poll and no log
	quick := &Scenario{Name: "quick", Quirk: "Q-0", Title: "x", Start: day, Mark: Reproduce, Until: sec(1), Vars: []Var{boolVar("B", true)}}
	_, f = recorderFor(quick, time.Second)
	rr := &Recorder{Target: f}
	f.refBase = quick.StartTime().Add(-time.Second)
	f.realBase, f.real = time.Now(), time.Now()
	f.now() // the fake is not asked: the recorder reads the real clock
	rr.Now, rr.Sleep = nil, nil
	if d := time.Until(rr.now()); d > time.Second || d < -time.Second {
		t.Error("the real clock")
	}
	rr.sleep(time.Millisecond)
	rr.sleep(-time.Second)
	rr.logf("nothing")
}

func TestRecorderErrors(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(sc *Scenario, f *fakeRef)
	}{
		{"a bad scenario", "no end", func(sc *Scenario, f *fakeRef) { sc.Until = 0 }},
		{"no version", "is gone", func(sc *Scenario, f *fakeRef) { f.fail = "BuildLabel" }},
		{"setup", "the setup failed", func(sc *Scenario, f *fakeRef) { f.wrong = "setup ok" }},
		{"program", "is gone", func(sc *Scenario, f *fakeRef) { f.fail = `created P1 ` }},
		{"program answer", "was not created", func(sc *Scenario, f *fakeRef) { f.wrong = `created P1 ` }},
		{"save", "was not saved", func(sc *Scenario, f *fakeRef) { f.wrong = "system.Save" }},
		{"database", "no database", func(sc *Scenario, f *fakeRef) { f.fail = "regadom" }},
		{"clock", "where a time was asked", func(sc *Scenario, f *fakeRef) { f.wrong = TimeScript }},
		{"late setup", "the setup took too long", func(sc *Scenario, f *fakeRef) { f.refBase = sc.StartTime().Add(time.Second) }},
		{"step", "is gone", func(sc *Scenario, f *fakeRef) { f.fail = `GetObject("B").State` }},
		{"step answer", "where a time was asked", func(sc *Scenario, f *fakeRef) { f.wrong = `GetObject("B").State` }},
		{"late step", "step 1 was done at", func(sc *Scenario, f *fakeRef) { f.stepLag = time.Minute }},
		{"restart", "no restart", func(sc *Scenario, f *fakeRef) { f.fail = "restart" }},
		{"dead", "no longer runs programs", func(sc *Scenario, f *fakeRef) { f.dead = true }},
		{"heartbeat", "is gone", func(sc *Scenario, f *fakeRef) { f.fail = BeatVar }},
		{"trace", "is no marker", func(sc *Scenario, f *fakeRef) { f.notes = "other@2026-11-10 12:00:00 " }},
	}
	for _, c := range cases {
		sc := everything()
		sc.Steps = []Step{set(1, "B", false), {At: sec(5), Restart: true}}
		sc.Until = sec(60)
		r, f := recorderFor(sc, 20*time.Second)
		c.change(sc, f)
		if _, _, err := r.Record(sc); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want %q, got %v", c.name, c.want, err)
		}
	}
	// the reference going away at each later point
	for _, n := range []int{1, 2, 3, 4, 5, 6} {
		sc := everything()
		sc.Steps = []Step{set(1, "B", false), {At: sec(30), Restart: true}}
		sc.Until = sec(90)
		r, f := recorderFor(sc, 20*time.Second)
		r.Poll = 5 * time.Second
		target := &failAfter{fakeRef: f, script: TraceScript, n: n}
		r.Target = target
		if _, _, err := r.Record(sc); err == nil {
			t.Errorf("the trace's read %d failing must fail the recording", n)
		}
	}
	for _, script := range []string{TimeScript, SaveScript} {
		for n := 1; n <= 12; n++ {
			sc := everything()
			sc.Steps = []Step{set(1, "B", false), {At: sec(30), Restart: true}}
			sc.Until = sec(90)
			r, f := recorderFor(sc, 20*time.Second)
			target := &failAfter{fakeRef: f, script: script, n: n}
			r.Target = target
			_, _, err := r.Record(sc)
			if target.seen >= n && err == nil {
				t.Errorf("%s failing at call %d must fail the recording", script, n)
			}
		}
	}
}

// failAfter fails the n-th run of one script.
type failAfter struct {
	*fakeRef
	script  string
	n, seen int
}

func (f *failAfter) Script(text string) (string, error) {
	if text == f.script {
		f.seen++
		if f.seen == f.n {
			return "", errors.New("the reference is gone")
		}
	}
	return f.fakeRef.Script(text)
}
