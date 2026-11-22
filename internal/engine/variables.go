package engine

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

// The alarm states, as the CCU's AlState.
const (
	AlarmNever        = 0
	AlarmOncoming     = 1
	AlarmAcknowledged = 2
)

// The system values' keys, as the model has them.
const (
	SystemAlarmMessages   = model.SystemAlarmMessages
	SystemServiceMessages = model.SystemServiceMessages
	SystemDutyCycle       = model.SystemDutyCycle
)

// IsSystemValue says whether a key is a system value's.
func IsSystemValue(key string) bool { return model.IsSystemValue(key) }

// LogSize is how many changes of a logged variable are kept.
const LogSize = 500

// logBuffer is how many lines of the logged variables' changes wait for TakeLog at most:
// with nobody taking them, the oldest go.
const logBuffer = 10000

// varState is a variable's value and what goes with it.
type varState struct {
	def   *model.Variable
	value any
	// ts is the last write, lc the last change of the value.
	ts, lc time.Time
	alarm  AlarmState
	// helper is what the device helper this variable leads remembers
	helper model.HelperState
}

// AlarmState is what an alarm variable carries beside its value.
type AlarmState struct {
	// State: AlarmNever, AlarmOncoming, AlarmAcknowledged.
	State int `json:"state"`
	// Count is how often the alarm was triggered since it last went from false to true.
	Count int `json:"count"`
	// First is the first trigger of the current occurrence.
	First time.Time `json:"first,omitzero"`
	// Trigger is what set the alarm last: "variable:<key>", "device:<ref>", "program:<key>"
	// for a manual run, "outside" for a write through the interface, "user" for the page.
	Trigger string `json:"trigger,omitempty"`
}

// LogEntry is one change of a logged variable.
type LogEntry struct {
	Time  time.Time `json:"time"`
	Value any       `json:"value"`
}

// LogLine is one line of the logged variables' journal: a change of the variable Key, or -
// with Clear - the note that what was kept of Key is void (the variable is gone, no longer
// logged, or of another type now). The engine keeps no variable's log itself: it hands the
// lines to whoever takes them (TakeLog), and the daemon keeps them in a file of their own.
// The field order is the file's: a line begins with its key.
type LogLine struct {
	Key   string    `json:"k"`
	Time  time.Time `json:"t,omitzero"`
	Value any       `json:"v,omitempty"`
	Clear bool      `json:"clear,omitempty"`
}

// logLine files a line for TakeLog (with the lock held).
func (e *Engine) logLine(l LogLine) {
	e.logNew = append(e.logNew, l)
	if len(e.logNew) > logBuffer {
		e.logNew = append([]LogLine(nil), e.logNew[len(e.logNew)-logBuffer/2:]...)
	}
}

// TakeLog answers the logged variables' changes since the last TakeLog, oldest first, and
// forgets them.
func (e *Engine) TakeLog() []LogLine {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := e.logNew
	e.logNew = nil
	return out
}

// VariableInfo is a variable as the page and the interface see it.
type VariableInfo struct {
	Def   *model.Variable `json:"def"`
	Value any             `json:"value"`
	// Time is the last write, Changed the last change of the value (zero: never).
	Time    time.Time   `json:"time,omitzero"`
	Changed time.Time   `json:"changed,omitzero"`
	Alarm   *AlarmState `json:"alarm,omitempty"`
}

// ErrUnknownVariable, ErrReadOnly and the errors of a refused write.
type WriteError struct {
	// Code: unknown-variable, wrong-type, out-of-range, read-only.
	Code string
	Msg  string
}

func (e *WriteError) Error() string { return e.Msg }

func writeErr(code, format string, args ...any) *WriteError {
	return &WriteError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// SetVariables replaces the variables' definitions. A variable that stays keeps its value when
// the value still fits its type (a list's entry that is gone falls back to the first); a new
// one gets its default; one that is gone loses its value.
func (e *Engine) SetVariables(defs map[string]*model.Variable) {
	e.mu.Lock()
	defer e.mu.Unlock()
	next := make(map[string]*varState, len(defs))
	for k, def := range defs {
		if IsSystemValue(k) {
			e.logf("variable %s: the key is a system value's, the variable is left out", k)
			continue
		}
		st := e.vars[k]
		if st != nil && st.def.Logged && (!def.Logged || st.def.Type != def.Type) {
			// what was logged of it is of no use any more
			e.logLine(LogLine{Key: k, Clear: true})
		}
		if st == nil || def.CheckValue(st.value) != nil {
			if st != nil {
				e.logf("variable %s: its value %v no longer fits, set to the default", k, st.value)
			}
			old := st
			st = &varState{value: def.Default()}
			if old != nil && old.def.Type == def.Type {
				st.alarm = old.alarm
			}
			e.dirty = true
		}
		if def.Type != model.VarAlarm {
			st.alarm = AlarmState{}
		}
		st.def = def
		next[k] = st
	}
	for k, st := range e.vars {
		if next[k] == nil && st.def.Logged {
			e.logLine(LogLine{Key: k, Clear: true})
		}
	}
	if len(next) != len(e.vars) {
		e.dirty = true
	}
	e.vars = next
	e.set.Variables = defs
	// the number of alarm messages follows the variables that are there now, without an event
	e.sys[SystemAlarmMessages] = float64(e.alarmMessagesLocked())
	e.indexComputed()
	e.recomputeAll()
	e.indexHelpers()
	e.arm()
}

// RenameEntry follows a renamed entry of a list variable: a value that is the old label becomes
// the new one. Call it before SetVariables with the renamed definition.
func (e *Engine) RenameEntry(key, from, to string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if st := e.vars[key]; st != nil && st.value == any(from) {
		st.value = to
		e.dirty = true
	}
}

func (e *Engine) info(st *varState) VariableInfo {
	in := VariableInfo{Def: st.def, Value: st.value, Time: st.ts, Changed: st.lc}
	if st.def.Type == model.VarAlarm {
		a := st.alarm
		in.Alarm = &a
	}
	return in
}

// Variable answers one variable.
func (e *Engine) Variable(key string) (VariableInfo, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.vars[key]
	if st == nil {
		return VariableInfo{}, false
	}
	return e.info(st), true
}

// Variables answers every variable, sorted by key.
func (e *Engine) Variables() []VariableInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]VariableInfo, 0, len(e.vars))
	for _, st := range e.vars {
		out = append(out, e.info(st))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Def.Key < out[j].Def.Key })
	return out
}

// Value answers a variable's value, or a system value's; false when there is none of that key.
func (e *Engine) Value(key string) (any, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.valueLocked(key)
}

func (e *Engine) valueLocked(key string) (any, bool) {
	if IsSystemValue(key) {
		return e.sys[key], true
	}
	if st := e.vars[key]; st != nil {
		return st.value, true
	}
	return nil, false
}

func (e *Engine) alarmMessagesLocked() int {
	n := 0
	for _, st := range e.vars {
		if st.def.Type == model.VarAlarm && st.alarm.State == AlarmOncoming && st.value == any(true) {
			n++
		}
	}
	return n
}

// AlarmMessages lists the alarm messages - the alarm variables that are triggered and not
// acknowledged - oldest occurrence first.
func (e *Engine) AlarmMessages() []VariableInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []VariableInfo
	for _, st := range e.vars {
		if st.def.Type == model.VarAlarm && st.alarm.State == AlarmOncoming && st.value == any(true) {
			out = append(out, e.info(st))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Alarm.First.Equal(out[j].Alarm.First) {
			return out[i].Alarm.First.Before(out[j].Alarm.First)
		}
		return out[i].Def.Key < out[j].Def.Key
	})
	return out
}

// SetVariable writes a variable. Every write is an update for the programs that listen to it,
// and a change when the value differs - there is no silent write. What is refused: a variable
// that does not exist, a value of another type, an entry a list does not have (for everyone),
// and a number outside its limits when a person or another program writes it (not when a
// program of this addon does: the CCU stores what its programs write, and an imported counter
// beyond its maximum must keep counting).
func (e *Engine) SetVariable(key string, value any, src Source) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	trigger := "user"
	if src == FromRPC {
		trigger = "outside"
	}
	if err := e.write(key, value, src, trigger); err != nil {
		return err
	}
	e.drain()
	return nil
}

// Acknowledge acknowledges an alarm: on the CCU that is a write of false, with everything a
// write of false does. Unlike a plain write of false it is an event also when the alarm is
// false already ("bei Aktualisierung" lines run) - both as measured on the reference.
func (e *Engine) Acknowledge(key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.vars[key]
	if st == nil {
		return writeErr("unknown-variable", "there is no variable %s", key)
	}
	if st.def.Type != model.VarAlarm {
		return writeErr("wrong-type", "%s is not an alarm", key)
	}
	e.acknowledging = true
	err := e.write(key, false, FromUser, "user")
	e.acknowledging = false
	if err != nil {
		return err
	}
	e.drain()
	return nil
}

// writable says whether a variable takes a value from this writer, and answers the variable and
// the value in the form it is kept in. It changes nothing.
func (e *Engine) writable(key string, value any, src Source) (*varState, any, error) {
	st := e.vars[key]
	if IsSystemValue(key) {
		return nil, nil, writeErr("read-only", "%s is a system value and cannot be written", key)
	}
	if st == nil {
		return nil, nil, writeErr("unknown-variable", "there is no variable %s", key)
	}
	def := st.def
	if def.Computed != nil && !e.computing {
		return nil, nil, writeErr("read-only", "%s is a computed value and cannot be written", key)
	}
	if h := def.Helper; h != nil && h.Role != model.RoleTotal && !e.helping && src != FromImport {
		return nil, nil, writeErr("read-only", "%s is kept by a device helper and cannot be written", key)
	}
	if n, ok := value.(int); ok {
		value = float64(n)
	}
	if err := def.CheckValue(value); err != nil {
		return nil, nil, writeErr("wrong-type", "%v", err)
	}
	if f, ok := value.(float64); ok && (src == FromUser || src == FromRPC) {
		if (def.Min != nil && f < *def.Min) || (def.Max != nil && f > *def.Max) {
			return nil, nil, writeErr("out-of-range", "%s takes %s", key, limits(def))
		}
	}
	return st, value, nil
}

// write does a write with the lock held and posts its event; the caller drains.
func (e *Engine) write(key string, value any, src Source, trigger string) error {
	st, value, err := e.writable(key, value, src)
	if err != nil {
		return err
	}
	def := st.def
	now := e.o.Clock.Now()
	prev := st.value
	changed := !equal(prev, value)
	if def.Type == model.VarAlarm && !changed && value == false && src != FromImport && !e.acknowledging {
		// a write of false to an alarm that is false already is nothing on the CCU - not even
		// "bei Aktualisierung" lines run, unlike on a boolean variable, where every write is an
		// event. (The acknowledge itself, repeated, is one: see Acknowledge.)
		return nil
	}
	st.value = value
	e.dirty = true
	if src == FromImport {
		// a value an import brought along: there before anything listens, never an event
		e.recompute("var:"+key, true)
		return nil
	}
	st.ts = now
	if changed {
		st.lc = now
		if def.Logged {
			e.logLine(LogLine{Key: key, Time: now, Value: value})
		}
	}
	if def.Type == model.VarAlarm {
		e.alarm(st, value.(bool), changed, now, trigger)
	}
	e.notify(Notification{Kind: "variable", Key: key, Value: value, Changed: changed})
	e.post(event{source: "var:" + key, value: value, prev: prev, changed: changed, at: now})
	if def.Type == model.VarAlarm {
		e.syncAlarmMessages()
	}
	return nil
}

// syncAlarmMessages announces the number of alarm messages when it moved: an event like a
// variable's write, so programs on the system value run.
func (e *Engine) syncAlarmMessages() {
	e.setSystem(SystemAlarmMessages, float64(e.alarmMessagesLocked()))
}

// setSystem takes a system value (with the lock held; the caller drains).
func (e *Engine) setSystem(key string, value float64) {
	prev, known := e.sys[key]
	if known && prev == value {
		return
	}
	e.sys[key] = value
	e.notify(Notification{Kind: "variable", Key: key, Value: value, Changed: true})
	e.post(event{source: "var:" + key, value: value, prev: prev, changed: true, at: e.at()})
}

// SetSystemValue feeds a system value from outside the engine: the number of service messages,
// the duty cycle. The number of alarm messages is the engine's own.
func (e *Engine) SetSystemValue(key string, value float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !IsSystemValue(key) || key == SystemAlarmMessages {
		return
	}
	e.setSystem(key, value)
	e.drain()
}

// UpdateSun computes where the sun stands now and takes it into the two system values, to a
// tenth of a degree: an event for programs that look at them, when a value moved. Without a
// position, or while the clock is not set, nothing happens. The daemon calls it every few
// minutes.
func (e *Engine) UpdateSun() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.o.Position.Known || !e.clockReady(e.now()) {
		return
	}
	az, el := schedule.SunPosition(e.now(), e.o.Position)
	e.setSystem(model.SystemSunAzimuth, math.Round(az*10)/10)
	e.setSystem(model.SystemSunElevation, math.Round(el*10)/10)
	e.drain()
}

// SystemValue is a system value as the page and the interface see it.
type SystemValue struct {
	Key   string  `json:"key"`
	Value float64 `json:"value"`
}

// SystemValues answers the system values, in the model's order.
func (e *Engine) SystemValues() []SystemValue {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]SystemValue, 0, len(model.SystemValues))
	for _, k := range model.SystemValues {
		v, _ := e.valueLocked(k)
		out = append(out, SystemValue{Key: k, Value: v.(float64)})
	}
	return out
}

// alarm moves an alarm variable's state as the CCU does (measured on ReGaHss, concept §7.1):
// true from false starts an occurrence with the counter at 1; true again counts up; false, by
// whoever, is the acknowledgement; true after that starts anew.
func (e *Engine) alarm(st *varState, value, changed bool, now time.Time, trigger string) {
	a := &st.alarm
	switch {
	case value && changed:
		a.State, a.Count, a.First, a.Trigger = AlarmOncoming, 1, now, trigger
	case value:
		a.State = AlarmOncoming
		a.Count++
		a.Trigger = trigger
	case a.State != AlarmNever:
		a.State = AlarmAcknowledged
	}
}

func limits(def *model.Variable) string {
	switch {
	case def.Min != nil && def.Max != nil:
		return fmt.Sprintf("a number from %v to %v", *def.Min, *def.Max)
	case def.Min != nil:
		return fmt.Sprintf("a number of at least %v", *def.Min)
	default:
		return fmt.Sprintf("a number of at most %v", *def.Max)
	}
}

// ApplyStartValues sets every variable that has a "Wert beim Systemstart" to it, without an
// event: restoring values at a start is never something a program reacts to. The daemon calls
// it once after a boot of the system, before anything else.
func (e *Engine) ApplyStartValues() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, st := range e.vars {
		if st.def.StartValue != nil && !equal(st.value, st.def.StartValue) {
			st.value = st.def.StartValue
			e.dirty = true
		}
	}
}
