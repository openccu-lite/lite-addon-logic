package engine

import (
	"fmt"
	"sort"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// The run trace: what every run of a program looked at and did, as data - the page says it in
// the reader's language. The engine keeps the last TraceKept entries of all programs together
// (the global log; the daemon writes it to a ring on disk beside the state file), and every
// program its own last RunsKept - so that a program which runs once a day still has its last
// runs when the others have filled the global log many times over. What a program shows is
// both together: its entries of the global log and its own last ones.
//
// The programs' own rings are small on purpose: they are what the entries cost in memory
// beyond the global log - RunsKept entries times the number of programs. (With two hundred
// each, three hundred programs held sixty thousand entries after some hours.)
const (
	RunsKept  = 20
	TraceKept = 5000
)

// Why an entry is there although the program was not evaluated.
const (
	// InfoPaused: the loop guard paused the program at this run.
	InfoPaused = "paused"
	// InfoMissed: a time module was due (Due) and is not made up - the clock was set, or the
	// system stood still.
	InfoMissed = "missed"
	// The source a line listens to reported, and the program was not triggered: its line
	// triggers on a change of its truth and that did not change (InfoNoChange), it triggers
	// on a new value and the value is the same (InfoSameValue), it only checks
	// (InfoCheckOnly), or the value is the first the line sees (InfoFirstValue).
	InfoNoChange   = "no-change"
	InfoSameValue  = "same-value"
	InfoCheckOnly  = "check-only"
	InfoFirstValue = "first-value"
)

// What a step did.
const (
	// StepSent: a device write was handed to the interface (Confirmed or Failed says later
	// what became of it). StepWritten: a variable was written.
	StepSent    = "sent"
	StepWritten = "written"
	// StepScheduled: a delayed activity was put into the registry, due at Due (Ran or Ended
	// says later what became of it).
	StepScheduled = "scheduled"
	// StepCancelled: the block ended Count running delays for its targets before it ran.
	StepCancelled = "cancelled"
	// StepCalled: a webhook or a Wake-on-LAN was handed to its worker (Confirmed or Failed
	// says later what became of it; a webhook's Status and Took the answer's code and how long
	// it took).
	StepCalled = "called"
	// StepSkipped: nothing was done, Reason says why. StepRefused: the variable did not take
	// the value (Reason the error's code, Detail its text).
	StepSkipped = "skipped"
	StepRefused = "refused"
	// StepRun: another program was asked to run. StepSwitched: another program was switched
	// on or off.
	StepRun      = "run"
	StepSwitched = "switched"
	// StepDropped: a delayed activity came due while the addon was stopped and was not made up.
	StepDropped = "dropped"
)

// Why a step was skipped.
const (
	ReasonScript       = "script"
	ReasonSpecial      = "special"
	ReasonUnknownValue = "unknown-value"
	ReasonNoVariable   = "no-variable"
	ReasonNoProgram    = "no-program"
	ReasonNoWriter     = "no-writer"
	// ReasonQueueFull: too many calls were waiting, this one was dropped.
	ReasonQueueFull = "queue-full"
)

// Run is one entry of the run trace: a run of a program - or a delayed activity that came due,
// or the note that a source reported and the program was not triggered.
type Run struct {
	// Seq numbers the entries of all programs in the order they happened.
	Seq     uint64    `json:"seq"`
	Program string    `json:"program"`
	Time    time.Time `json:"time"`
	// Trigger is what caused it: "variable:<key>", "device:<ref>", "program:<key>", "user",
	// "outside", "start"; for time modules "time:<line>:<kind>", with a comma between several
	// that fired at the same instant (the line counted over the program's condition lines
	// from 1; the kind point, begin, repeat or end); "held:<line>" when a line's comparison
	// has held for as long as its "for" asks.
	Trigger string `json:"trigger"`
	// Event is what a variable or a device brought: the value and the one before.
	Event *Event `json:"event,omitempty"`
	// Mode is "then" when the first block ran without a look at the conditions.
	Mode string `json:"mode,omitempty"`
	// Block is the rule that ran: 1 for the first, 2 for the next; 0 for SONST. None is true
	// when the program was evaluated and nothing ran.
	Block int  `json:"block"`
	None  bool `json:"none,omitempty"`
	// Delayed marks the entry of a delayed activity that came due, not a run: nothing is
	// checked again then.
	Delayed bool `json:"delayed,omitempty"`
	// Untriggered marks the note that the program was not evaluated: Info says why. Such
	// notes are kept with the program only, and one after the other with the same reason
	// and trigger are one entry: Count of them since Since.
	Untriggered bool      `json:"untriggered,omitempty"`
	Info        string    `json:"info,omitempty"`
	Count       int       `json:"count,omitempty"`
	Since       time.Time `json:"since,omitzero"`
	// Due is the missed time module's time (InfoMissed).
	Due time.Time `json:"due,omitzero"`
	// Dry marks a simulation: nothing was sent, written or scheduled, and the entry is in no
	// ring - the steps say what would have been done.
	Dry bool `json:"dry,omitempty"`
	// Lines are the condition lines as the run saw them, Steps what it did.
	Lines []LineTrace `json:"lines,omitempty"`
	Steps []Step      `json:"steps,omitempty"`
	// brought is the trigger's value a delayed activity kept from the run that scheduled it.
	brought any
}

// value answers the value the trigger brought (nil: none).
func (r *Run) value() any {
	if r.Event != nil {
		return r.Event.Value
	}
	return r.brought
}

// Event is what a trigger brought along.
type Event struct {
	Value any `json:"value"`
	Prev  any `json:"prev"`
	// First says the value is the first one known: there is none before it.
	First bool `json:"first,omitempty"`
}

// LineTrace is a condition line as a run saw it.
type LineTrace struct {
	// Line counts the program's condition lines from 1, Rule its rules.
	Line int `json:"line"`
	Rule int `json:"rule"`
	// Def is the line as it was then - the program may have been edited since.
	Def *model.Line `json:"def"`
	// Value is what the line compared (nothing for a time module); Unknown says there was no
	// value: such a line is false.
	Value   any  `json:"value,omitempty"`
	Unknown bool `json:"unknown,omitempty"`
	Truth   bool `json:"truth"`
	// Skipped says the line was not looked at: a block before its own ran.
	Skipped bool `json:"skipped,omitempty"`
	// Since says, for a line with "for", since when its comparison holds (zero: it does not).
	Since time.Time `json:"since,omitzero"`
}

// Step is one thing a run did.
type Step struct {
	// Action counts the block's activities from 1; 0 is the block's "end running delays".
	Action int `json:"action"`
	// Def is the activity as it was then (a script's text left out).
	Def  *model.Action `json:"def,omitempty"`
	What string        `json:"what"`
	// Value is what was written.
	Value  any       `json:"value,omitempty"`
	Due    time.Time `json:"due,omitzero"`
	Count  int       `json:"count,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Detail string    `json:"detail,omitempty"`
	// What became of a device write: the interface took it (Confirmed) or refused it (Failed).
	Confirmed time.Time `json:"confirmed,omitzero"`
	Failed    string    `json:"failed,omitempty"`
	// A webhook's answer: the status code (0: none) and how long the call took, in milliseconds.
	// A command's: Status is its exit code, Output the beginning of what it wrote.
	Status int    `json:"status,omitempty"`
	Took   int64  `json:"took,omitempty"`
	Output string `json:"output,omitempty"`
	// What became of a scheduled activity: it ran (Ran), or it was ended before (Ended) - by
	// "user", by "program:<key>" whose block ended the delays for its target, or because its
	// program was "removed".
	Ran     time.Time `json:"ran,omitzero"`
	Ended   time.Time `json:"ended,omitzero"`
	EndedBy string    `json:"ended_by,omitempty"`
}

// cause is why a program is evaluated.
type cause struct {
	by string
	ev *Event
}

// String says the step in English, for the log and the tests.
func (s Step) String() string {
	if s.What == StepCancelled {
		return fmt.Sprintf("cancelled %d pending", s.Count)
	}
	what := describe(s.Def)
	switch s.What {
	case StepSkipped:
		switch s.Reason {
		case ReasonScript:
			return what + ": skipped, scripts are never run"
		case ReasonSpecial:
			return what + ": skipped, the special value " + s.Detail + " is not supported"
		case ReasonUnknownValue:
			return what + ": skipped, the value of " + s.Detail + " is not known"
		case ReasonNoVariable:
			return what + ": skipped, there is no variable " + s.Detail
		case ReasonNoProgram:
			return what + ": there is no such program"
		case ReasonQueueFull:
			return what + ": dropped, too many calls are waiting"
		}
		if s.Def.Kind == model.ActionWebhook || s.Def.Kind == model.ActionWake || s.Def.Kind == model.ActionCommand {
			return what + ": dropped, nothing sends calls"
		}
		if s.Def.Kind == model.ActionChannel || s.Def.Kind == model.ActionDisplay {
			return what + ": dropped, nothing sends device writes"
		}
		return fmt.Sprintf("%s = %v: dropped, nothing sends device writes", what, s.Value)
	case StepRefused:
		return what + ": " + s.Detail
	case StepRun:
		return what + " (" + s.Def.Mode + ")"
	case StepSwitched, StepCalled:
		return what
	case StepSent:
		if s.Def.Kind == model.ActionChannel || s.Def.Kind == model.ActionDisplay {
			return what
		}
	case StepDropped:
		return what + ": dropped, it was due at " + s.Due.Format(time.RFC3339) + " while the addon was stopped"
	}
	if s.What == StepSent && s.Def.OnFor > 0 {
		return fmt.Sprintf("%s = %v for %s", what, s.Value, s.Def.OnFor)
	}
	return fmt.Sprintf("%s = %v", what, s.Value)
}

// Notes says what the entry did in English, one line per step.
func (r Run) Notes() []string {
	var out []string
	switch r.Info {
	case InfoPaused:
		out = append(out, "paused: the program ran too often (the loop guard)")
	case InfoMissed:
		out = append(out, "missed: due at "+r.Due.Format(time.RFC3339)+", the clock was set or the system stood still")
	case "":
	default:
		out = append(out, "not triggered: "+r.Info)
	}
	for _, s := range r.Steps {
		switch {
		case s.What == StepCancelled:
			out = append(out, s.String())
		case r.Delayed:
			out = append(out, "delayed: "+s.String())
		case s.What == StepScheduled:
			out = append(out, fmt.Sprintf("%d: %s in %s", s.Action, describe(s.Def), model.Duration(s.Due.Sub(r.Time))))
		default:
			out = append(out, fmt.Sprintf("%d: %s", s.Action, s.String()))
		}
	}
	return out
}

// copyRun answers an entry that shares nothing the engine still changes.
func copyRun(r *Run) Run {
	c := *r
	c.Steps = append([]Step(nil), r.Steps...)
	return c
}

// record files an entry: with its number, in its program's ring and - unless it only says the
// program was not triggered - in the global log (with the lock held).
func (e *Engine) record(ps *progState, r *Run) {
	e.seq++
	r.Seq = e.seq
	ps.runs = append(ps.runs, r)
	if kept := e.o.RunsKept; len(ps.runs) > kept+kept/2 {
		ps.runs = append([]*Run(nil), ps.runs[len(ps.runs)-kept:]...)
	}
	if r.Untriggered {
		return
	}
	e.trace = append(e.trace, r)
	if len(e.trace) > TraceKept+TraceKept/10 {
		e.trace = append([]*Run(nil), e.trace[len(e.trace)-TraceKept:]...)
	}
}

// untriggered notes that a source reported and the program was not evaluated. One note after
// the other with the same reason and trigger is one entry that counts.
func (e *Engine) untriggered(key string, ps *progState, c cause, info string) {
	now := e.at()
	if n := len(ps.runs); n > 0 {
		if last := ps.runs[n-1]; last.Untriggered && last.Info == info && last.Trigger == c.by {
			last.Count++
			last.Time, last.Event = now, c.ev
			return
		}
	}
	e.record(ps, &Run{Program: key, Time: now, Trigger: c.by, Event: c.ev, None: true, Untriggered: true, Info: info, Count: 1, Since: now})
}

// findStep answers the step that scheduled a delayed activity, if its run is still remembered.
func (e *Engine) findStep(p *Pending) *Step {
	if p.Run == 0 {
		return nil
	}
	i := sort.Search(len(e.trace), func(i int) bool { return e.trace[i].Seq >= p.Run })
	if i == len(e.trace) || e.trace[i].Seq != p.Run {
		return nil
	}
	r := e.trace[i]
	if r.Program != p.Program || p.Step < 0 || p.Step >= len(r.Steps) {
		return nil
	}
	if s := &r.Steps[p.Step]; s.What == StepScheduled && s.Due.Equal(p.Due) {
		return s
	}
	return nil
}

// ended notes on the step that scheduled a delayed activity that it was ended before it ran.
func (e *Engine) ended(p *Pending, by string) {
	if s := e.findStep(p); s != nil {
		s.Ended, s.EndedBy = e.o.Clock.Now(), by
	}
}

// Runs answers a program's remembered entries, oldest first - also those in which nothing ran,
// and the notes that it was not triggered.
func (e *Engine) Runs(key string) []Run {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps := e.progs[key]
	if ps == nil {
		return nil
	}
	runs := e.runsOf(key, ps)
	out := make([]Run, len(runs))
	for i, r := range runs {
		out[i] = copyRun(r)
	}
	return out
}

// runsOf answers what is remembered of a program, oldest first: its entries of the global log
// and its own last ones (with the lock held).
func (e *Engine) runsOf(key string, ps *progState) []*Run {
	own := ps.runs
	if kept := e.o.RunsKept; len(own) > kept {
		own = own[len(own)-kept:]
	}
	out := make([]*Run, 0, len(own))
	for _, r := range e.trace {
		if r.Program != key {
			continue
		}
		// the program's own entries before this one first: among them are those the global
		// log does not have (the notes that it was not triggered) or has dropped
		for len(own) > 0 && own[0].Seq < r.Seq {
			out, own = append(out, own[0]), own[1:]
		}
		if len(own) > 0 && own[0].Seq == r.Seq {
			own = own[1:]
		}
		out = append(out, r)
	}
	return append(out, own...)
}

// Trace answers the global log, oldest first: the entries of all programs with a number above
// after (0: all that are remembered).
func (e *Engine) Trace(after uint64) []Run {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := sort.Search(len(e.trace), func(i int) bool { return e.trace[i].Seq > after })
	out := make([]Run, 0, len(e.trace)-i)
	for _, r := range e.trace[i:] {
		out = append(out, copyRun(r))
	}
	return out
}

// ImportTrace takes the global log a ring on disk kept, oldest first: the engine goes on
// counting after it, and every program that still exists has its entries again. For a fresh
// engine, before anything ran.
func (e *Engine) ImportTrace(runs []Run) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(runs) > TraceKept {
		runs = runs[len(runs)-TraceKept:]
	}
	for i := range runs {
		r := &runs[i]
		if r.Seq <= e.seq {
			continue // not in order: a damaged ring's line
		}
		e.seq = r.Seq
		e.trace = append(e.trace, r)
		if ps := e.progs[r.Program]; ps != nil {
			ps.runs = append(ps.runs, r)
			if !r.None && !r.Delayed {
				ps.lastRun = r
			}
		}
	}
	for _, ps := range e.progs {
		if kept := e.o.RunsKept; len(ps.runs) > kept {
			ps.runs = append([]*Run(nil), ps.runs[len(ps.runs)-kept:]...)
		}
	}
}

// TracePage answers a page of the trace, the newest entry first: at most limit entries with a
// number below before (0: from the newest), and whether there are older ones. With a program's
// key they are that program's - with the notes that it was not triggered - otherwise the
// global log's.
func (e *Engine) TracePage(program string, before uint64, limit int) (runs []Run, more bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	from := e.trace
	if program != "" {
		ps := e.progs[program]
		if ps == nil {
			return []Run{}, false
		}
		from = e.runsOf(program, ps)
	}
	end := len(from)
	if before > 0 {
		end = sort.Search(len(from), func(i int) bool { return from[i].Seq >= before })
	}
	start := max(end-limit, 0)
	runs = make([]Run, 0, end-start)
	for i := end - 1; i >= start; i-- {
		runs = append(runs, copyRun(from[i]))
	}
	return runs, start > 0
}

// Text says the entry in English, a line for the run and one per condition line and step: the
// form the tests and a report from a user's system read.
func (r Run) Text() string {
	head := r.Time.Format("15:04:05") + " " + r.Trigger
	if ev := r.Event; ev != nil {
		if ev.First {
			head += fmt.Sprintf(" (first value %v)", ev.Value)
		} else {
			head += fmt.Sprintf(" (%v -> %v)", ev.Prev, ev.Value)
		}
	}
	switch {
	case r.Untriggered:
		head += ": not triggered, " + r.Info
		if r.Count > 1 {
			head += fmt.Sprintf(" (%d times since %s)", r.Count, r.Since.Format("15:04:05"))
		}
	case r.Info != "":
		head += ": " + r.Notes()[0]
	case r.Delayed:
		head += fmt.Sprintf(": a delayed activity of block %d is due, nothing is checked again", r.Block)
	case r.None:
		head += ": evaluated, no block ran"
	case r.Mode == model.RunThen:
		head += fmt.Sprintf(": block %d without a look at the conditions", r.Block)
	default:
		head += fmt.Sprintf(": block %d", r.Block)
	}
	out := head
	for _, l := range r.Lines {
		out += fmt.Sprintf("\n  line %d (block %d) %s [%s]: ", l.Line, l.Rule, lineWords(l.Def), l.Def.Trigger)
		switch {
		case l.Skipped:
			out += "not looked at, a block before ran"
		case l.Def.Time != nil:
			out += fmt.Sprint(l.Truth)
		case l.Unknown:
			out += "the value is not known: false"
		default:
			out += fmt.Sprintf("%v is %t", l.Value, l.Truth)
			if l.Def.For > 0 && !l.Since.IsZero() {
				out += ", holds since " + l.Since.Format("15:04:05")
			}
		}
	}
	for _, s := range r.Steps {
		out += "\n  "
		if s.Action > 0 {
			out += fmt.Sprintf("%d: ", s.Action)
		}
		if s.What == StepScheduled {
			out += describe(s.Def) + " scheduled for " + s.Due.Format("15:04:05")
		} else {
			out += s.String()
		}
		switch {
		case !s.Ran.IsZero():
			out += ", ran " + s.Ran.Format("15:04:05")
		case !s.Ended.IsZero():
			out += ", ended " + s.Ended.Format("15:04:05") + " by " + s.EndedBy
		case s.Failed != "":
			out += ", failed: " + s.Failed
		case !s.Confirmed.IsZero():
			out += ", confirmed " + s.Confirmed.Format("15:04:05")
		}
	}
	return out
}

// lineWords says a condition line's subject and comparison.
func lineWords(l *model.Line) string {
	var out string
	switch {
	case l.Time != nil:
		return "time module " + l.Time.Kind
	case l.Device != nil:
		out = l.Device.String()
	default:
		out = l.Variable
	}
	if l.Not {
		out += " not"
	}
	switch l.Op {
	case model.OpRange:
		out += fmt.Sprintf(" from %v to less than %v", l.From, l.Below)
	case model.OpOutside:
		out += fmt.Sprintf(" outside %v to %v", l.From, l.To)
	case "":
		out += fmt.Sprintf(" eq %v", l.Value)
	default:
		out += fmt.Sprintf(" %s %v", l.Op, l.Value)
	}
	if l.For > 0 {
		out += " for " + l.For.String()
	}
	return out
}
