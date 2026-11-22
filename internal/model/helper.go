package model

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Helper makes a number variable one a device helper maintains: the running total of a
// device's counter, or the amount of today or of yesterday - what the CCU's WebUI kept in its
// svEnergyCounter…, svHmIPRainCounter… and their sibling variables with a hidden program each.
// The variables with the same kind and channel are one helper.
type Helper struct {
	// Kind is the device family, one of HelperKinds.
	Kind string `json:"kind"`
	// Channel is the channel whose counter is followed, "Interface/ADDRESS:CH".
	Channel string `json:"channel"`
	// Role is what this variable holds: the total, today's amount or yesterday's.
	Role string `json:"role"`
	// Off stops the helper: it adds nothing, and counts from the device's reading at the time
	// it is switched on again. Said on the helper's leading variable only.
	Off bool `json:"off,omitempty"`
}

// A helper's roles.
const (
	RoleTotal     = "total"
	RoleToday     = "today"
	RoleYesterday = "yesterday"
)

// HelperRoles lists the roles in the order they are shown.
var HelperRoles = []string{RoleTotal, RoleToday, RoleYesterday}

// The kinds of device helper: one per family of eQ-3's counter scripts.
const (
	HelperEnergy    = "energy"    // HmIP measuring actuators (PSM, BSM, FSM …)
	HelperFeedIn    = "feed-in"   // HmIP-PSMCO, ELV-SH-PSMCI: the energy fed in
	HelperESI       = "esi"       // HmIP-ESI: energy or gas, whichever the sensor reports
	HelperEnergyHM  = "energy-hm" // HomeMatic measuring actuators (HM-ES-PMSw…)
	HelperEnergyWM  = "energy-wm" // HM-ES-TX-WM: the electricity meter's pulses
	HelperGasWM     = "gas-wm"    // HM-ES-TX-WM: the gas meter
	HelperIECWM     = "iec-wm"    // HM-ES-TX-WM: the IEC sensor
	HelperRain      = "rain"      // HmIP weather sensors with a rain gauge
	HelperSunshine  = "sunshine"  // HmIP weather sensors
	HelperWater     = "water"     // HmIP-WSM, ELV-SH-WSM
	HelperRainHM    = "rain-hm"   // the HomeMatic weather sensor (HM-WDS100-C6-O)
	algoHmIP        = "hmip"
	algoESI         = "esi"
	algoHM          = "hm"
	algoWM          = "wm"
	algoIEC         = "iec"
	algoLast        = "last"
	scaleHmIP       = 1000
	scaleHM         = 100000
	helperPrecision = 6
)

// HelperKind says what a kind of helper follows and how it counts.
type HelperKind struct {
	Kind string `json:"kind"`
	// Counters are the channel's counter datapoints: every report of one is counted. (Only
	// the HmIP-ESI has two: its sensor reports energy or gas.)
	Counters []string `json:"counters"`
	// Flag is the datapoint that says what a smaller reading means: the HmIP devices'
	// …_OVERFLOW, the HomeMatic devices' BOOT. Empty where the family has none.
	Flag string `json:"flag,omitempty"`
	// Roles are the variables the helper can have; the first is its leading one, which every
	// helper has.
	Roles []string `json:"roles"`
	// Hour and Minute are the day's boundary on the system's clock: when today's amount
	// becomes yesterday's.
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
	// Unit is the counter's unit as the devices report it.
	Unit string `json:"unit"`
	// ChannelTypes are the channel types the family's devices have the counter on; Devices,
	// where the channel type is not enough, what the device's type must begin with (compared
	// without case).
	ChannelTypes []string `json:"channel_types"`
	Devices      []string `json:"devices,omitempty"`
	NotDevices   []string `json:"not_devices,omitempty"`
	algo         string
	scale        float64
}

var allRoles = []string{RoleTotal, RoleToday, RoleYesterday}

// HelperKinds lists the kinds in the order a form offers them.
var HelperKinds = []*HelperKind{
	{Kind: HelperEnergy, Counters: []string{"ENERGY_COUNTER"}, Flag: "ENERGY_COUNTER_OVERFLOW", Roles: allRoles, Unit: "Wh",
		ChannelTypes: []string{"ENERGIE_METER_TRANSMITTER"}, NotDevices: []string{"HmIP-ESI"}, algo: algoHmIP, scale: scaleHmIP},
	{Kind: HelperFeedIn, Counters: []string{"ENERGY_COUNTER_FEED_IN"}, Flag: "ENERGY_COUNTER_FEED_IN_OVERFLOW", Roles: allRoles, Unit: "Wh",
		ChannelTypes: []string{"ENERGIE_METER_TRANSMITTER"}, Devices: []string{"HmIP-PSMCO", "ELV-SH-PSMCI"}, algo: algoHmIP, scale: scaleHmIP},
	{Kind: HelperESI, Counters: []string{"ENERGY_COUNTER", "GAS_VOLUME"}, Roles: allRoles, Unit: "",
		ChannelTypes: []string{"ENERGIE_METER_TRANSMITTER"}, Devices: []string{"HmIP-ESI"}, algo: algoESI, scale: scaleHmIP},
	{Kind: HelperEnergyHM, Counters: []string{"ENERGY_COUNTER"}, Flag: "BOOT", Roles: allRoles, Unit: "Wh",
		ChannelTypes: []string{"POWERMETER"}, algo: algoHM, scale: scaleHM},
	{Kind: HelperEnergyWM, Counters: []string{"ENERGY_COUNTER"}, Flag: "BOOT", Roles: allRoles, Unit: "Wh",
		ChannelTypes: []string{"POWERMETER_IGL", "POWERMETER_IEC1"}, algo: algoWM, scale: scaleHM},
	{Kind: HelperGasWM, Counters: []string{"GAS_ENERGY_COUNTER"}, Flag: "BOOT", Roles: allRoles, Unit: "m³",
		ChannelTypes: []string{"POWERMETER_IGL", "POWERMETER_IEC1"}, algo: algoWM, scale: scaleHM},
	{Kind: HelperIECWM, Counters: []string{"IEC_ENERGY_COUNTER"}, Roles: allRoles, Unit: "",
		ChannelTypes: []string{"POWERMETER_IEC1"}, algo: algoIEC, scale: scaleHmIP},
	{Kind: HelperRain, Counters: []string{"RAIN_COUNTER"}, Flag: "RAIN_COUNTER_OVERFLOW", Roles: allRoles, Hour: 7, Unit: "mm",
		ChannelTypes: []string{"WEATHER_TRANSMIT"}, algo: algoHmIP, scale: scaleHmIP},
	{Kind: HelperSunshine, Counters: []string{"SUNSHINEDURATION"}, Flag: "SUNSHINEDURATION_OVERFLOW", Roles: allRoles, Hour: 7, Unit: "min",
		ChannelTypes: []string{"WEATHER_TRANSMIT"}, algo: algoHmIP, scale: scaleHmIP},
	{Kind: HelperWater, Counters: []string{"WATER_VOLUME"}, Flag: "WATER_VOLUME_OVERFLOW", Roles: allRoles, Unit: "l",
		ChannelTypes: []string{"FLOW_METER_TRANSMITTER"}, algo: algoHmIP, scale: scaleHmIP},
	{Kind: HelperRainHM, Counters: []string{"RAIN_COUNTER"}, Roles: []string{RoleToday, RoleYesterday}, Hour: 7, Minute: 30, Unit: "mm",
		ChannelTypes: []string{"WEATHER"}, algo: algoLast, scale: scaleHmIP},
}

// HelperKindOf answers a kind, nil when there is none of that name.
func HelperKindOf(kind string) *HelperKind {
	for _, k := range HelperKinds {
		if k.Kind == kind {
			return k
		}
	}
	return nil
}

// Lead is the role of the helper's leading variable: the one every helper of the kind has,
// and the one its state is kept with.
func (k *HelperKind) Lead() string { return k.Roles[0] }

// HasRole says whether a helper of the kind can have a variable of that role.
func (k *HelperKind) HasRole(role string) bool {
	for _, r := range k.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HelperState is what a helper remembers beside its variables' values; it is kept in the
// state file with the helper's leading variable.
type HelperState struct {
	// Old is the device's reading at the last report that was counted (the CCU's …OldVal
	// variable). Seen says there was one: a helper made here has none until the device
	// reports, and so counts from then - one that was imported continues from the CCU's.
	Old  float64 `json:"old"`
	Seen bool    `json:"seen,omitempty"`
	// Latch is the HM-ES-TX-WM's "the device restarted": set by a report with BOOT, it makes
	// the next reading count whole (the CCU's …_DEVICE_RESET variable).
	Latch bool `json:"latch,omitempty"`
	// Rolled is the day boundary up to which today's amount was moved to yesterday's.
	Rolled time.Time `json:"rolled,omitzero"`
}

// HelperStep is what one report of the counter does.
type HelperStep struct {
	// Delta is added to the total and to today's amount.
	Delta float64
	// Set says the total is not added to but set to Total (the first report of a HomeMatic
	// measuring actuator; a restart of an HM-ES-TX-WM that never counted).
	Set   bool
	Total float64
	// Note says what was unusual about the report, for the log: "restart", "overflow",
	// "overflow-unknown" (the reading wrapped and the datapoint's maximum is not known: only
	// the new reading is counted), "ignored", "first"; empty for a plain step.
	Note string
}

func trunc(x, scale float64) float64 { return math.Trunc(x * scale) }

// roundTo rounds what a helper stores: sums of device readings, which have three decimals at
// most, must not collect binary fractions.
func roundTo(x float64) float64 { return round(x, helperPrecision) }

// Step counts one report of the counter: dev is the reading, flag the value of the kind's
// flag datapoint (false when it has none or its value is not known), max the counter's
// maximum from the device's description (known says whether it is). It answers what to add
// and the state after it. The arithmetic is each family's script on the CCU, line by line
// (roadmap notes: counters.md) - apart from what a helper made here does with its first
// report: it only notes the reading.
func (k *HelperKind) Step(st HelperState, dev float64, flag bool, max float64, known bool) (HelperStep, HelperState) {
	if !st.Seen {
		st.Old, st.Seen = dev, true
		if k.algo == algoWM {
			st.Latch = false
		}
		return HelperStep{Note: "first"}, st
	}
	d, o := trunc(dev, k.scale), trunc(st.Old, k.scale)
	overflow := func() (float64, string) {
		if !known {
			return dev, "overflow-unknown"
		}
		return dev + max - st.Old, "overflow"
	}
	var step HelperStep
	switch k.algo {
	case algoHmIP:
		switch {
		case o <= d:
			step.Delta = dev - st.Old
		case flag:
			step.Delta, step.Note = overflow()
		default:
			step.Delta, step.Note = dev, "restart"
		}
		if d == 0 {
			return HelperStep{Note: "ignored"}, st
		}
		st.Old = dev
	case algoESI:
		if o <= d {
			step.Delta = dev - st.Old
		} else {
			step.Note = "restart"
		}
		if d == 0 {
			return HelperStep{Note: "ignored"}, st
		}
		st.Old = dev
	case algoIEC:
		if o > d {
			step.Delta, step.Note = overflow()
		} else {
			step.Delta = dev - st.Old
		}
		if d == 0 {
			return HelperStep{Note: "ignored"}, st
		}
		st.Old = dev
	case algoHM:
		if st.Old <= 0 {
			st.Old = dev
			return HelperStep{Set: true, Total: dev, Note: "first"}, st
		}
		switch {
		case flag && d < o:
			step.Delta, step.Note = dev, "restart"
		case d >= o:
			step.Delta = dev - st.Old
		case d > 0:
			step.Delta, step.Note = overflow()
		default:
			step.Note = "ignored"
		}
		if dev > 0 {
			st.Old = dev
		}
	case algoWM:
		if flag {
			// a report with BOOT: the device restarted; nothing is counted, the next reading
			// counts whole
			st.Latch = true
			if st.Old <= 0 {
				return HelperStep{Set: true, Total: 0, Note: "restart"}, st
			}
			return HelperStep{Note: "restart"}, st
		}
		switch {
		case st.Latch:
			step.Delta, step.Note = dev, "restart"
			st.Latch = d == 0
		case d >= o:
			step.Delta = dev - st.Old
		case d > 0:
			step.Delta, step.Note = overflow()
		default:
			step.Note = "ignored"
		}
		st.Old = dev
	case algoLast:
		step.Delta = dev - st.Old
		if step.Delta < 0 {
			if known {
				step.Delta, step.Note = step.Delta+max, "overflow"
			} else {
				step.Delta, step.Note = dev, "overflow-unknown"
			}
		}
		st.Old = dev
	}
	step.Delta = roundTo(step.Delta)
	return step, st
}

// Boundary answers the kind's last day boundary at or before now, on now's clock.
func (k *HelperKind) Boundary(now time.Time) time.Time {
	b := time.Date(now.Year(), now.Month(), now.Day(), k.Hour, k.Minute, 0, 0, now.Location())
	if b.After(now) {
		b = time.Date(now.Year(), now.Month(), now.Day()-1, k.Hour, k.Minute, 0, 0, now.Location())
	}
	return b
}

// NextBoundary answers the kind's first day boundary after now.
func (k *HelperKind) NextBoundary(now time.Time) time.Time {
	b := time.Date(now.Year(), now.Month(), now.Day(), k.Hour, k.Minute, 0, 0, now.Location())
	if !b.After(now) {
		b = time.Date(now.Year(), now.Month(), now.Day()+1, k.Hour, k.Minute, 0, 0, now.Location())
	}
	return b
}

// Fits says whether a channel of this type, on a device of this type, is one the kind's
// devices have their counter on.
func (k *HelperKind) Fits(deviceType, channelType string) bool {
	ok := false
	for _, t := range k.ChannelTypes {
		ok = ok || t == channelType
	}
	if !ok {
		return false
	}
	for _, d := range k.NotDevices {
		if hasPrefixFold(deviceType, d) {
			return false
		}
	}
	if len(k.Devices) == 0 {
		return true
	}
	for _, d := range k.Devices {
		if hasPrefixFold(deviceType, d) {
			return true
		}
	}
	return false
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// ID names the helper a variable belongs to: its kind and channel.
func (h *Helper) ID() string { return h.Kind + "@" + h.Channel }

// Refs are the datapoints the helper follows: its counters, then its flag.
func (h *Helper) Refs() []Ref {
	k := HelperKindOf(h.Kind)
	ch := ChannelRef(h.Channel)
	if k == nil || ch.IsZero() {
		return nil
	}
	out := []Ref{}
	for _, p := range k.Counters {
		out = append(out, Ref{Interface: ch.Interface, Address: ch.Address, Param: p})
	}
	if k.Flag != "" {
		out = append(out, Ref{Interface: ch.Interface, Address: ch.Address, Param: k.Flag})
	}
	return out
}

// HelperGroup is one helper: its variables by role.
type HelperGroup struct {
	Kind    *HelperKind
	Channel string
	// Vars are the helper's variables' keys by role.
	Vars map[string]string
	// Off says the helper is switched off.
	Off bool
}

// Lead is the key of the helper's leading variable.
func (g *HelperGroup) Lead() string { return g.Vars[g.Kind.Lead()] }

// Helpers lists the helpers the set's variables form, sorted by kind and channel: only
// variables without a problem (HelperProblem) are part of one.
func (s *Set) Helpers() []*HelperGroup {
	by := map[string]*HelperGroup{}
	for _, key := range sortedVars(s.Variables) {
		v := s.Variables[key]
		if v.Helper == nil || s.HelperProblem(v) != "" {
			continue
		}
		g := by[v.Helper.ID()]
		if g == nil {
			g = &HelperGroup{Kind: HelperKindOf(v.Helper.Kind), Channel: v.Helper.Channel, Vars: map[string]string{}}
			by[v.Helper.ID()] = g
		}
		g.Vars[v.Helper.Role] = key
		if v.Helper.Off {
			g.Off = true
		}
	}
	out := make([]*HelperGroup, 0, len(by))
	for _, g := range by {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind.Kind != out[j].Kind.Kind {
			return out[i].Kind.Kind < out[j].Kind.Kind
		}
		return out[i].Channel < out[j].Channel
	})
	return out
}

func sortedVars(m map[string]*Variable) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// HelperProblem says what keeps a helper's variable from being maintained, as far as the set
// can tell: another variable holds the same role of the same helper (the first by key wins),
// or the helper has no leading variable. Empty when nothing does, and for a variable that is
// no helper's.
func (s *Set) HelperProblem(v *Variable) string {
	h := v.Helper
	if h == nil {
		return ""
	}
	k := HelperKindOf(h.Kind)
	lead := false
	for _, key := range sortedVars(s.Variables) {
		o := s.Variables[key]
		if o.Helper == nil || o.Helper.ID() != h.ID() {
			continue
		}
		if o.Helper.Role == h.Role && key < v.Key {
			return fmt.Sprintf("%s already holds this helper's %s", key, h.Role)
		}
		lead = lead || o.Helper.Role == k.Lead()
	}
	if !lead && h.Role != k.Lead() {
		return fmt.Sprintf("the helper has no variable for its %s", k.Lead())
	}
	return ""
}

// HelperMembers lists the other variables of the helper a variable leads, by name, sorted;
// none for a variable that leads no helper.
func (s *Set) HelperMembers(key string) []string {
	v := s.Variables[key]
	out := []string{}
	if v == nil || v.Helper == nil || v.Helper.Role != HelperKindOf(v.Helper.Kind).Lead() {
		return out
	}
	for k, o := range s.Variables {
		if k != key && o.Helper != nil && o.Helper.ID() == v.Helper.ID() {
			out = append(out, o.Name)
		}
	}
	sort.Strings(out)
	return out
}
