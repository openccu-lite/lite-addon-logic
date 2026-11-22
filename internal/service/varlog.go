package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// The logged variables' changes: a journal beside the state file (state/log.jsonl), a line per
// change - {"k":"<key>","t":"<time>","v":<value>} - appended at the state's pace with only the
// new lines, and rewritten with the newest engine.LogSize lines of every logged variable once
// it holds more than twice of that. Nothing of it is kept in memory, and the state file holds
// none of it: a system that logs twenty sensor values writes a few hundred bytes a minute for
// them, where the state file used to carry every kept change at every write.
//
// A power cut costs the lines since the last write (half a minute at most) and may leave a
// line cut short, which is not read; the rewrite is atomic (store.Journal).

// logWaitMax is how many lines that could not be written wait for the next turn.
const logWaitMax = 10000

func (s *Service) logPath() string { return filepath.Join(s.o.Dir, "state", "log.jsonl") }

func newLogJournal(path string) store.Journal {
	return store.Journal{Path: path, Max: engine.LogSize, Slack: 1000, Key: logLineKey}
}

// logLineKey reads a journal line's key without decoding the line: every line is written
// here and begins with it.
func logLineKey(line []byte) (key string, clear, ok bool) {
	const head = `{"k":"`
	if !bytes.HasPrefix(line, []byte(head)) || line[len(line)-1] != '}' {
		return "", false, false
	}
	rest := line[len(head):]
	end := bytes.IndexByte(rest, '"')
	if end <= 0 || !model.ValidKey(string(rest[:end])) {
		return "", false, false
	}
	return string(rest[:end]), bytes.HasSuffix(rest[end:], []byte(`","clear":true}`)), true
}

// logged answers the test a rewrite of the journal keeps a key by: the variable is there and
// logged now. (The set is looked up when the first line asks: most turns rewrite nothing.)
func (s *Service) logged() func(key string) bool {
	var now map[string]bool
	return func(key string) bool {
		if now == nil {
			now = map[string]bool{}
			for _, v := range s.eng.Variables() {
				if v.Def.Logged {
					now[v.Def.Key] = true
				}
			}
		}
		return now[key]
	}
}

// saveLogs appends the logged variables' new changes to the journal.
func (s *Service) saveLogs() error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	return s.saveLogsLocked()
}

func (s *Service) saveLogsLocked() error {
	lines := s.logWait
	for _, l := range s.eng.TakeLog() {
		b, err := json.Marshal(l)
		if err != nil {
			continue // a value JSON cannot say: no variable has one
		}
		lines = append(lines, b)
	}
	s.logWait = nil
	before := s.logs.Written()
	err := s.logs.Append(lines, s.logged())
	s.saver.mu.Lock()
	s.saver.written.LogBytes += s.logs.Written() - before
	s.saver.mu.Unlock()
	if err != nil {
		// they wait for the next turn - not without end
		if len(lines) > logWaitMax {
			lines = lines[len(lines)-logWaitMax:]
		}
		s.logWait = lines
	}
	return err
}

// VariableLog answers the kept changes of a logged variable, oldest first: the last
// engine.LogSize. A variable that is not logged has none.
func (s *Service) VariableLog(key string) ([]engine.LogEntry, error) {
	in, ok := s.eng.Variable(key)
	if !ok {
		return nil, &engine.WriteError{Code: "unknown-variable", Msg: "there is no variable " + key}
	}
	out := []engine.LogEntry{}
	if !in.Def.Logged {
		return out, nil
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	// what is new goes to the file first: then the file says it all, in order
	werr := s.saveLogsLocked()
	lines, err := s.logs.Read(key)
	if err != nil {
		return nil, errors.Join(err, werr)
	}
	for _, l := range s.logWait {
		if k, clear, ok := logLineKey(l); ok && k == key {
			if clear {
				lines = lines[:0]
				continue
			}
			lines = append(lines, l)
		}
	}
	for _, l := range lines {
		var e engine.LogLine
		if json.Unmarshal(l, &e) == nil {
			out = append(out, engine.LogEntry{Time: e.Time, Value: e.Value})
		}
	}
	if len(out) > engine.LogSize {
		out = out[len(out)-engine.LogSize:]
	}
	return out, nil
}

// moveLogs takes the logged changes a state file of before the journal held into the journal:
// once, when there is no journal yet.
func (s *Service) moveLogs(st engine.State) {
	if s.logs.Exists() {
		return
	}
	var all []engine.LogLine
	for key, vs := range st.Variables {
		for _, e := range vs.Log {
			all = append(all, engine.LogLine{Key: key, Time: e.Time, Value: e.Value})
		}
	}
	if len(all) == 0 {
		return
	}
	sort.SliceStable(all, func(a, b int) bool {
		if !all[a].Time.Equal(all[b].Time) {
			return all[a].Time.Before(all[b].Time)
		}
		return all[a].Key < all[b].Key
	})
	lines := make([][]byte, 0, len(all))
	for _, l := range all {
		if b, err := json.Marshal(l); err == nil {
			lines = append(lines, b)
		}
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if err := s.logs.Append(lines, s.logged()); err != nil {
		s.logf("the logged variables' changes could not be moved out of the state file: %v", err)
		return
	}
	s.logf("moved %d logged changes out of the state file into a file of their own", len(lines))
}
