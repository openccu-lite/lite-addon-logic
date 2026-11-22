package engine

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const commanding = `rules:
  - when:
      all:
        - all:
            - variable: x
              op: gt
              value: 10
              trigger: change
    then:
      - command:
          - "/usr/bin/tool"
          - "--text={var:text} {secret:key}"
          - variable: modus
            format: "index"
          - variable: regen
            format: "01"
          - device: HmIP-RF/A:1/LEVEL
            format: ".1"
          - variable: text
        wait: true
        output: out
        exit_code: y
      - variable: offen
        to: 1
      - variable: offen
        to: 2
        after: 10s
      - command: "echo {var:text} {value} {secret:key} > \"$1\""
        shell: true
        args:
          - variable: text
  - else:
      - command:
          - "bg"
        output: temp
      - variable: offen
        to: 5
`

// A command goes to the caller with its values as single arguments; the block waits for it
// when it says so, and the engine does not; what it brought goes into the variables.
func TestCommands(t *testing.T) {
	r := newPrig(t)
	c := &fakeCaller{}
	r.e.o.Caller = c
	r.set("text", "a b; reboot", FromUser)
	r.set("modus", "Winter", FromUser)
	r.set("regen", true, FromUser)
	r.dev("HmIP-RF/A:1/LEVEL", 0.26)
	r.add("p", commanding)
	if refs := r.e.Refs(); len(refs) != 1 || refs[0].String() != "HmIP-RF/A:1/LEVEL" {
		t.Errorf("refs %v", refs)
	}
	r.set("x", 11, FromUser)
	if len(c.calls) != 1 {
		t.Fatalf("calls %+v", c.calls)
	}
	if got := c.calls[0].Args; !reflect.DeepEqual(got, []string{"", "", "2", "1", "0.3", "a b; reboot"}) {
		t.Errorf("arguments %q", got)
	}
	if got := c.calls[0].Values; !reflect.DeepEqual(got, map[string]string{"var:text": "a b; reboot"}) {
		t.Errorf("values %v", got)
	}
	run := r.e.Runs("p")[0]
	if len(run.Steps) != 1 || run.Steps[0].What != StepCalled || run.Steps[0].Value != "/usr/bin/tool '--text=a b; reboot {secret:key}' 2 1 0.3 'a b; reboot'" {
		t.Fatalf("the block did not wait: %+v %v", run.Steps, run.Steps[0].Value)
	}
	if r.value("offen") != 0.0 {
		t.Error("the next activity ran before the command ended")
	}
	// another program runs meanwhile
	r.add("q", "rules:\n  - when:\n      all:\n        - all:\n            - variable: regen\n              is: false\n              trigger: change\n    then:\n      - variable: bis9\n        to: 3\n")
	r.set("regen", false, FromUser)
	if r.value("bis9") != 3.0 {
		t.Error("another program waited for the command")
	}
	// the command ends: its output and exit code are written, the block goes on, its delay
	// counts from now
	r.clock.Advance(7 * time.Second)
	c.done[0](CallResult{Exit: 0, Took: 7 * time.Second, Output: "  fertig\n"})
	if r.value("out") != "fertig" || r.value("y") != 0.0 || r.value("offen") != 1.0 {
		t.Errorf("after the command: out %v, y %v, offen %v", r.value("out"), r.value("y"), r.value("offen"))
	}
	run = r.e.Runs("p")[0]
	if s := run.Steps[0]; !s.Confirmed.Equal(t0.Add(7*time.Second)) || s.Took != 7000 || s.Output != "fertig" || s.Status != 0 || s.Failed != "" {
		t.Errorf("step %+v", s)
	}
	if len(run.Steps) != 4 || run.Steps[2].What != StepScheduled || !run.Steps[2].Due.Equal(t0.Add(17*time.Second)) {
		t.Fatalf("steps %+v", run.Steps)
	}
	// the shell's line: the placeholders quoted, the secret kept out of the history
	if got := run.Steps[3].Value; got != `echo 'a b; reboot' '11' {secret:key} > "$1" 'a b; reboot'` {
		t.Errorf("shell step: %s", got)
	}
	if got := c.calls[1]; !reflect.DeepEqual(got.Args, []string{"a b; reboot"}) || got.Values["value"] != "11" {
		t.Errorf("shell call %+v", got)
	}
	if got := strings.Join(run.Notes(), "\n"); !strings.HasPrefix(got, "1: command /usr/bin/tool\n2: variable offen = 1\n3: variable offen in 17s\n4: command echo") {
		t.Errorf("notes:\n%s", got)
	}
	r.clock.Advance(10 * time.Second)
	if r.value("offen") != 2.0 {
		t.Error("the delayed activity after the command did not run")
	}

	// SONST: in the background - the block goes on at once; a failure is said, the exit code
	// is not asked for, the output is not written
	r.set("x", 0, FromUser)
	if r.value("offen") != 5.0 || len(c.calls) != 3 {
		t.Fatalf("background: offen %v, %d calls", r.value("offen"), len(c.calls))
	}
	c.done[2](CallResult{Exit: 3, Took: time.Second, Output: "42", Stderr: "kaputt\n", Err: errors.New("exit code 3")})
	runs := r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; s.Failed != "exit code 3: kaputt" || s.Status != 3 || s.Output != "42" || r.value("temp") != 0.0 {
		t.Errorf("failed step %+v", s)
	}
	if in, _ := r.e.Program("p"); in.Failures != 1 || in.LastFailure != "command bg: exit code 3: kaputt" {
		t.Errorf("program %+v", in)
	}
	// an output the variable does not take, and one beyond its limits a program may write
	r.set("x", 11, FromUser)
	r.set("x", 0, FromUser)
	c.done[len(c.done)-1](CallResult{Output: "viel"})
	runs = r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; s.Detail != `temp does not take the output "viel"` {
		t.Errorf("refused output %+v", s)
	}
	// a failed command the block waited for: the exit code is written, the output is not, the
	// block goes on
	r.set("x", 12, FromUser)
	n := len(c.done)
	r.set("offen", 0, FromUser)
	c.done[n-1](CallResult{Exit: CommandFailed, Err: errors.New("ended after 10s, its timeout")})
	if r.value("y") != -1.0 || r.value("out") != "fertig" || r.value("offen") != 1.0 {
		t.Errorf("after the timeout: y %v, out %v, offen %v", r.value("y"), r.value("out"), r.value("offen"))
	}
	// long output is cut for the history
	r.set("x", 0, FromUser)
	r.set("x", 13, FromUser)
	c.done[len(c.done)-1](CallResult{Output: strings.Repeat("ä", 400)})
	runs = r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; len(s.Output) > OutputKept+3 || !strings.HasSuffix(s.Output, "…") {
		t.Errorf("kept %d bytes", len(s.Output))
	}

	// the queue is full: said so, and the block goes on at once
	r.set("x", 0, FromUser)
	c.full = true
	r.set("offen", 0, FromUser)
	r.set("x", 14, FromUser)
	runs = r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; s.What != StepSkipped || s.Reason != ReasonQueueFull || r.value("offen") != 1.0 {
		t.Errorf("full: %+v", s)
	}
	c.full = false
	// the program is gone when the command ends: the rest of the block does not run
	r.set("x", 0, FromUser)
	r.set("offen", 0, FromUser)
	r.set("x", 15, FromUser)
	delete(r.progs, "p")
	r.e.SetPrograms(r.progs)
	c.done[len(c.done)-1](CallResult{})
	if r.value("offen") != 0.0 {
		t.Error("the block of a removed program went on")
	}
}

// A value that is not known skips the command; without a caller it is skipped; a dry run
// notes it and goes on; a delayed one keeps the trigger's value; the variables that are gone.
func TestCommandsNotRun(t *testing.T) {
	r := newPrig(t)
	r.set("text", "t", FromUser)
	r.add("p", commanding)
	r.set("x", 11, FromUser)
	run := r.e.Runs("p")[0]
	if s := run.Steps[0]; s.What != StepSkipped || s.Reason != ReasonUnknownValue || s.Detail != "HmIP-RF/A:1/LEVEL" || len(run.Steps) != 4 {
		t.Errorf("unknown value: %+v", run.Steps)
	}
	r.dev("HmIP-RF/A:1/LEVEL", 1.0)
	r.set("x", 0, FromUser)
	r.set("x", 11, FromUser)
	runs := r.e.Runs("p")
	if s := runs[len(runs)-1].Steps[0]; s.What != StepSkipped || s.Reason != ReasonNoWriter || !strings.Contains(s.String(), "nothing sends calls") {
		t.Errorf("no caller: %+v", s)
	}
	c := &fakeCaller{}
	r.e.o.Caller = c
	dry, err := r.e.DryRun("p")
	if err != nil || len(dry.Steps) != 4 || dry.Steps[0].What != StepCalled || len(c.calls) != 0 {
		t.Errorf("dry run: %+v, %v", dry.Steps, err)
	}

	r.add("d", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: gt\n              value: 100\n              trigger: change\n    then:\n      - command:\n          - \"echo\"\n          - \"{value}\"\n          - variable: text\n        output: out\n        after: 5s\n        wait: true\n      - variable: offen\n        to: 9\n")
	r.set("x", 101, FromUser)
	if p := r.e.PendingActions(); len(p) != 3 || p[0].Program != "d" || p[0].Value != 101.0 {
		t.Fatalf("pending %+v", p)
	}
	if r.value("offen") != 9.0 {
		t.Error("a delayed command held the block up")
	}
	r.clock.Advance(5 * time.Second)
	if len(c.calls) != 1 || c.calls[0].Values["value"] != "101" {
		t.Errorf("the delayed command: %+v", c.calls)
	}
	// a variable that is gone
	r.e.mu.Lock()
	run = Run{Program: "p"}
	r.e.command(&run, &Step{}, &model.Action{Kind: model.ActionCommand, Command: &model.Command{Args: []model.CommandArg{{Text: "x"}, {Variable: "weg"}}}}, nil)
	r.e.mu.Unlock()
	if s := run.Steps[0]; s.What != StepSkipped || s.Reason != ReasonNoVariable || s.Detail != "weg" {
		t.Errorf("a variable that is gone: %+v", s)
	}
}

// The variables a command writes are gone or read-only when it ends.
func TestCommandResultRefused(t *testing.T) {
	r := newPrig(t)
	c := &fakeCaller{}
	r.e.o.Caller = c
	r.e.mu.Lock()
	run := &Run{Program: "p"}
	step := &Step{}
	r.e.command(run, step, &model.Action{Kind: model.ActionCommand, Command: &model.Command{Args: []model.CommandArg{{Text: "x"}}, Output: "weg", ExitCode: "sun-elevation"}}, nil)
	r.e.mu.Unlock()
	c.done[0](CallResult{Output: "1"})
	if got := run.Steps[0].Detail; got != "sun-elevation is a system value and cannot be written; there is no variable weg" {
		t.Errorf("detail: %s", got)
	}
	r.e.mu.Lock()
	r.e.command(run, step, &model.Action{Kind: model.ActionCommand, Command: &model.Command{Args: []model.CommandArg{{Text: "x"}}, Output: "ab5"}}, nil)
	r.e.mu.Unlock()
	r.e.vars["ab5"].def.Computed = &model.Computed{}
	c.done[1](CallResult{Output: "1"})
	if got := run.Steps[1].Detail; !strings.Contains(got, "computed value") {
		t.Errorf("detail: %s", got)
	}
}
