package outbound

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Commands: programs of the system, run as the user the addon runs as - nothing here asks for
// more, and nothing can. They have workers and a queue of their own, so a command that takes
// its ten minutes never delays a webhook.
const (
	CommandWorkers = 4
	CommandQueue   = 50
	// what is kept of a command's output; the rest is read and dropped
	maxStdout = 64 << 10
	maxStderr = 4 << 10
	// how long a command's pipes are waited for after it was ended
	waitDelay = 2 * time.Second
	// Path is where a command's program is looked for when it is named without a directory.
	Path = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// capped keeps the first max bytes written to it and counts the rest as written.
type capped struct {
	buf bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// argv makes what is run, with the placeholders filled in: a secret that is not set is an
// error, and nothing is run.
func (s *Sender) argv(c engine.Call) (argv []string, err error) {
	cmd := c.Action.Command
	fill := func(name string) string {
		if secret, ok := strings.CutPrefix(name, "secret:"); ok {
			v, found := "", false
			if s.Secret != nil {
				v, found = s.Secret(secret)
			}
			if !found && err == nil {
				err = fmt.Errorf("the secret %s is not set", secret)
			}
			return v
		}
		return c.Values[name]
	}
	if cmd.Shell {
		// the line's placeholders are one word each for the shell; the arguments are its
		// positional parameters and never part of the line
		argv = []string{"/bin/sh", "-c", model.Expand(cmd.Line, fill, model.ShellQuote), "sh"}
	}
	for i, a := range cmd.Args {
		if a.IsValue() {
			if i >= len(c.Args) {
				return nil, errors.New("an argument's value is missing")
			}
			argv = append(argv, c.Args[i])
			continue
		}
		argv = append(argv, model.Expand(a.Text, fill, func(v string) string { return v }))
	}
	for _, a := range argv {
		if strings.ContainsRune(a, 0) {
			return nil, errors.New("an argument holds a NUL byte")
		}
	}
	return argv, err
}

// command runs a command to its end or its timeout. Its whole process group is ended with it:
// what a shell's line started does not stay behind.
func (s *Sender) command(c engine.Call) engine.CallResult {
	res := engine.CallResult{Exit: engine.CommandFailed}
	argv, err := s.argv(c)
	if err != nil {
		res.Err = err
		return res
	}
	if s.Dir != "" {
		if _, err := os.Stat(s.Dir); err != nil {
			res.Err = fmt.Errorf("the commands' directory %s is not there", s.Dir)
			return res
		}
	}
	limit := c.Action.Command.Limit()
	ctx, cancel := context.WithTimeout(s.ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = s.Dir
	cmd.Env = append([]string{"PATH=" + Path, "LANG=C", "HOME=" + or(s.Dir, "/")}, s.Env...)
	stdout, stderr := &capped{max: maxStdout}, &capped{max: maxStderr}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = waitDelay
	if !strings.Contains(argv[0], "/") {
		// exec looks a bare name up on this process' path: look on the command's own instead
		if found := lookPath(argv[0]); found != "" {
			cmd.Path, cmd.Err = found, nil
		}
	}
	err = cmd.Run()
	if cmd.Process != nil {
		// whatever the command left running in its group
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	res.Output, res.Stderr = stdout.buf.String(), stderr.buf.String()
	var exit *exec.ExitError
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.Err = fmt.Errorf("ended after %s, its timeout", model.Duration(limit))
	case s.ctx.Err() != nil:
		res.Err = errors.New("ended, the addon stops")
	case err == nil:
		res.Exit = 0
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		res.Exit = exit.ExitCode()
		res.Err = fmt.Errorf("exit code %d", res.Exit)
	case errors.As(err, &exit):
		res.Err = errors.New("ended by a signal")
	case errors.Is(err, exec.ErrWaitDelay):
		// it ended with 0 and left something holding its output open
		res.Exit = 0
	default:
		res.Err = startError(argv[0], err)
	}
	return res
}

// startError says why a program could not be started, without Go's wording around it.
func startError(name string, err error) error {
	switch {
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, syscall.ENOENT):
		return fmt.Errorf("there is no program %s", name)
	case errors.Is(err, syscall.EACCES):
		return fmt.Errorf("%s may not be run (the addon's rights are all a command has)", name)
	}
	return fmt.Errorf("%s could not be started: %v", name, err)
}

// lookPath finds a program by its bare name on the commands' path ("" when it is not there).
func lookPath(name string) string {
	for _, dir := range strings.Split(Path, ":") {
		if p, err := exec.LookPath(dir + "/" + name); err == nil {
			return p
		}
	}
	return ""
}
