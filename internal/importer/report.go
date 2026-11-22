package importer

import (
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// Status is what became of an object.
type Status string

const (
	// Imported: taken over; a program keeps the state it had on the CCU.
	Imported Status = "imported"
	// Inactive: a program taken over but switched off, for the reasons given.
	Inactive Status = "inactive"
	// Skipped: not taken over.
	Skipped Status = "skipped"
)

// Note is one statement of the report: a code the page and the Markdown both have words for, and
// what it is about.
type Note struct {
	Code string   `json:"code"`
	Args []string `json:"args,omitempty"`
}

// The reasons a program is imported inactive.
const (
	ReasonScript       = "script"             // a script activity that is not one of the two recognised one-liners
	ReasonCUxD         = "cuxd"               // a CUxD datapoint
	ReasonMissing      = "missing-device"     // devices this system does not have
	ReasonSpecial      = "special-value"      // a special value ("Letzter Wert")
	ReasonUnknownField = "unknown-field"      // an element of the database this version does not know
	ReasonComparison   = "unknown-comparison" // a comparison type it does not know
	ReasonTrigger      = "unknown-trigger"    // a trigger mode it does not know
	ReasonValue        = "unknown-value"      // a value of a type it does not know
	ReasonOperators    = "mixed-operators"    // AND and OR mixed inside one level
	ReasonDelay        = "unknown-delay"      // a delay it cannot read
	ReasonAllInactive  = "all-inactive"       // the wizard's choice
	ReasonListIndex    = "list-index"         // a value list index the list does not have
	ReasonCommand      = "command"            // a system.Exec that became a command: it runs with the addon's rights now
)

// The reasons a program is not imported.
const (
	SkipCounter    = "counter-program" // one of eQ-3's counter programs: the device helper replaces it
	SkipLeftover   = "editor-leftover" // the WebUI editor's half-saved temporary program
	SkipUnreadable = "unreadable"      // its rules cannot be followed
	SkipDangling   = "dangling"        // it refers to an object the database does not hold
	SkipInvalid    = "invalid"         // it does not fit the file format
)

// Behaviour notes on a program (the quirk catalogue's numbers are in the guide).
const (
	NoteAtStart      = "at-start"         // evaluated once after a system start, as on the CCU
	NoteNotAtStart   = "not-at-start"     // the CCU's own switch said: not at start
	NoteElseUpdate   = "else-update"      // "bei Aktualisierung" with a SONST: SONST runs at every report
	NoteCancels      = "cancels"          // a block ends running delays for its targets, other programs' too
	NoteOrder        = "order"            // several activities: sent in their listed order here
	NoteDST          = "dst"              // a fixed time between 02:00 and 03:00
	NoteAstro        = "astro"            // astro times are the real ones; the CCU's are off by up to about ten minutes
	NoteKeyLine      = "key-line"         // a key press line
	NoteOutside      = "outside-range"    // the comparison "outside the range", never measured on a CCU
	NoteZeroDelay    = "zero-delay"       // a delay stored as zero (24 h and more on the CCU): immediate
	NoteElseIfChange = "change-in-elseif" // "bei Änderung" below the first rule fires on every write while an earlier block is true
	NoteNoTrigger    = "no-trigger"       // only "nur prüfen" lines: never runs by itself
	NoteTimeCheck    = "time-check"       // a time module on "nur prüfen": it triggers all the same, as on the CCU
	NoteNegated      = "negated"          // a negated line
	NotePattern      = "time-pattern"     // a time module pattern stored as found, not understood yet
	NoteExecWebhook  = "exec-webhook"     // a system.Exec with wget or curl became a webhook
	NoteExecWake     = "exec-wake"        // a system.Exec with ether-wake became a Wake-on-LAN
	NoteCommandRoot  = "command-root"     // a command that names a system path or a tool only root can use
)

// Notes on a variable.
const (
	VarSystemValue  = "system-value"     // a CCU built-in the system maintains (message counters, duty cycle)
	VarPresence     = "presence"         // the CCU's presence variable: true again after every system start
	VarCounter      = "counter"          // one of eQ-3's counter variables: family, channel
	VarCounterAddr  = "counter-address"  // its name and the database disagree about the channel
	VarCounterPlain = "counter-plain"    // its channel is not in the database: a plain variable, nobody counts on
	VarHelper       = "helper"           // a device helper keeps it: kind, channel
	VarHelperState  = "helper-state"     // the device's last reading: kept inside the helper
	VarHelperUnused = "helper-unused"    // a scratch variable of the CCU's counter script
	VarHelperDay    = "helper-day"       // a day's amount the CCU kept with the channel: the helper's total
	VarAlarm        = "alarm-no-state"   // an alarm variable: the database holds no alarm state
	VarEntries      = "entries-repaired" // empty or double entries of a value list were renamed
	VarListValue    = "value-not-in-list"
	VarAction       = "action-subtype" // a boolean of the subtype "action"
	VarLimits       = "limits-dropped" // min above max in the database
	SkipWatchdog    = "watchdog"       // OpenCCU's own watchdog alarms
	SkipSystem      = "system-unused"  // a built-in no program uses
	SkipUnknownType = "unknown-type"
)

// ProgramReport is one program's line.
type ProgramReport struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Key    string `json:"key,omitempty"`
	Status Status `json:"status"`
	// WasActive is the CCU's state, Active what the import gives it.
	WasActive bool `json:"was_active"`
	Active    bool `json:"active"`
	// Updated: an earlier import of this source made the file already.
	Updated bool   `json:"updated,omitempty"`
	Reasons []Note `json:"reasons,omitempty"`
	Notes   []Note `json:"notes,omitempty"`
}

// VariableReport is one variable's line.
type VariableReport struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Key     string   `json:"key,omitempty"`
	Type    string   `json:"type,omitempty"`
	Value   string   `json:"value,omitempty"`
	Entries []string `json:"entries,omitempty"`
	Status  Status   `json:"status"`
	Updated bool     `json:"updated,omitempty"`
	Notes   []Note   `json:"notes,omitempty"`
}

// DeviceReport is one channel programs use.
type DeviceReport struct {
	SeenDevice
	// Known: the channel exists on this system.
	Known    bool     `json:"known"`
	Programs []string `json:"programs"`
}

// Location is where the CCU was, for the astro times.
type Location struct {
	Latitude  string `json:"latitude,omitempty"`
	Longitude string `json:"longitude,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
}

// Count is one row of the construct histogram.
type Count struct {
	What string `json:"what"`
	N    int    `json:"n"`
}

// Report is the dry run's account.
type Report struct {
	Source      string           `json:"source"`
	Kind        string           `json:"kind"` // sbk or regadom
	Firmware    string           `json:"firmware,omitempty"`
	AllInactive bool             `json:"all_inactive,omitempty"`
	Location    Location         `json:"location"`
	Programs    []ProgramReport  `json:"programs"`
	Variables   []VariableReport `json:"variables"`
	Devices     []DeviceReport   `json:"devices"`
	Repairs     []regadom.Repair `json:"repairs,omitempty"`
	Services    int              `json:"services,omitempty"` // service messages left out
	Events      []Note           `json:"events,omitempty"`   // where the CCU's own trigger table disagrees
	Histogram   []Count          `json:"histogram"`
}

// Totals counts the programs by status, and the variables imported.
func (r *Report) Totals() (imported, inactive, skipped, variables int) {
	for _, p := range r.Programs {
		switch p.Status {
		case Imported:
			imported++
		case Inactive:
			inactive++
		default:
			skipped++
		}
	}
	for _, v := range r.Variables {
		if v.Status == Imported {
			variables++
		}
	}
	return
}

func has(notes []Note, code string) bool {
	for _, n := range notes {
		if n.Code == code {
			return true
		}
	}
	return false
}

// add appends a note once per code and argument list.
func add(notes []Note, code string, args ...string) []Note {
	for _, n := range notes {
		if n.Code == code && equal(n.Args, args) {
			return notes
		}
	}
	return append(notes, Note{Code: code, Args: args})
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
