package engine

import (
	"math"
	"sort"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Device helpers: number variables that follow a device's counter - the running total, and the
// amounts of today and of yesterday - as the CCU's WebUI did with a hidden program per device.
// What a report of the counter adds is the family's arithmetic (model.HelperKind.Step); here is
// what happens around it:
//
//   - A report is counted before the programs look at it, and what it adds is written like any
//     variable: an update for the programs on it, a change when it differs.
//   - What a smaller reading means, the device says with a flag (an HmIP device's …_OVERFLOW,
//     a HomeMatic one's BOOT) that reaches the engine as an event of its own. When the flag
//     arrives within lateFlag after the reading it belongs to, the reading is counted again
//     with it.
//   - The day's boundary (07:00 for rain and sunshine, 07:30 for the HomeMatic rain sensor,
//     midnight for the others) moves today's amount to yesterday's. A boundary that passed
//     while the addon was stopped is made up at the start - yesterday's amount then holds
//     everything since the boundary before - but only on a clock that is set.
//   - A value the system's store has is never counted (it is no report), but it tells a helper
//     that has not seen its device yet where the counter stands.

// lateFlag is how long after a reading its flag may arrive.
const lateFlag = 5 * time.Second

// counted is the last reading a helper counted, kept until its flag can no longer arrive.
type counted struct {
	at     time.Time
	source string
	before model.HelperState
	dev    float64
	flag   bool
	step   model.HelperStep
}

// HelperInfo is a helper as the page sees it.
type HelperInfo struct {
	Kind    string `json:"kind"`
	Channel string `json:"channel"`
	// Variables are the helper's variables' keys by role.
	Variables map[string]string `json:"variables"`
	Off       bool              `json:"off,omitempty"`
	// State is what the helper remembers: the device's last counted reading.
	State model.HelperState `json:"state"`
	// Maximum is the counter's maximum from the device's description (nil: not known yet).
	Maximum *float64 `json:"maximum,omitempty"`
}

// indexHelpers notes which helpers each datapoint feeds (with the lock held).
func (e *Engine) indexHelpers() {
	e.helpers = e.set.Helpers()
	e.counters, e.flags = map[string][]*model.HelperGroup{}, map[string][]*model.HelperGroup{}
	for _, g := range e.helpers {
		ch := model.ChannelRef(g.Channel)
		for _, p := range g.Kind.Counters {
			src := "dp:" + model.Ref{Interface: ch.Interface, Address: ch.Address, Param: p}.String()
			e.counters[src] = append(e.counters[src], g)
		}
		if g.Kind.Flag != "" {
			src := "dp:" + model.Ref{Interface: ch.Interface, Address: ch.Address, Param: g.Kind.Flag}.String()
			e.flags[src] = append(e.flags[src], g)
		}
	}
	for key := range e.last {
		if st := e.vars[key]; st == nil || st.def.Helper == nil {
			delete(e.last, key)
		}
	}
}

// flagOf reads a helper's flag datapoint: false when the kind has none or nothing is known.
func (e *Engine) flagOf(g *model.HelperGroup) bool {
	if g.Kind.Flag == "" {
		return false
	}
	ch := model.ChannelRef(g.Channel)
	v, ok := e.dps[model.Ref{Interface: ch.Interface, Address: ch.Address, Param: g.Kind.Flag}.String()]
	return ok && truthy(v)
}

func truthy(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	f, ok := v.(float64)
	return ok && f != 0
}

// helperWrite writes a helper's variable (with the lock held): nobody else may write today's
// and yesterday's amounts.
func (e *Engine) helperWrite(key string, value float64) {
	if key == "" {
		return
	}
	e.helping = true
	_ = e.write(key, value, FromProgram, "helper")
	e.helping = false
}

func (e *Engine) number(key string) float64 {
	if st := e.vars[key]; st != nil {
		if f, ok := st.value.(float64); ok {
			return f
		}
	}
	return 0
}

func roundHelper(x float64) float64 { return math.Round(x*1e6) / 1e6 }

// apply takes a step's amount into the helper's variables.
func (e *Engine) apply(g *model.HelperGroup, step model.HelperStep) {
	total, today := g.Vars[model.RoleTotal], g.Vars[model.RoleToday]
	if step.Set {
		e.helperWrite(total, step.Total)
		return
	}
	e.helperWrite(total, roundHelper(e.number(total)+step.Delta))
	e.helperWrite(today, roundHelper(e.number(today)+step.Delta))
}

// count handles a report of a datapoint for the helpers (with the lock held): a counter's
// reading is counted, a flag that comes after its reading corrects it.
func (e *Engine) count(ev event) {
	for _, g := range e.counters[ev.source] {
		dev, ok := ev.value.(float64)
		if !ok {
			continue
		}
		lead := e.vars[g.Lead()]
		before := lead.helper
		e.dirty = true
		if g.Off {
			// switched off: nothing is added, and the reading is where counting goes on from
			lead.helper.Old, lead.helper.Seen, lead.helper.Latch = dev, true, false
			delete(e.last, g.Lead())
			continue
		}
		max, known := e.maxima[ev.source[len("dp:"):]]
		flag := e.flagOf(g)
		step, next := g.Kind.Step(before, dev, flag, max, known)
		lead.helper = next
		e.last[g.Lead()] = &counted{at: ev.at, source: ev.source, before: before, dev: dev, flag: flag, step: step}
		e.noteStep(g, step, dev)
		if step.Note == "ignored" || (step.Note == "first" && !step.Set) {
			continue
		}
		e.apply(g, step)
	}
	for _, g := range e.flags[ev.source] {
		c := e.last[g.Lead()]
		if c == nil || g.Off {
			continue
		}
		delete(e.last, g.Lead())
		flag := truthy(ev.value)
		if flag == c.flag || ev.at.Sub(c.at) > lateFlag || ev.at.Before(c.at) {
			continue
		}
		// the flag belongs to the reading counted a moment ago: count that reading again
		max, known := e.maxima[c.source[len("dp:"):]]
		step, next := g.Kind.Step(c.before, c.dev, flag, max, known)
		if step.Set || c.step.Set {
			continue
		}
		e.vars[g.Lead()].helper = next
		e.dirty = true
		e.noteStep(g, step, c.dev)
		if diff := roundHelper(step.Delta - c.step.Delta); diff != 0 {
			e.apply(g, model.HelperStep{Delta: diff})
		}
	}
}

func (e *Engine) noteStep(g *model.HelperGroup, step model.HelperStep, dev float64) {
	switch step.Note {
	case "restart":
		e.logf("helper %s on %s: the device restarted, counting on from its reading %v", g.Kind.Kind, g.Channel, dev)
	case "overflow":
		e.logf("helper %s on %s: the device's counter began anew at %v after its maximum", g.Kind.Kind, g.Channel, dev)
	case "overflow-unknown":
		e.logf("helper %s on %s: the device's counter began anew at %v and its maximum is not known: only the new reading is counted", g.Kind.Kind, g.Channel, dev)
	}
}

// seedHelpers tells the helpers of a counter a value that is no report (with the lock held):
// one that has not seen its device yet takes it as where the counter stands.
func (e *Engine) seedHelpers(source string, value any) {
	dev, ok := value.(float64)
	if !ok {
		return
	}
	for _, g := range e.counters[source] {
		if lead := e.vars[g.Lead()]; !lead.helper.Seen {
			lead.helper.Old, lead.helper.Seen = dev, true
			e.dirty = true
		}
	}
}

// dayHelpers says whether a helper has a day's amount to roll.
func (e *Engine) dayHelpers() bool {
	for _, g := range e.helpers {
		if g.Vars[model.RoleToday] != "" {
			return true
		}
	}
	return false
}

// nextRoll answers the next day boundary of a helper with a day's amount after now.
func (e *Engine) nextRoll(now time.Time) (time.Time, bool) {
	var first time.Time
	for _, g := range e.helpers {
		if g.Vars[model.RoleToday] == "" {
			continue
		}
		if b := g.Kind.NextBoundary(e.local(now)); first.IsZero() || b.Before(first) {
			first = b
		}
	}
	return first, !first.IsZero()
}

// rollHelpers moves today's amount to yesterday's for every helper whose day boundary passed
// (with the lock held; the clock is set; the caller drains).
func (e *Engine) rollHelpers(now time.Time) {
	for _, g := range e.helpers {
		today := g.Vars[model.RoleToday]
		if today == "" {
			continue
		}
		lead := e.vars[g.Lead()]
		b := g.Kind.Boundary(e.local(now))
		rolled := lead.helper.Rolled
		switch {
		case rolled.IsZero() || rolled.After(now):
			// a helper that never rolled starts at the boundary behind it; a boundary in
			// the future was noted on a wrong clock
			lead.helper.Rolled = b
			e.dirty = true
		case b.After(rolled):
			if days := int(math.Round(b.Sub(rolled).Hours() / 24)); days > 1 {
				e.logf("helper %s on %s: %d day boundaries passed since %s: yesterday's amount holds all of them", g.Kind.Kind, g.Channel, days, rolled.Format(time.RFC3339))
			}
			lead.helper.Rolled = b
			e.dirty = true
			e.helperWrite(g.Vars[model.RoleYesterday], e.number(today))
			e.helperWrite(today, 0)
		}
	}
}

// Helpers answers the helpers, sorted by kind and channel.
func (e *Engine) Helpers() []HelperInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]HelperInfo, 0, len(e.helpers))
	for _, g := range e.helpers {
		in := HelperInfo{Kind: g.Kind.Kind, Channel: g.Channel, Variables: map[string]string{}, Off: g.Off, State: e.vars[g.Lead()].helper}
		for role, key := range g.Vars {
			in.Variables[role] = key
		}
		ch := model.ChannelRef(g.Channel)
		for _, p := range g.Kind.Counters {
			if m, ok := e.maxima[model.Ref{Interface: ch.Interface, Address: ch.Address, Param: p}.String()]; ok {
				in.Maximum = &m
				break
			}
		}
		out = append(out, in)
	}
	return out
}

// HelperCounters lists the counter datapoints the helpers follow, sorted: the daemon asks the
// system for the maximum of each.
func (e *Engine) HelperCounters() []model.Ref {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := map[model.Ref]bool{}
	for _, g := range e.helpers {
		ch := model.ChannelRef(g.Channel)
		for _, p := range g.Kind.Counters {
			seen[model.Ref{Interface: ch.Interface, Address: ch.Address, Param: p}] = true
		}
	}
	out := make([]model.Ref, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// SetCounterMaximum tells the engine a counter datapoint's maximum, from the device's
// description: what a counter that began anew had reached.
func (e *Engine) SetCounterMaximum(ref model.Ref, max float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maxima[ref.String()] = max
}

// SetHelperState sets what a helper remembers, by its leading variable's key: an import brings
// the CCU's last reading along. False when the variable leads no helper.
func (e *Engine) SetHelperState(key string, st model.HelperState) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := e.vars[key]
	if v == nil || v.def.Helper == nil || v.def.Helper.Role != model.HelperKindOf(v.def.Helper.Kind).Lead() {
		return false
	}
	v.helper = st
	delete(e.last, key)
	e.dirty = true
	return true
}
