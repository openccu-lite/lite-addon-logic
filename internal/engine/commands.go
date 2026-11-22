package engine

import (
	"fmt"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// A command (model.Command) is carried out by the Caller, like a webhook: the engine hands it
// over and goes on. What the command wrote and its exit code come back later and are written
// into the variables the activity names; with "wait" the rest of the block goes on then - the
// block waits, the engine and every other program do not.

// CommandFailed is the exit code a command has that did not run to its end: it could not be
// started, or it was ended at its timeout.
const CommandFailed = -1

// arguments answers a command's arguments with the values of variables and datapoints put in:
// args by position ("" for a text, whose placeholders the caller fills), shown the command as
// the history says it - secrets stay placeholders there. A value that is not known is the
// reason, with what it is the value of.
func (e *Engine) arguments(run *Run, c *model.Command) (args []string, values map[string]string, shown, reason, detail string) {
	values = e.filled(run, c.Placeholders())
	show := func(name string) string {
		if strings.HasPrefix(name, "secret:") {
			return "{" + name + "}"
		}
		return values[name]
	}
	same := func(s string) string { return s }
	var words []string
	if c.Shell {
		words = append(words, model.Expand(c.Line, show, func(s string) string {
			if strings.HasPrefix(s, "{secret:") {
				return s
			}
			return model.ShellQuote(s)
		}))
	}
	args = make([]string, len(c.Args))
	for i, a := range c.Args {
		switch {
		case a.Device != nil:
			v, ok := e.source("dp:" + a.Device.String())
			if !ok {
				return nil, nil, "", ReasonUnknownValue, a.Device.String()
			}
			args[i] = model.FormatValue(v, a.Format, nil)
		case a.Variable != "":
			v, ok := e.source("var:" + a.Variable)
			if !ok {
				return nil, nil, "", ReasonNoVariable, a.Variable
			}
			var entries []string
			if st := e.vars[a.Variable]; st != nil {
				entries = st.def.Entries
			}
			args[i] = model.FormatValue(v, a.Format, entries)
		default:
			words = append(words, word(model.Expand(a.Text, show, same)))
			continue
		}
		words = append(words, word(args[i]))
	}
	return args, values, strings.Join(words, " "), "", ""
}

// word writes an argument for the history: as it is, in quotes when it would read as several.
func word(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\$`|&;<>()*?#~") {
		return s
	}
	return model.ShellQuote(s)
}

// command hands a command to the caller and notes the step. then, when given, is what goes on
// once the command has ended: the answer says whether it will be called (false: nothing waits,
// the block goes on at once).
func (e *Engine) command(run *Run, step *Step, a *model.Action, then func()) bool {
	key, c := run.Program, a.Command
	args, values, shown, reason, detail := e.arguments(run, c)
	step.Value = shown
	switch {
	case reason != "":
		step.Value = nil
		step.What, step.Reason, step.Detail = StepSkipped, reason, detail
	case e.o.Caller == nil:
		step.What, step.Reason = StepSkipped, ReasonNoWriter
	default:
		step.What = StepCalled
	}
	run.Steps = append(run.Steps, *step)
	if step.What != StepCalled || run.Dry {
		return false
	}
	at, what := len(run.Steps)-1, describe(a)
	queued := e.o.Caller.Call(Call{Action: *a, Values: values, Args: args}, func(res CallResult) {
		e.mu.Lock()
		defer e.mu.Unlock()
		s := &run.Steps[at]
		s.Status, s.Took, s.Output = res.Exit, res.Took.Milliseconds(), head(res.Output)
		if res.Err == nil {
			s.Confirmed = e.o.Clock.Now()
		} else {
			s.Failed = res.Err.Error()
			if errs := head(res.Stderr); errs != "" {
				s.Failed += ": " + errs
			}
			e.logf("program %s: %s failed: %s", key, what, s.Failed)
			if ps := e.progs[key]; ps != nil {
				ps.failures++
				ps.lastFailure = fmt.Sprintf("%s: %s", what, s.Failed)
			}
		}
		e.result(run, s, c, res)
		if then != nil && e.progs[key] != nil {
			then()
		}
		e.dirty = true
		e.drain()
	})
	if !queued {
		s := &run.Steps[at]
		s.What, s.Reason = StepSkipped, ReasonQueueFull
		e.logf("program %s: %s dropped, too many commands are waiting", key, what)
		return false
	}
	return then != nil
}

// result writes what a command brought into its variables: the exit code always, the output
// when the command ended with 0. What a variable does not take is said on the step.
func (e *Engine) result(run *Run, s *Step, c *model.Command, res CallResult) {
	refuse := func(err error) {
		if s.Detail != "" {
			s.Detail += "; "
		}
		s.Detail += err.Error()
	}
	if c.ExitCode != "" {
		if err := e.write(c.ExitCode, float64(res.Exit), FromProgram, run.Trigger); err != nil {
			refuse(err)
		}
	}
	if c.Output == "" || res.Err != nil {
		return
	}
	st := e.vars[c.Output]
	if st == nil {
		refuse(writeErr("unknown-variable", "there is no variable %s", c.Output))
		return
	}
	v, ok := model.ParseOutput(st.def.Type, res.Output)
	if !ok {
		refuse(fmt.Errorf("%s does not take the output %q", c.Output, head(strings.TrimSpace(res.Output))))
		return
	}
	if err := e.write(c.Output, v, FromProgram, run.Trigger); err != nil {
		refuse(err)
	}
}

// OutputKept is how much of a command's output a step of the history keeps, in bytes.
const OutputKept = 400

// head answers the beginning of a command's output, as the history keeps it.
func head(s string) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "?")
	if len(s) > OutputKept {
		s = strings.ToValidUTF8(s[:OutputKept], "") + "…"
	}
	return s
}
