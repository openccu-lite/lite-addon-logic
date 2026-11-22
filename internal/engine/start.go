package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// StartTrigger is what a run by the evaluation after a system start names as its trigger.
const StartTrigger = "start"

// EvaluateAtStart evaluates once every active program whose "beim Systemstart prüfen" is on, as
// the CCU evaluates every program when it starts: with its conditions - the first true block
// runs, otherwise SONST. A program with an empty condition runs its first block; a time module
// is true only if the moment lies in its range (an all-day interval has not fired yet: false),
// so a program with only time modules runs its SONST. The evaluation also gives every "bei
// Änderung" line the truth it starts from. The daemon calls it once after a boot of the
// system, when the values of the devices are read and the clock is set - not at a restart or
// an update of the addon. The programs are evaluated in the order of their keys; their writes
// go through the writer one after the other like all writes, so the start is no burst on air.
// Answers the keys of the programs it evaluated.
func (e *Engine) EvaluateAtStart() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys := make([]string, 0, len(e.progs))
	for k, ps := range e.progs {
		if ps.def.Active && ps.def.AtStart && len(ps.problems) == 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		e.execute(k, e.progs[k], cause{by: StartTrigger}, model.RunCheck)
		e.drain()
	}
	return keys
}

// StartUnknown lists the device datapoints the evaluation after a system start would look at
// and has no value for yet (sorted): right after a boot the radio interfaces are still starting
// and the system knows no values - a program evaluated then would find every such line false
// and run its SONST. The daemon waits until the list is empty. A key press has no value to
// wait for.
func (e *Engine) StartUnknown() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := map[string]bool{}
	for _, ps := range e.progs {
		if !ps.def.Active || !ps.def.AtStart || len(ps.problems) > 0 {
			continue
		}
		ps.def.Lines(func(_ int, l *model.Line) {
			if l.Device == nil || strings.HasPrefix(l.Device.Param, "PRESS_") {
				return
			}
			if _, known := e.dps[l.Device.String()]; !known {
				seen[l.Device.String()] = true
			}
		})
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ClockSet says whether the engine takes the system's clock as set: the time modules run.
func (e *Engine) ClockSet() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.clockReady(e.now())
}

// LoopGuard is the loop guard's setting: a program that runs more often than Runs times within
// Window is paused for Pause.
type LoopGuard struct {
	Runs   int
	Window time.Duration
	Pause  time.Duration
}

// DefaultLoopGuard is 20 runs in 10 s, a pause of one minute.
var DefaultLoopGuard = LoopGuard{Runs: 20, Window: 10 * time.Second, Pause: time.Minute}

// SetLoopGuard changes the loop guard; a zero field keeps its default. A pause that runs
// already runs to its end.
func (e *Engine) SetLoopGuard(g LoopGuard) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if g.Runs <= 0 {
		g.Runs = DefaultLoopGuard.Runs
	}
	if g.Window <= 0 {
		g.Window = DefaultLoopGuard.Window
	}
	if g.Pause <= 0 {
		g.Pause = DefaultLoopGuard.Pause
	}
	e.o.LoopRuns, e.o.LoopWindow, e.o.LoopPause = g.Runs, g.Window, g.Pause
}

// LoopGuard answers the loop guard as it is set.
func (e *Engine) LoopGuard() LoopGuard {
	e.mu.Lock()
	defer e.mu.Unlock()
	return LoopGuard{e.o.LoopRuns, e.o.LoopWindow, e.o.LoopPause}
}
