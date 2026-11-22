package schedule

import (
	"fmt"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // the tests name their zones; the runner need not have them

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const stamp = "2006-01-02 15:04:05 MST"

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// at reads "2020-03-29 01:59:48 CET" in a zone: the abbreviation picks the pass of a repeated
// hour.
func at(t *testing.T, l *time.Location, s string) time.Time {
	t.Helper()
	// (a zone without a name is written as its offset, which the layout cannot read back)
	v, err := time.ParseInLocation("2006-01-02 15:04:05", s[:19], l)
	if err != nil {
		t.Fatal(err)
	}
	if v.Format(stamp) != s {
		// the other pass of a repeated hour
		for _, d := range []time.Duration{time.Hour, -time.Hour, 30 * time.Minute, -30 * time.Minute} {
			if v.Add(d).Format(stamp) == s {
				return v.Add(d)
			}
		}
		t.Fatalf("%s does not exist in %s (got %s)", s, l, v.Format(stamp))
	}
	return v
}

func d(s string) model.Duration {
	v, err := model.ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return v
}

var berlin = Position{Lat: 52.52, Lon: 13.405, Known: true}

func compile(t *testing.T, def model.TimeModule, l *time.Location, pos Position) *Module {
	t.Helper()
	m, err := Compile(&def, l, pos)
	if err != nil {
		t.Fatalf("compile %+v: %v", def, err)
	}
	return m
}

func show(edges []Edge) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		out[i] = e.At.Format(stamp) + " " + e.Kind.String()
	}
	return out
}

type fireCase struct {
	name  string
	zone  string
	def   model.TimeModule
	after string
	want  []string
	// last says nothing fires after the wanted ones
	last bool
}

func runFires(t *testing.T, cases []fireCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			zn := c.zone
			if zn == "" {
				zn = "Europe/Berlin"
			}
			l := zone(t, zn)
			m := compile(t, c.def, l, berlin)
			n := len(c.want)
			if c.last {
				n++
			}
			got := show(m.Upcoming(at(t, l, c.after), n))
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("after %s\n got:\n  %s\nwant:\n  %s", c.after, strings.Join(got, "\n  "), strings.Join(c.want, "\n  "))
			}
		})
	}
}

func daily(clock string) model.TimeModule { return model.TimeModule{Kind: "daily", At: clock} }
func every(iv string) model.TimeModule {
	return model.TimeModule{Kind: "interval", AllDay: true, Interval: d(iv)}
}
func span(clock, length string) model.TimeModule {
	return model.TimeModule{Kind: "daily", At: clock, Duration: d(length)}
}

// The spring change, Europe/Berlin 2020-03-29: 02:00 CET becomes 03:00 CEST. The fixed times
// and the one-minute and ten-second timers are OpenCCU's ReGaHss-Test (07a to 07c): what the
// real ReGaHss does - a newer one than the reference's (R1.00.0388.0254 on the lab's stock OpenCCU
// fires a time in the skipped hour an hour early, 02:00 at 01:00 CET; that is not reproduced: fix).
func TestSpringChange(t *testing.T) {
	runFires(t, []fireCase{
		{name: "01:00 before the change", def: daily("01:00"), after: "2020-03-29 00:59:48 CET", want: []string{"2020-03-29 01:00:00 CET point"}},
		{name: "01:55 before the change", def: daily("01:55"), after: "2020-03-29 01:54:48 CET", want: []string{"2020-03-29 01:55:00 CET point"}},
		{name: "02:00 does not exist: at the jump", def: daily("02:00"), after: "2020-03-29 01:59:48 CET",
			want: []string{"2020-03-29 03:00:00 CEST point", "2020-03-30 02:00:00 CEST point"}},
		{name: "02:30 does not exist: at the jump", def: daily("02:30"), after: "2020-03-29 01:59:48 CET",
			want: []string{"2020-03-29 03:00:00 CEST point", "2020-03-30 02:30:00 CEST point"}},
		{name: "02:59:59 does not exist: at the jump", def: daily("02:59:59"), after: "2020-03-28 12:00:00 CET",
			want: []string{"2020-03-29 03:00:00 CEST point"}},
		{name: "03:00 is the jump itself", def: daily("03:00"), after: "2020-03-29 01:59:48 CET", want: []string{"2020-03-29 03:00:00 CEST point"}},
		{name: "03:05 after the change", def: daily("03:05"), after: "2020-03-29 03:04:48 CEST", want: []string{"2020-03-29 03:05:00 CEST point"}},
		{name: "03:30 after the change", def: daily("03:30"), after: "2020-03-29 03:29:48 CEST", want: []string{"2020-03-29 03:30:00 CEST point"}},
		{name: "every minute across the change", def: every("1min"), after: "2020-03-29 01:58:40 CET",
			want: []string{"2020-03-29 01:59:00 CET point", "2020-03-29 03:00:00 CEST point", "2020-03-29 03:01:00 CEST point"}},
		{name: "every ten seconds across the change", def: every("10s"), after: "2020-03-29 01:59:50 CET",
			want: []string{"2020-03-29 03:00:00 CEST point", "2020-03-29 03:00:10 CEST point", "2020-03-29 03:00:20 CEST point"}},
		{name: "every minute the night before", def: every("1min"), after: "2020-03-28 23:58:48 CET",
			want: []string{"2020-03-28 23:59:00 CET point", "2020-03-29 00:00:00 CET point", "2020-03-29 00:01:00 CET point"}},
		{name: "every minute the night after", def: every("1min"), after: "2020-03-29 23:58:48 CEST",
			want: []string{"2020-03-29 23:59:00 CEST point", "2020-03-30 00:00:00 CEST point", "2020-03-30 00:01:00 CEST point"}},
		// measured on the CCU (ReGaHss 0254, task 14's t06): 01:52, 01:59 CET, then 03:06, 03:13 CEST - the
		// series keeps its spacing; the CCU's extra firing at 03:00:00 is not reproduced (fix)
		{name: "every seven minutes keeps its spacing across the missing hour", def: every("7min"), after: "2020-03-29 01:50:00 CET",
			want: []string{"2020-03-29 01:52:00 CET point", "2020-03-29 01:59:00 CET point", "2020-03-29 03:06:00 CEST point", "2020-03-29 03:13:00 CEST point"}},
		{name: "every two hours: two hours after midnight is 03:00 that night", def: every("2h"), after: "2020-03-29 00:30:00 CET",
			want: []string{"2020-03-29 03:00:00 CEST point", "2020-03-29 05:00:00 CEST point"}},
		{name: "every two hours: the day's last one is short, midnight begins again", def: every("2h"), after: "2020-03-29 21:30:00 CEST",
			want: []string{"2020-03-29 23:00:00 CEST point", "2020-03-30 00:00:00 CEST point", "2020-03-30 02:00:00 CEST point"}},
		{name: "a range over the change ends at its clock time (as the CCU: measured)", def: span("22:00", "8h"), after: "2020-03-28 21:00:00 CET",
			want: []string{"2020-03-28 22:00:00 CET begin", "2020-03-29 06:00:00 CEST end", "2020-03-29 22:00:00 CEST begin"}},
		{name: "a range that ends in the missing hour ends at the jump", def: span("01:30", "1h"), after: "2020-03-29 00:00:00 CET",
			want: []string{"2020-03-29 01:30:00 CET begin", "2020-03-29 03:00:00 CEST end"}},
		{name: "a range that begins in the missing hour begins at the jump", def: span("02:30", "1h"), after: "2020-03-29 00:00:00 CET",
			want: []string{"2020-03-29 03:00:00 CEST begin", "2020-03-29 03:30:00 CEST end"}},
		// (the old CCU ran such a range an hour early, 01:10 to 01:30 CET: both edges happened)
		{name: "a range inside the missing hour begins at the jump and ends a second later", def: span("02:10", "20min"), after: "2020-03-29 00:00:00 CET",
			want: []string{"2020-03-29 03:00:00 CEST begin", "2020-03-29 03:00:01 CEST end", "2020-03-30 02:10:00 CEST begin", "2020-03-30 02:30:00 CEST end"}},
		{name: "an interval with a range that begins in the missing hour", def: model.TimeModule{Kind: "interval", At: "02:30", Duration: d("1h"), Interval: d("15min")},
			after: "2020-03-29 00:00:00 CET",
			want:  []string{"2020-03-29 03:00:00 CEST begin", "2020-03-29 03:15:00 CEST repeat", "2020-03-29 03:30:00 CEST end"}},
	})
}

// The autumn change, Europe/Berlin 2020-10-25: 03:00 CEST becomes 02:00 CET, the hour from
// 02:00 to 03:00 happens twice. ReGaHss-Test 07b and 07d: a fixed time in it fires the first
// time, the periodic timer runs through both.
func TestAutumnChange(t *testing.T) {
	runFires(t, []fireCase{
		{name: "01:55 before the change", def: daily("01:55"), after: "2020-10-25 01:54:48 CEST", want: []string{"2020-10-25 01:55:00 CEST point"}},
		{name: "02:00 fires the first time only", def: daily("02:00"), after: "2020-10-25 01:59:48 CEST",
			want: []string{"2020-10-25 02:00:00 CEST point", "2020-10-26 02:00:00 CET point"}},
		{name: "02:30 fires the first time only", def: daily("02:30"), after: "2020-10-25 02:29:48 CEST",
			want: []string{"2020-10-25 02:30:00 CEST point", "2020-10-26 02:30:00 CET point"}},
		{name: "02:30, asked in the second pass: tomorrow", def: daily("02:30"), after: "2020-10-25 02:10:00 CET",
			want: []string{"2020-10-26 02:30:00 CET point"}},
		{name: "02:55 fires the first time only", def: daily("02:55"), after: "2020-10-25 02:54:48 CEST",
			want: []string{"2020-10-25 02:55:00 CEST point", "2020-10-26 02:55:00 CET point"}},
		{name: "03:00 after the change", def: daily("03:00"), after: "2020-10-25 02:59:48 CET", want: []string{"2020-10-25 03:00:00 CET point"}},
		{name: "03:30 after the change", def: daily("03:30"), after: "2020-10-25 03:29:48 CET", want: []string{"2020-10-25 03:30:00 CET point"}},
		{name: "every minute across the change", def: every("1min"), after: "2020-10-25 02:58:40 CEST",
			want: []string{"2020-10-25 02:59:00 CEST point", "2020-10-25 02:00:00 CET point", "2020-10-25 02:01:00 CET point"}},
		{name: "every ten seconds across the change", def: every("10s"), after: "2020-10-25 02:59:49 CEST",
			want: []string{"2020-10-25 02:59:50 CEST point", "2020-10-25 02:00:00 CET point", "2020-10-25 02:00:10 CET point"}},
		{name: "every minute the night before", def: every("1min"), after: "2020-10-24 23:58:48 CEST",
			want: []string{"2020-10-24 23:59:00 CEST point", "2020-10-25 00:00:00 CEST point"}},
		{name: "every minute the night after", def: every("1min"), after: "2020-10-25 23:58:48 CET",
			want: []string{"2020-10-25 23:59:00 CET point", "2020-10-26 00:00:00 CET point"}},
		// measured on the CCU (task 14's t07): … 02:55 CEST, then 02:02, 02:09 … 02:58 CET, 03:05 CET
		{name: "every seven minutes keeps its spacing through the repeated hour", def: every("7min"), after: "2020-10-25 02:50:00 CEST",
			want: []string{"2020-10-25 02:55:00 CEST point", "2020-10-25 02:02:00 CET point", "2020-10-25 02:09:00 CET point"}},
		{name: "every seven minutes after the repeated hour", def: every("7min"), after: "2020-10-25 02:55:00 CET",
			want: []string{"2020-10-25 02:58:00 CET point", "2020-10-25 03:05:00 CET point"}},
		{name: "every two hours: every two hours that night too", def: every("2h"), after: "2020-10-25 00:30:00 CEST",
			want: []string{"2020-10-25 02:00:00 CEST point", "2020-10-25 03:00:00 CET point", "2020-10-25 05:00:00 CET point"}},
		{name: "every two hours: the 25th hour ends the day", def: every("2h"), after: "2020-10-25 21:30:00 CET",
			want: []string{"2020-10-25 23:00:00 CET point", "2020-10-26 00:00:00 CET point", "2020-10-26 02:00:00 CET point"}},
		{name: "a range over the change ends at its clock time", def: span("22:00", "8h"), after: "2020-10-24 21:00:00 CEST",
			want: []string{"2020-10-24 22:00:00 CEST begin", "2020-10-25 06:00:00 CET end"}},
		{name: "a range that ends in the repeated hour ends the first time", def: span("01:30", "1h"), after: "2020-10-25 00:00:00 CEST",
			want: []string{"2020-10-25 01:30:00 CEST begin", "2020-10-25 02:30:00 CEST end", "2020-10-26 01:30:00 CET begin"}},
		{name: "a range that begins in the repeated hour begins the first time", def: span("02:30", "1h"), after: "2020-10-25 00:00:00 CEST",
			want: []string{"2020-10-25 02:30:00 CEST begin", "2020-10-25 03:30:00 CET end"}},
		{name: "a range inside the repeated hour happens once", def: span("02:10", "20min"), after: "2020-10-25 00:00:00 CEST",
			want: []string{"2020-10-25 02:10:00 CEST begin", "2020-10-25 02:30:00 CEST end", "2020-10-26 02:10:00 CET begin"}},
		{name: "an interval with a range runs through the repeated hour twice", def: model.TimeModule{Kind: "interval", At: "02:00", Duration: d("2h"), Interval: d("30min")},
			after: "2020-10-25 01:00:00 CEST",
			want: []string{"2020-10-25 02:00:00 CEST begin", "2020-10-25 02:30:00 CEST repeat", "2020-10-25 02:00:00 CET repeat", "2020-10-25 02:30:00 CET repeat",
				"2020-10-25 03:00:00 CET repeat", "2020-10-25 03:30:00 CET repeat", "2020-10-25 04:00:00 CET end"}},
	})
}

// Other zones change differently: New York at 02:00 local, Lord Howe Island by half an hour,
// Havana at midnight - there the day begins at 01:00 - and the southern half the other way
// round.
func TestOtherZones(t *testing.T) {
	runFires(t, []fireCase{
		{name: "New York spring, 02:30", zone: "America/New_York", def: daily("02:30"), after: "2026-03-08 01:00:00 EST",
			want: []string{"2026-03-08 03:00:00 EDT point"}},
		{name: "New York autumn, 01:30 the first time", zone: "America/New_York", def: daily("01:30"), after: "2026-11-01 00:00:00 EDT",
			want: []string{"2026-11-01 01:30:00 EDT point", "2026-11-02 01:30:00 EST point"}},
		{name: "Sydney's spring is in October", zone: "Australia/Sydney", def: daily("02:30"), after: "2026-10-04 01:00:00 AEST",
			want: []string{"2026-10-04 03:00:00 AEDT point"}},
		{name: "Sydney's autumn is in April", zone: "Australia/Sydney", def: every("30min"), after: "2026-04-05 02:20:00 AEDT",
			want: []string{"2026-04-05 02:30:00 AEDT point", "2026-04-05 02:00:00 AEST point", "2026-04-05 02:30:00 AEST point", "2026-04-05 03:00:00 AEST point"}},
		{name: "Lord Howe skips half an hour", zone: "Australia/Lord_Howe", def: daily("02:15"), after: "2026-10-04 01:00:00 +1030",
			want: []string{"2026-10-04 02:30:00 +11 point"}},
		{name: "Lord Howe, every 20 minutes", zone: "Australia/Lord_Howe", def: every("20min"), after: "2026-10-04 01:30:00 +1030",
			want: []string{"2026-10-04 01:40:00 +1030 point", "2026-10-04 02:30:00 +11 point", "2026-10-04 02:50:00 +11 point"}},
		{name: "Havana: midnight does not exist, the day begins at 01:00", zone: "America/Havana", def: model.TimeModule{Kind: "daily", AllDay: true},
			after: "2026-03-07 12:00:00 CST",
			want:  []string{"2026-03-08 01:00:00 CDT begin", "2026-03-09 00:00:00 CDT begin"}},
		{name: "Havana: 00:00 at the jump", zone: "America/Havana", def: daily("00:00"), after: "2026-03-07 12:00:00 CST",
			want: []string{"2026-03-08 01:00:00 CDT point"}},
		{name: "a zone without changes", zone: "Asia/Kolkata", def: daily("02:30"), after: "2026-03-29 00:00:00 IST",
			want: []string{"2026-03-29 02:30:00 IST point", "2026-03-30 02:30:00 IST point"}},
	})
	// Havana's short first hour: 23:15 is the day's last multiple, then the day begins at 01:00
	l := zone(t, "America/Havana")
	m := compile(t, every("45min"), l, berlin)
	got := show(m.Upcoming(at(t, l, "2026-03-07 23:20:00 CST"), 3))
	want := []string{"2026-03-08 01:00:00 CDT point", "2026-03-08 01:45:00 CDT point", "2026-03-08 02:30:00 CDT point"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Havana every 45 min: %v, want %v", got, want)
	}
}

// Intervals as measured on the stock OpenCCU (concept Appendix B-5, measurements/intervals.log
// of 2026-11-21): the phase is the module's own start, the series begins again at midnight, a
// range's end fires once more, a point fires once.
func TestIntervalsAsMeasured(t *testing.T) {
	rng := func(clock, length, iv string) model.TimeModule {
		return model.TimeModule{Kind: "interval", At: clock, Duration: d(length), Interval: d(iv)}
	}
	runFires(t, []fireCase{
		{name: "5 min all day", def: every("5min"), after: "2026-11-21 22:17:52 CET",
			want: []string{"2026-11-21 22:20:00 CET point", "2026-11-21 22:25:00 CET point"}},
		{name: "7 min all day: multiples from midnight", def: every("7min"), after: "2026-11-21 22:17:52 CET",
			want: []string{"2026-11-21 22:24:00 CET point", "2026-11-21 22:31:00 CET point", "2026-11-21 22:38:00 CET point"}},
		{name: "7 min at midnight: the last interval is short", def: every("7min"), after: "2026-11-21 23:50:00 CET",
			want: []string{"2026-11-21 23:55:00 CET point", "2026-11-22 00:00:00 CET point", "2026-11-22 00:07:00 CET point", "2026-11-22 00:14:00 CET point"}},
		{name: "31 min all day", def: every("31min"), after: "2026-11-21 22:17:52 CET",
			want: []string{"2026-11-21 22:44:00 CET point", "2026-11-21 23:15:00 CET point", "2026-11-21 23:46:00 CET point", "2026-11-22 00:00:00 CET point", "2026-11-22 00:31:00 CET point"}},
		{name: "5 min from 22:27 to 23:42", def: rng("22:27", "1h15min", "5min"), after: "2026-11-21 22:17:52 CET",
			want: []string{"2026-11-21 22:27:00 CET begin", "2026-11-21 22:32:00 CET repeat", "2026-11-21 22:37:00 CET repeat"}},
		{name: "the range's end fires once more", def: rng("22:27", "1h15min", "5min"), after: "2026-11-21 23:30:00 CET",
			want: []string{"2026-11-21 23:32:00 CET repeat", "2026-11-21 23:37:00 CET repeat", "2026-11-21 23:42:00 CET end", "2026-11-22 22:27:00 CET begin"}},
		{name: "31 min from 22:27 for four hours: over midnight without a new start", def: rng("22:27", "4h", "31min"), after: "2026-11-21 22:17:52 CET",
			want: []string{"2026-11-21 22:27:00 CET begin", "2026-11-21 22:58:00 CET repeat", "2026-11-21 23:29:00 CET repeat", "2026-11-22 00:00:00 CET repeat",
				"2026-11-22 00:31:00 CET repeat", "2026-11-22 01:02:00 CET repeat", "2026-11-22 01:33:00 CET repeat", "2026-11-22 02:04:00 CET repeat", "2026-11-22 02:27:00 CET end"}},
		{name: "a point with an interval fires once, at the point", def: model.TimeModule{Kind: "interval", At: "22:32", Interval: d("5min")}, after: "2026-11-21 22:17:52 CET",
			want: []string{"2026-11-21 22:32:00 CET point", "2026-11-22 22:32:00 CET point"}},
		{name: "an interval that ends exactly at the range's end does not fire there", def: rng("08:00", "1h", "30min"), after: "2026-11-21 08:00:00 CET",
			want: []string{"2026-11-21 08:30:00 CET repeat", "2026-11-21 09:00:00 CET end"}},
		{name: "an interval longer than its range", def: rng("08:00", "1h", "2h"), after: "2026-11-21 07:00:00 CET",
			want: []string{"2026-11-21 08:00:00 CET begin", "2026-11-21 09:00:00 CET end"}},
		{name: "every second", def: every("1s"), after: "2026-11-21 23:59:58 CET",
			want: []string{"2026-11-21 23:59:59 CET point", "2026-11-22 00:00:00 CET point", "2026-11-22 00:00:01 CET point"}},
		{name: "every 25 hours is once a day, at midnight", def: every("25h"), after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-11-22 00:00:00 CET point", "2026-11-23 00:00:00 CET point"}},
		{name: "the year's end, every minute (ReGaHss-Test 07a)", def: every("1min"), after: "2019-12-31 23:58:48 CET",
			want: []string{"2019-12-31 23:59:00 CET point", "2020-01-01 00:00:00 CET point", "2020-01-01 00:01:00 CET point"}},
		{name: "the end of a leap February, every minute (07b)", def: every("1min"), after: "2020-02-29 23:58:48 CET",
			want: []string{"2020-02-29 23:59:00 CET point", "2020-03-01 00:00:00 CET point"}},
		{name: "the end of a common February, every minute (07b)", def: every("1min"), after: "2021-02-28 23:58:48 CET",
			want: []string{"2021-02-28 23:59:00 CET point", "2021-03-01 00:00:00 CET point"}},
	})
}

func TestDaysAndMidnight(t *testing.T) {
	runFires(t, []fireCase{
		{name: "00:00 belongs to the new day (ReGaHss-Test 07a)", def: daily("00:00"), after: "2020-01-01 23:59:48 CET", want: []string{"2020-01-02 00:00:00 CET point"}},
		{name: "00:30", def: daily("00:30"), after: "2020-01-01 00:29:48 CET", want: []string{"2020-01-01 00:30:00 CET point"}},
		{name: "23:59:59", def: daily("23:59:59"), after: "2020-01-01 23:59:58 CET", want: []string{"2020-01-01 23:59:59 CET point", "2020-01-02 23:59:59 CET point"}},
		{name: "asked at the instant: the next day", def: daily("06:30"), after: "2026-11-21 06:30:00 CET", want: []string{"2026-11-22 06:30:00 CET point"}},
		{name: "02:00 on 29 February (07b)", def: daily("02:00"), after: "2020-02-29 01:59:48 CET", want: []string{"2020-02-29 02:00:00 CET point"}},
		{name: "a range to midnight", def: span("22:00", "2h"), after: "2026-11-21 21:00:00 CET",
			want: []string{"2026-11-21 22:00:00 CET begin", "2026-11-22 00:00:00 CET end", "2026-11-22 22:00:00 CET begin"}},
		{name: "a range of 24 hours never ends", def: span("06:00", "24h"), after: "2026-11-21 05:00:00 CET",
			want: []string{"2026-11-21 06:00:00 CET begin", "2026-11-22 06:00:00 CET begin", "2026-11-23 06:00:00 CET begin"}},
		{name: "a range of 30 hours overlaps itself", def: span("06:00", "30h"), after: "2026-11-21 05:00:00 CET",
			want: []string{"2026-11-21 06:00:00 CET begin", "2026-11-21 12:00:00 CET end", "2026-11-22 06:00:00 CET begin", "2026-11-22 12:00:00 CET end"}},
		{name: "all day, every day: begins at every midnight, never ends", def: model.TimeModule{Kind: "daily", AllDay: true}, after: "2026-11-20 12:00:00 CET",
			want: []string{"2026-11-21 00:00:00 CET begin", "2026-11-22 00:00:00 CET begin", "2026-11-23 00:00:00 CET begin"}},
		{name: "all day at the weekend: ends on Monday", def: model.TimeModule{Kind: "daily", AllDay: true, Weekdays: []string{"sat", "sun"}}, after: "2026-11-20 12:00:00 CET",
			want: []string{"2026-11-21 00:00:00 CET begin", "2026-11-22 00:00:00 CET begin", "2026-11-23 00:00:00 CET end", "2026-11-28 00:00:00 CET begin"}},
		{name: "all day, once", def: model.TimeModule{Kind: "once", AllDay: true, Date: "2027-01-01"}, after: "2026-11-20 12:00:00 CET",
			want: []string{"2027-01-01 00:00:00 CET begin", "2027-01-02 00:00:00 CET end"}, last: true},
		{name: "once, a point", def: model.TimeModule{Kind: "once", At: "12:00", Date: "2027-01-01"}, after: "2026-11-20 12:00:00 CET",
			want: []string{"2027-01-01 12:00:00 CET point"}, last: true},
		{name: "once, in the past", def: model.TimeModule{Kind: "once", At: "12:00", Date: "2026-01-01"}, after: "2026-11-20 12:00:00 CET", last: true},
		{name: "once, a range over midnight", def: model.TimeModule{Kind: "once", At: "23:00", Duration: d("2h"), Date: "2026-12-31"}, after: "2026-12-31 23:30:00 CET",
			want: []string{"2027-01-01 01:00:00 CET end"}, last: true},
	})
}

// Q-29: the weekdays are those of the day a range begins on - "at the weekend from 20:00 to
// 10:00" ends on Monday at 10:00.
func TestWeekdays(t *testing.T) {
	weekend := model.TimeModule{Kind: "weekly", At: "20:00", Duration: d("14h"), Weekdays: []string{"sat", "sun"}}
	runFires(t, []fireCase{
		{name: "weekend 20:00 to 10:00", def: weekend, after: "2026-11-20 12:00:00 CET", // a Friday
			want: []string{"2026-11-21 20:00:00 CET begin", "2026-11-22 10:00:00 CET end", "2026-11-22 20:00:00 CET begin", "2026-11-23 10:00:00 CET end", "2026-11-28 20:00:00 CET begin"}},
		{name: "Monday and Wednesday, 22:00 to 06:00", def: model.TimeModule{Kind: "weekly", At: "22:00", Duration: d("8h"), Weekdays: []string{"mon", "wed"}}, after: "2026-11-22 12:00:00 CET",
			want: []string{"2026-11-23 22:00:00 CET begin", "2026-11-24 06:00:00 CET end", "2026-11-25 22:00:00 CET begin", "2026-11-26 06:00:00 CET end", "2026-11-30 22:00:00 CET begin"}},
		{name: "workdays (daily)", def: model.TimeModule{Kind: "daily", At: "07:00", Weekdays: []string{"mon", "tue", "wed", "thu", "fri"}}, after: "2026-11-19 12:00:00 CET",
			want: []string{"2026-11-20 07:00:00 CET point", "2026-11-23 07:00:00 CET point", "2026-11-24 07:00:00 CET point"}},
		{name: "the weekend (daily)", def: model.TimeModule{Kind: "daily", At: "09:00", Weekdays: []string{"sat", "sun"}}, after: "2026-11-19 12:00:00 CET",
			want: []string{"2026-11-21 09:00:00 CET point", "2026-11-22 09:00:00 CET point", "2026-11-28 09:00:00 CET point"}},
		{name: "every second week, from a Monday", def: model.TimeModule{Kind: "weekly", At: "07:00", Weekdays: []string{"mon", "thu"}, Every: 2, From: "2026-11-02"}, after: "2026-11-01 00:00:00 CET",
			want: []string{"2026-11-02 07:00:00 CET point", "2026-11-05 07:00:00 CET point", "2026-11-16 07:00:00 CET point", "2026-11-19 07:00:00 CET point", "2026-11-30 07:00:00 CET point"}},
		{name: "every second week counts whole weeks: from a Wednesday, the Monday of that week is over", def: model.TimeModule{Kind: "weekly", At: "07:00", Weekdays: []string{"mon"}, Every: 2, From: "2026-11-04"}, after: "2026-11-01 00:00:00 CET",
			want: []string{"2026-11-16 07:00:00 CET point", "2026-11-30 07:00:00 CET point"}},
		{name: "every third week, asked in a week that is left out", def: model.TimeModule{Kind: "weekly", At: "07:00", Weekdays: []string{"sun"}, Every: 3, From: "2026-11-02"}, after: "2026-11-10 00:00:00 CET",
			want: []string{"2026-11-29 07:00:00 CET point", "2026-12-20 07:00:00 CET point"}},
		{name: "every third day", def: model.TimeModule{Kind: "daily", At: "07:00", Every: 3, From: "2026-12-01"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-01 07:00:00 CET point", "2026-12-04 07:00:00 CET point", "2026-12-07 07:00:00 CET point"}},
		{name: "every third day, asked in between", def: model.TimeModule{Kind: "daily", At: "07:00", Every: 3, From: "2026-12-01"}, after: "2026-12-04 07:00:00 CET",
			want: []string{"2026-12-07 07:00:00 CET point", "2026-12-10 07:00:00 CET point"}},
		{name: "every seventh day on a weekday it never is: never", def: model.TimeModule{Kind: "daily", At: "07:00", Every: 7, Weekdays: []string{"sat"}, From: "2026-11-02"}, after: "2026-11-21 00:00:00 CET", last: true},
		{name: "every second day at the weekend", def: model.TimeModule{Kind: "daily", At: "07:00", Every: 2, Weekdays: []string{"sat", "sun"}, From: "2026-11-21"}, after: "2026-11-20 00:00:00 CET",
			want: []string{"2026-11-21 07:00:00 CET point", "2026-11-29 07:00:00 CET point", "2026-12-05 07:00:00 CET point"}},
	})
	l := zone(t, "Europe/Berlin")
	m := compile(t, weekend, l, berlin)
	for when, want := range map[string]bool{
		"2026-11-21 09:00:00 CET": false, // Saturday morning: Friday is not in the pattern
		"2026-11-21 20:00:00 CET": true,  // the begin belongs to the range
		"2026-11-22 09:59:59 CET": true,
		"2026-11-22 10:00:00 CET": false, // the end does not
		"2026-11-23 09:00:00 CET": true,  // Monday morning: Sunday's range
		"2026-11-23 21:00:00 CET": false,
	} {
		if got := m.Active(at(t, l, when)); got != want {
			t.Errorf("weekend range at %s: %v, want %v", when, got, want)
		}
	}
}

// Month ends and leap years: a day the month does not have is left out.
func TestMonthsAndYears(t *testing.T) {
	monthly := func(day int) model.TimeModule { return model.TimeModule{Kind: "monthly", At: "03:00", Day: day} }
	runFires(t, []fireCase{
		{name: "the 31st", def: monthly(31), after: "2026-01-01 00:00:00 CET",
			want: []string{"2026-01-31 03:00:00 CET point", "2026-03-31 03:00:00 CEST point", "2026-05-31 03:00:00 CEST point", "2026-07-31 03:00:00 CEST point", "2026-08-31 03:00:00 CEST point"}},
		{name: "the 30th", def: monthly(30), after: "2027-01-01 00:00:00 CET",
			want: []string{"2027-01-30 03:00:00 CET point", "2027-03-30 03:00:00 CEST point", "2027-04-30 03:00:00 CEST point"}},
		{name: "the 29th in a common year", def: monthly(29), after: "2027-01-01 00:00:00 CET",
			want: []string{"2027-01-29 03:00:00 CET point", "2027-03-29 03:00:00 CEST point"}},
		{name: "the 29th in a leap year", def: monthly(29), after: "2028-01-30 00:00:00 CET",
			want: []string{"2028-02-29 03:00:00 CET point", "2028-03-29 03:00:00 CEST point"}},
		{name: "the 1st, over the year's end", def: monthly(1), after: "2026-12-01 03:00:00 CET",
			want: []string{"2027-01-01 03:00:00 CET point", "2027-02-01 03:00:00 CET point"}},
		{name: "29 February, yearly", def: model.TimeModule{Kind: "yearly", At: "12:00", Day: 29, Month: 2}, after: "2026-01-01 00:00:00 CET",
			want: []string{"2028-02-29 12:00:00 CET point", "2032-02-29 12:00:00 CET point"}},
		{name: "29 February over 2100, which is no leap year", def: model.TimeModule{Kind: "yearly", At: "12:00", Day: 29, Month: 2}, after: "2096-03-01 00:00:00 CET",
			want: []string{"2104-02-29 12:00:00 CET point"}},
		{name: "30 February: never", def: model.TimeModule{Kind: "yearly", At: "12:00", Day: 30, Month: 2}, after: "2026-01-01 00:00:00 CET", last: true},
		{name: "the 31st every twelfth month from a February: never", def: model.TimeModule{Kind: "monthly", At: "03:00", Day: 31, Every: 12, From: "2026-02-01"}, after: "2026-01-01 00:00:00 CET", last: true},
		{name: "24 December", def: model.TimeModule{Kind: "yearly", At: "00:00", Day: 24, Month: 12}, after: "2026-12-24 00:00:00 CET",
			want: []string{"2027-12-24 00:00:00 CET point"}},
		{name: "the fifth Friday: only in months that have one", def: model.TimeModule{Kind: "monthly", At: "08:00", Nth: 5, Weekdays: []string{"fri"}}, after: "2026-01-01 00:00:00 CET",
			want: []string{"2026-01-30 08:00:00 CET point", "2026-05-29 08:00:00 CEST point", "2026-07-31 08:00:00 CEST point", "2026-10-30 08:00:00 CET point"}},
		{name: "the first Monday", def: model.TimeModule{Kind: "monthly", At: "08:00", Nth: 1, Weekdays: []string{"mon"}}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-07 08:00:00 CET point", "2027-01-04 08:00:00 CET point", "2027-02-01 08:00:00 CET point"}},
		{name: "the fourth Thursday in November", def: model.TimeModule{Kind: "yearly", At: "08:00", Nth: 4, Weekdays: []string{"thu"}, Month: 11}, after: "2026-01-01 00:00:00 CET",
			want: []string{"2026-11-26 08:00:00 CET point", "2027-11-25 08:00:00 CET point"}},
		{name: "the last Sunday of March is the fifth only sometimes", def: model.TimeModule{Kind: "yearly", At: "08:00", Nth: 5, Weekdays: []string{"sun"}, Month: 3}, after: "2026-01-01 00:00:00 CET",
			want: []string{"2026-03-29 08:00:00 CEST point", "2030-03-31 08:00:00 CEST point"}},
		{name: "a monthly range over the month's end", def: model.TimeModule{Kind: "monthly", At: "22:00", Duration: d("4h"), Day: 31}, after: "2026-01-31 23:00:00 CET",
			want: []string{"2026-02-01 02:00:00 CET end", "2026-03-31 22:00:00 CEST begin"}},
	})
}

// Every n months, the cases of OpenCCU's ReGaHss-Test for OpenCCU#886 (19-programs.js): asked
// on 2020-08-08 12:00 CEST, each module at 08:00 and valid from the day named. The weekday is
// ReGa's bit (4 is Wednesday, 1 is Monday); the old ReGaHss got several of these wrong, which
// is why this is a fix.
func TestEveryNMonths(t *testing.T) {
	l := zone(t, "Europe/Berlin")
	now := at(t, l, "2020-08-08 12:00:00 CEST")
	for _, c := range []struct {
		from    string
		weekday string
		period  int
		every   int
		want    string
	}{
		{"2020-10-01", "wed", 1, 6, "2020-10-07"},
		{"2020-08-01", "wed", 1, 6, "2021-02-03"},
		{"2020-07-01", "wed", 1, 6, "2021-01-06"},
		{"2020-06-01", "wed", 1, 6, "2020-12-02"},
		{"2020-05-01", "wed", 1, 6, "2020-11-04"},
		{"2020-04-01", "wed", 1, 6, "2020-10-07"},
		{"2020-03-01", "wed", 1, 6, "2020-09-02"},
		{"2020-02-01", "wed", 1, 6, "2021-02-03"},
		{"2020-01-01", "wed", 1, 6, "2021-01-06"},
		{"2019-09-01", "wed", 1, 6, "2020-09-02"},
		{"2020-01-01", "mon", 3, 2, "2020-09-21"},
		{"2020-07-15", "", 15, 6, "2021-01-15"},
		{"2020-07-05", "", 5, 6, "2021-01-05"},
		{"2020-03-20", "", 20, 6, "2020-09-20"},
		{"2020-02-15", "", 15, 6, "2020-08-15"},
		{"2020-02-05", "", 5, 6, "2021-02-05"},
		{"2020-01-10", "", 10, 3, "2020-10-10"},
	} {
		def := model.TimeModule{Kind: "monthly", At: "08:00", Every: c.every, From: c.from + " 08:00"}
		if c.weekday == "" {
			def.Day = c.period
		} else {
			def.Nth, def.Weekdays = c.period, []string{c.weekday}
		}
		e, ok := compile(t, def, l, berlin).Next(now)
		if got := e.At.Format("2006-01-02 15:04"); !ok || got != c.want+" 08:00" {
			t.Errorf("from %s, %s %d, every %d months: %s (%v), want %s 08:00", c.from, c.weekday, c.period, c.every, got, ok, c.want)
		}
	}
}

// The validity window: what begins at or after "from" and before "until" - an "until" that is a
// date without a time includes that day, as the CCU's "endet am" does; "count" ends the
// module after that many times, counted on the calendar from "from".
func TestValidity(t *testing.T) {
	runFires(t, []fireCase{
		{name: "from and until", def: model.TimeModule{Kind: "daily", At: "07:00", From: "2026-12-01", Until: "2026-12-04"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-01 07:00:00 CET point", "2026-12-02 07:00:00 CET point", "2026-12-03 07:00:00 CET point", "2026-12-04 07:00:00 CET point"}, last: true},
		{name: "from and until the same day: that day", def: model.TimeModule{Kind: "daily", At: "07:00", From: "2026-12-01", Until: "2026-12-01"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-01 07:00:00 CET point"}, last: true},
		{name: "until with a time: that instant is outside", def: model.TimeModule{Kind: "daily", At: "07:00", From: "2026-12-01 07:00", Until: "2026-12-03 07:00"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-01 07:00:00 CET point", "2026-12-02 07:00:00 CET point"}, last: true},
		{name: "from later that day", def: model.TimeModule{Kind: "daily", At: "07:00", From: "2026-12-01 07:01"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-02 07:00:00 CET point"}},
		{name: "a range that began before until ends all the same", def: model.TimeModule{Kind: "daily", At: "22:00", Duration: d("8h"), Until: "2026-12-01"}, after: "2026-12-01 12:00:00 CET",
			want: []string{"2026-12-01 22:00:00 CET begin", "2026-12-02 06:00:00 CET end"}, last: true},
		{name: "all day until a date: that day is the last, and ends", def: model.TimeModule{Kind: "daily", AllDay: true, Until: "2026-12-02"}, after: "2026-12-01 12:00:00 CET",
			want: []string{"2026-12-02 00:00:00 CET begin", "2026-12-03 00:00:00 CET end"}, last: true},
		{name: "an interval inside its window", def: model.TimeModule{Kind: "interval", AllDay: true, Interval: d("6h"), From: "2026-12-01 05:00", Until: "2026-12-02 00:00"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-01 06:00:00 CET point", "2026-12-01 12:00:00 CET point", "2026-12-01 18:00:00 CET point"}, last: true},
		{name: "an interval with a range: the window cuts the series, the end still fires", def: model.TimeModule{Kind: "interval", At: "08:00", Duration: d("2h"), Interval: d("30min"), Until: "2026-12-01 08:45"}, after: "2026-12-01 07:00:00 CET",
			want: []string{"2026-12-01 08:00:00 CET begin", "2026-12-01 08:30:00 CET repeat", "2026-12-01 10:00:00 CET end"}, last: true},
		{name: "ten times (the fixture's Rollläden morgens)", def: model.TimeModule{Kind: "daily", At: "07:15", From: "2026-11-20", Count: 10}, after: "2026-11-27 12:00:00 CET",
			want: []string{"2026-11-28 07:15:00 CET point", "2026-11-29 07:15:00 CET point"}, last: true},
		{name: "ten times, long over", def: model.TimeModule{Kind: "daily", At: "07:15", From: "2026-11-20", Count: 10}, after: "2027-06-01 12:00:00 CEST", last: true},
		{name: "three times every hour", def: model.TimeModule{Kind: "interval", AllDay: true, Interval: d("1h"), From: "2026-11-20", Count: 3}, after: "2026-11-19 12:00:00 CET",
			want: []string{"2026-11-20 00:00:00 CET point", "2026-11-20 01:00:00 CET point", "2026-11-20 02:00:00 CET point"}, last: true},
		{name: "a range once: begin and end", def: model.TimeModule{Kind: "daily", At: "22:00", Duration: d("8h"), From: "2026-11-20", Count: 1}, after: "2026-11-19 12:00:00 CET",
			want: []string{"2026-11-20 22:00:00 CET begin", "2026-11-21 06:00:00 CET end"}, last: true},
		{name: "an interval with a range counts every time it fires", def: model.TimeModule{Kind: "interval", At: "08:00", Duration: d("2h"), Interval: d("30min"), From: "2026-11-20", Count: 3}, after: "2026-11-19 12:00:00 CET",
			want: []string{"2026-11-20 08:00:00 CET begin", "2026-11-20 08:30:00 CET repeat", "2026-11-20 09:00:00 CET repeat", "2026-11-20 10:00:00 CET end"}, last: true},
		{name: "more times than the pattern has", def: model.TimeModule{Kind: "once", At: "12:00", Date: "2027-01-01", From: "2026-11-20", Count: 5}, after: "2026-11-19 12:00:00 CET",
			want: []string{"2027-01-01 12:00:00 CET point"}, last: true},
		{name: "from in the middle of all-day days", def: model.TimeModule{Kind: "daily", AllDay: true, From: "2026-12-01 12:00"}, after: "2026-11-21 00:00:00 CET",
			want: []string{"2026-12-02 00:00:00 CET begin", "2026-12-03 00:00:00 CET begin"}},
	})
	l := zone(t, "Europe/Berlin")
	m := compile(t, model.TimeModule{Kind: "daily", At: "22:00", Duration: d("8h"), From: "2026-12-01", Until: "2026-12-02"}, l, berlin)
	for when, want := range map[string]bool{
		"2026-12-01 05:00:00 CET": false, // the range of 30 November began before "from"
		"2026-12-01 23:00:00 CET": true,
		"2026-12-03 05:00:00 CET": true, // began on the 2nd, the last day
		"2026-12-03 23:00:00 CET": false,
	} {
		if got := m.Active(at(t, l, when)); got != want {
			t.Errorf("valid range at %s: %v, want %v", when, got, want)
		}
	}
}

// Q-50, Q-34, Q-28: a point is true at its instant only, an all-day interval all day (on the
// CCU from its first firing on - the engine's part); a range
// from its begin (inside) to its end (outside); all day is true all day.
func TestActive(t *testing.T) {
	l := zone(t, "Europe/Berlin")
	for _, c := range []struct {
		name string
		def  model.TimeModule
		when string
		want bool
	}{
		{"a point at its instant", daily("06:30"), "2026-11-21 06:30:00 CET", true},
		{"a point a second later", daily("06:30"), "2026-11-21 06:30:01 CET", false},
		{"a point a second before", daily("06:30"), "2026-11-21 06:29:59 CET", false},
		{"a point on a day outside its pattern", model.TimeModule{Kind: "weekly", At: "06:30", Weekdays: []string{"mon"}}, "2026-11-21 06:30:00 CET", false},
		{"an all-day interval at its instant", every("15min"), "2026-11-21 06:30:00 CET", true},
		{"an all-day interval between its instants: true all day (as the CCU: measured)", every("15min"), "2026-11-21 06:31:00 CET", true},
		{"an all-day interval before its validity", model.TimeModule{Kind: "interval", AllDay: true, Interval: d("15min"), From: "2026-11-22"}, "2026-11-21 06:31:00 CET", false},
		{"an interval with a point", model.TimeModule{Kind: "interval", At: "22:32", Interval: d("5min")}, "2026-11-21 22:37:00 CET", false},
		{"an interval with a range, inside", model.TimeModule{Kind: "interval", At: "08:00", Duration: d("10h"), Interval: d("30min")}, "2026-11-21 12:10:00 CET", true},
		{"an interval with a range, outside", model.TimeModule{Kind: "interval", At: "08:00", Duration: d("10h"), Interval: d("30min")}, "2026-11-21 18:00:00 CET", false},
		{"a range at its begin", span("22:00", "8h"), "2026-11-21 22:00:00 CET", true},
		{"a range after midnight", span("22:00", "8h"), "2026-11-22 05:59:59 CET", true},
		{"a range at its end", span("22:00", "8h"), "2026-11-22 06:00:00 CET", false},
		{"all day", model.TimeModule{Kind: "daily", AllDay: true}, "2026-11-21 13:37:00 CET", true},
		{"all day at the weekend, on Monday", model.TimeModule{Kind: "daily", AllDay: true, Weekdays: []string{"sat", "sun"}}, "2026-11-23 00:00:00 CET", false},
		{"all day at the weekend, on Sunday night", model.TimeModule{Kind: "daily", AllDay: true, Weekdays: []string{"sat", "sun"}}, "2026-11-22 23:59:59 CET", true},
		{"a range inside the missing hour, at the jump", span("02:10", "20min"), "2020-03-29 03:00:00 CEST", true},
		{"a range inside the missing hour, a second after the jump", span("02:10", "20min"), "2020-03-29 03:00:01 CEST", false},
		{"a range in the repeated hour, the first pass", span("02:10", "20min"), "2020-10-25 02:15:00 CEST", true},
		{"a range in the repeated hour, the second pass", span("02:10", "20min"), "2020-10-25 02:15:00 CET", false},
		{"once, on its day", model.TimeModule{Kind: "once", AllDay: true, Date: "2027-01-01"}, "2027-01-01 10:00:00 CET", true},
		{"once, the day after", model.TimeModule{Kind: "once", AllDay: true, Date: "2027-01-01"}, "2027-01-02 00:00:00 CET", false},
		{"a counted module that is over", model.TimeModule{Kind: "daily", At: "22:00", Duration: d("8h"), From: "2026-11-01", Count: 2}, "2026-11-21 23:00:00 CET", false},
		{"a counted module in its last time", model.TimeModule{Kind: "daily", At: "22:00", Duration: d("8h"), From: "2026-11-01", Count: 2}, "2026-11-03 05:00:00 CET", true},
		{"a counted interval after its last time", model.TimeModule{Kind: "interval", AllDay: true, Interval: d("1h"), From: "2026-11-20", Count: 3}, "2026-11-20 03:00:00 CET", false},
		{"half a second after a point is still its second", daily("06:30"), "2026-11-21 06:30:00 CET", true},
		{"every second month, in a month that is left out", model.TimeModule{Kind: "monthly", AllDay: true, Day: 21, Every: 2, From: "2026-10-01"}, "2026-11-21 12:00:00 CET", false},
		{"every second month, in a month that counts", model.TimeModule{Kind: "monthly", AllDay: true, Day: 21, Every: 2, From: "2026-10-01"}, "2026-12-21 12:00:00 CET", true},
		{"a yearly range, in another month", model.TimeModule{Kind: "yearly", AllDay: true, Day: 21, Month: 12}, "2026-11-21 12:00:00 CET", false},
		{"a yearly range, on its day", model.TimeModule{Kind: "yearly", AllDay: true, Day: 21, Month: 12}, "2026-12-21 12:00:00 CET", true},
		{"the n-th weekday, on its day", model.TimeModule{Kind: "monthly", AllDay: true, Nth: 3, Weekdays: []string{"sat"}}, "2026-11-21 12:00:00 CET", true},
		{"the n-th weekday, a week later", model.TimeModule{Kind: "monthly", AllDay: true, Nth: 3, Weekdays: []string{"sat"}}, "2026-11-28 12:00:00 CET", false},
	} {
		m := compile(t, c.def, l, berlin)
		when := at(t, l, c.when)
		if strings.HasPrefix(c.name, "half a second") {
			when = when.Add(500 * time.Millisecond)
		}
		if got := m.Active(when); got != c.want {
			t.Errorf("%s: at %s %v, want %v", c.name, c.when, got, c.want)
		}
		if got, want := m.AllDayInterval(), c.def.Kind == "interval" && c.def.AllDay; got != want {
			t.Errorf("%s: an all-day interval %v", c.name, got)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	for name, def := range map[string]model.TimeModule{
		"fails the model's check": {Kind: "daily"},
		"a date that is none":     {Kind: "once", At: "12:00", Date: "2027-13-01"},
		"a from that is none":     {Kind: "daily", At: "12:00", From: "yesterday"},
		"an until that is none":   {Kind: "daily", At: "12:00", Until: "2027-01-01T00:00"},
		"a clock that is none":    {Kind: "daily", At: "25:00"},
	} {
		if _, err := Compile(&def, nil, berlin); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
	// without a zone the system's own is used
	if m, err := Compile(&model.TimeModule{Kind: "daily", At: "12:00:30"}, nil, Position{}); err != nil || m.loc != time.Local || m.atSec != 12*3600+30 {
		t.Errorf("no zone: %v %+v", err, m)
	}
	if EdgeKind(2).String() != "repeat" {
		t.Error("an edge's name")
	}
}

// What must hold for every module at every moment, tried on a corpus across both clock changes,
// a month's end and a year's end: Next is after the instant asked; asking again anywhere before
// that edge gives the same edge (nothing is skipped, nothing fires twice); and the truth fits
// the edges - true at a begin, a repeat and a point, true the second before an end, false at an
// end unless the module begins again there, false the second after a point unless a range holds.
func TestInvariants(t *testing.T) {
	corpus := []model.TimeModule{
		daily("02:00"), daily("02:30"), daily("00:00"), daily("23:59:59"),
		span("22:00", "8h"), span("01:30", "1h"), span("02:10", "20min"), span("02:30", "1h"), span("06:00", "24h"), span("06:00", "30h"),
		every("1min"), every("7min"), every("2h"), every("25h"), every("45min"),
		{Kind: "interval", At: "02:00", Duration: d("2h"), Interval: d("30min")},
		{Kind: "interval", At: "22:27", Duration: d("4h"), Interval: d("31min")},
		{Kind: "interval", At: "22:32", Interval: d("5min")},
		{Kind: "daily", AllDay: true}, {Kind: "daily", AllDay: true, Weekdays: []string{"sat", "sun"}},
		{Kind: "weekly", At: "20:00", Duration: d("14h"), Weekdays: []string{"sat", "sun"}},
		{Kind: "weekly", At: "07:00", Weekdays: []string{"mon", "thu"}, Every: 2, From: "2020-01-06"},
		{Kind: "monthly", At: "03:00", Day: 31}, {Kind: "monthly", At: "08:00", Nth: 5, Weekdays: []string{"fri"}},
		{Kind: "monthly", At: "22:00", Duration: d("4h"), Day: 31, Every: 2, From: "2020-01-01"},
		{Kind: "yearly", At: "12:00", Day: 29, Month: 2}, {Kind: "yearly", AllDay: true, Day: 1, Month: 1},
		{Kind: "daily", Astro: "day"}, {Kind: "daily", Astro: "night", Offset: d("1h")},
		{Kind: "daily", Astro: "before_sunset", Offset: d("30min"), Duration: d("1h")},
		{Kind: "interval", Astro: "night", Duration: d("3h"), Interval: d("25min")},
		{Kind: "daily", At: "07:15", From: "2020-03-27", Count: 4},
		{Kind: "interval", AllDay: true, Interval: d("6h"), From: "2020-03-28 05:00", Until: "2020-03-30 00:00"},
		{Kind: "once", At: "02:30", Duration: d("1h"), Date: "2020-10-25"},
	}
	type place struct {
		zone string
		pos  Position
	}
	places := []place{{"Europe/Berlin", berlin}, {"America/New_York", Position{40.71, -74.01, true}}, {"Australia/Lord_Howe", Position{-31.55, 159.08, true}},
		{"America/Havana", Position{23.13, -82.38, true}}, {"Europe/Oslo", Position{69.65, 18.96, true}}}
	starts := []string{"2020-03-28 20:00:00", "2020-10-24 20:00:00", "2020-02-28 20:00:00", "2020-12-31 12:00:00", "2020-03-07 20:00:00", "2020-10-31 20:00:00", "2020-04-04 12:00:00", "2020-10-03 12:00:00", "2020-05-17 00:00:00", "2020-07-24 00:00:00"}
	checked := 0
	for _, pl := range places {
		l := zone(t, pl.zone)
		for _, def := range corpus {
			m := compile(t, def, l, pl.pos)
			for _, s := range starts {
				from, err := time.ParseInLocation("2006-01-02 15:04:05", s, l)
				if err != nil {
					t.Fatal(err)
				}
				// asked every 17 minutes and 13 seconds for 36 hours
				for at := from; at.Before(from.Add(36 * time.Hour)); at = at.Add(17*time.Minute + 13*time.Second) {
					e, ok := m.Next(at)
					if !ok {
						continue
					}
					checked++
					fail := func(format string, args ...any) {
						t.Helper()
						t.Fatalf("%s %+v, asked at %s, next %s %v: %s", pl.zone, def, at.Format(stamp), e.At.Format(stamp), e.Kind, fmt.Sprintf(format, args...))
					}
					if !e.At.After(at) {
						fail("not after the instant asked")
					}
					if e.At.Nanosecond() != 0 {
						fail("not on a whole second")
					}
					for _, between := range []time.Time{at.Add(e.At.Sub(at) / 2), e.At.Add(-time.Second), e.At.Add(-time.Nanosecond)} {
						if between.Before(at) {
							continue
						}
						if again, ok := m.Next(between); !ok || again != e {
							fail("asked again at %s: %s %v %v", between.Format(stamp), again.At.Format(stamp), again.Kind, ok)
						}
					}
					if follows, more := m.Next(e.At); more && follows.At.Equal(e.At) {
						fail("two edges at one instant")
					}
					switch e.Kind {
					case Begin, Repeat, Point:
						if !m.Active(e.At) {
							fail("not true at its instant")
						}
					case End:
						if !m.Active(e.At.Add(-time.Second)) {
							fail("not true the second before its end")
						}
						// (true at its own end only where its ranges overlap: 30 hours a day)
						if m.Active(e.At) && def.Duration <= d("24h") {
							fail("true at its end")
						}
					}
				}
			}
		}
	}
	if checked < 50000 {
		t.Errorf("only %d instants were checked", checked)
	}
}
