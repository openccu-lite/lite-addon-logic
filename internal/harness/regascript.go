package harness

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The reference's objects are built with the script calls the WebUI's pages make (the program
// editor's, system.fn::saveTimeModule, the variable dialog), so the database the reference saves
// is the editor's object graph.

// valueType is the ReGa value type of a variable's values.
func valueType(v *Var) string {
	switch v.Type {
	case "number":
		return "ivtFloat"
	case "list":
		return "ivtInteger"
	case "string":
		return "ivtString"
	}
	return "ivtBinary"
}

// literal writes a value of a variable as ReGa script.
func literal(v *Var, value any) (string, error) {
	switch v.Type {
	case "bool", "alarm", "action":
		if b, ok := value.(bool); ok {
			return strconv.FormatBool(b), nil
		}
	case "number":
		switch n := value.(type) {
		case float64:
			s := strconv.FormatFloat(n, 'f', -1, 64)
			if !strings.Contains(s, ".") {
				s += ".0"
			}
			return s, nil
		case int:
			return strconv.Itoa(n) + ".0", nil
		}
	case "list":
		if n, ok := value.(int); ok {
			return strconv.Itoa(n), nil
		}
	case "string":
		if s, ok := value.(string); ok && (s == "" || asciiName(s)) {
			return `"` + s + `"`, nil
		}
	default:
		return "", fmt.Errorf("unknown type %q", v.Type)
	}
	return "", fmt.Errorf("%v (%T) is no value of a %s", value, value, v.Type)
}

func (s *Scenario) variable(name string) *Var {
	for i := range s.Vars {
		if s.Vars[i].Name == name {
			return &s.Vars[i]
		}
	}
	return nil
}

// varScript creates a variable with its initial value.
func varScript(v *Var) string {
	lit, _ := literal(v, v.Value)
	head := fmt.Sprintf(`sv = dom.CreateObject(OT_VARDP, "%s"); dom.GetObject(ID_SYSTEM_VARIABLES).Add(sv.ID()); `, v.Name)
	switch v.Type {
	case "bool":
		return head + `sv.ValueType(ivtBinary); sv.ValueSubType(istBool); sv.ValueName1("an"); sv.ValueName0("aus"); sv.State(` + lit + ");\n"
	case "action":
		return head + `sv.ValueType(ivtBinary); sv.ValueSubType(istAction); sv.State(` + lit + ");\n"
	case "number":
		lo, hi := v.Min, v.Max
		if lo == 0 && hi == 0 {
			hi = 65000
		}
		return head + fmt.Sprintf(`sv.ValueType(ivtFloat); sv.ValueSubType(istGeneric); sv.ValueMin(%s); sv.ValueMax(%s); sv.State(%s);`+"\n",
			strconv.FormatFloat(lo, 'f', -1, 64), strconv.FormatFloat(hi, 'f', -1, 64), lit)
	case "list":
		return head + `sv.ValueType(ivtInteger); sv.ValueSubType(istEnum); sv.ValueList("` + strings.Join(v.Entries, ";") + `"); sv.State(` + lit + ");\n"
	case "string":
		return head + `sv.ValueType(ivtString); sv.ValueSubType(istChar8859); sv.State(` + lit + ");\n"
	}
	// an alarm variable, as the variable dialog makes it
	return fmt.Sprintf(`sv = dom.CreateObject(OT_ALARMDP); sv.Name("%s"); sv.AlSetBinaryCondition(); dom.GetObject(ID_SYSTEM_VARIABLES).Add(sv.ID()); sv.ValueUnit(""); sv.ValueType(ivtBinary); sv.ValueSubType(istAlarm); sv.ValueName1("an"); sv.ValueName0("aus"); sv.AlType(atSystem); sv.AlArm(true);`+"\n", v.Name)
}

// SetupScript creates the scenario's variables, the markers, the trace variable and the tracer
// program.
func (s *Scenario) SetupScript() string {
	var b strings.Builder
	b.WriteString("object sv;\n")
	if s.Position != nil {
		fmt.Fprintf(&b, "system.Longitude(%s); system.Latitude(%s);\n", strconv.FormatFloat(s.Position.Lon, 'f', -1, 64), strconv.FormatFloat(s.Position.Lat, 'f', -1, 64))
	}
	for i := range s.Vars {
		b.WriteString(varScript(&s.Vars[i]))
	}
	b.WriteString(varScript(&Var{Name: TraceVar, Type: "string", Value: ""}))
	b.WriteString(varScript(&Var{Name: BeatVar, Type: "string", Value: ""}))
	marks := s.Marks()
	for _, m := range marks {
		b.WriteString(varScript(&Var{Name: MarkerPrefix + m, Type: "string", Value: ""}))
	}
	// the tracer: one line per marker, "bei Aktualisierung", joined by OR; its DANN and its SONST
	// both note which marker was written and when. The line on the heartbeat variable lets the
	// recorder ask whether the reference still runs programs.
	note := `object s=dom.GetObject("$src$"); if(s){object t=dom.GetObject("` + TraceVar + `"); t.State(t.Value()#s.Name()#"@"#system.Date("%F %T")#" ");}`
	tracer := Program{Name: TracerName, Rules: []Rule{{Acts: []Act{{Script: note}}}, {Acts: []Act{{Script: note}}}}}
	sc := *s
	sc.Vars = append(append([]Var(nil), s.Vars...), Var{Name: BeatVar, Type: "string", Value: ""})
	tracer.Rules[0].Groups = append(tracer.Rules[0].Groups, []Line{{Var: BeatVar, Cmp: Equal, V1: "-", Mode: Update}})
	for _, m := range marks {
		sc.Vars = append(sc.Vars, Var{Name: MarkerPrefix + m, Type: "string", Value: ""})
		tracer.Rules[0].Groups = append(tracer.Rules[0].Groups, []Line{{Var: MarkerPrefix + m, Cmp: Equal, V1: "-", Mode: Update}})
	}
	b.WriteString(sc.ProgramScript(&tracer))
	b.WriteString(`WriteLine("setup ok");` + "\n")
	return b.String()
}

// timeModuleScript is system.fn::saveTimeModule with the dialog's values.
func (s *Scenario) timeModuleScript(name string, tm *TimeModule) string {
	or0 := func(v string) string {
		if v == "" {
			return "0"
		}
		return v
	}
	begin := tm.Begin
	if begin == "" {
		begin = s.StartTime().AddDate(0, 0, -1).Format("2006-01-02")
	}
	astro := ""
	when := or0(tm.Time)
	if tm.Sot != 0 {
		// as the dialog: the offset goes where the time goes, and once more as plain seconds
		when = strconv.Itoa(tm.Offset)
		astro = fmt.Sprintf(" %s.TimeSeconds(%d);", name, tm.Offset)
	}
	return fmt.Sprintf(`object %[1]s = dom.CreateObject(OT_CALENDARDP, "Zeitmodul"); dom.GetObject(ID_CALENDARDPS).Add(%[1]s);`+"\n"+
		`%[1]s.TimerType(%[2]d); %[1]s.Time("%[3]s"); %[1]s.CalDuration(%[4]d); %[1]s.CalRepeatTime("%[5]s"); %[1]s.Weekdays(%[6]d); %[1]s.Period(%[7]d); %[1]s.CalRepetitionValue(%[8]d); %[1]s.Begin("%[9]s"); %[1]s.End("%[10]s"); %[1]s.CalRepetitionCount(%[11]d); %[1]s.SunOffsetType(%[12]d);%[13]s`+"\n",
		name, tm.Type, when, tm.Caldur, or0(tm.Caltime), tm.Wd, tm.Period, tm.Calrep, begin, or0(tm.End), tm.Calcount, tm.Sot, astro)
}

// ProgramScript creates a program the way the editor does: the rule chain, the condition
// blocks and lines, the activities, then ProgramUpdate and Active. A program's time modules are
// created immediately before it (ProgramUpdate removes every time module no program refers to).
// The script answers "created <name> <time>".
func (s *Scenario) ProgramScript(p *Program) string {
	var b strings.Builder
	n := 0
	id := func(prefix string) string { n++; return prefix + strconv.Itoa(n) }
	// the time modules first
	tms := map[*TimeModule]string{}
	for ri := range p.Rules {
		for _, g := range p.Rules[ri].Groups {
			for li := range g {
				if tm := g[li].Time; tm != nil && tms[tm] == "" {
					tms[tm] = id("tm")
					b.WriteString(s.timeModuleScript(tms[tm], tm))
				}
			}
		}
	}
	fmt.Fprintf(&b, `object p = dom.CreateObject(OT_PROGRAM); p.Name("%s"); dom.GetObject(ID_PROGRAMS).Add(p.ID());`+"\n", p.Name)
	r := id("r")
	fmt.Fprintf(&b, "object %s = p.Rule();\n", r)
	for ri := range p.Rules {
		rule := &p.Rules[ri]
		if ri > 0 {
			nr := id("r")
			fmt.Fprintf(&b, "object %s = %s.RuleCreateSubRule();", nr, r)
			if len(rule.Groups) > 0 {
				fmt.Fprintf(&b, " %s.ElseIfFlag(true);", nr)
			}
			b.WriteString("\n")
			r = nr
		}
		cop, sop := 2, 1
		if rule.GroupsAnd {
			cop = 1
		}
		if rule.LinesOr {
			sop = 2
		}
		for _, g := range rule.Groups {
			c := id("c")
			fmt.Fprintf(&b, "object %s = %s.RuleAddCondition(); %s.CndOperatorType(%d);\n", c, r, c, cop)
			for li := range g {
				l := &g[li]
				sn := id("s")
				fmt.Fprintf(&b, "object %s = %s.CndAddSingle(); %s.OperatorType(%d);\n", sn, c, sn, sop)
				mode := l.Mode
				if l.Time != nil {
					if mode == 0 {
						mode = Update
					}
					fmt.Fprintf(&b, "%[1]s.ConditionType(3); %[1]s.ConditionType2(%[2]d); %[1]s.LeftValType(ivtCurrentDate); %[1]s.RightVal1ValType(ivtObjectId); %[1]s.RightVal1(%[3]s.ID());\n", sn, mode, tms[l.Time])
					continue
				}
				v := s.variable(l.Var)
				cmp := l.Cmp
				if cmp == 0 {
					cmp = Equals
				}
				v1, _ := literal(v, l.V1)
				fmt.Fprintf(&b, `%[1]s.LeftValType(ivtSystemId); %[1]s.LeftVal(dom.GetObject("%[2]s").ID()); %[1]s.ConditionType(%[3]d); `, sn, l.Var, cmp)
				if mode != 0 {
					// a line whose mode the editor never sets (a key) keeps ReGa's default
					fmt.Fprintf(&b, "%s.ConditionType2(%d); ", sn, mode)
				}
				fmt.Fprintf(&b, "%[1]s.RightVal1ValType(%[2]s); %[1]s.RightVal1(%[3]s);", sn, valueType(v), v1)
				if l.V2 != nil {
					v2, _ := literal(v, l.V2)
					fmt.Fprintf(&b, " %[1]s.RightVal2ValType(%[2]s); %[1]s.RightVal2(%[3]s);", sn, valueType(v), v2)
				}
				b.WriteString("\n")
			}
		}
		d := id("d")
		fmt.Fprintf(&b, "object %s = %s.RuleDestination(); %s.BreakOnRestart(%t);\n", d, r, d, rule.Cancel)
		for _, a := range rule.Acts {
			sd := id("a")
			fmt.Fprintf(&b, "object %s = %s.DestAddSingle(); ", sd, d)
			switch {
			case a.Script != "":
				fmt.Fprintf(&b, "%[1]s.DestinationParam(ivtString); %[1]s.DestinationValueType(ivtString); %[1]s.DestinationValue('%[2]s');", sd, a.Script)
			case a.Mark != "":
				fmt.Fprintf(&b, `%[1]s.DestinationParam(ivtSystemId); %[1]s.DestinationDP(dom.GetObject("%[2]s").ID()); %[1]s.DestinationValueType(ivtString); %[1]s.DestinationValue("%[3]s");`, sd, MarkerPrefix+a.Mark, a.Mark)
			default:
				v := s.variable(a.Var)
				lit, _ := literal(v, a.Value)
				fmt.Fprintf(&b, `%[1]s.DestinationParam(ivtSystemId); %[1]s.DestinationDP(dom.GetObject("%[2]s").ID()); %[1]s.DestinationValueType(%[3]s); %[1]s.DestinationValue(%[4]s);`, sd, a.Var, valueType(v), lit)
			}
			if a.Delay > 0 {
				// the editor sends HH:MM:SS without wrapping the hours
				fmt.Fprintf(&b, ` %[1]s.DestinationValueParamType(ivtDelay); %[1]s.DestinationValueParam("%02[2]d:%02[3]d:%02[4]d");`, sd, a.Delay/3600, a.Delay%3600/60, a.Delay%60)
			}
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "p.ProgramUpdate(); p.Active(%t);\n", !p.Inactive)
	fmt.Fprintf(&b, `WriteLine("created %s " # system.Date("%%F %%T"));`+"\n", p.Name)
	return b.String()
}

// StepScript does a step on the reference; it answers the time it was done at.
func (s *Scenario) StepScript(st *Step) string {
	when := ` Write(system.Date("%F %T"));`
	switch {
	case st.Set != "":
		lit, _ := literal(s.variable(st.Set), st.Value)
		return fmt.Sprintf(`dom.GetObject("%s").State(%s);`, st.Set, lit) + when
	case st.Run != "":
		return fmt.Sprintf(`dom.GetObject("%s").ProgramExecute();`, st.Run) + when
	case st.Activate != "":
		return fmt.Sprintf(`dom.GetObject("%s").Active(true);`, st.Activate) + when
	case st.Deactivate != "":
		return fmt.Sprintf(`dom.GetObject("%s").Active(false);`, st.Deactivate) + when
	}
	return when
}

// Scripts of the recorder.
const (
	// TraceScript reads the trace and empties it.
	TraceScript = `object t=dom.GetObject("` + TraceVar + `"); Write(t.Value()); t.State("");`
	// TimeScript reads the reference's clock.
	TimeScript = `Write(system.Date("%F %T"));`
	// BeatScript writes the heartbeat: the tracer must note it.
	BeatScript = `dom.GetObject("` + BeatVar + `").State("x"); Write("beat");`
	// SaveScript writes the database.
	SaveScript = `system.Save(); Write("saved");`
)

// ClockOffset is what the reference's clock file holds so that its clock shows start at the real
// time now and runs rate times as fast (libfaketime: "+<seconds>s x<rate>"; the offset counts
// from the moment the process starts, which is why the file is written just before).
func ClockOffset(start, now time.Time, rate int) string {
	return fmt.Sprintf("%+ds x%d\n", int64(start.Sub(now).Round(time.Second)/time.Second), rate)
}
