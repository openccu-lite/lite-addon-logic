package engine

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// fakeCaller keeps the calls; full makes it refuse them.
type fakeCaller struct {
	calls []Call
	done  []func(CallResult)
	full  bool
}

func (c *fakeCaller) Call(call Call, done func(CallResult)) bool {
	if c.full {
		return false
	}
	c.calls = append(c.calls, call)
	c.done = append(c.done, done)
	return true
}

const calling = `rules:
  - when:
      all:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: change
    cancel_pending: true
    then:
      - webhook: "http://h/{var:y}"
        method: POST
        body_type: json
        body: '{"p": "{program}", "by": "{trigger}", "v": {value}, "t": "{time}", "text": "{var:text}", "k": "{secret:key}"}'
      - wake: "AA:BB:CC:00:11:22"
      - webhook: "http://h/{var:y}"
        method: GET
        after: 10min
  - else:
      - webhook: "http://h/else?v={value}&by={trigger}"
        method: GET
`

// A webhook and a Wake-on-LAN are handed to the caller with the placeholders' values; what
// became of them is on the step.
func TestCalls(t *testing.T) {
	r := newPrig(t)
	c := &fakeCaller{}
	r.e.o.Caller = c
	r.set("text", "hallo", FromUser)
	r.set("y", 7, FromUser)
	r.add("p", calling)
	r.set("x", 11.5, FromUser)
	if len(c.calls) != 2 {
		t.Fatalf("calls %+v", c.calls)
	}
	want := map[string]string{"var:y": "7", "program": "p", "trigger": "X", "value": "11.5", "time": t0.Format(time.RFC3339), "var:text": "hallo"}
	if got := c.calls[0].Values; !reflect.DeepEqual(got, want) || c.calls[0].Action.Webhook.Method != "POST" {
		t.Errorf("webhook %+v", c.calls[0])
	}
	if c.calls[1].Action.Wake.MAC != "AA:BB:CC:00:11:22" || c.calls[1].Values != nil {
		t.Errorf("wake %+v", c.calls[1])
	}
	run := r.e.Runs("p")[0]
	if got := strings.Join(run.Notes(), "\n"); got != "1: webhook POST http://h/{var:y}\n2: wake AA:BB:CC:00:11:22\n3: webhook GET http://h/{var:y} in 10min" {
		t.Errorf("notes:\n%s", got)
	}
	if run.Steps[0].What != StepCalled || !run.Steps[0].Confirmed.IsZero() {
		t.Errorf("step %+v", run.Steps[0])
	}
	// the answers: the webhook failed, the wake went out
	r.clock.Advance(time.Second)
	c.done[0](CallResult{Status: 500, Took: 1234 * time.Millisecond, Err: errors.New("HTTP 500")})
	c.done[1](CallResult{Took: 200 * time.Millisecond})
	run = r.e.Runs("p")[0]
	if s := run.Steps[0]; s.Failed != "HTTP 500" || s.Status != 500 || s.Took != 1234 || !s.Confirmed.IsZero() {
		t.Errorf("failed step %+v", s)
	}
	if s := run.Steps[1]; s.Failed != "" || !s.Confirmed.Equal(t0.Add(time.Second)) || s.Took != 200 {
		t.Errorf("confirmed step %+v", s)
	}
	if in, _ := r.e.Program("p"); in.Failures != 1 || in.LastFailure != "webhook POST http://h/{var:y}: HTTP 500" {
		t.Errorf("program %+v", in)
	}
	if !strings.Contains(strings.Join(r.logs, "\n"), "program p: webhook POST http://h/{var:y} failed: HTTP 500") {
		t.Errorf("logs %v", r.logs)
	}

	// the delayed one keeps the trigger's value, takes the variables as they are when it runs,
	// and can be ended like any delay
	r.set("y", 8, FromUser)
	r.clock.Advance(10 * time.Minute)
	if len(c.calls) != 3 || c.calls[2].Values["var:y"] != "8" {
		t.Fatalf("delayed %+v", c.calls)
	}
	r.set("x", 0, FromUser) // SONST: the value and the trigger in the address
	if got := c.calls[3].Values; got["value"] != "0" || got["trigger"] != "X" {
		t.Errorf("else %+v", got)
	}
	r.set("x", 11, FromUser)
	if n := len(r.e.PendingActions()); n != 1 {
		t.Fatalf("%d pending", n)
	}
	r.set("x", 0, FromUser)
	r.set("x", 12, FromUser) // "end running delays": the pending webhook goes, a new one comes
	p := r.e.PendingActions()
	if len(p) != 1 || p[0].Value != 12.0 {
		t.Fatalf("pending %+v", p)
	}
	// it survives a restart with its value
	st, _ := r.e.Export()
	b, _ := MarshalState(st)
	back, err := UnmarshalState(b)
	if err != nil || len(back.Pending) != 1 || back.Pending[0].Value != 12.0 || back.Pending[0].Action.Webhook.URL != "http://h/{var:y}" {
		t.Fatalf("state %s: %v", b, err)
	}

	// too many waiting: dropped, and said so
	c.full = true
	n := len(c.calls)
	r.set("x", 0, FromUser)
	runs := r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; s.What != StepSkipped || s.Reason != ReasonQueueFull || !strings.Contains(s.String(), "too many calls are waiting") || len(c.calls) != n {
		t.Errorf("full: %+v", s)
	}
	// a program that is gone when the answer comes
	c.full = false
	r.set("x", 13, FromUser)
	delete(r.progs, "p")
	r.e.SetPrograms(r.progs)
	c.done[len(c.done)-1](CallResult{Err: errors.New("late")})
}

// Without a caller the activities are skipped; a dry run notes the step and calls nothing; the
// other triggers have their words.
func TestCallsNotMade(t *testing.T) {
	r := newPrig(t)
	r.add("p", calling)
	r.set("x", 11, FromUser)
	run := r.e.Runs("p")[0]
	if s := run.Steps[0]; s.What != StepSkipped || s.Reason != ReasonNoWriter || !strings.Contains(s.String(), "nothing sends calls") {
		t.Errorf("no caller: %+v", s)
	}
	c := &fakeCaller{}
	r.e.o.Caller = c
	dry, err := r.e.DryRun("p")
	if err != nil || len(c.calls) != 0 || len(dry.Steps) != 4 || dry.Steps[1].What != StepCalled || dry.Steps[2].What != StepCalled || dry.Steps[3].What != StepScheduled {
		t.Errorf("dry run %+v: %v", dry.Steps, err)
	}
	// run by hand, by another program, by one that is gone
	r.add("caller", "rules:\n  - when:\n      all: []\n    then:\n      - run: p\n        mode: then\n")
	_ = r.e.RunProgramBy("p", model.RunCheck, "user")
	_ = r.e.RunProgramBy("caller", model.RunThen, "user")
	if got := []string{c.calls[0].Values["trigger"], c.calls[0].Values["value"], c.calls[2].Values["trigger"]}; !reflect.DeepEqual(got, []string{"user", "", "caller"}) {
		t.Errorf("triggers %v", got)
	}
	r.e.mu.Lock()
	names := []string{r.e.triggerName("variable:gone"), r.e.triggerName("program:gone"), r.e.triggerName("device:BidCos-RF/ABC:1/STATE"), r.e.triggerName("time:1:point"), r.e.triggerName("start")}
	gone := r.e.placeholders(&Run{Program: "gone", Trigger: "held:1"}, &model.Webhook{URL: "{program}{trigger}{var:nope}"})
	r.e.mu.Unlock()
	if !reflect.DeepEqual(names, []string{"gone", "gone", "BidCos-RF/ABC:1/STATE", "time", "start"}) || gone["program"] != "gone" || gone["trigger"] != "held" || gone["var:nope"] != "" {
		t.Errorf("names %v, %v", names, gone)
	}
	if text(true) != "true" || text(nil) != "" || text(0.1) != "0.1" || text("a") != "a" {
		t.Error("values as text")
	}
}

const light = `rules:
  - when:
      all: []
    cancel_pending: true
    then:
      - channel: HmIP-RF/A:8
        values:
          COLOR: RED
          LEVEL: 0.5
      - channel: HmIP-RF/A:8
        values:
          COLOR: BLACK
        after: 10min
`

// Several values of a channel go out as one command, in a step of their own; a writer that
// cannot send them together gets them one after the other.
func TestChannelValues(t *testing.T) {
	r := newPrig(t)
	w := &setWriter{}
	r.e.o.Writer = w
	r.add("p", light)
	_ = r.e.RunProgramBy("p", model.RunThen, "user")
	if len(w.sets) != 1 || w.sets[0] != "HmIP-RF/A:8 [{COLOR RED} {LEVEL 0.5}]" || len(w.writes) != 0 {
		t.Fatalf("sets %v, writes %v", w.sets, w.writes)
	}
	run := r.e.Runs("p")[0]
	if got := strings.Join(run.Notes(), "\n"); got != "1: channel HmIP-RF/A:8: COLOR = RED, LEVEL = 0.5\n2: channel HmIP-RF/A:8: COLOR = BLACK in 10min" {
		t.Errorf("notes:\n%s", got)
	}
	// the interface refuses it: said on the step and on the program
	w.done[0](errors.New(`COLOR has no entry "RED"`))
	run = r.e.Runs("p")[0]
	if s := run.Steps[0]; s.What != StepSent || s.Failed != `COLOR has no entry "RED"` {
		t.Errorf("step %+v", s)
	}
	if in, _ := r.e.Program("p"); in.Failures != 1 || in.LastFailure != `HmIP-RF/A:8: COLOR has no entry "RED"` {
		t.Errorf("program %+v", in)
	}
	// run again: "end running delays" ends the delay for the same channel
	_ = r.e.RunProgramBy("p", model.RunThen, "user")
	if n := len(r.e.PendingActions()); n != 1 {
		t.Errorf("%d pending", n)
	}
	w.done[1](nil)
	runs := r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[1]; s.Confirmed.IsZero() {
		t.Errorf("confirmed step %+v", s)
	}
	// the delayed one, when it is due
	r.clock.Advance(10 * time.Minute)
	if len(w.sets) != 3 || w.sets[2] != "HmIP-RF/A:8 [{COLOR BLACK}]" {
		t.Errorf("delayed: %v", w.sets)
	}
	// a dry run sends nothing
	dry, err := r.e.DryRun("p")
	if err != nil || len(w.sets) != 3 || dry.Steps[0].What != StepSent {
		t.Errorf("dry run %+v: %v", dry.Steps, err)
	}

	// a writer without WriteSet, and none at all
	plain := &fakeWriter{}
	r.e.o.Writer = plain
	_ = r.e.RunProgramBy("p", model.RunThen, "user")
	if !reflect.DeepEqual(plain.writes, []write{{"HmIP-RF/A:8/COLOR", "RED"}, {"HmIP-RF/A:8/LEVEL", 0.5}}) {
		t.Errorf("one after the other: %v", plain.writes)
	}
	r.e.o.Writer = nil
	_ = r.e.RunProgramBy("p", model.RunThen, "user")
	runs = r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[1]; s.What != StepSkipped || s.Reason != ReasonNoWriter || s.String() != "channel HmIP-RF/A:8: COLOR = RED, LEVEL = 0.5: dropped, nothing sends device writes" {
		t.Errorf("no writer: %q", s.String())
	}
}

const showing = `rules:
  - when:
      all:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: value
    then:
      - display: HmIP-RF/W:3
        lines:
          - text: "Wert {value}"
            icon: BELL
          - text: "{var:text} ü"
        sound: EVENT
      - display: HmIP-RF/W:3
        lines:
          - text: "spät {var:text}"
        after: 10min
`

// A display activity goes out as one command with the text the channel takes, the placeholders
// as they are when it is sent; the step keeps that text.
func TestDisplay(t *testing.T) {
	r := newPrig(t)
	w := &setWriter{}
	r.e.o.Writer = w
	r.set("text", "hallo", FromUser)
	r.add("p", showing)
	r.set("x", 11.5, FromUser)
	want := "{DDBC=WHITE,DDTC=BLACK,DDI=30,DDA=CENTER,DDS=Wert 11.5,DDID=1},{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=hallo ³,DDID=2,DDC=true},{R=0,IN=5,ANS=6}"
	if len(w.sets) != 1 || w.sets[0] != "HmIP-RF/W:3 [{COMBINED_PARAMETER "+want+"}]" {
		t.Fatalf("sets %v", w.sets)
	}
	run := r.e.Runs("p")[0]
	if got := strings.Join(run.Notes(), "\n"); got != "1: display HmIP-RF/W:3: Wert {value} BELL | {var:text} ü | sound EVENT\n2: display HmIP-RF/W:3: spät {var:text} in 10min" {
		t.Errorf("notes:\n%s", got)
	}
	if s := run.Steps[0]; s.What != StepSent || s.Value != want {
		t.Errorf("step %+v", s)
	}
	w.done[0](errors.New("the channel has no datapoint COMBINED_PARAMETER"))
	if in, _ := r.e.Program("p"); in.Failures != 1 || in.LastFailure != "HmIP-RF/W:3: the channel has no datapoint COMBINED_PARAMETER" {
		t.Errorf("program %+v", in)
	}
	// the delayed one takes the variable as it is when it is due
	r.set("text", "neu", FromUser)
	r.clock.Advance(10 * time.Minute)
	if len(w.sets) != 2 || w.sets[1] != "HmIP-RF/W:3 [{COMBINED_PARAMETER {DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=sp²t neu,DDID=1,DDC=true}}]" {
		t.Errorf("delayed: %v", w.sets)
	}
	// a dry run sends nothing and says what would be shown
	dry, err := r.e.DryRun("p")
	if err != nil || len(w.sets) != 2 || dry.Steps[0].What != StepSent || !strings.Contains(dry.Steps[0].Value.(string), "DDS=neu ³") {
		t.Errorf("dry run %+v: %v", dry.Steps, err)
	}
	// a writer without WriteSet, and none at all
	plain := &fakeWriter{}
	r.e.o.Writer = plain
	_ = r.e.RunProgramBy("p", model.RunThen, "user")
	if len(plain.writes) != 1 || plain.writes[0].ref != "HmIP-RF/W:3/COMBINED_PARAMETER" || !strings.Contains(plain.writes[0].value.(string), "DDS=Wert,DDID=1") {
		t.Errorf("a plain writer: %v", plain.writes)
	}
	r.e.o.Writer = nil
	_ = r.e.RunProgramBy("p", model.RunThen, "user")
	runs := r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; s.What != StepSkipped || s.Reason != ReasonNoWriter || s.String() != "display HmIP-RF/W:3: Wert {value} BELL | {var:text} ü | sound EVENT: dropped, nothing sends device writes" {
		t.Errorf("no writer: %q", s.String())
	}
}
