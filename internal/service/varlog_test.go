package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

func openAt(t *testing.T, dir string, clock *engine.FakeClock) (*Service, *[]string) {
	t.Helper()
	var logs []string
	s, err := Open(Options{Dir: dir, Clock: clock, Log: func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }})
	if err != nil {
		t.Fatal(err)
	}
	s.OnChange(s.queue.Flush)
	return s, &logs
}

func values(log []engine.LogEntry) string {
	var out []string
	for _, e := range log {
		out = append(out, fmt.Sprint(e.Value))
	}
	return strings.Join(out, ",")
}

// The logged variables' changes live in a journal of their own beside the state file: the
// state holds none of them, a read answers a variable's newest, another start finds them.
func TestVariableLog(t *testing.T) {
	dir := t.TempDir()
	clock := engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	s, _ := openAt(t, dir, clock)
	for _, v := range []*model.Variable{
		{Name: "Temp", Type: model.VarNumber, Visible: true, Logged: true},
		{Name: "Regen", Type: model.VarBool, Visible: true, Logged: true},
		{Name: "Text", Type: model.VarString, Visible: true},
	} {
		if _, err := s.CreateVariable(v, nil, admin); err != nil {
			t.Fatal(err)
		}
	}
	set := func(s *Service, key string, v any) {
		t.Helper()
		clock.Advance(time.Second)
		if err := s.Engine().SetVariable(key, v, engine.FromUser); err != nil {
			t.Fatal(err)
		}
	}
	logOf := func(s *Service, key string) string {
		t.Helper()
		log, err := s.VariableLog(key)
		if err != nil {
			t.Fatal(err)
		}
		return values(log)
	}
	set(s, "temp", 1)
	set(s, "regen", true)
	set(s, "temp", 2)
	set(s, "temp", 2) // no change
	set(s, "text", "x")
	journal := filepath.Join(dir, "state", "log.jsonl")
	if _, err := os.Stat(journal); err == nil {
		t.Error("the journal is written before the saver's turn")
	}
	// a read sees what was not written yet (and writes it)
	if logOf(s, "temp") != "1,2" || logOf(s, "regen") != "true" || logOf(s, "text") != "" {
		t.Fatalf("temp %q, regen %q, text %q", logOf(s, "temp"), logOf(s, "regen"), logOf(s, "text"))
	}
	if log, _ := s.VariableLog("temp"); !log[1].Time.Equal(time.Date(2026, 11, 22, 12, 0, 3, 0, time.UTC)) {
		t.Errorf("the time of a change: %v", log[1].Time)
	}
	var we *engine.WriteError
	if _, err := s.VariableLog("nichts"); !errors.As(err, &we) || we.Code != "unknown-variable" {
		t.Errorf("a variable that is not there: %v", err)
	}
	b, err := os.ReadFile(journal)
	if err != nil || string(b) != `{"k":"temp","t":"2026-11-22T12:00:01Z","v":1}
{"k":"regen","t":"2026-11-22T12:00:02Z","v":true}
{"k":"temp","t":"2026-11-22T12:00:03Z","v":2}
` {
		t.Fatalf("the journal: %s %v", b, err)
	}
	set(s, "temp", 3)
	if err := s.SaveState(false); err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(dir, "state", "state.json"))
	if err != nil || strings.Contains(string(state), `"log"`) || !strings.Contains(string(state), `"temp":{"value":3`) {
		t.Fatalf("the state file: %s %v", state, err)
	}
	if w := s.Written(); w.LogBytes != int64(len(b))+46 || w.TraceBytes != 0 {
		t.Errorf("written: %+v", w)
	}
	// more than are kept: the newest LogSize
	for i := 0; i < engine.LogSize+20; i++ {
		set(s, "temp", 100+i)
	}
	if log, err := s.VariableLog("temp"); err != nil || len(log) != engine.LogSize || log[0].Value != 120.0 || log[len(log)-1].Value != float64(100+engine.LogSize+19) {
		t.Fatalf("%d entries, %v", len(log), err)
	}
	set(s, "regen", false)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// another start: the journal is all there is to read
	s, _ = openAt(t, dir, clock)
	if log, err := s.VariableLog("temp"); err != nil || len(log) != engine.LogSize || log[0].Value != 120.0 {
		t.Fatalf("after a start: %d entries, %v", len(log), err)
	}
	if logOf(s, "regen") != "true,false" {
		t.Errorf("regen after a start: %q", logOf(s, "regen"))
	}
	// no longer logged: what was kept is void, also when it is logged again later
	def, _ := s.Engine().Variable("regen")
	off := *def.Def
	off.Logged = false
	if _, err := s.UpdateVariable("regen", &off, nil, admin); err != nil {
		t.Fatal(err)
	}
	if logOf(s, "regen") != "" {
		t.Errorf("not logged: %q", logOf(s, "regen"))
	}
	on := off
	on.Logged = true
	if _, err := s.UpdateVariable("regen", &on, nil, admin); err != nil {
		t.Fatal(err)
	}
	set(s, "regen", true)
	if logOf(s, "regen") != "true" {
		t.Errorf("logged again: %q", logOf(s, "regen"))
	}
	// a removed variable's lines go with the next rewrite: here the file holds more than
	// twice what it kept at the start and the slack
	if err := s.DeleteVariable("temp", admin); err != nil {
		t.Fatal(err)
	}
	set(s, "regen", false)
	if err := s.SaveState(false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1600; i++ {
		set(s, "regen", i%2 == 0)
	}
	if err := s.SaveState(false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(journal)
	if strings.Contains(string(b), `"temp"`) || strings.Contains(string(b), "clear") || strings.Count(string(b), "\n") != engine.LogSize {
		t.Errorf("after the rewrite the journal has %d lines, temp in it: %t", strings.Count(string(b), "\n"), strings.Contains(string(b), `"temp"`))
	}
	_ = s.Close()
}

// A power cut: the journal's last line is cut short, or the lines of the last half minute
// never arrived - the start reads what is whole, and goes on writing after it. A journal that
// cannot be written keeps its lines for the next turn.
func TestVariableLogCutOff(t *testing.T) {
	dir := t.TempDir()
	clock := engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	s, _ := openAt(t, dir, clock)
	if _, err := s.CreateVariable(&model.Variable{Name: "Temp", Type: model.VarNumber, Visible: true, Logged: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	set := func(v any) {
		t.Helper()
		clock.Advance(time.Second)
		if err := s.Engine().SetVariable("temp", v, engine.FromUser); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 3; i++ {
		set(i)
	}
	if err := s.SaveState(false); err != nil {
		t.Fatal(err)
	}
	set(4) // never written: the power is cut before the saver's turn
	journal := filepath.Join(dir, "state", "log.jsonl")
	whole, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	// (the process is gone: no Close) - the last line arrived in part, a rewrite's temporary
	// file was left, and something that is no line stands in the middle
	torn := append(append([]byte("not a line\n{\"k\":\"Not A Key\",\"t\":1}\n{\"k\":\"temp\",\"v\":\"no end\"\n"), whole...), []byte(`{"k":"temp","t":"2026-11-22T12:0`)...)
	if err := os.WriteFile(journal, torn, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state", ".tmp-123456"), whole[:10], 0o600); err != nil {
		t.Fatal(err)
	}
	s, logs := openAt(t, dir, clock)
	if log, err := s.VariableLog("temp"); err != nil || values(log) != "1,2,3" {
		t.Fatalf("after the cut: %v, %v", values(log), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "state", ".tmp-123456")); err == nil || !strings.Contains(strings.Join(*logs, "\n"), "removed 1 temporary files") {
		t.Errorf("the temporary file is still there: %v", *logs)
	}
	set(5)
	if log, err := s.VariableLog("temp"); err != nil || values(log) != "1,2,3,5" {
		t.Fatalf("after the next change: %v, %v", values(log), err)
	}
	b, _ := os.ReadFile(journal)
	if !strings.HasSuffix(string(b), "{\"k\":\"temp\",\"t\":\"2026-11-22T12:0\n{\"k\":\"temp\",\"t\":\"2026-11-22T12:00:05Z\",\"v\":5}\n") {
		t.Errorf("the new line does not begin on a line of its own: %q", b)
	}
	// the journal cannot be written: the lines wait, a read still answers them, and they
	// are written when it can be again
	if err := os.Rename(journal, journal+".aside"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0o755); err != nil {
		t.Fatal(err)
	}
	s.logs = newLogJournal(journal)
	set(6)
	if err := s.SaveState(false); err == nil {
		t.Error("a journal that is a folder was written")
	}
	set(7)
	if _, err := s.VariableLog("temp"); err == nil {
		t.Error("a journal that is a folder was read")
	}
	if len(s.logWait) != 2 {
		t.Fatalf("%d lines wait", len(s.logWait))
	}
	// (not without end)
	for i := 0; i < logWaitMax+5; i++ {
		s.logWait = append(s.logWait, []byte(`{"k":"temp","v":0}`))
	}
	_ = s.saveLogs()
	if len(s.logWait) != logWaitMax {
		t.Fatalf("%d lines wait", len(s.logWait))
	}
	s.logWait = s.logWait[:2]
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(journal+".aside", journal); err != nil {
		t.Fatal(err)
	}
	s.logs = newLogJournal(journal)
	// a clear among the waiting lines counts for a read as in the file
	s.logWait = append(s.logWait, []byte(`{"k":"temp","clear":true}`), []byte(`{"k":"temp","t":"2026-11-22T12:00:09Z","v":8}`), []byte(`{"k":"other","v":1}`))
	blocked := newLogJournal(filepath.Join(dir, "none", "log.jsonl"))
	s.logs, blocked = blocked, s.logs
	if _, err := s.VariableLog("temp"); err != nil {
		t.Fatal(err) // no file to read is no error; the write's is said by the saver
	}
	if log, _ := s.VariableLog("temp"); values(log) != "8" {
		t.Errorf("waiting lines with a clear: %v", values(log))
	}
	s.logs = blocked
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if len(s.logWait) != 0 {
		t.Errorf("%d lines still wait", len(s.logWait))
	}
	s, _ = openAt(t, dir, clock)
	if log, err := s.VariableLog("temp"); err != nil || values(log) != "8" {
		t.Errorf("after all: %v, %v", values(log), err)
	}
	_ = s.Close()
}

// A state file of before the journal holds the logged changes: they are moved into the
// journal once, and the state file is small from its next write on.
func TestVariableLogMoved(t *testing.T) {
	dir := t.TempDir()
	clock := engine.NewFakeClock(time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC))
	s, _ := openAt(t, dir, clock)
	for _, v := range []*model.Variable{
		{Name: "Temp", Type: model.VarNumber, Visible: true, Logged: true},
		{Name: "Regen", Type: model.VarBool, Visible: true, Logged: true},
		{Name: "Frueher", Type: model.VarBool, Visible: true},
	} {
		if _, err := s.CreateVariable(v, nil, admin); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state", "state.json")
	journal := filepath.Join(dir, "state", "log.jsonl")
	old := `{"variables":{
"temp":{"value":3,"time":"2026-11-22T11:00:03Z","log":[{"time":"2026-11-22T11:00:01Z","value":1},{"time":"2026-11-22T11:00:03Z","value":3}]},
"regen":{"value":true,"log":[{"time":"2026-11-22T11:00:02Z","value":true}]},
"frueher":{"value":false,"log":[{"time":"2026-11-22T10:00:00Z","value":true}]},
"weg":{"value":1,"log":[{"time":"2026-11-22T11:00:01Z","value":1}]}}}`
	if err := os.WriteFile(state, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(journal); err == nil {
		t.Fatal("a journal without a change")
	}
	s, logs := openAt(t, dir, clock)
	if !strings.Contains(strings.Join(*logs, "\n"), "moved 5 logged changes out of the state file") {
		t.Errorf("logs: %v", *logs)
	}
	b, err := os.ReadFile(journal)
	if err != nil || !strings.HasPrefix(string(b), `{"k":"frueher","t":"2026-11-22T10:00:00Z","v":true}
{"k":"temp","t":"2026-11-22T11:00:01Z","v":1}
{"k":"weg","t":"2026-11-22T11:00:01Z","v":1}
{"k":"regen",`) {
		t.Fatalf("the journal: %s %v", b, err)
	}
	if log, err := s.VariableLog("temp"); err != nil || values(log) != "1,3" {
		t.Errorf("temp: %v %v", values(log), err)
	}
	if log, err := s.VariableLog("frueher"); err != nil || len(log) != 0 {
		t.Errorf("a variable that is not logged: %v %v", log, err)
	}
	if v, _ := s.Engine().Value("temp"); v != 3.0 {
		t.Errorf("the value: %v", v)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(state); strings.Contains(string(b), `"log"`) {
		t.Errorf("the state file still holds logs: %s", b)
	}
	// the state file before it still has them: it is not moved a second time
	if err := os.WriteFile(state, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, logs = openAt(t, dir, clock)
	if strings.Contains(strings.Join(*logs, "\n"), "moved") {
		t.Errorf("moved twice: %v", *logs)
	}
	if log, _ := s.VariableLog("temp"); values(log) != "1,3" {
		t.Errorf("temp: %v", values(log))
	}
	_ = s.Close()
	// a journal that cannot be made: said, and the start goes on
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journal, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ = openAt(t, dir, clock)
	if v, _ := s.Engine().Value("temp"); v != 3.0 {
		t.Errorf("the value: %v", v)
	}
	_ = s.Close()
	// (a folder "exists": nothing is tried; a path that cannot be written is)
	dir2 := t.TempDir()
	s, _ = openAt(t, dir2, clock)
	if _, err := s.CreateVariable(&model.Variable{Name: "Temp", Type: model.VarNumber, Visible: true, Logged: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	s.logs = newLogJournal(filepath.Join(dir2, "none", "log.jsonl"))
	var said []string
	s.o.Log = func(f string, a ...any) { said = append(said, f) }
	st, _ := engine.UnmarshalState([]byte(old))
	s.moveLogs(st)
	s.moveLogs(engine.State{})
	if len(said) != 1 || !strings.Contains(said[0], "could not be moved") {
		t.Errorf("said: %v", said)
	}
	_ = s.Close()
}

func TestLogLineKey(t *testing.T) {
	for line, want := range map[string]string{
		`{"k":"temp","t":"2026-11-22T12:00:01Z","v":1}`:                  "temp",
		`{"k":"temp","clear":true}`:                                      "temp!",
		`{"k":"temp","t":"2026-11-22T12:00:01Z","v":",\"clear\":true}"}`: "temp",
		`{"k":"temp","t":"2026-11-22T12:0`:                               "-",
		`{"k":"Temp","v":1}`:                                             "-",
		`{"k":"","v":1}`:                                                 "-",
		`{"k":"temp}`:                                                    "-",
		`{"x":"temp"}`:                                                   "-",
		`}`:                                                              "-",
	} {
		key, clear, ok := logLineKey([]byte(line))
		got := key
		if clear {
			got += "!"
		}
		if !ok {
			got = "-"
		}
		if got != want {
			t.Errorf("%s: %q, want %q", line, got, want)
		}
	}
}
