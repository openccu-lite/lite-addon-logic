// Package engine is what runs: the system variables with their values and the alarm machinery,
// and the programs that react to them and to the devices' datapoints.
//
// It is a state machine behind one lock. Everything that happens - a datapoint's report, a write
// to a variable, a delay that is due - is an event; events are handled one after the other in
// the order they came, and an event that causes others (a program writes a variable another
// program listens to) puts them at the end of the same queue. Nothing in it blocks or waits:
// device writes are handed to a Writer, time comes from a Clock. With a fake clock and a
// recording writer the engine is deterministic, which is how it is tested.
package engine

import (
	"math/rand/v2"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

// Clock is the one way the engine reads the time and waits: tests give a fake, and the
// scheduler of time modules will use the same one.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f once d has passed, on a goroutine of the clock's; Stop on the answer
	// keeps a call that has not started from happening.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a pending AfterFunc.
type Timer interface {
	Stop() bool
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}

// SystemClock is the real time.
var SystemClock Clock = systemClock{}

// Source says who writes a variable: the limits of a number hold for some writers and not for
// others, as on the CCU.
type Source int

const (
	// FromProgram is a program's activity: a number may leave its limits (an imported counter
	// beyond its maximum keeps counting).
	FromProgram Source = iota
	// FromUser is manual input on the page and FromRPC a write through the interface: both
	// are refused outside the limits.
	FromUser
	FromRPC
	// FromImport sets a value an import brought along: as written, and without an event.
	FromImport
)

// Notification is what the engine tells its listener: a variable was written, an alarm's state
// moved, a program ran or was switched, the delayed activities changed.
type Notification struct {
	// Kind: "variable", "program"; or "pending" - a delayed activity was scheduled, ran or was
	// removed, so the state on disk is behind in a way a power cut would show.
	Kind string
	Key  string
	// Variable: the value after the write, and whether it differs from the one before.
	Value   any
	Changed bool
	// Program: the block that ran (1 = the first rule, 2 = the next, 0 = SONST); or Active set
	// when a program's activity switched the program on or off; or Paused when the loop guard
	// stopped it for a while.
	Block  int
	Active *bool
	Paused bool
}

// Writer sends a write to a device datapoint: it queues the write and returns at once; done is
// called later, from another goroutine, with the interface's answer.
type Writer interface {
	Write(ref model.Ref, value any, done func(error))
}

// SetWriter is a Writer that can write several parameters of one channel in one command (the
// interface's putParamset on VALUES): "ein für" sends the on-time and the value together. With
// a Writer that cannot, the engine writes them one after the other, the on-time first.
type SetWriter interface {
	WriteSet(channel model.Ref, values []model.ParamValue, done func(error))
}

// Caller carries out a webhook, a Wake-on-LAN or a command: it queues the call and returns at once; done
// is called later, from another goroutine, with what became of it. False when its queue is
// full: the call is dropped, and done is never called.
type Caller interface {
	Call(c Call, done func(CallResult)) bool
}

// Call is a webhook, a Wake-on-LAN or a command to carry out: the activity, and what its
// placeholders stand for - all but the secrets, which the engine never sees. For a command
// Args holds, by position, the arguments that are a variable's or a datapoint's value.
type Call struct {
	Action model.Action
	Values map[string]string
	Args   []string
}

// CallResult is what became of a call: a webhook's status code (0 when there was no answer),
// how long it took, and the error - an answer that is not a 2xx is one.
//
// A command's: Exit is its exit code (CommandFailed when it did not run to its end), Output
// and Stderr what it wrote; an exit code that is not 0 is an error.
type CallResult struct {
	Status int
	Took   time.Duration
	Err    error
	Exit   int
	Output string
	Stderr string
}

// Options configure an engine.
type Options struct {
	// Clock is the time (SystemClock when nil).
	Clock Clock
	// Writer sends device writes (nil: a write is logged and dropped).
	Writer Writer
	// Caller carries out webhooks and Wake-on-LAN (nil: such an activity is skipped).
	Caller Caller
	// LoopRuns and LoopWindow are the loop guard: a program that runs more often than LoopRuns
	// times within LoopWindow is paused for LoopPause (20 runs in 10 s, one minute, when zero).
	LoopRuns   int
	LoopWindow time.Duration
	LoopPause  time.Duration
	// RunsKept is how many entries of the run trace every program keeps of its own, beside
	// the global log (RunsKept, twenty, when zero).
	RunsKept int
	// Location is the time zone the time modules' clock times are in (the system's when nil);
	// Position is where the sun rises and sets for the astro ones (SetPosition changes it).
	Location *time.Location
	Position schedule.Position
	// ClockReady says whether the system's clock is set (nil: it is). Until it says yes - or
	// ClockWait has passed, five minutes when zero - no time module fires and none is true:
	// a system without a battery clock starts with a wrong time.
	ClockReady func() bool
	ClockWait  time.Duration
	// ClockTolerance is how late a time module may fire, and how far the clock may be set
	// back without the schedule starting anew (two minutes when zero). Tick is the longest
	// the scheduler sleeps before it looks at the clock again (twenty seconds when zero).
	ClockTolerance time.Duration
	Tick           time.Duration
	// Random answers a whole number from 0 to n, both included: the random delay's dice
	// (math/rand when nil).
	Random func(n int64) int64
	// Notify is told what happened, in order. It is called with the engine locked: it must
	// hand the notification on and return, never call the engine.
	Notify func(Notification)
	// Log receives a line per noteworthy event (nil: silent).
	Log func(format string, args ...any)
}

// Engine is the state machine.
type Engine struct {
	mu  sync.Mutex
	o   Options
	set *model.Set
	// vars holds every variable's value and times, by key.
	vars map[string]*varState
	// sys holds the system values.
	sys map[string]float64
	// queue is what happened and is not handled yet; draining says a loop is at it.
	queue    []event
	draining bool
	// dirty says the state changed since the last Export.
	dirty bool
	// logNew are the logged variables' changes nobody took yet (TakeLog)
	logNew []LogLine
	// acknowledging says the write in hand is an alarm's acknowledge.
	acknowledging bool
	// inputs says which computed values a source ("dp:…", "var:…") feeds; computing says the
	// write in hand is a computed value's own.
	inputs    map[string][]string
	computing bool
	// the device helpers (helpers.go): which helpers a counter and a flag datapoint feed, the
	// counters' maxima, each helper's last counted reading by its leading variable; helping
	// says the write in hand is a helper's own
	helpers  []*model.HelperGroup
	counters map[string][]*model.HelperGroup
	flags    map[string][]*model.HelperGroup
	maxima   map[string]float64
	last     map[string]*counted
	helping  bool
	// handle is what an event is given to: the programs' side, or a test's recorder.
	handle func(ev event)

	// progs holds the programs with their runtime state, by key; index says which lines listen
	// to a source ("var:<key>", "dp:<ref>").
	progs map[string]*progState
	index map[string][]lineRef
	// dps holds the last known value of the device datapoints, by reference.
	dps map[string]any
	// reports is when each datapoint last reported, and how often it does (by reference)
	reports map[string]*report
	// pending are the delayed activities that are not due yet.
	pending []*pending
	nextID  int
	// sim is set while a simulation looks at the programs: the values it tries
	sim *simulation
	// trace is the global log of runs, oldest first; seq the number of its newest entry
	trace []*Run
	seq   uint64

	// the scheduler of the time modules (timers.go)
	mods  map[*model.Line]*schedule.Module
	sched scheduler
	// evalAt is the time of the event being handled: a time module is asked for that moment
	evalAt time.Time
}

// event is one thing that happened.
type event struct {
	// variable or device: the source's key ("var:<key>" or "dp:<ref>")
	source  string
	value   any
	prev    any
	changed bool
	// first says the value is the first one known for the source: nothing to compare it with
	first bool
	at    time.Time
	// run asks for a program to be run: its key and the mode
	run, mode string
	// by is what caused the run
	by string
	// held says a line's comparison has held for as long as its "for" asks: the program and
	// the line
	held *lineRef
	// fires are the time modules that fire at this instant
	fires []fire
}

// New makes an engine without programs or variables.
func New(o Options) *Engine {
	if o.Clock == nil {
		o.Clock = SystemClock
	}
	if o.LoopRuns <= 0 {
		o.LoopRuns = 20
	}
	if o.LoopWindow <= 0 {
		o.LoopWindow = 10 * time.Second
	}
	if o.LoopPause <= 0 {
		o.LoopPause = time.Minute
	}
	if o.RunsKept <= 0 {
		o.RunsKept = RunsKept
	}
	if o.ClockWait <= 0 {
		o.ClockWait = 5 * time.Minute
	}
	if o.ClockTolerance <= 0 {
		o.ClockTolerance = 2 * time.Minute
	}
	if o.Tick <= 0 {
		o.Tick = 20 * time.Second
	}
	if o.Random == nil {
		o.Random = func(n int64) int64 { return rand.Int64N(n + 1) }
	}
	e := &Engine{o: o, set: model.NewSet(), vars: map[string]*varState{}, sys: map[string]float64{},
		progs: map[string]*progState{}, index: map[string][]lineRef{}, dps: map[string]any{}, reports: map[string]*report{}, mods: map[*model.Line]*schedule.Module{},
		maxima: map[string]float64{}, last: map[string]*counted{}}
	e.sched.clockOK = o.ClockReady == nil
	e.handle = e.react
	return e
}

func (e *Engine) logf(format string, args ...any) {
	if e.o.Log != nil {
		e.o.Log(format, args...)
	}
}

func (e *Engine) notify(n Notification) {
	if e.o.Notify != nil {
		e.o.Notify(n)
	}
}

// post puts an event at the end of the queue; drain handles the queue until it is empty. Both
// are called with the lock held.
func (e *Engine) post(ev event) { e.queue = append(e.queue, ev) }

func (e *Engine) drain() {
	if e.draining {
		return
	}
	e.draining = true
	for len(e.queue) > 0 {
		ev := e.queue[0]
		e.queue = e.queue[1:]
		e.evalAt = ev.at
		if ev.source != "" {
			e.count(ev)
			e.recompute(ev.source, false)
		}
		if e.handle != nil {
			e.handle(ev)
		}
	}
	e.draining = false
	e.evalAt = time.Time{}
}

// at is the moment the engine looks at the world: the time of the event it is handling - for a
// time module that fires, the instant it was due - and otherwise now.
func (e *Engine) at() time.Time {
	if !e.evalAt.IsZero() {
		return e.evalAt
	}
	return e.o.Clock.Now()
}

// equal compares two values of the kinds the model has: bools, numbers, strings.
func equal(a, b any) bool { return a == b }
