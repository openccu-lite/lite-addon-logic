package engine

import (
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// HoldLine is a line with "for" whose comparison holds, as the schedule page shows it: since
// when, and when it has held (or had held) long enough.
type HoldLine struct {
	// Line counts the program's condition lines from 1.
	Line  int            `json:"line"`
	For   model.Duration `json:"for"`
	Since time.Time      `json:"since"`
	Due   time.Time      `json:"due"`
	// True: it has held long enough, the line is true now.
	True bool `json:"true"`
}

// HoldLines answers a program's lines with "for" that are counting or have counted to their
// end; a line whose comparison does not hold is not among them.
func (e *Engine) HoldLines(key string) []HoldLine {
	e.mu.Lock()
	defer e.mu.Unlock()
	ps := e.progs[key]
	if ps == nil {
		return nil
	}
	now := e.now()
	out := []HoldLine{}
	for i, l := range ps.lines {
		if l.def.For <= 0 || l.since.IsZero() {
			continue
		}
		due := l.since.Add(l.def.For.Std())
		out = append(out, HoldLine{Line: i + 1, For: l.def.For, Since: l.since, Due: due, True: !now.Before(due)})
	}
	return out
}
