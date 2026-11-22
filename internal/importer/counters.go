package importer

import (
	"sort"
	"strconv"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// eQ-3's counter variables become device helpers: the total, today's and yesterday's amounts
// are the helper's variables, with their values; the device's last counted reading (…OldVal)
// and the HM-ES-TX-WM's restart latch (…_DEVICE_RESET) go into what the helper remembers, so
// counting goes on without a jump; the scripts' other scratch variables are not needed. The
// channel comes from the id in the variable's name, the kind from the name and the channel's
// type. A variable whose channel the database does not hold stays a plain variable.

// What a counter variable is to its helper, beside the three roles.
const (
	partOld    = "old"
	partLatch  = "latch"
	partUnused = "unused"
)

// counterPart is what one of eQ-3's counter variables is.
type counterPart struct {
	kind    *model.HelperKind
	channel string // Interface/ADDRESS:CH
	chn     int
	what    string // a role, or partOld, partLatch, partUnused
}

func (p *counterPart) id() string { return p.kind.Kind + "@" + p.channel }

// helperGroup is one helper as the import finds it.
type helperGroup struct {
	part  *counterPart
	lead  string // the leading variable's key, once it is imported
	name  string // its name
	state model.HelperState
}

// The classic weather sensor's variables, as hm_autoconf names them.
const (
	rainToday     = "${sysVarRainToday}"
	rainYesterday = "${sysVarRainYesterday}"
)

// counterPartOf says what a variable is to a device helper; nil when it is none of eQ-3's
// counter variables, or its channel cannot be told.
func (m *imp) counterPartOf(v *regadom.Variable) *counterPart {
	if v.Name == rainToday || v.Name == rainYesterday {
		ch := m.db.Channels[v.ChannelID]
		ref := m.channelRef(v.ChannelID)
		if ch == nil || ref == "" || ch.Label != "WEATHER" {
			return nil
		}
		what := model.RoleToday
		if v.Name == rainYesterday {
			what = model.RoleYesterday
		}
		return &counterPart{kind: model.HelperKindOf(model.HelperRainHM), channel: ref, chn: v.ChannelID, what: what}
	}
	c := counterName.FindStringSubmatch(v.Name)
	if c == nil || (v.ValueType != regadom.TypeFloat && v.ValueType != regadom.TypeBinary) {
		return nil
	}
	chn, _ := strconv.Atoi(c[3])
	ch := m.db.Channels[chn]
	ref := m.channelRef(chn)
	if ch == nil || ref == "" {
		return nil
	}
	device := ""
	if dev := m.db.Devices[ch.DeviceID]; dev != nil {
		device = dev.Label
	}
	mid, what := c[2], model.RoleTotal
	switch {
	case strings.HasSuffix(c[4], "_DEVICE_RESET"):
		what = partLatch
	case strings.HasSuffix(c[4], "_RESET"), strings.HasSuffix(c[4], "_TMP_OLDVAL"):
		what = partUnused
	case strings.Contains(mid, "OldVal"):
		mid, what = strings.Replace(mid, "OldVal", "", 1), partOld
	case mid == "Today":
		mid, what = "", model.RoleToday
	case mid == "Yesterday":
		mid, what = "", model.RoleYesterday
	}
	kind := ""
	switch c[1] + mid {
	case "EnergyCounter":
		for _, k := range []string{model.HelperEnergy, model.HelperESI, model.HelperEnergyHM, model.HelperEnergyWM} {
			if model.HelperKindOf(k).Fits(device, ch.Label) {
				kind = k
			}
		}
	case "EnergyCounterFeedIn":
		kind = model.HelperFeedIn
	case "EnergyCounterGas":
		kind = model.HelperGasWM
	case "EnergyCounterIEC":
		kind = model.HelperIECWM
	case "HmIPRainCounter":
		kind = model.HelperRain
	case "HmIPSunshineCounter":
		kind = model.HelperSunshine
	case "Counter":
		kind = model.HelperWater
	}
	k := model.HelperKindOf(kind)
	if k == nil || (what != partOld && what != partLatch && what != partUnused && !k.HasRole(what)) {
		return nil
	}
	return &counterPart{kind: k, channel: ref, chn: chn, what: what}
}

// counters finds the helpers among the variables, before any is mapped: a helper needs its
// leading variable, and what the others are depends on it being there.
func (m *imp) counters() {
	m.parts = map[int]*counterPart{}
	m.groups = map[string]*helperGroup{}
	found := map[int]*counterPart{}
	for _, v := range m.db.Variables {
		if p := m.counterPartOf(v); p != nil {
			found[v.ID] = p
			if p.what == p.kind.Lead() && m.groups[p.id()] == nil {
				m.groups[p.id()] = &helperGroup{part: p, state: model.HelperState{Seen: true}}
			}
		}
	}
	for id, p := range found {
		if m.groups[p.id()] != nil {
			m.parts[id] = p
		}
	}
}

// The day's amounts the CCU kept as channel metadata, per kind: the keys of today's and of
// yesterday's, and what the variables made for them are called (with the channel's id).
var dayMeta = map[string]struct {
	today, yesterday string
	names            [2]string
	slot             int
}{
	model.HelperEnergy: {"energy0", "energy1", [2]string{"svEnergyCounterToday_", "svEnergyCounterYesterday_"}, 1},
	model.HelperESI:    {"energy0", "energy1", [2]string{"svEnergyCounterToday_", "svEnergyCounterYesterday_"}, 1},
	model.HelperFeedIn: {"energy0FeedIn", "energy1FeedIn", [2]string{"svEnergyCounterTodayFeedIn_", "svEnergyCounterYesterdayFeedIn_"}, 3},
	model.HelperWater:  {"counter0", "counter1", [2]string{"svCounterToday_", "svCounterYesterday_"}, 5},
}

// dayID is the ledger's id of a day variable the import makes: the database has no object
// for it, so it is a negative number made of the channel's id.
func dayID(chn, slot int) int { return -(chn*8 + slot) }

// helperVariable turns a counter variable's definition into its helper's variable.
func (m *imp) helperVariable(def *model.Variable, p *counterPart) {
	def.Helper = &model.Helper{Kind: p.kind.Kind, Channel: p.channel, Role: p.what}
	if def.Unit == "" {
		def.Unit = p.kind.Unit
	}
	if def.Max != nil && *def.Max > 1e300 {
		def.Max = nil // the CCU's "no limit"
	}
}

// helpers finishes the helpers once the variables are mapped: what each remembers, and the
// day's amounts of the kinds whose CCU scripts kept them in the channel's metadata.
func (m *imp) helpers(used map[string]bool) {
	m.res.Helpers = map[string]model.HelperState{}
	for _, id := range sortedGroups(m.groups) {
		g := m.groups[id]
		if g.lead == "" {
			continue
		}
		m.res.Helpers[g.lead] = g.state
		m.hist["helper: "+g.part.kind.Kind]++
		day, ok := dayMeta[g.part.kind.Kind]
		if !ok {
			continue
		}
		ch := m.db.Channels[g.part.chn]
		for i, role := range []string{model.RoleToday, model.RoleYesterday} {
			pid := dayID(g.part.chn, day.slot+i)
			meta := day.today
			if i == 1 {
				meta = day.yesterday
			}
			value, _ := num(ch.Meta[meta])
			def := &model.Variable{Name: day.names[i] + strconv.Itoa(g.part.chn), Type: model.VarNumber, Visible: true, Internal: true}
			m.helperVariable(def, &counterPart{kind: g.part.kind, channel: g.part.channel, chn: g.part.chn, what: role})
			def.Key = m.key("variable", pid, def.Name, used)
			rep := VariableReport{ID: pid, Name: def.Name, Key: def.Key, Type: string(model.VarNumber), Value: strconv.FormatFloat(value, 'f', -1, 64), Status: Imported,
				Notes: []Note{{Code: VarHelperDay, Args: []string{g.name}}}}
			if p := m.opt.Previous; p != nil && p.Variables[pid] == def.Key {
				rep.Updated = true
			}
			m.res.Values[def.Key] = value
			m.res.Variables = append(m.res.Variables, def)
			m.res.Ledger.Variables[pid] = def.Key
			m.res.Report.Variables = append(m.res.Report.Variables, rep)
		}
	}
}

func sortedGroups(m map[string]*helperGroup) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
