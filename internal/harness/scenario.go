// Package harness is the differential harness: the same scenario - programs, a script of writes
// with their times - runs against a real ReGaHss and against this addon's engine, and the two
// traces are compared.
//
// The reference side is recorded, not run in CI: cmd/regaharness starts the reference system's
// ReGaHss in a container under a faked clock (rega/Dockerfile), builds the scenario's programs
// with the script calls the WebUI's pages make, saves the database, plays the steps and writes
// what ran into testdata/<scenario>/: rega.trace, the database (homematic.regadom.gz) and the
// programs as this addon reads them (programs.yaml). The engine side runs on every commit: it
// imports the recorded database - so the importer is part of what is compared -, plays the same
// steps with a fake clock, and its trace must equal the recorded one (a scenario marked
// Reproduce) or the stated one (a scenario marked Fix, which documents a difference).
//
// What runs is told by markers: every block of a program under test sets a marker variable; on
// the reference a tracer program writes each marker's name and the time into a trace variable,
// on the engine the variable's notification is the trace.
package harness

import (
	"fmt"
	"sort"
	"time"
)

// Mark says what a scenario claims.
type Mark string

const (
	// Reproduce: the engine does what the reference does.
	Reproduce Mark = "reproduce"
	// Fix: the engine differs on purpose; Scenario.Engine is its trace, Note says why.
	Fix Mark = "fix"
)

// Names of the harness's own objects on the reference.
const (
	MarkerPrefix = "zzM_"
	TraceVar     = "zzH_T"
	TracerName   = "zzH_tracer"
	BeatVar      = "zzH_beat"
	TimeLayout   = "2006-01-02 15:04:05"
)

// Zone is where the scenarios' clock times are: the reference container's time zone.
var Zone = mustZone("Europe/Berlin")

func mustZone(name string) *time.Location {
	l, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return l
}

// Scenario is one case.
type Scenario struct {
	// Name is the folder in testdata (lower case, digits, hyphens).
	Name string
	// Quirk names the catalogue entries it covers ("Q-2", "Q-16, Q-17"); Title says it in a line.
	Quirk string
	Title string
	// Start is the steps' zero, a wall-clock time in Zone ("2026-11-10 12:00:00"). The programs
	// are created shortly before it.
	Start string
	// Rate is how fast the reference's clock runs (1 when zero); Slack how far a time on the
	// reference may be from the engine's (two seconds of the reference's clock per rate when
	// zero).
	Rate  int
	Slack time.Duration
	Vars  []Var
	// Position is where the reference stands, for sunrise and sunset (nil: the scenario has no
	// astro time module). The reference keeps it outside its database, so the engine side gets
	// it from here.
	Position *Position
	// Programs are created in this order.
	Programs []Program
	Steps    []Step
	// Until is when the scenario ends, from Start.
	Until time.Duration
	Mark  Mark
	// Engine is the engine's trace for a Fix ("2026-11-10 12:00:05 a_then"), Note why it
	// differs from the reference.
	Engine []string
	Note   string
	// Needs names what the engine side needs and may not have yet ("scheduler", "restart"):
	// such a scenario is recorded, and compared once Capabilities has the word.
	Needs string
	// Open says that the engine differs from the reference here and it is not decided yet which
	// of the two is right - it names where the decision is to be made. Such a scenario is
	// played and must differ (once it no longer does, Open is out of date and the test says so);
	// it is the list of what is still to settle, never a place to park a failure.
	Open string
}

// Var is a system variable.
type Var struct {
	Name string
	// Type: bool, number, list, string, alarm, action (a boolean of subtype action, the
	// stand-in for a key).
	Type    string
	Entries []string
	// Min and Max of a number (0 and 65000 when both are zero).
	Min, Max float64
	// Value is the initial value: bool, float64, int (a list's index), string.
	Value any
}

// Position is a place on earth in degrees, north and east positive.
type Position struct {
	Lat, Lon float64
}

// Trigger modes of a line, as the database has them.
const (
	Legacy = 1  // what the editor leaves on a key line
	Change = 4  // bei Änderung auslösen
	Update = 13 // bei Aktualisierung auslösen; a time module's "zu Zeitpunkten auslösen"
	Check  = 15 // nur prüfen
)

// Comparisons, as the database has them.
const (
	Equals       = 1 // boolean and list
	Equal        = 5
	InRange      = 6
	OutOfRange   = 7
	Greater      = 8
	GreaterEqual = 9
	Less         = 10
	LessEqual    = 11
	Unequal      = 12
)

// Program is a program as the WebUI's editor builds it.
type Program struct {
	Name     string
	Inactive bool
	Rules    []Rule
}

// Rule is WENN/SONST WENN with its DANN, or the final SONST (no Groups).
type Rule struct {
	// Groups are the blocks; lines of a block are joined by AND and blocks by OR unless
	// LinesOr / GroupsAnd say otherwise.
	Groups    [][]Line
	LinesOr   bool
	GroupsAnd bool
	// Cancel is "vor dem Ausführen alle laufenden Verzögerungen für diese Aktivitäten beenden".
	Cancel bool
	Acts   []Act
}

// Line is a condition line on a variable or a time module.
type Line struct {
	Var string
	// Cmp is the comparison (Equals when zero) with V1 (and V2 for the ranges): bool, float64,
	// int (a list's index) or string.
	Cmp    int
	V1, V2 any
	Mode   int
	Time   *TimeModule
}

// Act is an activity.
type Act struct {
	// Mark sets the marker of that name: the trace gets "<time> <mark>".
	Mark string
	// Var and Value write a variable.
	Var   string
	Value any
	// Script is a script activity (only the two one-liners the import knows keep a program
	// importable).
	Script string
	// Delay is "verzögert um", in seconds.
	Delay int
}

// Time module kinds (TimerType).
const (
	Periodic = 4
	Weekly   = 5
	Monthly  = 6
	Yearly   = 7
	Once     = 8
	Daily    = 9
)

// Weekday bits of a time module.
const (
	Mon = 1 << iota
	Tue
	Wed
	Thu
	Fri
	Sat
	Sun
	Workdays = Mon | Tue | Wed | Thu | Fri
	Weekend  = Sat | Sun
)

// TimeModule holds the eleven values the WebUI's time module dialog sends (timemodule.htm,
// system.fn::saveTimeModule), named as there; a field left empty is the dialog's default "0".
type TimeModule struct {
	Type int
	Time string // "2007-01-01 HH:MM:SS"; empty: ganztägig or astro
	// Offset is an astro module's distance from the sun's event in seconds (the dialog sends it
	// in place of the time, and saveTimeModule stores it with TimeSeconds).
	Offset   int
	Caldur   int    // the range's length in seconds
	Caltime  string // the date of "einmalig"
	Wd       int
	Period   int
	Calrep   int
	Begin    string // "YYYY-MM-DD"; empty: the day before the scenario starts, as the dialog proposes
	Calcount int
	End      string // "YYYY-MM-DD"
	// Sot is the astro kind (SunOffsetType), 0 for none.
	Sot int
}

// Astro kinds of a time module (SunOffsetType). The WebUI of the reference stores the Before and
// After kinds with an offset; a module that runs to the sun's other event has no duration.
const (
	Sunrise = 1 + iota
	BeforeSunrise
	AfterSunrise
	Sunset
	BeforeSunset
	AfterSunset
)

// Step is something done at a time.
type Step struct {
	// At is when, from Start.
	At time.Duration
	// Set writes Value to the variable of that name (as a script's State() does).
	Set   string
	Value any
	// Run starts the program of that name without its conditions (ProgramExecute).
	Run string
	// Activate / Deactivate switch the program of that name.
	Activate, Deactivate string
	// Restart stops the logic with its state saved and starts it again.
	Restart bool
}

// StartTime is Start as a time.
func (s *Scenario) StartTime() time.Time {
	t, err := parseWall(s.Start)
	if err != nil {
		panic(fmt.Sprintf("scenario %s: start %q: %v", s.Name, s.Start, err))
	}
	return t
}

// SpeedRate is Rate with its default.
func (s *Scenario) SpeedRate() int {
	if s.Rate <= 0 {
		return 1
	}
	return s.Rate
}

// SlackTime is Slack with its default.
func (s *Scenario) SlackTime() time.Duration {
	if s.Slack > 0 {
		return s.Slack
	}
	return time.Duration(2*s.SpeedRate()) * time.Second
}

// Timed says whether a program of the scenario has a time module.
func (s *Scenario) Timed() bool {
	for _, p := range s.Programs {
		for _, r := range p.Rules {
			for _, g := range r.Groups {
				for _, l := range g {
					if l.Time != nil {
						return true
					}
				}
			}
		}
	}
	return false
}

// astro says whether a program of the scenario has an astro time module.
func (s *Scenario) astro() bool {
	for _, p := range s.Programs {
		for _, r := range p.Rules {
			for _, g := range r.Groups {
				for _, l := range g {
					if l.Time != nil && l.Time.Sot != 0 {
						return true
					}
				}
			}
		}
	}
	return false
}

// Marks lists the markers the scenario's programs set, sorted.
func (s *Scenario) Marks() []string {
	seen := map[string]bool{}
	for _, p := range s.Programs {
		for _, r := range p.Rules {
			for _, a := range r.Acts {
				if a.Mark != "" {
					seen[a.Mark] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// Check says what is wrong with a scenario's own text.
func (s *Scenario) Check() error {
	if !nameOK(s.Name) {
		return fmt.Errorf("scenario %q: the name must be lower case letters, digits and hyphens", s.Name)
	}
	if _, err := time.ParseInLocation(TimeLayout, s.Start, Zone); err != nil {
		return fmt.Errorf("scenario %s: start %q is not a time", s.Name, s.Start)
	}
	if s.Mark != Reproduce && s.Mark != Fix {
		return fmt.Errorf("scenario %s: the mark must be reproduce or fix", s.Name)
	}
	if s.Mark == Fix && s.Note == "" {
		return fmt.Errorf("scenario %s: a fix needs a note that says what differs", s.Name)
	}
	if s.Until <= 0 {
		return fmt.Errorf("scenario %s: no end", s.Name)
	}
	if s.astro() && s.Position == nil {
		return fmt.Errorf("scenario %s: an astro time module needs a position", s.Name)
	}
	vars := map[string]*Var{}
	for i := range s.Vars {
		v := &s.Vars[i]
		if !asciiName(v.Name) || vars[v.Name] != nil {
			return fmt.Errorf("scenario %s: variable name %q", s.Name, v.Name)
		}
		if _, err := literal(v, v.Value); err != nil {
			return fmt.Errorf("scenario %s: variable %s: %v", s.Name, v.Name, err)
		}
		vars[v.Name] = v
	}
	progs := map[string]bool{}
	for _, p := range s.Programs {
		if !asciiName(p.Name) || progs[p.Name] {
			return fmt.Errorf("scenario %s: program name %q", s.Name, p.Name)
		}
		progs[p.Name] = true
		for _, r := range p.Rules {
			for _, g := range r.Groups {
				for _, l := range g {
					if l.Time != nil {
						continue
					}
					if vars[l.Var] == nil {
						return fmt.Errorf("scenario %s: program %s: no variable %q", s.Name, p.Name, l.Var)
					}
				}
			}
			for _, a := range r.Acts {
				if a.Var != "" && vars[a.Var] == nil {
					return fmt.Errorf("scenario %s: program %s: no variable %q", s.Name, p.Name, a.Var)
				}
				if a.Mark != "" && !nameOK(a.Mark) {
					return fmt.Errorf("scenario %s: program %s: marker %q", s.Name, p.Name, a.Mark)
				}
			}
		}
	}
	last := time.Duration(-1)
	for _, st := range s.Steps {
		if st.At < last || st.At > s.Until {
			return fmt.Errorf("scenario %s: the steps must be in order and before the end", s.Name)
		}
		last = st.At
		switch {
		case st.Set != "":
			v := vars[st.Set]
			if v == nil {
				return fmt.Errorf("scenario %s: step: no variable %q", s.Name, st.Set)
			}
			if _, err := literal(v, st.Value); err != nil {
				return fmt.Errorf("scenario %s: step: %s: %v", s.Name, st.Set, err)
			}
		case st.Run != "", st.Activate != "", st.Deactivate != "":
			if !progs[st.Run+st.Activate+st.Deactivate] {
				return fmt.Errorf("scenario %s: step: no program %q", s.Name, st.Run+st.Activate+st.Deactivate)
			}
		case st.Restart:
		default:
			return fmt.Errorf("scenario %s: a step that does nothing", s.Name)
		}
	}
	return nil
}

func nameOK(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func asciiName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\'' || r == '\\' {
			return false
		}
	}
	return true
}
