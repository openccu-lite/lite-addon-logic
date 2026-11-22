package engine

import "fmt"

// simulation is what a dry run tries instead of the real world: values of sources, and a time
// module line taken as firing now.
type simulation struct {
	values map[string]any
	fired  *lineState
}

// DryRun answers what a program would do if it were evaluated now - "Jetzt prüfen" without an
// effect: the block that would run, every condition line as it stands, and what each activity
// would do. Nothing is sent, written, scheduled or remembered, and no line's remembered truth
// moves. It works for an inactive program too.
func (e *Engine) DryRun(key string) (Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps, err := e.simulable(key)
	if err != nil {
		return Run{}, err
	}
	return e.dry(key, ps, cause{by: "user"}), nil
}

// Simulate answers what would happen if a condition line's source reported a value - or, for
// a time module line, if it fired now: whether that would trigger the program (as the line's
// mode and the truth it remembers decide; an entry "not triggered" with the reason otherwise),
// and the run it would cause. line counts the program's condition lines from 1. Nothing is
// changed.
func (e *Engine) Simulate(key string, line int, value any) (Run, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps, err := e.simulable(key)
	if err != nil {
		return Run{}, err
	}
	if line < 1 || line > len(ps.lines) {
		return Run{}, writeErr("invalid", "%s has no condition line %d", key, line)
	}
	ls := ps.lines[line-1]
	l := ls.def
	if l.Time != nil {
		e.sim = &simulation{fired: ls}
		defer func() { e.sim = nil }()
		return e.dry(key, ps, cause{by: fmt.Sprintf("time:%d:point", line)}), nil
	}
	src := lineSource(l)
	if n, ok := value.(int); ok {
		value = float64(n)
	}
	switch value.(type) {
	case bool, float64, string:
	default:
		return Run{}, writeErr("invalid", "a value is true or false, a number or a text")
	}
	if l.Variable != "" && !IsSystemValue(l.Variable) {
		if _, _, err := e.writable(l.Variable, value, FromProgram); err != nil {
			return Run{}, err
		}
	}
	prev, known := e.source(src)
	ev := event{source: src, value: value, prev: prev, changed: known && !looseEqual(prev, value), first: !known && l.Device != nil, at: e.o.Clock.Now()}
	c := cause{by: "variable:" + l.Variable, ev: &Event{Value: value, Prev: prev, First: ev.first}}
	if l.Device != nil {
		c.by = "device:" + l.Device.String()
	}
	e.sim = &simulation{values: map[string]any{src: value}}
	defer func() { e.sim = nil }()
	// every line of the program on this source has its say, as with a real report
	fires, why := false, InfoCheckOnly
	for _, other := range ps.lines {
		if lineSource(other.def) != src {
			continue
		}
		f, info, _ := e.decide(other, ev)
		fires = fires || f
		if infoRank[info] > infoRank[why] {
			why = info
		}
	}
	if !fires {
		return Run{Program: key, Time: ev.at, Trigger: c.by, Event: c.ev, None: true, Untriggered: true, Info: why, Count: 1, Since: ev.at, Dry: true}, nil
	}
	return e.dry(key, ps, c), nil
}

func (e *Engine) simulable(key string) (*progState, error) {
	ps := e.progs[key]
	if ps == nil {
		return nil, writeErr("unknown-program", "there is no program %s", key)
	}
	if len(ps.problems) > 0 {
		return nil, writeErr("invalid", "%s cannot run: %s", key, ps.problems[0])
	}
	return ps, nil
}

// dry evaluates a program and walks the block that would run, without an effect.
func (e *Engine) dry(key string, ps *progState, c cause) Run {
	run := &Run{Program: key, Time: e.o.Clock.Now(), Trigger: c.by, Event: c.ev, None: true, Dry: true}
	block, lines := e.evaluate(ps)
	run.Lines = lines
	if block >= 0 {
		r := &ps.def.Rules[block]
		run.None = false
		run.Block = block + 1
		if r.Else {
			run.Block = 0
		}
		e.runBlock(run, r, block)
	}
	return *run
}
