package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// A command: a program of the system, run with the addon's own rights - never more. It is what
// a CCU's system.Exec was, without its two faults: it never holds up another program, and what
// is put into it from a variable or a device is one argument, whatever it says.
//
// Without a shell the command is a list: the program and its arguments, each handed over as it
// is. With one (Shell) it is a line for /bin/sh, the placeholders in it quoted for the shell,
// and Args are its positional parameters ("$1", "$2", …).

// Limits of a command.
const (
	MaxArgs        = 64
	MaxArg         = 4000
	MaxCommandLine = 8000
	// DefaultTimeout is how long a command may run when it says nothing; MaxTimeout the most
	// it can ask for.
	DefaultTimeout = Duration(10 * time.Second)
	MaxTimeout     = Duration(10 * time.Minute)
)

// The ways a value becomes an argument (CommandArg.Format).
const (
	// FormatBinary writes a logic value as 1 or 0 instead of true or false.
	FormatBinary = "01"
	// FormatIndex writes a value list's entry as its number, counted from 0, instead of its name.
	FormatIndex = "index"
	// ".0" to ".6" write a number with that many decimals.
)

// CommandFormats lists the formats an editor offers.
var CommandFormats = []string{"", ".0", ".1", ".2", ".3", ".4", ".5", ".6", FormatBinary, FormatIndex}

// Command is ActionCommand's.
type Command struct {
	// Shell: Line is run by /bin/sh -c.
	Shell bool   `json:"shell,omitempty"`
	Line  string `json:"line,omitempty"`
	// Args are the program and its arguments; with Shell the line's positional parameters.
	Args []CommandArg `json:"args,omitempty"`
	// Timeout is how long it may run before it is ended (0: DefaultTimeout).
	Timeout Duration `json:"timeout,omitempty"`
	// Wait: the block's following activities wait until the command has ended. Other programs
	// never do.
	Wait bool `json:"wait,omitempty"`
	// Output is the variable that takes what the command wrote (a text, a number, a logic
	// value); ExitCode the number variable that takes its exit code.
	Output   string `json:"output,omitempty"`
	ExitCode string `json:"exit_code,omitempty"`
}

// CommandArg is one argument: a text with the placeholders a webhook has, or the value of a
// variable or of a device's datapoint - one argument, never split.
type CommandArg struct {
	Text     string `json:"text,omitempty"`
	Variable string `json:"variable,omitempty"`
	Device   *Ref   `json:"device,omitempty"`
	// Format says how the value is written (the Format… constants; "" as it is).
	Format string `json:"format,omitempty"`
}

// IsValue says whether the argument is a variable's or a datapoint's value.
func (a CommandArg) IsValue() bool { return a.Variable != "" || a.Device != nil }

// Check says what is wrong with a command ("" when nothing is).
func (c *Command) Check() string {
	if len(c.Args) > MaxArgs {
		return fmt.Sprintf("a command has at most %d arguments", MaxArgs)
	}
	if c.Shell {
		if strings.TrimSpace(c.Line) == "" || len(c.Line) > MaxCommandLine || strings.ContainsRune(c.Line, 0) {
			return fmt.Sprintf("command: with a shell it is the line to run, at most %d bytes", MaxCommandLine)
		}
	} else {
		if c.Line != "" {
			return "command: without a shell it is a list - the program, then its arguments"
		}
		if len(c.Args) == 0 || c.Args[0].IsValue() || strings.TrimSpace(c.Args[0].Text) == "" || placeholder.MatchString(c.Args[0].Text) {
			return "command: the first entry is the program to run, written out"
		}
	}
	for i, a := range c.Args {
		switch {
		case a.Variable != "" && a.Device != nil:
			return fmt.Sprintf("argument %d: either a variable or a device", i+1)
		case a.IsValue() && a.Text != "":
			return fmt.Sprintf("argument %d: either a text or a value", i+1)
		case a.Variable != "" && !ValidKey(a.Variable):
			return fmt.Sprintf("argument %d: %q is not a variable's key", i+1, a.Variable)
		case len(a.Text) > MaxArg || strings.ContainsRune(a.Text, 0):
			return fmt.Sprintf("argument %d is longer than %d bytes", i+1, MaxArg)
		case a.Format != "" && !a.IsValue():
			return fmt.Sprintf("argument %d: only a variable's or a device's value has a format", i+1)
		case !validFormat(a.Format):
			return fmt.Sprintf("argument %d: the format %q is not one of .0 to .6, %s and %s", i+1, a.Format, FormatBinary, FormatIndex)
		case a.Format == FormatIndex && a.Device != nil:
			return fmt.Sprintf("argument %d: the format %s is for a value list variable", i+1, FormatIndex)
		}
	}
	if c.Timeout < 0 || c.Timeout > MaxTimeout || time.Duration(c.Timeout)%time.Second != 0 {
		return fmt.Sprintf("timeout: from 1s to %s", MaxTimeout)
	}
	for _, key := range []string{c.Output, c.ExitCode} {
		if key != "" && !ValidKey(key) {
			return fmt.Sprintf("%q is not a variable's key", key)
		}
	}
	if c.Output != "" && c.Output == c.ExitCode {
		return "output and exit_code name the same variable"
	}
	return ""
}

func validFormat(f string) bool {
	for _, known := range CommandFormats {
		if f == known {
			return true
		}
	}
	return false
}

// Limit answers how long the command may run.
func (c *Command) Limit() time.Duration {
	if c.Timeout <= 0 {
		return DefaultTimeout.Std()
	}
	return c.Timeout.Std()
}

// Program answers what is run, for a label: the program's path, or the shell's line.
func (c *Command) Program() string {
	if c.Shell {
		return c.Line
	}
	if len(c.Args) > 0 {
		return c.Args[0].Text
	}
	return ""
}

// texts lists everything of the command that can hold a placeholder.
func (c *Command) texts() []string {
	out := []string{c.Line}
	for _, a := range c.Args {
		out = append(out, a.Text)
	}
	return out
}

// Placeholders lists the placeholders' names in the line and the text arguments, each once, in
// the order they come.
func (c *Command) Placeholders() []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range c.texts() {
		for _, m := range placeholder.FindAllStringSubmatch(t, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// Variables lists every variable the command reads or writes, each once.
func (c *Command) Variables() []string {
	var out []string
	seen := map[string]bool{}
	add := func(key string) {
		if key != "" && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	for _, p := range c.Placeholders() {
		if key, ok := strings.CutPrefix(p, "var:"); ok {
			add(key)
		}
	}
	for _, a := range c.Args {
		add(a.Variable)
	}
	add(c.Output)
	add(c.ExitCode)
	return out
}

// Devices lists the datapoints whose values are arguments.
func (c *Command) Devices() []Ref {
	var out []Ref
	for _, a := range c.Args {
		if a.Device != nil {
			out = append(out, *a.Device)
		}
	}
	return out
}

// uses says whether the command reads or writes a variable.
func (c *Command) uses(variable string) bool {
	for _, key := range c.Variables() {
		if key == variable {
			return true
		}
	}
	return false
}

func (c *Command) clone() *Command {
	out := *c
	out.Args = make([]CommandArg, len(c.Args))
	for i, a := range c.Args {
		if a.Device != nil {
			d := *a.Device
			a.Device = &d
		}
		out.Args[i] = a
	}
	return &out
}

// FormatValue writes a value as an argument: v is a bool, a number or a text, entries the
// value list's when the value is one of its entries. Nothing for no value.
func FormatValue(v any, format string, entries []string) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if format == FormatBinary {
			if x {
				return "1"
			}
			return "0"
		}
		return strconv.FormatBool(x)
	case float64:
		if strings.HasPrefix(format, ".") {
			n, _ := strconv.Atoi(format[1:])
			return strconv.FormatFloat(x, 'f', n, 64)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		if format == FormatIndex {
			for i, e := range entries {
				if e == x {
					return strconv.Itoa(i)
				}
			}
		}
		return x
	}
	return fmt.Sprint(v)
}

// ShellQuote writes s as one word of a shell's line, whatever it holds.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ParseOutput reads what a command wrote as a value of a variable: a text as it is, cut after
// MaxOutput bytes; a number; a logic value from true, false, 1 or 0. The output is trimmed
// first. False when it is not such a value.
func ParseOutput(t VarType, out string) (any, bool) {
	out = strings.TrimSpace(out)
	switch t {
	case VarString:
		if len(out) > MaxOutput {
			out = strings.ToValidUTF8(out[:MaxOutput], "")
		}
		return strings.ToValidUTF8(out, "?"), true
	case VarNumber:
		f, err := strconv.ParseFloat(out, 64)
		if err != nil || f != f || f > 1e300 || f < -1e300 {
			return nil, false
		}
		return f, true
	case VarBool:
		switch strings.ToLower(out) {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
	}
	return nil, false
}

// MaxOutput is the most of a command's output a text variable takes, in bytes.
const MaxOutput = 4000
