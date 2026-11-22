package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const commands = `name: Melden
rules:
  - when:
      all:
        - all:
            - variable: v
              is: true
              trigger: change
    then:
      - command:
          - "/usr/bin/curl"
          - "-s"
          - "http://h/?v={var:v}&k={secret:key}"
          - device: HmIP-RF/000A1B2C3D4E01:1/LEVEL
            format: ".1"
          - variable: modus
            format: "index"
          - variable: v
        timeout: 30s
        wait: true
        output: antwort
        exit_code: code
        after: 5s
      - command: "wget -q -O - 'http://h/' | grep -c {var:v}"
        shell: true
      - command: |-
          echo "$1"
          echo "$2" > /tmp/x
        shell: true
        args:
          - "eins {value}"
          - variable: v
            format: "01"
        note: zwei Zeilen
      - command:
          - "true"
`

// A command is written and read back the same, says what it runs, what it reads and writes.
func TestCommandRoundTrip(t *testing.T) {
	p, err := DecodeProgram("melden.yaml", "melden", []byte(commands))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(EncodeProgram(p, nil)); got != commands {
		t.Fatalf("written:\n%s", got)
	}
	names := func(r Ref) string { return "Dimmer" }
	if got := string(EncodeProgram(p, names)); !strings.Contains(got, "device: HmIP-RF/000A1B2C3D4E01:1/LEVEL  # Dimmer\n") {
		t.Errorf("the device's name is not written:\n%s", got)
	}
	then := p.Rules[0].Then
	c := then[0].Command
	if then[0].Kind != ActionCommand || c.Shell || len(c.Args) != 6 || c.Timeout != Duration(30*time.Second) || !c.Wait || c.Output != "antwort" || c.ExitCode != "code" || then[0].After != Duration(5*time.Second) {
		t.Errorf("command %+v", then[0])
	}
	if got := c.Placeholders(); !reflect.DeepEqual(got, []string{"var:v", "secret:key"}) {
		t.Errorf("placeholders %v", got)
	}
	if got := c.Variables(); !reflect.DeepEqual(got, []string{"v", "modus", "antwort", "code"}) {
		t.Errorf("variables %v", got)
	}
	if got := c.Devices(); len(got) != 1 || got[0].String() != "HmIP-RF/000A1B2C3D4E01:1/LEVEL" {
		t.Errorf("devices %v", got)
	}
	if c.Limit() != 30*time.Second || then[1].Command.Limit() != 10*time.Second {
		t.Errorf("limits %v %v", c.Limit(), then[1].Command.Limit())
	}
	sh := then[2].Command
	if !sh.Shell || sh.Line != "echo \"$1\"\necho \"$2\" > /tmp/x" || len(sh.Args) != 2 || sh.Args[1].Format != FormatBinary || then[2].Note != "zwei Zeilen" {
		t.Errorf("shell %+v", sh)
	}
	for i, want := range []string{"cmd:/usr/bin/curl", "cmd:wget -q -O - 'http://h/' | grep -c {var:v}", "cmd:" + sh.Line, "cmd:true"} {
		if got := then[i].Target(); got != want {
			t.Errorf("target %d: %s", i, got)
		}
	}
	if (Action{Kind: ActionCommand}).Target() != "" || (&Command{}).Program() != "" {
		t.Error("a target without a command")
	}
	// a copy shares nothing
	cp := p.Clone()
	cp.Rules[0].Then[0].Command.Args[3].Device.Param = "X"
	cp.Rules[0].Then[0].Command.Args[0].Text = "x"
	if c.Args[3].Device.Param != "LEVEL" || c.Args[0].Text == "x" {
		t.Error("the copy shares the command")
	}
	bare := &Program{Key: "p", Name: "P", Rules: []Rule{{When: p.Rules[0].When, Then: []Action{{Kind: ActionCommand}}}}}
	if _, err := bare.Check(); err == nil {
		t.Error("a command without its program is taken")
	}

	// the set: every variable must exist and fit
	s := NewSet()
	s.Programs["melden"] = p
	if got := s.ProgramProblems(p); len(got) != 7 {
		t.Errorf("problems %v", got)
	}
	s.Variables["v"] = &Variable{Key: "v", Name: "V", Type: VarBool}
	s.Variables["modus"] = &Variable{Key: "modus", Name: "M", Type: VarNumber}
	s.Variables["antwort"] = &Variable{Key: "antwort", Name: "A", Type: VarAlarm}
	s.Variables["code"] = &Variable{Key: "code", Name: "C", Type: VarString}
	got := strings.Join(s.ProgramProblems(p), "\n")
	for _, want := range []string{"argument 5 has the format index, and modus is no value list", "output goes into a text, a number or a logic value, and antwort is none", "exit code goes into a number, and code is none"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
	s.Variables["modus"] = &Variable{Key: "modus", Name: "M", Type: VarList, Entries: []string{"a", "b"}}
	s.Variables["antwort"] = &Variable{Key: "antwort", Name: "A", Type: VarString}
	s.Variables["code"] = &Variable{Key: "code", Name: "C", Type: VarNumber}
	if got := s.ProgramProblems(p); len(got) != 0 {
		t.Errorf("problems %v", got)
	}
	s.Variables["code"].Computed = &Computed{}
	if got := s.ProgramProblems(p); len(got) != 1 {
		t.Errorf("a computed value as the exit code: %v", got)
	}
	p.Rules[0].When = nil
	for _, key := range []string{"v", "modus", "antwort", "code"} {
		if got := s.VariableUsers(key); len(got) != 1 {
			t.Errorf("users of %s: %v", key, got)
		}
	}
	if got := s.VariableUsers("w"); len(got) != 0 {
		t.Errorf("users of w %v", got)
	}
}

func TestCommandErrors(t *testing.T) {
	list := func(entries ...string) string {
		out := "      - command:\n"
		for _, e := range entries {
			out += "          - " + e + "\n"
		}
		return out
	}
	for _, c := range []struct{ name, action, msg string }{
		{"no program", "      - command: []\n", "the first entry is the program"},
		{"empty program", list(`" "`), "the first entry is the program"},
		{"a value as the program", list("variable: v"), "the first entry is the program"},
		{"a placeholder as the program", list(`"/bin/{var:v}"`), "the first entry is the program"},
		{"a line without a shell", "      - command: \"ls -l\"\n", "must be a list"},
		{"a list with a shell", list(`"ls"`) + "        shell: true\n", "must be a single value"},
		{"an empty line", "      - command: \" \"\n        shell: true\n", "with a shell it is the line"},
		{"a long line", "      - command: \"" + strings.Repeat("x", MaxCommandLine+1) + "\"\n        shell: true\n", "with a shell it is the line"},
		{"shell false", list(`"ls"`) + "        shell: false\n", "shell: true, or left out"},
		{"shell not a bool", list(`"ls"`) + "        shell: ja\n", "true or false"},
		{"args without a shell", list(`"ls"`) + "        args:\n          - \"x\"\n", "only a command with \"shell: true\" has \"args\""},
		{"args not a list", "      - command: \"ls\"\n        shell: true\n        args: x\n", "must be a list"},
		{"a number as an argument", list(`"ls"`, "5"), "put 5 in quotes"},
		{"an argument's key", list(`"ls"`, "wert: x"), "unknown key"},
		{"an empty mapping", list(`"ls"`, "format: \".1\""), "an argument is a text, or the value"},
		{"both values", list(`"ls"`, "variable: v\n            device: A/B:1/C"), "either a variable or a device"},
		{"a device", list(`"ls"`, "device: nix"), "is not a datapoint"},
		{"a variable's key", list(`"ls"`, "variable: \"A B\""), "is not a variable's key"},
		{"a variable not text", list(`"ls"`, "variable: 5"), "must be text"},
		{"a format not text", list(`"ls"`, "variable: v\n            format: 1"), "must be text"},
		{"a format", list(`"ls"`, "variable: v\n            format: \".9\""), "the format \".9\" is not one of"},
		{"index of a device", list(`"ls"`, "device: A/B:1/C\n            format: index"), "is for a value list variable"},
		{"a long argument", list(`"ls"`, `"`+strings.Repeat("x", MaxArg+1)+`"`), "argument 2 is longer"},
		{"too many", list(append([]string{`"ls"`}, strings.Split(strings.Repeat(`"x",`, MaxArgs), ",")[:MaxArgs]...)...), "at most 64 arguments"},
		{"timeout zero", list(`"ls"`) + "        timeout: 0s\n", "timeout: from 1s to 10min"},
		{"timeout long", list(`"ls"`) + "        timeout: 11min\n", "timeout: from 1s to 10min"},
		{"timeout", list(`"ls"`) + "        timeout: bald\n", "is not a duration"},
		{"wait", list(`"ls"`) + "        wait: ja\n", "true or false"},
		{"output", list(`"ls"`) + "        output: \"A B\"\n", "is not a variable's key"},
		{"output not text", list(`"ls"`) + "        output: 5\n", "must be text"},
		{"exit code", list(`"ls"`) + "        exit_code: \"A B\"\n", "is not a variable's key"},
		{"exit code not text", list(`"ls"`) + "        exit_code: 5\n", "must be text"},
		{"the same variable", list(`"ls"`) + "        output: a\n        exit_code: a\n", "name the same variable"},
		{"wait elsewhere", "      - variable: v\n        to: true\n        wait: true\n", "only a \"command\" activity has \"wait\""},
		{"two kinds", list(`"ls"`) + "        wake: \"AA:BB:CC:00:11:22\"\n", "exactly one of"},
		{"to on a command", list(`"ls"`) + "        to: true\n", "only \"set\" and \"variable\""},
	} {
		_, err := DecodeProgram("p.yaml", "p", []byte(prog("", c.action)))
		var e *Error
		if !errors.As(err, &e) || !strings.Contains(e.Msg, c.msg) || e.Line == 0 {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// what the codec cannot write wrong, checked all the same
	for _, c := range []struct {
		cmd Command
		msg string
	}{
		{Command{Line: "x", Args: []CommandArg{{Text: "ls"}}}, "without a shell it is a list"},
		{Command{Args: []CommandArg{{Text: "ls"}, {Text: "x", Variable: "v"}}}, "either a text or a value"},
		{Command{Args: []CommandArg{{Text: "ls"}, {Text: "x", Format: ".1"}}}, "only a variable's or a device's value has a format"},
		{Command{Args: []CommandArg{{Text: "ls"}}, Timeout: Duration(1500 * time.Millisecond)}, "timeout"},
	} {
		if got := c.cmd.Check(); !strings.Contains(got, c.msg) {
			t.Errorf("%+v: %s", c.cmd, got)
		}
	}
}

func TestFormatValue(t *testing.T) {
	entries := []string{"Sommer", "Winter"}
	for _, c := range []struct {
		v      any
		format string
		want   string
	}{
		{nil, "", ""}, {true, "", "true"}, {false, "", "false"}, {true, "01", "1"}, {false, "01", "0"},
		{21.5, "", "21.5"}, {21.0, "", "21"}, {21.55, ".1", "21.6"}, {21.0, ".2", "21.00"}, {0.4, ".0", "0"},
		{"Winter", "", "Winter"}, {"Winter", "index", "1"}, {"Herbst", "index", "Herbst"}, {"a b; rm -rf /", "", "a b; rm -rf /"},
		{7, "", "7"},
	} {
		if got := FormatValue(c.v, c.format, entries); got != c.want {
			t.Errorf("%v as %q: %q", c.v, c.format, got)
		}
	}
	for in, want := range map[string]string{"": "''", "a b": "'a b'", "it's": `'it'\''s'`, "$(reboot); `x`": "'$(reboot); `x`'"} {
		if got := ShellQuote(in); got != want {
			t.Errorf("quoted %q: %s", in, got)
		}
	}
}

func TestParseOutput(t *testing.T) {
	long := strings.Repeat("ä", MaxOutput)
	for _, c := range []struct {
		t    VarType
		out  string
		want any
		ok   bool
	}{
		{VarString, "  hallo welt\n", "hallo welt", true},
		{VarString, "", "", true},
		{VarString, "a\xffb", "a?b", true},
		{VarString, long, long[:MaxOutput], true},
		{VarNumber, " 21.5\n", 21.5, true},
		{VarNumber, "-3", -3.0, true},
		{VarNumber, "1e3", 1000.0, true},
		{VarNumber, "21,5", nil, false},
		{VarNumber, "NaN", nil, false},
		{VarNumber, "1e999", nil, false},
		{VarNumber, "", nil, false},
		{VarBool, "true\n", true, true},
		{VarBool, "FALSE", false, true},
		{VarBool, "1", true, true},
		{VarBool, "0", false, true},
		{VarBool, "ja", nil, false},
		{VarList, "a", nil, false},
	} {
		got, ok := ParseOutput(c.t, c.out)
		if ok != c.ok || got != c.want {
			t.Errorf("%s from %q: %v %v", c.t, c.out, got, ok)
		}
	}
}
