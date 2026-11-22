package outbound

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

func text(args ...string) []model.CommandArg {
	out := make([]model.CommandArg, len(args))
	for i, a := range args {
		out[i] = model.CommandArg{Text: a}
	}
	return out
}

// run carries one command out through the pool and waits for what became of it.
func run(t *testing.T, s *Sender, c engine.Call) engine.CallResult {
	t.Helper()
	done := make(chan engine.CallResult, 1)
	if !s.Call(c, func(r engine.CallResult) { done <- r }) {
		t.Fatal("the queue is full")
	}
	select {
	case r := <-done:
		return r
	case <-time.After(20 * time.Second):
		t.Fatal("the command did not end")
	}
	return engine.CallResult{}
}

func command(c *model.Command) engine.Call {
	return engine.Call{Action: model.Action{Kind: model.ActionCommand, Command: c}}
}

// A command without a shell gets its arguments as they are: nothing in a value is a second
// argument or a shell's word.
func TestCommandArguments(t *testing.T) {
	dir := t.TempDir()
	s := &Sender{Dir: dir, Env: []string{"EXTRA=ja"}, Secret: func(name string) (string, bool) { return "s3 cret", name == "key" }}
	defer s.Close()
	evil := "a b; touch " + filepath.Join(dir, "pwned") + " $(touch " + filepath.Join(dir, "pwned2") + ")"
	c := command(&model.Command{Args: []model.CommandArg{{Text: "/bin/sh"}, {Text: "-c"}, {Text: `printf '%s|' "$#" "$@"; pwd; echo "$HOME $EXTRA $LANG $PATH"`}, {Text: "sh"},
		{Text: "v={var:v} k={secret:key}"}, {Variable: "v"}, {Text: ""}}})
	c.Values = map[string]string{"var:v": evil}
	c.Args = []string{"", "", "", "", "", evil, ""}
	res := run(t, s, c)
	want := "3|v=" + evil + " k=s3 cret|" + evil + "||" + dir + "\n" + dir + " ja C " + Path + "\n"
	if res.Err != nil || res.Exit != 0 || res.Output != want {
		t.Errorf("%+v\nwant %q", res, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Error("a value was run")
	}
	// a bare name is looked up on the commands' own path
	if res := run(t, s, command(&model.Command{Args: text("true")})); res.Err != nil || res.Exit != 0 {
		t.Errorf("true: %+v", res)
	}
}

// With a shell the placeholders are one word each, and the arguments are $1, $2, ….
func TestCommandShell(t *testing.T) {
	dir := t.TempDir()
	s := &Sender{Dir: dir, Secret: func(name string) (string, bool) { return "it's", name == "key" }}
	defer s.Close()
	evil := "x'; touch pwned; echo '$(touch pwned2)`touch pwned3`"
	c := command(&model.Command{Shell: true, Line: `printf '%s|' {var:v} {secret:key} "$1" "$#"; echo err >&2; exit 3`, Args: []model.CommandArg{{Variable: "v"}}})
	c.Values = map[string]string{"var:v": evil}
	c.Args = []string{evil}
	res := run(t, s, c)
	if res.Exit != 3 || res.Err == nil || res.Err.Error() != "exit code 3" || res.Output != evil+"|it's|"+evil+"|1|" || res.Stderr != "err\n" {
		t.Errorf("%+v", res)
	}
	if list, _ := os.ReadDir(dir); len(list) != 0 {
		t.Errorf("a value was run: %v", list)
	}
}

func TestCommandFailures(t *testing.T) {
	dir := t.TempDir()
	s := &Sender{Dir: dir}
	defer s.Close()
	script := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	garbage := filepath.Join(dir, "garbage")
	if err := os.WriteFile(garbage, []byte("\x00\x01\x02"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		cmd  *model.Command
		args []string
		msg  string
	}{
		{"no such program", &model.Command{Args: text("/nowhere/tool")}, nil, "there is no program /nowhere/tool"},
		{"no such name", &model.Command{Args: text("no-such-tool-here")}, nil, "there is no program no-such-tool-here"},
		{"not executable", &model.Command{Args: text(script)}, nil, "may not be run"},
		{"no program at all", &model.Command{Args: text(garbage)}, nil, "could not be started"},
		{"a directory", &model.Command{Args: text(dir)}, nil, "may not be run"},
		{"a secret that is not set", &model.Command{Args: text("true", "{secret:nix}")}, nil, "the secret nix is not set"},
		{"a value that is missing", &model.Command{Args: []model.CommandArg{{Text: "true"}, {Variable: "v"}}}, []string{""}, "an argument's value is missing"},
		{"a NUL byte", &model.Command{Args: []model.CommandArg{{Text: "true"}, {Variable: "v"}}}, []string{"", "a\x00b"}, "a NUL byte"},
		{"a signal", &model.Command{Shell: true, Line: "kill -9 $$"}, nil, "ended by a signal"},
	} {
		call := command(c.cmd)
		call.Args = c.args
		res := run(t, s, call)
		if res.Err == nil || !strings.Contains(res.Err.Error(), c.msg) || res.Exit != engine.CommandFailed {
			t.Errorf("%s: %+v", c.name, res)
		}
	}
	// a directory that is not there
	gone := &Sender{Dir: filepath.Join(dir, "gone")}
	defer gone.Close()
	if res := run(t, gone, command(&model.Command{Args: text("true")})); res.Err == nil || !strings.Contains(res.Err.Error(), "is not there") {
		t.Errorf("no directory: %+v", res)
	}
}

// A command is ended at its timeout with everything it started; its output is kept up to a
// limit and read to its end.
func TestCommandTimeoutAndOutput(t *testing.T) {
	dir := t.TempDir()
	s := &Sender{Dir: dir}
	defer s.Close()
	start := time.Now()
	res := run(t, s, command(&model.Command{Shell: true, Line: "echo begun; (sleep 30; touch late) & sleep 30", Timeout: model.Duration(time.Second)}))
	if res.Err == nil || res.Err.Error() != "ended after 1s, its timeout" || res.Exit != engine.CommandFailed || res.Output != "begun\n" || time.Since(start) > 10*time.Second {
		t.Errorf("%+v after %v", res, time.Since(start))
	}
	// a background job that keeps the output open does not hold the command: it is ended
	start = time.Now()
	res = run(t, s, command(&model.Command{Shell: true, Line: "sleep 30 & echo ok"}))
	if res.Err != nil || res.Exit != 0 || res.Output != "ok\n" || time.Since(start) > 10*time.Second {
		t.Errorf("background job: %+v after %v", res, time.Since(start))
	}
	res = run(t, s, command(&model.Command{Shell: true, Line: "head -c 200000 /dev/zero | tr '\\0' x; head -c 10000 /dev/zero | tr '\\0' e >&2"}))
	if res.Err != nil || len(res.Output) != maxStdout || len(res.Stderr) != maxStderr {
		t.Errorf("output %d, stderr %d, %v", len(res.Output), len(res.Stderr), res.Err)
	}
}

// Commands have a queue and workers of their own; a stop ends what runs.
func TestCommandQueueAndStop(t *testing.T) {
	s := &Sender{Dir: t.TempDir(), CommandWorkers: 1, CommandQueue: 1}
	results := make(chan engine.CallResult, 3)
	slow := command(&model.Command{Shell: true, Line: "sleep 30"})
	if !s.Call(slow, func(r engine.CallResult) { results <- r }) {
		t.Fatal("the first was refused")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(s.commands) > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !s.Call(slow, func(r engine.CallResult) { results <- r }) {
		t.Fatal("the second was refused")
	}
	if s.Call(slow, func(r engine.CallResult) { results <- r }) {
		t.Error("the third was taken although the queue is full")
	}
	// a webhook's queue is another one
	if len(s.queue) != 0 {
		t.Error("a command is in the webhooks' queue")
	}
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	s.Close()
	if time.Since(start) > 10*time.Second {
		t.Error("the stop waited for the command")
	}
	select {
	case r := <-results:
		if r.Err == nil || r.Err.Error() != "ended, the addon stops" {
			t.Errorf("stopped: %+v", r)
		}
	default:
		t.Error("the stopped command said nothing")
	}
}
