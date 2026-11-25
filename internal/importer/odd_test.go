package importer

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// build makes a database in memory for what no file at hand holds.
type build struct {
	db   *regadom.DB
	next int
}

func newBuild() *build {
	return &build{next: 1000, db: &regadom.DB{
		Interfaces: map[int]*regadom.Interface{}, Devices: map[int]*regadom.Device{}, Channels: map[int]*regadom.Channel{}, Datapoints: map[int]*regadom.Datapoint{},
		Calendars: map[int]*regadom.Calendar{}, Rules: map[int]*regadom.Rule{}, Conditions: map[int]*regadom.Condition{}, Lines: map[int]*regadom.Line{},
		Destinations: map[int]*regadom.Destination{}, Actions: map[int]*regadom.Action{}, Names: map[int]string{}}}
}

func (b *build) id() int { b.next++; return b.next }

func (b *build) obj(name string) regadom.Object {
	o := regadom.Object{ID: b.id(), Name: name, Enabled: true, AccessRights: regadom.AllRights, Flags: regadom.FlagVisible, Meta: map[string]string{}}
	b.db.Names[o.ID] = name
	return o
}

func (b *build) variable(name string, vt, st int, value string, meta map[string]string) *regadom.Variable {
	v := &regadom.Variable{Object: b.obj(name), ChannelID: regadom.None, ValueType: vt, SubType: st, Value: regadom.Value{Text: value, Type: vt}}
	for k, x := range meta {
		v.Meta[k] = x
	}
	b.db.Variables = append(b.db.Variables, v)
	return v
}

func (b *build) datapoint(iface, addr, param string) int {
	ch := &regadom.Channel{Object: b.obj("Kanal " + addr), Address: addr}
	b.db.Channels[ch.ID] = ch
	dp := &regadom.Datapoint{Object: b.obj(iface + "." + addr + "." + param), Interface: iface, Address: addr, Param: param, ChannelID: ch.ID}
	b.db.Datapoints[dp.ID] = dp
	return dp.ID
}

type (
	lineSpec   = regadom.Line
	actionSpec = regadom.Action
)

func builtin(id int, name, value string) *regadom.Variable {
	return &regadom.Variable{Object: regadom.Object{ID: id, Name: name, Flags: 10, Meta: map[string]string{}}, ValueType: itg, Value: val(value, itg), ChannelID: regadom.None}
}

type ruleSpec struct {
	sonst    bool
	blocks   [][]regadom.Line
	blockOps []int
	actions  []regadom.Action
	bor      bool
}

func val(text string, typ int) regadom.Value { return regadom.Value{Text: text, Type: typ} }

// on is a line on an object: comparison, trigger mode, value.
func on(id, type1, type2 int, r1 regadom.Value) regadom.Line {
	return regadom.Line{Op: 1, Type1: type1, Type2: type2, Left: val(strconv.Itoa(id), regadom.TypeSystemID), Right1: r1, ChannelID: regadom.None}
}

func set(target int, v regadom.Value) regadom.Action {
	return regadom.Action{Param: regadom.DestVariable, TargetID: target, Value: v, ChannelID: regadom.None}
}

func script(text string) regadom.Action {
	return regadom.Action{Param: regadom.DestScript, TargetID: regadom.None, Value: val(text, regadom.TypeString)}
}

func (b *build) program(name string, active bool, rules ...ruleSpec) *regadom.Program {
	p := &regadom.Program{Object: b.obj(name), Active: active, FirstRule: regadom.None}
	var prev *regadom.Rule
	for _, rs := range rules {
		r := &regadom.Rule{ID: b.id(), Op: 2, SubRule: regadom.None, ElseIf: !rs.sonst}
		for bi, lines := range rs.blocks {
			c := &regadom.Condition{ID: b.id(), Op: 2}
			if bi < len(rs.blockOps) {
				c.Op = rs.blockOps[bi]
			}
			for i := range lines {
				l := lines[i]
				l.ID = b.id()
				b.db.Lines[l.ID] = &l
				c.Lines = append(c.Lines, l.ID)
			}
			b.db.Conditions[c.ID] = c
			r.Conditions = append(r.Conditions, c.ID)
		}
		d := &regadom.Destination{ID: b.id(), BreakOnRestart: rs.bor}
		for i := range rs.actions {
			a := rs.actions[i]
			a.ID = b.id()
			b.db.Actions[a.ID] = &a
			d.Actions = append(d.Actions, a.ID)
		}
		b.db.Destinations[d.ID] = d
		r.Destination = d.ID
		b.db.Rules[r.ID] = r
		if prev == nil {
			p.FirstRule = r.ID
		} else {
			prev.SubRule = r.ID
		}
		prev = r
	}
	b.db.Programs = append(b.db.Programs, p)
	return p
}

func (b *build) run(t *testing.T, opt Options) *Result {
	t.Helper()
	if opt.Location == nil {
		opt.Location = berlin(t)
	}
	res := Import(&regadom.Source{Kind: "regadom", DB: b.db}, opt)
	for _, p := range res.Programs {
		if _, err := p.Check(); err != nil {
			t.Errorf("%s does not fit the format: %v", p.Key, err)
		}
	}
	return res
}

const (
	bin = regadom.TypeBinary
	flt = regadom.TypeFloat
	str = regadom.TypeString
	itg = regadom.TypeInteger
)

// The cases where the importer refuses to guess: the program comes in inactive, with the reason.
func TestInactiveReasons(t *testing.T) {
	b := newBuild()
	flag := b.variable("Merker", bin, regadom.SubBool, "0", nil)
	list := b.variable("Modus", itg, regadom.SubEnum, "0", map[string]string{"VALUE_LIST": "a;b"})
	number := b.variable("Zahl", flt, 0, "1", nil)
	text := b.variable("Text", str, regadom.SubString, "x", nil)
	dp := b.datapoint("HmIP-RF", "0001:1", "STATE")
	one := []regadom.Action{set(flag.ID, val("1", bin))}
	line := on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))
	with := func(f func(*regadom.Line)) [][]regadom.Line { l := line; f(&l); return [][]regadom.Line{{l}} }

	b.program("Auslöser 99", true, ruleSpec{blocks: with(func(l *regadom.Line) { l.Type2 = 99 }), actions: one})
	b.program("Vergleich 99", true, ruleSpec{blocks: with(func(l *regadom.Line) { l.Type1 = 99 }), actions: one})
	b.program("Listenindex", true, ruleSpec{blocks: [][]regadom.Line{{on(list.ID, regadom.CmpEquals, regadom.TrigChange, val("7", itg))}}, actions: []regadom.Action{set(list.ID, val("-1", itg))}})
	b.program("Text größer", true, ruleSpec{blocks: [][]regadom.Line{{on(text.ID, regadom.CmpGreater, regadom.TrigChange, val("a", str))}}, actions: one})
	b.program("Zahl als Text", true, ruleSpec{blocks: [][]regadom.Line{{on(number.ID, regadom.CmpEqual, regadom.TrigChange, val("abc", str))}}, actions: []regadom.Action{set(number.ID, val("abc", str))}})
	b.program("Bereich auf Text", true, ruleSpec{blocks: [][]regadom.Line{{on(text.ID, regadom.CmpInRange, regadom.TrigChange, val("1", flt))}}, actions: one})
	b.program("Logik als Text", true, ruleSpec{blocks: [][]regadom.Line{{on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("ja", str))}}, actions: []regadom.Action{set(flag.ID, val("ja", str))}})
	b.program("Gerät Bereich kaputt", true, ruleSpec{blocks: [][]regadom.Line{{on(dp, regadom.CmpInRange, regadom.TrigChange, val("x", flt))}}, actions: one})
	b.program("Gerät Wert kaputt", true, ruleSpec{blocks: [][]regadom.Line{{on(dp, regadom.CmpGreater, regadom.TrigChange, val("1", bin))}},
		actions: []regadom.Action{{Param: regadom.DestDatapoint, TargetID: dp, Value: val("", 99)}, {Param: regadom.DestDatapoint, TargetID: dp, Value: val("1;2", 99)}}})
	b.program("Gerät Bereich außen", true, ruleSpec{blocks: [][]regadom.Line{{on(dp, regadom.CmpOutOfRange, regadom.TrigChange, val("1", flt))}}, actions: one})
	b.program("Sonderwert Variable", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{set(number.ID, val("[SV]x", str))}})
	b.program("Verzögerung kaputt", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{
		{Param: regadom.DestVariable, TargetID: flag.ID, Value: val("1", bin), Delay: val("soon", regadom.TypeDelay)},
		{Param: regadom.DestVariable, TargetID: flag.ID, Value: val("1", bin), Delay: val("00:xx:00", regadom.TypeDelay)},
		{Param: regadom.DestVariable, TargetID: flag.ID, Value: val("1", bin), Delay: val("5", 77)}}})
	mixed := line
	mixed.Op = 2
	b.program("Gemischt in der Zeile", true, ruleSpec{blocks: [][]regadom.Line{{line, line, mixed}}, actions: one})
	b.program("Gemischt zwischen Blöcken", true, ruleSpec{blocks: [][]regadom.Line{{line}, {line}, {line}}, blockOps: []int{2, 1, 2}, actions: one})
	b.program("Zeit als Änderung", true, ruleSpec{blocks: [][]regadom.Line{{{Op: 1, Type1: regadom.CmpTime, Type2: regadom.TrigChange, Left: val("2026-01-01", regadom.TypeCurrentDate), Right1: val("5000", regadom.TypeObjectID)}}}, actions: one})
	b.db.Calendars[5000] = &regadom.Calendar{Object: regadom.Object{ID: 5000}, Type: 3, Unknown: []string{"timer-dp/neu"}}
	res := b.run(t, Options{Known: func(string) bool { return true }})

	for name, want := range map[string]string{
		"Auslöser 99":               ReasonTrigger,
		"Vergleich 99":              ReasonComparison,
		"Listenindex":               ReasonListIndex + "," + ReasonListIndex,
		"Text größer":               ReasonValue,
		"Zahl als Text":             ReasonValue,
		"Bereich auf Text":          ReasonValue,
		"Logik als Text":            ReasonValue,
		"Gerät Bereich kaputt":      ReasonValue,
		"Gerät Wert kaputt":         ReasonValue,
		"Gerät Bereich außen":       ReasonValue,
		"Sonderwert Variable":       ReasonSpecial,
		"Verzögerung kaputt":        ReasonDelay + "," + ReasonDelay + "," + ReasonDelay,
		"Gemischt in der Zeile":     ReasonOperators,
		"Gemischt zwischen Blöcken": ReasonOperators,
		"Zeit als Änderung":         ReasonUnknownField + "," + ReasonUnknownField,
	} {
		r := programReport(res, name)
		if r == nil || r.Status != Inactive || r.Active || codes(r.Reasons) != want {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if p := program(res, "ausloeser-99"); p.Rules[0].When.Groups[0].Lines[0].Trigger != model.TriggerCheck {
		t.Errorf("an unknown trigger mode must not trigger: %+v", p.Rules[0].When.Groups[0].Lines[0])
	}
	if p := program(res, "listenindex"); p.Rules[0].When.Groups[0].Lines[0].Value != "a" || p.Rules[0].Then[0].To != "a" {
		t.Errorf("list index: %s", model.EncodeProgram(p, nil))
	}
	if p := program(res, "geraet-wert-kaputt"); p.Rules[0].Then[0].Special != "?" || p.Rules[0].Then[1].Special != "1;2" {
		t.Errorf("unknown value type: %s", model.EncodeProgram(p, nil))
	}
	if p := program(res, "sonderwert-variable"); p.Rules[0].Then[0].Special != "[SV]x" {
		t.Errorf("special value on a variable: %s", model.EncodeProgram(p, nil))
	}
	if p := program(res, "zeit-als-aenderung"); p.Rules[0].When.Groups[0].Lines[0].Trigger != model.TriggerUpdate || p.Rules[0].When.Groups[0].Lines[0].Time.Kind != "daily" {
		t.Errorf("time line: %s", model.EncodeProgram(p, nil))
	}
	if p := program(res, "gemischt-in-der-zeile"); p.Rules[0].When.Groups[0].Op != model.All {
		t.Errorf("the second line decides the block's operator: %s", model.EncodeProgram(p, nil))
	}
	if p := program(res, "gemischt-zwischen-bloecken"); p.Rules[0].When.Op != model.All {
		t.Errorf("the second block decides the condition's operator: %s", model.EncodeProgram(p, nil))
	}
}

// What cannot be written at all is not imported, with the reason; a program that runs one of those
// goes with it.
func TestSkipped(t *testing.T) {
	b := newBuild()
	flag := b.variable("Merker", bin, regadom.SubBool, "0", nil)
	line := on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))
	one := []regadom.Action{set(flag.ID, val("1", bin))}
	ok := ruleSpec{blocks: [][]regadom.Line{{line}}, actions: one}

	b.program(leftover, true, ok)
	b.program("prgSetEnergyValuesAtMidnight4711", true, ok)
	loop := b.program("Schleife", true, ok)
	b.db.Rules[loop.FirstRule].SubRule = loop.FirstRule
	b.program("Sonst in der Mitte", true, ruleSpec{sonst: true, actions: one}, ok)
	b.program("Nur Sonst", true, ruleSpec{sonst: true, actions: one})
	noBlock := b.program("Block fehlt", true, ok)
	b.db.Rules[noBlock.FirstRule].Conditions = []int{424242}
	noLine := b.program("Zeile fehlt", true, ok)
	b.db.Conditions[b.db.Rules[noLine.FirstRule].Conditions[0]].Lines = []int{424243}
	noAction := b.program("Aktivität fehlt", true, ok)
	b.db.Destinations[b.db.Rules[noAction.FirstRule].Destination].Actions = []int{424244}
	b.program("Zeitmodul fehlt", true, ruleSpec{blocks: [][]regadom.Line{{{Type1: regadom.CmpTime, Type2: regadom.TrigUpdate, Right1: val("424245", regadom.TypeObjectID)}}}, actions: one})
	b.program("Linke Seite keine Zahl", true, ruleSpec{blocks: [][]regadom.Line{{{Type1: regadom.CmpEqual, Type2: regadom.TrigChange, Left: val("abc", regadom.TypeSystemID)}}}, actions: one})
	b.program("Ziel fehlt", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{set(424246, val("1", bin))}})
	b.program("Aktivitätsart 99", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{{Param: 99, TargetID: flag.ID}}})
	bad := b.datapoint("Kein gültiges Interface!", "0001:1", "STATE")
	b.program("Adresse ungültig", true, ruleSpec{blocks: [][]regadom.Line{{on(bad, regadom.CmpEquals, regadom.TrigChange, val("1", bin))}}, actions: one})
	b.program("", true, ok) // a program without a name cannot be a file
	// a chain: A runs B, B switches on a program further down that turns out unreadable
	b.program("Kette A", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{script(`dom.GetObject("Kette B").ProgramExecute();`)}})
	b.program("Kette B", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{script(`dom.GetObject("Kette C").Active(true);`)}})
	late := b.program("Kette C", true, ok)
	b.db.Rules[late.FirstRule].SubRule = late.FirstRule
	b.program("Bleibt", true, ok)
	res := b.run(t, Options{})

	for name, want := range map[string]string{
		leftover:                           SkipLeftover,
		"prgSetEnergyValuesAtMidnight4711": SkipCounter,
		"Schleife":                         SkipUnreadable,
		"Sonst in der Mitte":               SkipUnreadable,
		"Nur Sonst":                        SkipUnreadable,
		"Block fehlt":                      SkipDangling,
		"Zeile fehlt":                      SkipDangling,
		"Aktivität fehlt":                  SkipDangling,
		"Zeitmodul fehlt":                  SkipDangling,
		"Linke Seite keine Zahl":           SkipDangling,
		"Ziel fehlt":                       SkipDangling,
		"Aktivitätsart 99":                 SkipUnreadable,
		"Adresse ungültig":                 SkipDangling,
		"":                                 SkipInvalid,
		"Kette A":                          SkipInvalid,
		"Kette B":                          SkipInvalid,
		"Kette C":                          SkipUnreadable,
	} {
		r := programReport(res, strings.ReplaceAll(strings.ReplaceAll(name, "${", ""), "}", ""))
		if r == nil || r.Status != Skipped || r.Key != "" || codes(r.Reasons) != want || len(r.Notes) != 0 {
			t.Errorf("%q: %+v", name, r)
		}
	}
	if len(res.Programs) != 1 || res.Programs[0].Key != "bleibt" || len(res.Ledger.Programs) != 1 {
		t.Errorf("imported: %d, ledger %v", len(res.Programs), res.Ledger.Programs)
	}
}

// The quirks the importer names, and the values it converts.
func TestNotes(t *testing.T) {
	b := newBuild()
	flag := b.variable("Merker", bin, regadom.SubBool, "0", nil)
	line := on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))
	zero := set(flag.ID, val("1", bin))
	zero.Delay = val("00:00:00", regadom.TypeDelay) // Q-49: what the CCU stores for 24 h and more
	day := set(flag.ID, val("0", bin))
	day.Delay = val("23:59:59", regadom.TypeDelay)
	b.program("Null-Verzögerung", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{zero, day}})
	b.db.Calendars[5001] = &regadom.Calendar{Object: regadom.Object{ID: 5001}, Type: 9, Time: 1167613800} // 02:10
	b.db.Calendars[5002] = &regadom.Calendar{Object: regadom.Object{ID: 5002}, Type: 9, Time: 1167613800, Sot: 9}
	tl := func(id int) regadom.Line {
		return regadom.Line{Op: 1, Type1: regadom.CmpTime, Type2: regadom.TrigUpdate, Left: val("2026-01-01", regadom.TypeCurrentDate), Right1: val(strconv.Itoa(id), regadom.TypeObjectID)}
	}
	b.program("Zwei Uhr", true, ruleSpec{blocks: [][]regadom.Line{{tl(5001)}}, actions: []regadom.Action{set(flag.ID, val("1", bin))}})
	b.program("Muster", true, ruleSpec{blocks: [][]regadom.Line{{tl(5002)}}, actions: []regadom.Action{set(flag.ID, val("1", bin))}})
	leer := b.program("Ohne Regeln", false)
	_ = leer
	long := b.program("Lange Notiz", false, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{script(strings.Repeat("! lang ", 20000))}})
	long.Info = strings.Repeat("ä", 40000)
	starred := b.program("Start aus", true, ruleSpec{blocks: [][]regadom.Line{{line}}})
	no := false
	starred.StartupExec = &no
	starred.AccessRights = 0xFF000000
	starred.Flags = regadom.FlagInternal
	res := b.run(t, Options{})

	p := program(res, "null-verzoegerung")
	if p.Rules[0].Then[0].After != 0 || p.Rules[0].Then[1].After != model.Duration(24*time.Hour-time.Second) {
		t.Errorf("delays: %s", model.EncodeProgram(p, nil))
	}
	if r := programReport(res, "Null-Verzögerung"); codes(r.Notes) != "at-start,order,zero-delay" || !r.Active {
		t.Errorf("%+v", r)
	}
	if r := programReport(res, "Zwei Uhr"); codes(r.Notes) != "at-start,dst" {
		t.Errorf("%+v", r)
	}
	if r := programReport(res, "Muster"); codes(r.Notes) != "at-start,dst,time-pattern" {
		t.Errorf("%+v", r)
	}
	if p := program(res, "ohne-regeln"); p == nil || len(p.Rules) != 0 || p.Active {
		t.Errorf("a program without rules: %+v", p)
	}
	if p := program(res, "lange-notiz"); len(p.Note) != model.MaxText || len(p.Rules[0].Then[0].Script) != model.MaxText {
		t.Errorf("long texts: %d %d", len(p.Note), len(p.Rules[0].Then[0].Script))
	}
	if p := program(res, "start-aus"); p.AtStart || p.Operable || p.Visible || !p.Internal || !p.Active {
		t.Errorf("flags: %+v", p)
	}
	if r := programReport(res, "Start aus"); codes(r.Notes) != "not-at-start" {
		t.Errorf("%+v", r)
	}
}

// Built-ins, counters and the odd variable.
func TestVariables(t *testing.T) {
	b := newBuild()
	alarms := &regadom.Variable{Object: regadom.Object{ID: 40, Name: "${sysVarAlarmMessages}", Flags: 10, Meta: map[string]string{"MIN": "0", "MAX": "65000"}}, ValueType: itg, Value: val("2", itg), ChannelID: regadom.None}
	services := &regadom.Variable{Object: regadom.Object{ID: 41, Name: "${sysVarServiceMessages}", Flags: 10, Meta: map[string]string{}}, ValueType: itg, Value: val("1", itg), ChannelID: regadom.None}
	b.db.Variables = append(b.db.Variables, alarms, services)
	duty := b.variable("DutyCycle", flt, 0, "17.5", map[string]string{"UNIT": "%", "MIN": "-1", "MAX": "100"})
	duty.Info = "DutyCycle CCU"
	dog := b.variable("WatchDog: no-internet", bin, regadom.SubAlarm, "", nil)
	dog.IsAlarm = true
	gateway := &regadom.Variable{Object: regadom.Object{ID: 14, Name: "Gateway-DP", Meta: map[string]string{}}, ValueType: itg, Value: val("0", itg), ChannelID: regadom.None}
	b.db.Variables = append(b.db.Variables, gateway)
	lines := [][]regadom.Line{{
		on(40, regadom.CmpGreater, regadom.TrigChange, val("0", itg)),
		on(41, regadom.CmpGreater, regadom.TrigCheck, val("0", itg)),
		on(duty.ID, regadom.CmpGreater, regadom.TrigCheck, val("80", flt)),
		on(dog.ID, regadom.CmpEquals, regadom.TrigCheck, val("1", bin)),
		on(14, regadom.CmpEqual, regadom.TrigCheck, val("0", itg)),
	}}
	b.program("Meldungen", true, ruleSpec{blocks: lines})

	b.variable("Duty Cycle", flt, 0, "1", nil)
	b.variable("Liste kaputt", itg, regadom.SubEnum, "9", map[string]string{"VALUE_LIST": "a;;a;b\nc; "})
	b.variable("Grenzen verdreht", flt, 0, "nan", map[string]string{"MIN": "10", "MAX": "1", "UNIT": " kWh "})
	b.variable("Unbekannt", 77, 3, "", nil)
	b.variable("", bin, regadom.SubBool, "1", nil)
	b.variable(strings.Repeat("Sehr langer Name ", 30), bin, regadom.SubBool, "true", map[string]string{"BV0": "${unbekannterPlatzhalter}", "BV1": "an"})
	long := b.variable("Lange Beschreibung", str, regadom.SubString, "", nil)
	long.Info = strings.Repeat("x", 70000)
	hidden := b.variable("Versteckt", bin, regadom.SubAlarm, "", nil) // an alarm by its subtype alone
	hidden.Flags = 0
	// a counter whose name and channel disagree, and one bound to a channel without a datapoint
	dp := b.datapoint("HmIP-RF", "000A:6", "ENERGY_COUNTER")
	chn := b.db.Datapoints[dp].ChannelID
	b.variable("svEnergyCounter_"+strconv.Itoa(chn)+"_000B:6", flt, 0, "5", nil)
	dev := &regadom.Device{Object: b.obj("Gerät"), InterfaceID: 77}
	b.db.Devices[dev.ID] = dev
	b.db.Interfaces[77] = &regadom.Interface{Object: regadom.Object{ID: 77, Name: "BidCos-RF"}}
	ch := &regadom.Channel{Object: b.obj("Kanal ohne Datenpunkt"), Address: "LEQ1:1", DeviceID: dev.ID}
	b.db.Channels[ch.ID] = ch
	bound := b.variable("Gebunden", bin, regadom.SubBool, "0", nil)
	bound.ChannelID = ch.ID
	lost := b.variable("Kanal weg", bin, regadom.SubBool, "0", nil)
	lost.ChannelID = 31337
	noIface := &regadom.Channel{Object: b.obj("Kanal ohne Interface"), Address: "X:1", DeviceID: 31338}
	b.db.Channels[noIface.ID] = noIface
	orphan := b.variable("Kanal ohne Interface", bin, regadom.SubBool, "0", nil)
	orphan.ChannelID = noIface.ID
	res := b.run(t, Options{})

	// the three system values: no variable, no value - the lines read the system's own
	for _, key := range model.SystemValues {
		if variable(res, key) != nil {
			t.Errorf("%s was written as a variable", key)
		}
		if _, ok := res.Values[key]; ok {
			t.Errorf("%s got a value", key)
		}
	}
	if r := variableReport(res, "Alarmmeldungen"); r.Status != Skipped || codes(r.Notes) != VarSystemValue || r.Key != model.SystemAlarmMessages {
		t.Errorf("%+v", r)
	}
	var on []string
	program(res, "meldungen").Lines(func(_ int, l *model.Line) { on = append(on, l.Variable) })
	if strings.Join(on, ",") != "alarm-messages,service-messages,duty-cycle,watchdog-no-internet,gateway-dp" {
		t.Errorf("the lines are on %v", on)
	}
	if v := variable(res, "gateway-dp"); v == nil || res.Values["gateway-dp"] != 0.0 {
		t.Errorf("gateway-dp: %+v", v)
	}
	// a variable that happens to be named like a system value gets another key
	if v := variable(res, "duty-cycle-2"); v == nil || v.Name != "Duty Cycle" {
		t.Errorf("a variable named like a system value: %+v", v)
	}
	if v := variable(res, "watchdog-no-internet"); v == nil || v.Type != model.VarAlarm {
		t.Errorf("a watchdog alarm a program uses: %+v", v)
	}
	if r := programReport(res, "Meldungen"); r.Status != Imported || !r.Active {
		t.Errorf("%+v", r)
	}
	v := variable(res, "liste-kaputt")
	if !reflect.DeepEqual(v.Entries, []string{"a", "(1)", "a (2)", "b c", "(4)"}) || res.Values["liste-kaputt"] != "a" {
		t.Errorf("entries: %q = %v", v.Entries, res.Values["liste-kaputt"])
	}
	if r := variableReport(res, "Liste kaputt"); codes(r.Notes) != "entries-repaired,value-not-in-list" {
		t.Errorf("%+v", r)
	}
	v = variable(res, "grenzen-verdreht")
	if v.Min != nil || v.Max != nil || v.Unit != "kWh" || res.Values["grenzen-verdreht"] != 0.0 {
		t.Errorf("limits: %+v", v)
	}
	if r := variableReport(res, "Unbekannt"); r.Status != Skipped || codes(r.Notes) != SkipUnknownType || !reflect.DeepEqual(r.Notes[0].Args, []string{"77", "3"}) {
		t.Errorf("%+v", r)
	}
	if r := variableReport(res, ""); r.Status != Skipped || codes(r.Notes) != SkipInvalid {
		t.Errorf("a variable without a name: %+v", r)
	}
	v = variable(res, model.Slug(strings.Repeat("Sehr langer Name ", 30)))
	if v == nil || len([]rune(v.Name)) != model.MaxName || v.Labels.False != "unbekannterPlatzhalter" || res.Values[v.Key] != true {
		t.Errorf("long name: %+v", v)
	}
	if v := variable(res, "lange-beschreibung"); len(v.Note) != model.MaxText {
		t.Errorf("long note: %d", len(v.Note))
	}
	if v := variable(res, "versteckt"); v.Type != model.VarAlarm || v.Visible {
		t.Errorf("alarm by subtype: %+v", v)
	}
	r := variableReport(res, "svEnergyCounter_"+strconv.Itoa(chn)+"_000B:6")
	if codes(r.Notes) != "counter,counter-address,counter-plain" || !reflect.DeepEqual(r.Notes[0].Args, []string{"EnergyCounter", "HmIP-RF/000A:6"}) || !reflect.DeepEqual(r.Notes[1].Args, []string{"000B:6", "000A:6"}) {
		t.Errorf("counter: %+v", r.Notes)
	}
	if v := variable(res, "gebunden"); v.Channel != "BidCos-RF/LEQ1:1" {
		t.Errorf("the interface from the device: %q", v.Channel)
	}
	if variable(res, "kanal-weg").Channel != "" || variable(res, "kanal-ohne-interface").Channel != "" {
		t.Error("a channel the database cannot place was written")
	}
}

func TestOneLiner(t *testing.T) {
	for _, c := range []struct {
		in   string
		name string
		id   int
		kind model.ActionKind
		ok   bool
	}{
		{`dom.GetObject("Rollläden abends").ProgramExecute();`, "Rollläden abends", 0, model.ActionRun, true},
		{`dom.GetObject('Licht').ProgramExecute()`, "Licht", 0, model.ActionRun, true},
		{" dom . GetObject ( 1234 ) . Active ( true ) ; \n", "", 1234, model.ActionActivate, true},
		{`(dom.GetObject("Nachtabsenkung")).Active(false);`, "Nachtabsenkung", 0, model.ActionDeactivate, true},
		{`var x = dom.GetObject("Nachtabsenkung"); x.Active(false);`, "Nachtabsenkung", 0, model.ActionDeactivate, true},
		{`object prg=dom.GetObject(77);prg.ProgramExecute();`, "", 77, model.ActionRun, true},
		// not the two shapes: kept as a script
		{`var x = dom.GetObject("A"); y.Active(false);`, "", 0, "", false},
		{`(dom.GetObject("A").Active(false);`, "", 0, "", false},
		{`dom.GetObject("A")).Active(false);`, "", 0, "", false},
		{`dom.GetObject("A").Active(x);`, "", 0, "", false},
		{`dom.GetObject("A").State(true);`, "", 0, "", false},
		{`dom.GetObject("A").ProgramExecute(); dom.GetObject("B").ProgramExecute();`, "", 0, "", false},
		{`if (x) { dom.GetObject("A").ProgramExecute(); }`, "", 0, "", false},
		{`dom.GetObject("A\"B").ProgramExecute();`, "", 0, "", false},
		{``, "", 0, "", false},
	} {
		name, id, kind, ok := oneLiner(c.in)
		if name != c.name || id != c.id || kind != c.kind || ok != c.ok {
			t.Errorf("%q: %q %d %q %v", c.in, name, id, kind, ok)
		}
	}
	// a one-liner on something that is not an importable program stays a script
	b := newBuild()
	flag := b.variable("Merker", bin, regadom.SubBool, "0", nil)
	b.program("Ruft", true, ruleSpec{blocks: [][]regadom.Line{{on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))}}, actions: []regadom.Action{
		script(`dom.GetObject("Merker").ProgramExecute();`), script(`dom.GetObject(99999).Active(true);`), script(`dom.GetObject("prgEnergyCounter_1_A:1").ProgramExecute();`)}})
	b.program("prgEnergyCounter_1_A:1", true)
	res := b.run(t, Options{})
	p := program(res, "ruft")
	for _, a := range p.Rules[0].Then {
		if a.Kind != model.ActionScript {
			t.Errorf("%+v", a)
		}
	}
	if r := programReport(res, "Ruft"); codes(r.Reasons) != ReasonScript {
		t.Errorf("%+v", r)
	}
}

// Every field of a time module, as the WebUI's dialog stores it: the clock is the local time of
// an epoch value, "alle N" is the repetition value, an astro offset is plain seconds.
func TestTimeModules(t *testing.T) {
	m := &imp{loc: berlin(t)}
	at := func(h, min, s int) int64 { return time.Date(2007, 1, 1, h, min, s, 0, m.loc).Unix() }
	day := func(y int, mo time.Month, d int) int64 { return time.Date(y, mo, d, 0, 0, 0, 0, m.loc).Unix() }
	min := func(n int) model.Duration { return model.Duration(time.Duration(n) * time.Minute) }
	for _, c := range []struct {
		name    string
		in      regadom.Calendar
		want    model.TimeModule
		pattern bool
	}{
		{"daily", regadom.Calendar{Type: 9, Time: at(6, 30, 0)}, model.TimeModule{Kind: "daily", At: "06:30"}, false},
		{"seconds, every third day", regadom.Calendar{Type: 9, Time: at(6, 30, 15), RepVal: 3, Begin: day(2026, 12, 1)}, model.TimeModule{Kind: "daily", At: "06:30:15", Every: 3, From: "2026-12-01"}, false},
		{"the period of a daily module means nothing", regadom.Calendar{Type: 9, Time: at(6, 30, 0), Period: 3, RepVal: 1}, model.TimeModule{Kind: "daily", At: "06:30"}, false},
		{"midnight is a point, not all day", regadom.Calendar{Type: 9, Time: at(0, 0, 0)}, model.TimeModule{Kind: "daily", At: "00:00"}, false},
		{"all day", regadom.Calendar{Type: 9}, model.TimeModule{Kind: "daily", AllDay: true}, false},
		{"a range from the epoch's midnight", regadom.Calendar{Type: 9, Duration: 600}, model.TimeModule{Kind: "daily", At: "00:00", Duration: min(10)}, false},
		{"workdays", regadom.Calendar{Type: 9, Time: at(7, 0, 0), Weekdays: 31}, model.TimeModule{Kind: "daily", At: "07:00", Weekdays: []string{"mon", "tue", "wed", "thu", "fri"}}, false},
		{"the weekend", regadom.Calendar{Type: 9, Time: at(9, 0, 0), Weekdays: 96}, model.TimeModule{Kind: "daily", At: "09:00", Weekdays: []string{"sat", "sun"}}, false},
		{"the base 2000", regadom.Calendar{Type: 9, Time: time.Date(2000, 1, 1, 7, 0, 0, 0, m.loc).Unix(), Begin: day(2000, 1, 1)}, model.TimeModule{Kind: "daily", At: "07:00", From: "2000-01-01"}, false},
		{"legacy daily", regadom.Calendar{Type: 1}, model.TimeModule{Kind: "daily", AllDay: true}, false},
		{"legacy once", regadom.Calendar{Type: 0, TmRep: day(2027, 5, 1)}, model.TimeModule{Kind: "once", AllDay: true, Date: "2027-05-01"}, false},
		{"once: the date's day, whatever its time", regadom.Calendar{Type: 8, Time: at(12, 0, 0), TmRep: day(2027, 5, 1) + 3600}, model.TimeModule{Kind: "once", At: "12:00", Date: "2027-05-01"}, false},
		{"weekly, every second week", regadom.Calendar{Type: 5, Weekdays: 96, RepVal: 2, Duration: 600, Time: at(20, 0, 0), Begin: day(2026, 11, 2)}, model.TimeModule{Kind: "weekly", At: "20:00", Weekdays: []string{"sat", "sun"}, Every: 2, Duration: min(10), From: "2026-11-02"}, false},
		{"legacy weekdays", regadom.Calendar{Type: 2, Weekdays: 31}, model.TimeModule{Kind: "weekly", AllDay: true, Weekdays: []string{"mon", "tue", "wed", "thu", "fri"}}, false},
		{"interval all day", regadom.Calendar{Type: 4, Period: 300}, model.TimeModule{Kind: "interval", AllDay: true, Interval: min(5)}, false},
		{"interval with a range", regadom.Calendar{Type: 4, Period: 300, Time: at(22, 27, 0), Duration: 4500}, model.TimeModule{Kind: "interval", At: "22:27", Duration: min(75), Interval: min(5)}, false},
		{"interval without a period", regadom.Calendar{Type: 4}, model.TimeModule{Kind: "daily", AllDay: true}, true},
		{"monthly on a day, every third month", regadom.Calendar{Type: 6, Period: 31, RepVal: 3, Begin: day(2026, 1, 1)}, model.TimeModule{Kind: "monthly", AllDay: true, Day: 31, Every: 3, From: "2026-01-01"}, false},
		{"monthly on the first Wednesday, every sixth month", regadom.Calendar{Type: 6, Period: 1, Weekdays: 4, RepVal: 6, Time: at(8, 0, 0), Begin: day(2020, 7, 1)}, model.TimeModule{Kind: "monthly", At: "08:00", Nth: 1, Weekdays: []string{"wed"}, Every: 6, From: "2020-07-01"}, false},
		{"yearly on a day", regadom.Calendar{Type: 7, Period: 24, RepVal: 12}, model.TimeModule{Kind: "yearly", AllDay: true, Day: 24, Month: 12}, false},
		{"yearly on the fourth Thursday in November", regadom.Calendar{Type: 7, Period: 4, Weekdays: 8, RepVal: 11}, model.TimeModule{Kind: "yearly", AllDay: true, Nth: 4, Weekdays: []string{"thu"}, Month: 11}, false},
		{"tagsüber, as the stock WebUI stores it", regadom.Calendar{Type: 9, Sot: 3}, model.TimeModule{Kind: "daily", Astro: "day"}, false},
		{"45 minutes after sunrise, for an hour (OpenCCU: plain seconds)", regadom.Calendar{Type: 9, Sot: 3, Time: 2700, Duration: 3600}, model.TimeModule{Kind: "daily", Astro: "day", Offset: min(45), Duration: min(60)}, false},
		{"30 minutes before sunset", regadom.Calendar{Type: 9, Sot: 5, Time: 1800}, model.TimeModule{Kind: "daily", Astro: "before_sunset", Offset: min(30)}, false},
		{"an astro offset a script wrote as a time of day", regadom.Calendar{Type: 9, Sot: 5, Time: at(0, 30, 0)}, model.TimeModule{Kind: "daily", Astro: "before_sunset", Offset: min(30)}, false},
		{"plain sunrise", regadom.Calendar{Type: 9, Sot: 1}, model.TimeModule{Kind: "daily", Astro: "sunrise"}, false},
		{"plain sunrise with an offset is tagsüber", regadom.Calendar{Type: 9, Sot: 1, Time: 600}, model.TimeModule{Kind: "daily", Astro: "day", Offset: min(10)}, false},
		{"plain sunset with an offset is nachts", regadom.Calendar{Type: 9, Sot: 4, Time: 600}, model.TimeModule{Kind: "daily", Astro: "night", Offset: min(10)}, false},
		{"an astro kind nobody knows, all day", regadom.Calendar{Type: 9, Sot: 42}, model.TimeModule{Kind: "daily", AllDay: true}, true},
		{"an astro kind nobody knows, with a clock", regadom.Calendar{Type: 9, Sot: 42, Time: at(2, 10, 0)}, model.TimeModule{Kind: "daily", At: "02:10"}, true},
		{"validity in summer time", regadom.Calendar{Type: 9, Begin: day(2026, 7, 1), End: day(2026, 8, 31), RepCnt: 5}, model.TimeModule{Kind: "daily", AllDay: true, From: "2026-07-01", Until: "2026-08-31", Count: 5}, false},
		{"a begin with a time of day", regadom.Calendar{Type: 9, Begin: day(2026, 7, 1) + 3600}, model.TimeModule{Kind: "daily", AllDay: true, From: "2026-07-01 01:00"}, false},
		{"counted from the start", regadom.Calendar{Type: 9, Begin: -3600, RepVal: 3, RepCnt: 7}, model.TimeModule{Kind: "daily", AllDay: true, Every: 3, From: "1970-01-01", Count: 7}, false},
		{"numbers out of every range are left out", regadom.Calendar{Type: 9, Begin: -3600, RepVal: 20000, RepCnt: 2000000}, model.TimeModule{Kind: "daily", AllDay: true}, false},
	} {
		got, why, pattern := m.timeModule(&c.in)
		if why != "" || pattern != c.pattern || !reflect.DeepEqual(*got, c.want) {
			t.Errorf("%s: %+v, pattern %v, why %q", c.name, *got, pattern, why)
		}
		p := &model.Program{Key: "x", Name: "x", Rules: []model.Rule{{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Time: got, Trigger: model.TriggerUpdate}}}}}, Then: []model.Action{}}}}
		if _, err := p.Check(); err != nil {
			t.Errorf("%s does not fit the format: %v", c.name, err)
		}
		if back, err := model.DecodeProgram("programs/x.yaml", "x", model.EncodeProgram(p, nil)); err != nil || !reflect.DeepEqual(back.Rules[0].When.Groups[0].Lines[0].Time, got) {
			t.Errorf("%s does not read back: %v", c.name, err)
		}
	}
	// what cannot be run as stored: the caller takes the program inactive
	for name, in := range map[string]regadom.Calendar{
		"an unknown timer type":     {Type: 3},
		"once without a date":       {Type: 8},
		"weekly without a day":      {Type: 5},
		"monthly, day 0":            {Type: 6, RepVal: 1},
		"monthly, the sixth Monday": {Type: 6, Period: 6, Weekdays: 1},
		"monthly on two weekdays":   {Type: 6, Period: 1, Weekdays: 3},
		"yearly, month 13":          {Type: 7, Period: 24, RepVal: 13},
		"a range of a month":        {Type: 9, Time: at(1, 0, 0), Duration: 30 * 86400},
		"an end before the begin":   {Type: 9, Begin: day(2026, 8, 31), End: day(2026, 7, 1)},
	} {
		got, why, _ := m.timeModule(&in)
		if why == "" || got.Kind != "daily" || got.At != "00:00" {
			t.Errorf("%s: %+v, why %q", name, got, why)
		}
	}
	if !inDSTHour(&model.TimeModule{At: "02:59"}) || inDSTHour(&model.TimeModule{At: "03:00"}) || inDSTHour(&model.TimeModule{Astro: "day"}) {
		t.Error("inDSTHour")
	}
}

// Without a location in the options the backup's time zone is used, else the system's.
func TestLocation(t *testing.T) {
	b := newBuild()
	b.db.Calendars[5000] = &regadom.Calendar{Object: regadom.Object{ID: 5000}, Type: 9, Time: 1167629400} // 05:30 UTC
	b.program("Zeit", true, ruleSpec{blocks: [][]regadom.Line{{{Type1: regadom.CmpTime, Type2: regadom.TrigUpdate, Right1: val("5000", regadom.TypeObjectID)}}}})
	at := func(src *regadom.Source) string {
		return Import(src, Options{}).Programs[0].Rules[0].When.Groups[0].Lines[0].Time.At
	}
	if got := at(&regadom.Source{DB: b.db, Timezone: "Europe/Berlin"}); got != "06:30" {
		t.Errorf("Berlin: %s", got)
	}
	if got := at(&regadom.Source{DB: b.db, Timezone: "America/New_York"}); got != "00:30" {
		t.Errorf("New York: %s", got)
	}
	want := time.Unix(1167629400, 0).In(time.Local).Format("15:04")
	if got := at(&regadom.Source{DB: b.db, Timezone: "Nowhere/Nothing"}); got != want {
		t.Errorf("an unknown zone: %s, the system's is %s", got, want)
	}
	if got := at(&regadom.Source{DB: b.db}); got != want {
		t.Errorf("no zone: %s", got)
	}
}

// The CCU's own trigger table against the triggers the importer finds.
func TestCheckEvents(t *testing.T) {
	src := source(t, "fixture")
	if got := CheckEvents(src.DB); len(got) != 0 {
		t.Fatalf("the fixture: %v", got)
	}
	tr := Triggers(src.DB)
	// Markise bei Regen einfahren: Regen on change, Wassermelder Keller on update; the two
	// "nur prüfen" lines are not triggers
	if !reflect.DeepEqual(tr[1233], []int{1202, 1204}) {
		t.Errorf("triggers: %v", tr[1233])
	}
	// Zisterne prüfen: ReGa lists the time module of the "nur prüfen" line too - no difference
	if !reflect.DeepEqual(tr[1371], []int{1369}) {
		t.Errorf("triggers: %v", tr[1371])
	}
	// take a row away, add one
	var events []regadom.Event
	for _, e := range src.DB.Events {
		if !(e.Dst == 1233 && e.Src == 1204) {
			events = append(events, e)
		}
	}
	src.DB.Events = append(events, regadom.Event{Dst: 1306, Src: 1209}, regadom.Event{Dst: 1306, Src: 31337})
	got := CheckEvents(src.DB)
	want := []Note{
		{Code: EventMissing, Args: []string{"Markise bei Regen einfahren", "Wassermelder Keller (1204)"}},
		{Code: EventExtra, Args: []string{"Alarm bei Wasser", "Markise-Hinweis (1209)"}},
		{Code: EventExtra, Args: []string{"Alarm bei Wasser", "31337"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%+v", got)
	}
	res := Import(src, Options{Location: berlin(t)})
	if md := res.Report.Markdown("de"); !strings.Contains(md, "## Abweichungen von der Auslösertabelle") || !strings.Contains(md, "Wassermelder Keller (1204): eine Zeile löst darauf aus") {
		t.Error("the report does not show the differences")
	}
	// a program whose chain cannot be followed, a block and a line that are gone: left out
	b := newBuild()
	flag := b.variable("Merker", bin, regadom.SubBool, "0", nil)
	p := b.program("Kaputt", true, ruleSpec{blocks: [][]regadom.Line{{on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))}}})
	b.db.Rules[p.FirstRule].SubRule = 424242
	q := b.program("Lücken", true, ruleSpec{blocks: [][]regadom.Line{{on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))}, {}}})
	b.db.Rules[q.FirstRule].Conditions[1] = 424243
	b.db.Conditions[b.db.Rules[q.FirstRule].Conditions[0]].Lines = append(b.db.Conditions[b.db.Rules[q.FirstRule].Conditions[0]].Lines, 424244)
	tr = Triggers(b.db)
	if _, ok := tr[p.ID]; ok || !reflect.DeepEqual(tr[q.ID], []int{flag.ID}) {
		t.Errorf("%v", tr)
	}
	if got := CheckEvents(b.db); got != nil {
		t.Errorf("no table, nothing to compare: %v", got)
	}
}

// Two objects of one name, a string sent to a device as it was, "outside" on a device, a SONST
// that carries blocks, a device the database only half knows.
func TestRest(t *testing.T) {
	b := newBuild()
	first := b.variable("Regen", bin, regadom.SubBool, "0", nil)
	second := b.variable("Regen", bin, regadom.SubBool, "1", nil)
	dp := b.datapoint("HmIP-RF", "0001:8", "COMBINED_PARAMETER")
	level := b.datapoint("HmIP-RF", "0001:4", "LEVEL")
	ch := b.db.Channels[b.db.Datapoints[level].ChannelID]
	dev := &regadom.Device{Object: b.obj("Rollladen")}
	b.db.Devices[dev.ID] = dev
	ch.DeviceID = dev.ID
	outside := on(level, regadom.CmpOutOfRange, regadom.TrigChange, val("0.2", flt))
	outside.Right2 = val("0.8", flt)
	line := on(first.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))
	b.program("Doppelt", true,
		ruleSpec{blocks: [][]regadom.Line{{line, on(second.ID, regadom.CmpEquals, regadom.TrigCheck, val("1", bin)), outside}},
			actions: []regadom.Action{{Param: regadom.DestDatapoint, TargetID: dp, Value: val("L=50,C=4", str)}}},
		ruleSpec{sonst: true, blocks: [][]regadom.Line{{line}}})
	b.program("Doppelt", false)
	b.db.Services = 2
	res := b.run(t, Options{Known: func(string) bool { return true }})
	if variable(res, "regen") == nil || variable(res, "regen-2") == nil || program(res, "doppelt-2") == nil {
		t.Fatalf("keys: %v", res.Ledger)
	}
	p := program(res, "doppelt")
	lines := p.Rules[0].When.Groups[0].Lines
	if lines[0].Variable != "regen" || lines[1].Variable != "regen-2" || lines[2].Op != model.OpOutside || lines[2].From != 0.2 || lines[2].To != 0.8 {
		t.Errorf("%s", model.EncodeProgram(p, nil))
	}
	if a := p.Rules[0].Then[0]; a.To != "L=50,C=4" || a.Device.Param != "COMBINED_PARAMETER" {
		t.Errorf("a raw string for a device: %+v", a)
	}
	if !p.Rules[1].Else || p.Rules[1].When != nil || !p.Active {
		t.Errorf("SONST: %+v", p.Rules[1])
	}
	md := res.Report.Markdown("en")
	for _, want := range []string{"2 service messages of the CCU were not imported", "| `HmIP-RF/0001:4` | Kanal 0001:4 | Rollladen | – | present | Doppelt |"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report lacks %q:\n%s", want, md)
		}
	}
	notes := add(add(nil, "a"), "a", "x")
	if len(add(notes, "a", "x")) != 2 || !has(notes, "a") || has(notes, "b") {
		t.Errorf("%+v", notes)
	}
}

// The rooms a CCU comes with are kept by their string-table keys (roomLivingRoom); the report
// names them as the WebUI shows them, in the report's language. A room of the user's own keeps
// its name, and so does a key the WebUI does not know.
func TestRoomNames(t *testing.T) {
	db := &regadom.DB{Rooms: []regadom.Room{
		{Name: "roomLivingRoom", Channels: []int{1}},
		{Name: "${roomOffice}", Channels: []int{1}},
		{Name: "Keller", Channels: []int{1, 2}},
		{Name: "roomAttic", Channels: []int{2}},
	}}
	for _, c := range []struct {
		lang string
		ch   int
		want string
	}{
		{"de", 1, "Wohnzimmer, Büro, Keller"},
		{"en", 1, "Living room, Home office, Keller"},
		{"de", 2, "Keller, roomAttic"},
		{"de", 3, ""},
	} {
		m := &imp{db: db, lang: c.lang}
		if got := strings.Join(m.rooms(c.ch), ", "); got != c.want {
			t.Errorf("%s %d: %q, want %q", c.lang, c.ch, got, c.want)
		}
	}
	if db.Rooms[0].Name != "roomLivingRoom" {
		t.Error("the database's names were changed")
	}
}

// The report's summary counts in the singular where a number is one.
func TestSummaryCounts(t *testing.T) {
	for _, c := range []struct {
		lang, code string
		n          int
		want       string
	}{
		{"de", "n-programs", 1, "1 Programm"},
		{"de", "n-programs", 0, "0 Programme"},
		{"de", "n-variables", 1, "1 Systemvariable"},
		{"en", "n-programs", 1, "1 program"},
		{"en", "n-variables", 2, "2 system variables"},
	} {
		if got := count(c.lang, c.code, c.n); got != c.want {
			t.Errorf("%s %s %d: %q, want %q", c.lang, c.code, c.n, got, c.want)
		}
	}
}
