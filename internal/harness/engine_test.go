package harness

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// the first scenario's recording, for the tests of the engine side's own errors
func recorded(t *testing.T) (*Scenario, []byte, *Recording) {
	t.Helper()
	sc := *Scenarios()[0]
	packed, err := os.ReadFile("testdata/" + sc.Name + "/homematic.regadom.gz")
	if err != nil {
		t.Fatal(err)
	}
	dom, err := Gunzip(packed)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/" + sc.Name + "/rega.trace")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := DecodeRecording(raw)
	if err != nil {
		t.Fatal(err)
	}
	return &sc, dom, rec
}

func TestImportErrors(t *testing.T) {
	sc, dom, _ := recorded(t)
	if _, err := Import(sc, []byte("no database")); err == nil {
		t.Error("no database")
	}
	cases := map[string]func(*Scenario){
		"program other was not imported":  func(s *Scenario) { s.Programs = append(s.Programs, Program{Name: "other"}) },
		"was imported with active: true":  func(s *Scenario) { s.Programs = []Program{{Name: s.Programs[0].Name, Inactive: true}} },
		"variable other was not imported": func(s *Scenario) { s.Vars = append(s.Vars, boolVar("other", true)) },
		"marker other was not imported": func(s *Scenario) {
			s.Programs = append([]Program{{Name: "P", Rules: []Rule{{Acts: mark("other")}}}}, s.Programs[1:]...)
		},
	}
	for want, change := range cases {
		c := *sc
		c.Programs = append([]Program(nil), sc.Programs...)
		c.Vars = append([]Var(nil), sc.Vars...)
		change(&c)
		if _, err := Import(&c, dom); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}

func TestRunEngineErrors(t *testing.T) {
	sc, dom, rec := recorded(t)
	im, err := Import(sc, dom)
	if err != nil {
		t.Fatal(err)
	}
	run := func(change func(*Scenario, *Recording)) error {
		c, r := *sc, *rec
		c.Steps = append([]Step(nil), sc.Steps...)
		r.Created = append([]Created(nil), rec.Created...)
		change(&c, &r)
		_, err := RunEngine(&c, im, &r)
		return err
	}
	cases := map[string]func(*Scenario, *Recording){
		`cannot play "magic" yet`:        func(s *Scenario, r *Recording) { s.Needs = "magic" },
		"has 0 programs, the scenario 1": func(s *Scenario, r *Recording) { r.Created = nil },
		"program 1 is other":             func(s *Scenario, r *Recording) { r.Created[0].Program = "other" },
		"step 1: ":                       func(s *Scenario, r *Recording) { s.Steps[0].Value = "text" },
		"variable N: ":                   func(s *Scenario, r *Recording) { s.Vars = []Var{{Name: "N", Type: "number", Value: "text"}} },
	}
	for want, change := range cases {
		if err := run(change); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
	// a capability that is there lets the scenario through
	Capabilities["magic"] = true
	defer delete(Capabilities, "magic")
	if err := run(func(s *Scenario, r *Recording) { s.Needs = "magic" }); err != nil {
		t.Error(err)
	}
	// run by hand, switched off and on again: the steps the first scenario does not use
	c := *sc
	c.Steps = []Step{{At: sec(1), Run: "P"}, {At: sec(2), Deactivate: "P"}, set(3, "N", 50.0), {At: sec(4), Activate: "P"}, set(5, "N", 1.0), set(6, "N", 60)}
	got, err := RunEngine(&c, im, rec)
	if err != nil {
		t.Fatal(err)
	}
	// the hand start runs DANN; the write while switched off runs nothing; switching on runs
	// nothing; then the flips run
	want := "12:00:01 then|12:00:05 else|12:00:06 then"
	if lines := strings.ReplaceAll(strings.Join(fromStart(&c, got).Lines(), "|"), "2026-11-10 ", ""); lines != want {
		t.Errorf("got %s, want %s", lines, want)
	}
}

func TestValue(t *testing.T) {
	sc, dom, _ := recorded(t)
	im, err := Import(sc, dom)
	if err != nil {
		t.Fatal(err)
	}
	num := im.Variables[im.VarKeys["N"]]
	list := *num
	list.Type, list.Entries = "list", []string{"a", "b"}
	if value(num, 3) != 3.0 || value(&list, 1) != "b" || value(&list, 7) != "" || value(num, "x") != "x" {
		t.Error("values")
	}
}

func TestVerdict(t *testing.T) {
	sc := &Scenario{Name: "v", Start: day, Mark: Reproduce}
	t0 := sc.StartTime()
	ref := &Recording{Trace: Trace{{t0.Add(-time.Second), "else"}, {t0.Add(2 * time.Second), "then"}}}
	same := Trace{{t0.Add(-5 * time.Second), "x"}, {t0.Add(3 * time.Second), "then"}}
	other := Trace{{t0.Add(2 * time.Second), "else"}}
	if err := Verdict(sc, ref, same); err != nil {
		t.Errorf("what ran before the start is not compared, a second is within the slack: %v", err)
	}
	if err := Verdict(sc, ref, other); err == nil || !strings.Contains(err.Error(), "the engine differs from the reference") {
		t.Errorf("%v", err)
	}
	fix := &Scenario{Name: "v", Start: day, Mark: Fix, Note: "the engine runs else", Engine: []string{"2026-11-10 12:00:02 else"}}
	if err := Verdict(fix, ref, other); err != nil {
		t.Error(err)
	}
	if err := Verdict(fix, ref, same); err == nil || !strings.Contains(err.Error(), "differs from what the scenario states") {
		t.Errorf("%v", err)
	}
	fix.Engine = []string{"2026-11-10 12:00:03 then"}
	if err := Verdict(fix, ref, same); err == nil || !strings.Contains(err.Error(), "marked fix, but the engine does what the reference does") {
		t.Errorf("%v", err)
	}
	fix.Engine = []string{"nonsense"}
	if err := Verdict(fix, ref, same); err == nil {
		t.Error("a stated trace that is no trace")
	}
	// an open question: a difference that waits is not a failure, and no longer open once the
	// two sides agree
	open := &Scenario{Name: "v", Start: day, Mark: Reproduce, Open: "task 12"}
	if err := Verdict(open, ref, other); !errors.Is(err, ErrOpen) || !strings.Contains(err.Error(), "task 12") || !strings.Contains(err.Error(), "12:00:02 else") {
		t.Errorf("an open difference: %v", err)
	}
	if err := Verdict(open, ref, same); err == nil || errors.Is(err, ErrOpen) || !strings.Contains(err.Error(), "take the mark away") {
		t.Errorf("an open question that is settled: %v", err)
	}
	if (&Scenario{Slack: time.Minute}).SlackTime() != time.Minute || (&Scenario{Rate: 30}).SlackTime() != time.Minute {
		t.Error("slack")
	}
	if (&Scenario{}).variable("none") != nil || nameOK("") {
		t.Error("lookups")
	}
}
