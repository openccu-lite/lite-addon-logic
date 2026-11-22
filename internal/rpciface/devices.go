// Package rpciface is the interface process "Logic": the addon's system variables and programs as
// devices of an XML-RPC interface of the kind rfd and hmipserver are, on a loopback port. The
// system lists it in InterfacesList.xml and lite-rpc offers it like the radios' interfaces, so
// other software reads, writes and follows variables and programs with the API it already uses -
// there is no addon-specific one.
//
// One device per object, its address made of the object's key (a rename changes the name, never
// the address): var-<key> of type LOGIC-VARIABLE, prg-<key> of type LOGIC-PROGRAM, each with the
// channel :0 every Homematic device has and its values on :1.
package rpciface

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Interface is the name the manifest declares.
const Interface = "Logic"

// Device types and address prefixes.
const (
	TypeVariable = "LOGIC-VARIABLE"
	TypeProgram  = "LOGIC-PROGRAM"
	prefixVar    = "var-"
	prefixPrg    = "prg-"
)

// Operations of a datapoint.
const (
	opRead  = 1
	opWrite = 2
	opEvent = 4
)

// Program is a program as the interface shows it.
type Program struct {
	Key      string
	Name     string
	Active   bool
	Operable bool
	// LastRun is the block that ran last: 1 the first rule, 2 the second ..., 0 SONST or nothing yet.
	LastRun int
}

// Backend is what the interface serves: the addon's variables and programs.
type Backend interface {
	Variables() []engine.VariableInfo
	Programs() []Program
	// SetVariable is a write from outside: limits hold, system values are read-only.
	SetVariable(key string, value any) error
	Acknowledge(key string) error
	SetActive(key string, active bool) error
	// Run evaluates a program (then false), or runs its first DANN without looking at the
	// conditions (then true: the CCU's "Start").
	Run(key string, then bool) error
}

// param is one datapoint's description.
type param struct {
	name  string
	typ   string // BOOL, FLOAT, INTEGER, ENUM, STRING, ACTION
	ops   int
	min   any
	max   any
	def   any
	unit  string
	list  []string
	order int
}

func (p param) description() map[string]any {
	d := map[string]any{"ID": p.name, "TYPE": p.typ, "OPERATIONS": p.ops, "FLAGS": 1, "TAB_ORDER": p.order,
		"MIN": p.min, "MAX": p.max, "DEFAULT": p.def, "UNIT": p.unit}
	if p.list != nil {
		d["VALUE_LIST"] = p.list
	}
	return d
}

// device is one object as a device: what its descriptions say, and what is needed to serve it.
type device struct {
	address  string
	typ      string
	key      string
	name     string
	params   []param
	firmware string
	variable *model.Variable // nil for a program
	readOnly bool
	operable bool
}

func (d *device) param(name string) *param {
	for i := range d.params {
		if d.params[i].name == name {
			return &d.params[i]
		}
	}
	return nil
}

// channelType names the value channel.
func (d *device) channelType() string {
	switch {
	case d.variable == nil:
		return "PROGRAM"
	case d.variable.Type == model.VarAlarm:
		return "ALARM"
	}
	return "VARIABLE"
}

func limit(p *float64, fallback float64) float64 {
	if p != nil && !math.IsInf(*p, 0) && !math.IsNaN(*p) {
		return *p
	}
	return fallback
}

func variableDevice(v *model.Variable) *device {
	d := &device{address: prefixVar + v.Key, typ: TypeVariable, key: v.Key, name: v.Name, variable: v, readOnly: engine.IsSystemValue(v.Key) || v.Computed != nil || (v.Helper != nil && v.Helper.Role != model.RoleTotal)}
	ops := opRead | opWrite | opEvent
	if d.readOnly {
		ops = opRead | opEvent
	}
	value := param{name: "VALUE", ops: ops}
	switch v.Type {
	case model.VarBool, model.VarAlarm:
		value.typ, value.min, value.max, value.def = "BOOL", false, true, false
	case model.VarNumber:
		value.typ, value.unit = "FLOAT", v.Unit
		value.min, value.max = limit(v.Min, -math.MaxFloat32), limit(v.Max, math.MaxFloat32)
		value.def = math.Max(0, value.min.(float64))
	case model.VarList:
		value.typ, value.min, value.max, value.def = "ENUM", 0, len(v.Entries)-1, 0
		value.list = append([]string{}, v.Entries...)
	default:
		value.typ, value.min, value.max, value.def = "STRING", "", "", ""
	}
	d.params = []param{value}
	if v.Type == model.VarAlarm {
		d.params = append(d.params,
			param{name: "ACKNOWLEDGE", typ: "ACTION", ops: opWrite, min: false, max: true, def: false, order: 1},
			param{name: "COUNT", typ: "INTEGER", ops: opRead | opEvent, min: 0, max: math.MaxInt32, def: 0, order: 2},
			param{name: "FIRST", typ: "INTEGER", ops: opRead | opEvent, min: 0, max: math.MaxInt32, def: 0, unit: "s", order: 3},
			param{name: "LAST", typ: "INTEGER", ops: opRead | opEvent, min: 0, max: math.MaxInt32, def: 0, unit: "s", order: 4})
	}
	d.fingerprint()
	return d
}

func programDevice(p Program) *device {
	d := &device{address: prefixPrg + p.Key, typ: TypeProgram, key: p.Key, name: p.Name, operable: p.Operable}
	d.params = []param{
		{name: "ACTIVE", typ: "BOOL", ops: opRead | opWrite | opEvent, min: false, max: true, def: false},
		{name: "RUN", typ: "ACTION", ops: opWrite, min: false, max: true, def: false, order: 1},
		{name: "RUN_THEN", typ: "ACTION", ops: opWrite, min: false, max: true, def: false, order: 2},
		{name: "LAST_RUN", typ: "INTEGER", ops: opRead | opEvent, min: 0, max: math.MaxInt32, def: 0, order: 3},
	}
	d.fingerprint()
	return d
}

// fingerprint makes the device's FIRMWARE out of its datapoints' descriptions. Software that
// keeps paramset descriptions per device type and firmware (the system's own cache does) then
// keeps one per shape - a number with other limits, a list with other entries - and asks again
// when a variable's definition changes.
func (d *device) fingerprint() {
	h := fnv.New32a()
	for _, p := range d.params {
		fmt.Fprintf(h, "%s|%s|%d|%v|%v|%v|%s|%s\n", p.name, p.typ, p.ops, p.min, p.max, p.def, p.unit, strings.Join(p.list, ";"))
	}
	d.firmware = "1." + strconv.FormatUint(uint64(h.Sum32()), 10)
}

func (d *device) channels() []string { return []string{d.address + ":0", d.address + ":1"} }

// descriptions are the device's and its two channels', as listDevices gives them.
func (d *device) descriptions() []any {
	dev := map[string]any{"ADDRESS": d.address, "TYPE": d.typ, "PARENT": "", "CHILDREN": d.channels(), "PARAMSETS": []string{"MASTER"},
		"FIRMWARE": d.firmware, "VERSION": 1, "FLAGS": 1, "INTERFACE": Interface, "RF_ADDRESS": 0, "ROAMING": 0, "RX_MODE": 0}
	channel := func(index int, typ string) map[string]any {
		return map[string]any{"ADDRESS": d.address + ":" + strconv.Itoa(index), "TYPE": typ, "PARENT": d.address, "PARENT_TYPE": d.typ, "INDEX": index,
			"PARAMSETS": []string{"MASTER", "VALUES"}, "VERSION": 1, "FLAGS": 1, "DIRECTION": 0, "AES_ACTIVE": 0,
			"LINK_SOURCE_ROLES": "", "LINK_TARGET_ROLES": ""}
	}
	return []any{dev, channel(0, "MAINTENANCE"), channel(1, d.channelType())}
}

// build makes the devices of everything the backend has, by address.
func build(b Backend) map[string]*device {
	out := map[string]*device{}
	for _, v := range b.Variables() {
		if v.Def != nil && model.ValidKey(v.Def.Key) {
			d := variableDevice(v.Def)
			out[d.address] = d
		}
	}
	for _, p := range b.Programs() {
		if model.ValidKey(p.Key) {
			d := programDevice(p)
			out[d.address] = d
		}
	}
	return out
}

func sorted(m map[string]*device) []*device {
	out := make([]*device, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].address < out[j].address })
	return out
}

func unix(t time.Time) int {
	if t.IsZero() {
		return 0
	}
	return int(t.Unix())
}

// wire converts a variable's value to what the datapoint carries: a list's entry as its index.
func wire(v *model.Variable, value any) any {
	switch v.Type {
	case model.VarList:
		s, _ := value.(string)
		if i := v.EntryIndex(s); i >= 0 {
			return i
		}
		return 0
	case model.VarBool, model.VarAlarm:
		b, _ := value.(bool)
		return b
	case model.VarNumber:
		f, _ := value.(float64)
		return f
	}
	s, _ := value.(string)
	return s
}

var errValueType = errors.New("the value does not fit the datapoint")

func truth(x any) (bool, bool) {
	switch v := x.(type) {
	case bool:
		return v, true
	case int:
		return v != 0, true
	case float64:
		return v != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return true, true
		case "false", "0", "":
			return false, true
		}
	}
	return false, false
}

// unwire converts what a client sent to the variable's own value: a list takes the index or the
// entry's label, a number a number in any spelling, a boolean true/false/1/0.
func unwire(v *model.Variable, x any) (any, error) {
	switch v.Type {
	case model.VarBool, model.VarAlarm:
		if b, ok := truth(x); ok {
			return b, nil
		}
	case model.VarNumber:
		switch n := x.(type) {
		case float64:
			return n, nil
		case int:
			return float64(n), nil
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
				return f, nil
			}
		}
	case model.VarList:
		switch n := x.(type) {
		case int:
			if n >= 0 && n < len(v.Entries) {
				return v.Entries[n], nil
			}
		case float64:
			if i := int(n); float64(i) == n && i >= 0 && i < len(v.Entries) {
				return v.Entries[i], nil
			}
		case string:
			if v.EntryIndex(n) >= 0 {
				return n, nil
			}
			if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && i >= 0 && i < len(v.Entries) {
				return v.Entries[i], nil
			}
		}
	default:
		switch s := x.(type) {
		case string:
			return s, nil
		case nil:
			return "", nil
		case bool, int, float64:
			return fmt.Sprint(s), nil
		}
	}
	return nil, errValueType
}
