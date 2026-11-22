package regadom

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func load(t testing.TB, name string) *DB {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	db, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// The fixture is a stock OpenCCU's database with twenty programs built the way the WebUI does.
func TestFixtureCounts(t *testing.T) {
	db := load(t, "fixture.regadom")
	if db.Version != "33620337" || db.Serial != "12345678" || db.Latitude != "" {
		t.Errorf("header: %q %q %q", db.Version, db.Serial, db.Latitude)
	}
	// 35 user and stock variables of vardpmap (its 20 -ActiveState helpers are the programs'),
	// 6 alarm variables, and the three var-dp of dom-root (Gateway-DP, the two message counters)
	if len(db.Programs) != 20 || len(db.Calendars) != 20 || len(db.Variables) != 44 || db.Services != 0 {
		t.Fatalf("programs %d, time modules %d, variables %d, services %d", len(db.Programs), len(db.Calendars), len(db.Variables), db.Services)
	}
	if len(db.Rules) != 34 || len(db.Lines) != 46 || len(db.Destinations) != 34 || len(db.Actions) != 37 || len(db.Events) != 31 {
		t.Errorf("rules %d, lines %d, destinations %d, actions %d, events %d", len(db.Rules), len(db.Lines), len(db.Destinations), len(db.Actions), len(db.Events))
	}
	for _, v := range db.Variables {
		if v.Name == "-ActiveState" {
			t.Errorf("a program's helper %d is listed as a variable", v.ID)
		}
	}
	if len(db.Repairs) != 0 || len(db.Devices) != 1 || len(db.Datapoints) != 0 {
		t.Errorf("repairs %v, devices %d", db.Repairs, len(db.Devices))
	}
	if db.Names[1202] != "Regen" || db.Names[1] != "DOM" {
		t.Errorf("names: %q %q", db.Names[1202], db.Names[1])
	}
}

func TestFixturePrograms(t *testing.T) {
	db := load(t, "fixture.regadom")
	inactive := map[string]bool{"Nachtabsenkung": true, "Wetter holen": true, "Inaktiv und unsichtbar": true}
	for _, p := range db.Programs {
		if p.Active == inactive[p.Name] {
			t.Errorf("%s: active %v", p.Name, p.Active)
		}
		if !p.Enabled {
			t.Errorf("%s: obj.enabled is 1 for every program in the file", p.Name)
		}
		if p.StartupExec != nil || len(p.Unknown) != 0 {
			t.Errorf("%s: startupexec %v, unknown %v", p.Name, p.StartupExec, p.Unknown)
		}
		if _, err := db.Chain(p); err != nil {
			t.Error(err)
		}
	}
	p := db.ProgramByName("Inaktiv und unsichtbar")
	if p == nil || p.Visible() || p.Internal() || p.Info != "Beschreibung mit Umlauten: äöüß" {
		t.Errorf("%+v", p)
	}
	if p := db.ProgramByName("prgDailySunshineRainCounter_4730"); p == nil || !p.Internal() || !p.Visible() {
		t.Errorf("counter program: %+v", p)
	}
	if p := db.ProgramByName("Szene Abend"); p == nil || p.AccessRights == AllRights || p.Info != "ruft andere Programme per Skript auf" {
		t.Errorf("Szene Abend: %+v", p)
	}
	if db.ProgramByName("gibt es nicht") != nil || db.Program(1) != nil || db.Variable(1) != nil {
		t.Error("a lookup found what is not there")
	}

	// Markise bei Regen einfahren: two blocks of two lines, DANN with the cancel box and a delayed
	// activity, a SONST
	p = db.ProgramByName("Markise bei Regen einfahren")
	if db.Program(p.ID) != p {
		t.Error("Program(id)")
	}
	chain, err := db.Chain(p)
	if err != nil || len(chain) != 2 {
		t.Fatalf("chain: %v %v", chain, err)
	}
	when, otherwise := chain[0], chain[1]
	if !when.ElseIf || otherwise.ElseIf || len(when.Conditions) != 2 || len(otherwise.Conditions) != 0 || otherwise.SubRule != None {
		t.Errorf("rules: %+v %+v", when, otherwise)
	}
	b0 := db.Conditions[when.Conditions[0]]
	if b0.Op != 2 || len(b0.Lines) != 2 {
		t.Fatalf("block: %+v", b0)
	}
	l0, l1 := db.Lines[b0.Lines[0]], db.Lines[b0.Lines[1]]
	want0 := Line{ID: l0.ID, Op: 1, Type1: CmpEquals, Type2: TrigChange, Left: Value{"1202", TypeSystemID}, Right1: Value{"1", TypeBinary}, Right2: Value{"0", TypeBinary}, ChannelID: None}
	if !reflect.DeepEqual(*l0, want0) {
		t.Errorf("line 0: %+v", *l0)
	}
	if l1.Type1 != CmpGreater || l1.Type2 != TrigCheck || l1.Right1 != (Value{"5.000000", TypeFloat}) || db.Variable(1207).Name != "Außentemperatur Mittel" {
		t.Errorf("line 1: %+v", *l1)
	}
	d := db.Destinations[when.Destination]
	if !d.BreakOnRestart || len(d.Actions) != 2 {
		t.Fatalf("destination: %+v", d)
	}
	a0, a1 := db.Actions[d.Actions[0]], db.Actions[d.Actions[1]]
	if a0.Param != DestVariable || a0.TargetID != 1206 || a0.Value != (Value{"2", TypeInteger}) || a0.Delay.Type != TypeEmpty {
		t.Errorf("activity 0: %+v", *a0)
	}
	if a1.Value != (Value{"wegen Regen eingefahren", TypeString}) || a1.Delay != (Value{"00:00:30", TypeDelay}) {
		t.Errorf("activity 1: %+v", *a1)
	}

	// a script activity keeps its text with the quotes unescaped
	p = db.ProgramByName("Szene Abend")
	chain, _ = db.Chain(p)
	acts := db.Destinations[chain[0].Destination].Actions
	if a := db.Actions[acts[0]]; a.Param != DestScript || a.Value.Text != `dom.GetObject("Rollläden abends").ProgramExecute();` || a.TargetID != None {
		t.Errorf("script: %+v", *a)
	}
	// a time module line: the left side is the date, the right side the module
	p = db.ProgramByName("Nachtabsenkung")
	chain, _ = db.Chain(p)
	l := db.Lines[db.Conditions[chain[0].Conditions[0]].Lines[0]]
	if l.Type1 != CmpTime || l.Type2 != TrigUpdate || l.Left.Type != TypeCurrentDate || l.Right1 != (Value{"1354", TypeObjectID}) {
		t.Errorf("time line: %+v", *l)
	}
	c := db.Calendars[1354]
	if c == nil || c.Type != 5 || c.Weekdays != 5 || c.Duration != 28800 || c.Time != 1167685200 || c.Period != 1 || c.Sot != 0 || len(c.Unknown) != 0 {
		t.Errorf("time module: %+v", c)
	}
	if c := db.Calendars[1385]; c.Type != 7 || c.Period != 24 || c.RepVal != 12 || c.TmRep != 946681200 {
		t.Errorf("yearly: %+v", c)
	}
	if c := db.Calendars[1318]; c.Begin != 1796079600 || c.End != 1806444000 || c.Period != 2 {
		t.Errorf("validity: %+v", c)
	}
	if c := db.Calendars[1319]; c.RepCnt != 10 {
		t.Errorf("repetitions: %+v", c)
	}
	if c := db.Calendars[1335]; c.Sot != 5 || c.Duration != 3600 {
		t.Errorf("astro: %+v", c)
	}
}

func TestFixtureVariables(t *testing.T) {
	db := load(t, "fixture.regadom")
	v := db.Variable(1202)
	if v.Name != "Regen" || !v.Logged || v.Info != "Regensensor Terrasse" || v.ValueType != TypeBinary || v.SubType != SubBool ||
		v.Meta["BV0"] != "trocken" || v.Meta["BV1"] != "es regnet" || v.Value != (Value{"0", TypeBinary}) || v.ChannelID != None || v.IsAlarm {
		t.Errorf("Regen: %+v", v)
	}
	logged := 0
	for _, v := range db.Variables {
		if v.Logged {
			logged++
		}
	}
	if logged != 1 {
		t.Errorf("%d logged variables", logged)
	}
	if v := db.Variable(1205); v.SubType != SubEnum || v.Meta["VALUE_LIST"] != "Sommer;Übergang;Winter" || v.Value.Text != "1" {
		t.Errorf("Heizperiode: %+v", v)
	}
	if v := db.Variable(1207); v.Meta["UNIT"] != "°C" || v.Meta["MIN"] != "-40" || v.Meta["MAX"] != "60" || v.Value.Text != "12.500000" {
		t.Errorf("Außentemperatur: %+v", v)
	}
	if v := db.Variable(1210); !v.Internal() || v.Visible() {
		t.Errorf("intern Merker: flags %d", v.Flags)
	}
	// a string keeps its trailing blank
	if v := db.Variable(1047); v.Value.Text != "x3:$src$@22:17:20 " || v.SubType != SubString {
		t.Errorf("string: %q", v.Value.Text)
	}
	if v := db.Variable(1126); v.SubType != SubAction {
		t.Errorf("the key stand-in: %+v", v)
	}
	// an alarm variable has neither a value nor a state in the file
	a := db.Variable(1214)
	if a.Name != "Alarm Einbruch" || !a.IsAlarm || a.AlarmType != AlarmSystem || a.SubType != SubAlarm || a.Value != (Value{}) || a.Info != "Alarmzone Erdgeschoss" {
		t.Errorf("alarm: %+v", a)
	}
	if v := db.Variable(950); !v.System() || v.Name != "${sysVarPresence}" {
		t.Errorf("presence: %+v", v)
	}
	if v := db.Variable(40); v == nil || !v.System() || !v.Internal() {
		t.Errorf("the alarm message counter: %+v", v)
	}
}

// ReGa's own trigger table lists a source for every program the fixture's README names.
func TestFixtureEvents(t *testing.T) {
	db := load(t, "fixture.regadom")
	bySrc := map[int][]int{}
	for _, e := range db.Events {
		bySrc[e.Src] = append(bySrc[e.Src], e.Dst)
	}
	// Regen (1202): "Markise bei Regen einfahren" (1233) on change and "Szene Abend" (1405)
	if got := bySrc[1202]; !reflect.DeepEqual(got, []int{1233, 1405}) {
		t.Errorf("Regen triggers %v", got)
	}
}

func TestDevices(t *testing.T) {
	db := load(t, "devices.regadom")
	if len(db.Interfaces) != 3 || len(db.Devices) != 3 || len(db.Channels) != 4 || len(db.Datapoints) != 5 || len(db.Programs) != 4 || len(db.Variables) != 3 {
		t.Fatalf("%d interfaces, %d devices, %d channels, %d datapoints, %d programs, %d variables",
			len(db.Interfaces), len(db.Devices), len(db.Channels), len(db.Datapoints), len(db.Programs), len(db.Variables))
	}
	if db.Latitude != "52.52" || db.Longitude != "13.40" || db.Serial != "NEQ7654321" {
		t.Errorf("gateway: %q %q %q", db.Latitude, db.Longitude, db.Serial)
	}
	dp := db.Datapoints[2010]
	if dp.Interface != "HmIP-RF" || dp.Address != "00111BE9AB9012:4" || dp.Param != "LEVEL" || dp.ChannelID != 2002 || dp.ValueType != TypeFloat || dp.Meta["UNIT"] != "100%" || dp.Ops != 7 {
		t.Errorf("datapoint: %+v", dp)
	}
	if dp := db.Datapoints[2210]; dp.Interface != "CUxD" || dp.Meta["UNIT"] != "°C" {
		t.Errorf("CUxD datapoint: %+v", dp)
	}
	if dp := db.Datapoints[2110]; dp.SubType != SubAction || dp.Param != "PRESS_SHORT" {
		t.Errorf("key: %+v", dp)
	}
	ch := db.Channels[2002]
	if ch.Name != "Markise Behang" || ch.Address != "00111BE9AB9012:4" || ch.DeviceID != 2000 || ch.Label != "BLIND_VIRTUAL_RECEIVER" || ChannelParent(ch) != "00111BE9AB9012" {
		t.Errorf("channel: %+v", ch)
	}
	if ch := db.Channels[2201]; ch.Name != "Wetter außen" {
		t.Errorf("entity in a name: %q", ch.Name)
	}
	dev := db.Devices[2000]
	if dev.Name != "Markise Terrasse" || dev.Address != "00111BE9AB9012" || dev.Label != "HmIP-BROLL" || !reflect.DeepEqual(dev.Channels, []int{2001, 2002}) || db.InterfaceOf(dev) != "HmIP-RF" {
		t.Errorf("device: %+v", dev)
	}
	if db.Interfaces[1009].URL != "xmlrpc_bin://127.0.0.1:32001" {
		t.Errorf("interface: %+v", db.Interfaces[1009])
	}
	if got := db.RoomsOf(2002); !reflect.DeepEqual(got, []string{"Terrasse", "Flur"}) {
		t.Errorf("rooms of the channel: %v", got)
	}
	if got := db.RoomsOf(2201); got != nil {
		t.Errorf("rooms of a channel in none: %v", got)
	}
	// a variable bound to a channel, and a name that was UTF-8 in the Latin-1 file
	if v := db.Variable(3000); v.ChannelID != 2201 {
		t.Errorf("channel binding: %d", v.ChannelID)
	}
	if v := db.Variable(3001); v.Name != "Küche Licht" {
		t.Errorf("mixed encoding: %q", v.Name)
	}
	if !reflect.DeepEqual(db.Repairs, []Repair{{ID: 3001, Before: "KÃ¼che Licht", After: "Küche Licht"}}) {
		t.Errorf("repairs: %+v", db.Repairs)
	}
}

func TestDeviceProgram(t *testing.T) {
	db := load(t, "devices.regadom")
	p := db.ProgramByName("Markise bei Wind")
	if p.StartupExec == nil || *p.StartupExec || p.Info != "Geräte und Taster" || !p.Active {
		t.Errorf("program: %+v", p)
	}
	if q := db.ProgramByName("Frostwarnung"); q.StartupExec == nil || !*q.StartupExec {
		t.Errorf("startupexec 1: %+v", q.StartupExec)
	}
	chain, err := db.Chain(p)
	if err != nil || len(chain) != 2 {
		t.Fatal(chain, err)
	}
	b0, b1 := db.Conditions[chain[0].Conditions[0]], db.Conditions[chain[0].Conditions[1]]
	key := db.Lines[b0.Lines[1]]
	if key.Type2 != TrigLegacy || key.Left != (Value{"2110", TypeObjectID}) || key.ChannelID != 2101 || key.Negated {
		t.Errorf("key line: %+v", key)
	}
	neg := db.Lines[b1.Lines[1]]
	if !neg.Negated || neg.Type1 != CmpInRange || neg.Right1.Text != "0.200000" || neg.Right2.Text != "0.800000" {
		t.Errorf("negated range: %+v", neg)
	}
	d := db.Destinations[chain[0].Destination]
	a := db.Actions[d.Actions[1]]
	if a.Param != DestDatapoint || a.TargetID != 2011 || a.ChannelID != 2002 || a.Delay != (Value{"00:01:30", TypeDelay}) {
		t.Errorf("delayed device activity: %+v", a)
	}
	sv := db.Actions[db.Destinations[chain[1].Destination].Actions[0]]
	if sv.Value != (Value{"[SV]lastValue", TypeSpecialValue}) {
		t.Errorf("special value: %+v", sv)
	}
}

// What a newer ReGa adds is named, element by element, so the importer can refuse to guess.
func TestUnknownElements(t *testing.T) {
	db := load(t, "devices.regadom")
	p := db.ProgramByName("Neues Feld")
	if !reflect.DeepEqual(p.Unknown, []string{"program/runlimit"}) {
		t.Errorf("program: %v", p.Unknown)
	}
	chain, _ := db.Chain(p)
	if !reflect.DeepEqual(chain[0].Unknown, []string{"rule/rule-prio"}) {
		t.Errorf("rule: %v", chain[0].Unknown)
	}
	l := db.Lines[db.Conditions[chain[0].Conditions[0]].Lines[0]]
	if !reflect.DeepEqual(l.Unknown, []string{"s-cond/hysteresis"}) {
		t.Errorf("line: %v", l.Unknown)
	}
	for _, q := range db.Programs {
		if q != p && len(q.Unknown) != 0 {
			t.Errorf("%s: %v", q.Name, q.Unknown)
		}
	}
}

func TestChainErrors(t *testing.T) {
	db := load(t, "devices.regadom")
	p := db.ProgramByName("Frostwarnung")
	first := db.Rules[p.FirstRule]
	first.SubRule = 9999
	if _, err := db.Chain(p); err == nil || !strings.Contains(err.Error(), "rule 9999 is not in the file") {
		t.Errorf("a dangling rule: %v", err)
	}
	first.SubRule = first.ID
	if _, err := db.Chain(p); err == nil || !strings.Contains(err.Error(), "loop") {
		t.Errorf("a loop: %v", err)
	}
	// a program without a rule has an empty chain
	if chain, err := db.Chain(&Program{FirstRule: None}); err != nil || len(chain) != 0 {
		t.Errorf("no rule: %v %v", chain, err)
	}
}

// A device whose interface object is gone is named by one of its datapoints; a channel without
// DEVDESC by its address.
func TestInterfaceFallbacks(t *testing.T) {
	db := load(t, "devices.regadom")
	dev := db.Devices[2100]
	dev.InterfaceID = None
	if got := db.InterfaceOf(dev); got != "BidCos-RF" {
		t.Errorf("from the datapoint: %q", got)
	}
	if got := db.InterfaceOf(&Device{Object: Object{ID: 1}, InterfaceID: None}); got != "" {
		t.Errorf("unknown device: %q", got)
	}
	if got := ChannelParent(&Channel{Address: "ABC0000001:3"}); got != "ABC0000001" {
		t.Errorf("parent from the address: %q", got)
	}
	if got := ChannelParent(&Channel{Address: "ABC"}); got != "" {
		t.Errorf("no parent: %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := Parse(strings.NewReader("<html><body/></html>")); !errors.Is(err, ErrNotRegadom) {
		t.Errorf("not a database: %v", err)
	}
	if _, err := Parse(strings.NewReader("<dom><objmap><programmap><program>")); err == nil {
		t.Error("a truncated file was accepted")
	}
	if _, err := Parse(strings.NewReader("<dom>" + strings.Repeat("<a>", 200) + strings.Repeat("</a>", 200) + "</dom>")); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Errorf("deep nesting: %v", err)
	}
	defer func(n int64) { maxSize = n }(maxSize)
	maxSize = 64
	if _, err := Parse(strings.NewReader("<dom>" + strings.Repeat("<a>x</a>", 20) + "</dom>")); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
	maxSize = 30 // one byte more than allowed, and the file ends there
	if _, err := Parse(strings.NewReader("<dom><version>1</version></dom>")); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large at the end: %v", err)
	}
}

// Objects without an id, duplicates, a foreign charset: read around, never a panic.
func TestParseOddities(t *testing.T) {
	x := `<?xml version="1.0" encoding="windows-1252" ?>
<dom><version> 7 </version>
<objmap>
<vardpmap>
<var-dp><dp><obj><id>x</id><name>ohne Id</name></obj></dp></var-dp>
<var-dp><dp><obj><id>5</id><name>A` + "\t" + `B</name><metadata><value>ohne Eigenschaft</value><property>K</property><value>v</value></metadata></obj></dp><var>t </var><varType>20</varType></var-dp>
<var-dp><dp><obj><id>5</id><name>zweimal</name></obj></dp></var-dp>
<alarm-dp><dp><obj><id>6</id><name>AL-X:0.UNREACH</name></obj></dp><type>5</type></alarm-dp>
<hss-dp><dp><obj><id>7</id><name>ohne Punkt</name></obj></dp></hss-dp>
<hss-dp><dp><obj><id>8</id><name>HmIP-RF.ohneParam</name></obj></dp></hss-dp>
<hss-dp><dp><obj><id>9</id><name>HmIP-RF.A:1.STATE</name></obj></dp></hss-dp>
<hist-dp><dp-archive>keine Zahl</dp-archive></hist-dp>
<hss-dp><dp><obj><id>9</id><name>HmIP-RF.B:1.STATE</name></obj></dp></hss-dp>
<calendar-dp><timer-dp><dp><obj><id>10</id><name>Z</name></obj></dp></timer-dp></calendar-dp>
<calendar-dp><timer-dp><dp><obj><id>10</id><name>Z2</name></obj></dp><type>4</type></timer-dp></calendar-dp>
<calendar-dp><calduration>1</calduration></calendar-dp>
<s-cond><obj><id>11</id><name>C` + "\t" + `D</name></obj></s-cond>
<s-cond><obj><id>11</id></obj><type1>5</type1></s-cond>
<sdestination><obj><id>12</id></obj></sdestination>
<sdestination><obj><id>12</id></obj><param>20</param></sdestination>
</vardpmap>
<programmap><program><obj><id>20</id><name>P</name><enabled>0</enabled></obj></program><program><obj><id>20</id><name>P2</name></obj></program></programmap>
<rulemap><rule><obj><id>21</id></obj><rule-conds><oid>x</oid><oid>22</oid><count>1</count></rule-conds></rule></rulemap>
<event><oidDst>a</oidDst><oidSrc>1</oidSrc></event>
<enummap><enum><obj><id>30</id><name>Räume</name></obj><enel><oid>31</oid><oid>99</oid></enel></enum><enum><obj><id>31</id><name>Bad</name></obj><enel><oid>9</oid></enel></enum></enummap>
</objmap></dom>`
	db, err := Parse(strings.NewReader(x))
	if err != nil {
		t.Fatal(err)
	}
	if db.Version != "7" || len(db.Variables) != 1 || db.Services != 1 || len(db.Events) != 0 {
		t.Fatalf("%+v", db)
	}
	v := db.Variables[0]
	if v.Name != "A B" || v.Meta["K"] != "v" || len(v.Meta) != 1 || v.Value.Text != "t " || !v.Enabled || v.Flags != FlagVisible || v.AccessRights != AllRights {
		t.Errorf("variable: %+v", v)
	}
	if !reflect.DeepEqual(db.Repairs, []Repair{{ID: 5, Before: "A\tB", After: "A B"}, {ID: 11, Before: "C\tD", After: "C D"}}) {
		t.Errorf("repairs: %+v", db.Repairs)
	}
	if d := db.Datapoints[7]; d.Interface != "" || d.Param != "" {
		t.Errorf("a datapoint name without a dot: %+v", d)
	}
	if d := db.Datapoints[8]; d.Interface != "HmIP-RF" || d.Param != "" {
		t.Errorf("a datapoint name without a parameter: %+v", d)
	}
	if d := db.Datapoints[9]; d.Address != "A:1" || d.Param != "STATE" {
		t.Errorf("address and parameter from the name: %+v", d)
	}
	if l := db.Lines[11]; l.Type1 != 0 || l.Left != (Value{}) || l.ChannelID != None {
		t.Errorf("the first of two lines with one id wins: %+v", l)
	}
	if a := db.Actions[12]; a.Param != 0 || a.TargetID != None {
		t.Errorf("the first of two activities wins: %+v", a)
	}
	if c := db.Calendars[10]; len(db.Calendars) != 1 || c.Type != 0 {
		t.Errorf("time modules: %+v", db.Calendars)
	}
	// without an -ActiveState helper the program's own enabled flag is all there is
	if len(db.Programs) != 1 || db.Programs[0].Active || db.Programs[0].FirstRule != None {
		t.Errorf("programs: %+v", db.Programs)
	}
	if r := db.Rules[21]; !reflect.DeepEqual(r.Conditions, []int{22}) || r.Destination != None || r.SubRule != None {
		t.Errorf("rule: %+v", r)
	}
	if len(db.Rooms) != 1 || db.Rooms[0].Name != "Bad" {
		t.Errorf("rooms: %+v", db.Rooms)
	}
}

func sbk(t *testing.T, inner map[string][]byte, outer map[string][]byte) []byte {
	t.Helper()
	var in bytes.Buffer
	gz := gzip.NewWriter(&in)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"usr/local/etc/config/ids", "usr/local/etc/config/time.conf", "usr/local/etc/config/homematic.regadom.bak", "usr/local/etc/config/homematic.regadom"} {
		body, ok := inner[name]
		if !ok {
			continue
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))})
		_, _ = tw.Write(body)
	}
	_ = tw.Close()
	_ = gz.Close()
	var out bytes.Buffer
	ow := tar.NewWriter(&out)
	for _, name := range []string{"signature", "firmware_version"} {
		if body, ok := outer[name]; ok {
			_ = ow.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))})
			_, _ = ow.Write(body)
		}
	}
	if inner != nil {
		_ = ow.WriteHeader(&tar.Header{Name: "./usr_local.tar.gz", Mode: 0o644, Size: int64(in.Len())})
		_, _ = ow.Write(in.Bytes())
	}
	_ = ow.Close()
	return out.Bytes()
}

func TestReadBackup(t *testing.T) {
	body, err := os.ReadFile("testdata/fixture.regadom")
	if err != nil {
		t.Fatal(err)
	}
	b := sbk(t, map[string][]byte{
		"usr/local/etc/config/ids":                   []byte("abc"),
		"usr/local/etc/config/time.conf":             []byte("# location\nLATITUDE=\"52.5\"\nLONGITUDE=13.4\nTIMEZONE='Europe/Berlin'\nnoise\n"),
		"usr/local/etc/config/homematic.regadom.bak": []byte("<dom><version>old</version></dom>"),
		"usr/local/etc/config/homematic.regadom":     body,
	}, map[string][]byte{"signature": []byte("xx"), "firmware_version": []byte("VERSION=3.89.8.20260719\n")})
	src, err := Read(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != "sbk" || src.Firmware != "3.89.8.20260719" || src.Latitude != "52.5" || src.Longitude != "13.4" || src.Timezone != "Europe/Berlin" || len(src.DB.Programs) != 20 {
		t.Errorf("%+v", src)
	}
	// without time.conf the database's own location is what there is
	dev, _ := os.ReadFile("testdata/devices.regadom")
	src, err = Read(bytes.NewReader(sbk(t, map[string][]byte{"usr/local/etc/config/homematic.regadom": dev}, nil)))
	if err != nil || src.Latitude != "52.52" || src.Longitude != "13.40" || src.Firmware != "" {
		t.Errorf("%+v %v", src, err)
	}
	// errors: truncated, no archive, no database, a broken gzip, a broken database
	if _, err := Read(bytes.NewReader(b[:700])); err == nil {
		t.Error("a truncated backup was accepted")
	}
	if _, err := Read(bytes.NewReader(sbk(t, nil, map[string][]byte{"signature": []byte("xx")}))); !errors.Is(err, ErrNoArchive) {
		t.Errorf("no archive: %v", err)
	}
	if _, err := Read(bytes.NewReader(sbk(t, map[string][]byte{"usr/local/etc/config/ids": []byte("abc")}, nil))); !errors.Is(err, ErrNoDatabase) {
		t.Errorf("no database: %v", err)
	}
	if _, err := Read(bytes.NewReader(sbk(t, map[string][]byte{"usr/local/etc/config/homematic.regadom": []byte("<html/>")}, nil))); !errors.Is(err, ErrNotRegadom) {
		t.Errorf("a wrong database: %v", err)
	}
	var plain bytes.Buffer
	ow := tar.NewWriter(&plain)
	_ = ow.WriteHeader(&tar.Header{Name: "usr_local.tar.gz", Mode: 0o644, Size: 9})
	_, _ = ow.Write([]byte("not gzip!"))
	_ = ow.Close()
	if _, err := Read(&plain); err == nil || !strings.Contains(err.Error(), "usr_local.tar.gz") {
		t.Errorf("a broken gzip: %v", err)
	}
	var cut bytes.Buffer
	gz := gzip.NewWriter(&cut)
	_, _ = gz.Write([]byte("no tar in here, only text that is long enough to be read as a header ........"))
	_ = gz.Close()
	var outer bytes.Buffer
	ow = tar.NewWriter(&outer)
	_ = ow.WriteHeader(&tar.Header{Name: "usr_local.tar.gz", Mode: 0o644, Size: int64(cut.Len())})
	_, _ = ow.Write(cut.Bytes())
	_ = ow.Close()
	if _, err := Read(&outer); err == nil {
		t.Error("a gzip without a tar was accepted")
	}
	if _, err := Read(strings.NewReader(strings.Repeat("neither XML nor a tar archive ", 40))); err == nil || !strings.Contains(err.Error(), "backup:") {
		t.Errorf("garbage: %v", err)
	}
	if _, err := Read(strings.NewReader("")); err == nil {
		t.Error("an empty upload was accepted")
	}
}

func TestReadBare(t *testing.T) {
	f, err := os.Open("testdata/devices.regadom")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	src, err := Read(f)
	if err != nil {
		t.Fatal(err)
	}
	if src.Kind != "regadom" || src.Latitude != "52.52" || src.Timezone != "" || len(src.DB.Programs) != 4 {
		t.Errorf("%+v", src)
	}
	// a byte order mark and white space before the declaration
	src, err = Read(strings.NewReader("\xef\xbb\xbf \n<dom><version>1</version></dom>"))
	if err != nil || src.DB.Version != "1" {
		t.Errorf("%+v %v", src, err)
	}
	if _, err := Read(strings.NewReader("<nothing/>")); !errors.Is(err, ErrNotRegadom) {
		t.Errorf("bare, not a database: %v", err)
	}
}

// The declared ISO-8859-1 is honoured: a single high byte is one character.
func TestLatin1(t *testing.T) {
	x := "<?xml version=\"1.0\" encoding=\"iso-8859-1\" ?>\n<dom><objmap><vardpmap><var-dp><dp><obj><id>5</id><name>K\xfcche " + strings.Repeat("\xe4", 5000) + "</name>" +
		"<metadata><property>UNIT</property><value>\xb0C</value></metadata></obj></dp></var-dp></vardpmap></objmap></dom>"
	db, err := Parse(strings.NewReader(x))
	if err != nil {
		t.Fatal(err)
	}
	v := db.Variables[0]
	if v.Name != "Küche "+strings.Repeat("ä", 5000) || v.Meta["UNIT"] != "°C" {
		t.Errorf("%q %q", v.Name[:20], v.Meta["UNIT"])
	}
}

// A control character XML forbids, as a script can write it into a string variable, is read as a
// blank instead of failing the file.
func TestControlCharacters(t *testing.T) {
	x := "<dom><objmap><vardpmap><var-dp><dp><obj><id>5</id><name>Bad\x00 1</name></obj></dp><var>a\x1bb\nc</var><varType>20</varType></var-dp></vardpmap></objmap></dom>"
	db, err := Parse(strings.NewReader(x))
	if err != nil {
		t.Fatal(err)
	}
	if v := db.Variables[0]; v.Name != "Bad  1" || v.Value.Text != "a b\nc" {
		t.Errorf("%q %q", v.Name, v.Value.Text)
	}
}

func FuzzParse(f *testing.F) {
	for _, name := range []string{"fixture.regadom", "devices.regadom"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte("<dom><objmap><programmap><program><obj><id>1</id></obj><prgrule><rule><obj><id>2</id></obj><subruleid>2</subruleid></rule></prgrule></program></programmap></objmap></dom>"))
	f.Fuzz(func(t *testing.T, b []byte) {
		src, err := Read(bytes.NewReader(b))
		if err != nil {
			return
		}
		for _, p := range src.DB.Programs {
			_, _ = src.DB.Chain(p)
		}
	})
}
