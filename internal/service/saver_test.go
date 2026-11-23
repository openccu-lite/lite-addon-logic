package service

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/soak"
)

func later(t *testing.T, s *Service) {
	t.Helper()
	if _, err := s.CreateVariable(&model.Variable{Name: "Licht", Type: model.VarBool, Visible: true}, false, admin); err != nil {
		t.Fatal(err)
	}
	p := &model.Program{Name: "Später", Active: true, Visible: true, Operable: true, Rules: []model.Rule{{
		When: &model.Condition{Op: model.All, Groups: []model.Group{}},
		Then: []model.Action{{Kind: model.ActionVariable, Variable: "licht", To: true, After: model.Duration(time.Minute)}}}}}
	if _, err := s.CreateProgram(p, admin); err != nil {
		t.Fatal(err)
	}
}

// The delayed activities have a small file of their own: written when they changed, read at a
// start in place of the state file's list - a delay survives a power cut without the whole
// state being written for it.
func TestPendingFile(t *testing.T) {
	dir := t.TempDir()
	clock := engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	open := func() (*Service, *[]string) {
		t.Helper()
		var logs []string
		s, err := Open(Options{Dir: dir, Clock: clock, Log: func(f string, a ...any) { logs = append(logs, f) }})
		if err != nil {
			t.Fatal(err)
		}
		return s, &logs
	}
	pending := func(s *Service) int { return len(s.Engine().PendingActions()) }
	file := filepath.Join(dir, "state", "pending.json")
	state := filepath.Join(dir, "state", "state.json")

	s, _ := open()
	later(t, s)
	if err := s.Engine().RunProgram("spaeter", model.RunThen); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePending(); err != nil {
		t.Fatal(err)
	}
	if w := s.Written(); w.PendingFiles != 1 || w.StateFiles != 0 || w.PendingBytes == 0 {
		t.Errorf("written: %+v", w)
	}
	if _, err := os.Stat(state); err == nil {
		t.Error("the state file was written for a delay")
	}
	// the same list is not written twice
	if err := s.SavePending(); err != nil || s.Written().PendingFiles != 1 {
		t.Errorf("written again: %+v %v", s.Written(), err)
	}
	// a power cut (no Close) before any state file: the delay is there at the next start
	s2, _ := open()
	if pending(s2) != 1 {
		t.Fatalf("after the power cut without a state file: %d", pending(s2))
	}
	// the state file, then a second delay only in the pending file: both at the next start
	if err := s2.SaveState(true); err != nil {
		t.Fatal(err)
	}
	if w := s2.Written(); w.StateFiles != 1 || w.PendingFiles != 0 { // (the list is what the file has)
		t.Errorf("written: %+v", w)
	}
	if err := s2.Engine().RunProgram("spaeter", model.RunThen); err != nil {
		t.Fatal(err)
	}
	if err := s2.SavePending(); err != nil {
		t.Fatal(err)
	}
	s3, _ := open()
	if pending(s3) != 2 {
		t.Fatalf("the pending file's list: %d", pending(s3))
	}
	// both are removed; only the pending file hears of it - the state file still lists one
	if err := s3.SaveState(true); err != nil {
		t.Fatal(err)
	}
	for _, p := range s3.Engine().PendingActions() {
		s3.Engine().CancelPending(p.ID)
	}
	if err := s3.SavePending(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(file); string(data) != "{\"pending\":[]}\n" {
		t.Errorf("the empty list: %q", data)
	}
	s4, _ := open()
	if pending(s4) != 0 {
		t.Fatalf("a removed delay came back from the state file: %d", pending(s4))
	}
	// a pending file nobody can read: the state file's list, and the log says so
	if err := s4.Engine().RunProgram("spaeter", model.RunThen); err != nil {
		t.Fatal(err)
	}
	if err := s4.SaveState(true); err != nil {
		t.Fatal(err)
	}
	for _, damage := range []func(){
		func() { _ = os.WriteFile(file, []byte(`{"pending":[`), 0o600) },
		func() { _ = os.Remove(file); _ = os.Mkdir(file, 0o755) },
	} {
		damage()
		s5, logs := open()
		if pending(s5) != 1 {
			t.Errorf("with a damaged pending file: %d", pending(s5))
		}
		if !strings.Contains(strings.Join(*logs, "\n"), "file of the delayed activities") {
			t.Errorf("logs: %v", *logs)
		}
		// and it cannot be written either, when a folder is in its place
		if fi, _ := os.Stat(file); fi.IsDir() {
			if err := s5.Engine().RunProgram("spaeter", model.RunThen); err != nil {
				t.Fatal(err)
			}
			if err := s5.SavePending(); err == nil {
				t.Error("written over a folder")
			}
			if err := s5.SaveState(true); err == nil {
				t.Error("the state's write does not say that the pending file failed")
			}
		}
	}
	_ = os.Remove(file)
	// the state file damaged in both generations: default values, but the delays of their own file
	s6, _ := open()
	if err := s6.SavePending(); err != nil {
		t.Fatal(err)
	}
	n := pending(s6)
	_ = os.WriteFile(state, []byte("{"), 0o600)
	_ = os.WriteFile(state+".1", []byte("{"), 0o600)
	s7, _ := open()
	if n == 0 || pending(s7) != n {
		t.Errorf("without a state: %d delays, had %d", pending(s7), n)
	}
}

// The saver writes the delays a moment after they changed, however often they changed
// meanwhile, and the state at its own pace.
func TestRunSaverCoalesces(t *testing.T) {
	s, err := Open(Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	later(t, s)
	// (the loop guard would stop a program that runs this often)
	s.Engine().SetLoopGuard(engine.LoopGuard{Runs: 1 << 20, Window: time.Second, Pause: time.Second})
	var logs []string
	s.o.Log = func(f string, a ...any) { logs = append(logs, f) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	const pendingEvery, stateEvery = 40 * time.Millisecond, 150 * time.Millisecond
	go func() { s.RunSaver(ctx, stateEvery, pendingEvery); close(done) }()
	start := time.Now()
	changes := 0
	for time.Since(start) < 600*time.Millisecond {
		if err := s.Engine().RunProgram("spaeter", model.RunThen); err != nil {
			t.Fatal(err)
		}
		for _, p := range s.Engine().PendingActions() {
			if changes%2 == 1 {
				s.Engine().CancelPending(p.ID)
			}
		}
		changes++
		time.Sleep(2 * time.Millisecond)
	}
	elapsed := time.Since(start)
	// what is still to come: the last change's write
	time.Sleep(3 * pendingEvery)
	w := s.Written()
	cancel()
	<-done
	// counted over the whole time the saver ran, as for the state below: under -race a slow write
	// lets a tick fall into the waiting time after the last change
	if most := int((elapsed+3*pendingEvery)/pendingEvery) + 1; w.PendingFiles < 3 || w.PendingFiles > most {
		t.Errorf("%d changes in %v: the delays' file written %d times, at most %d expected", changes, elapsed, w.PendingFiles, most)
	}
	if most := int((elapsed+3*pendingEvery)/stateEvery) + 1; w.StateFiles < 1 || w.StateFiles > most {
		t.Errorf("the state written %d times in %v, at most %d expected", w.StateFiles, elapsed, most)
	}
	if changes < 50 {
		t.Errorf("only %d changes", changes)
	}
	// the last change is on disk
	data, _ := os.ReadFile(s.pendingPath())
	if want := len(s.Engine().PendingActions()); strings.Count(string(data), `"id":`) != want {
		t.Errorf("the file has %d delays, the engine %d", strings.Count(string(data), `"id":`), want)
	}
	// a write that fails is said, and the saver goes on
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	_ = os.Remove(s.pendingPath())
	_ = os.Mkdir(s.pendingPath(), 0o755)
	go func() { s.RunSaver(ctx, 20*time.Millisecond, 10*time.Millisecond); close(done) }()
	_ = s.Engine().RunProgram("spaeter", model.RunThen)
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done
	all := strings.Join(logs, "\n")
	if !strings.Contains(all, "the delayed activities could not be written") || !strings.Contains(all, "the state could not be written") {
		t.Errorf("logs: %v", logs)
	}
}

// The budget: with the soak's tree (300 programs, 800 variables, some of the programs with
// delays) under the soak's load (two writes a second), storage gets the state twice a minute
// and the small file of the delays when they changed, at most every five seconds - where the
// whole state was written for every change of a delay (seen on a Raspberry Pi 3: more than a
// megabyte a minute with the run trace).
func TestWriteBudgetUnderDelays(t *testing.T) {
	r := newSoakRig(t, nil)
	s, minute := r.s, func() int { return r.minute(t) }
	for i := 0; i < 3; i++ {
		minute() // until as many delays come due as are scheduled
	}
	before := s.Written()
	asks := minute()
	w := s.Written()
	stateFiles, stateBytes := w.StateFiles-before.StateFiles, w.StateBytes-before.StateBytes
	pendingFiles, pendingBytes := w.PendingFiles-before.PendingFiles, w.PendingBytes-before.PendingBytes
	t.Logf("a minute: the state %d times (%d bytes), the delays %d times (%d bytes); the delays changed in %d of 120 half seconds; %d pending", stateFiles, stateBytes, pendingFiles, pendingBytes, asks, len(s.Engine().PendingActions()))
	if asks < 4 {
		t.Fatalf("the load does not move the delays (%d of 120): the test measures nothing", asks)
	}
	if stateFiles != 2 {
		t.Errorf("the state was written %d times in a minute", stateFiles)
	}
	if pendingFiles < 3 || pendingFiles > 12 {
		t.Errorf("the delays' file was written %d times in a minute", pendingFiles)
	}
	// the budget, and what it was before: the whole state for every one of those writes
	const budget = 250 << 10
	if total := stateBytes + pendingBytes; total > budget {
		t.Errorf("%d bytes a minute, the budget is %d", total, budget)
	}
	if old := stateBytes / int64(stateFiles) * int64(stateFiles+pendingFiles); stateBytes+pendingBytes > old/2 {
		t.Errorf("%d bytes a minute is no better than the %d of a whole state per change", stateBytes+pendingBytes, old)
	}
	if one, whole := pendingBytes/int64(pendingFiles), stateBytes/int64(stateFiles); one*4 > whole {
		t.Errorf("the delays' file (%d bytes) is not small beside the state (%d bytes)", one, whole)
	}
	_ = s.Close()
}

// soakRig is the daemon with the soak's tree under the soak's load, on a clock the test moves.
type soakRig struct {
	s     *Service
	dir   string
	clock *engine.FakeClock
	tree  *soak.Tree
	rng   *rand.Rand
	asked bool
}

// newSoakRig makes the soak's tree (300 programs, 800 variables) in a folder, lets before
// put files beside it, and opens the service.
func newSoakRig(t *testing.T, before func(dir string)) *soakRig {
	t.Helper()
	tree, err := soak.New(soak.Options{Programs: 300, Variables: 800})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for folder, files := range map[string]map[string][]byte{"programs": tree.Programs, "variables": tree.Variables} {
		if err := os.MkdirAll(filepath.Join(dir, "tree", folder), 0o755); err != nil {
			t.Fatal(err)
		}
		for key, data := range files {
			if err := os.WriteFile(filepath.Join(dir, "tree", folder, key+".yaml"), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if before != nil {
		before(dir)
	}
	r := &soakRig{dir: dir, tree: tree, rng: rand.New(rand.NewSource(36))}
	r.clock = engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	if r.s, err = Open(Options{Dir: dir, Clock: r.clock}); err != nil {
		t.Fatal(err)
	}
	if len(r.s.Programs()) != 300 || len(r.s.Variables()) != 800 {
		t.Fatalf("%d programs, %d variables", len(r.s.Programs()), len(r.s.Variables()))
	}
	return r
}

// load writes one of the tree's inputs, as the soak's load does twice a second.
func (r *soakRig) load(t *testing.T) {
	in := r.tree.Inputs[r.rng.Intn(len(r.tree.Inputs))]
	var v any
	switch in.Type {
	case "bool":
		v = r.rng.Intn(2) == 1
	case "number":
		v = float64(r.rng.Intn(100))
	case "list":
		v = soak.Entries[r.rng.Intn(len(soak.Entries))]
	default:
		v = "text " + string(rune('a'+r.rng.Intn(26)))
	}
	if err := r.s.Engine().SetVariable(in.Key, v, engine.FromRPC); err != nil {
		t.Fatalf("%s: %v", in.Key, err)
	}
}

// minute is one minute of the daemon: the load every half second, the saver's two paces. It
// answers in how many of the half seconds the delays changed.
func (r *soakRig) minute(t *testing.T) (asks int) {
	for half := 0; half < 120; half++ {
		r.load(t)
		r.clock.Advance(500 * time.Millisecond)
		select {
		case <-r.s.SaveSoon():
			r.asked = true
			asks++
		default:
		}
		if half%10 == 9 && r.asked {
			r.asked = false
			if err := r.s.SavePending(); err != nil {
				t.Fatal(err)
			}
		}
		if half%60 == 59 {
			if err := r.s.SaveState(false); err != nil {
				t.Fatal(err)
			}
		}
	}
	return asks
}

// fullLogs writes a journal in which every logged variable of the soak's tree has all the
// changes it keeps: the state the week's run reaches after days.
func fullLogs(t *testing.T, r *soak.Tree, dir string) (lines int) {
	t.Helper()
	var b bytes.Buffer
	at := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < engine.LogSize; i++ {
		for key, def := range r.Variables {
			if !bytes.Contains(def, []byte("logged: true")) {
				continue
			}
			fmt.Fprintf(&b, "{\"k\":%q,\"t\":%q,\"v\":%d}\n", key, at.Add(time.Duration(i)*time.Minute).Format(time.RFC3339), i%10)
			lines++
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state", "log.jsonl"), b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return lines
}

// The budget with every log full (the soak's tree has 300 logged variables that change all
// the time; full, their kept changes are ten megabytes): what goes to storage in a minute
// is what it is on the first day - the state file's size follows the variables, not their
// histories, and the journal gets only the new lines. Before, the state file carried every
// kept change: eleven megabytes twice a minute.
func TestWriteBudgetWithFullLogs(t *testing.T) {
	kept := 0
	r := newSoakRig(t, func(dir string) {
		tree, _ := soak.New(soak.Options{Programs: 300, Variables: 800})
		kept = fullLogs(t, tree, dir)
	})
	s := r.s
	if kept != 300*engine.LogSize {
		t.Fatalf("%d lines in the journal", kept)
	}
	journal := filepath.Join(r.dir, "state", "log.jsonl")
	size := func(path string) int64 {
		t.Helper()
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return st.Size()
	}
	full := size(journal)
	for i := 0; i < 3; i++ {
		r.minute(t)
	}
	const minutes = 10
	before := s.Written()
	for i := 0; i < minutes; i++ {
		r.minute(t)
	}
	w := s.Written()
	state := (w.StateBytes - before.StateBytes) / minutes
	pending := (w.PendingBytes - before.PendingBytes) / minutes
	logs := (w.LogBytes - before.LogBytes) / minutes
	trace := (w.TraceBytes - before.TraceBytes) / minutes
	stateFile := size(filepath.Join(r.dir, "state", "state.json"))
	t.Logf("a minute with every log full (the journal %d bytes): the state %d bytes (the file %d), the delays %d, the journal %d, the run trace %d", full, state, stateFile, pending, logs, trace)
	if logs == 0 {
		t.Fatal("the load changes no logged variable: the test measures nothing")
	}
	if stateFile > 100<<10 {
		t.Errorf("the state file has %d bytes", stateFile)
	}
	// the journal's lines are written once when they are new and at most twice more by its
	// rewrites (store.TestJournalCost); the trace's ring is written anew when it has doubled
	const budget = 250 << 10
	if total := state + pending + 3*logs; total > budget {
		t.Errorf("%d bytes a minute for the state, the delays and the journal, the budget is %d", total, budget)
	}
	if logs > 10<<10 {
		t.Errorf("the journal gets %d bytes a minute", logs)
	}
	// everything the daemon writes, the trace's rewrites counted in: the soak's criterion is
	// 0.3 MB a minute at 106 writes a minute - this load is 120
	if total := state + pending + 3*logs + 2*trace; total > 300_000*120/106 {
		t.Errorf("%d bytes a minute in all", total)
	}
	// a read finds a variable's newest among them
	if log, err := s.VariableLog("soak-out-n-005"); err != nil || len(log) != engine.LogSize {
		t.Errorf("a log of %d entries, %v", len(log), err)
	}
	// and the rewrite, when its time has come: the journal is as large as before, and the
	// rewrite is the only thing that was written beside the new lines
	var lines [][]byte
	var bytesNew int64
	for i := 0; i <= kept+1000; i++ {
		l := []byte(fmt.Sprintf("{\"k\":\"soak-out-n-%03d\",\"t\":\"2026-11-23T12:00:00Z\",\"v\":%d}", i%300, i%10))
		lines = append(lines, l)
		bytesNew += int64(len(l)) + 1
	}
	before = s.Written()
	s.logMu.Lock()
	err := s.logs.Append(lines, s.logged())
	s.logMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if now := size(journal); now > full+full/10 {
		t.Errorf("the journal has %d bytes after its rewrite, full it had %d", now, full)
	}
	if rewrite := s.logs.Written() - bytesNew; rewrite > full+full/10 {
		t.Errorf("the rewrite wrote %d bytes", rewrite)
	}
	_ = s.Close()
}
