// Package schedule says when a time module fires and when it is true: the calendar and the sun,
// and nothing else. It has no clock and no state - every answer is a function of the module, the
// time zone, the position and an instant - which is what makes it testable to the last case.
//
// The rules, each pinned by a test (the old CCU is the reference where it was measured; "fix"
// marks what the CCU does differently):
//
//   - A module occurs on the days its pattern names, inside its validity window. On such a day it
//     begins at its clock time, at the sun's event (shifted by the offset), or at midnight.
//   - A point (a clock time without a duration) fires once, at its instant, and is true at that
//     instant only: looked at at any other moment it is false.
//   - A range (a clock time with a duration, every astro module, an all-day module) fires when it
//     begins and when it ends, and is true in between: the begin belongs to it, the end does
//     not. The weekday and the date are those of the day it begins on, also when it ends after
//     midnight. The end of a range that is followed at once by its own next begin (all day,
//     every day) does not fire.
//   - A range with a clock time ends at its clock time - begin plus duration on the wall clock,
//     not that many seconds later: 22:00 for eight hours ends at 06:00, also in the night the
//     clock changes (as the CCU: measured). An astro range with a duration lasts that long.
//   - An interval fires by elapsed time: all day at every multiple of the interval counted from
//     the day's first moment, beginning again at every midnight (the last interval of a day is
//     short when the interval does not divide the day), and true all day - another trigger
//     that evaluates the program finds it true (as the CCU: measured; there it is false only
//     until its first firing after a start, which the engine reproduces); with a range from
//     the range's begin to its end, true inside the range, and the end fires once more; with a
//     point once, at the point. On a day without a clock change that is the wall
//     clock's multiples; across a change the series keeps its spacing (as the CCU: measured -
//     a 7-minute interval fires at 01:59 and 03:06 in the spring night, at 02:55 summer time
//     and 02:02 winter time in the autumn night). The CCU's one extra firing at the moment of
//     the spring change is not reproduced (fix).
//   - A clock time the day does not have (the hour skipped in spring) happens at the moment the
//     clock jumps: 02:00 and 02:30 both at 03:00 (fix: the old CCU fired them an hour early, at
//     01:00 and 01:30 winter time; a newer ReGaHss does it this way). A range that lies in the
//     skipped hour altogether still begins and ends: it begins at the jump and ends a second
//     later. A clock time the day has twice (the hour repeated in autumn) happens the first
//     time.
//   - A day of the month the month does not have (the 31st in April, 29 February) and a fifth
//     weekday it does not have are left out: the module does not fire that month.
//   - Every n days, weeks or months counts from the day, the week (Monday to Sunday) or the
//     month of "from".
//   - An astro module marked as a point fires once, at the sun's event shifted by its offset,
//     and is true at that instant only (the CCU has ranges only). With a twilight, "sunrise"
//     and "sunset" are the dawn and the dusk of that twilight.
//   - Sunrise and sunset are the position's, and the real ones (the CCU's own are up to ten
//     minutes off, measured; the comparison's astro scenario holds kinds and order); on a day without the event (polar night, midnight
//     sun) the module does not begin, and a range that waits for the sun's other event stays
//     true until that happens - "day" through the whole midnight sun.
package schedule

import (
	"fmt"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// EdgeKind says what happens to a module at an instant.
type EdgeKind int

const (
	// Point is a point in time: the module is true at this instant only.
	Point EdgeKind = iota
	// Begin is the begin of a range: true from here.
	Begin
	// Repeat is an interval firing again inside its range.
	Repeat
	// End is the end of a range: false from here.
	End
)

func (k EdgeKind) String() string {
	return [...]string{"point", "begin", "repeat", "end"}[k]
}

// Edge is one instant at which a module fires.
type Edge struct {
	At   time.Time
	Kind EdgeKind
}

// Module is a time module made ready to be asked.
type Module struct {
	loc *time.Location
	pos Position

	kind     string
	allDay   bool
	astro    string
	point    bool    // an astro module that is a point
	zenith   float64 // where the sun stands at "sunrise" and "sunset" (a twilight's when set)
	atSec    int64
	offset   time.Duration
	dur      time.Duration
	interval time.Duration
	every    int
	weekdays [7]bool // Monday first
	anyDay   bool    // no weekday named
	day      int
	nth      int
	month    int
	date     int64 // once: the day
	from     time.Time
	until    time.Time
	hasFrom  bool
	hasUntil bool
	anchor   int64 // the day "every" counts from
	count    int

	limitOnce sync.Once
	limit     time.Time

	// suns remembers the sun of the days asked for: a range near the poles looks at many
	sunMu sync.Mutex
	suns  map[int64]SunDay
}

const daySec = 86400

// Compile makes a module of its definition, for a time zone and a position. The definition
// must have passed model.TimeModule.Check; what Check cannot see (a date that does not exist)
// is an error here.
func Compile(def *model.TimeModule, loc *time.Location, pos Position) (*Module, error) {
	if err := def.Check(); err != nil {
		return nil, err
	}
	if loc == nil {
		loc = time.Local
	}
	m := &Module{loc: loc, pos: pos, kind: def.Kind, allDay: def.AllDay, astro: def.Astro,
		offset: def.Offset.Std(), dur: def.Duration.Std(), interval: def.Interval.Std(),
		every: def.Every, day: def.Day, nth: def.Nth, month: def.Month, count: def.Count, point: def.Point, zenith: zenith}
	if z, ok := twilightZenith[def.Twilight]; ok {
		m.zenith = z
	}
	if def.At != "" {
		at, err := time.Parse("15:04:05", (def.At + ":00")[:8])
		if err != nil {
			return nil, fmt.Errorf("at: %q is not a time of day", def.At)
		}
		m.atSec = int64(at.Hour()*3600 + at.Minute()*60 + at.Second())
	}
	m.anyDay = len(def.Weekdays) == 0
	for _, w := range def.Weekdays {
		for i, name := range model.Weekdays {
			if name == w {
				m.weekdays[i] = true
			}
		}
	}
	instant := func(s, field string) (time.Time, error) {
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
				return m.first(t.Unix()), nil
			}
		}
		return time.Time{}, fmt.Errorf("%s: %q is not a date", field, s)
	}
	var err error
	if def.Date != "" {
		d, err := time.ParseInLocation("2006-01-02", def.Date, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("date: %q is not a date", def.Date)
		}
		m.date = d.Unix() / daySec
	}
	if def.From != "" {
		if m.from, err = instant(def.From, "from"); err != nil {
			return nil, err
		}
		m.hasFrom = true
		m.anchor = m.dayOf(m.from)
	}
	if def.Until != "" {
		if m.until, err = instant(def.Until, "until"); err != nil {
			return nil, err
		}
		// "endet am" with a date and no time includes that day, as on the CCU: the end is the
		// begin of the day after it
		if len(def.Until) == len("2006-01-02") {
			m.until = m.startOfDay(m.dayOf(m.until) + 1)
		}
		m.hasUntil = true
	}
	return m, nil
}

// NeedsPosition says the module is an astro one and the position is not known: it never fires.
func (m *Module) NeedsPosition() bool { return m.astro != "" && !m.pos.Known }

// ---- the wall clock ----

func (m *Module) offsetAt(unix int64) int64 {
	_, off := time.Unix(unix, 0).In(m.loc).Zone()
	return int64(off)
}

// wall is an instant's wall clock as seconds, as if the clock's reading were UTC.
func (m *Module) wall(t time.Time) int64 { return t.Unix() + m.offsetAt(t.Unix()) }

// dayOf is the civil day an instant lies on, counted from 1970-01-01.
func (m *Module) dayOf(t time.Time) int64 { return floorDiv(m.wall(t), daySec) }

// instants answers the moments at which the wall clock reads w: one, none (the skipped hour)
// or two (the repeated hour), in order.
func (m *Module) instants(w int64) []time.Time {
	a, b := m.offsetAt(w-daySec), m.offsetAt(w+daySec)
	var out []time.Time
	if m.offsetAt(w-a) == a {
		out = append(out, time.Unix(w-a, 0).In(m.loc))
	}
	if b != a && m.offsetAt(w-b) == b {
		out = append(out, time.Unix(w-b, 0).In(m.loc))
	}
	return out // in order: a clock that repeats an hour went back, so the earlier offset is the larger one
}

// first answers the moment the wall clock reads w for the first time - or, when it never does,
// the moment the clock jumps over it.
func (m *Module) first(w int64) time.Time {
	if all := m.instants(w); len(all) > 0 {
		return all[0]
	}
	a, b := w-m.offsetAt(w-daySec), w-m.offsetAt(w+daySec)
	if b < a {
		a, b = b, a
	}
	return time.Unix(m.transition(a, b), 0).In(m.loc)
}

// transition answers the first second in (lo, hi] with another offset than lo's (hi's must be
// another one).
func (m *Module) transition(lo, hi int64) int64 {
	o := m.offsetAt(lo)
	for hi-lo > 1 {
		mid := lo + (hi-lo)/2
		if m.offsetAt(mid) == o {
			lo = mid
		} else {
			hi = mid
		}
	}
	return hi
}

// startOfDay is the first moment of a civil day.
func (m *Module) startOfDay(d int64) time.Time { return m.first(d * daySec) }

// aligned answers the first instant at or after t at which an all-day interval fires: a
// multiple of the interval after the first moment of t's day, by elapsed time, or the next day's
// first moment when that comes first.
func (m *Module) aligned(t time.Time) time.Time {
	iv := int64(m.interval / time.Second)
	d := m.dayOf(t)
	start, next := m.startOfDay(d).Unix(), m.startOfDay(d+1).Unix()
	at := start + (t.Unix()-start+iv-1)/iv*iv
	if at > next {
		at = next
	}
	return time.Unix(at, 0).In(m.loc)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

// ---- the pattern: which days ----

func civil(d int64) (year int, month time.Month, day int, weekday int) {
	t := time.Unix(d*daySec, 0).UTC()
	year, month, day = t.Date()
	return year, month, day, (int(t.Weekday()) + 6) % 7 // Monday is 0
}

func dayNumber(year int, month time.Month, day int) int64 {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / daySec
}

// dayIn answers the module's day in a month: the Day-th, or the Nth of its weekday; false when
// the month has no such day.
func (m *Module) dayIn(year int, month time.Month) (int64, bool) {
	if m.day > 0 {
		d := dayNumber(year, month, m.day)
		if _, mo, _, _ := civil(d); mo != month {
			return 0, false
		}
		return d, true
	}
	first := dayNumber(year, month, 1)
	_, _, _, wd := civil(first)
	want := 0
	for i, on := range m.weekdays {
		if on {
			want = i
		}
	}
	d := first + int64((want-wd+7)%7) + int64(7*(m.nth-1))
	if _, mo, _, _ := civil(d); mo != month {
		return 0, false
	}
	return d, true
}

func weekIndex(d int64) int64 { return floorDiv(d+3, 7) } // 1970-01-05 was a Monday

func monthIndex(d int64) int64 {
	y, mo, _, _ := civil(d)
	return int64(y)*12 + int64(mo) - 1
}

// matches says whether the module occurs on a day by its pattern (the validity window is the
// instants' matter).
func (m *Module) matches(d int64) bool {
	_, _, _, wd := civil(d)
	switch m.kind {
	case "once":
		return d == m.date
	case "interval":
		return true
	case "daily":
		return (m.every <= 1 || floorMod(d-m.anchor, int64(m.every)) == 0) && (m.anyDay || m.weekdays[wd])
	case "weekly":
		return m.weekdays[wd] && (m.every <= 1 || floorMod(weekIndex(d)-weekIndex(m.anchor), int64(m.every)) == 0)
	}
	y, mo, _, _ := civil(d)
	if m.kind == "yearly" && int(mo) != m.month {
		return false
	}
	if m.kind == "monthly" && m.every > 1 && floorMod(monthIndex(d)-monthIndex(m.anchor), int64(m.every)) != 0 {
		return false
	}
	want, ok := m.dayIn(y, mo)
	return ok && want == d
}

// nextDay answers the first day at or after d the pattern names; false when there is none
// (within what is worth looking at: 500 fitting months, 40 years).
func (m *Module) nextDay(d int64) (int64, bool) {
	switch m.kind {
	case "once":
		return m.date, m.date >= d
	case "interval":
		return d, true
	case "daily", "weekly":
		// at most every n-th week or day is tried, and of those two weeks' worth
		for tries := 0; tries < 15; {
			switch {
			case m.kind == "daily" && m.every > 1 && floorMod(d-m.anchor, int64(m.every)) != 0:
				d += int64(m.every) - floorMod(d-m.anchor, int64(m.every))
			case m.kind == "weekly" && m.every > 1 && floorMod(weekIndex(d)-weekIndex(m.anchor), int64(m.every)) != 0:
				skip := int64(m.every) - floorMod(weekIndex(d)-weekIndex(m.anchor), int64(m.every))
				d = (weekIndex(d)+skip)*7 - 3
			case m.matches(d):
				return d, true
			default:
				d++
				tries++
			}
		}
		return 0, false
	case "monthly":
		mi := monthIndex(d)
		for tries := 0; tries < 500; tries++ {
			if m.every > 1 {
				if r := floorMod(mi-monthIndex(m.anchor), int64(m.every)); r != 0 {
					mi += int64(m.every) - r
				}
			}
			if want, ok := m.dayIn(int(floorDiv(mi, 12)), time.Month(floorMod(mi, 12)+1)); ok && want >= d {
				return want, true
			}
			mi++
		}
		return 0, false
	}
	y, _, _, _ := civil(d)
	for year := y; year <= y+40; year++ {
		if want, ok := m.dayIn(year, time.Month(m.month)); ok && want >= d {
			return want, true
		}
	}
	return 0, false
}

// ---- the occurrence of a day ----

// sunEvent answers the sunrise or the sunset of a day. Where the midnight sun begins and ends,
// the formulas give a last sunset with no sunrise after it, and a first sunrise with no sunset
// before it: the sun only touched the horizon. Those two do not count - the day simply goes on.
func (m *Module) sunEvent(d int64, rise bool) (time.Time, bool) {
	if !m.pos.Known {
		return time.Time{}, false
	}
	sun := func(d int64) SunDay {
		m.sunMu.Lock()
		defer m.sunMu.Unlock()
		if v, ok := m.suns[d]; ok {
			return v
		}
		if m.suns == nil || len(m.suns) > 2000 {
			m.suns = map[int64]SunDay{}
		}
		y, mo, day, _ := civil(d)
		m.suns[d] = SunAt(y, mo, day, m.pos, m.zenith)
		return m.suns[d]
	}
	today := sun(d)
	if rise {
		return today.Rise.In(m.loc), today.HasRise && !sun(d-1).Up
	}
	return today.Set.In(m.loc), today.HasSet && !sun(d+1).Up
}

func (m *Module) byDay() bool {
	return m.astro == "sunrise" || m.astro == "before_sunrise" || m.astro == "day"
}

// occurrence answers when the module begins and ends on a day its pattern names: s == e for a
// point. ok is false when it does not happen that day (no sunrise, an offset beyond the end).
func (m *Module) occurrence(d int64) (s, e time.Time, ok bool) {
	switch {
	case m.allDay:
		return m.startOfDay(d), m.startOfDay(d + 1), true
	case m.astro == "":
		s = m.first(d*daySec + m.atSec)
		if m.dur == 0 {
			return s, s, true
		}
		e = m.first(d*daySec + m.atSec + int64(m.dur/time.Second))
		if !e.After(s) {
			// the whole range lies in an hour the day does not have: it still begins and ends
			e = s.Add(time.Second)
		}
		return s, e, true
	}
	event, has := m.sunEvent(d, m.byDay())
	if !has {
		return s, e, false
	}
	s = event.Add(m.offset)
	if m.astro == "before_sunrise" || m.astro == "before_sunset" {
		s = event.Add(-m.offset)
	}
	if m.point {
		return s, s, true
	}
	if m.dur > 0 {
		return s, s.Add(m.dur), true
	}
	// to the sun's other event: the first one after this one, however long that takes (half a
	// year at the poles)
	found := false
	for ahead := int64(0); ahead < 400 && !found; ahead++ {
		if other, has := m.sunEvent(d+ahead, !m.byDay()); has && other.After(event) {
			e, found = other, true
		}
	}
	return s, e, found && e.After(s)
}

// valid says whether something that happens at t is inside the validity window and the count.
func (m *Module) valid(t time.Time, limit bool) bool {
	if m.hasFrom && t.Before(m.from) {
		return false
	}
	if m.hasUntil && !t.Before(m.until) {
		return false
	}
	return !limit || m.count == 0 || !t.After(m.lastCounted())
}

// lastCounted is the instant of the count-th time the module fires, counted from "from":
// nothing after it happens. It is the same after every restart - the count is the calendar's,
// not a counter.
func (m *Module) lastCounted() time.Time {
	m.limitOnce.Do(func() {
		at := m.from.Add(-time.Second)
		for n := 0; n < m.count; {
			e, ok := m.next(at, false)
			if !ok {
				break
			}
			at = e.At
			if e.Kind != End {
				n++
				m.limit = at
			}
		}
	})
	return m.limit
}

// allDayInterval says the module is an interval without a range or a point.
func (m *Module) allDayInterval() bool { return m.kind == "interval" && m.allDay }

// AllDayInterval says the module is an interval without a range or a point. Such a module is
// true all day by the calendar; on the CCU it reads false until it has fired for the first time
// after a start, which is the engine's to know (it has the clock and the state).
func (m *Module) AllDayInterval() bool { return m.allDayInterval() }

// lookBack is how many days before an instant an occurrence that still matters may have begun.
func (m *Module) lookBack() int64 {
	switch {
	case m.astro != "" && m.dur == 0 && !m.point:
		if m.pos.Lat > 60 || m.pos.Lat < -60 {
			return 200 // a polar day or night
		}
		return 2
	case m.astro != "":
		return int64(m.dur/(24*time.Hour)) + 3
	}
	return int64(m.dur/(24*time.Hour)) + 2
}

// edges answers what an occurrence does: its instants in order.
func (m *Module) edges(d int64, after time.Time, limit bool) []Edge {
	s, e, ok := m.occurrence(d)
	if !ok || !m.valid(s, limit) {
		return nil
	}
	if !e.After(s) {
		return []Edge{{s, Point}}
	}
	out := []Edge{{s, Begin}}
	if m.kind == "interval" && m.dur+time.Duration(len(m.astro)) > 0 {
		// the first time the interval fires again after "after", inside the range
		lo := s.Add(time.Second)
		if after.Add(time.Second).After(lo) {
			lo = after.Truncate(time.Second).Add(time.Second)
		}
		r := s.Add(m.interval * ((lo.Sub(s) + m.interval - 1) / m.interval))
		if r.Before(e) && m.valid(r, limit) {
			out = append(out, Edge{r, Repeat})
		}
	}
	// the end, unless the module begins again at that very moment
	if nd := m.dayOf(e); !(nd != d && m.matches(nd) && m.beginsAt(nd, e, limit)) {
		out = append(out, Edge{e, End})
	}
	return out
}

func (m *Module) beginsAt(d int64, t time.Time, limit bool) bool {
	s, _, ok := m.occurrence(d)
	return ok && s.Equal(t) && m.valid(s, limit)
}

// Next answers the first time the module fires after an instant; false when it never does
// again.
func (m *Module) Next(after time.Time) (Edge, bool) { return m.next(after, true) }

func (m *Module) next(after time.Time, limit bool) (Edge, bool) {
	if m.allDayInterval() {
		t := after.Truncate(time.Second).Add(time.Second)
		if m.hasFrom && t.Before(m.from) {
			t = m.from
		}
		t = m.aligned(t)
		return Edge{t, Point}, m.valid(t, limit)
	}
	var best Edge
	found := false
	d := m.dayOf(after) - m.lookBack()
	if m.hasFrom && m.dayOf(m.from)-1 > d {
		d = m.dayOf(m.from) - 1
	}
	for tries := 0; tries < 800; tries++ {
		var ok bool
		if d, ok = m.nextDay(d); !ok {
			break
		}
		begin := m.startOfDay(d).Add(-26 * time.Hour) // nothing of this day or a later one is earlier
		if found && begin.After(best.At) {
			break
		}
		if (m.hasUntil && begin.After(m.until)) || (limit && m.count > 0 && begin.After(m.lastCounted())) {
			break
		}
		for _, e := range m.edges(d, after, limit) {
			if e.At.After(after) && (!found || e.At.Before(best.At)) {
				best, found = e, true
			}
		}
		d++
	}
	return best, found
}

// Upcoming answers the next n times the module fires after an instant.
func (m *Module) Upcoming(after time.Time, n int) []Edge {
	var out []Edge
	for len(out) < n {
		e, ok := m.Next(after)
		if !ok {
			break
		}
		out = append(out, e)
		after = e.At
	}
	return out
}

// Active says whether the module is true at an instant (to the second): inside a range, or
// exactly at a point's instant. An all-day interval is true all day (see AllDayInterval).
func (m *Module) Active(at time.Time) bool {
	at = at.Truncate(time.Second)
	if m.allDayInterval() {
		return m.valid(at, true)
	}
	today := m.dayOf(at)
	for d := today + 1; d >= today-m.lookBack(); d-- {
		if !m.matches(d) {
			continue
		}
		s, e, ok := m.occurrence(d)
		if !ok || !m.valid(s, true) || s.After(at) {
			continue
		}
		if at.Before(e) || at.Equal(s) {
			return true
		}
		if m.astro != "" && m.dur == 0 && !m.point {
			return false // the latest one that began is over: so are the ones before it
		}
	}
	return false
}
