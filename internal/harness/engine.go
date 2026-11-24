package harness

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/importer"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

// Capabilities says what the engine side can play besides writes: "scheduler" (time modules),
// "restart" (the logic stopped and started as after a boot of the system). A scenario that
// needs something else is recorded and not compared yet.
var Capabilities = map[string]bool{"scheduler": true, "restart": true}

// Imported is a scenario's database as this addon reads it.
type Imported struct {
	// Programs are the scenario's programs by their name; Keys the same names' keys.
	Programs map[string]*model.Program
	// Variables are all variables by key; VarKeys maps a name to its key.
	Variables map[string]*model.Variable
	VarKeys   map[string]string
	// Markers maps a marker variable's key to the marker.
	Markers map[string]string
}

// Gzip packs a database for testdata; Gunzip unpacks it.
func Gzip(data []byte) []byte {
	var b bytes.Buffer
	w, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	_, _ = w.Write(data) // a buffer takes everything
	_ = w.Close()
	return b.Bytes()
}

// Gunzip unpacks what Gzip packed.
func Gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// Import reads the database the reference saved and picks the scenario's objects out of it.
// Every program of the scenario must have come in as the scenario has it - active unless it
// says inactive: a program the importer refuses or switches off is a finding, not a skip.
func Import(sc *Scenario, dom []byte) (*Imported, error) {
	src, err := regadom.Read(bytes.NewReader(dom))
	if err != nil {
		return nil, err
	}
	res := importer.Import(src, importer.Options{Known: func(string) bool { return true }, Location: Zone, Lang: "de"})
	im := &Imported{Programs: map[string]*model.Program{}, Variables: map[string]*model.Variable{}, VarKeys: map[string]string{}, Markers: map[string]string{}}
	for _, v := range res.Variables {
		im.Variables[v.Key] = v
		im.VarKeys[v.Name] = v.Key
		if strings.HasPrefix(v.Name, MarkerPrefix) {
			im.Markers[v.Key] = v.Name[len(MarkerPrefix):]
		}
	}
	byName := map[string]*model.Program{}
	for _, p := range res.Programs {
		byName[p.Name] = p
	}
	for i := range sc.Programs {
		want := &sc.Programs[i]
		p := byName[want.Name]
		if p == nil {
			return nil, fmt.Errorf("program %s was not imported", want.Name)
		}
		if p.Active == want.Inactive {
			return nil, fmt.Errorf("program %s was imported with active: %t", want.Name, p.Active)
		}
		im.Programs[want.Name] = p
	}
	for i := range sc.Vars {
		if im.VarKeys[sc.Vars[i].Name] == "" {
			return nil, fmt.Errorf("variable %s was not imported", sc.Vars[i].Name)
		}
	}
	for _, m := range sc.Marks() {
		if im.VarKeys[MarkerPrefix+m] == "" {
			return nil, fmt.Errorf("marker %s was not imported", m)
		}
	}
	return im, nil
}

// YAML writes the scenario's programs as this addon stores them, one after the other in the
// scenario's order: what a reader of testdata looks at, and what shows when the importer changes.
func (im *Imported) YAML(sc *Scenario) []byte {
	var b bytes.Buffer
	for i := range sc.Programs {
		p := im.Programs[sc.Programs[i].Name]
		fmt.Fprintf(&b, "# programs/%s.yaml\n", p.Key)
		b.Write(model.EncodeProgram(p, nil))
		b.WriteString("\n")
	}
	return b.Bytes()
}

// value turns a scenario's value into the engine's: a list's index into its entry.
func value(def *model.Variable, v any) any {
	switch x := v.(type) {
	case int:
		if def.Type == model.VarList {
			if x >= 0 && x < len(def.Entries) {
				return def.Entries[x]
			}
			return ""
		}
		return float64(x)
	}
	return v
}

// RunEngine plays a scenario on the engine: the programs as imported, created at the times the
// reference created them - an active program is evaluated once then, as the reference's
// activation does -, the steps at their times on a fake clock.
func RunEngine(sc *Scenario, im *Imported, rec *Recording) (Trace, error) {
	trace, _, err := RunEngineRuns(sc, im, rec)
	return trace, err
}

// RunEngineRuns is RunEngine with the engine's own account of it: every program's entries of
// the run trace, by the program's name (after a restart in the scenario those since then).
func RunEngineRuns(sc *Scenario, im *Imported, rec *Recording) (Trace, map[string][]engine.Run, error) {
	if sc.Needs != "" && !Capabilities[sc.Needs] {
		return nil, nil, fmt.Errorf("the engine side cannot play %q yet", sc.Needs)
	}
	if len(rec.Created) != len(sc.Programs) {
		return nil, nil, fmt.Errorf("the recording has %d programs, the scenario %d", len(rec.Created), len(sc.Programs))
	}
	start := sc.StartTime()
	begin := start
	if len(rec.Created) > 0 && rec.Created[0].Time.Before(begin) {
		begin = rec.Created[0].Time
	}
	clock := engine.NewFakeClock(begin)
	var trace Trace
	var pos schedule.Position
	if sc.Position != nil {
		pos = schedule.Position{Lat: sc.Position.Lat, Lon: sc.Position.Lon, Known: true}
	}
	start0 := func() *engine.Engine {
		// (a scenario's account of a program is all of its entries, however many)
		e := engine.New(engine.Options{Clock: clock, Location: Zone, Position: pos, RunsKept: 1 << 20, Notify: func(n engine.Notification) {
			if m, ok := im.Markers[n.Key]; ok && n.Kind == "variable" {
				trace = append(trace, Entry{Time: clock.Now(), Mark: m})
			}
		}})
		e.SetVariables(im.Variables)
		return e
	}
	eng := start0()
	for i := range sc.Vars {
		v := &sc.Vars[i]
		key := im.VarKeys[v.Name]
		if im.Variables[key].Type == model.VarAlarm {
			continue
		}
		if err := eng.SetVariable(key, value(im.Variables[key], v.Value), engine.FromImport); err != nil {
			return nil, nil, fmt.Errorf("variable %s: %v", v.Name, err)
		}
	}
	// all programs are there from the start, switched off; each is switched on at its time
	progs := map[string]*model.Program{}
	for name, p := range im.Programs {
		c := *p
		c.Active = false
		progs[im.Programs[name].Key] = &c
	}
	eng.SetPrograms(progs)
	to := func(t time.Time) {
		if d := t.Sub(clock.Now()); d > 0 {
			clock.Advance(d)
		}
	}
	activate := func(name string, on bool) {
		key := im.Programs[name].Key
		next := map[string]*model.Program{}
		for k, p := range progs {
			next[k] = p
		}
		c := *progs[key]
		c.Active = on
		next[key] = &c
		progs = next
		eng.SetPrograms(progs)
	}
	for i, c := range rec.Created {
		p := &sc.Programs[i]
		if c.Program != p.Name {
			return nil, nil, fmt.Errorf("the recording's program %d is %s, the scenario's %s", i+1, c.Program, p.Name)
		}
		to(c.Time)
		if p.Inactive {
			continue
		}
		activate(p.Name, true)
		if err := eng.RunProgramBy(im.Programs[p.Name].Key, model.RunCheck, "creation"); err != nil {
			return nil, nil, fmt.Errorf("program %s: %v", p.Name, err)
		}
	}
	for i := range sc.Steps {
		st := &sc.Steps[i]
		to(start.Add(st.At))
		var err error
		switch {
		case st.Set != "":
			key := im.VarKeys[st.Set]
			err = eng.SetVariable(key, value(im.Variables[key], st.Value), engine.FromProgram)
		case st.Run != "":
			err = eng.RunProgramBy(im.Programs[st.Run].Key, model.RunThen, "outside")
		case st.Activate != "":
			activate(st.Activate, true)
		case st.Deactivate != "":
			activate(st.Deactivate, false)
		default:
			// the logic stops with its state and starts as after a boot of the system: the
			// state comes back, then every program is evaluated once
			state, _ := eng.Export()
			eng = start0()
			eng.Import(state)
			eng.SetPrograms(progs)
			eng.ApplyStartValues()
			eng.EvaluateAtStart()
		}
		if err != nil {
			return nil, nil, fmt.Errorf("step %d: %v", i+1, err)
		}
	}
	to(start.Add(sc.Until))
	runs := map[string][]engine.Run{}
	for name, p := range im.Programs {
		runs[name] = eng.Runs(p.Key)
	}
	return trace, runs, nil
}

// fromStart drops what ran before the scenario's start.
func fromStart(sc *Scenario, t Trace) Trace {
	start := sc.StartTime()
	var out Trace
	for _, e := range t {
		if !e.Time.Before(start) {
			out = append(out, e)
		}
	}
	return out
}

// ErrOpen is Verdict's answer for a scenario marked Open whose two sides still differ: not a
// failure, a question that waits.
var ErrOpen = errors.New("open")

// Verdict compares the two sides of a scenario: nil when the engine's trace is what the
// scenario claims - the reference's for Reproduce, the stated one for Fix (which must differ
// from the reference's, or the note is out of date). For a scenario marked Open the answer is
// an error that wraps ErrOpen while the two sides differ, and a plain error once they agree.
func Verdict(sc *Scenario, rec *Recording, got Trace) error {
	if sc.Open != "" {
		ref := fromStart(sc, rec.Trace)
		mine := fromStart(sc, got)
		if err := Compare(ref, mine, sc.SlackTime()); err != nil {
			return fmt.Errorf("%w: %s (%v)\nreference:\n  %s\nengine:\n  %s", ErrOpen, sc.Open, err, strings.Join(ref.Lines(), "\n  "), strings.Join(mine.Lines(), "\n  "))
		}
		return fmt.Errorf("the scenario is marked open (%s), but the engine does what the reference does now: take the mark away", sc.Open)
	}
	// what ran while the programs were created is not compared: the reference evaluates a new
	// program when it is saved and switched on (more than once, at times), the engine side
	// mirrors that with one evaluation so that both start from the same state
	ref := fromStart(sc, rec.Trace)
	got = fromStart(sc, got)
	rec = &Recording{Trace: ref}
	if sc.Mark == Reproduce {
		if err := Compare(rec.Trace, got, sc.SlackTime()); err != nil {
			return fmt.Errorf("the engine differs from the reference: %v\nreference:\n  %s\nengine:\n  %s", err, strings.Join(rec.Trace.Lines(), "\n  "), strings.Join(got.Lines(), "\n  "))
		}
		return nil
	}
	want, err := ParseLines(sc.Engine)
	if err != nil {
		return err
	}
	if err := Compare(want, got, 0); err != nil {
		return fmt.Errorf("the engine differs from what the scenario states: %v\nstated:\n  %s\nengine:\n  %s", err, strings.Join(sc.Engine, "\n  "), strings.Join(got.Lines(), "\n  "))
	}
	if Compare(rec.Trace, got, sc.SlackTime()) == nil {
		return fmt.Errorf("the scenario is marked fix, but the engine does what the reference does: %s", sc.Note)
	}
	return nil
}
