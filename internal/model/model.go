// Package model is what a program and a system variable are: the types, their validation, and
// the canonical YAML they are stored as (codec.go) - one file per program and per variable,
// written the same way every time, so the same object always gives the same bytes and a diff
// shows only what changed.
//
// The shape is the CCU's on purpose: an ordered chain of rules (WENN, any number of SONST WENN,
// at most one SONST), each condition two levels deep (groups of lines), each line a comparison
// on a device datapoint, a variable or a time module with a trigger mode, each activity a write,
// immediate or delayed. A CCU user recognises it, and an import is one to one.
package model

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GroupOp joins the lines of a group, or the groups of a condition.
type GroupOp string

const (
	// All is UND: every part must hold.
	All GroupOp = "all"
	// Any is ODER: one part is enough.
	Any GroupOp = "any"
)

// Trigger is a line's trigger mode: when the line makes the program look at its conditions.
// It is never part of the condition itself.
type Trigger string

const (
	// TriggerChange is the CCU's "bei Änderung auslösen": when the truth of this one line
	// flips, in either direction - not when the value changes.
	TriggerChange Trigger = "change"
	// TriggerUpdate is "bei Aktualisierung auslösen": at every report, the same value included.
	// On a time module it is "zu Zeitpunkten auslösen".
	TriggerUpdate Trigger = "update"
	// TriggerValue is "bei jeder Wertänderung": when the value differs from the one before.
	// The CCU does not have it.
	TriggerValue Trigger = "value"
	// TriggerCheck is "nur prüfen": the line never triggers.
	TriggerCheck Trigger = "check"
)

// Op is a line's comparison.
type Op string

const (
	OpEq Op = "eq"
	OpNe Op = "ne"
	OpGt Op = "gt"
	OpGe Op = "ge"
	OpLt Op = "lt"
	OpLe Op = "le"
	// OpRange is the CCU's "im Wertebereich von … bis kleiner …": from <= value < below.
	OpRange Op = "range"
	// OpOutside is the CCU's comparison type 7 (value out of range): value < from or
	// value > to. The CCU's editor cannot make it; it exists for imported lines.
	OpOutside Op = "outside"
)

// Ops lists the comparisons in the order an editor offers them.
var Ops = []Op{OpEq, OpNe, OpGt, OpGe, OpLt, OpLe, OpRange, OpOutside}

// Triggers lists the trigger modes.
var Triggers = []Trigger{TriggerChange, TriggerUpdate, TriggerValue, TriggerCheck}

// Ref names a device datapoint as lite-rpc does: the interface, the channel's address and the
// parameter. In a file it is "Interface/ADDRESS:CH/PARAM".
type Ref struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Param     string `json:"param"`
}

var (
	refIface = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)
	refAddr  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}:[0-9]{1,3}$`)
	refParam = regexp.MustCompile(`^[A-Za-z0-9_.]{1,64}$`)
)

// String is the reference as a file has it.
func (r Ref) String() string { return r.Interface + "/" + r.Address + "/" + r.Param }

// Channel is the reference's channel, "Interface/ADDRESS:CH".
func (r Ref) Channel() string { return r.Interface + "/" + r.Address }

// IsZero says whether the reference names nothing.
func (r Ref) IsZero() bool { return r == Ref{} }

// ParseRef reads "Interface/ADDRESS:CH/PARAM".
func ParseRef(s string) (Ref, error) {
	p := strings.Split(s, "/")
	if len(p) != 3 || !refIface.MatchString(p[0]) || !refAddr.MatchString(p[1]) || !refParam.MatchString(p[2]) {
		return Ref{}, fmt.Errorf("%q is not a datapoint (Interface/ADDRESS:CHANNEL/PARAMETER)", s)
	}
	return Ref{Interface: p[0], Address: p[1], Param: p[2]}, nil
}

// ChannelRef answers a channel ("Interface/ADDRESS:CH") as a reference without a parameter;
// the zero one when s is no channel.
func ChannelRef(s string) Ref {
	if ParseChannel(s) != nil {
		return Ref{}
	}
	p := strings.Split(s, "/")
	return Ref{Interface: p[0], Address: p[1]}
}

// ParseChannel checks "Interface/ADDRESS:CH".
func ParseChannel(s string) error {
	p := strings.Split(s, "/")
	if len(p) != 2 || !refIface.MatchString(p[0]) || !refAddr.MatchString(p[1]) {
		return fmt.Errorf("%q is not a channel (Interface/ADDRESS:CHANNEL)", s)
	}
	return nil
}

// Duration is a delay or a length as a file has it: "30s", "5min", "2h", "1h30min".
type Duration time.Duration

var durationPart = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)min)?(?:(\d+)s)?$`)

// ParseDuration reads hours, minutes and seconds, in that order, each optional: "90s", "1h30min".
func ParseDuration(s string) (Duration, error) {
	m := durationPart.FindStringSubmatch(s)
	if m == nil || s == "" {
		return 0, fmt.Errorf("%q is not a duration (like 30s, 5min, 2h or 1h30min)", s)
	}
	var total time.Duration
	for i, unit := range []time.Duration{time.Hour, time.Minute, time.Second} {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.ParseInt(m[i+1], 10, 64)
		if err != nil || n > 1<<31 {
			return 0, fmt.Errorf("%q is not a duration: a part is too large", s)
		}
		total += time.Duration(n) * unit
	}
	return Duration(total), nil
}

// String writes the duration the one way a file has it: whole hours, then minutes, then seconds.
func (d Duration) String() string {
	total := int64(time.Duration(d) / time.Second)
	if total <= 0 {
		return "0s"
	}
	var b strings.Builder
	if h := total / 3600; h > 0 {
		fmt.Fprintf(&b, "%dh", h)
	}
	if m := total % 3600 / 60; m > 0 {
		fmt.Fprintf(&b, "%dmin", m)
	}
	if s := total % 60; s > 0 {
		fmt.Fprintf(&b, "%ds", s)
	}
	return b.String()
}

// Std is the duration as the standard library's.
func (d Duration) Std() time.Duration { return time.Duration(d) }

// MarshalText writes the duration as a file has it (for the page's JSON).
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText reads it.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// Program is one program.
type Program struct {
	// Key is the file's name without .yaml: a slug of the name at creation, stable afterwards.
	Key  string `json:"key"`
	Name string `json:"name"`
	// Note is the CCU's description.
	Note string `json:"note,omitempty"`
	// Active: only an active program reacts to triggers.
	Active bool `json:"active"`
	// Visible and Operable are the CCU's "sichtbar" and "bedienbar" (both true unless said).
	Visible  bool `json:"visible"`
	Operable bool `json:"operable"`
	// Internal is the CCU's "systemintern": hidden from the ordinary lists.
	Internal bool `json:"internal,omitempty"`
	// AtStart is "beim Systemstart prüfen": the program is evaluated once after a boot of the
	// system, as the CCU does with every program. On for imported programs, off for new ones.
	AtStart bool   `json:"at_start,omitempty"`
	Rules   []Rule `json:"rules"`
}

// Rule is one block of the chain: WENN or SONST WENN with a condition, or SONST without.
type Rule struct {
	// Else marks the SONST block: no condition, it runs when no block before it was true.
	Else bool `json:"else,omitempty"`
	// When is the condition of a WENN or SONST WENN block; one without groups is always true.
	When *Condition `json:"when,omitempty"`
	Note string     `json:"note,omitempty"`
	// CancelPending is the CCU's checkbox "vor dem Ausführen alle laufenden Verzögerungen für
	// diese Aktivitäten beenden": before the block runs, pending delayed actions for the targets
	// this block names are removed - whoever scheduled them.
	CancelPending bool     `json:"cancel_pending,omitempty"`
	Then          []Action `json:"then"`
}

// Condition is two levels deep, as the CCU's: groups of lines.
type Condition struct {
	// Op joins the groups.
	Op     GroupOp `json:"op"`
	Groups []Group `json:"groups"`
}

// Group is one "Bedingung" block of lines.
type Group struct {
	// Op joins the lines.
	Op    GroupOp `json:"op"`
	Lines []Line  `json:"lines"`
}

// Line is one condition line: exactly one of Device, Variable and Time is set.
type Line struct {
	Device   *Ref        `json:"device,omitempty"`
	Variable string      `json:"variable,omitempty"`
	Time     *TimeModule `json:"time,omitempty"`
	// Op and the values: Value for the simple comparisons; From and Below for OpRange; From
	// and To for OpOutside. A value is a bool, a number or a string (a value list's entry by
	// its label). A time line has none.
	Op    Op  `json:"op,omitempty"`
	Value any `json:"value,omitempty"`
	From  any `json:"from,omitempty"`
	Below any `json:"below,omitempty"`
	To    any `json:"to,omitempty"`
	// Not negates the line (OpenCCU's "nicht bei").
	Not bool `json:"not,omitempty"`
	// For is "hält seit": the line is true only once its comparison has held without a break
	// for this long - and the moment that is reached triggers the program, unless the line
	// only checks. The CCU does not have it. Not on a time line.
	For     Duration `json:"for,omitempty"`
	Trigger Trigger  `json:"trigger"`
	Note    string   `json:"note,omitempty"`
}

// ActionKind says what an activity does.
type ActionKind string

const (
	// ActionSet writes a device datapoint.
	ActionSet ActionKind = "set"
	// ActionVariable writes a variable.
	ActionVariable ActionKind = "variable"
	// ActionRun runs another program: Mode "then" runs its first block without looking at the
	// conditions (the CCU's ProgramExecute), "check" evaluates it.
	ActionRun ActionKind = "run"
	// ActionActivate and ActionDeactivate switch another program on or off.
	ActionActivate   ActionKind = "activate"
	ActionDeactivate ActionKind = "deactivate"
	// ActionScript is an imported script, kept as text and never run.
	ActionScript ActionKind = "script"
	// ActionWebhook calls a web address, ActionWake sends a Wake-on-LAN.
	ActionWebhook ActionKind = "webhook"
	ActionWake    ActionKind = "wake"
	// ActionChannel writes several datapoints of one channel in one command (the interface's
	// putParamset on VALUES): a coloured light takes its colour, its brightness and its
	// behaviour together.
	ActionChannel ActionKind = "channel"
	// ActionCommand runs a program of the system with the addon's rights (command.go).
	ActionCommand ActionKind = "command"
)

// MaxValues bounds the datapoints one ActionChannel writes.
const MaxValues = 32

// Run modes of ActionRun.
const (
	RunThen  = "then"
	RunCheck = "check"
)

// Action is one activity.
type Action struct {
	Kind ActionKind `json:"kind"`
	// Device is ActionSet's target, Variable ActionVariable's, Program the other program's key.
	Device   *Ref   `json:"device,omitempty"`
	Variable string `json:"variable,omitempty"`
	Program  string `json:"program,omitempty"`
	// What is written: To (a bool, a number, a string), or the current value of another
	// datapoint (FromDevice) or variable (FromVariable), or Special - one of the CCU's special
	// values ("Letzter Wert"), kept as imported; such an action is skipped until it is edited.
	To           any    `json:"to,omitempty"`
	FromDevice   *Ref   `json:"from_device,omitempty"`
	FromVariable string `json:"from_variable,omitempty"`
	Special      string `json:"special,omitempty"`
	// Mode is ActionRun's.
	Mode string `json:"mode,omitempty"`
	// Script is the kept text.
	Script string `json:"script,omitempty"`
	// Channel ("Interface/ADDRESS:CH") and Values are ActionChannel's: the datapoints to
	// write, in the order they are listed. A value is true or false, a number, or a text -
	// for a datapoint with a list of entries the entry's name.
	Channel string       `json:"channel,omitempty"`
	Values  []ParamValue `json:"values,omitempty"`
	// Webhook is ActionWebhook's request, Wake ActionWake's target.
	Webhook *Webhook `json:"webhook,omitempty"`
	// Display is ActionDisplay's: what the display channel Channel shows.
	Display *Display `json:"display,omitempty"`
	Wake    *Wake    `json:"wake,omitempty"`
	// Command is ActionCommand's: what is run, and what becomes of its output.
	Command *Command `json:"command,omitempty"`
	// OnFor is "ein für": the value is written together with the channel's on-time (ON_TIME),
	// in one command - the device switches back by itself. Only a "set" with a fixed value.
	OnFor Duration `json:"on_for,omitempty"`
	// After is "verzögert um": counted from the trigger, not from the line above. With
	// AfterUpTo the delay is drawn at random, to the second, between After and AfterUpTo
	// each time the block runs ("verzögert um zufällig ... bis ...").
	After     Duration `json:"after,omitempty"`
	AfterUpTo Duration `json:"after_up_to,omitempty"`
	Note      string   `json:"note,omitempty"`
}

// ParamValue is one parameter of a channel with the value to write.
type ParamValue struct {
	Param string `json:"param"`
	Value any    `json:"value"`
}

// Target is the key the registry of pending delays files this action under; a block's
// CancelPending removes the pending entries with the targets the block names.
func (a Action) Target() string {
	switch a.Kind {
	case ActionSet:
		if a.Device != nil {
			return "dp:" + a.Device.String()
		}
	case ActionVariable:
		return "var:" + a.Variable
	case ActionRun, ActionActivate, ActionDeactivate:
		return "prg:" + a.Program
	case ActionChannel, ActionDisplay:
		return "ch:" + a.Channel
	case ActionWebhook:
		if a.Webhook != nil {
			return "url:" + a.Webhook.URL
		}
	case ActionWake:
		if a.Wake != nil {
			return "mac:" + a.Wake.MAC
		}
	case ActionCommand:
		if a.Command != nil {
			return "cmd:" + a.Command.Program()
		}
	}
	return ""
}

// TimeModule is a CCU time module: when, in which pattern, and for how long it is valid. The
// scheduler (package schedule) says what each field does to the time; in short:
//
//   - when on a day: exactly one of At (a clock time), Astro (sunrise or sunset) and AllDay;
//   - a point or a range: with a Duration - and always with Astro or AllDay - it is a range,
//     true from its begin to its end; without it is a point, true at its instant only;
//   - on which days: Kind with Every, Weekdays, Day, Nth, Month and Date;
//   - how long: From, Until and Count.
type TimeModule struct {
	// Kind: once, interval, daily, weekly, monthly, yearly.
	Kind string `json:"kind"`
	// At is the clock time "HH:MM" or "HH:MM:SS" on the system's clock.
	At string `json:"at,omitempty"`
	// AllDay is the CCU's "ganztägig": the whole day, from midnight to midnight.
	AllDay bool `json:"all_day,omitempty"`
	// Astro: day ("von Sonnenaufgang ... bis Sonnenuntergang") and night ("von Sonnenuntergang
	// ... bis Sonnenaufgang") begin Offset after the sun's event, before_sunrise and
	// before_sunset begin Offset before it; sunrise and sunset are day and night as an old
	// CCU stored them, without an offset. All of them are ranges: to the sun's other event,
	// or for Duration.
	Astro  string   `json:"astro,omitempty"`
	Offset Duration `json:"offset,omitempty"`
	// Point makes an astro module a point instead of a range: it fires once, at the sun's
	// event shifted by Offset, and is true at that instant only. Twilight moves the sun's
	// events to a twilight: civil (the sun 6 degrees below the horizon: dawn before sunrise,
	// dusk after sunset), nautical (12) or astronomical (18). The CCU has neither.
	Point    bool   `json:"point,omitempty"`
	Twilight string `json:"twilight,omitempty"`
	// Duration makes a clock time a range, and gives an astro range its length.
	Duration Duration `json:"duration,omitempty"`
	// Interval is the period of an interval module.
	Interval Duration `json:"interval,omitempty"`
	// Every: every n days (daily), weeks (weekly) or months (monthly), counted from From.
	Every int `json:"every,omitempty"`
	// Weekdays: the days of a weekly module; on a daily one the CCU's "werktags" and "am
	// Wochenende"; with Nth the one weekday of a monthly or yearly module.
	Weekdays []string `json:"weekdays,omitempty"`
	// Day is the day of the month of a monthly or yearly module; Nth (1 to 5) with one weekday
	// is "on the n-th Monday" instead. Month is a yearly module's month.
	Day   int `json:"day,omitempty"`
	Nth   int `json:"nth,omitempty"`
	Month int `json:"month,omitempty"`
	// Date is a once module's date; From and Until the validity window ("YYYY-MM-DD" or with
	// a time "YYYY-MM-DD HH:MM"): what begins at or after From and before Until - an Until
	// without a time includes that day, as the CCU's "endet am" does. Count ends
	// the module after that many times, counted from From.
	Date  string `json:"date,omitempty"`
	From  string `json:"from,omitempty"`
	Until string `json:"until,omitempty"`
	Count int    `json:"count,omitempty"`
}

// TimeKinds, AstroKinds and Weekdays are the words a time module uses.
var (
	TimeKinds  = []string{"once", "interval", "daily", "weekly", "monthly", "yearly"}
	AstroKinds = []string{"sunrise", "before_sunrise", "day", "sunset", "before_sunset", "night"}
	Weekdays   = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	Twilights  = []string{"civil", "nautical", "astronomical"}
)

// MaxHold is the longest "for" of a line and "on_for" of an activity.
const MaxHold = Duration(7 * 24 * time.Hour)

// MaxTimeRange is the longest range a time module can have.
const MaxTimeRange = Duration(7 * 24 * time.Hour)

// Check says what is wrong with a time module's fields taken together - nil when the scheduler
// can run it. Each field's own form (a clock time, a date, a word) is the codec's to check.
func (t *TimeModule) Check() error {
	n := 0
	for _, set := range []bool{t.At != "", t.Astro != "", t.AllDay} {
		if set {
			n++
		}
	}
	if n != 1 {
		return fmt.Errorf("a time module has exactly one of \"at\", \"astro\" and \"all_day\"")
	}
	if t.Offset != 0 && t.Astro == "" {
		return fmt.Errorf("\"offset\" belongs to \"astro\"")
	}
	if t.Offset != 0 && (t.Astro == "sunrise" || t.Astro == "sunset") {
		return fmt.Errorf("astro: %s has no \"offset\" (day, night, before_sunrise and before_sunset have)", t.Astro)
	}
	if t.Offset < 0 || t.Offset >= Duration(24*time.Hour) {
		return fmt.Errorf("offset: less than a day, and not negative (before_sunrise and before_sunset are the earlier ones)")
	}
	if t.Duration < 0 || t.Duration > MaxTimeRange {
		return fmt.Errorf("duration: at most %s", MaxTimeRange)
	}
	if (t.Point || t.Twilight != "") && t.Astro == "" {
		return fmt.Errorf("\"point\" and \"twilight\" belong to \"astro\"")
	}
	if t.Point && t.Duration != 0 {
		return fmt.Errorf("a point has no \"duration\"")
	}
	if t.Duration != 0 && t.AllDay {
		return fmt.Errorf("an all-day module has no \"duration\"")
	}
	if (t.Kind == "interval") != (t.Interval > 0) {
		return fmt.Errorf("an interval module, and only that, has an \"interval\"")
	}
	only := func(field string, set bool, kinds ...string) error {
		if !set {
			return nil
		}
		for _, k := range kinds {
			if t.Kind == k {
				return nil
			}
		}
		return fmt.Errorf("a module of kind %s has no %q", t.Kind, field)
	}
	for _, err := range []error{
		only("every", t.Every != 0, "daily", "weekly", "monthly"),
		only("weekdays", len(t.Weekdays) > 0, "daily", "weekly", "monthly", "yearly"),
		only("day", t.Day != 0, "monthly", "yearly"),
		only("nth", t.Nth != 0, "monthly", "yearly"),
		only("month", t.Month != 0, "yearly"),
		only("date", t.Date != "", "once"),
	} {
		if err != nil {
			return err
		}
	}
	switch t.Kind {
	case "once":
		if t.Date == "" {
			return fmt.Errorf("a module of kind once needs a \"date\"")
		}
		if len(t.Date) > len("2006-01-02") {
			return fmt.Errorf("date: the day only, the time is \"at\"")
		}
	case "weekly":
		if len(t.Weekdays) == 0 {
			return fmt.Errorf("a weekly module needs \"weekdays\"")
		}
	case "monthly", "yearly":
		if t.Kind == "yearly" && t.Month == 0 {
			return fmt.Errorf("a yearly module needs a \"month\"")
		}
		byDay, byWeekday := t.Day != 0, t.Nth != 0 || len(t.Weekdays) > 0
		if byDay == byWeekday {
			return fmt.Errorf("a %s module is on a \"day\" of the month, or on the \"nth\" of one weekday", t.Kind)
		}
		if byWeekday && (t.Nth < 1 || t.Nth > 5 || len(t.Weekdays) != 1) {
			return fmt.Errorf("\"nth\" is 1 to 5 and goes with exactly one weekday")
		}
	}
	if (t.Every > 1 || t.Count > 0) && t.From == "" {
		return fmt.Errorf("\"every\" and \"count\" are counted from \"from\": it is missing")
	}
	if t.From != "" && t.Until != "" && padUntil(t.Until) <= padDate(t.From) {
		return fmt.Errorf("until: it is not after \"from\"")
	}
	return nil
}

// padDate makes "YYYY-MM-DD" comparable with "YYYY-MM-DD HH:MM" as text.
func padDate(s string) string {
	if len(s) == len("2006-01-02") {
		return s + " 00:00"
	}
	return s
}

// padUntil is padDate for "until": a date without a time is the end of that day.
func padUntil(s string) string {
	if len(s) == len("2006-01-02") {
		return s + " 24:00"
	}
	return s
}

// VarType is a system variable's type.
type VarType string

const (
	// VarBool is the CCU's Logikwert, VarList its Werteliste, VarNumber its Zahl, VarString
	// its Zeichenkette, VarAlarm its Alarm.
	VarBool   VarType = "bool"
	VarList   VarType = "list"
	VarNumber VarType = "number"
	VarString VarType = "string"
	VarAlarm  VarType = "alarm"
)

// VarTypes lists the types.
var VarTypes = []VarType{VarBool, VarList, VarNumber, VarString, VarAlarm}

// Labels names a bool's or an alarm's two states ("ist falsch" / "ist wahr").
type Labels struct {
	False string `json:"false,omitempty"`
	True  string `json:"true,omitempty"`
}

// Variable is one system variable's definition. Its value is not part of it: values live in the
// state file, not in the tree.
type Variable struct {
	Key  string  `json:"key"`
	Name string  `json:"name"`
	Note string  `json:"note,omitempty"`
	Type VarType `json:"type"`
	// Labels: bool and alarm. Entries: list - a program names an entry by its label.
	Labels  *Labels  `json:"labels,omitempty"`
	Entries []string `json:"entries,omitempty"`
	// Unit, Min and Max: number. The limits hold for manual input and for writes from outside;
	// a program may write beyond them, as on the CCU.
	Unit string   `json:"unit,omitempty"`
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	// Visible (true unless said), Internal and Logged are the CCU's flags.
	Visible  bool `json:"visible"`
	Internal bool `json:"internal,omitempty"`
	Logged   bool `json:"logged,omitempty"`
	// Channel is the CCU's "Kanalzuordnung", "Interface/ADDRESS:CH"; information only.
	Channel string `json:"channel,omitempty"`
	// StartValue is "Wert beim Systemstart": set to this after a boot of the system; nil keeps
	// the value. The CCU's presence variable has true here.
	StartValue any `json:"start_value,omitempty"`
	// Computed makes a number a computed value (read-only, calculated from its inputs).
	Computed *Computed `json:"computed,omitempty"`
	// Helper makes a number one a device helper maintains (a counter's total, today's or
	// yesterday's amount).
	Helper *Helper `json:"helper,omitempty"`
}

// Default is the value a variable has before anything wrote it: false, the first entry, the
// number 0 (or its minimum when that is above 0), the empty string.
func (v *Variable) Default() any {
	switch v.Type {
	case VarBool, VarAlarm:
		return false
	case VarList:
		if len(v.Entries) > 0 {
			return v.Entries[0]
		}
		return ""
	case VarNumber:
		if v.Min != nil && *v.Min > 0 {
			return *v.Min
		}
		return float64(0)
	}
	return ""
}

var keyPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,46}[a-z0-9])?$`)

// ValidKey says whether s can be a key: lower-case letters, digits and hyphens, 1 to 48
// characters, no hyphen at either end.
func ValidKey(s string) bool { return keyPattern.MatchString(s) && !strings.Contains(s, "--") }

var slugReplace = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss",
	"é", "e", "è", "e", "ê", "e", "á", "a", "à", "a", "â", "a", "ó", "o", "ò", "o", "ô", "o", "ú", "u", "ù", "u", "û", "u",
	"í", "i", "ì", "i", "î", "i", "ç", "c", "ñ", "n", "å", "a", "ø", "o", "æ", "ae")

// Slug makes a key of a name: "Markise bei Regen einfahren" becomes
// "markise-bei-regen-einfahren". Whatever is not a letter or a digit becomes one hyphen; a name
// without any gives "x".
func Slug(name string) string {
	s := slugReplace.Replace(strings.ToLower(name))
	var b strings.Builder
	hyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			hyphen = false
			b.WriteRune(r)
		default:
			hyphen = true
		}
		if b.Len() >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = strings.TrimRight(out[:48], "-")
	}
	if out == "" {
		return "x"
	}
	return out
}

// UniqueKey answers Slug(name), or with -2, -3, … appended while taken says it is.
func UniqueKey(name string, taken func(string) bool) string {
	base := Slug(name)
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		suffix := "-" + strconv.Itoa(n)
		b := base
		if len(b)+len(suffix) > 48 {
			b = strings.TrimRight(b[:48-len(suffix)], "-")
		}
		if k := b + suffix; !taken(k) {
			return k
		}
	}
}
