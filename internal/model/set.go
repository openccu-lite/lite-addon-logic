package model

import (
	"fmt"
	"sort"
	"strings"
)

// Set is every program and variable together: what the references between them are checked
// against. A program with a problem does not run; everything else does.
type Set struct {
	Programs  map[string]*Program
	Variables map[string]*Variable
}

// The system values: read-only numbers the addon keeps or the system feeds. They exist without
// a file; a program checks them like a number variable and cannot write them.
const (
	// SystemAlarmMessages counts the alarm messages: the alarm variables that are triggered and
	// not acknowledged (the CCU's variable 40).
	SystemAlarmMessages = "alarm-messages"
	// SystemServiceMessages is the number of service messages (the CCU's variable 41) and
	// SystemDutyCycle the duty cycle in percent (OpenCCU's DutyCycle).
	SystemServiceMessages = "service-messages"
	SystemDutyCycle       = "duty-cycle"
	// SystemSunAzimuth and SystemSunElevation are where the sun stands at the system's
	// position, in degrees: the azimuth clockwise from north (90 east, 180 south, 270 west),
	// the elevation above the horizon (negative below it). For shading programs.
	SystemSunAzimuth   = "sun-azimuth"
	SystemSunElevation = "sun-elevation"
)

// SystemValues lists the system values' keys.
var SystemValues = []string{SystemAlarmMessages, SystemServiceMessages, SystemDutyCycle, SystemSunAzimuth, SystemSunElevation}

// IsSystemValue says whether a key is a system value's: no variable can have it.
func IsSystemValue(key string) bool {
	for _, k := range SystemValues {
		if k == key {
			return true
		}
	}
	return false
}

// variable answers the variable of a key as a program's line sees it: the variable, or for a
// system value a read-only number.
func (s *Set) variable(key string) *Variable {
	if IsSystemValue(key) {
		return &Variable{Key: key, Name: key, Type: VarNumber, Visible: true}
	}
	return s.Variables[key]
}

// NewSet makes an empty set.
func NewSet() *Set {
	return &Set{Programs: map[string]*Program{}, Variables: map[string]*Variable{}}
}

// Lines calls fn for every condition line of the program, with the rule's index.
func (p *Program) Lines(fn func(rule int, l *Line)) {
	for ri := range p.Rules {
		if p.Rules[ri].When == nil {
			continue
		}
		for gi := range p.Rules[ri].When.Groups {
			g := &p.Rules[ri].When.Groups[gi]
			for li := range g.Lines {
				fn(ri, &g.Lines[li])
			}
		}
	}
}

// Actions calls fn for every activity of the program, with the rule's index.
func (p *Program) Actions(fn func(rule int, a *Action)) {
	for ri := range p.Rules {
		for ai := range p.Rules[ri].Then {
			fn(ri, &p.Rules[ri].Then[ai])
		}
	}
}

// HasScript says whether the program holds a kept script: such a program cannot be active.
func (p *Program) HasScript() bool {
	found := false
	p.Actions(func(_ int, a *Action) { found = found || a.Kind == ActionScript })
	return found
}

// HasTime says whether the program has a time module line.
func (p *Program) HasTime() bool {
	found := false
	p.Lines(func(_ int, l *Line) { found = found || l.Time != nil })
	return found
}

// compareValue checks a line's or an activity's value against the variable it is for.
func compareValue(v *Variable, op Op, x any) error {
	switch v.Type {
	case VarNumber:
		if !isNumber(x) {
			return fmt.Errorf("the variable %s is a number", v.Key)
		}
		return nil
	case VarBool, VarAlarm, VarList:
		if op != OpEq && op != OpNe {
			return fmt.Errorf("the variable %s (%s) can only be compared for equality", v.Key, v.Type)
		}
	case VarString:
		if op != OpEq && op != OpNe {
			return fmt.Errorf("the variable %s is text and can only be compared for equality", v.Key)
		}
	}
	return v.CheckValue(x)
}

// ProgramProblems lists what is wrong with a program's references: a variable or program that
// does not exist, a value that does not fit its variable, a script on an active program.
func (s *Set) ProgramProblems(p *Program) []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	p.Lines(func(ri int, l *Line) {
		if l.Variable == "" {
			return
		}
		v := s.variable(l.Variable)
		if v == nil {
			add("rule %d: the variable %s does not exist", ri+1, l.Variable)
			return
		}
		switch l.Op {
		case OpRange, OpOutside:
			if v.Type != VarNumber {
				add("rule %d: a range on %s, which is not a number", ri+1, v.Key)
			}
		default:
			if err := compareValue(v, l.Op, l.Value); err != nil {
				add("rule %d: %v", ri+1, err)
			}
		}
	})
	p.Actions(func(ri int, a *Action) {
		switch a.Kind {
		case ActionVariable:
			if IsSystemValue(a.Variable) {
				add("rule %d: %s is a system value and cannot be written", ri+1, a.Variable)
				return
			}
			v := s.Variables[a.Variable]
			if v == nil {
				add("rule %d: the variable %s does not exist", ri+1, a.Variable)
				return
			}
			switch {
			case a.FromVariable != "":
				if s.variable(a.FromVariable) == nil {
					add("rule %d: the variable %s does not exist", ri+1, a.FromVariable)
				}
			case a.FromDevice != nil, a.Special != "":
			default:
				if err := v.CheckValue(a.To); err != nil {
					add("rule %d: %v", ri+1, err)
				}
			}
		case ActionSet:
			if a.FromVariable != "" && s.variable(a.FromVariable) == nil {
				add("rule %d: the variable %s does not exist", ri+1, a.FromVariable)
			}
		case ActionRun, ActionActivate, ActionDeactivate:
			if s.Programs[a.Program] == nil {
				add("rule %d: the program %s does not exist", ri+1, a.Program)
			}
		case ActionWebhook:
			for _, name := range a.Webhook.Placeholders() {
				if key, ok := strings.CutPrefix(name, "var:"); ok && s.variable(key) == nil {
					add("rule %d: the variable %s does not exist", ri+1, key)
				}
			}
		case ActionCommand:
			c := a.Command
			for _, key := range c.Variables() {
				if s.variable(key) == nil {
					add("rule %d: the variable %s does not exist", ri+1, key)
				}
			}
			for i, arg := range c.Args {
				if v := s.variable(arg.Variable); v != nil && arg.Format == FormatIndex && v.Type != VarList {
					add("rule %d: argument %d has the format %s, and %s is no value list", ri+1, i+1, FormatIndex, arg.Variable)
				}
			}
			if v := s.variable(c.Output); v != nil && (v.Computed != nil || (v.Type != VarString && v.Type != VarNumber && v.Type != VarBool)) {
				add("rule %d: a command's output goes into a text, a number or a logic value, and %s is none of them", ri+1, c.Output)
			}
			if v := s.variable(c.ExitCode); v != nil && (v.Computed != nil || v.Type != VarNumber) {
				add("rule %d: a command's exit code goes into a number, and %s is none", ri+1, c.ExitCode)
			}
		case ActionDisplay:
			for _, name := range a.Display.Placeholders() {
				if key, ok := strings.CutPrefix(name, "var:"); ok && s.variable(key) == nil {
					add("rule %d: the variable %s does not exist", ri+1, key)
				}
			}
		case ActionScript:
			if p.Active {
				add("rule %d: a program with a script cannot be active", ri+1)
			}
		}
	})
	return out
}

// Problems lists the problems of every program, by key.
func (s *Set) Problems() map[string][]string {
	out := map[string][]string{}
	for k, p := range s.Programs {
		if pr := s.ProgramProblems(p); len(pr) > 0 {
			out[k] = pr
		}
	}
	return out
}

// VariableUsers lists the programs that use a variable in a condition or an activity, sorted.
func (s *Set) VariableUsers(key string) []string {
	var out []string
	for k, p := range s.Programs {
		used := false
		p.Lines(func(_ int, l *Line) { used = used || l.Variable == key })
		p.Actions(func(_ int, a *Action) {
			used = used || (a.Kind == ActionVariable && a.Variable == key) || a.FromVariable == key || (a.Webhook != nil && a.Webhook.uses(key)) || (a.Display != nil && a.Display.uses(key)) || (a.Command != nil && a.Command.uses(key))
		})
		if used {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// EntryUsers lists the programs that name an entry of a list variable, sorted.
func (s *Set) EntryUsers(key, entry string) []string {
	var out []string
	for k, p := range s.Programs {
		used := false
		p.Lines(func(_ int, l *Line) { used = used || (l.Variable == key && l.Value == any(entry)) })
		p.Actions(func(_ int, a *Action) {
			used = used || (a.Kind == ActionVariable && a.Variable == key && a.To == any(entry))
		})
		if used {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// RenameEntry rewrites, in a copy of each program that names the entry, the old label to the
// new one, and answers the changed programs by key. The set itself is not changed.
func (s *Set) RenameEntry(key, from, to string) map[string]*Program {
	out := map[string]*Program{}
	for _, k := range s.EntryUsers(key, from) {
		c := s.Programs[k].Clone()
		c.Lines(func(_ int, l *Line) {
			if l.Variable == key && l.Value == any(from) {
				l.Value = to
			}
		})
		c.Actions(func(_ int, a *Action) {
			if a.Kind == ActionVariable && a.Variable == key && a.To == any(from) {
				a.To = to
			}
		})
		out[k] = c
	}
	return out
}

// ProgramUsers lists the programs whose activities run or switch the program, sorted.
func (s *Set) ProgramUsers(key string) []string {
	var out []string
	for k, p := range s.Programs {
		used := false
		p.Actions(func(_ int, a *Action) { used = used || a.Program == key })
		if used && k != key {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Clone copies a program deeply.
func (p *Program) Clone() *Program {
	c := *p
	c.Rules = make([]Rule, len(p.Rules))
	for i, r := range p.Rules {
		c.Rules[i] = r
		if r.When != nil {
			w := *r.When
			w.Groups = make([]Group, len(r.When.Groups))
			for gi, g := range r.When.Groups {
				w.Groups[gi] = g
				w.Groups[gi].Lines = make([]Line, len(g.Lines))
				for li, l := range g.Lines {
					if l.Device != nil {
						d := *l.Device
						l.Device = &d
					}
					if l.Time != nil {
						t := *l.Time
						t.Weekdays = append([]string(nil), l.Time.Weekdays...)
						l.Time = &t
					}
					w.Groups[gi].Lines[li] = l
				}
			}
			c.Rules[i].When = &w
		}
		c.Rules[i].Then = make([]Action, len(r.Then))
		for ai, a := range r.Then {
			if a.Device != nil {
				d := *a.Device
				a.Device = &d
			}
			if a.FromDevice != nil {
				d := *a.FromDevice
				a.FromDevice = &d
			}
			a.Values = append([]ParamValue(nil), a.Values...)
			if a.Display != nil {
				d := *a.Display
				d.Lines = append([]DisplayLine(nil), a.Display.Lines...)
				a.Display = &d
			}
			if a.Webhook != nil {
				w := *a.Webhook
				w.Headers = append([]Header(nil), a.Webhook.Headers...)
				a.Webhook = &w
			}
			if a.Wake != nil {
				w := *a.Wake
				a.Wake = &w
			}
			if a.Command != nil {
				a.Command = a.Command.clone()
			}
			c.Rules[i].Then[ai] = a
		}
	}
	return &c
}

// Clone copies a variable deeply.
func (v *Variable) Clone() *Variable {
	c := *v
	if v.Labels != nil {
		l := *v.Labels
		c.Labels = &l
	}
	c.Entries = append([]string(nil), v.Entries...)
	if v.Min != nil {
		m := *v.Min
		c.Min = &m
	}
	if v.Max != nil {
		m := *v.Max
		c.Max = &m
	}
	if v.Computed != nil {
		cc := Computed{Function: v.Computed.Function}
		for _, in := range v.Computed.Inputs {
			if in.Device != nil {
				r := *in.Device
				in.Device = &r
			}
			cc.Inputs = append(cc.Inputs, in)
		}
		c.Computed = &cc
	}
	if v.Helper != nil {
		h := *v.Helper
		c.Helper = &h
	}
	return &c
}
