package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

// A line's truth as the program last saw it.
const (
	truthUnknown int8 = -1
	truthFalse   int8 = 0
	truthTrue    int8 = 1
)

// progState is a program with what the engine remembers about it.
type progState struct {
	def *model.Program
	// problems are broken references: such a program does not run
	problems []string
	// lines are the condition lines, flattened in file order
	lines []*lineState
	runs  []*Run
	// lastRun is the last entry in which a block ran
	lastRun *Run
	// recent are the times of the last runs, for the loop guard; pausedUntil is its pause
	recent      []time.Time
	pausedUntil time.Time
	// failures counts the device writes the system refused; lastFailure is the last one
	failures    int
	lastFailure string
	// heldAt is the instant of the last run a line's "for" caused
	heldAt time.Time
}

// lineState is one condition line with the truth it had when the program last looked at it.
// A "bei Änderung" line triggers when its truth now differs from that - and the CCU only
// remembers the truth of lines it evaluated: a line in a SONST WENN that was not reached has no
// remembered truth and triggers on every write (Q-48).
type lineState struct {
	def   *model.Line
	rule  int
	truth int8
	// unseen says the truth was set while the line's value was not known yet
	unseen bool
	// a time module line: when it last fired, and how
	lastFire time.Time
	lastKind string
	// a line with "for": since when its comparison holds (zero: it does not), the timer that
	// waits for it to have held long enough, and hint - the since a stored state brought
	// along, taken the first time the comparison is seen to hold after a start
	since time.Time
	hint  time.Time
	hold  Timer
}

type lineRef struct {
	prog string
	line int
}

// ProgramInfo is a program's runtime state as the page and the interface see it.
type ProgramInfo struct {
	Key string `json:"key"`
	// LastRun is the last run in which a block ran (nil: never).
	LastRun *Run `json:"last_run,omitempty"`
	// PausedUntil is set while the loop guard pauses the program.
	PausedUntil time.Time `json:"paused_until,omitzero"`
	// NextTime is when a time module next triggers the program (nil: none does, or the program
	// is not active). NoPosition says it has an astro time module and no position is set: that
	// one never fires. NoClock says the system's clock is not set yet: no time module fires.
	NextTime   *time.Time `json:"next_time,omitempty"`
	NoPosition bool       `json:"no_position,omitempty"`
	NoClock    bool       `json:"no_clock,omitempty"`
	Problems   []string   `json:"problems,omitempty"`
	// Failures counts the device writes the system refused; LastFailure is the last one.
	Failures    int    `json:"failures,omitempty"`
	LastFailure string `json:"last_failure,omitempty"`
}

// SetPrograms replaces the programs. A program whose file did not change keeps what the engine
// remembers about it; a new or changed one starts with the truth its lines have now, without
// running anything: saving a program, or switching it on, never runs it.
func (e *Engine) SetPrograms(programs map[string]*model.Program) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.set.Programs = programs
	next := make(map[string]*progState, len(programs))
	for k, def := range programs {
		old := e.progs[k]
		ps := &progState{def: def}
		if old != nil {
			ps.runs, ps.lastRun, ps.recent, ps.pausedUntil, ps.failures, ps.lastFailure = old.runs, old.lastRun, old.recent, old.pausedUntil, old.failures, old.lastFailure
		}
		def.Lines(func(rule int, l *model.Line) {
			ps.lines = append(ps.lines, &lineState{def: l, rule: rule, truth: truthUnknown})
		})
		same := old != nil && old.def.Active == def.Active && sameConditions(old.def, def) && len(old.lines) == len(ps.lines)
		if same {
			for i, l := range old.lines {
				ps.lines[i].truth, ps.lines[i].unseen = l.truth, l.unseen
				ps.lines[i].lastFire, ps.lines[i].lastKind = l.lastFire, l.lastKind
				ps.lines[i].since, ps.lines[i].hint = l.since, l.hint
			}
		}
		if old != nil {
			for _, l := range old.lines {
				e.stopHold(l)
			}
		}
		next[k] = ps
		if !same {
			e.settle(ps)
		}
	}
	for k, old := range e.progs {
		if next[k] == nil {
			for _, l := range old.lines {
				e.stopHold(l)
			}
		}
	}
	e.progs = next
	for k, ps := range next {
		for i, l := range ps.lines {
			e.track(k, i, l)
		}
	}
	e.compile()
	e.reindex()
	// a delayed activity of a program that is gone goes with it
	kept := e.pending[:0]
	for _, p := range e.pending {
		if next[p.Program] != nil {
			kept = append(kept, p)
		} else {
			p.timer.Stop()
			e.ended(&p.Pending, "removed")
		}
	}
	e.pending = kept
	e.dirty = true
	e.arm()
}

// sameConditions says whether two versions of a program have the same condition lines: then the
// remembered truths still mean the same.
func sameConditions(a, b *model.Program) bool { return fingerprint(a) == fingerprint(b) }

// reindex rebuilds the problems and the index of who listens to what (with the lock held).
func (e *Engine) reindex() {
	e.index = map[string][]lineRef{}
	keys := make([]string, 0, len(e.progs))
	for k := range e.progs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ps := e.progs[k]
		ps.problems = e.set.ProgramProblems(ps.def)
		for i, l := range ps.lines {
			if l.def.Time != nil && e.mods[l.def] == nil {
				_, err := schedule.Compile(l.def.Time, e.o.Location, e.o.Position)
				ps.problems = append(ps.problems, fmt.Sprintf("time module: %v", err))
			}
			if src := lineSource(l.def); src != "" {
				e.index[src] = append(e.index[src], lineRef{prog: k, line: i})
			}
		}
	}
}

func lineSource(l *model.Line) string {
	switch {
	case l.Device != nil:
		return "dp:" + l.Device.String()
	case l.Variable != "":
		return "var:" + l.Variable
	}
	return ""
}

// settle gives every line of a program the truth it has now, without running anything.
func (e *Engine) settle(ps *progState) {
	for _, l := range ps.lines {
		truth, known := e.truth(l)
		l.truth, l.unseen = b2t(truth), !known
	}
}

func b2t(b bool) int8 {
	if b {
		return truthTrue
	}
	return truthFalse
}

// Refs lists the device datapoints the programs read, sorted: what the daemon asks the system
// for and follows.
func (e *Engine) Refs() []model.Ref {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := map[model.Ref]bool{}
	for _, ps := range e.progs {
		ps.def.Lines(func(_ int, l *model.Line) {
			if l.Device != nil {
				seen[*l.Device] = true
			}
		})
		ps.def.Actions(func(_ int, a *model.Action) {
			if a.FromDevice != nil {
				seen[*a.FromDevice] = true
			}
			if a.Command != nil {
				for _, r := range a.Command.Devices() {
					seen[r] = true
				}
			}
		})
	}
	for _, st := range e.vars {
		if st.def.Computed != nil {
			for _, in := range st.def.Computed.Inputs {
				if in.Device != nil {
					seen[*in.Device] = true
				}
			}
		}
		if st.def.Helper != nil {
			for _, r := range st.def.Helper.Refs() {
				seen[r] = true
			}
		}
	}
	out := make([]model.Ref, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// Program answers a program's runtime state.
func (e *Engine) Program(key string) (ProgramInfo, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps := e.progs[key]
	if ps == nil {
		return ProgramInfo{}, false
	}
	return e.programInfo(key, ps), true
}

// ProgramStates answers every program's runtime state, sorted by key.
func (e *Engine) ProgramStates() []ProgramInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]ProgramInfo, 0, len(e.progs))
	for k, ps := range e.progs {
		out = append(out, e.programInfo(k, ps))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (e *Engine) programInfo(key string, ps *progState) ProgramInfo {
	in := ProgramInfo{Key: key, Problems: ps.problems, Failures: ps.failures, LastFailure: ps.lastFailure}
	e.timeInfo(ps, &in)
	if ps.pausedUntil.After(e.o.Clock.Now()) {
		in.PausedUntil = ps.pausedUntil
	}
	if ps.lastRun != nil {
		r := copyRun(ps.lastRun)
		in.LastRun = &r
	}
	return in
}

// DeviceEvent is a datapoint's report from the system: every report is an update, a different
// value a change. The first value known for a datapoint is neither for a "bei Änderung" line:
// it only tells the line where it stands (after a start a restored or first reported value must
// not look like something that happened).
func (e *Engine) DeviceEvent(ref model.Ref, value any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := ref.String()
	prev, known := e.dps[key]
	if n, ok := value.(int); ok {
		value = float64(n)
	}
	e.dps[key] = value
	e.reported(key)
	e.post(event{source: "dp:" + key, value: value, prev: prev, changed: known && !looseEqual(prev, value), first: !known, at: e.o.Clock.Now()})
	e.drain()
}

// SeedDevice tells the engine a datapoint's value without it being an event: what the system's
// state store says at the start, or after a resync of the event stream.
func (e *Engine) SeedDevice(ref model.Ref, value any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if n, ok := value.(int); ok {
		value = float64(n)
	}
	key := ref.String()
	e.dps[key] = value
	e.seedHelpers("dp:"+key, value)
	e.recompute("dp:"+key, true)
	for _, lr := range e.index["dp:"+key] {
		l := e.progs[lr.prog].lines[lr.line]
		e.track(lr.prog, lr.line, l)
		if l.unseen {
			truth, _ := e.truth(l)
			l.truth, l.unseen = b2t(truth), false
		}
	}
}

// DeviceValue answers the last known value of a datapoint.
func (e *Engine) DeviceValue(ref model.Ref) (any, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	v, ok := e.dps[ref.String()]
	return v, ok
}

// report is what the engine remembers of a datapoint's reports: the last one, and the first and
// the number since the addon started - their quotient is how often the device reports.
type report struct {
	first, last time.Time
	n           int
}

func (e *Engine) reported(key string) {
	now := e.o.Clock.Now()
	r := e.reports[key]
	if r == nil {
		r = &report{first: now}
		e.reports[key] = r
	}
	r.last = now
	r.n++
}

// DeviceState is a datapoint as the engine knows it.
type DeviceState struct {
	Value any `json:"value"`
	// Time is the last report since the addon started (zero: none yet - the value is the
	// system's stored one).
	Time time.Time `json:"time,omitzero"`
	// EverySeconds is how often the datapoint reported on average, once it reported three
	// times; 0 before.
	EverySeconds int `json:"every_seconds,omitempty"`
}

// DeviceStates answers every datapoint the engine has a value for, by its reference.
func (e *Engine) DeviceStates() map[string]DeviceState {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]DeviceState, len(e.dps))
	for key, v := range e.dps {
		st := DeviceState{Value: v}
		if r := e.reports[key]; r != nil {
			st.Time = r.last
			if r.n >= 3 {
				st.EverySeconds = int(r.last.Sub(r.first).Seconds()) / (r.n - 1)
			}
		}
		out[key] = st
	}
	return out
}

// RunProgram runs a program for other software (the interface): see RunProgramBy.
func (e *Engine) RunProgram(key, mode string) error { return e.RunProgramBy(key, mode, "outside") }

// LastRun answers the block of the program's last run: 1 for the first rule, 2 for the next, 0
// for SONST - and 0 when it never ran.
func (e *Engine) LastRun(key string) int {
	if in, ok := e.Program(key); ok && in.LastRun != nil {
		return in.LastRun.Block
	}
	return 0
}

// RunProgramBy runs a program by hand: mode model.RunCheck evaluates it with its conditions,
// model.RunThen runs its first block without looking at them - the CCU's "Start", which runs
// an inactive program too. by says who asked: "user" for the page, "outside" for other software.
func (e *Engine) RunProgramBy(key, mode, by string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps := e.progs[key]
	if ps == nil {
		return writeErr("unknown-program", "there is no program %s", key)
	}
	if mode != model.RunCheck && mode != model.RunThen {
		return writeErr("invalid", "a program is run with mode %s or %s", model.RunCheck, model.RunThen)
	}
	if len(ps.problems) > 0 {
		return writeErr("invalid", "%s cannot run: %s", key, ps.problems[0])
	}
	e.post(event{run: key, mode: mode, by: by, at: e.o.Clock.Now()})
	e.drain()
	return nil
}

// react handles one event: a requested run, or a report that programs may listen to.
func (e *Engine) react(ev event) {
	if ev.fires != nil {
		e.reactTime(ev)
		return
	}
	if ev.run != "" {
		if ps := e.progs[ev.run]; ps != nil {
			e.execute(ev.run, ps, cause{by: ev.by}, ev.mode)
		}
		return
	}
	if ev.held != nil {
		e.reactHeld(*ev.held)
		return
	}
	// the lines with "for" on this source first learn whether their comparison holds
	for _, lr := range e.index[ev.source] {
		e.track(lr.prog, lr.line, e.progs[lr.prog].lines[lr.line])
	}
	// which programs does the event trigger? One evaluation per program, however many of its
	// lines the event touches (the CCU runs a program once per write for two lines on one
	// variable, too). A program that listens and is not triggered gets a note why.
	c := cause{by: "variable:" + ev.source[len("var:"):], ev: &Event{Value: ev.value, Prev: ev.prev, First: ev.first}}
	if ev.source[:3] == "dp:" {
		c.by = "device:" + ev.source[len("dp:"):]
	}
	var listening, triggered []string
	why := map[string]string{}
	seen := map[string]bool{}
	for _, lr := range e.index[ev.source] {
		ps := e.progs[lr.prog]
		l := ps.lines[lr.line]
		if !ps.def.Active || len(ps.problems) > 0 {
			continue
		}
		if _, ok := why[lr.prog]; !ok {
			listening = append(listening, lr.prog)
			why[lr.prog] = InfoCheckOnly
		}
		fires, info, learn := e.decide(l, ev)
		if learn {
			// the first value the line sees: it learns where it stands, nothing happened
			truth, _ := e.truth(l)
			l.truth, l.unseen = b2t(truth), false
		}
		if infoRank[info] > infoRank[why[lr.prog]] {
			why[lr.prog] = info
		}
		if fires && !seen[lr.prog] {
			seen[lr.prog] = true
			triggered = append(triggered, lr.prog)
		}
	}
	for _, k := range listening {
		if !seen[k] {
			e.untriggered(k, e.progs[k], c, why[k])
		}
	}
	for _, k := range triggered {
		e.execute(k, e.progs[k], c, model.RunCheck)
	}
}

// decide says whether an event on a line's source triggers the line's program, why not, and
// whether the line only learns where it stands (the first value it sees). It changes nothing.
func (e *Engine) decide(l *lineState, ev event) (fires bool, info string, learn bool) {
	switch l.def.Trigger {
	case model.TriggerUpdate:
		return true, "", false
	case model.TriggerValue:
		return ev.changed, InfoSameValue, false
	case model.TriggerChange:
		if l.unseen || (ev.first && l.truth != truthUnknown) {
			return false, InfoFirstValue, true
		}
		truth, _ := e.truth(l)
		return l.truth != b2t(truth), InfoNoChange, false
	}
	return false, InfoCheckOnly, false
}

// infoRank orders the reasons a program was not triggered: the one that says most wins when
// several of its lines listen to the source.
var infoRank = map[string]int{InfoCheckOnly: 0, InfoFirstValue: 1, InfoSameValue: 2, InfoNoChange: 3}

// execute runs a program once: with its conditions (the chain top-down, the first true block,
// else SONST) or, in mode then, its first block without looking.
func (e *Engine) execute(key string, ps *progState, c cause, mode string) {
	now := e.at()
	if ps.pausedUntil.After(now) {
		return
	}
	// the loop guard: a program that writes what it listens to would run forever
	cut := now.Add(-e.o.LoopWindow)
	recent := ps.recent[:0]
	for _, t := range ps.recent {
		if t.After(cut) {
			recent = append(recent, t)
		}
	}
	ps.recent = append(recent, now)
	if len(ps.recent) > e.o.LoopRuns {
		ps.pausedUntil = now.Add(e.o.LoopPause)
		ps.recent = nil
		e.logf("program %s ran more than %d times in %v: paused until %s (does it write what it listens to?)", key, e.o.LoopRuns, e.o.LoopWindow, ps.pausedUntil.Format("15:04:05"))
		e.record(ps, &Run{Program: key, Time: now, Trigger: c.by, Event: c.ev, None: true, Info: InfoPaused})
		e.notify(Notification{Kind: "program", Key: key, Paused: true})
		return
	}
	run := &Run{Program: key, Time: now, Trigger: c.by, Event: c.ev, None: true}
	block := -1
	if mode == model.RunThen {
		run.Mode = model.RunThen
		if len(ps.def.Rules) > 0 {
			block = 0
		}
	} else {
		block, run.Lines = e.evaluate(ps)
		// every line that was looked at remembers its truth; the CCU does not look at the
		// lines of the blocks after the one that ran, and forgets what it knew about them
		for i, lt := range run.Lines {
			l := ps.lines[i]
			if lt.Skipped {
				l.truth, l.unseen = truthUnknown, false
			} else {
				l.truth, l.unseen = b2t(lt.Truth), lt.Unknown
			}
		}
	}
	// the entry is filed before the block runs: what the block schedules names it
	e.record(ps, run)
	if block >= 0 {
		r := &ps.def.Rules[block]
		run.None = false
		ps.lastRun = run
		run.Block = block + 1
		if r.Else {
			run.Block = 0
		}
		e.runBlock(run, r, block)
	}
	e.dirty = true
	if !run.None {
		e.notify(Notification{Kind: "program", Key: key, Block: run.Block})
	}
}

// evaluate looks at a program's conditions and changes nothing: the block that would run (-1:
// none) and every condition line as it stands. Every line of a condition is looked at - no
// short cut; a condition without groups is true; the lines of the blocks after the one that
// runs are not looked at.
func (e *Engine) evaluate(ps *progState) (int, []LineTrace) {
	block := -1
	lines := make([]LineTrace, 0, len(ps.lines))
	for ri := range ps.def.Rules {
		r := &ps.def.Rules[ri]
		result := true
		if c := r.When; c != nil && len(c.Groups) > 0 {
			result = c.Op == model.All
			for gi := range c.Groups {
				g := &c.Groups[gi]
				group := g.Op == model.All
				for range g.Lines {
					i := len(lines)
					ls := ps.lines[i]
					lt := LineTrace{Line: i + 1, Rule: ri + 1, Def: ls.def, Skipped: block >= 0}
					if !lt.Skipped {
						lt.Value, _ = e.lineValue(ls.def)
						known := false
						lt.Truth, known = e.truth(ls)
						lt.Unknown = !known
						if ls.def.For > 0 {
							lt.Since = ls.since
						}
					}
					lines = append(lines, lt)
					if g.Op == model.All {
						group = group && lt.Truth
					} else {
						group = group || lt.Truth
					}
				}
				if c.Op == model.All {
					result = result && group
				} else {
					result = result || group
				}
			}
		}
		if block < 0 && (r.Else || result) {
			block = ri
		}
	}
	return block, lines
}

// lineValue answers the value a line compares, and whether there is one (a time module has
// none).
func (e *Engine) lineValue(l *model.Line) (any, bool) {
	switch {
	case l.Time != nil:
		return nil, false
	case l.Device != nil:
		return e.source("dp:" + l.Device.String())
	}
	return e.source("var:" + l.Variable)
}

// source answers the value of a variable ("var:<key>") or a datapoint ("dp:<ref>") - during a
// simulation the value that is tried instead of the real one.
func (e *Engine) source(src string) (any, bool) {
	if e.sim != nil {
		if v, ok := e.sim.values[src]; ok {
			return v, true
		}
	}
	if strings.HasPrefix(src, "dp:") {
		v, ok := e.dps[src[len("dp:"):]]
		return v, ok
	}
	return e.valueLocked(src[len("var:"):])
}

// truth answers a line's truth with the values known now, and whether its value is known at
// all. A line on an unknown value is false - also with "not". A time module line is asked for
// the moment the engine looks at (the instant of the event): true inside its range or exactly
// at its point, false while the system's clock is not set. A line with "for" is true once its
// comparison has held that long.
func (e *Engine) truth(ls *lineState) (truth, known bool) {
	truth, known = e.compares(ls)
	if ls.def.For > 0 && truth {
		truth = !ls.since.IsZero() && !e.at().Before(ls.since.Add(ls.def.For.Std()))
	}
	return truth, known
}

// compares answers whether a line's comparison holds now (its "for" aside).
func (e *Engine) compares(ls *lineState) (truth, known bool) {
	l := ls.def
	if l.Time != nil {
		if e.sim != nil && e.sim.fired == ls {
			return true, true // the simulation tries the moment it fires
		}
		return e.timeTruth(ls, e.at()), true
	}
	v, known := e.lineValue(l)
	if !known {
		return false, false
	}
	t := compare(l, v)
	if l.Not {
		t = !t
	}
	return t, true
}

// track keeps a line with "for" up to date (with the lock held): since when its comparison
// holds, and the timer for the moment it has held long enough. Called whenever the line's
// value may have changed.
func (e *Engine) track(key string, i int, ls *lineState) {
	if ls.def.For <= 0 {
		return
	}
	holds, known := e.compares(ls)
	if !holds {
		ls.since = time.Time{}
		if known {
			ls.hint = time.Time{}
		}
		e.stopHold(ls)
		return
	}
	if ls.since.IsZero() {
		ls.since = e.at()
		if !ls.hint.IsZero() && ls.hint.Before(ls.since) {
			ls.since = ls.hint // it held before the start already, the stored state says
		}
		ls.hint = time.Time{}
		e.dirty = true
	}
	if ls.hold != nil {
		return
	}
	due := ls.since.Add(ls.def.For.Std())
	wait := due.Sub(e.o.Clock.Now())
	if wait < 0 {
		// long enough already (a start after the time was up): now is the moment
		wait, due = 0, e.o.Clock.Now()
	}
	ref := &lineRef{prog: key, line: i}
	var timer Timer
	timer = e.o.Clock.AfterFunc(wait, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if ps := e.progs[key]; ps == nil || i >= len(ps.lines) || ps.lines[i] != ls || ls.hold != timer {
			return
		}
		ls.hold = nil
		e.post(event{held: ref, at: due})
		e.drain()
	})
	ls.hold = timer
}

func (e *Engine) stopHold(ls *lineState) {
	if ls.hold != nil {
		ls.hold.Stop()
		ls.hold = nil
	}
}

// reactHeld handles the moment a line's comparison has held for as long as its "for" asks: the
// line is true from now on, and that triggers its program - unless the line only checks.
func (e *Engine) reactHeld(lr lineRef) {
	ps := e.progs[lr.prog]
	l := ps.lines[lr.line]
	if !ps.def.Active || len(ps.problems) > 0 || l.def.Trigger == model.TriggerCheck {
		return
	}
	if truth, _ := e.truth(l); !truth {
		return
	}
	// several lines of one program that come true at the same instant are one evaluation
	if at := e.at(); ps.heldAt.Equal(at) {
		return
	} else {
		ps.heldAt = at
	}
	e.execute(lr.prog, ps, cause{by: fmt.Sprintf("held:%d", lr.line+1)}, model.RunCheck)
}

func asNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// looseEqual compares two values; a bool and a number compare as numbers (a device reports a
// state as true or as 1, depending on its datapoint's type).
func looseEqual(a, b any) bool {
	if a == b {
		return true
	}
	_, as := a.(string)
	_, bs := b.(string)
	if as || bs {
		return false
	}
	x, ok1 := asNumber(a)
	y, ok2 := asNumber(b)
	return ok1 && ok2 && x == y
}

// compare applies a line's comparison to a value. The ranges are the CCU's as measured: "von a
// bis kleiner b" is a <= v < b (and never true when a is above b); "outside" is v < a or v > b.
func compare(l *model.Line, v any) bool {
	switch l.Op {
	case model.OpEq, "":
		return looseEqual(v, l.Value)
	case model.OpNe:
		return !looseEqual(v, l.Value)
	}
	x, ok := asNumber(v)
	if !ok {
		return false
	}
	num := func(a any) float64 { f, _ := asNumber(a); return f }
	switch l.Op {
	case model.OpGt:
		return x > num(l.Value)
	case model.OpGe:
		return x >= num(l.Value)
	case model.OpLt:
		return x < num(l.Value)
	case model.OpLe:
		return x <= num(l.Value)
	case model.OpRange:
		return x >= num(l.From) && x < num(l.Below)
	case model.OpOutside:
		return x < num(l.From) || x > num(l.To)
	}
	return false
}
