package importer

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// leftover is the name of the program the WebUI's editor keeps while a new program is unsaved.
const leftover = "${ruleTmpProgramName}"

// programKeys gives every program that will be mapped its key, before any is mapped: an activity
// may name a program further down.
func (m *imp) programKeys() {
	used := map[string]bool{}
	for _, p := range m.db.Programs {
		if IsCounterProgram(p.Name) || p.Name == leftover {
			continue
		}
		m.prgKey[p.ID] = m.key("program", p.ID, name(m.words(p.Name)), used)
	}
}

// one program's mapping in progress
type prog struct {
	rep     ProgramReport
	skip    *Note           // set: the program cannot be imported
	devices map[string]bool // the channels it uses
}

func (p *prog) reason(code string, args ...string) { p.rep.Reasons = add(p.rep.Reasons, code, args...) }
func (p *prog) note(code string, args ...string)   { p.rep.Notes = add(p.rep.Notes, code, args...) }
func (p *prog) drop(code string, args ...string) {
	if p.skip == nil {
		p.skip = &Note{Code: code, Args: args}
	}
}
func (p *prog) unknown(fields []string) {
	for _, f := range fields {
		p.reason(ReasonUnknownField, f)
	}
}

func (m *imp) programs() {
	set := model.NewSet()
	for _, v := range m.res.Variables {
		set.Variables[v.Key] = v
	}
	type mapped struct {
		def *model.Program
		st  *prog
	}
	var all []mapped
	for _, p := range m.db.Programs {
		st := &prog{rep: ProgramReport{ID: p.ID, Name: name(m.words(p.Name)), WasActive: p.Active, Status: Skipped}, devices: map[string]bool{}}
		switch {
		case IsCounterProgram(p.Name):
			st.drop(SkipCounter)
		case p.Name == leftover:
			st.drop(SkipLeftover)
		}
		var def *model.Program
		if st.skip == nil {
			def = m.program(p, st)
		}
		if st.skip == nil {
			if _, err := def.Check(); err != nil {
				st.drop(SkipInvalid, err.Error())
			}
		}
		if st.skip != nil {
			delete(m.prgKey, p.ID)
			def = nil
		} else {
			set.Programs[def.Key] = def
		}
		all = append(all, mapped{def, st})
	}
	// an activity names a program by the key it was given before the mapping; when that program
	// was dropped after all, the one that runs it cannot be written either
	for changed := true; changed; {
		changed = false
		for _, mp := range all {
			if mp.st.skip != nil {
				continue
			}
			if problems := set.ProgramProblems(&model.Program{Key: mp.def.Key, Rules: mp.def.Rules}); len(problems) > 0 {
				mp.st.drop(SkipInvalid, problems[0])
				delete(set.Programs, mp.def.Key)
				changed = true
			}
		}
	}
	for _, mp := range all {
		st := mp.st
		if st.skip != nil {
			st.rep.Reasons, st.rep.Notes = []Note{*st.skip}, nil
			m.res.Report.Programs = append(m.res.Report.Programs, st.rep)
			continue
		}
		def := mp.def
		// activation (OQ-13): the CCU's state where nothing stands against it
		var missing []string
		for ch := range st.devices {
			if m.users[ch] == nil {
				m.users[ch] = map[string]bool{}
			}
			m.users[ch][def.Name] = true
			if strings.HasPrefix(ch, "CUxD/") {
				st.reason(ReasonCUxD, ch)
			} else if m.opt.Known == nil || !m.opt.Known(ch) {
				missing = append(missing, ch)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			st.reason(ReasonMissing, missing...)
		}
		if m.opt.AllInactive && st.rep.WasActive && len(st.rep.Reasons) == 0 {
			st.reason(ReasonAllInactive)
		}
		def.Active = st.rep.WasActive && len(st.rep.Reasons) == 0
		st.rep.Active = def.Active
		st.rep.Status = Imported
		if len(st.rep.Reasons) > 0 {
			st.rep.Status = Inactive
		}
		if def.Active && def.AtStart {
			st.note(NoteAtStart)
		}
		st.rep.Key = def.Key
		if prev := m.opt.Previous; prev != nil && prev.Programs[st.rep.ID] == def.Key {
			st.rep.Updated = true
		}
		m.res.Programs = append(m.res.Programs, def)
		m.res.Ledger.Programs[st.rep.ID] = def.Key
		m.res.Report.Programs = append(m.res.Report.Programs, st.rep)
		m.hist["program"]++
	}
	m.res.Ledger.Merge(m.opt.Previous)
}

func groupOp(code int) (model.GroupOp, bool) {
	switch code {
	case 1:
		return model.All, true
	case 2:
		return model.Any, true
	}
	return model.All, false
}

// program maps one program; st.skip is set when it cannot be.
func (m *imp) program(p *regadom.Program, st *prog) *model.Program {
	def := &model.Program{Key: m.prgKey[p.ID], Name: st.rep.Name, Note: m.words(p.Info), Visible: p.Visible(), Internal: p.Internal(),
		Operable: p.AccessRights&0x00FFFFFF == 0x00FFFFFF, AtStart: true}
	if len(def.Note) > model.MaxText {
		def.Note = def.Note[:model.MaxText]
	}
	if p.StartupExec != nil {
		def.AtStart = *p.StartupExec
		if !def.AtStart {
			st.note(NoteNotAtStart)
		}
	}
	st.unknown(p.Unknown)
	chain, err := m.db.Chain(p)
	if err != nil {
		st.drop(SkipUnreadable, err.Error())
		return def
	}
	triggers, updates := 0, false
	for ri, r := range chain {
		st.unknown(r.Unknown)
		rule := model.Rule{Then: []model.Action{}}
		last := ri == len(chain)-1
		switch {
		case !r.ElseIf && last && ri > 0:
			rule.Else = true
			m.hist["rule: else"]++
		case !r.ElseIf:
			// a SONST that is not the end of a chain: nothing the editor makes
			st.drop(SkipUnreadable, "a rule without a condition is not the last one")
			return def
		default:
			rule.When = &model.Condition{Op: model.Any, Groups: []model.Group{}}
			m.hist[map[bool]string{true: "rule: if", false: "rule: else if"}[ri == 0]]++
		}
		for bi, cid := range r.Conditions {
			if rule.Else {
				break
			}
			c := m.db.Conditions[cid]
			if c == nil {
				st.drop(SkipDangling, strconv.Itoa(cid))
				return def
			}
			st.unknown(c.Unknown)
			// the blocks join by the operator each block carries against the one before
			if op, ok := groupOp(c.Op); ok && bi > 0 {
				if bi > 1 && op != rule.When.Op {
					st.reason(ReasonOperators)
				}
				if bi == 1 {
					rule.When.Op = op
				}
			}
			g := model.Group{Op: model.All, Lines: []model.Line{}}
			for li, lid := range c.Lines {
				l := m.db.Lines[lid]
				if l == nil {
					st.drop(SkipDangling, strconv.Itoa(lid))
					return def
				}
				// the lines of a block join by the operator each line carries against the one before
				if op, ok := groupOp(l.Op); ok && li > 0 {
					if li > 1 && op != g.Op {
						st.reason(ReasonOperators)
					}
					if li == 1 {
						g.Op = op
					}
				}
				line, ok := m.line(l, st)
				if !ok {
					return def
				}
				if line.Time != nil && line.Trigger == model.TriggerCheck {
					// the CCU lists a program under each of its time modules whatever the line's
					// mode, and the module evaluates it at its instants (measured)
					triggers++
					st.note(NoteTimeCheck)
				}
				if line.Trigger != model.TriggerCheck {
					triggers++
					updates = updates || (line.Trigger == model.TriggerUpdate && line.Time == nil)
					if ri > 0 && line.Trigger == model.TriggerChange {
						st.note(NoteElseIfChange)
					}
				}
				g.Lines = append(g.Lines, line)
			}
			if len(g.Lines) > 0 {
				rule.When.Groups = append(rule.When.Groups, g)
			}
		}
		if d := m.db.Destinations[r.Destination]; d != nil {
			st.unknown(d.Unknown)
			rule.CancelPending = d.BreakOnRestart
			for _, aid := range d.Actions {
				a := m.db.Actions[aid]
				if a == nil {
					st.drop(SkipDangling, strconv.Itoa(aid))
					return def
				}
				act, ok := m.action(a, st)
				if !ok {
					return def
				}
				rule.Then = append(rule.Then, act)
			}
			if rule.CancelPending && len(rule.Then) > 0 {
				st.note(NoteCancels)
				m.hist["block: cancels running delays"]++
			}
			if len(rule.Then) > 1 {
				st.note(NoteOrder)
			}
		}
		def.Rules = append(def.Rules, rule)
	}
	if def.Rules == nil {
		def.Rules = []model.Rule{}
	}
	if triggers == 0 {
		st.note(NoteNoTrigger)
	}
	if n := len(def.Rules); updates && n > 1 && def.Rules[n-1].Else && len(def.Rules[n-1].Then) > 0 {
		st.note(NoteElseUpdate)
	}
	return def
}

// ref resolves a datapoint id; the channel is noted as used by the program.
func (m *imp) ref(id int, st *prog) (*model.Ref, *regadom.Datapoint) {
	dp := m.db.Datapoints[id]
	if dp == nil {
		return nil, nil
	}
	r, err := model.ParseRef(dp.Interface + "/" + dp.Address + "/" + dp.Param)
	if err != nil {
		return nil, nil
	}
	st.devices[r.Channel()] = true
	s := m.seen[r.Channel()]
	if s == nil {
		s = &SeenDevice{Channel: r.Channel()}
		if ch := m.db.Channels[dp.ChannelID]; ch != nil {
			s.Name, s.ChannelType, s.Rooms = ch.Name, ch.Label, m.db.RoomsOf(ch.ID)
			if dev := m.db.Devices[ch.DeviceID]; dev != nil {
				s.Device, s.Type = dev.Name, dev.Label
			}
		}
		m.seen[r.Channel()] = s
	}
	found := false
	for _, p := range s.Params {
		found = found || p == r.Param
	}
	if !found {
		s.Params = append(s.Params, r.Param)
		sort.Strings(s.Params)
	}
	return &r, dp
}

// typed converts a stored value for a device datapoint by the value's own type.
func typed(v regadom.Value) (any, bool) {
	switch v.Type {
	case regadom.TypeBinary:
		return isTrue(v.Text), true
	case regadom.TypeFloat, regadom.TypeInteger:
		f, ok := num(v.Text)
		return f, ok
	case regadom.TypeString:
		return v.Text, true
	}
	return nil, false
}

// forVariable converts a stored value for a variable by the variable's type; a value list's index
// becomes the entry's label.
func forVariable(def *model.Variable, v regadom.Value) (x any, ok bool, badIndex bool) {
	switch def.Type {
	case model.VarBool, model.VarAlarm:
		if v.Type != regadom.TypeBinary && v.Type != regadom.TypeInteger && v.Type != regadom.TypeFloat {
			return nil, false, false
		}
		return isTrue(v.Text), true, false
	case model.VarNumber:
		f, ok := num(v.Text)
		return f, ok && v.Type != regadom.TypeString, false
	case model.VarList:
		i, err := strconv.Atoi(v.Text)
		if err != nil || i < 0 || i >= len(def.Entries) {
			return nil, false, true
		}
		return def.Entries[i], true, false
	}
	return v.Text, v.Type == regadom.TypeString || v.Type == regadom.TypeEmpty, false
}

var compare = map[int]model.Op{
	regadom.CmpEquals: model.OpEq, regadom.CmpEqual: model.OpEq, regadom.CmpUnequal: model.OpNe,
	regadom.CmpGreater: model.OpGt, regadom.CmpGreaterEqual: model.OpGe, regadom.CmpLess: model.OpLt, regadom.CmpLessEqual: model.OpLe,
	regadom.CmpInRange: model.OpRange, regadom.CmpOutOfRange: model.OpOutside,
}

// line maps a condition line; false means the program was dropped.
func (m *imp) line(l *regadom.Line, st *prog) (model.Line, bool) {
	st.unknown(l.Unknown)
	out := model.Line{Not: l.Negated}
	if l.Negated {
		st.note(NoteNegated)
		m.hist["line: negated"]++
	}
	switch l.Type2 {
	case regadom.TrigChange:
		out.Trigger = model.TriggerChange
	case regadom.TrigUpdate:
		out.Trigger = model.TriggerUpdate
	case regadom.TrigCheck:
		out.Trigger = model.TriggerCheck
	case regadom.TrigLegacy:
		// what the editor leaves on a key line: it fires on every event, like "bei Aktualisierung"
		out.Trigger = model.TriggerUpdate
		st.note(NoteKeyLine)
	default:
		out.Trigger = model.TriggerCheck
		st.reason(ReasonTrigger, strconv.Itoa(l.Type2))
	}
	m.hist["trigger: "+string(out.Trigger)]++

	// a time module
	if l.Type1 == regadom.CmpTime || l.Left.Type == regadom.TypeCurrentDate {
		id, _ := strconv.Atoi(l.Right1.Text)
		c := m.db.Calendars[id]
		if c == nil {
			st.drop(SkipDangling, l.Right1.Text)
			return out, false
		}
		st.unknown(c.Unknown)
		t, why, pattern := m.timeModule(c)
		if why != "" {
			st.reason(ReasonUnknownField, why)
		}
		if pattern {
			st.note(NotePattern)
		}
		if t.Astro != "" {
			st.note(NoteAstro)
		} else if inDSTHour(t) {
			st.note(NoteDST)
		}
		if out.Trigger == model.TriggerChange {
			out.Trigger = model.TriggerUpdate
		}
		out.Time = t
		m.hist["line: time module"]++
		m.hist["time module: "+t.Kind]++
		return out, true
	}

	id, err := strconv.Atoi(l.Left.Text)
	if err != nil {
		st.drop(SkipDangling, l.Left.Text)
		return out, false
	}
	op, known := compare[l.Type1]
	if !known {
		op = model.OpEq
		st.reason(ReasonComparison, strconv.Itoa(l.Type1))
	}
	if op == model.OpOutside {
		st.note(NoteOutside)
	}
	out.Op = op
	m.hist["comparison: "+string(op)]++

	if def := m.vars[id]; def != nil {
		out.Variable = def.Key
		m.hist["line: variable"]++
		switch op {
		case model.OpRange, model.OpOutside:
			from, ok1 := num(l.Right1.Text)
			to, ok2 := num(l.Right2.Text)
			if !ok1 || !ok2 || def.Type != model.VarNumber {
				out.Op, out.Value = model.OpNe, def.Default()
				st.reason(ReasonValue, def.Name)
				break
			}
			out.From = from
			if op == model.OpRange {
				out.Below = to
			} else {
				out.To = to
			}
		default:
			x, ok, badIndex := forVariable(def, l.Right1)
			if def.Type != model.VarNumber && op != model.OpEq && op != model.OpNe {
				ok = false
			}
			switch {
			case badIndex:
				out.Op, out.Value = model.OpNe, def.Default()
				st.reason(ReasonListIndex, def.Name, l.Right1.Text)
			case !ok:
				out.Op, out.Value = model.OpNe, def.Default()
				st.reason(ReasonValue, def.Name)
			default:
				out.Value = x
			}
		}
		return out, true
	}

	ref, _ := m.ref(id, st)
	if ref == nil {
		st.drop(SkipDangling, objectName(m.db, id))
		return out, false
	}
	out.Device = ref
	m.hist["line: device"]++
	switch op {
	case model.OpRange, model.OpOutside:
		from, ok1 := num(l.Right1.Text)
		to, ok2 := num(l.Right2.Text)
		if !ok1 || !ok2 {
			out.Op, out.Value = model.OpNe, ""
			st.reason(ReasonValue, ref.String())
			break
		}
		out.From = from
		if op == model.OpRange {
			out.Below = to
		} else {
			out.To = to
		}
	default:
		x, ok := typed(l.Right1)
		if _, isNum := x.(float64); ok && !isNum && op != model.OpEq && op != model.OpNe {
			ok = false
		}
		if !ok {
			out.Op, out.Value = model.OpNe, ""
			st.reason(ReasonValue, ref.String())
			break
		}
		out.Value = x
	}
	return out, true
}

// oneLiner recognises the two one-line scripts users write because the WebUI has no such
// activity (regadom.OneLiner): the program named (by name or id), and what to do with it.
func oneLiner(script string) (byName string, byID int, kind model.ActionKind, ok bool) {
	byName, byID, call, ok := regadom.OneLiner(script)
	switch call {
	case regadom.CallRun:
		kind = model.ActionRun
	case regadom.CallActivate:
		kind = model.ActionActivate
	case regadom.CallDeactivate:
		kind = model.ActionDeactivate
	}
	return byName, byID, kind, ok
}

// delay reads "verzögert um": a time of day, HH:MM:SS.
func delay(v regadom.Value) (time.Duration, bool) {
	p := strings.Split(strings.TrimSpace(v.Text), ":")
	if len(p) != 3 {
		return 0, false
	}
	var total time.Duration
	for i, unit := range []time.Duration{time.Hour, time.Minute, time.Second} {
		n, err := strconv.Atoi(p[i])
		if err != nil || n < 0 || n > 100000 {
			return 0, false
		}
		total += time.Duration(n) * unit
	}
	return total, true
}

// action maps an activity; false means the program was dropped.
func (m *imp) action(a *regadom.Action, st *prog) (model.Action, bool) {
	st.unknown(a.Unknown)
	var out model.Action
	switch a.Delay.Type {
	case regadom.TypeEmpty:
	case regadom.TypeDelay:
		d, ok := delay(a.Delay)
		switch {
		case !ok:
			st.reason(ReasonDelay, a.Delay.Text)
		case d == 0:
			// 24 hours and more are stored as zero by the CCU and run at once there
			st.note(NoteZeroDelay)
		default:
			out.After = model.Duration(d)
			m.hist["activity: delayed"]++
		}
	default:
		st.reason(ReasonDelay, a.Delay.Text)
	}
	switch a.Param {
	case regadom.DestScript:
		byName, byID, kind, ok := oneLiner(a.Value.Text)
		if ok {
			target := m.db.Program(byID)
			if byName != "" {
				target = m.db.ProgramByName(byName)
			}
			if target != nil && m.prgKey[target.ID] != "" {
				out.Kind, out.Program = kind, m.prgKey[target.ID]
				if kind == model.ActionRun {
					out.Mode = model.RunThen
				}
				m.hist["activity: program ("+string(kind)+")"]++
				return out, true
			}
		}
		if call, ok := regadom.Exec(a.Value.Text); ok {
			if act, ok := m.exec(call, st); ok {
				act.After = out.After
				return act, true
			}
		}
		out.Kind, out.Script = model.ActionScript, a.Value.Text
		if len(out.Script) > model.MaxText {
			out.Script = out.Script[:model.MaxText]
		}
		st.reason(ReasonScript)
		m.hist["activity: script"]++
		return out, true
	case regadom.DestVariable, regadom.DestDatapoint:
	default:
		st.drop(SkipUnreadable, "an activity of the unknown kind "+strconv.Itoa(a.Param))
		return out, false
	}
	special := a.Value.Type == regadom.TypeSpecialValue || strings.HasPrefix(a.Value.Text, "[SV]")
	if def := m.vars[a.TargetID]; def != nil {
		out.Kind, out.Variable = model.ActionVariable, def.Key
		m.hist["activity: variable"]++
		if special {
			out.Special = a.Value.Text
			st.reason(ReasonSpecial)
			return out, true
		}
		x, ok, badIndex := forVariable(def, a.Value)
		switch {
		case badIndex:
			out.To = def.Default()
			st.reason(ReasonListIndex, def.Name, a.Value.Text)
		case !ok:
			out.To = def.Default()
			st.reason(ReasonValue, def.Name)
		default:
			out.To = x
		}
		return out, true
	}
	ref, _ := m.ref(a.TargetID, st)
	if ref == nil {
		st.drop(SkipDangling, objectName(m.db, a.TargetID))
		return out, false
	}
	out.Kind, out.Device = model.ActionSet, ref
	m.hist["activity: device"]++
	if special {
		out.Special = a.Value.Text
		st.reason(ReasonSpecial)
		return out, true
	}
	x, ok := typed(a.Value)
	if !ok {
		out.Special = a.Value.Text
		if out.Special == "" {
			out.Special = "?"
		}
		st.reason(ReasonValue, ref.String())
		return out, true
	}
	out.To = x
	return out, true
}

// devices writes the report's device list and the notes on the ones this system does not have.
func (m *imp) devices() {
	channels := make([]string, 0, len(m.users))
	for ch := range m.users {
		channels = append(channels, ch)
	}
	sort.Strings(channels)
	for _, ch := range channels {
		s := m.seen[ch]
		d := DeviceReport{SeenDevice: *s, Known: m.opt.Known != nil && m.opt.Known(ch) && !strings.HasPrefix(ch, "CUxD/")}
		for p := range m.users[ch] {
			d.Programs = append(d.Programs, p)
		}
		sort.Strings(d.Programs)
		m.res.Report.Devices = append(m.res.Report.Devices, d)
		if !d.Known {
			m.res.Seen = append(m.res.Seen, *s)
		}
	}
}
