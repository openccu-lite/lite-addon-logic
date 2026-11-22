package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// pending is a delayed activity that is not due yet. The registry is one for all programs,
// keyed by what the activity targets: a block with "cancel pending" removes the entries for the
// targets it names - whoever scheduled them.
type pending struct {
	Pending
	timer Timer
}

// Pending is a delayed activity as the page sees it and the state file keeps it.
type Pending struct {
	ID  int       `json:"id"`
	Due time.Time `json:"due"`
	// Set is when it was scheduled (zero in a state file written before this was kept).
	Set     time.Time    `json:"set"`
	Program string       `json:"program"`
	Rule    int          `json:"rule"`
	Action  model.Action `json:"action"`
	// By is what triggered the run that scheduled it.
	By string `json:"by"`
	// Index counts the block's activities from 1 (0 in a state file written before this was
	// kept); Run and Step name the trace entry and its step that scheduled it.
	Index int    `json:"index,omitempty"`
	Run   uint64 `json:"run,omitempty"`
	Step  int    `json:"step,omitempty"`
	// Value is the value the trigger brought, kept for a webhook's {value}.
	Value any `json:"value,omitempty"`
}

// PendingActions lists the delayed activities that are not due yet, the next one first.
func (e *Engine) PendingActions() []Pending {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Pending, 0, len(e.pending))
	for _, p := range e.pending {
		out = append(out, p.Pending)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Due.Equal(out[j].Due) {
			return out[i].Due.Before(out[j].Due)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// CancelPending removes one delayed activity; false when it is not there (any more).
func (e *Engine) CancelPending(id int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, p := range e.pending {
		if p.ID == id {
			p.timer.Stop()
			e.ended(&p.Pending, "user")
			e.pending = append(e.pending[:i], e.pending[i+1:]...)
			e.dirty = true
			e.notify(Notification{Kind: "pending"})
			return true
		}
	}
	return false
}

// RunPending carries out one delayed activity now instead of when it is due; false when it is
// not there (any more).
func (e *Engine) RunPending(id int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.pending {
		if p.ID == id {
			p.timer.Stop()
			e.fire(id)
			e.drain()
			return true
		}
	}
	return false
}

// runBlock runs a rule's activities for a run and notes on it what each did: first, with the
// CCU's checkbox, the pending delayed activities for the targets this block names are removed;
// then the activities in their listed order, an immediate one now, a delayed one into the
// registry - due after its delay counted from this moment, not from the line above, and not
// checked again when it is due.
func (e *Engine) runBlock(run *Run, r *model.Rule, rule int) {
	if r.CancelPending {
		targets := map[string]bool{}
		for i := range r.Then {
			if t := r.Then[i].Target(); t != "" {
				targets[t] = true
			}
		}
		n := 0
		if run.Dry {
			for _, p := range e.pending {
				if targets[p.Action.Target()] {
					n++
				}
			}
		} else {
			n = e.cancelTargets(targets, "program:"+run.Program)
		}
		if n > 0 {
			run.Steps = append(run.Steps, Step{What: StepCancelled, Count: n})
		}
	}
	e.runFrom(run, r, rule, 0, run.Time)
}

// runFrom runs a block's activities from one of them on (counted from 0); base is the moment
// their delays count from: the run's, or - after a command the block waited for - the moment
// the command ended.
func (e *Engine) runFrom(run *Run, r *model.Rule, rule, from int, base time.Time) {
	for i := from; i < len(r.Then); i++ {
		a := &r.Then[i]
		delay := a.After
		if a.AfterUpTo > a.After {
			// a random delay, to the second, drawn anew each time the block runs
			span := int64((a.AfterUpTo - a.After).Std() / time.Second)
			delay += model.Duration(time.Duration(e.o.Random(span)) * time.Second)
		}
		if delay > 0 && a.Kind != model.ActionScript {
			due := base.Add(delay.Std())
			run.Steps = append(run.Steps, Step{Action: i + 1, Def: a, What: StepScheduled, Due: due})
			if run.Dry {
				continue
			}
			p := Pending{Due: due, Set: base, Program: run.Program, Rule: rule, Action: *a, By: run.Trigger, Index: i + 1, Run: run.Seq, Step: len(run.Steps) - 1}
			if a.Kind == model.ActionWebhook || a.Kind == model.ActionCommand {
				p.Value = run.value()
			}
			e.schedule(p)
			continue
		}
		if a.Kind == model.ActionCommand && a.Command.Wait {
			next := i + 1
			// the block waits for this command: the rest goes on when it has ended
			if e.command(run, &Step{Action: i + 1, Def: a}, a, func() { e.runFrom(run, r, rule, next, e.o.Clock.Now()) }) {
				return
			}
			continue
		}
		e.do(run, i+1, a)
	}
}

func (e *Engine) cancelTargets(targets map[string]bool, by string) int {
	n := 0
	kept := e.pending[:0]
	for _, p := range e.pending {
		if targets[p.Action.Target()] {
			p.timer.Stop()
			e.ended(&p.Pending, by)
			n++
			continue
		}
		kept = append(kept, p)
	}
	e.pending = kept
	if n > 0 {
		e.dirty = true
		e.notify(Notification{Kind: "pending"})
	}
	return n
}

// schedule puts a delayed activity into the registry and arms its timer (with the lock held).
func (e *Engine) schedule(p Pending) {
	e.nextID++
	p.ID = e.nextID
	entry := &pending{Pending: p}
	id := p.ID
	wait := p.Due.Sub(e.o.Clock.Now())
	entry.timer = e.o.Clock.AfterFunc(wait, func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.fire(id)
		e.drain()
	})
	e.pending = append(e.pending, entry)
	e.dirty = true
	e.notify(Notification{Kind: "pending"})
}

// fire runs a delayed activity that is due: an entry of its own in the trace, and a note on
// the step that scheduled it.
func (e *Engine) fire(id int) {
	for i, p := range e.pending {
		if p.ID != id {
			continue
		}
		e.pending = append(e.pending[:i], e.pending[i+1:]...)
		e.dirty = true
		e.notify(Notification{Kind: "pending"})
		now := e.o.Clock.Now()
		if s := e.findStep(&p.Pending); s != nil {
			s.Ran = now
		}
		run := &Run{Program: p.Program, Time: now, Trigger: p.By, Block: p.Rule + 1, Delayed: true, brought: p.Value}
		if ps := e.progs[p.Program]; ps != nil {
			if p.Rule < len(ps.def.Rules) && ps.def.Rules[p.Rule].Else {
				run.Block = 0
			}
			e.record(ps, run)
		}
		e.do(run, p.Index, &p.Action)
		return
	}
}

// sent answers what a device write calls when the interface has answered: the step (the
// run's step at) is confirmed, or it failed - said on the step, in the log and on the program.
func (e *Engine) sent(run *Run, at int, what string) func(error) {
	key := run.Program
	return func(err error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		s := &run.Steps[at]
		if err == nil {
			if s.Failed == "" {
				s.Confirmed = e.o.Clock.Now()
			}
			return
		}
		s.Failed, s.Confirmed = err.Error(), time.Time{}
		e.logf("program %s: the write to %s failed: %v", key, what, err)
		if ps := e.progs[key]; ps != nil {
			ps.failures++
			ps.lastFailure = fmt.Sprintf("%s: %v", what, err)
		}
	}
}

func describe(a *model.Action) string {
	switch a.Kind {
	case model.ActionSet:
		return "set " + a.Device.String()
	case model.ActionVariable:
		return "variable " + a.Variable
	case model.ActionRun:
		return "run " + a.Program
	case model.ActionActivate, model.ActionDeactivate:
		return string(a.Kind) + " " + a.Program
	case model.ActionChannel:
		parts := make([]string, len(a.Values))
		for i, v := range a.Values {
			parts[i] = fmt.Sprintf("%s = %v", v.Param, v.Value)
		}
		return "channel " + a.Channel + ": " + strings.Join(parts, ", ")
	case model.ActionDisplay:
		var parts []string
		for _, l := range a.Display.Lines {
			parts = append(parts, strings.TrimSpace(l.Text+" "+l.Icon))
		}
		if a.Display.Sound != "" {
			parts = append(parts, "sound "+a.Display.Sound)
		}
		return "display " + a.Channel + ": " + strings.Join(parts, " | ")
	case model.ActionWebhook:
		return "webhook " + a.Webhook.Method + " " + a.Webhook.URL
	case model.ActionWake:
		return "wake " + a.Wake.MAC
	case model.ActionCommand:
		return "command " + a.Command.Program()
	}
	return "script"
}

// do carries out one activity of a run and notes on the run what it did.
func (e *Engine) do(run *Run, index int, a *model.Action) {
	key := run.Program
	step := Step{Action: index, Def: a}
	add := func(what, reason, detail string) {
		step.What, step.Reason, step.Detail = what, reason, detail
		run.Steps = append(run.Steps, step)
	}
	switch a.Kind {
	case model.ActionScript:
		// the text is in the program's file: the trace does not carry it
		bare := *a
		bare.Script = ""
		step.Def = &bare
		add(StepSkipped, ReasonScript, "")
		return
	case model.ActionRun:
		if e.progs[a.Program] == nil {
			add(StepSkipped, ReasonNoProgram, a.Program)
			return
		}
		if !run.Dry {
			e.post(event{run: a.Program, mode: a.Mode, by: "program:" + key, at: e.at()})
		}
		add(StepRun, "", "")
		return
	case model.ActionWebhook, model.ActionWake:
		e.call(run, &step, a)
		return
	case model.ActionCommand:
		e.command(run, &step, a, nil)
		return
	case model.ActionDisplay:
		// the text the display takes, with the placeholders as they are now: the step keeps it
		values := e.filled(run, a.Display.Placeholders())
		text := a.Display.Combined(func(name string) string { return values[name] })
		step.Value = text
		if e.o.Writer == nil {
			add(StepSkipped, ReasonNoWriter, "")
			return
		}
		add(StepSent, "", "")
		if run.Dry {
			return
		}
		ref := model.ChannelRef(a.Channel)
		done := e.sent(run, len(run.Steps)-1, a.Channel)
		if sw, ok := e.o.Writer.(SetWriter); ok {
			sw.WriteSet(ref, []model.ParamValue{{Param: "COMBINED_PARAMETER", Value: text}}, done)
			return
		}
		e.o.Writer.Write(model.Ref{Interface: ref.Interface, Address: ref.Address, Param: "COMBINED_PARAMETER"}, text, done)
		return
	case model.ActionChannel:
		if e.o.Writer == nil {
			add(StepSkipped, ReasonNoWriter, "")
			return
		}
		add(StepSent, "", "")
		if run.Dry {
			return
		}
		ref := model.ChannelRef(a.Channel)
		done := e.sent(run, len(run.Steps)-1, a.Channel)
		if sw, ok := e.o.Writer.(SetWriter); ok {
			sw.WriteSet(ref, a.Values, done)
			return
		}
		// a writer that cannot write them together: one after the other, in their order
		for _, v := range a.Values {
			e.o.Writer.Write(model.Ref{Interface: ref.Interface, Address: ref.Address, Param: v.Param}, v.Value, done)
		}
		return
	case model.ActionActivate, model.ActionDeactivate:
		if e.progs[a.Program] == nil {
			add(StepSkipped, ReasonNoProgram, a.Program)
			return
		}
		if active := a.Kind == model.ActionActivate; !run.Dry {
			e.notify(Notification{Kind: "program", Key: a.Program, Active: &active})
		}
		add(StepSwitched, "", "")
		return
	}
	switch {
	case a.Special != "":
		add(StepSkipped, ReasonSpecial, a.Special)
		return
	case a.FromDevice != nil:
		v, ok := e.source("dp:" + a.FromDevice.String())
		if !ok {
			add(StepSkipped, ReasonUnknownValue, a.FromDevice.String())
			return
		}
		step.Value = v
	case a.FromVariable != "":
		v, ok := e.source("var:" + a.FromVariable)
		if !ok {
			add(StepSkipped, ReasonNoVariable, a.FromVariable)
			return
		}
		step.Value = v
	default:
		step.Value = a.To
	}
	value := step.Value
	if a.Kind == model.ActionVariable {
		var err error
		if run.Dry {
			_, _, err = e.writable(a.Variable, value, FromProgram)
		} else {
			err = e.write(a.Variable, value, FromProgram, run.Trigger)
		}
		if err != nil {
			code := ""
			if we, ok := err.(*WriteError); ok {
				code = we.Code
			}
			add(StepRefused, code, err.Error())
			return
		}
		add(StepWritten, "", "")
		return
	}
	if e.o.Writer == nil {
		add(StepSkipped, ReasonNoWriter, "")
		return
	}
	add(StepSent, "", "")
	if run.Dry {
		return
	}
	ref := *a.Device
	done := e.sent(run, len(run.Steps)-1, ref.String())
	if a.OnFor > 0 {
		// "ein für": the on-time with the value, in one command where the writer can
		onTime := model.ParamValue{Param: "ON_TIME", Value: a.OnFor.Std().Seconds()}
		if sw, ok := e.o.Writer.(SetWriter); ok {
			sw.WriteSet(ref, []model.ParamValue{onTime, {Param: ref.Param, Value: value}}, done)
		} else {
			e.o.Writer.Write(model.Ref{Interface: ref.Interface, Address: ref.Address, Param: onTime.Param}, onTime.Value, done)
			e.o.Writer.Write(ref, value, done)
		}
		return
	}
	e.o.Writer.Write(ref, value, done)
}
