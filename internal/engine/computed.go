package engine

import (
	"sort"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Computed values: number variables whose value is calculated from device datapoints and other
// variables. The engine calculates one again whenever one of its inputs reports - before the
// programs look at that report, so a program that reads the input and the computed value sees
// both as they are now - and the new value is a write like any other: an update for the
// programs on the variable, a change when it differs. While an input is not known the value
// stays what it was.

// indexComputed notes which computed values each source feeds (with the lock held).
func (e *Engine) indexComputed() {
	e.inputs = map[string][]string{}
	for k, st := range e.vars {
		if st.def.Computed == nil || e.set.ComputedProblem(st.def) != "" {
			// (inputs that are missing or lead back to the value itself: it is not calculated)
			continue
		}
		seen := map[string]bool{}
		for _, in := range st.def.Computed.Inputs {
			if src := in.Source(); !seen[src] {
				seen[src] = true
				e.inputs[src] = append(e.inputs[src], k)
			}
		}
	}
	for _, keys := range e.inputs {
		sort.Strings(keys)
	}
}

// compute calculates a computed value from what is known now (with the lock held); ok is false
// while an input has no value, or no number.
func (e *Engine) compute(def *model.Variable) (float64, bool) {
	if e.set.ComputedProblem(def) != "" {
		return 0, false
	}
	in := make([]float64, 0, len(def.Computed.Inputs))
	for _, input := range def.Computed.Inputs {
		var v any
		var known bool
		if input.Device != nil {
			v, known = e.dps[input.Device.String()]
		} else {
			v, known = e.valueLocked(input.Variable)
		}
		f, isNumber := v.(float64)
		if !known || !isNumber {
			return 0, false
		}
		in = append(in, f)
	}
	return model.Compute(def.Computed.Function, in)
}

// recompute calculates the computed values a source feeds (with the lock held). Loud, each is
// written like a variable: its event is queued, and the values it feeds follow when that event
// is handled. Silent - for values that are told without being events: the state at a start, an
// import - the values are set and what they feed is calculated right away, without events. (There is
// no circle to run around in: a value whose inputs lead back to itself is not indexed.)
func (e *Engine) recompute(source string, silent bool) {
	for _, key := range e.inputs[source] {
		st := e.vars[key]
		value, ok := e.compute(st.def)
		if !ok {
			continue
		}
		if silent {
			if !equal(st.value, value) {
				st.value = value
				e.dirty = true
			}
			e.recompute("var:"+key, true)
			continue
		}
		e.computing = true
		_ = e.write(key, value, FromProgram, "computed")
		e.computing = false
	}
}

// recomputeAll calculates every computed value without events: after the definitions changed.
func (e *Engine) recomputeAll() {
	keys := make([]string, 0)
	for k, st := range e.vars {
		if st.def.Computed != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	// as many rounds as there are computed values settle any chain of them
	for range keys {
		moved := false
		for _, k := range keys {
			st := e.vars[k]
			if value, ok := e.compute(st.def); ok && !equal(st.value, value) {
				st.value, moved, e.dirty = value, true, true
			}
		}
		if !moved {
			break
		}
	}
}
