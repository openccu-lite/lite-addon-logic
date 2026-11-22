package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

// The scheduler: it makes the time modules fire. What a module does to the calendar is package
// schedule's; here is the clock.
//
// One timer waits for the earliest instant at which a time module of an active program fires -
// but never longer than Options.Tick, because the system's clock can be set while the timer
// waits, and a timer does not notice. Each time it wakes it reads the clock and handles what
// came due since the last time ("cursor": everything up to it is done):
//
//   - All modules that fire at one instant are one event, and each program they trigger is
//     evaluated once for it - also when two of its own lines fire together (the CCU ran such a
//     program twice, Q-7). The program is evaluated for that instant, not for the moment the
//     timer happened to wake: a point is true, a range that ends is false.
//   - What is late by more than Options.ClockTolerance is not made up: the clock was set
//     forward, or the system stood still. The program's runs get a note. A range that is
//     under way is simply true when the program is next evaluated.
//   - A clock set back by more than the tolerance means the time was wrong before: the
//     schedule follows the new time, and what is due on it fires, also if it fired on the
//     wrong one. A smaller step back never fires anything twice.
//   - Nothing is ever replayed after a start: the cursor begins at the first reading of the
//     clock. Until the clock is set (Options.ClockReady) no module fires and none is true.
type scheduler struct {
	timer Timer
	// armedAt and armedFor: when the timer was armed and for how long - the clock should
	// show their sum when it wakes
	armedAt  time.Time
	armedFor time.Duration
	cursor   time.Time
	started  bool
	// clockOK says the clock is set; clockAsked is when that was first asked
	clockOK    bool
	clockAsked time.Time
}

// fire is one time module line firing.
type fire struct {
	prog string
	line int
	kind schedule.EdgeKind
}

// now reads the clock as a wall clock: without Go's monotonic reading, which would hide that
// the clock was set.
func (e *Engine) now() time.Time { return e.o.Clock.Now().Round(0) }

// Now is the engine's time, in the time zone its time modules use.
func (e *Engine) Now() time.Time { return e.local(e.now()) }

// local is a time on the clock the time modules and the helpers' days go by.
func (e *Engine) local(t time.Time) time.Time {
	loc := e.o.Location
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc)
}

// compile makes the modules of every time line (with the lock held); a line whose module
// cannot be made has none, and reindex names the reason as a problem of its program.
func (e *Engine) compile() {
	e.mods = map[*model.Line]*schedule.Module{}
	for _, ps := range e.progs {
		for _, l := range ps.lines {
			if l.def.Time == nil {
				continue
			}
			if mod, err := schedule.Compile(l.def.Time, e.o.Location, e.o.Position); err == nil {
				e.mods[l.def] = mod
			}
		}
	}
}

// SetPosition says where the system stands: the astro time modules follow.
func (e *Engine) SetPosition(pos schedule.Position) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.o.Position = pos
	e.compile()
	e.reindex()
	e.arm()
}

// Position answers where the system stands.
func (e *Engine) Position() schedule.Position {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.o.Position
}

// clockReady says whether the clock is set, asking the option until it says yes or the wait is
// over.
func (e *Engine) clockReady(now time.Time) bool {
	s := &e.sched
	if s.clockOK {
		return true
	}
	if s.clockAsked.IsZero() {
		s.clockAsked = now
	}
	if e.o.ClockReady() {
		s.clockOK = true
	} else if waited := now.Sub(s.clockAsked); waited >= e.o.ClockWait || waited < 0 {
		s.clockOK = true
		e.logf("the system's clock is not synchronised after %v: the time modules run on it as it is (%s)", e.o.ClockWait, now.Format(time.RFC3339))
	}
	return s.clockOK
}

// triggering lists the time lines that can trigger: every one in an active program without
// problems, whatever its mode says. On the CCU a time module evaluates its program at each of
// its instants also when its line is "nur prüfen" (measured on the real ReGaHss, which lists a
// program under each of its time modules in its own trigger table): the mode of a time line
// changes nothing, and that is reproduced.
func (e *Engine) triggering(f func(key string, ps *progState, i int, mod *schedule.Module)) {
	for k, ps := range e.progs {
		if !ps.def.Active || len(ps.problems) > 0 {
			continue
		}
		for i, l := range ps.lines {
			if mod := e.mods[l.def]; mod != nil {
				f(k, ps, i, mod)
			}
		}
	}
}

// arm sets the timer for the next instant something fires (with the lock held). Without a
// time line that can trigger, and without a device helper that keeps a day's amount, there is
// no timer at all.
func (e *Engine) arm() {
	s := &e.sched
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	any := e.dayHelpers()
	e.triggering(func(string, *progState, int, *schedule.Module) { any = true })
	if !any {
		return
	}
	now := e.now()
	wait := e.o.Tick
	if e.clockReady(now) {
		if b, ok := e.nextRoll(now); ok && b.Sub(now) < wait {
			wait = b.Sub(now)
		}
		if !s.started {
			s.cursor, s.started = now, true
		}
		e.triggering(func(_ string, _ *progState, _ int, mod *schedule.Module) {
			if edge, ok := mod.Next(s.cursor); ok && edge.At.Sub(now) < wait {
				wait = max(edge.At.Sub(now), 0)
			}
		})
	}
	s.armedAt, s.armedFor = now, wait
	s.timer = e.o.Clock.AfterFunc(wait, e.wake)
}

// wake is the timer's function.
func (e *Engine) wake() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sched.timer = nil
	e.runTimers()
	e.drain()
	e.arm()
}

// runTimers handles what came due since the cursor (with the lock held).
func (e *Engine) runTimers() {
	s := &e.sched
	now := e.now()
	if !e.clockReady(now) {
		return
	}
	e.rollHelpers(now)
	if !s.started {
		s.cursor, s.started = now, true
		return
	}
	if back := s.armedAt.Add(s.armedFor).Sub(now); back > e.o.ClockTolerance {
		e.logf("the system's clock was set back by %v: the time modules follow the new time", back.Round(time.Second))
		s.cursor = now
		return
	}
	lo := s.cursor
	if cut := now.Add(-e.o.ClockTolerance); cut.After(lo) {
		// too late for what was due before the cut: say so on the programs it concerns
		missed := map[string]time.Time{}
		e.triggering(func(k string, _ *progState, _ int, mod *schedule.Module) {
			if edge, ok := mod.Next(lo); ok && !edge.At.After(cut) && (missed[k].IsZero() || edge.At.Before(missed[k])) {
				missed[k] = edge.At
			}
		})
		for k, first := range missed {
			e.logf("program %s: its time module was due at %s and is not made up (the clock was set, or the system stood still)", k, first.Format(time.RFC3339))
			e.record(e.progs[k], &Run{Program: k, Time: now, Trigger: "time", None: true, Info: InfoMissed, Due: first})
		}
		lo = cut
	}
	type due struct {
		at time.Time
		fire
	}
	var all []due
	e.triggering(func(k string, _ *progState, i int, mod *schedule.Module) {
		for edge, ok := mod.Next(lo); ok && !edge.At.After(now); edge, ok = mod.Next(edge.At) {
			all = append(all, due{edge.At, fire{k, i, edge.Kind}})
		}
	})
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if !a.at.Equal(b.at) {
			return a.at.Before(b.at)
		}
		if a.prog != b.prog {
			return a.prog < b.prog
		}
		return a.line < b.line
	})
	for i := 0; i < len(all); {
		ev := event{at: all[i].at}
		for ; i < len(all) && all[i].at.Equal(ev.at); i++ {
			ev.fires = append(ev.fires, all[i].fire)
		}
		e.post(ev)
	}
	if now.After(s.cursor) {
		s.cursor = now
	}
}

// reactTime handles the time modules that fire at one instant: each program among them is
// evaluated once, whatever number of its lines fire.
func (e *Engine) reactTime(ev event) {
	var order []string
	by := map[string][]string{}
	for _, f := range ev.fires {
		ps := e.progs[f.prog]
		l := ps.lines[f.line]
		l.lastFire, l.lastKind = ev.at, f.kind.String()
		if by[f.prog] == nil {
			order = append(order, f.prog)
		}
		by[f.prog] = append(by[f.prog], fmt.Sprintf("%d:%s", f.line+1, f.kind))
	}
	for _, k := range order {
		e.execute(k, e.progs[k], cause{by: "time:" + strings.Join(by[k], ",")}, model.RunCheck)
	}
}

// timeInfo fills in what a program's time modules have to say (with the lock held).
func (e *Engine) timeInfo(ps *progState, in *ProgramInfo) {
	has := false
	for _, l := range ps.lines {
		if mod := e.mods[l.def]; mod != nil {
			has = true
			in.NoPosition = in.NoPosition || mod.NeedsPosition()
		}
	}
	if !has {
		return
	}
	in.NoClock = !e.sched.clockOK
	if !ps.def.Active || len(ps.problems) > 0 || in.NoClock {
		return
	}
	from := e.now()
	if e.sched.started && e.sched.cursor.After(from) {
		from = e.sched.cursor
	}
	for _, l := range ps.lines {
		mod := e.mods[l.def]
		if mod == nil {
			continue
		}
		if edge, ok := mod.Next(from); ok && (in.NextTime == nil || edge.At.Before(*in.NextTime)) {
			at := edge.At
			in.NextTime = &at
		}
	}
}

// timeTruth is a time module line's truth at an instant: the calendar's (schedule.Active),
// false while the system's clock is not set. An interval that runs all day is true all day -
// but, as on the CCU (measured), not before it fired for the first time since the start: a
// program evaluated at the start, or by another trigger before the interval's first firing,
// finds it false.
func (e *Engine) timeTruth(l *lineState, at time.Time) bool {
	mod := e.mods[l.def]
	if mod == nil || !e.sched.clockOK {
		return false
	}
	if mod.AllDayInterval() && l.lastFire.IsZero() {
		return false
	}
	return mod.Active(at)
}

// TimeLine is one time module line of a program, as the page shows it.
type TimeLine struct {
	// Line counts the program's condition lines from 1.
	Line int `json:"line"`
	// Triggers says the line triggers the program: always, a time module does so whatever its
	// line's mode is (as on the CCU).
	Triggers bool `json:"triggers"`
	// Active is the module's truth now.
	Active bool `json:"active"`
	// Next are the next times it fires - whether or not the program is active (OpenCCU#889: a
	// module that was never armed still knows its next time); Last is when it last fired.
	Next     []TimeEdge `json:"next"`
	Last     *TimeEdge  `json:"last,omitempty"`
	Position bool       `json:"no_position,omitempty"`
}

// TimeEdge is one instant a time module fires at: a point, or a range's begin, repeat or end.
type TimeEdge struct {
	At   time.Time `json:"at"`
	Kind string    `json:"kind"`
}

// TimeLines answers a program's time module lines with their next n fires each.
func (e *Engine) TimeLines(key string, n int) []TimeLine {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps := e.progs[key]
	if ps == nil {
		return nil
	}
	now := e.now()
	out := []TimeLine{}
	for i, l := range ps.lines {
		mod := e.mods[l.def]
		if mod == nil {
			continue
		}
		tl := TimeLine{Line: i + 1, Triggers: true, Active: e.timeTruth(l, now), Next: []TimeEdge{}, Position: mod.NeedsPosition()}
		for _, edge := range mod.Upcoming(now, n) {
			tl.Next = append(tl.Next, TimeEdge{edge.At, edge.Kind.String()})
		}
		if !l.lastFire.IsZero() {
			tl.Last = &TimeEdge{l.lastFire, l.lastKind}
		}
		out = append(out, tl)
	}
	return out
}
