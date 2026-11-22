package engine

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// text says a value as a webhook sends it: true, 21.5, the text itself; nothing for none.
func text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// triggerName says what triggered a run, for a webhook's {trigger}: a variable's or a
// program's name, a datapoint as Interface/ADDRESS:CH/PARAM, else the kind - time, held,
// start, user, outside.
func (e *Engine) triggerName(trigger string) string {
	kind, rest, _ := strings.Cut(trigger, ":")
	switch kind {
	case "variable":
		if st := e.vars[rest]; st != nil {
			return st.def.Name
		}
		return rest
	case "program":
		if ps := e.progs[rest]; ps != nil {
			return ps.def.Name
		}
		return rest
	case "device":
		return rest
	}
	return kind
}

// placeholders answers what a webhook's placeholders stand for in a run - all but the secrets.
func (e *Engine) placeholders(run *Run, w *model.Webhook) map[string]string {
	return e.filled(run, w.Placeholders())
}

// filled answers what placeholders stand for in a run, by name - all but the secrets.
func (e *Engine) filled(run *Run, names []string) map[string]string {
	out := map[string]string{}
	for _, name := range names {
		switch {
		case name == "program":
			out[name] = run.Program
			if ps := e.progs[run.Program]; ps != nil {
				out[name] = ps.def.Name
			}
		case name == "trigger":
			out[name] = e.triggerName(run.Trigger)
		case name == "value":
			out[name] = text(run.value())
		case name == "time":
			out[name] = run.Time.Format(time.RFC3339)
		case strings.HasPrefix(name, "var:"):
			v, _ := e.source(name)
			out[name] = text(v)
		}
	}
	return out
}

// call hands a webhook or a Wake-on-LAN to the caller and notes the step; what became of it is
// noted on the step when the worker says so. A dry run notes the step and calls nothing.
func (e *Engine) call(run *Run, step *Step, a *model.Action) {
	key := run.Program
	if e.o.Caller == nil {
		step.What, step.Reason = StepSkipped, ReasonNoWriter
		run.Steps = append(run.Steps, *step)
		return
	}
	step.What = StepCalled
	run.Steps = append(run.Steps, *step)
	if run.Dry {
		return
	}
	c := Call{Action: *a}
	if a.Webhook != nil {
		c.Values = e.placeholders(run, a.Webhook)
	}
	at, what := len(run.Steps)-1, describe(a)
	queued := e.o.Caller.Call(c, func(res CallResult) {
		e.mu.Lock()
		defer e.mu.Unlock()
		s := &run.Steps[at]
		s.Status, s.Took = res.Status, res.Took.Milliseconds()
		if res.Err == nil {
			s.Confirmed = e.o.Clock.Now()
			return
		}
		s.Failed = res.Err.Error()
		e.logf("program %s: %s failed: %v", key, what, res.Err)
		if ps := e.progs[key]; ps != nil {
			ps.failures++
			ps.lastFailure = fmt.Sprintf("%s: %v", what, res.Err)
		}
	})
	if !queued {
		s := &run.Steps[at]
		s.What, s.Reason = StepSkipped, ReasonQueueFull
		e.logf("program %s: %s dropped, too many calls are waiting", key, what)
	}
}
