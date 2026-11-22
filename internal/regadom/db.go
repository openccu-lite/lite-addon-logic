// Package regadom reads a ReGa database - /etc/config/homematic.regadom, bare or inside a CCU
// backup (.sbk) - into plain Go values: the program graph (programs, rules, condition blocks and
// lines, activities), time modules, system and alarm variables, and the id-to-address tables of
// the device world. It reads and never writes, needs no network, and interprets nothing: what a
// field means for lite-addon-logic's model is the importer's business (package importer).
//
// The file is plain XML, declared ISO-8859-1 and actually mixed (see fixMojibake): one <obj> per
// DOM object (id, name, type, flags, metadata), wrapped in an element that names its class and
// grouped in about thirty typed maps. Programs are stored twice - nested under <program> and flat
// in rulemap, condmap, scondmap, destmap and sdestmap - so everything is indexed by id and
// references are followed. The ISO-8859-1 reader, the encoding repair, the name cleaning and the
// backup reader are copies of occulited's internal/regaimport, with their tests.
package regadom

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// None is ReGa's "no object" id.
const None = 65535

// Object flags (<objflgs>).
const (
	FlagVisible  = 0x1 // listed in the WebUI
	FlagInternal = 0x2 // "systemintern": the WebUI's generated helpers
	FlagSystem   = 0x8 // built-ins: presence, the message counters
)

// AllRights is the <accessrights> of an object every user may operate.
const AllRights = 0xFFFFFFFF

// Value types (ivt*), as far as programs use them.
const (
	TypeEmpty        = 0
	TypeBinary       = 2
	TypeFloat        = 4
	TypeInteger      = 16
	TypeObjectID     = 18
	TypeSystemID     = 19
	TypeString       = 20
	TypeCurrentDate  = 24
	TypeDelay        = 28
	TypeSpecialValue = 35
)

// Subtypes (ist*) the importer tells apart.
const (
	SubGeneric = 0
	SubBool    = 2
	SubAlarm   = 6
	SubString  = 11
	SubAction  = 28
	SubEnum    = 29
)

// Comparison types of a condition line (<type1>).
const (
	CmpEquals       = 1 // binary and value list
	CmpTime         = 3 // a time module
	CmpEqual        = 5
	CmpInRange      = 6 // "von ... bis kleiner"
	CmpOutOfRange   = 7
	CmpGreater      = 8
	CmpGreaterEqual = 9
	CmpLess         = 10
	CmpLessEqual    = 11
	CmpUnequal      = 12
)

// Trigger modes of a condition line (<type2>).
const (
	TrigLegacy = 1  // a fresh line's default; the editor leaves it on key lines
	TrigChange = 4  // "bei Änderung auslösen"
	TrigUpdate = 13 // "bei Aktualisierung auslösen"; a time module's "zu Zeitpunkten auslösen"
	TrigCheck  = 15 // "nur prüfen"
)

// Activity kinds (<param>).
const (
	DestDatapoint = 18
	DestVariable  = 19
	DestScript    = 20
)

// Alarm types (<type> of an alarm-dp).
const (
	AlarmGeneric = 0
	AlarmSystem  = 4
	AlarmService = 5
)

// Object is the header every DOM object carries.
type Object struct {
	ID           int
	Name         string // the encoding repaired, control characters replaced
	Type         int
	Enabled      bool
	AccessRights uint32
	Flags        int
	Meta         map[string]string
}

// Visible, Internal and System read the flags.
func (o *Object) Visible() bool  { return o.Flags&FlagVisible != 0 }
func (o *Object) Internal() bool { return o.Flags&FlagInternal != 0 }
func (o *Object) System() bool   { return o.Flags&FlagSystem != 0 }

// Value is a typed value as the file holds it (<val>, <type>).
type Value struct {
	Text string
	Type int
}

// Interface is an interface process the CCU knew (BidCos-RF, HmIP-RF, CUxD ...).
type Interface struct {
	Object
	URL string
}

// Device is a paired device.
type Device struct {
	Object
	Address     string
	InterfaceID int
	Label       string // the device type
	Channels    []int
}

// Channel is a device's channel.
type Channel struct {
	Object
	Address  string // ADDRESS:CH
	DeviceID int
	Label    string // the channel type
}

// Datapoint is a device datapoint; its name is Interface.ADDRESS:CH.PARAM.
type Datapoint struct {
	Object
	Interface string
	Address   string // ADDRESS:CH
	Param     string
	ChannelID int
	ValueType int
	SubType   int
	Ops       int
}

// Variable is a system variable or an alarm variable.
type Variable struct {
	Object
	Info      string // the description
	ChannelID int    // the channel binding, None without
	ValueType int
	SubType   int
	Value     Value // the stored value; an alarm variable has none
	IsAlarm   bool
	AlarmType int
	Logged    bool // a history datapoint archives it ("protokolliert")
}

// Calendar is a time module (calendar-dp wrapping timer-dp), every field as stored.
type Calendar struct {
	Object
	Type     int   // TimerType: 4 periodic, 5 weekly, 6 monthly, 7 yearly, 8 once, 9 daily
	Time     int64 // the start time of day as seconds of an epoch date; with Sot the offset
	Weekdays int   // bit mask, Monday = bit 0
	Period   int64
	Begin    int64 // the validity window
	End      int64
	Sot      int   // SunOffsetType
	Duration int64 // the range's length in seconds, 0 = a point
	TmRep    int64 // caltmrep
	RepVal   int64 // calrepval
	RepCnt   int64 // calrepcnt
	Unknown  []string
}

// Program is a program's own part; its chain starts at FirstRule.
type Program struct {
	Object
	Info        string // the description
	Active      bool   // the -ActiveState variable, not obj.enabled
	StartupExec *bool  // OpenCCU >= 3.89.14's <startupexec>; nil when the file has none
	FirstRule   int
	Unknown     []string // elements this parser does not know, as paths
}

// Rule is one link of the chain: WENN, SONST WENN or SONST.
type Rule struct {
	ID          int
	Op          int   // how the blocks join: 1 AND, 2 OR
	Conditions  []int // the blocks
	Destination int
	SubRule     int  // the next rule, None at the end
	ElseIf      bool // true: WENN / SONST WENN; false: SONST
	Unknown     []string
}

// Condition is a block of lines.
type Condition struct {
	ID      int
	Op      int // against the previous block: 1 AND, 2 OR
	Lines   []int
	Unknown []string
}

// Line is one condition line (s-cond).
type Line struct {
	ID        int
	Op        int // against the previous line
	Type1     int // the comparison
	Type2     int // the trigger mode
	Left      Value
	Right1    Value
	Right2    Value
	Negated   bool
	ChannelID int
	Unknown   []string
}

// Destination is a rule's activity list.
type Destination struct {
	ID             int
	Actions        []int
	BreakOnRestart bool // the checkbox "laufende Verzögerungen ... beenden"
	Unknown        []string
}

// Action is one activity (sdestination).
type Action struct {
	ID        int
	Param     int // DestDatapoint, DestVariable, DestScript
	TargetID  int
	ChannelID int
	Value     Value // for a script the whole text
	Delay     Value // empty = "sofort"; TypeDelay = "verzögert um" as HH:MM:SS
	Unknown   []string
}

// Event is one row of ReGa's own trigger table: a write of Src evaluates Dst.
type Event struct {
	Dst int
	Src int
}

// Room is a room with its member channels.
type Room struct {
	ID       int
	Name     string
	Channels []int
}

// Repair is a name the reader changed.
type Repair struct {
	ID     int
	Before string
	After  string
}

// DB is a parsed database.
type DB struct {
	Version   string
	Serial    string
	Latitude  string
	Longitude string

	Interfaces map[int]*Interface
	Devices    map[int]*Device
	Channels   map[int]*Channel
	Datapoints map[int]*Datapoint

	Variables []*Variable // system and alarm variables in file order, service messages left out
	Services  int         // the service-message alarms that were left out
	Calendars map[int]*Calendar
	Programs  []*Program // in file order

	Rules        map[int]*Rule
	Conditions   map[int]*Condition
	Lines        map[int]*Line
	Destinations map[int]*Destination
	Actions      map[int]*Action

	Events  []Event
	Rooms   []Room
	Names   map[int]string // every object's name by id
	Repairs []Repair
}

// Variable looks a variable up by id.
func (db *DB) Variable(id int) *Variable {
	for _, v := range db.Variables {
		if v.ID == id {
			return v
		}
	}
	return nil
}

// Program looks a program up by id.
func (db *DB) Program(id int) *Program {
	for _, p := range db.Programs {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// ProgramByName finds the first program of that name.
func (db *DB) ProgramByName(name string) *Program {
	for _, p := range db.Programs {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// Chain is a program's rules in order. A reference to a rule the file does not hold, or a chain
// that loops, is an error: the program cannot be read.
func (db *DB) Chain(p *Program) ([]*Rule, error) {
	var out []*Rule
	seen := map[int]bool{}
	for id := p.FirstRule; id != None; {
		r := db.Rules[id]
		if r == nil {
			return nil, fmt.Errorf("program %d %q: rule %d is not in the file", p.ID, p.Name, id)
		}
		if seen[id] {
			return nil, fmt.Errorf("program %d %q: its rules form a loop at %d", p.ID, p.Name, id)
		}
		seen[id] = true
		out = append(out, r)
		id = r.SubRule
	}
	return out, nil
}

// RoomsOf names the rooms a channel is in.
func (db *DB) RoomsOf(channel int) []string {
	var out []string
	for _, r := range db.Rooms {
		for _, c := range r.Channels {
			if c == channel {
				out = append(out, r.Name)
				break
			}
		}
	}
	return out
}

// ErrNotRegadom is returned for XML that is not a ReGa database.
var ErrNotRegadom = errors.New("regadom: not a ReGa database (no <dom> element)")

// Parse reads a database.
func Parse(r io.Reader) (*DB, error) {
	db, _, err := parse(r)
	return db, err
}

// parse is Parse; it answers the elements as well.
func parse(r io.Reader) (*DB, *node, error) {
	root, err := parseTree(r)
	if err != nil {
		return nil, nil, err
	}
	dom := root.child("dom")
	if dom == nil {
		return nil, nil, ErrNotRegadom
	}
	db := &DB{
		Version:      dom.str("version"),
		Interfaces:   map[int]*Interface{},
		Devices:      map[int]*Device{},
		Channels:     map[int]*Channel{},
		Datapoints:   map[int]*Datapoint{},
		Calendars:    map[int]*Calendar{},
		Rules:        map[int]*Rule{},
		Conditions:   map[int]*Condition{},
		Lines:        map[int]*Line{},
		Destinations: map[int]*Destination{},
		Actions:      map[int]*Action{},
		Names:        map[int]string{},
	}
	x := &extractor{db: db, vars: map[int]*Variable{}, programs: map[int]*Program{}, enums: map[int]*enum{}, logged: map[int]bool{}, helpers: map[int]bool{}}
	x.walk(dom, false)
	x.finish()
	return db, root, nil
}

type enum struct {
	id      int
	name    string
	members []int
}

type extractor struct {
	db       *DB
	vars     map[int]*Variable
	programs map[int]*Program
	enums    map[int]*enum
	logged   map[int]bool
	helpers  map[int]bool // the programs' -ActiveState variables
}

// object reads an <obj> header and records the name, repairing it.
func (x *extractor) object(n *node) (Object, bool) {
	if n == nil {
		return Object{}, false
	}
	id, err := strconv.Atoi(n.str("id"))
	if err != nil {
		return Object{}, false
	}
	o := Object{
		ID:           id,
		Type:         int(n.num("type", 0)),
		Enabled:      n.str("enabled") != "0",
		AccessRights: uint32(n.num("accessrights", AllRights)),
		Flags:        int(n.num("objflgs", FlagVisible)),
		Meta:         map[string]string{},
	}
	rawName := ""
	if c := n.child("name"); c != nil {
		rawName = c.text // as decoded, before the repair
	}
	o.Name = strings.TrimSpace(cleanName(rawName))
	if md := n.child("metadata"); md != nil {
		prop := ""
		for _, k := range md.kids {
			switch k.name {
			case "property":
				prop = strings.TrimSpace(k.text)
			case "value":
				if prop != "" {
					o.Meta[prop] = fixMojibake(k.text)
					prop = ""
				}
			}
		}
	}
	if _, dup := x.db.Names[id]; !dup {
		x.db.Names[id] = o.Name
		if before := strings.TrimSpace(rawName); before != o.Name {
			x.db.Repairs = append(x.db.Repairs, Repair{ID: id, Before: before, After: o.Name})
		}
	}
	return o, true
}

func value(n *node) Value {
	if n == nil {
		return Value{}
	}
	t := int(n.num("type", 0))
	text := n.raw("val")
	if t != TypeString {
		text = strings.TrimSpace(text)
	}
	return Value{Text: text, Type: t}
}

// unknown lists the child elements of n that are not in known, as "<parent>/<child>".
func unknown(n *node, known string) []string {
	var out []string
	for _, k := range n.kids {
		if !strings.Contains(known, " "+k.name+" ") {
			out = append(out, n.name+"/"+k.name)
		}
	}
	return out
}

// The elements each class is known to have. Anything else in a file is reported, and the importer
// takes the program inactive: better than guessing what a newer ReGa meant.
const (
	knownProgram     = " obj type valid-from valid-until day-types day-prof-vals day-prg prgrule objmap timer-dp var-dp prgcpid prgcnd prginfo breakrestart trigdelay startupexec "
	knownRule        = " obj rule-op rule-conds destobj subruleid rule-trig rule-trigval rule-parent rule-elseif rule-prg "
	knownCondition   = " obj op-type objid-dest val-dest delay sconds objid-par "
	knownLine        = " obj op-type type1 type2 lval rval1 rval2 neg trgchnid parid owncal "
	knownDestination = " obj enabled dests trigdp bor dparid dprgid "
	knownAction      = " obj enabled trgdpid trgval trgvalparam param trgchnid sdparid sdprgid "
	knownCalendar    = " timer-dp calduration caltmrep calrepval calrepcnt "
	knownTimer       = " dp type time weekdays period begin end sot state-cto "
)

func (x *extractor) walk(n *node, inProgram bool) {
	switch n.name {
	case "ifc":
		if o, ok := x.object(n.child("obj")); ok && x.db.Interfaces[o.ID] == nil {
			x.db.Interfaces[o.ID] = &Interface{Object: o, URL: n.str("ifc-url")}
		}
	case "device":
		if o, ok := x.object(n.child("obj")); ok && x.db.Devices[o.ID] == nil {
			x.db.Devices[o.ID] = &Device{Object: o, Address: n.str("devadr"), InterfaceID: int(n.num("devifc", None)),
				Label: n.str("devlabel"), Channels: n.child("device-chns").oids()}
		}
	case "channel":
		if o, ok := x.object(n.child("obj")); ok && x.db.Channels[o.ID] == nil {
			x.db.Channels[o.ID] = &Channel{Object: o, Address: n.str("chnadr"), DeviceID: int(n.num("deviceid", None)), Label: n.str("chnlabel")}
		}
	case "hss-dp":
		x.datapoint(n)
	case "var-dp":
		if n.child("dp") != nil && !inProgram {
			x.variable(n, false)
		}
	case "alarm-dp":
		x.variable(n, true)
	case "hist-dp":
		if id, err := strconv.Atoi(n.str("dp-archive")); err == nil {
			x.logged[id] = true
		}
	case "calendar-dp":
		x.calendar(n)
	case "program":
		x.program(n)
		inProgram = true
	case "rule":
		x.rule(n)
	case "condition":
		x.condition(n)
	case "s-cond":
		x.line(n)
	case "destination":
		x.destination(n)
	case "sdestination":
		x.action(n)
	case "enum":
		if o, ok := x.object(n.child("obj")); ok && x.enums[o.ID] == nil {
			x.enums[o.ID] = &enum{id: o.ID, name: o.Name, members: n.child("enel").oids()}
		}
	case "event":
		dst, err1 := strconv.Atoi(n.str("oidDst"))
		src, err2 := strconv.Atoi(n.str("oidSrc"))
		if err1 == nil && err2 == nil {
			x.db.Events = append(x.db.Events, Event{Dst: dst, Src: src})
		}
	case "gateway-config":
		x.db.Serial = n.str("serial-number")
		x.db.Latitude = n.str("latitude")
		x.db.Longitude = n.str("longitude")
	case "obj":
		x.object(n) // every other object: only its name, for the report
		return
	}
	for _, k := range n.kids {
		x.walk(k, inProgram)
	}
}

func (x *extractor) datapoint(n *node) {
	dp := n.child("dp")
	o, ok := x.object(dp.child("obj"))
	if !ok || x.db.Datapoints[o.ID] != nil {
		return
	}
	d := &Datapoint{Object: o, Address: n.str("hss-adr"), Param: n.str("hss-id"), ChannelID: int(dp.num("chnid", None)),
		ValueType: int(dp.num("valtype", 0)), SubType: int(dp.num("subtype", 0)), Ops: int(dp.num("op", 0))}
	// the name is Interface.ADDRESS:CH.PARAM - the only place the interface's name appears
	if i := strings.IndexByte(o.Name, '.'); i > 0 {
		d.Interface = o.Name[:i]
		rest := o.Name[i+1:]
		if j := strings.LastIndexByte(rest, '.'); j > 0 {
			if d.Address == "" {
				d.Address = rest[:j]
			}
			if d.Param == "" {
				d.Param = rest[j+1:]
			}
		}
	}
	x.db.Datapoints[o.ID] = d
}

func (x *extractor) variable(n *node, alarm bool) {
	dp := n.child("dp")
	o, ok := x.object(dp.child("obj"))
	if !ok || x.vars[o.ID] != nil {
		return
	}
	v := &Variable{Object: o, Info: strings.TrimSpace(cleanName(dp.raw("dp-info"))), ChannelID: int(dp.num("chnid", None)),
		ValueType: int(dp.num("valtype", 0)), SubType: int(dp.num("subtype", 0)), IsAlarm: alarm}
	if alarm {
		v.AlarmType = int(n.num("type", AlarmGeneric))
		if v.AlarmType == AlarmService {
			x.db.Services++
			return
		}
	} else {
		v.Value = Value{Text: n.raw("var"), Type: int(n.num("varType", 0))}
		if v.Value.Type != TypeString {
			v.Value.Text = strings.TrimSpace(v.Value.Text)
		}
	}
	x.vars[o.ID] = v
	x.db.Variables = append(x.db.Variables, v)
}

func (x *extractor) calendar(n *node) {
	t := n.child("timer-dp")
	o, ok := x.object(t.path("dp", "obj"))
	if !ok || x.db.Calendars[o.ID] != nil {
		return
	}
	c := &Calendar{Object: o, Type: int(t.num("type", 0)), Time: t.num("time", 0), Weekdays: int(t.num("weekdays", 0)),
		Period: t.num("period", 0), Begin: t.num("begin", 0), End: t.num("end", 0), Sot: int(t.num("sot", 0)),
		Duration: n.num("calduration", 0), TmRep: n.num("caltmrep", 0), RepVal: n.num("calrepval", 0), RepCnt: n.num("calrepcnt", 0)}
	c.Unknown = append(unknown(n, knownCalendar), unknown(t, knownTimer)...)
	x.db.Calendars[o.ID] = c
}

func (x *extractor) program(n *node) {
	o, ok := x.object(n.child("obj"))
	if !ok || x.programs[o.ID] != nil {
		return
	}
	p := &Program{Object: o, Info: strings.TrimSpace(cleanName(n.raw("prginfo"))), FirstRule: None, Active: o.Enabled}
	if id, err := strconv.Atoi(n.path("prgrule", "rule", "obj").str("id")); err == nil {
		p.FirstRule = id
	}
	// "aktiv" is the value of the program's -ActiveState variable; obj.enabled is 1 for every
	// program, the inactive ones included
	if v := n.path("var-dp", "var-dp"); v != nil {
		p.Active = v.str("var") != "0"
		if id, err := strconv.Atoi(v.path("dp", "obj").str("id")); err == nil {
			x.helpers[id] = true
		}
	}
	if s := n.child("startupexec"); s != nil {
		b := strings.TrimSpace(s.text) != "0"
		p.StartupExec = &b
	}
	p.Unknown = unknown(n, knownProgram)
	x.programs[o.ID] = p
	x.db.Programs = append(x.db.Programs, p)
}

func (x *extractor) rule(n *node) {
	o, ok := x.object(n.child("obj"))
	if !ok || x.db.Rules[o.ID] != nil {
		return
	}
	r := &Rule{ID: o.ID, Op: int(n.num("rule-op", 0)), Conditions: n.child("rule-conds").oids(), Destination: None,
		SubRule: int(n.num("subruleid", None)), ElseIf: n.str("rule-elseif") != "0", Unknown: unknown(n, knownRule)}
	if id, err := strconv.Atoi(n.path("destobj", "destination", "obj").str("id")); err == nil {
		r.Destination = id
	}
	x.db.Rules[o.ID] = r
}

func (x *extractor) condition(n *node) {
	o, ok := x.object(n.child("obj"))
	if !ok || x.db.Conditions[o.ID] != nil {
		return
	}
	x.db.Conditions[o.ID] = &Condition{ID: o.ID, Op: int(n.num("op-type", 0)), Lines: n.child("sconds").oids(), Unknown: unknown(n, knownCondition)}
}

func (x *extractor) line(n *node) {
	o, ok := x.object(n.child("obj"))
	if !ok || x.db.Lines[o.ID] != nil {
		return
	}
	x.db.Lines[o.ID] = &Line{ID: o.ID, Op: int(n.num("op-type", 0)), Type1: int(n.num("type1", 0)), Type2: int(n.num("type2", 0)),
		Left: value(n.child("lval")), Right1: value(n.child("rval1")), Right2: value(n.child("rval2")),
		Negated: n.num("neg", 0) != 0, ChannelID: int(n.num("trgchnid", None)), Unknown: unknown(n, knownLine)}
}

func (x *extractor) destination(n *node) {
	o, ok := x.object(n.child("obj"))
	if !ok || x.db.Destinations[o.ID] != nil {
		return
	}
	x.db.Destinations[o.ID] = &Destination{ID: o.ID, Actions: n.child("dests").oids(), BreakOnRestart: n.num("bor", 0) != 0, Unknown: unknown(n, knownDestination)}
}

func (x *extractor) action(n *node) {
	o, ok := x.object(n.child("obj"))
	if !ok || x.db.Actions[o.ID] != nil {
		return
	}
	x.db.Actions[o.ID] = &Action{ID: o.ID, Param: int(n.num("param", 0)), TargetID: int(n.num("trgdpid", None)),
		ChannelID: int(n.num("trgchnid", None)), Value: value(n.child("trgval")), Delay: value(n.child("trgvalparam")), Unknown: unknown(n, knownAction)}
}

var devdescParent = regexp.MustCompile(`PARENT:"([^"]*)"`)

// finish resolves what needs the whole file: logged variables, a device's interface, the rooms.
func (x *extractor) finish() {
	// a program's -ActiveState helper is listed in vardpmap like a variable: it is the program's
	// active flag, not a variable
	kept := x.db.Variables[:0]
	for _, v := range x.db.Variables {
		if !x.helpers[v.ID] {
			kept = append(kept, v)
		}
	}
	x.db.Variables = kept
	for _, v := range x.db.Variables {
		v.Logged = x.logged[v.ID]
	}
	// rooms: the container named Rooms lists the room enums, each lists channel ids
	for _, e := range x.enums {
		if e.name != "Rooms" && e.name != "Räume" {
			continue
		}
		for _, id := range e.members {
			if r := x.enums[id]; r != nil {
				x.db.Rooms = append(x.db.Rooms, Room{ID: r.id, Name: r.name, Channels: r.members})
			}
		}
	}
	sort.SliceStable(x.db.Rooms, func(i, j int) bool { return x.db.Rooms[i].ID < x.db.Rooms[j].ID })
	sort.SliceStable(x.db.Repairs, func(i, j int) bool { return x.db.Repairs[i].ID < x.db.Repairs[j].ID })
}

// InterfaceOf names the interface of a device: from the interface object it points to, else from
// one of its datapoints (a file may list a device whose interface object is gone).
func (db *DB) InterfaceOf(d *Device) string {
	if i := db.Interfaces[d.InterfaceID]; i != nil && i.Name != "" {
		return i.Name
	}
	for _, dp := range db.Datapoints {
		if c := db.Channels[dp.ChannelID]; c != nil && c.DeviceID == d.ID && dp.Interface != "" {
			return dp.Interface
		}
	}
	return ""
}

// ChannelParent is a channel's device address from its DEVDESC, for a file whose channel has lost
// its device object.
func ChannelParent(c *Channel) string {
	if m := devdescParent.FindStringSubmatch(c.Meta["DEVDESC"]); m != nil {
		return m[1]
	}
	if i := strings.LastIndexByte(c.Address, ':'); i > 0 {
		return c.Address[:i]
	}
	return ""
}
