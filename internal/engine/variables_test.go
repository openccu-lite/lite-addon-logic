package engine

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

var t0 = time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

func f(x float64) *float64 { return &x }

func defs() map[string]*model.Variable {
	return map[string]*model.Variable{
		"regen":  {Key: "regen", Name: "Regen", Type: model.VarBool, Visible: true, Logged: true},
		"modus":  {Key: "modus", Name: "Modus", Type: model.VarList, Entries: []string{"Sommer", "Übergang", "Winter"}, Visible: true},
		"temp":   {Key: "temp", Name: "Temp", Type: model.VarNumber, Min: f(0), Max: f(100), Visible: true},
		"offen":  {Key: "offen", Name: "Offen", Type: model.VarNumber, Visible: true},
		"ab5":    {Key: "ab5", Name: "Ab 5", Type: model.VarNumber, Min: f(5), Visible: true},
		"bis9":   {Key: "bis9", Name: "Bis 9", Type: model.VarNumber, Max: f(9), Visible: true},
		"text":   {Key: "text", Name: "Text", Type: model.VarString, Visible: true},
		"alarm":  {Key: "alarm", Name: "Alarm", Type: model.VarAlarm, Visible: true},
		"alarm2": {Key: "alarm2", Name: "Alarm 2", Type: model.VarAlarm, Visible: true},
		"da":     {Key: "da", Name: "Anwesenheit", Type: model.VarBool, Visible: true, StartValue: true},
	}
}

type rig struct {
	t     *testing.T
	e     *Engine
	clock *FakeClock
	notes []Notification
	logs  []string
}

func newRig(t *testing.T) *rig {
	r := &rig{t: t, clock: NewFakeClock(t0)}
	r.e = New(Options{Clock: r.clock, Notify: func(n Notification) { r.notes = append(r.notes, n) }, Log: func(format string, args ...any) { r.logs = append(r.logs, format) }})
	r.e.SetVariables(defs())
	return r
}

func (r *rig) set(key string, v any, src Source) {
	r.t.Helper()
	if err := r.e.SetVariable(key, v, src); err != nil {
		r.t.Fatalf("set %s = %v: %v", key, v, err)
	}
}

func (r *rig) refused(key string, v any, src Source, code string) {
	r.t.Helper()
	err := r.e.SetVariable(key, v, src)
	var we *WriteError
	if !errors.As(err, &we) || we.Code != code {
		r.t.Fatalf("set %s = %v by %d: %v, want %s", key, v, src, err, code)
	}
}

func (r *rig) value(key string) any {
	r.t.Helper()
	v, ok := r.e.Value(key)
	if !ok {
		r.t.Fatalf("no value for %s", key)
	}
	return v
}

func TestDefaultsAndTypes(t *testing.T) {
	r := newRig(t)
	for key, want := range map[string]any{"regen": false, "modus": "Sommer", "temp": 0.0, "ab5": 5.0, "text": "", "alarm": false, SystemAlarmMessages: 0.0} {
		if got := r.value(key); got != want {
			t.Errorf("%s: %v, want %v", key, got, want)
		}
	}
	if _, ok := r.e.Value("nothing"); ok {
		t.Error("a value for a variable that is not there")
	}
	if _, ok := r.e.Variable("nothing"); ok {
		t.Error("a variable that is not there")
	}
	r.set("regen", true, FromUser)
	r.set("modus", "Winter", FromRPC)
	r.set("temp", 21.5, FromUser)
	r.set("temp", 7, FromProgram) // an int is a number
	r.set("text", "hallo", FromProgram)
	if r.value("regen") != true || r.value("modus") != "Winter" || r.value("temp") != 7.0 || r.value("text") != "hallo" {
		t.Errorf("values after the writes")
	}
	for _, c := range []struct {
		key  string
		v    any
		code string
	}{{"regen", "ja", "wrong-type"}, {"regen", 1.0, "wrong-type"}, {"modus", "Herbst", "wrong-type"}, {"modus", 1.0, "wrong-type"}, {"temp", "warm", "wrong-type"},
		{"text", true, "wrong-type"}, {"alarm", "x", "wrong-type"}, {"nothing", true, "unknown-variable"}, {SystemAlarmMessages, 3.0, "read-only"}} {
		for _, src := range []Source{FromProgram, FromUser, FromRPC, FromImport} {
			r.refused(c.key, c.v, src, c.code)
		}
	}
	infos := r.e.Variables()
	if len(infos) != 10 || infos[0].Def.Key != "ab5" || infos[9].Def.Key != "text" {
		t.Errorf("the list: %d, first %s", len(infos), infos[0].Def.Key)
	}
	in, _ := r.e.Variable("temp")
	if in.Value != 7.0 || in.Alarm != nil || !in.Time.Equal(t0) || !in.Changed.Equal(t0) {
		t.Errorf("info: %+v", in)
	}
}

// OQ-23: what a program writes is stored as written, as on the CCU; a person and another
// program are held to the limits; an entry a list does not have is refused for everyone.
func TestLimitsPerWriter(t *testing.T) {
	r := newRig(t)
	for _, v := range []float64{150, -5, 100.5} {
		r.set("temp", v, FromProgram)
		if r.value("temp") != v {
			t.Errorf("a program's %v was not stored", v)
		}
		r.set("temp", v, FromImport)
	}
	r.set("temp", 50, FromUser)
	for _, src := range []Source{FromUser, FromRPC} {
		for _, v := range []float64{150, -5, 100.5, -0.001} {
			r.refused("temp", v, src, "out-of-range")
		}
		r.set("temp", 0, src)
		r.set("temp", 100, src)
		r.refused("ab5", 4, src, "out-of-range")
		r.set("ab5", 1e9, src)
		r.refused("bis9", 10, src, "out-of-range")
		r.set("bis9", -1e9, src)
		r.set("offen", 1e300, src)
	}
	for _, c := range []struct{ key, want string }{{"temp", "from 0 to 100"}, {"ab5", "at least 5"}, {"bis9", "at most 9"}} {
		err := r.e.SetVariable(c.key, 1e6*map[string]float64{"temp": 1, "ab5": -1, "bis9": 1}[c.key], FromUser)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.key, err)
		}
	}
}

// Every write is an update, a different value a change; an import's value is no event.
func TestEventsAndTimes(t *testing.T) {
	r := newRig(t)
	var events []event
	r.e.handle = func(ev event) { events = append(events, ev) }
	r.set("text", "x", FromProgram)
	r.clock.Advance(time.Minute)
	r.set("text", "x", FromProgram)
	r.clock.Advance(time.Minute)
	r.set("text", "y", FromRPC)
	if len(events) != 3 || !events[0].changed || events[1].changed || !events[2].changed || events[2].prev != "x" || events[2].source != "var:text" {
		t.Fatalf("events: %+v", events)
	}
	want := []Notification{{Kind: "variable", Key: "text", Value: "x", Changed: true}, {Kind: "variable", Key: "text", Value: "x"}, {Kind: "variable", Key: "text", Value: "y", Changed: true}}
	if !reflect.DeepEqual(r.notes, want) {
		t.Fatalf("notifications: %+v", r.notes)
	}
	in, _ := r.e.Variable("text")
	if !in.Time.Equal(t0.Add(2*time.Minute)) || !in.Changed.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("times: %+v", in)
	}
	r.set("text", "y", FromUser)
	r.clock.Advance(time.Minute)
	r.set("text", "y", FromUser)
	if in, _ = r.e.Variable("text"); !in.Time.Equal(t0.Add(3*time.Minute)) || !in.Changed.Equal(t0.Add(2*time.Minute)) {
		t.Errorf("an update moved the change time: %+v", in)
	}
	n := len(events)
	r.set("text", "imported", FromImport)
	if len(events) != n || len(r.notes) != 5 || r.value("text") != "imported" {
		t.Errorf("an import's value was an event")
	}
	// an event caused by an event's handling is handled after it, in order
	events = nil
	r.e.handle = func(ev event) {
		events = append(events, ev)
		if ev.source == "var:regen" && ev.value == true {
			_ = r.e.write("text", "regen!", FromProgram, "variable:regen")
			_ = r.e.write("modus", "Winter", FromProgram, "variable:regen")
		}
	}
	r.set("regen", true, FromUser)
	if len(events) != 3 || events[0].source != "var:regen" || events[1].source != "var:text" || events[2].source != "var:modus" {
		t.Fatalf("order: %+v", events)
	}
	// without a clock and without listeners an engine still works
	bare := New(Options{})
	bare.SetVariables(defs())
	if err := bare.SetVariable("regen", true, FromUser); err != nil {
		t.Fatal(err)
	}
	if in, _ := bare.Variable("regen"); time.Since(in.Time) > time.Minute {
		t.Errorf("the system clock: %v", in.Time)
	}
	timer := SystemClock.AfterFunc(time.Hour, func() {})
	if !timer.Stop() {
		t.Error("the system clock's timer")
	}
}

// The alarm machinery, row by row as measured on the CCU (concept §7.1).
func TestAlarmStateMachine(t *testing.T) {
	r := newRig(t)
	alarm := func() AlarmState {
		in, _ := r.e.Variable("alarm")
		return *in.Alarm
	}
	last := func() Notification { return r.notes[len(r.notes)-1] }
	if a := alarm(); a.State != AlarmNever || a.Count != 0 || !a.First.IsZero() {
		t.Fatalf("fresh: %+v", a)
	}
	// false while never triggered: nothing - no event, and it stays "never"
	r.set("alarm", false, FromProgram)
	if a := alarm(); a.State != AlarmNever || len(r.notes) != 0 {
		t.Fatalf("false on a fresh alarm: %+v %+v", a, r.notes)
	}
	// true (was false): oncoming, counter 1, first time now; a change
	r.clock.Advance(time.Minute)
	if err := r.e.write("alarm", true, FromProgram, "variable:regen"); err != nil {
		t.Fatal(err)
	}
	first := t0.Add(time.Minute)
	if a := alarm(); a.State != AlarmOncoming || a.Count != 1 || !a.First.Equal(first) || a.Trigger != "variable:regen" || !last().Changed {
		t.Fatalf("true: %+v", a)
	}
	if r.value(SystemAlarmMessages) != 1.0 {
		t.Errorf("alarm messages: %v", r.value(SystemAlarmMessages))
	}
	// true again, unacknowledged: counter up, first time unchanged; an update only
	r.clock.Advance(time.Minute)
	r.set("alarm", true, FromRPC)
	if a := alarm(); a.State != AlarmOncoming || a.Count != 2 || !a.First.Equal(first) || a.Trigger != "outside" || last().Changed {
		t.Fatalf("true again: %+v %+v", a, last())
	}
	// false, by anything: that is the acknowledgement; counter and first time kept; a change
	r.clock.Advance(time.Minute)
	r.set("alarm", false, FromProgram)
	if a := alarm(); a.State != AlarmAcknowledged || a.Count != 2 || !a.First.Equal(first) || !last().Changed || r.value("alarm") != false {
		t.Fatalf("false: %+v", a)
	}
	if r.value(SystemAlarmMessages) != 0.0 {
		t.Errorf("alarm messages: %v", r.value(SystemAlarmMessages))
	}
	// false again, by a write: nothing, not even an update (the CCU makes no event of it)
	n := len(r.notes)
	r.set("alarm", false, FromRPC)
	if a := alarm(); a.State != AlarmAcknowledged || a.Count != 2 || len(r.notes) != n {
		t.Fatalf("false while false: %+v %+v", a, r.notes[n:])
	}
	// acknowledge while already false: still an update event
	if err := r.e.Acknowledge("alarm"); err != nil {
		t.Fatal(err)
	}
	if a := alarm(); a.State != AlarmAcknowledged || a.Count != 2 || len(r.notes) != n+1 || last().Changed {
		t.Fatalf("acknowledge while false: %+v", a)
	}
	// true after an acknowledge: the counter restarts
	r.clock.Advance(time.Minute)
	r.set("alarm", true, FromUser)
	if a := alarm(); a.State != AlarmOncoming || a.Count != 1 || !a.First.Equal(t0.Add(4*time.Minute)) || a.Trigger != "user" {
		t.Fatalf("true after the acknowledge: %+v", a)
	}
	// the list's Bestätigen writes false
	if err := r.e.Acknowledge("alarm"); err != nil {
		t.Fatal(err)
	}
	if a := alarm(); a.State != AlarmAcknowledged || r.value("alarm") != false || !last().Changed {
		t.Fatalf("acknowledged: %+v", a)
	}
	if err := r.e.Acknowledge("regen"); err == nil || err.(*WriteError).Code != "wrong-type" {
		t.Errorf("acknowledging a bool: %v", err)
	}
	if err := r.e.Acknowledge("nothing"); err == nil || err.(*WriteError).Code != "unknown-variable" {
		t.Errorf("acknowledging nothing: %v", err)
	}
	// the list of alarm messages: the oncoming ones, the oldest occurrence first
	r.set("alarm2", true, FromUser)
	r.clock.Advance(time.Minute)
	r.set("alarm", true, FromUser)
	msgs := r.e.AlarmMessages()
	if len(msgs) != 2 || msgs[0].Def.Key != "alarm2" || msgs[1].Def.Key != "alarm" || r.value(SystemAlarmMessages) != 2.0 {
		t.Fatalf("alarm messages: %+v", msgs)
	}
	r.set("alarm2", false, FromUser)
	r.set("alarm2", true, FromUser) // the same time as alarm's: sorted by key then
	if msgs = r.e.AlarmMessages(); len(msgs) != 2 || msgs[0].Def.Key != "alarm" {
		t.Fatalf("alarm messages at the same time: %+v", msgs)
	}
}

// The system values: read-only numbers beside the variables, with events when they move.
func TestSystemValues(t *testing.T) {
	r := newRig(t)
	r.set("alarm", true, FromUser)
	if r.value(SystemAlarmMessages) != 1.0 || r.value(SystemServiceMessages) != 0.0 || r.value(SystemDutyCycle) != 0.0 {
		t.Fatalf("values: %v", r.e.SystemValues())
	}
	for _, key := range []string{SystemAlarmMessages, SystemServiceMessages, SystemDutyCycle} {
		for _, src := range []Source{FromProgram, FromUser, FromRPC, FromImport} {
			r.refused(key, 5.0, src, "read-only")
		}
	}
	var events []string
	r.e.handle = func(ev event) { events = append(events, ev.source) }
	r.notes = nil
	r.set("alarm2", true, FromUser)
	if r.value(SystemAlarmMessages) != 2.0 || !reflect.DeepEqual(events, []string{"var:alarm2", "var:" + SystemAlarmMessages}) {
		t.Fatalf("after a second alarm: %v, %v", r.value(SystemAlarmMessages), events)
	}
	if len(r.notes) != 2 || r.notes[1] != (Notification{Kind: "variable", Key: SystemAlarmMessages, Value: 2.0, Changed: true}) {
		t.Errorf("notifications: %+v", r.notes)
	}
	r.set("alarm2", true, FromUser) // the number stays: no event of it
	if len(events) != 3 {
		t.Errorf("an unchanged number was an event: %v", events)
	}
	if err := r.e.Acknowledge("alarm"); err != nil || r.value(SystemAlarmMessages) != 1.0 {
		t.Errorf("after an acknowledge: %v %v", r.value(SystemAlarmMessages), err)
	}
	events = nil
	r.e.SetSystemValue(SystemServiceMessages, 3)
	r.e.SetSystemValue(SystemServiceMessages, 3)
	r.e.SetSystemValue(SystemDutyCycle, 40)
	r.e.SetSystemValue(SystemAlarmMessages, 40) // the engine's own
	r.e.SetSystemValue("regen", 1)
	want := []SystemValue{{SystemAlarmMessages, 1}, {SystemServiceMessages, 3}, {SystemDutyCycle, 40}, {model.SystemSunAzimuth, 0}, {model.SystemSunElevation, 0}}
	if !reflect.DeepEqual(r.e.SystemValues(), want) || !reflect.DeepEqual(events, []string{"var:" + SystemServiceMessages, "var:" + SystemDutyCycle}) {
		t.Errorf("fed values: %v, %v", r.e.SystemValues(), events)
	}
	if !IsSystemValue(SystemDutyCycle) || IsSystemValue("regen") {
		t.Error("IsSystemValue")
	}
	// a variable file under a system value's key is left out; removing an alarm moves the number
	d := defs()
	d[SystemDutyCycle] = &model.Variable{Key: SystemDutyCycle, Name: "X", Type: model.VarString}
	delete(d, "alarm2")
	r.e.SetVariables(d)
	if r.value(SystemDutyCycle) != 40.0 || r.value(SystemAlarmMessages) != 0.0 || len(r.e.Variables()) != 9 {
		t.Errorf("after a reload: %v", r.e.SystemValues())
	}
}

// A logged variable's changes are handed out as lines, once: the engine keeps none itself.
func TestLoggedVariable(t *testing.T) {
	r := newRig(t)
	r.set("regen", true, FromUser)
	r.set("regen", true, FromUser) // no change, no entry
	r.clock.Advance(time.Second)
	r.set("regen", false, FromUser)
	r.set("text", "x", FromUser) // not logged
	log := r.e.TakeLog()
	if len(log) != 2 || log[0].Key != "regen" || log[0].Value != true || log[1].Value != false || !log[1].Time.Equal(t0.Add(time.Second)) || log[1].Clear {
		t.Fatalf("log: %+v", log)
	}
	if again := r.e.TakeLog(); len(again) != 0 {
		t.Fatalf("taken twice: %+v", again)
	}
	// the file's form: a line begins with its key, a false is said
	b, err := json.Marshal(log[1])
	if err != nil || string(b) != `{"k":"regen","t":"2026-11-22T12:00:01Z","v":false}` {
		t.Errorf("a line: %s %v", b, err)
	}
	if b, _ := json.Marshal(LogLine{Key: "regen", Clear: true}); string(b) != `{"k":"regen","clear":true}` {
		t.Errorf("a clear: %s", b)
	}
	// with nobody taking them the oldest go
	for i := 0; i < logBuffer+10; i++ {
		r.set("regen", i%2 == 0, FromUser)
	}
	log = r.e.TakeLog()
	if len(log) < logBuffer/2 || len(log) > logBuffer || log[len(log)-1].Value != false {
		t.Fatalf("%d lines wait", len(log))
	}
	// a variable that is no longer logged, of another type or gone: its log is void
	d := defs()
	d["regen"] = &model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool}
	r.e.SetVariables(d)
	if log = r.e.TakeLog(); len(log) != 1 || log[0] != (LogLine{Key: "regen", Clear: true}) {
		t.Fatalf("no longer logged: %+v", log)
	}
	d = defs()
	d["regen"] = &model.Variable{Key: "regen", Name: "Regen", Type: model.VarString, Logged: true}
	r.e.SetVariables(d)
	r.e.SetVariables(d)
	if log = r.e.TakeLog(); len(log) != 0 {
		t.Fatalf("logged again is nothing to say: %+v", log)
	}
	d = defs()
	r.e.SetVariables(d)
	if log = r.e.TakeLog(); len(log) != 1 || !log[0].Clear {
		t.Fatalf("another type: %+v", log)
	}
	delete(d, "regen")
	delete(d, "text")
	r.e.SetVariables(d)
	if log = r.e.TakeLog(); len(log) != 1 || log[0] != (LogLine{Key: "regen", Clear: true}) {
		t.Fatalf("gone: %+v", log)
	}
}

// A changed set of definitions: what stays keeps its value where it still fits.
func TestSetVariablesKeepsValues(t *testing.T) {
	r := newRig(t)
	r.set("regen", true, FromUser)
	r.set("modus", "Winter", FromUser)
	r.set("temp", 42, FromUser)
	r.set("alarm", true, FromUser)
	r.set("text", "x", FromUser)
	d := defs()
	delete(d, "text")
	d["neu"] = &model.Variable{Key: "neu", Name: "Neu", Type: model.VarList, Entries: []string{"a", "b"}}
	d["modus"] = &model.Variable{Key: "modus", Name: "Modus", Type: model.VarList, Entries: []string{"Sommer", "Kalt"}}
	d["temp"] = &model.Variable{Key: "temp", Name: "Temp", Type: model.VarString}
	d["regen"] = &model.Variable{Key: "regen", Name: "Regen neu", Type: model.VarBool}
	d["alarm"] = &model.Variable{Key: "alarm", Name: "Alarm", Type: model.VarBool}
	// a renamed entry is followed when the caller says so
	r.e.RenameEntry("modus", "Winter", "Kalt")
	r.e.RenameEntry("modus", "Sommer", "Warm") // not the value: nothing
	r.e.RenameEntry("nothing", "a", "b")
	r.e.SetVariables(d)
	if r.value("regen") != true || r.value("modus") != "Kalt" || r.value("temp") != "" || r.value("neu") != "a" || r.value("alarm") != true {
		t.Errorf("values: regen %v modus %v temp %v neu %v", r.value("regen"), r.value("modus"), r.value("temp"), r.value("neu"))
	}
	if _, ok := r.e.Value("text"); ok {
		t.Error("the removed variable still has a value")
	}
	if in, _ := r.e.Variable("alarm"); in.Alarm != nil {
		t.Error("a bool with an alarm state")
	}
	if in, _ := r.e.Variable("regen"); in.Def.Name != "Regen neu" {
		t.Error("the new definition is not in")
	}
	// an entry that is gone without a rename: the first entry
	d2 := map[string]*model.Variable{"modus": {Key: "modus", Name: "Modus", Type: model.VarList, Entries: []string{"x", "y"}}}
	r.e.SetVariables(d2)
	if r.value("modus") != "x" || len(r.logs) == 0 {
		t.Errorf("an entry that is gone: %v", r.value("modus"))
	}
}

func TestStartValues(t *testing.T) {
	r := newRig(t)
	r.set("da", false, FromUser)
	n := len(r.notes)
	r.e.ApplyStartValues()
	if r.value("da") != true || len(r.notes) != n || r.value("regen") != false {
		t.Errorf("start values: da %v, %d notifications", r.value("da"), len(r.notes)-n)
	}
}

func TestStateExportImport(t *testing.T) {
	r := newRig(t)
	if _, dirty := r.e.Export(); !dirty {
		t.Error("a fresh engine with variables is not dirty")
	}
	if _, dirty := r.e.Export(); dirty {
		t.Error("dirty without a change")
	}
	r.set("regen", true, FromUser)
	r.set("modus", "Winter", FromUser)
	r.set("temp", 150, FromProgram)
	r.set("text", "hallo", FromUser)
	r.clock.Advance(time.Minute)
	r.set("alarm", true, FromUser)
	r.set("alarm", true, FromUser)
	s, dirty := r.e.Export()
	if !dirty {
		t.Fatal("not dirty after writes")
	}
	b, err := MarshalState(s)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalState(b)
	if err != nil {
		t.Fatal(err)
	}
	// a state with things the variables no longer take
	back.Variables["gone"] = VariableState{Value: true}
	back.Variables["text"] = VariableState{Value: 5.0}
	back.Variables["da"] = VariableState{Value: true, Alarm: &AlarmState{State: 1}, Log: []LogEntry{{Value: true}}}
	r2 := newRig(t)
	r2.e.Import(back)
	for _, k := range []string{"regen", "modus", "temp", "alarm"} {
		if r2.value(k) != r.value(k) {
			t.Errorf("%s: %v, want %v", k, r2.value(k), r.value(k))
		}
	}
	if r2.value("text") != "" || len(r2.logs) != 1 {
		t.Errorf("a value of another type was taken: %v", r2.value("text"))
	}
	a, _ := r2.e.Variable("alarm")
	if a.Alarm.State != AlarmOncoming || a.Alarm.Count != 2 || !a.Alarm.First.Equal(t0.Add(time.Minute)) || !a.Time.Equal(t0.Add(time.Minute)) {
		t.Errorf("the alarm did not survive: %+v", a)
	}
	if r2.value(SystemAlarmMessages) != 1.0 {
		t.Error("the alarm message count is not computed from the state")
	}
	if strings.Contains(string(b), `"log"`) {
		t.Errorf("the state holds a log: %s", b)
	}
	if in, _ := r2.e.Variable("da"); in.Alarm != nil || len(r2.e.TakeLog()) != 0 {
		t.Error("a bool took an alarm state or a log from the file")
	}
	if len(r2.notes) != 0 {
		t.Error("an import of the state was an event")
	}
	if _, err := UnmarshalState([]byte("{")); err == nil {
		t.Error("a torn state")
	}
}

func TestFakeClock(t *testing.T) {
	c := NewFakeClock(t0)
	var ran []string
	at := func(name string) func() {
		return func() { ran = append(ran, name+"@"+c.Now().Sub(t0).String()) }
	}
	c.AfterFunc(2*time.Second, at("b"))
	c.AfterFunc(time.Second, at("a"))
	stopped := c.AfterFunc(time.Second, at("never"))
	c.AfterFunc(2*time.Second, at("c"))
	c.AfterFunc(time.Second, func() {
		ran = append(ran, "arms")
		c.AfterFunc(500*time.Millisecond, at("d"))
		c.AfterFunc(time.Hour, at("late"))
	})
	if !stopped.Stop() || stopped.Stop() {
		t.Error("Stop")
	}
	if c.Pending() != 4 {
		t.Errorf("pending %d", c.Pending())
	}
	c.Advance(500 * time.Millisecond)
	if len(ran) != 0 {
		t.Errorf("too early: %v", ran)
	}
	c.Advance(2 * time.Second)
	want := []string{"a@1s", "arms", "d@1.5s", "b@2s", "c@2s"}
	if !reflect.DeepEqual(ran, want) {
		t.Errorf("ran %v, want %v", ran, want)
	}
	if !c.Now().Equal(t0.Add(2500*time.Millisecond)) || c.Pending() != 1 {
		t.Errorf("now %v, pending %d", c.Now(), c.Pending())
	}
	// a jump: nothing runs until the next Advance, which runs what is due by then
	c.Set(t0.Add(2 * time.Hour))
	if len(ran) != 5 {
		t.Error("Set ran a timer")
	}
	c.Advance(0)
	if len(ran) != 6 || ran[5] != "late@2h0m0s" {
		t.Errorf("after the jump: %v", ran)
	}
	if done := c.AfterFunc(0, func() {}); true {
		c.Advance(0)
		if done.Stop() {
			t.Error("a timer that ran can be stopped")
		}
	}
}
