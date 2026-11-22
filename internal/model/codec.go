package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Namer answers the name to write as a comment after a device reference ("" for none). The
// comments are written at save time and never read back.
type Namer func(Ref) string

// MaxName bounds a name's length (in characters), MaxText a note's or a script's (in bytes).
const (
	MaxName = 200
	MaxText = 64 << 10
)

var (
	programKeys  = []string{"name", "note", "active", "visible", "operable", "internal", "at_start", "rules"}
	ruleKeys     = []string{"when", "else", "note", "cancel_pending", "then"}
	lineKeys     = []string{"device", "variable", "time", "is", "op", "value", "from", "below", "to", "not", "for", "trigger", "note"}
	actionKeys   = []string{"set", "variable", "run", "activate", "deactivate", "script", "webhook", "wake", "channel", "values", "display", "lines", "sound", "repetitions", "interval", "to", "from", "from_variable", "special", "on_for", "mode", "method", "headers", "body_type", "body", "insecure", "broadcast", "command", "shell", "args", "timeout", "wait", "output", "exit_code", "after", "after_up_to", "note"}
	timeKeys     = []string{"kind", "at", "all_day", "astro", "offset", "point", "twilight", "duration", "interval", "every", "weekdays", "day", "nth", "month", "date", "from", "until", "count"}
	variableKeys = []string{"name", "note", "type", "labels", "entries", "unit", "min", "max", "visible", "internal", "logged", "channel", "start_value", "computed", "helper"}
)

// --- programs: writing ---

// EncodeProgram writes a program the one canonical way: fixed key order, two spaces, lists in
// their order, what is the default left out.
func EncodeProgram(p *Program, names Namer) []byte {
	e := &emitter{}
	e.kv(0, false, "name", str(p.Name))
	if p.Note != "" {
		e.text(0, false, "note", p.Note)
	}
	if p.Active {
		e.kv(0, false, "active", "true")
	}
	if !p.Visible {
		e.kv(0, false, "visible", "false")
	}
	if !p.Operable {
		e.kv(0, false, "operable", "false")
	}
	if p.Internal {
		e.kv(0, false, "internal", "true")
	}
	if p.AtStart {
		e.kv(0, false, "at_start", "true")
	}
	if len(p.Rules) == 0 {
		e.kv(0, false, "rules", "[]")
		return []byte(e.b.String())
	}
	e.key(0, false, "rules")
	for i := range p.Rules {
		e.rule(&p.Rules[i], names)
	}
	return []byte(e.b.String())
}

func (e *emitter) rule(r *Rule, names Namer) {
	const in = 4 // the keys of a rule
	actions := "then"
	if r.Else {
		actions = "else"
	} else {
		c := r.When
		if c == nil {
			c = &Condition{Op: All}
		}
		e.key(in, true, "when")
		if len(c.Groups) == 0 {
			e.kv(in+2, false, string(c.Op), "[]")
		} else {
			e.key(in+2, false, string(c.Op))
			for gi := range c.Groups {
				g := &c.Groups[gi]
				if len(g.Lines) == 0 {
					e.kv(in+6, true, string(g.Op), "[]")
					continue
				}
				e.key(in+6, true, string(g.Op))
				for li := range g.Lines {
					e.condLine(in+10, &g.Lines[li], names)
				}
			}
		}
	}
	first := r.Else
	put := func(write func(first bool)) {
		write(first)
		first = false
	}
	if r.Else {
		put(func(f bool) { e.actions(in, f, actions, r.Then, names) })
	}
	if r.Note != "" {
		put(func(f bool) { e.text(in, f, "note", r.Note) })
	}
	if r.CancelPending {
		put(func(f bool) { e.kv(in, f, "cancel_pending", "true") })
	}
	if !r.Else {
		put(func(f bool) { e.actions(in, f, actions, r.Then, names) })
	}
}

func comment(names Namer, r *Ref) string {
	if names == nil || r == nil {
		return ""
	}
	n := strings.Join(strings.Fields(names(*r)), " ")
	if n == "" || !utf8.ValidString(n) {
		return ""
	}
	return "  # " + n
}

func (e *emitter) condLine(in int, l *Line, names Namer) {
	switch {
	case l.Device != nil:
		e.kv(in, true, "device", str(l.Device.String())+comment(names, l.Device))
	case l.Time != nil:
		e.key(in, true, "time")
		e.timeModule(in+2, l.Time)
	default:
		e.kv(in, true, "variable", str(l.Variable))
	}
	if l.Time == nil {
		switch l.Op {
		case OpEq, "":
			e.kv(in, false, "is", scalar(l.Value))
		case OpRange:
			e.kv(in, false, "op", string(l.Op))
			e.kv(in, false, "from", scalar(l.From))
			e.kv(in, false, "below", scalar(l.Below))
		case OpOutside:
			e.kv(in, false, "op", string(l.Op))
			e.kv(in, false, "from", scalar(l.From))
			e.kv(in, false, "to", scalar(l.To))
		default:
			e.kv(in, false, "op", string(l.Op))
			e.kv(in, false, "value", scalar(l.Value))
		}
	}
	if l.Not {
		e.kv(in, false, "not", "true")
	}
	if l.For != 0 {
		e.kv(in, false, "for", l.For.String())
	}
	e.kv(in, false, "trigger", string(l.Trigger))
	if l.Note != "" {
		e.text(in, false, "note", l.Note)
	}
}

func (e *emitter) timeModule(in int, t *TimeModule) {
	e.kv(in, false, "kind", str(t.Kind))
	if t.At != "" {
		e.kv(in, false, "at", quoted(t.At))
	}
	if t.AllDay {
		e.kv(in, false, "all_day", "true")
	}
	if t.Astro != "" {
		e.kv(in, false, "astro", str(t.Astro))
	}
	if t.Offset != 0 {
		e.kv(in, false, "offset", t.Offset.String())
	}
	if t.Point {
		e.kv(in, false, "point", "true")
	}
	if t.Twilight != "" {
		e.kv(in, false, "twilight", str(t.Twilight))
	}
	if t.Duration != 0 {
		e.kv(in, false, "duration", t.Duration.String())
	}
	if t.Interval != 0 {
		e.kv(in, false, "interval", t.Interval.String())
	}
	if t.Every != 0 {
		e.kv(in, false, "every", number(float64(t.Every)))
	}
	if len(t.Weekdays) > 0 {
		e.key(in, false, "weekdays")
		for _, d := range t.Weekdays {
			e.line(in+2, "- "+str(d))
		}
	}
	if t.Day != 0 {
		e.kv(in, false, "day", number(float64(t.Day)))
	}
	if t.Nth != 0 {
		e.kv(in, false, "nth", number(float64(t.Nth)))
	}
	if t.Month != 0 {
		e.kv(in, false, "month", number(float64(t.Month)))
	}
	if t.Date != "" {
		e.kv(in, false, "date", quoted(t.Date))
	}
	if t.From != "" {
		e.kv(in, false, "from", quoted(t.From))
	}
	if t.Until != "" {
		e.kv(in, false, "until", quoted(t.Until))
	}
	if t.Count != 0 {
		e.kv(in, false, "count", number(float64(t.Count)))
	}
}

func (e *emitter) actions(in int, first bool, key string, list []Action, names Namer) {
	if len(list) == 0 {
		e.kv(in, first, key, "[]")
		return
	}
	e.key(in, first, key)
	for i := range list {
		e.action(in+4, &list[i], names)
	}
}

func (e *emitter) action(in int, a *Action, names Namer) {
	switch a.Kind {
	case ActionSet:
		r := a.Device
		if r == nil {
			r = &Ref{}
		}
		e.kv(in, true, "set", str(r.String())+comment(names, a.Device))
	case ActionVariable:
		e.kv(in, true, "variable", str(a.Variable))
	case ActionRun, ActionActivate, ActionDeactivate:
		e.kv(in, true, string(a.Kind), str(a.Program))
	case ActionChannel:
		ref := ChannelRef(a.Channel)
		e.kv(in, true, "channel", str(a.Channel)+comment(names, &ref))
		if len(a.Values) == 0 {
			e.kv(in, false, "values", "{}")
		} else {
			e.key(in, false, "values")
			for _, v := range a.Values {
				e.kv(in+2, false, str(v.Param), scalar(v.Value))
			}
		}
	case ActionDisplay:
		e.display(in, a, names)
	case ActionWebhook:
		e.webhook(in, a.Webhook)
	case ActionWake:
		w := a.Wake
		if w == nil {
			w = &Wake{}
		}
		e.kv(in, true, "wake", quoted(w.MAC))
		if w.Broadcast != "" {
			e.kv(in, false, "broadcast", quoted(w.Broadcast))
		}
	case ActionCommand:
		e.command(in, a.Command, names)
	default:
		e.text(in, true, "script", a.Script)
	}
	if a.Kind == ActionSet || a.Kind == ActionVariable {
		switch {
		case a.FromDevice != nil:
			e.kv(in, false, "from", str(a.FromDevice.String())+comment(names, a.FromDevice))
		case a.FromVariable != "":
			e.kv(in, false, "from_variable", str(a.FromVariable))
		case a.Special != "":
			e.kv(in, false, "special", str(a.Special))
		default:
			e.kv(in, false, "to", scalar(a.To))
		}
	}
	if a.OnFor != 0 {
		e.kv(in, false, "on_for", a.OnFor.String())
	}
	if a.Kind == ActionRun {
		e.kv(in, false, "mode", str(a.Mode))
	}
	if a.After != 0 || a.AfterUpTo != 0 {
		e.kv(in, false, "after", a.After.String())
	}
	if a.AfterUpTo != 0 {
		e.kv(in, false, "after_up_to", a.AfterUpTo.String())
	}
	if a.Note != "" {
		e.text(in, false, "note", a.Note)
	}
}

func (e *emitter) display(in int, a *Action, names Namer) {
	d := a.Display
	if d == nil {
		d = &Display{}
	}
	ref := ChannelRef(a.Channel)
	e.kv(in, true, "display", str(a.Channel)+comment(names, &ref))
	if len(d.Lines) > 0 {
		e.key(in, false, "lines")
	}
	for _, l := range d.Lines {
		if l == (DisplayLine{}) {
			e.line(in+2, "- {}")
			continue
		}
		first := true
		put := func(key, value string) {
			if value != "" {
				e.kv(in+4, first, key, value)
				first = false
			}
		}
		if l.Text != "" {
			put("text", quoted(l.Text))
		}
		for _, kv := range [][2]string{{"icon", l.Icon}, {"align", l.Align}, {"color", l.Color}, {"background", l.Background}} {
			if kv[1] != "" {
				put(kv[0], str(kv[1]))
			}
		}
	}
	if d.Sound != "" {
		e.kv(in, false, "sound", str(d.Sound))
		if d.Repetitions != 0 {
			e.kv(in, false, "repetitions", strconv.Itoa(d.Repetitions))
		}
		if d.Interval != 0 {
			e.kv(in, false, "interval", strconv.Itoa(d.Interval))
		}
	}
}

// command writes a "command" activity: without a shell the program and its arguments as a
// list, with one the line, and the positional parameters as "args".
func (e *emitter) command(in int, c *Command, names Namer) {
	if c == nil {
		c = &Command{}
	}
	args := func(key string, first bool) {
		if len(c.Args) == 0 {
			e.kv(in, first, key, "[]")
			return
		}
		e.key(in, first, key)
		for _, a := range c.Args {
			switch {
			case a.Device != nil:
				e.kv(in+4, true, "device", str(a.Device.String())+comment(names, a.Device))
			case a.Variable != "":
				e.kv(in+4, true, "variable", str(a.Variable))
			default:
				e.line(in+2, "- "+quoted(a.Text))
				continue
			}
			if a.Format != "" {
				e.kv(in+4, false, "format", quoted(a.Format))
			}
		}
	}
	if c.Shell {
		if literal(c.Line) {
			e.text(in, true, "command", c.Line)
		} else {
			e.kv(in, true, "command", quoted(c.Line))
		}
		e.kv(in, false, "shell", "true")
		if len(c.Args) > 0 {
			args("args", false)
		}
	} else {
		args("command", true)
	}
	if c.Timeout != 0 {
		e.kv(in, false, "timeout", c.Timeout.String())
	}
	if c.Wait {
		e.kv(in, false, "wait", "true")
	}
	if c.Output != "" {
		e.kv(in, false, "output", str(c.Output))
	}
	if c.ExitCode != "" {
		e.kv(in, false, "exit_code", str(c.ExitCode))
	}
}

func (e *emitter) webhook(in int, w *Webhook) {
	if w == nil {
		w = &Webhook{}
	}
	e.kv(in, true, "webhook", quoted(w.URL))
	e.kv(in, false, "method", str(w.Method))
	if len(w.Headers) > 0 {
		e.key(in, false, "headers")
		for _, h := range w.Headers {
			e.kv(in+4, true, "name", str(h.Name))
			e.kv(in+4, false, "value", quoted(h.Value))
		}
	}
	if w.BodyType != "" {
		e.kv(in, false, "body_type", str(w.BodyType))
	}
	if w.Body != "" {
		e.text(in, false, "body", w.Body)
	}
	if w.Insecure {
		e.kv(in, false, "insecure", "true")
	}
}

// --- programs: reading ---

func checkName(d *decoder, f *fields, what string) (string, error) {
	name, err := f.str("name")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		return "", d.errf(orNode(f.vals["name"], f.node), "%s needs a name", what)
	}
	if utf8.RuneCountInString(name) > MaxName || strings.ContainsAny(name, "\n\r\t") {
		return "", d.errf(f.vals["name"], "the name is longer than %d characters or has a line break", MaxName)
	}
	return name, nil
}

func orNode(n, fallback *yaml.Node) *yaml.Node {
	if n != nil {
		return n
	}
	return fallback
}

func (f *fields) note() (string, error) {
	s, err := f.str("note")
	if err != nil {
		return "", err
	}
	if len(s) > MaxText {
		return "", f.d.errf(f.vals["note"], "the note is longer than %d bytes", MaxText)
	}
	return s, nil
}

// DecodeProgram reads a program's file. file names it in errors; key becomes the program's.
func DecodeProgram(file, key string, data []byte) (*Program, error) {
	d := &decoder{file: file}
	if !ValidKey(key) {
		return nil, &Error{File: file, Msg: fmt.Sprintf("%q cannot be a program's key (lower-case letters, digits and hyphens, at most 48)", key)}
	}
	root, err := d.document(data)
	if err != nil {
		return nil, err
	}
	f, err := d.mapping(root, "a program")
	if err != nil {
		return nil, err
	}
	p := &Program{Key: key}
	if p.Name, err = checkName(d, f, "a program"); err != nil {
		return nil, err
	}
	if p.Note, err = f.note(); err != nil {
		return nil, err
	}
	for _, b := range []struct {
		key string
		def bool
		to  *bool
	}{{"active", false, &p.Active}, {"visible", true, &p.Visible}, {"operable", true, &p.Operable}, {"internal", false, &p.Internal}, {"at_start", false, &p.AtStart}} {
		if *b.to, err = f.boolean(b.key, b.def); err != nil {
			return nil, err
		}
	}
	if !f.has("rules") {
		return nil, d.errf(root, "a program needs rules (an empty list is \"rules: []\")")
	}
	items, err := f.list("rules")
	if err != nil {
		return nil, err
	}
	if err := f.done("a program", programKeys...); err != nil {
		return nil, err
	}
	p.Rules = make([]Rule, 0, len(items))
	for i, it := range items {
		r, err := d.rule(it)
		if err != nil {
			return nil, err
		}
		if r.Else && i != len(items)-1 {
			return nil, d.errf(it, "the \"else\" rule must be the last one")
		}
		p.Rules = append(p.Rules, *r)
	}
	return p, nil
}

func (d *decoder) rule(n *yaml.Node) (*Rule, error) {
	f, err := d.mapping(n, "a rule")
	if err != nil {
		return nil, err
	}
	r := &Rule{}
	switch {
	case f.has("when") && f.has("else"):
		return nil, d.errf(n, "a rule has \"when\" (with \"then\") or \"else\", not both")
	case f.has("else"):
		if f.has("then") {
			return nil, d.errf(f.vals["then"], "an \"else\" rule lists its activities under \"else\", it has no \"then\"")
		}
		r.Else = true
		if r.Then, err = d.actions(f.get("else"), "else"); err != nil {
			return nil, err
		}
	case f.has("when"):
		if r.When, err = d.condition(f.get("when")); err != nil {
			return nil, err
		}
		if !f.has("then") {
			return nil, d.errf(n, "a rule with \"when\" needs \"then\" (no activity is \"then: []\")")
		}
		if r.Then, err = d.actions(f.get("then"), "then"); err != nil {
			return nil, err
		}
	default:
		return nil, d.errf(n, "a rule needs \"when\" (with \"then\") or \"else\"")
	}
	if r.Note, err = f.note(); err != nil {
		return nil, err
	}
	if r.CancelPending, err = f.boolean("cancel_pending", false); err != nil {
		return nil, err
	}
	if err := f.done("a rule", ruleKeys...); err != nil {
		return nil, err
	}
	return r, nil
}

// group reads {all: [...]} or {any: [...]} and answers the operator and the list's items.
func (d *decoder) group(n *yaml.Node, what string) (GroupOp, []*yaml.Node, error) {
	f, err := d.mapping(n, what)
	if err != nil {
		return "", nil, err
	}
	if f.has("all") == f.has("any") {
		return "", nil, d.errf(n, "%s needs exactly one of \"all\" and \"any\"", what)
	}
	op := All
	if f.has("any") {
		op = Any
	}
	items, err := f.list(string(op))
	if err != nil {
		return "", nil, err
	}
	if err := f.done(what, "all", "any"); err != nil {
		return "", nil, err
	}
	return op, items, nil
}

func (d *decoder) condition(n *yaml.Node) (*Condition, error) {
	op, groups, err := d.group(n, "a condition (\"when\")")
	if err != nil {
		return nil, err
	}
	c := &Condition{Op: op, Groups: make([]Group, 0, len(groups))}
	for _, gn := range groups {
		gop, lines, err := d.group(gn, "a group of condition lines")
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			return nil, d.errf(gn, "a group needs at least one line")
		}
		g := Group{Op: gop, Lines: make([]Line, 0, len(lines))}
		for _, ln := range lines {
			l, err := d.condLine(ln)
			if err != nil {
				return nil, err
			}
			g.Lines = append(g.Lines, *l)
		}
		c.Groups = append(c.Groups, g)
	}
	return c, nil
}

func (d *decoder) condLine(n *yaml.Node) (*Line, error) {
	f, err := d.mapping(n, "a condition line")
	if err != nil {
		return nil, err
	}
	l := &Line{}
	kinds := 0
	for _, k := range []string{"device", "variable", "time"} {
		if f.has(k) {
			kinds++
		}
	}
	if kinds != 1 {
		return nil, d.errf(n, "a condition line is on exactly one of \"device\", \"variable\" and \"time\"")
	}
	if l.Device, err = f.ref("device"); err != nil {
		return nil, err
	}
	if l.Variable, err = f.str("variable"); err != nil {
		return nil, err
	}
	if f.has("variable") && !ValidKey(l.Variable) {
		return nil, d.errf(f.vals["variable"], "%q is not a variable's key", l.Variable)
	}
	if f.has("time") {
		if l.Time, err = d.timeModule(f.get("time")); err != nil {
			return nil, err
		}
		for _, k := range []string{"is", "op", "value", "from", "below", "to"} {
			if f.has(k) {
				return nil, d.errf(f.vals[k], "a time line has no %q", k)
			}
		}
	} else if err := d.comparison(f, l, n); err != nil {
		return nil, err
	}
	if l.Not, err = f.boolean("not", false); err != nil {
		return nil, err
	}
	if l.For, err = f.duration("for"); err != nil {
		return nil, err
	}
	if f.has("for") && (l.Time != nil || l.For <= 0 || l.For > MaxHold) {
		return nil, d.errf(f.vals["for"], "for: how long the comparison must have held, from 1s to %s; a time line has none", MaxHold)
	}
	modes := []string{string(TriggerChange), string(TriggerUpdate), string(TriggerValue), string(TriggerCheck)}
	if l.Time != nil {
		modes = []string{string(TriggerUpdate), string(TriggerCheck)}
	}
	if !f.has("trigger") {
		return nil, d.errf(n, "a condition line needs \"trigger\" (%s)", strings.Join(modes, ", "))
	}
	t, err := f.word("trigger", modes...)
	if err != nil {
		return nil, err
	}
	l.Trigger = Trigger(t)
	if l.Note, err = f.note(); err != nil {
		return nil, err
	}
	if err := f.done("a condition line", lineKeys...); err != nil {
		return nil, err
	}
	return l, nil
}

func isNumber(v any) bool { _, ok := v.(float64); return ok }

func (d *decoder) comparison(f *fields, l *Line, n *yaml.Node) error {
	var err error
	want := func(keys ...string) error {
		for _, k := range []string{"is", "value", "from", "below", "to"} {
			wanted := false
			for _, w := range keys {
				wanted = wanted || w == k
			}
			if wanted && !f.has(k) {
				return d.errf(n, "the comparison %q needs %q", l.Op, k)
			}
			if !wanted && f.has(k) {
				return d.errf(f.vals[k], "the comparison %q has no %q", l.Op, k)
			}
		}
		return nil
	}
	if f.has("is") {
		if f.has("op") {
			return d.errf(f.vals["op"], "a line has \"is\" (equal) or \"op\" with its values, not both")
		}
		l.Op = OpEq
		if err := want("is"); err != nil {
			return err
		}
		l.Value, err = f.value("is")
		return err
	}
	if !f.has("op") {
		return d.errf(n, "a condition line needs \"is\" or \"op\"")
	}
	op, err := f.word("op", "ne", "gt", "ge", "lt", "le", "range", "outside")
	if err != nil {
		return err
	}
	l.Op = Op(op)
	switch l.Op {
	case OpRange:
		if err := want("from", "below"); err != nil {
			return err
		}
		if l.From, err = f.value("from"); err != nil {
			return err
		}
		if l.Below, err = f.value("below"); err != nil {
			return err
		}
		if !isNumber(l.From) || !isNumber(l.Below) {
			return d.errf(n, "a range compares numbers")
		}
	case OpOutside:
		if err := want("from", "to"); err != nil {
			return err
		}
		if l.From, err = f.value("from"); err != nil {
			return err
		}
		if l.To, err = f.value("to"); err != nil {
			return err
		}
		if !isNumber(l.From) || !isNumber(l.To) {
			return d.errf(n, "a range compares numbers")
		}
	default:
		if err := want("value"); err != nil {
			return err
		}
		if l.Value, err = f.value("value"); err != nil {
			return err
		}
		if l.Op != OpNe && !isNumber(l.Value) {
			return d.errf(f.vals["value"], "the comparison %q compares numbers", l.Op)
		}
	}
	return nil
}

var (
	clockPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?$`)
	datePattern  = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])( ([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?)?$`)
)

func (d *decoder) timeModule(n *yaml.Node) (*TimeModule, error) {
	f, err := d.mapping(n, "a time module")
	if err != nil {
		return nil, err
	}
	t := &TimeModule{}
	if !f.has("kind") {
		return nil, d.errf(n, "a time module needs \"kind\" (%s)", strings.Join(TimeKinds, ", "))
	}
	if t.Kind, err = f.word("kind", TimeKinds...); err != nil {
		return nil, err
	}
	if t.At, err = f.str("at"); err != nil {
		return nil, err
	}
	if f.has("at") && !clockPattern.MatchString(t.At) {
		return nil, d.errf(f.vals["at"], "at: %q is not a time of day (\"HH:MM\" or \"HH:MM:SS\", in quotes)", t.At)
	}
	if t.Astro, err = f.word("astro", AstroKinds...); err != nil {
		return nil, err
	}
	if t.AllDay, err = f.boolean("all_day", false); err != nil {
		return nil, err
	}
	if t.Offset, err = f.duration("offset"); err != nil {
		return nil, err
	}
	if t.Point, err = f.boolean("point", false); err != nil {
		return nil, err
	}
	if t.Twilight, err = f.word("twilight", Twilights...); err != nil {
		return nil, err
	}
	if t.Duration, err = f.duration("duration"); err != nil {
		return nil, err
	}
	if t.Interval, err = f.duration("interval"); err != nil {
		return nil, err
	}
	if t.Every, err = f.integer("every", 1, 10000); err != nil {
		return nil, err
	}
	days, err := f.list("weekdays")
	if err != nil {
		return nil, err
	}
	for _, dn := range days {
		if err := d.scalarNode(dn, "a weekday"); err != nil {
			return nil, err
		}
		ok := false
		for _, w := range Weekdays {
			ok = ok || w == dn.Value
		}
		for _, have := range t.Weekdays {
			ok = ok && have != dn.Value
		}
		if !ok {
			return nil, d.errf(dn, "weekdays: %q is not one of %s, or is there twice", dn.Value, strings.Join(Weekdays, ", "))
		}
		t.Weekdays = append(t.Weekdays, dn.Value)
	}
	if t.Day, err = f.integer("day", 1, 31); err != nil {
		return nil, err
	}
	if t.Nth, err = f.integer("nth", 1, 5); err != nil {
		return nil, err
	}
	if t.Month, err = f.integer("month", 1, 12); err != nil {
		return nil, err
	}
	for _, s := range []struct {
		key string
		to  *string
	}{{"date", &t.Date}, {"from", &t.From}, {"until", &t.Until}} {
		if *s.to, err = f.str(s.key); err != nil {
			return nil, err
		}
		if f.has(s.key) && !datePattern.MatchString(*s.to) {
			return nil, d.errf(f.vals[s.key], "%s: %q is not a date (\"YYYY-MM-DD\" or \"YYYY-MM-DD HH:MM\", in quotes)", s.key, *s.to)
		}
	}
	if t.Count, err = f.integer("count", 1, 1000000); err != nil {
		return nil, err
	}
	if err := f.done("a time module", timeKeys...); err != nil {
		return nil, err
	}
	if err := t.Check(); err != nil {
		return nil, d.errf(n, "%v", err)
	}
	return t, nil
}

func (d *decoder) actions(n *yaml.Node, what string) ([]Action, error) {
	items, err := d.sequence(n, what)
	if err != nil {
		return nil, err
	}
	out := make([]Action, 0, len(items))
	for _, it := range items {
		a, err := d.action(it)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, nil
}

func (d *decoder) action(n *yaml.Node) (*Action, error) {
	f, err := d.mapping(n, "an activity")
	if err != nil {
		return nil, err
	}
	a := &Action{}
	kinds := []string{"set", "variable", "run", "activate", "deactivate", "script", "webhook", "wake", "channel", "display", "command"}
	for _, k := range kinds {
		if f.has(k) {
			if a.Kind != "" {
				return nil, d.errf(f.vals[k], "an activity is exactly one of %s", strings.Join(kinds, ", "))
			}
			a.Kind = ActionKind(k)
		}
	}
	if a.Kind == "" {
		return nil, d.errf(n, "an activity is one of %s", strings.Join(kinds, ", "))
	}
	writes := a.Kind == ActionSet || a.Kind == ActionVariable
	for _, k := range []string{"to", "from", "from_variable", "special"} {
		if f.has(k) && !writes {
			return nil, d.errf(f.vals[k], "only \"set\" and \"variable\" activities have %q", k)
		}
	}
	if f.has("mode") && a.Kind != ActionRun {
		return nil, d.errf(f.vals["mode"], "only a \"run\" activity has a \"mode\"")
	}
	for _, k := range []string{"method", "headers", "body_type", "body", "insecure"} {
		if f.has(k) && a.Kind != ActionWebhook {
			return nil, d.errf(f.vals[k], "only a \"webhook\" activity has %q", k)
		}
	}
	if f.has("broadcast") && a.Kind != ActionWake {
		return nil, d.errf(f.vals["broadcast"], "only a \"wake\" activity has \"broadcast\"")
	}
	for _, k := range []string{"shell", "args", "timeout", "wait", "output", "exit_code"} {
		if f.has(k) && a.Kind != ActionCommand {
			return nil, d.errf(f.vals[k], "only a \"command\" activity has %q", k)
		}
	}
	if f.has("values") && a.Kind != ActionChannel {
		return nil, d.errf(f.vals["values"], "only a \"channel\" activity has \"values\"")
	}
	for _, k := range []string{"lines", "sound", "repetitions", "interval"} {
		if f.has(k) && a.Kind != ActionDisplay {
			return nil, d.errf(f.vals[k], "only a \"display\" activity has %q", k)
		}
	}
	switch a.Kind {
	case ActionDisplay:
		if a.Channel, err = f.str("display"); err != nil {
			return nil, err
		}
		if err := ParseChannel(a.Channel); err != nil {
			return nil, d.errf(f.vals["display"], "display: %v", err)
		}
		if a.Display, err = d.display(f); err != nil {
			return nil, err
		}
	case ActionChannel:
		if a.Channel, err = f.str("channel"); err != nil {
			return nil, err
		}
		if err := ParseChannel(a.Channel); err != nil {
			return nil, d.errf(f.vals["channel"], "channel: %v", err)
		}
		if a.Values, err = d.values(f); err != nil {
			return nil, err
		}
	case ActionWebhook:
		if a.Webhook, err = d.webhook(f); err != nil {
			return nil, err
		}
	case ActionCommand:
		if a.Command, err = d.command(f); err != nil {
			return nil, err
		}
	case ActionWake:
		a.Wake = &Wake{}
		if a.Wake.MAC, err = f.str("wake"); err != nil {
			return nil, err
		}
		if a.Wake.Broadcast, err = f.str("broadcast"); err != nil {
			return nil, err
		}
		if msg := a.Wake.Check(); msg != "" {
			return nil, d.errf(f.vals["wake"], "%s", msg)
		}
	case ActionSet:
		if a.Device, err = f.ref("set"); err != nil {
			return nil, err
		}
	case ActionVariable:
		if a.Variable, err = f.str("variable"); err != nil {
			return nil, err
		}
		if !ValidKey(a.Variable) {
			return nil, d.errf(f.vals["variable"], "%q is not a variable's key", a.Variable)
		}
	case ActionRun, ActionActivate, ActionDeactivate:
		if a.Program, err = f.str(string(a.Kind)); err != nil {
			return nil, err
		}
		if !ValidKey(a.Program) {
			return nil, d.errf(f.vals[string(a.Kind)], "%q is not a program's key", a.Program)
		}
	case ActionScript:
		if a.Script, err = f.str("script"); err != nil {
			return nil, err
		}
		if len(a.Script) > MaxText {
			return nil, d.errf(f.vals["script"], "the script is longer than %d bytes", MaxText)
		}
	}
	if writes {
		sources := 0
		for _, k := range []string{"to", "from", "from_variable", "special"} {
			if f.has(k) {
				sources++
			}
		}
		if sources != 1 {
			return nil, d.errf(n, "a \"%s\" activity needs exactly one of \"to\", \"from\", \"from_variable\" and \"special\"", a.Kind)
		}
		if a.To, err = f.value("to"); err != nil {
			return nil, err
		}
		if a.FromDevice, err = f.ref("from"); err != nil {
			return nil, err
		}
		if a.FromVariable, err = f.str("from_variable"); err != nil {
			return nil, err
		}
		if f.has("from_variable") && !ValidKey(a.FromVariable) {
			return nil, d.errf(f.vals["from_variable"], "%q is not a variable's key", a.FromVariable)
		}
		if a.Special, err = f.str("special"); err != nil {
			return nil, err
		}
		if f.has("special") && a.Special == "" {
			return nil, d.errf(f.vals["special"], "\"special\" names the special value")
		}
	}
	if a.Kind == ActionRun {
		if !f.has("mode") {
			return nil, d.errf(n, "a \"run\" activity needs \"mode\": then (the first block, without checking) or check (with the conditions)")
		}
		if a.Mode, err = f.word("mode", RunThen, RunCheck); err != nil {
			return nil, err
		}
	}
	if a.OnFor, err = f.duration("on_for"); err != nil {
		return nil, err
	}
	if f.has("on_for") && (a.Kind != ActionSet || !f.has("to") || a.OnFor <= 0 || a.OnFor > MaxHold) {
		return nil, d.errf(f.vals["on_for"], "on_for: how long the channel stays on, from 1s to %s; only a \"set\" activity with \"to\" has it", MaxHold)
	}
	if a.After, err = f.duration("after"); err != nil {
		return nil, err
	}
	if a.AfterUpTo, err = f.duration("after_up_to"); err != nil {
		return nil, err
	}
	if f.has("after_up_to") && (a.AfterUpTo <= a.After || a.Kind == ActionScript) {
		return nil, d.errf(f.vals["after_up_to"], "after_up_to: the longest of a random delay, more than \"after\" (which is 0s when left out)")
	}
	if a.Note, err = f.note(); err != nil {
		return nil, err
	}
	if err := f.done("an activity", actionKeys...); err != nil {
		return nil, err
	}
	return a, nil
}

// values reads a "channel" activity's datapoints with their values, in their order.
func (d *decoder) values(f *fields) ([]ParamValue, error) {
	n := f.get("values")
	if n == nil {
		return nil, d.errf(f.vals["channel"], "a \"channel\" activity needs \"values\": the datapoints to write, each with its value")
	}
	vf, err := d.mapping(n, "values")
	if err != nil {
		return nil, err
	}
	if len(vf.keys) == 0 || len(vf.keys) > MaxValues {
		return nil, d.errf(n, "values: 1 to %d datapoints, each with its value", MaxValues)
	}
	out := make([]ParamValue, 0, len(vf.keys))
	for _, k := range vf.keys {
		if !refParam.MatchString(k.Value) {
			return nil, d.errf(k, "%q is not a datapoint's name", k.Value)
		}
		v, err := vf.value(k.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, ParamValue{Param: k.Value, Value: v})
	}
	return out, nil
}

// display reads what a "display" activity shows: its lines and its sound.
func (d *decoder) display(f *fields) (*Display, error) {
	out := &Display{}
	lines, err := f.list("lines")
	if err != nil {
		return nil, err
	}
	for _, ln := range lines {
		lf, err := d.mapping(ln, "a line")
		if err != nil {
			return nil, err
		}
		var l DisplayLine
		for i, to := range []*string{&l.Text, &l.Icon, &l.Align, &l.Color, &l.Background} {
			if *to, err = lf.str([]string{"text", "icon", "align", "color", "background"}[i]); err != nil {
				return nil, err
			}
		}
		if err := lf.done("a line", "text", "icon", "align", "color", "background"); err != nil {
			return nil, err
		}
		out.Lines = append(out.Lines, l)
	}
	if out.Sound, err = f.str("sound"); err != nil {
		return nil, err
	}
	if out.Repetitions, err = f.integer("repetitions", 0, 15); err != nil {
		return nil, err
	}
	if out.Interval, err = f.integer("interval", 5, 80); err != nil {
		return nil, err
	}
	if msg := out.Check(); msg != "" {
		return nil, d.errf(f.vals["display"], "%s", msg)
	}
	return out, nil
}

// command reads a "command" activity.
func (d *decoder) command(f *fields) (*Command, error) {
	c := &Command{}
	var err error
	if c.Shell, err = f.boolean("shell", false); err != nil {
		return nil, err
	}
	if f.has("shell") && !c.Shell {
		return nil, d.errf(f.vals["shell"], "shell: true, or left out")
	}
	list := "command"
	if c.Shell {
		list = "args"
		if c.Line, err = f.str("command"); err != nil {
			return nil, err
		}
	} else if f.has("args") {
		return nil, d.errf(f.vals["args"], "only a command with \"shell: true\" has \"args\": without a shell the arguments follow the program in \"command\"")
	}
	nodes, err := f.list(list)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		var a CommandArg
		if n.Kind == yaml.ScalarNode {
			if !isString(n) {
				return nil, d.errf(n, "an argument must be text: put %s in quotes", n.Value)
			}
			a.Text = n.Value
		} else {
			af, err := d.mapping(n, "an argument")
			if err != nil {
				return nil, err
			}
			if a.Device, err = af.ref("device"); err != nil {
				return nil, err
			}
			if a.Variable, err = af.str("variable"); err != nil {
				return nil, err
			}
			if a.Format, err = af.str("format"); err != nil {
				return nil, err
			}
			if err := af.done("an argument", "device", "variable", "format"); err != nil {
				return nil, err
			}
			if !a.IsValue() {
				return nil, d.errf(n, "an argument is a text, or the value of a \"variable\" or a \"device\"")
			}
		}
		c.Args = append(c.Args, a)
	}
	if c.Timeout, err = f.duration("timeout"); err != nil {
		return nil, err
	}
	if f.has("timeout") && c.Timeout <= 0 {
		return nil, d.errf(f.vals["timeout"], "timeout: from 1s to %s", MaxTimeout)
	}
	if c.Wait, err = f.boolean("wait", false); err != nil {
		return nil, err
	}
	if c.Output, err = f.str("output"); err != nil {
		return nil, err
	}
	if c.ExitCode, err = f.str("exit_code"); err != nil {
		return nil, err
	}
	if msg := c.Check(); msg != "" {
		return nil, d.errf(f.vals["command"], "%s", msg)
	}
	return c, nil
}

// webhook reads a "webhook" activity's request.
func (d *decoder) webhook(f *fields) (*Webhook, error) {
	w := &Webhook{}
	var err error
	if w.URL, err = f.str("webhook"); err != nil {
		return nil, err
	}
	if !f.has("method") {
		return nil, d.errf(f.vals["webhook"], "a \"webhook\" activity needs a \"method\" (%s)", strings.Join(WebhookMethods, ", "))
	}
	if w.Method, err = f.str("method"); err != nil {
		return nil, err
	}
	headers, err := f.list("headers")
	if err != nil {
		return nil, err
	}
	for _, hn := range headers {
		hf, err := d.mapping(hn, "a header")
		if err != nil {
			return nil, err
		}
		var h Header
		if h.Name, err = hf.str("name"); err != nil {
			return nil, err
		}
		if h.Value, err = hf.str("value"); err != nil {
			return nil, err
		}
		if err := hf.done("a header", "name", "value"); err != nil {
			return nil, err
		}
		w.Headers = append(w.Headers, h)
	}
	if w.BodyType, err = f.str("body_type"); err != nil {
		return nil, err
	}
	if w.Body, err = f.str("body"); err != nil {
		return nil, err
	}
	if w.Insecure, err = f.boolean("insecure", false); err != nil {
		return nil, err
	}
	if msg := w.Check(); msg != "" {
		return nil, d.errf(f.vals["webhook"], "%s", msg)
	}
	return w, nil
}

// --- variables ---

// EncodeVariable writes a variable's definition the one canonical way.
func EncodeVariable(v *Variable) []byte {
	e := &emitter{}
	e.kv(0, false, "name", str(v.Name))
	if v.Note != "" {
		e.text(0, false, "note", v.Note)
	}
	e.kv(0, false, "type", str(string(v.Type)))
	if v.Labels != nil && (v.Labels.False != "" || v.Labels.True != "") {
		e.key(0, false, "labels")
		if v.Labels.False != "" {
			e.kv(2, false, quoted("false"), str(v.Labels.False))
		}
		if v.Labels.True != "" {
			e.kv(2, false, quoted("true"), str(v.Labels.True))
		}
	}
	if len(v.Entries) > 0 {
		e.key(0, false, "entries")
		for _, s := range v.Entries {
			e.line(2, "- "+str(s))
		}
	}
	if v.Unit != "" {
		e.kv(0, false, "unit", str(v.Unit))
	}
	if v.Min != nil {
		e.kv(0, false, "min", number(*v.Min))
	}
	if v.Max != nil {
		e.kv(0, false, "max", number(*v.Max))
	}
	if !v.Visible {
		e.kv(0, false, "visible", "false")
	}
	if v.Internal {
		e.kv(0, false, "internal", "true")
	}
	if v.Logged {
		e.kv(0, false, "logged", "true")
	}
	if v.Channel != "" {
		e.kv(0, false, "channel", str(v.Channel))
	}
	if v.StartValue != nil {
		e.kv(0, false, "start_value", scalar(v.StartValue))
	}
	if c := v.Computed; c != nil {
		e.key(0, false, "computed")
		e.kv(2, false, "function", str(c.Function))
		e.key(2, false, "inputs")
		for _, in := range c.Inputs {
			if in.Device != nil {
				e.line(4, "- device: "+str(in.Device.String()))
			} else {
				e.line(4, "- variable: "+str(in.Variable))
			}
		}
	}
	if h := v.Helper; h != nil {
		e.key(0, false, "helper")
		e.kv(2, false, "kind", str(h.Kind))
		e.kv(2, false, "channel", str(h.Channel))
		e.kv(2, false, "role", str(h.Role))
		if h.Off {
			e.kv(2, false, "off", "true")
		}
	}
	return []byte(e.b.String())
}

// DecodeVariable reads a variable's file.
func DecodeVariable(file, key string, data []byte) (*Variable, error) {
	d := &decoder{file: file}
	if !ValidKey(key) {
		return nil, &Error{File: file, Msg: fmt.Sprintf("%q cannot be a variable's key (lower-case letters, digits and hyphens, at most 48)", key)}
	}
	root, err := d.document(data)
	if err != nil {
		return nil, err
	}
	f, err := d.mapping(root, "a variable")
	if err != nil {
		return nil, err
	}
	v := &Variable{Key: key}
	if v.Name, err = checkName(d, f, "a variable"); err != nil {
		return nil, err
	}
	if v.Note, err = f.note(); err != nil {
		return nil, err
	}
	if !f.has("type") {
		return nil, d.errf(root, "a variable needs a \"type\" (bool, list, number, string, alarm)")
	}
	t, err := f.word("type", "bool", "list", "number", "string", "alarm")
	if err != nil {
		return nil, err
	}
	v.Type = VarType(t)
	only := func(key string, types ...VarType) error {
		if !f.has(key) {
			return nil
		}
		for _, ty := range types {
			if v.Type == ty {
				return nil
			}
		}
		return d.errf(f.vals[key], "a variable of type %s has no %q", v.Type, key)
	}
	for key, types := range map[string][]VarType{"labels": {VarBool, VarAlarm}, "entries": {VarList}, "unit": {VarNumber}, "min": {VarNumber}, "max": {VarNumber}} {
		if err := only(key, types...); err != nil {
			return nil, err
		}
	}
	if ln := f.get("labels"); ln != nil {
		lf, err := d.mapping(ln, "labels")
		if err != nil {
			return nil, err
		}
		v.Labels = &Labels{}
		if v.Labels.False, err = lf.str("false"); err != nil {
			return nil, err
		}
		if v.Labels.True, err = lf.str("true"); err != nil {
			return nil, err
		}
		if err := lf.done("labels", "\"false\"", "\"true\""); err != nil {
			return nil, err
		}
		if v.Labels.False == "" && v.Labels.True == "" {
			v.Labels = nil
		}
	}
	entries, err := f.list("entries")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, en := range entries {
		if err := d.scalarNode(en, "an entry"); err != nil {
			return nil, err
		}
		if !isString(en) {
			return nil, d.errf(en, "an entry must be text: put %s in quotes", en.Value)
		}
		if en.Value == "" || seen[en.Value] || strings.ContainsAny(en.Value, ";\n\r") {
			return nil, d.errf(en, "an entry is empty, there twice, or has a semicolon or a line break: %q", en.Value)
		}
		seen[en.Value] = true
		v.Entries = append(v.Entries, en.Value)
	}
	if v.Type == VarList && len(v.Entries) == 0 {
		return nil, d.errf(root, "a list needs \"entries\"")
	}
	if v.Unit, err = f.str("unit"); err != nil {
		return nil, err
	}
	for _, b := range []struct {
		key string
		to  **float64
	}{{"min", &v.Min}, {"max", &v.Max}} {
		if n := f.get(b.key); n != nil {
			x, err := d.float(n, b.key)
			if err != nil {
				return nil, err
			}
			*b.to = &x
		}
	}
	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		return nil, d.errf(f.vals["min"], "min is above max")
	}
	if v.Visible, err = f.boolean("visible", true); err != nil {
		return nil, err
	}
	if v.Internal, err = f.boolean("internal", false); err != nil {
		return nil, err
	}
	if v.Logged, err = f.boolean("logged", false); err != nil {
		return nil, err
	}
	if v.Channel, err = f.str("channel"); err != nil {
		return nil, err
	}
	if f.has("channel") {
		if err := ParseChannel(v.Channel); err != nil {
			return nil, d.errf(f.vals["channel"], "channel: %v", err)
		}
	}
	if v.StartValue, err = f.value("start_value"); err != nil {
		return nil, err
	}
	if v.StartValue != nil {
		if err := v.CheckValue(v.StartValue); err != nil {
			return nil, d.errf(f.vals["start_value"], "start_value: %v", err)
		}
	}
	if cn := f.get("computed"); cn != nil {
		if v.Computed, err = d.computed(cn); err != nil {
			return nil, err
		}
		if v.Type != VarNumber {
			return nil, d.errf(cn, "only a number can be computed")
		}
		for _, k := range []string{"min", "max", "start_value"} {
			if f.has(k) {
				return nil, d.errf(f.vals[k], "a computed value has no %q: nobody writes it", k)
			}
		}
	}
	if hn := f.get("helper"); hn != nil {
		if v.Helper, err = d.helper(hn); err != nil {
			return nil, err
		}
		if v.Type != VarNumber || v.Computed != nil {
			return nil, d.errf(hn, "only a plain number can be a device helper's")
		}
		if f.has("start_value") {
			return nil, d.errf(f.vals["start_value"], "a device helper's variable has no \"start_value\": the helper keeps its value")
		}
	}
	if err := f.done("a variable", variableKeys...); err != nil {
		return nil, err
	}
	return v, nil
}

// computed reads a variable's "computed": the function and its inputs.
func (d *decoder) computed(n *yaml.Node) (*Computed, error) {
	f, err := d.mapping(n, "computed")
	if err != nil {
		return nil, err
	}
	c := &Computed{}
	if !f.has("function") {
		return nil, d.errf(n, "a computed value needs a \"function\" (%s)", strings.Join(ComputedFunctions, ", "))
	}
	if c.Function, err = f.word("function", ComputedFunctions...); err != nil {
		return nil, err
	}
	inputs, err := f.list("inputs")
	if err != nil {
		return nil, err
	}
	for _, in := range inputs {
		inf, err := d.mapping(in, "an input")
		if err != nil {
			return nil, err
		}
		if inf.has("device") == inf.has("variable") {
			return nil, d.errf(in, "an input is exactly one of \"device\" and \"variable\"")
		}
		var input Input
		if input.Device, err = inf.ref("device"); err != nil {
			return nil, err
		}
		if input.Variable, err = inf.str("variable"); err != nil {
			return nil, err
		}
		if inf.has("variable") && !ValidKey(input.Variable) {
			return nil, d.errf(inf.vals["variable"], "%q is not a variable's key", input.Variable)
		}
		if err := inf.done("an input", "device", "variable"); err != nil {
			return nil, err
		}
		c.Inputs = append(c.Inputs, input)
	}
	if lo, hi := InputCount(c.Function); len(c.Inputs) < lo || len(c.Inputs) > hi {
		if lo == hi {
			return nil, d.errf(n, "%s takes exactly %d inputs", c.Function, lo)
		}
		return nil, d.errf(n, "%s takes %d to %d inputs", c.Function, lo, hi)
	}
	if err := f.done("computed", "function", "inputs"); err != nil {
		return nil, err
	}
	return c, nil
}

// helper reads a variable's "helper": the kind, the channel and the role.
func (d *decoder) helper(n *yaml.Node) (*Helper, error) {
	f, err := d.mapping(n, "helper")
	if err != nil {
		return nil, err
	}
	h := &Helper{}
	kinds := make([]string, len(HelperKinds))
	for i, k := range HelperKinds {
		kinds[i] = k.Kind
	}
	for _, key := range []string{"kind", "channel", "role"} {
		if !f.has(key) {
			return nil, d.errf(n, "a helper needs a %q", key)
		}
	}
	if h.Kind, err = f.word("kind", kinds...); err != nil {
		return nil, err
	}
	if h.Channel, err = f.str("channel"); err != nil {
		return nil, err
	}
	if err := ParseChannel(h.Channel); err != nil {
		return nil, d.errf(f.vals["channel"], "channel: %v", err)
	}
	k := HelperKindOf(h.Kind)
	if h.Role, err = f.word("role", k.Roles...); err != nil {
		return nil, err
	}
	if h.Off, err = f.boolean("off", false); err != nil {
		return nil, err
	}
	if h.Off && h.Role != k.Lead() {
		return nil, d.errf(f.vals["off"], "\"off\" is said on the helper's %s", k.Lead())
	}
	if err := f.done("helper", "kind", "channel", "role", "off"); err != nil {
		return nil, err
	}
	return h, nil
}

// CheckValue says whether x is a value of the variable's type: a bool for bool and alarm, one of
// the entries for a list, a number, a string. The limits of a number are not checked here: who
// writes decides whether they apply.
func (v *Variable) CheckValue(x any) error {
	switch v.Type {
	case VarBool, VarAlarm:
		if _, ok := x.(bool); !ok {
			return fmt.Errorf("%s takes true or false", v.Key)
		}
	case VarNumber:
		if !isNumber(x) {
			return fmt.Errorf("%s takes a number", v.Key)
		}
	case VarString:
		if _, ok := x.(string); !ok {
			return fmt.Errorf("%s takes text", v.Key)
		}
	case VarList:
		s, ok := x.(string)
		if !ok || v.EntryIndex(s) < 0 {
			return fmt.Errorf("%s takes one of its entries (%s)", v.Key, strings.Join(v.Entries, ", "))
		}
	default:
		return fmt.Errorf("%s has no type", v.Key)
	}
	return nil
}

// EntryIndex answers the index of a list's entry, -1 when it has none of that label.
func (v *Variable) EntryIndex(label string) int {
	for i, e := range v.Entries {
		if e == label {
			return i
		}
	}
	return -1
}

// Check validates a program as the reader does, by writing and reading it; the answer is the
// program as a file would give it back.
func (p *Program) Check() (*Program, error) {
	return DecodeProgram(p.Key+".yaml", p.Key, EncodeProgram(p, nil))
}

// Check validates a variable as the reader does.
func (v *Variable) Check() (*Variable, error) {
	return DecodeVariable(v.Key+".yaml", v.Key, EncodeVariable(v))
}
