package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// State is what the engine remembers between starts: the variables' values with their times,
// the alarms - not the logged variables' changes, which have a journal of their own (LogLine):
// the state's size follows the number of variables and programs only. It is JSON in the state file - not in the tree of program and
// variable files, and so not in the history.
type State struct {
	Variables map[string]VariableState `json:"variables,omitempty"`
	// Programs holds what each program's lines last evaluated to; Pending the delayed
	// activities that were not due yet.
	Programs map[string]ProgramState `json:"programs,omitempty"`
	Pending  []Pending               `json:"pending,omitempty"`
}

// ProgramState is one program's part of the state: the truth each of its condition lines had
// when the program last looked at it (1 true, 0 false, -1 not looked at), valid for the
// conditions with this fingerprint.
type ProgramState struct {
	Conditions string `json:"conditions"`
	Truth      []int8 `json:"truth"`
	// Since says, per line with "for", since when its comparison held (zero: it did not).
	Since []time.Time `json:"since,omitempty"`
}

// VariableState is one variable's part of the state.
type VariableState struct {
	Value any         `json:"value"`
	Time  time.Time   `json:"time,omitzero"`
	Since time.Time   `json:"since,omitzero"`
	Alarm *AlarmState `json:"alarm,omitempty"`
	// Log is only read: a state file of before the journal held the logged changes here
	// (the daemon moves them over once); Export never fills it.
	Log []LogEntry `json:"log,omitempty"`
	// Helper is what the device helper the variable leads remembers.
	Helper *model.HelperState `json:"helper,omitempty"`
}

// Export answers the state, and whether it changed since the last Export.
func (e *Engine) Export() (State, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := State{Variables: make(map[string]VariableState, len(e.vars))}
	for k, st := range e.vars {
		vs := VariableState{Value: st.value, Time: st.ts, Since: st.lc}
		if st.alarm != (AlarmState{}) {
			a := st.alarm
			vs.Alarm = &a
		}
		if st.helper != (model.HelperState{}) {
			h := st.helper
			vs.Helper = &h
		}
		s.Variables[k] = vs
	}
	s.Programs = make(map[string]ProgramState, len(e.progs))
	for k, ps := range e.progs {
		st := ProgramState{Conditions: fingerprint(ps.def), Truth: make([]int8, len(ps.lines))}
		for i, l := range ps.lines {
			st.Truth[i] = l.truth
			if l.unseen {
				st.Truth[i] = truthUnknown
			}
			if !l.since.IsZero() {
				if st.Since == nil {
					st.Since = make([]time.Time, len(ps.lines))
				}
				st.Since[i] = l.since
			}
		}
		s.Programs[k] = st
	}
	for _, p := range e.pending {
		s.Pending = append(s.Pending, p.Pending)
	}
	dirty := e.dirty
	e.dirty = false
	return s, dirty
}

// fingerprint identifies a program's condition lines: the truths of a stored state belong to
// the lines that were there when it was written.
func fingerprint(p *model.Program) string {
	c := p.Clone()
	c.Name, c.Note, c.Active, c.Visible, c.Operable, c.Internal, c.AtStart = "x", "", false, true, true, false, false
	for i := range c.Rules {
		c.Rules[i].Then, c.Rules[i].Note, c.Rules[i].CancelPending = nil, "", false
	}
	sum := sha256.Sum256(model.EncodeProgram(c, nil))
	return hex.EncodeToString(sum[:8])
}

// Import takes a stored state for the variables that exist: a value that no longer fits its
// variable is left out (the variable keeps its default). Nothing here is an event.
func (e *Engine) Import(s State) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, vs := range s.Variables {
		st := e.vars[k]
		if st == nil {
			continue
		}
		if st.def.CheckValue(vs.Value) != nil {
			e.logf("variable %s: the stored value %v does not fit, kept the default", k, vs.Value)
			continue
		}
		st.value, st.ts, st.lc = vs.Value, vs.Time, vs.Since
		if vs.Alarm != nil && st.def.Type == "alarm" {
			st.alarm = *vs.Alarm
		}
		if vs.Helper != nil && st.def.Helper != nil {
			st.helper = *vs.Helper
		}
	}
	e.sys[SystemAlarmMessages] = float64(e.alarmMessagesLocked())
	// the lines' truths, where the conditions are still the ones the state was written for; a
	// line that was not looked at then starts with the truth it has now
	for k, st := range s.Programs {
		ps := e.progs[k]
		if ps == nil || st.Conditions != fingerprint(ps.def) || len(st.Truth) != len(ps.lines) {
			continue
		}
		for i, l := range ps.lines {
			if t := st.Truth[i]; t == truthTrue || t == truthFalse {
				l.truth, l.unseen = t, false
			}
			if i < len(st.Since) && !st.Since[i].IsZero() && l.def.For > 0 {
				// the comparison held when the state was written: if it still does, it has
				// held since then - whether it broke in between nobody can know
				e.stopHold(l)
				l.since, l.hint = time.Time{}, st.Since[i]
				e.track(k, i, l)
			}
		}
	}
	// the delayed activities: one that is still in the future is armed again, one that came
	// due while the addon was stopped is dropped - late is worse than never for a delay
	now := e.o.Clock.Now()
	for _, p := range s.Pending {
		switch {
		case e.progs[p.Program] == nil:
		case !p.Due.After(now):
			e.logf("a delayed activity of %s was due at %s, while the addon was stopped: dropped", p.Program, p.Due.Format(time.RFC3339))
			a := p.Action
			run := &Run{Program: p.Program, Time: now, Trigger: p.By, Block: p.Rule + 1, Delayed: true, Steps: []Step{{Action: p.Index, Def: &a, What: StepDropped, Due: p.Due}}}
			if ps := e.progs[p.Program]; p.Rule < len(ps.def.Rules) && ps.def.Rules[p.Rule].Else {
				run.Block = 0
			}
			e.record(e.progs[p.Program], run)
		default:
			e.schedule(p)
		}
	}
}

// MarshalState and UnmarshalState are the state file's form.
func MarshalState(s State) ([]byte, error) { return json.Marshal(s) }

// UnmarshalState reads a state file's bytes.
func UnmarshalState(b []byte) (State, error) {
	var s State
	err := json.Unmarshal(b, &s)
	return s, err
}
