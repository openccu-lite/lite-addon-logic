// Package importer turns a parsed ReGa database (package regadom) into lite-addon-logic's programs
// and variables, and says what it did: the dry-run report. Nothing is written here - the caller
// shows the report and, when the user agrees, stores Result's programs, variables, values, ledger
// and device notes.
//
// The promise it serves: a program that uses no script comes out as the same program - the same
// blocks, lines, trigger modes, values and delays. What cannot be taken over one to one is never
// guessed: the program is imported inactive, or not at all, with the reason in the report.
package importer

import (
	"sort"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// DefaultSource is the ledger's name when the caller gives none.
const DefaultSource = "ccu"

// Options are the import's inputs beside the database.
type Options struct {
	// Source names the ledger, imports/<source>.yaml: a second import of the same source updates
	// the same files. Empty is DefaultSource.
	Source string
	// AllInactive imports every program inactive ("alle inaktiv übernehmen").
	AllInactive bool
	// Known says whether a channel (Interface/ADDRESS:CH) exists on this system. Nil means the
	// device list is not available: every device then counts as missing.
	Known func(channel string) bool
	// Location is the time zone the CCU's clock values are read in; nil is the backup's
	// time.conf zone when the system knows it, else the system's own.
	Location *time.Location
	// Previous is the ledger of an earlier import of this source; its keys are kept.
	Previous *Ledger
	// Taken says whether a key is in use by a program ("program") or a variable ("variable")
	// that is not this source's.
	Taken func(kind, key string) bool
	// Lang names the built-in variables ("de" or "en"; empty is "de", the CCU's own).
	Lang string
}

// Result is a finished dry run: what an "Übernehmen" would store.
type Result struct {
	Programs  []*model.Program
	Variables []*model.Variable
	// Values are the variables' values in the database, by key; an alarm variable has none.
	Values map[string]any
	// Helpers is what the device helpers among the variables remember, by their leading
	// variable's key: the device's reading the CCU counted last.
	Helpers map[string]model.HelperState
	// Seen is what the database knew about the devices the programs use and this system does not
	// have (devices-seen.yaml).
	Seen   []SeenDevice
	Ledger *Ledger
	Report *Report
}

// imp is one import's working state.
type imp struct {
	db   *regadom.DB
	src  *regadom.Source
	opt  Options
	loc  *time.Location
	lang string

	varKey  map[int]string             // imported variables by id
	vars    map[int]*model.Variable    // the same, the definitions
	prgKey  map[int]string             // importable programs by id
	usedVar map[int]bool               // variables a program refers to
	seen    map[string]*SeenDevice     // referenced channels by Interface/ADDRESS:CH
	users   map[string]map[string]bool // channel -> program names
	hist    map[string]int
	res     *Result
	// the device helpers among the variables (counters.go): what each counter variable is,
	// and the helpers by kind and channel
	parts  map[int]*counterPart
	groups map[string]*helperGroup
}

// Import maps a source onto the model.
func Import(src *regadom.Source, opt Options) *Result {
	if opt.Source == "" {
		opt.Source = DefaultSource
	}
	m := &imp{db: src.DB, src: src, opt: opt, lang: opt.Lang,
		varKey: map[int]string{}, vars: map[int]*model.Variable{}, prgKey: map[int]string{}, usedVar: map[int]bool{},
		seen: map[string]*SeenDevice{}, users: map[string]map[string]bool{}, hist: map[string]int{}}
	if m.lang != "en" {
		m.lang = "de"
	}
	m.loc = opt.Location
	if m.loc == nil && src.Timezone != "" {
		m.loc, _ = time.LoadLocation(src.Timezone)
	}
	if m.loc == nil {
		m.loc = time.Local
	}
	m.res = &Result{Values: map[string]any{},
		Ledger: &Ledger{Source: opt.Source, Serial: src.DB.Serial, Firmware: src.Firmware, Programs: map[int]string{}, Variables: map[int]string{}},
		Report: &Report{Source: opt.Source, Kind: src.Kind, Firmware: src.Firmware, AllInactive: opt.AllInactive,
			Programs: []ProgramReport{}, Variables: []VariableReport{}, Devices: []DeviceReport{}, Histogram: []Count{},
			Location: Location{Latitude: src.Latitude, Longitude: src.Longitude, Timezone: src.Timezone}}}

	m.collectUsed()
	m.counters()
	m.variables()
	m.programKeys()
	m.programs()
	m.devices()

	rep := m.res.Report
	rep.Repairs = src.DB.Repairs
	rep.Services = src.DB.Services
	rep.Events = CheckEvents(src.DB)
	for k, n := range m.hist {
		rep.Histogram = append(rep.Histogram, Count{What: k, N: n})
	}
	sort.Slice(rep.Histogram, func(i, j int) bool { return rep.Histogram[i].What < rep.Histogram[j].What })
	return m.res
}

// taken says whether a key is in use by something else than this source's own object.
func (m *imp) taken(kind, key string, used map[string]bool) bool {
	if used[key] || (kind == "variable" && model.IsSystemValue(key)) {
		return true
	}
	return m.opt.Taken != nil && m.opt.Taken(kind, key)
}

// key picks an object's key: the one an earlier import of the source gave it, else a slug of
// its name that nobody has.
func (m *imp) key(kind string, id int, name string, used map[string]bool) string {
	if p := m.opt.Previous; p != nil {
		prev := p.Variables[id]
		if kind == "program" {
			prev = p.Programs[id]
		}
		if prev != "" && model.ValidKey(prev) && !used[prev] {
			used[prev] = true
			return prev
		}
	}
	k := model.UniqueKey(name, func(k string) bool { return m.taken(kind, k, used) })
	used[k] = true
	return k
}

// channelRef is "Interface/ADDRESS:CH" for a channel id, "" when the file cannot say.
func (m *imp) channelRef(id int) string {
	ch := m.db.Channels[id]
	if ch == nil || ch.Address == "" {
		return ""
	}
	iface := ""
	for _, dp := range m.db.Datapoints {
		if dp.ChannelID == id && dp.Interface != "" {
			iface = dp.Interface
			break
		}
	}
	if iface == "" {
		if dev := m.db.Devices[ch.DeviceID]; dev != nil {
			iface = m.db.InterfaceOf(dev)
		}
	}
	if iface == "" || model.ParseChannel(iface+"/"+ch.Address) != nil {
		return ""
	}
	return iface + "/" + ch.Address
}
