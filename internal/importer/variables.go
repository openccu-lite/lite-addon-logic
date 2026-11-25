package importer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// The CCU's built-in objects.
const (
	idAlarmMessages   = 40
	idServiceMessages = 41
)

// builtinNames are the words the WebUI shows for its string-table placeholders.
var builtinNames = map[string][2]string{ // placeholder: de, en
	"sysVarPresence":               {"Anwesenheit", "Presence"},
	"sysVarPresenceMsg":            {"Anwesenheit", "Presence"},
	"sysVarPresencePresent":        {"anwesend", "present"},
	"sysVarPresenceNotPresent":     {"nicht anwesend", "not present"},
	"sysVarAlarmZone1":             {"Alarmzone 1", "Alarm zone 1"},
	"sysVarAlarmZone1Msg":          {"Alarmmeldung Alarmzone 1", "Alarm message alarm zone 1"},
	"sysVarAlarmZone1Triggered":    {"ausgelöst", "triggered"},
	"sysVarAlarmZone1NotTriggered": {"nicht ausgelöst", "not triggered"},
	"sysVarAlarmMessages":          {"Alarmmeldungen", "Alarm messages"},
	"sysVarAlarmMessagesCount":     {"Anzahl Alarmmeldungen", "Number of alarm messages"},
	"sysVarServiceMessages":        {"Servicemeldungen", "Service messages"},
	"sysVarServiceMessagesCount":   {"Anzahl Servicemeldungen", "Number of service messages"},
	"sysVarRainToday":              {"Regen heute", "Rain today"},
	"sysVarRainYesterday":          {"Regen gestern", "Rain yesterday"},
}

// roomNames are the words the WebUI shows for the rooms a CCU comes with: the database keeps
// their string-table keys as their names (roomLivingRoom), the WebUI translates them.
var roomNames = map[string][2]string{ // key: de, en
	"roomLivingRoom":     {"Wohnzimmer", "Living room"},
	"roomKitchen":        {"Küche", "Kitchen"},
	"roomBedroom":        {"Schlafzimmer", "Bedroom"},
	"roomChildrensRoom1": {"Kinderzimmer 1", "Children's room 1"},
	"roomChildrensRoom2": {"Kinderzimmer 2", "Children's room 2"},
	"roomOffice":         {"Büro", "Home office"},
	"roomBathroom":       {"Badezimmer", "Bathroom"},
	"roomGarage":         {"Garage", "Garage"},
	"roomHWR":            {"Hauswirtschaftsraum", "Utility room"},
	"roomGarden":         {"Garten", "Garden"},
	"roomTerrace":        {"Terrasse", "Terrace"},
}

// rooms gives a channel's rooms by the names the WebUI shows for them.
func (m *imp) rooms(channel int) []string {
	out := m.db.RoomsOf(channel)
	for i, r := range out {
		key := strings.TrimSuffix(strings.TrimPrefix(r, "${"), "}")
		if w, ok := roomNames[key]; ok {
			out[i] = w[0]
			if m.lang == "en" {
				out[i] = w[1]
			}
		}
	}
	return out
}

var placeholder = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// words replaces the WebUI's ${...} placeholders by what the WebUI shows for them; one it does
// not know keeps its name without the braces.
func (m *imp) words(s string) string {
	return placeholder.ReplaceAllStringFunc(s, func(p string) string {
		name := p[2 : len(p)-1]
		if w, ok := builtinNames[name]; ok {
			if m.lang == "en" {
				return w[1]
			}
			return w[0]
		}
		return name
	})
}

// counterName is eQ-3's naming of the variables its WebUI creates for metering devices:
// sv<Family>[OldVal|Today|Yesterday...]_<channel id>[_<address>].
var counterName = regexp.MustCompile(`^sv(EnergyCounter|HmIPRainCounter|HmIPSunshineCounter|Counter)([A-Za-z]*)_([0-9]+)(?:_(.+))?$`)

// counterProgram names the hidden programs that go with them.
var counterProgram = regexp.MustCompile(`^(prg(EnergyCounter|GasCounter|RainCounter|SunshineCounter|FlowCounter|DailySunshineRainCounter|SetEnergyValuesAtMidnight|SetCounterAtMidnight)|Regentagesmenge (aktualisieren|zurücksetzen)$)`)

// IsCounterProgram says whether a program's name is one of eQ-3's counter programs.
func IsCounterProgram(name string) bool { return counterProgram.MatchString(name) }

// collectUsed notes which variables the programs refer to: a built-in is only imported when one
// does.
func (m *imp) collectUsed() {
	for _, l := range m.db.Lines {
		if id, err := strconv.Atoi(l.Left.Text); err == nil && (l.Left.Type == regadom.TypeSystemID || l.Left.Type == regadom.TypeObjectID) {
			m.usedVar[id] = true
		}
	}
	for _, a := range m.db.Actions {
		if a.Param != regadom.DestScript {
			m.usedVar[a.TargetID] = true
		}
	}
}

func num(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f != f || f > 1.7e308 || f < -1.7e308 {
		return 0, false
	}
	return f, true
}

func isTrue(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && s != "0" && s != "false"
}

// entries splits a VALUE_LIST and makes every entry usable as a label: no empty one, none twice.
func entries(list string) (out []string, repaired bool) {
	seen := map[string]bool{}
	for i, e := range strings.Split(list, ";") {
		e = strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ").Replace(e))
		label := e
		if label == "" {
			label = fmt.Sprintf("(%d)", i)
		}
		for n := 2; seen[label]; n++ {
			label = fmt.Sprintf("%s (%d)", e, n)
		}
		repaired = repaired || label != e
		seen[label] = true
		out = append(out, label)
	}
	return out, repaired
}

func name(s string) string {
	s = strings.TrimSpace(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s))
	if r := []rune(s); len(r) > model.MaxName {
		s = string(r[:model.MaxName])
	}
	return s
}

// variables maps the variables, in the database's order.
func (m *imp) variables() {
	used := map[string]bool{}
	for _, v := range m.db.Variables {
		rep := VariableReport{ID: v.ID, Name: name(m.words(v.Name)), Status: Skipped}
		def := &model.Variable{Name: rep.Name, Note: m.words(v.Info), Visible: v.Visible(), Internal: v.Internal(), Logged: v.Logged}
		if len(def.Note) > model.MaxText {
			def.Note = def.Note[:model.MaxText]
		}
		var value any
		switch {
		case v.IsAlarm && strings.HasPrefix(v.Name, "WatchDog: ") && !m.usedVar[v.ID]:
			rep.Notes = add(rep.Notes, SkipWatchdog)
		case v.ID == idAlarmMessages || v.ID == idServiceMessages || (v.Name == "DutyCycle" && v.Info == "DutyCycle CCU"):
			// the CCU's counters and OpenCCU's duty cycle are system values here: they exist without
			// a file, the system feeds them, and a line on one reads it
			key := model.SystemDutyCycle
			switch v.ID {
			case idAlarmMessages:
				key = model.SystemAlarmMessages
			case idServiceMessages:
				key = model.SystemServiceMessages
			}
			m.vars[v.ID] = &model.Variable{Key: key, Name: rep.Name, Type: model.VarNumber}
			rep.Key, rep.Type = key, string(model.VarNumber)
			rep.Notes = add(rep.Notes, VarSystemValue)
		case v.ID < 100 && !m.usedVar[v.ID]:
			rep.Notes = add(rep.Notes, SkipSystem) // the gateway's own objects
		case v.IsAlarm || (v.ValueType == regadom.TypeBinary && v.SubType == regadom.SubAlarm):
			def.Type = model.VarAlarm
			rep.Notes = add(rep.Notes, VarAlarm)
		case v.ValueType == regadom.TypeBinary:
			def.Type = model.VarBool
			value = isTrue(v.Value.Text)
			if v.SubType == regadom.SubAction {
				rep.Notes = add(rep.Notes, VarAction)
			}
		case v.ValueType == regadom.TypeFloat, v.ValueType == regadom.TypeInteger && v.SubType != regadom.SubEnum:
			def.Type = model.VarNumber
			f, _ := num(v.Value.Text)
			value = f
		case v.ValueType == regadom.TypeInteger:
			def.Type = model.VarList
			var repaired bool
			def.Entries, repaired = entries(m.words(v.Meta["VALUE_LIST"]))
			if repaired {
				rep.Notes = add(rep.Notes, VarEntries)
			}
			idx, err := strconv.Atoi(v.Value.Text)
			if err != nil || idx < 0 || idx >= len(def.Entries) {
				rep.Notes = add(rep.Notes, VarListValue, v.Value.Text)
				idx = 0
			}
			value = def.Entries[idx]
		case v.ValueType == regadom.TypeString:
			def.Type = model.VarString
			value = v.Value.Text
		default:
			rep.Notes = add(rep.Notes, SkipUnknownType, strconv.Itoa(v.ValueType), strconv.Itoa(v.SubType))
		}
		if def.Type == "" {
			m.res.Report.Variables = append(m.res.Report.Variables, rep)
			continue
		}
		switch def.Type {
		case model.VarBool, model.VarAlarm:
			if f, t := m.words(v.Meta["BV0"]), m.words(v.Meta["BV1"]); f != "" || t != "" {
				def.Labels = &model.Labels{False: f, True: t}
			}
		case model.VarNumber:
			def.Unit = strings.TrimSpace(v.Meta["UNIT"])
			if f, ok := num(v.Meta["MIN"]); ok {
				def.Min = &f
			}
			if f, ok := num(v.Meta["MAX"]); ok {
				def.Max = &f
			}
			if def.Min != nil && def.Max != nil && *def.Min > *def.Max {
				def.Min, def.Max = nil, nil
				rep.Notes = add(rep.Notes, VarLimits)
			}
		}
		if v.System() && v.Name == "${sysVarPresence}" {
			def.StartValue = true
			rep.Notes = add(rep.Notes, VarPresence)
		}
		if v.ChannelID != regadom.None {
			def.Channel = m.channelRef(v.ChannelID)
		}
		if c := counterName.FindStringSubmatch(v.Name); c != nil {
			chn, _ := strconv.Atoi(c[3])
			at := m.channelRef(chn)
			rep.Notes = add(rep.Notes, VarCounter, c[1]+c[2], at)
			if ch := m.db.Channels[chn]; ch != nil && c[4] != "" && !strings.HasPrefix(c[4], ch.Address) {
				rep.Notes = add(rep.Notes, VarCounterAddr, c[4], ch.Address)
			}
			if m.parts[v.ID] == nil {
				rep.Notes = add(rep.Notes, VarCounterPlain)
			}
		}
		part := m.parts[v.ID]
		if part != nil {
			g := m.groups[part.id()]
			switch part.what {
			case partOld:
				g.state.Old, _ = num(v.Value.Text)
				rep.Notes = add(rep.Notes, VarHelperState)
			case partLatch:
				g.state.Latch = isTrue(v.Value.Text)
				rep.Notes = add(rep.Notes, VarHelperState)
			case partUnused:
				rep.Notes = add(rep.Notes, VarHelperUnused)
			default:
				m.helperVariable(def, part)
				rep.Notes = add(rep.Notes, VarHelper, part.kind.Kind, part.channel)
			}
			if def.Helper == nil && !m.usedVar[v.ID] {
				// kept inside the helper: no variable of its own - unless a program uses it
				m.res.Report.Variables = append(m.res.Report.Variables, rep)
				continue
			}
		}
		def.Key = m.key("variable", v.ID, def.Name, used)
		checked, err := def.Check()
		if err != nil {
			rep.Notes = add(rep.Notes, SkipInvalid, err.Error())
			m.res.Report.Variables = append(m.res.Report.Variables, rep)
			continue
		}
		rep.Key, rep.Type, rep.Entries, rep.Status = def.Key, string(def.Type), def.Entries, Imported
		if value != nil {
			rep.Value = fmt.Sprint(value)
			m.res.Values[def.Key] = value
		}
		if p := m.opt.Previous; p != nil && p.Variables[v.ID] == def.Key {
			rep.Updated = true
		}
		m.varKey[v.ID] = def.Key
		m.vars[v.ID] = checked
		m.res.Variables = append(m.res.Variables, checked)
		m.res.Ledger.Variables[v.ID] = def.Key
		m.res.Report.Variables = append(m.res.Report.Variables, rep)
		m.hist["variable: "+string(def.Type)]++
		if part != nil && def.Helper != nil && part.what == part.kind.Lead() {
			g := m.groups[part.id()]
			g.lead, g.name = def.Key, def.Name
		}
	}
	m.helpers(used)
}
