package regadom

import (
	"regexp"
	"strconv"
	"strings"
)

// The two one-line scripts users write because the WebUI has no such activity: run a program,
// switch a program on or off. They are recognised by their shape; nothing is interpreted.
var (
	scriptTarget = `dom\s*\.\s*GetObject\s*\(\s*(?:"([^"\\]*)"|'([^'\\]*)'|([0-9]+))\s*\)`
	scriptCall   = `\s*\.\s*(?:(ProgramExecute)\s*\(\s*\)|Active\s*\(\s*(true|false)\s*\))\s*;?\s*$`
	scriptDirect = regexp.MustCompile(`^\s*(\(\s*)?` + scriptTarget + `(\s*\))?` + scriptCall)
	scriptViaVar = regexp.MustCompile(`^\s*(?:var|object)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*` + scriptTarget + `\s*;\s*([A-Za-z_][A-Za-z0-9_]*)` + scriptCall)
)

// What a one-line script does with the program it names.
const (
	CallRun        = "run"
	CallActivate   = "activate"
	CallDeactivate = "deactivate"
)

// OneLiner recognises the two scripts: the program named (by name or by id), and what is done
// with it.
func OneLiner(script string) (byName string, byID int, call string, ok bool) {
	var g []string
	if d := scriptDirect.FindStringSubmatch(script); d != nil && (d[1] == "") == (d[5] == "") {
		g = []string{d[2], d[3], d[4], d[6], d[7]}
	} else if v := scriptViaVar.FindStringSubmatch(script); v != nil && v[1] == v[5] {
		g = []string{v[2], v[3], v[4], v[6], v[7]}
	} else {
		return "", 0, "", false
	}
	byName = g[0] + g[1]
	if g[2] != "" {
		byID, _ = strconv.Atoi(g[2])
	}
	switch {
	case g[3] != "":
		call = CallRun
	case g[4] == "true":
		call = CallActivate
	default:
		call = CallDeactivate
	}
	return byName, byID, call, true
}

// ExecCall is a script that does nothing but run one command with system.Exec - and, at most,
// write what the command printed into a system variable.
type ExecCall struct {
	// Command is the command line as the script hands it to the shell.
	Command string
	// Output says the script writes the command's output into a variable: OutName its name,
	// or OutID its id.
	Output  bool
	OutName string
	OutID   int
}

var (
	scriptIdent = `[A-Za-z_][A-Za-z0-9_]*`
	// declarations before the call: "string out; string err;", "var out;", also with a value
	scriptDecl = regexp.MustCompile(`^\s*(?:string|var|integer|boolean|real|object)\s+` + scriptIdent + `(?:\s*=\s*(?:""|''|0))?\s*;`)
	// a string literal: double or single quotes, a backslash before what would end it
	scriptString = `"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'`
	scriptExec   = regexp.MustCompile(`^\s*(?:(?:var\s+)?` + scriptIdent + `\s*=\s*)?system\s*\.\s*Exec\s*\(\s*(?:` + scriptString + `)\s*(?:,\s*&\s*(` + scriptIdent + `)\s*(?:,\s*&\s*(` + scriptIdent + `)\s*)?)?\)\s*;?`)
	scriptState  = regexp.MustCompile(`^\s*(\(\s*)?` + scriptTarget + `(\s*\))?\s*\.\s*State\s*\(\s*(` + scriptIdent + `)\s*\)\s*;?\s*$`)
	scriptRemark = regexp.MustCompile(`(?m)^\s*!.*$`)
)

// unescape reads a string literal's inside as ReGa does.
func unescape(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b = append(b, s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b = append(b, '\n')
		case 't':
			b = append(b, '\t')
		case 'r':
			b = append(b, '\r')
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}

// Exec recognises a script that is one system.Exec: the call alone, or with the variables for
// its output declared before it and the output written into a system variable after it.
// Nothing is interpreted; a script that does anything else - a second call, a condition, a
// command put together from parts - is not recognised.
func Exec(script string) (ExecCall, bool) {
	rest := scriptRemark.ReplaceAllString(script, "")
	for {
		m := scriptDecl.FindString(rest)
		if m == "" {
			break
		}
		rest = rest[len(m):]
	}
	m := scriptExec.FindStringSubmatch(rest)
	if m == nil {
		return ExecCall{}, false
	}
	call := ExecCall{Command: unescape(m[1] + m[2])}
	rest = rest[len(m[0]):]
	if call.Command == "" {
		return ExecCall{}, false
	}
	if strings.TrimSpace(rest) == "" {
		return call, true
	}
	s := scriptState.FindStringSubmatch(rest)
	// the output is the first of the call's two variables; the script must write that one
	if s == nil || (s[1] == "") != (s[5] == "") || m[3] == "" || s[6] != m[3] {
		return ExecCall{}, false
	}
	call.Output, call.OutName = true, s[2]+s[3]
	if s[4] != "" {
		call.OutID, _ = strconv.Atoi(s[4])
	}
	return call, true
}
