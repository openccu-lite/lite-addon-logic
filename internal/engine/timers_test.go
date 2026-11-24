package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
)

const stamp = "2006-01-02 15:04:05 MST"

// trig is an engine in Europe/Berlin (Berlin's position) with a fake clock, for the time
// modules: every run of a program is told as "<time> <trigger> <block>".
type trig struct {
	*prig
	loc *time.Location
}

func berlinTime(t *testing.T, s string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	v, err := time.ParseInLocation("2006-01-02 15:04:05", s[:19], loc)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []time.Duration{0, time.Hour, -time.Hour} {
		if v.Add(d).Format(stamp) == s {
			return v.Add(d)
		}
	}
	t.Fatalf("%s does not exist", s)
	return v
}

func newTrig(t *testing.T, start string, change ...func(*Options)) *trig {
	t.Helper()
	at := berlinTime(t, start)
	r := &trig{prig: &prig{rig: &rig{t: t, clock: NewFakeClock(at)}, w: &fakeWriter{}, progs: map[string]*model.Program{}}, loc: at.Location()}
	o := Options{Clock: r.clock, Writer: r.w, Location: r.loc, Position: schedule.Position{Lat: 52.52, Lon: 13.405, Known: true},
		Notify: func(n Notification) {
			if n.Kind != "pending" {
				r.notes = append(r.notes, n)
			}
		}, Log: func(format string, args ...any) { r.logs = append(r.logs, fmt.Sprintf(format, args...)) }}
	for _, c := range change {
		c(&o)
	}
	r.e = New(o)
	d := defs()
	d["x"] = &model.Variable{Key: "x", Name: "X", Type: model.VarNumber}
	d["out"] = &model.Variable{Key: "out", Name: "Out", Type: model.VarString}
	r.e.SetVariables(d)
	return r
}

// timed makes a program of time module lines (YAML of the module, indented by the helper),
// OR-ed, with a THEN and - unless bare - an ELSE.
func timed(sonst bool, lines ...string) string {
	var b strings.Builder
	b.WriteString("rules:\n  - when:\n      any:\n")
	for _, l := range lines {
		mode := "update"
		if strings.HasPrefix(l, "check ") {
			mode, l = "check", strings.TrimPrefix(l, "check ")
		}
		b.WriteString("        - all:\n            - time:\n")
		for _, f := range strings.Split(l, ", ") {
			b.WriteString("                " + f + "\n")
		}
		b.WriteString("              trigger: " + mode + "\n")
	}
	b.WriteString("    then: []\n")
	if sonst {
		b.WriteString("  - else: []\n")
	}
	return b.String()
}

// ran answers the runs of a program since the last call, one per line.
func (r *trig) ran(key string) string {
	var out []string
	for _, run := range r.e.Runs(key) {
		if run.Untriggered {
			continue
		}
		block := fmt.Sprint(run.Block)
		switch {
		case run.Delayed:
			block = "delayed"
		case run.None && len(run.Notes()) > 0:
			block = "- " + strings.SplitN(run.Notes()[0], ":", 2)[0]
		case run.None:
			block = "-"
		}
		out = append(out, run.Time.In(r.loc).Format(stamp)+" "+run.Trigger+" "+block)
	}
	r.e.mu.Lock()
	r.e.forget(key)
	r.e.mu.Unlock()
	return strings.Join(out, "\n")
}

func (r *trig) want(key string, lines ...string) {
	r.t.Helper()
	if got, want := r.ran(key), strings.Join(lines, "\n"); got != want {
		r.t.Fatalf("%s ran:\n%s\nwant:\n%s", key, got, want)
	}
}

// to moves the clock to a time, running what comes due on the way.
func (r *trig) to(s string) {
	r.t.Helper()
	target := berlinTime(r.t, s)
	if d := target.Sub(r.clock.Now()); d >= 0 {
		r.clock.Advance(d)
	} else {
		r.t.Fatalf("%s is before the clock's %s", s, r.clock.Now().Format(stamp))
	}
}

// A daily point runs THEN at its instant, every day, and never ELSE.
func TestDailyPoint(t *testing.T) {
	r := newTrig(t, "2026-11-21 06:00:00 CET")
	r.add("p", timed(true, `kind: daily, at: "06:30"`))
	in, _ := r.e.Program("p")
	if in.NextTime == nil || in.NextTime.Format(stamp) != "2026-11-21 06:30:00 CET" || in.NoClock || in.NoPosition {
		t.Errorf("before: %+v", in)
	}
	r.to("2026-11-21 06:29:59 CET")
	r.want("p")
	r.to("2026-11-23 06:29:59 CET")
	in, _ = r.e.Program("p")
	r.want("p", "2026-11-21 06:30:00 CET time:1:point 1", "2026-11-22 06:30:00 CET time:1:point 1")
	if in.NextTime.Format(stamp) != "2026-11-23 06:30:00 CET" || in.LastRun == nil || in.LastRun.Trigger != "time:1:point" {
		t.Errorf("after: %+v", in)
	}
	tl := r.e.TimeLines("p", 2)
	if len(tl) != 1 || tl[0].Line != 1 || !tl[0].Triggers || tl[0].Active || len(tl[0].Next) != 2 || tl[0].Next[1].At.Format(stamp) != "2026-11-24 06:30:00 CET" ||
		tl[0].Next[0].Kind != "point" || tl[0].Last == nil || tl[0].Last.At.Format(stamp) != "2026-11-22 06:30:00 CET" || tl[0].Last.Kind != "point" {
		t.Errorf("time lines: %+v", tl)
	}
	if r.e.TimeLines("nothing", 2) != nil {
		t.Error("the time lines of a program that is not there")
	}
}

// Q-28, as measured: a range runs THEN at its begin and ELSE at its end.
func TestRangeRunsElseAtItsEnd(t *testing.T) {
	r := newTrig(t, "2026-11-21 22:00:00 CET")
	r.add("p", timed(true, `kind: daily, at: "22:10", duration: 2min`))
	r.to("2026-11-21 23:00:00 CET")
	r.want("p", "2026-11-21 22:10:00 CET time:1:begin 1", "2026-11-21 22:12:00 CET time:1:end 0")
	// without an ELSE the end evaluates the program and nothing runs
	r.add("q", timed(false, `kind: daily, at: "23:10", duration: 2min`))
	r.to("2026-11-21 23:30:00 CET")
	r.want("q", "2026-11-21 23:10:00 CET time:1:begin 1", "2026-11-21 23:12:00 CET time:1:end -")
}

// Appendix B-5, as measured: an interval never runs ELSE between its fires; with a range it
// runs THEN at the begin and at every interval, and ELSE once at the end.
func TestIntervals(t *testing.T) {
	r := newTrig(t, "2026-11-21 23:29:00 CET")
	r.add("all", timed(true, `kind: interval, all_day: true, interval: 7min`))
	r.add("rng", timed(true, `kind: interval, at: "22:27", duration: 1h15min, interval: 5min`))
	r.to("2026-11-22 00:10:00 CET")
	r.want("all", "2026-11-21 23:34:00 CET time:1:point 1", "2026-11-21 23:41:00 CET time:1:point 1", "2026-11-21 23:48:00 CET time:1:point 1",
		"2026-11-21 23:55:00 CET time:1:point 1", "2026-11-22 00:00:00 CET time:1:point 1", "2026-11-22 00:07:00 CET time:1:point 1")
	r.want("rng", "2026-11-21 23:32:00 CET time:1:repeat 1", "2026-11-21 23:37:00 CET time:1:repeat 1", "2026-11-21 23:42:00 CET time:1:end 0")
	// every second: ten runs in ten seconds are not a loop
	r.add("sec", timed(false, `kind: interval, all_day: true, interval: 1s`))
	r.clock.Advance(30 * time.Second)
	if got := strings.Count(r.ran("sec"), "\n") + 1; got != 30 {
		t.Errorf("every second ran %d times in 30 s", got)
	}
	if in, _ := r.e.Program("sec"); !in.PausedUntil.IsZero() {
		t.Error("every second was taken for a loop")
	}
}

// Q-7 (fixed): two time modules of one program that fire at the same instant are one run, and
// the run names both. The CCU ran the program twice.
func TestTwoModulesAtOneInstant(t *testing.T) {
	r := newTrig(t, "2026-11-21 06:00:00 CET")
	r.add("p", timed(true, `kind: daily, at: "06:30"`, `kind: daily, at: "06:30"`, `kind: daily, at: "06:45"`))
	r.add("a", timed(false, `kind: daily, at: "06:30"`))
	r.to("2026-11-21 07:00:00 CET")
	r.want("p", "2026-11-21 06:30:00 CET time:1:point,2:point 1", "2026-11-21 06:45:00 CET time:3:point 1")
	r.want("a", "2026-11-21 06:30:00 CET time:1:point 1")
	// at one instant the programs run in the order of their keys
	if len(r.notes) < 2 || r.notes[0].Key != "a" || r.notes[1].Key != "p" {
		t.Errorf("order: %+v", r.notes)
	}
}

// Q-33: several time modules each fire their begin and end; with an ELSE the ends that leave
// nothing true run it, and an end while another range holds runs THEN again.
func TestSeveralRanges(t *testing.T) {
	r := newTrig(t, "2026-11-21 05:00:00 CET")
	r.add("p", timed(true, `kind: daily, at: "06:00", duration: 2h`, `kind: daily, at: "07:00", duration: 2h`, `kind: daily, at: "09:00", duration: 1h`))
	r.to("2026-11-21 11:00:00 CET")
	r.want("p",
		"2026-11-21 06:00:00 CET time:1:begin 1",
		"2026-11-21 07:00:00 CET time:2:begin 1",
		"2026-11-21 08:00:00 CET time:1:end 1",         // the second range still holds
		"2026-11-21 09:00:00 CET time:2:end,3:begin 1", // one ends, one begins: one run
		"2026-11-21 10:00:00 CET time:3:end 0")
}

// Q-50: a time module is true inside its range or at its instant only. Looked at by another
// trigger a point is false - the program runs ELSE - and true in its own second; an interval
// that runs all day is true from its first firing on. And a time
// module on a "nur prüfen" line still evaluates its program at each of its instants, as on
// the CCU (measured on the real ReGaHss, task 14: a point, a range's begin and end, every
// instant of an interval): for a time module the mode changes nothing.
func TestTimeLinesCheckedByOtherTriggers(t *testing.T) {
	r := newTrig(t, "2026-11-21 21:00:00 CET")
	gate := func(module string) string {
		return "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: ge\n              value: 0\n              trigger: update\n            - time:\n" +
			"                " + strings.ReplaceAll(module, ", ", "\n                ") + "\n              trigger: check\n    then: []\n  - else: []\n"
	}
	r.add("range", gate(`kind: daily, at: "22:00", duration: 8h`))
	r.add("point", gate(`kind: daily, at: "23:00"`))
	r.add("iv", gate(`kind: interval, all_day: true, interval: 3h`))
	r.set("x", 1, FromUser) // 21:00: before the range
	r.to("2026-11-21 22:59:59 CET")
	r.set("x", 1, FromUser) // inside the range, a second before the point
	r.to("2026-11-21 23:00:00 CET")
	r.clock.Advance(400 * time.Millisecond)
	r.set("x", 1, FromUser) // in the point's own second
	r.to("2026-11-21 23:00:01 CET")
	r.set("x", 1, FromUser)
	r.to("2026-11-22 01:00:00 CET")
	r.set("x", 1, FromUser) // the interval has fired by now (at midnight)
	r.to("2026-11-22 06:00:00 CET")
	r.set("x", 1, FromUser) // the range's end is outside
	// the module evaluates its program at its own instants too, whatever the line's mode says
	r.want("range", "2026-11-21 21:00:00 CET variable:x 0", "2026-11-21 22:00:00 CET time:2:begin 1", "2026-11-21 22:59:59 CET variable:x 1",
		"2026-11-21 23:00:00 CET variable:x 1", "2026-11-21 23:00:01 CET variable:x 1", "2026-11-22 01:00:00 CET variable:x 1", "2026-11-22 06:00:00 CET time:2:end 0",
		"2026-11-22 06:00:00 CET variable:x 0")
	r.want("point", "2026-11-21 21:00:00 CET variable:x 0", "2026-11-21 22:59:59 CET variable:x 0", "2026-11-21 23:00:00 CET time:2:point 1",
		"2026-11-21 23:00:00 CET variable:x 1", "2026-11-21 23:00:01 CET variable:x 0", "2026-11-22 01:00:00 CET variable:x 0", "2026-11-22 06:00:00 CET variable:x 0")
	// an interval that runs all day is true all day - once it has fired for the first time since
	// the start (measured on the CCU: false in an evaluation before that, true in every later one)
	r.want("iv", "2026-11-21 21:00:00 CET variable:x 0", "2026-11-21 22:59:59 CET variable:x 0", "2026-11-21 23:00:00 CET variable:x 0",
		"2026-11-21 23:00:01 CET variable:x 0", "2026-11-22 00:00:00 CET time:2:point 1", "2026-11-22 01:00:00 CET variable:x 1", "2026-11-22 03:00:00 CET time:2:point 1",
		"2026-11-22 06:00:00 CET time:2:point 1", "2026-11-22 06:00:00 CET variable:x 1")
	tl := r.e.TimeLines("range", 1)
	if len(tl) != 1 || tl[0].Line != 2 || !tl[0].Triggers || tl[0].Last == nil || tl[0].Last.Kind != "end" || len(tl[0].Next) != 1 || tl[0].Next[0].Kind != "begin" {
		t.Errorf("a checked line: %+v", tl)
	}
	if in, _ := r.e.Program("range"); in.NextTime == nil || in.NextTime.Format("15:04") != "22:00" {
		t.Errorf("a checked time line has its next time: %+v", in)
	}
}

// A manual run with conditions sees the time modules as they are at that moment; a program run
// by a program that a point triggered sees the point's instant.
func TestManualAndChainedRuns(t *testing.T) {
	r := newTrig(t, "2026-11-21 21:00:00 CET")
	r.add("night", timed(true, `check kind: daily, at: "22:00", duration: 8h`))
	r.add("gate", timed(true, `check kind: daily, at: "23:00"`))
	r.add("caller", "rules:\n  - when:\n      all:\n        - all:\n            - time:\n                kind: daily\n                at: \"23:00\"\n              trigger: update\n    then:\n      - run: gate\n        mode: check\n      - run: night\n        mode: check\n")
	if err := r.e.RunProgramBy("night", model.RunCheck, "user"); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(90 * time.Minute)
	if err := r.e.RunProgramBy("night", model.RunCheck, "user"); err != nil {
		t.Fatal(err)
	}
	r.to("2026-11-21 23:30:00 CET")
	r.want("night", "2026-11-21 21:00:00 CET user 0", "2026-11-21 22:00:00 CET time:1:begin 1", "2026-11-21 22:30:00 CET user 1", "2026-11-21 23:00:00 CET program:caller 1")
	r.want("gate", "2026-11-21 23:00:00 CET time:1:point 1", "2026-11-21 23:00:00 CET program:caller 1")
}

// The spring change in the engine: 02:00 does not exist and runs at 03:00 (ReGaHss-Test 07c);
// the range that began the evening before ends at 06:00 on the clock; and a delay is a duration
// - two hours stay two hours (OpenCCU#584: the CCU showed and stored one hour that day).
func TestSpringChangeInTheEngine(t *testing.T) {
	r := newTrig(t, "2020-03-28 21:00:00 CET")
	r.add("two", timed(false, `kind: daily, at: "02:00"`))
	r.add("night", timed(true, `kind: daily, at: "22:00", duration: 8h`))
	r.add("minute", timed(false, `kind: interval, at: "01:58", duration: 1h4min, interval: 1min`))
	r.add("delay", "rules:\n  - when:\n      all:\n        - all:\n            - time:\n                kind: daily\n                at: \"01:30\"\n              trigger: update\n    then:\n      - variable: out\n        to: spaet\n        after: 2h\n")
	r.to("2020-03-29 07:00:00 CEST")
	r.want("two", "2020-03-29 03:00:00 CEST time:1:point 1")
	r.want("night", "2020-03-28 22:00:00 CET time:1:begin 1", "2020-03-29 06:00:00 CEST time:1:end 0")
	r.want("minute", "2020-03-29 01:58:00 CET time:1:begin 1", "2020-03-29 01:59:00 CET time:1:repeat 1", "2020-03-29 03:00:00 CEST time:1:repeat 1", "2020-03-29 03:01:00 CEST time:1:repeat 1", "2020-03-29 03:02:00 CEST time:1:end -")
	r.want("delay", "2020-03-29 01:30:00 CET time:1:point 1", "2020-03-29 04:30:00 CEST time:1:point delayed")
}

// The autumn change in the engine: 02:30 happens twice and runs the first time only
// (ReGaHss-Test 07d), an interval runs through both hours, a delay stays a duration.
func TestAutumnChangeInTheEngine(t *testing.T) {
	r := newTrig(t, "2020-10-24 21:00:00 CEST")
	r.add("half", timed(false, `kind: daily, at: "02:30"`))
	r.add("night", timed(true, `kind: daily, at: "22:00", duration: 8h`))
	r.add("iv", timed(false, `kind: interval, at: "02:00", duration: 1h1min, interval: 30min`))
	r.add("delay", "rules:\n  - when:\n      all:\n        - all:\n            - time:\n                kind: daily\n                at: \"01:30\"\n              trigger: update\n    then:\n      - variable: out\n        to: spaet\n        after: 2h\n")
	r.to("2020-10-25 07:00:00 CET")
	r.want("half", "2020-10-25 02:30:00 CEST time:1:point 1")
	r.want("night", "2020-10-24 22:00:00 CEST time:1:begin 1", "2020-10-25 06:00:00 CET time:1:end 0")
	r.want("iv", "2020-10-25 02:00:00 CEST time:1:begin 1", "2020-10-25 02:30:00 CEST time:1:repeat 1", "2020-10-25 02:00:00 CET time:1:repeat 1", "2020-10-25 02:30:00 CET time:1:repeat 1",
		"2020-10-25 03:00:00 CET time:1:repeat 1", "2020-10-25 03:01:00 CET time:1:end -")
	r.want("delay", "2020-10-25 01:30:00 CEST time:1:point 1", "2020-10-25 02:30:00 CET time:1:point delayed")
}

// A program that is not active does not run - but its time module knows its next time all the
// same (OpenCCU#889: the CCU's Next() of a module that was never armed was empty), and it runs
// from the moment the program is switched on, without making anything up.
func TestInactiveProgram(t *testing.T) {
	r := newTrig(t, "2026-11-21 06:00:00 CET")
	r.add("p", "active: false\n"+timed(true, `kind: daily, at: "06:30"`))
	if n := r.clock.Pending(); n != 0 {
		t.Errorf("%d timers for an inactive program", n)
	}
	r.to("2026-11-22 12:00:00 CET")
	r.want("p")
	tl := r.e.TimeLines("p", 1)
	if len(tl) != 1 || len(tl[0].Next) != 1 || tl[0].Next[0].At.Format(stamp) != "2026-11-23 06:30:00 CET" {
		t.Errorf("an inactive program's next time: %+v", tl)
	}
	if in, _ := r.e.Program("p"); in.NextTime != nil {
		t.Errorf("an inactive program is not triggered next: %+v", in)
	}
	on := r.progs["p"].Clone()
	on.Active = true
	r.progs["p"] = on
	r.e.SetPrograms(r.progs)
	r.want("p") // switching on runs nothing
	r.to("2026-11-23 07:00:00 CET")
	r.want("p", "2026-11-23 06:30:00 CET time:1:point 1")
	// and gone with its program
	delete(r.progs, "p")
	r.e.SetPrograms(r.progs)
	if n := r.clock.Pending(); n != 0 {
		t.Errorf("%d timers without a program", n)
	}
}

// A restart in the middle of a range: nothing runs, nothing is made up, the range is true for
// whoever asks, and its end runs ELSE.
func TestRestartInARange(t *testing.T) {
	r := newTrig(t, "2026-11-21 23:00:00 CET") // the engine starts at 23:00
	r.add("night", timed(true, `kind: daily, at: "22:00", duration: 8h`))
	r.add("morning", timed(true, `kind: daily, at: "06:30"`))
	r.add("gate", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: ge\n              value: 0\n              trigger: update\n            - time:\n                kind: daily\n                at: \"22:00\"\n                duration: 8h\n              trigger: check\n    then: []\n  - else: []\n")
	r.want("night")
	if tl := r.e.TimeLines("night", 1); !tl[0].Active || tl[0].Next[0].Kind != "end" {
		t.Errorf("in the range: %+v", tl)
	}
	r.set("x", 1, FromUser)
	r.want("gate", "2026-11-21 23:00:00 CET variable:x 1")
	r.to("2026-11-22 06:15:00 CET")
	r.want("night", "2026-11-22 06:00:00 CET time:1:end 0")
	// a second start, later: yesterday's 06:30 is not made up
	r2 := newTrig(t, "2026-11-22 06:31:00 CET")
	r2.add("morning", timed(true, `kind: daily, at: "06:30"`))
	r2.to("2026-11-22 12:00:00 CET")
	r2.want("morning")
}

// The clock is set forward (NTP) over a time that was due. A small step - within the tolerance
// - runs it late, once, for its own instant; a larger one does not make it up and says so; a
// range that the step landed in is true without having run.
func TestClockSetForward(t *testing.T) {
	r := newTrig(t, "2026-11-21 06:29:50 CET")
	r.add("p", timed(true, `kind: daily, at: "06:30"`))
	r.clock.Jump(30 * time.Second) // 06:30:20
	r.to("2026-11-21 06:31:00 CET")
	r.want("p", "2026-11-21 06:30:00 CET time:1:point 1")

	// two times in one step: each runs for its own instant, in their order, and a delay counts
	// from the instant that was due, not from the moment the clock was looked at
	r = newTrig(t, "2026-11-21 06:29:50 CET")
	r.add("b", timed(false, `kind: daily, at: "06:30"`))
	r.add("a", "rules:\n  - when:\n      all:\n        - all:\n            - time:\n                kind: daily\n                at: \"06:30:30\"\n              trigger: update\n    then:\n      - variable: out\n        to: x\n        after: 1min\n")
	r.clock.Jump(time.Minute) // 06:30:50
	r.to("2026-11-21 06:32:00 CET")
	r.want("b", "2026-11-21 06:30:00 CET time:1:point 1")
	r.want("a", "2026-11-21 06:30:30 CET time:1:point 1", "2026-11-21 06:31:30 CET time:1:point delayed")
	if len(r.notes) < 2 || r.notes[0].Key != "b" || r.notes[1].Key != "a" {
		t.Errorf("order: %+v", r.notes)
	}

	r = newTrig(t, "2026-11-21 06:00:00 CET")
	r.add("p", timed(true, `kind: daily, at: "06:30"`))
	r.add("rng", timed(true, `kind: daily, at: "07:00", duration: 4h`))
	r.add("other", timed(true, `kind: daily, at: "12:00"`))
	r.clock.Jump(3 * time.Hour) // 09:00: 06:30 and the range's begin are over
	r.to("2026-11-21 09:01:00 CET")
	// (the scheduler looks at the clock every twenty seconds: that is when it notices)
	r.want("p", "2026-11-21 09:00:20 CET time - missed")
	r.want("rng", "2026-11-21 09:00:20 CET time - missed")
	r.want("other")
	if !strings.Contains(strings.Join(r.logs, "\n"), "not made up") {
		t.Errorf("the log: %v", r.logs)
	}
	if tl := r.e.TimeLines("rng", 1); !tl[0].Active {
		t.Error("the range the clock landed in is true")
	}
	r.to("2026-11-22 06:45:00 CET")
	r.want("p", "2026-11-22 06:30:00 CET time:1:point 1")
	r.want("rng", "2026-11-21 11:00:00 CET time:1:end 0")
	r.want("other", "2026-11-21 12:00:00 CET time:1:point 1")
}

// The clock is set back. A small step never runs anything twice; a larger one means the time
// was wrong, and what is due on the corrected clock runs - again, if it ran on the wrong one.
func TestClockSetBack(t *testing.T) {
	r := newTrig(t, "2026-11-21 06:29:00 CET")
	r.add("p", timed(true, `kind: daily, at: "06:30"`))
	r.to("2026-11-21 06:30:30 CET")
	r.want("p", "2026-11-21 06:30:00 CET time:1:point 1")
	r.clock.Jump(-time.Minute) // 06:29:30: 06:30 comes a second time
	if in, _ := r.e.Program("p"); in.NextTime == nil || in.NextTime.Format(stamp) != "2026-11-22 06:30:00 CET" {
		t.Errorf("after the step back the next time is tomorrow's: %+v", in)
	}
	r.to("2026-11-21 06:35:00 CET")
	r.want("p")
	r.to("2026-11-22 06:35:00 CET")
	r.want("p", "2026-11-22 06:30:00 CET time:1:point 1")

	r.clock.Jump(-time.Hour) // 05:35 on the 22nd: the clock was wrong by an hour
	r.to("2026-11-22 06:35:00 CET")
	r.want("p", "2026-11-22 06:30:00 CET time:1:point 1")
	if !strings.Contains(strings.Join(r.logs, "\n"), "set back by 1h0m0s") {
		t.Errorf("the log: %v", r.logs)
	}
}

// The clock is not set when the system starts (no battery clock, no network yet): no time
// module fires and none is true until it is - and then nothing is made up.
func TestClockNotSetAtStart(t *testing.T) {
	ready := false
	r := newTrig(t, "2026-01-01 00:00:00 CET", func(o *Options) { o.ClockReady = func() bool { return ready } })
	r.add("p", timed(true, `kind: interval, all_day: true, interval: 1min`))
	r.add("gate", "rules:\n  - when:\n      all:\n        - all:\n            - variable: x\n              op: ge\n              value: 0\n              trigger: update\n            - time:\n                kind: daily\n                all_day: true\n              trigger: check\n    then: []\n  - else: []\n")
	r.clock.Advance(3 * time.Minute)
	r.set("x", 1, FromUser)
	r.want("p")
	r.want("gate", "2026-01-01 00:03:00 CET variable:x 0") // all day is not true on a clock nobody set
	if in, _ := r.e.Program("p"); !in.NoClock || in.NextTime != nil {
		t.Errorf("no clock: %+v", in)
	}
	if tl := r.e.TimeLines("gate", 1); tl[0].Active {
		t.Error("true without a clock")
	}
	// the network comes up, the clock jumps to the real time
	r.clock.Jump(berlinTime(t, "2026-11-21 06:29:30 CET").Sub(r.clock.Now()))
	ready = true
	r.to("2026-11-21 06:31:10 CET")
	r.want("p", "2026-11-21 06:30:00 CET time:1:point 1", "2026-11-21 06:31:00 CET time:1:point 1")
	r.set("x", 1, FromUser)
	r.want("gate", "2026-11-21 06:31:10 CET variable:x 1")
	if in, _ := r.e.Program("p"); in.NoClock || in.NextTime == nil {
		t.Errorf("with a clock: %+v", in)
	}

	// a system that never gets the time runs on the clock it has, after the wait
	r = newTrig(t, "2026-01-01 00:00:00 CET", func(o *Options) { o.ClockReady = func() bool { return false }; o.ClockWait = 5 * time.Minute })
	r.add("p", timed(true, `kind: interval, all_day: true, interval: 1min`))
	r.to("2026-01-01 00:07:30 CET")
	r.want("p", "2026-01-01 00:06:00 CET time:1:point 1", "2026-01-01 00:07:00 CET time:1:point 1")
	if !strings.Contains(strings.Join(r.logs, "\n"), "not synchronised after 5m0s") {
		t.Errorf("the log: %v", r.logs)
	}
	// and one whose unset clock runs backwards while it waits does not wait for ever
	r = newTrig(t, "2026-01-01 00:10:00 CET", func(o *Options) { o.ClockReady = func() bool { return false } })
	r.add("p", timed(true, `kind: interval, all_day: true, interval: 1min`))
	r.clock.Jump(-5 * time.Minute)
	r.to("2026-01-01 00:07:30 CET")
	r.want("p", "2026-01-01 00:06:00 CET time:1:point 1", "2026-01-01 00:07:00 CET time:1:point 1")
}

// Astro modules follow the position: sunrise and sunset in Berlin on 2026-06-15 are 04:43 and
// 21:31 CEST; without a position the module never fires and the program says why.
func TestAstroInTheEngine(t *testing.T) {
	r := newTrig(t, "2026-06-15 00:00:00 CEST", func(o *Options) { o.Position = schedule.Position{} })
	r.add("day", timed(true, `kind: daily, astro: day`))
	if in, _ := r.e.Program("day"); !in.NoPosition || in.NextTime != nil {
		t.Errorf("without a position: %+v", in)
	}
	r.to("2026-06-15 12:00:00 CEST")
	r.want("day")
	if r.e.Position().Known {
		t.Error("the position")
	}
	r.e.SetPosition(schedule.Position{Lat: 52.52, Lon: 13.405, Known: true})
	if in, _ := r.e.Program("day"); in.NoPosition || in.NextTime == nil || !r.e.Position().Known {
		t.Errorf("with a position: %+v", in)
	}
	r.to("2026-06-16 12:00:00 CEST")
	// the day that was under way when the position came ends at sunset; the next one begins
	got := strings.Split(r.ran("day"), "\n")
	if len(got) != 2 || !strings.HasPrefix(got[0], "2026-06-15 21:31:") || !strings.HasSuffix(got[0], " CEST time:1:end 0") ||
		!strings.HasPrefix(got[1], "2026-06-16 04:42:") || !strings.HasSuffix(got[1], " CEST time:1:begin 1") {
		t.Errorf("day ran:\n%s", strings.Join(got, "\n"))
	}
}

// A time module the calendar cannot make (30 February) is a problem of its program: it does
// not run, and nothing else is disturbed.
func TestTimeModuleProblem(t *testing.T) {
	r := newTrig(t, "2026-11-21 06:00:00 CET")
	p, err := model.DecodeProgram("bad.yaml", "bad", []byte("name: bad\nactive: true\n"+timed(false, `kind: once, at: "12:00", date: "2027-02-31"`)))
	if err != nil {
		t.Fatal(err)
	}
	r.progs["bad"] = p
	r.add("good", timed(false, `kind: daily, at: "06:30"`))
	in, _ := r.e.Program("bad")
	if len(in.Problems) != 1 || !strings.Contains(in.Problems[0], "time module: date") {
		t.Errorf("problems: %v", in.Problems)
	}
	if r.e.RunProgramBy("bad", model.RunCheck, "user") == nil {
		t.Error("a program with a problem ran")
	}
	if len(r.e.TimeLines("bad", 1)) != 0 {
		t.Error("time lines of a module that could not be made")
	}
	r.to("2026-11-21 07:00:00 CET")
	r.want("good", "2026-11-21 06:30:00 CET time:1:point 1")
}

// The validity window in the engine: "ten times" ends, and the program is told no next time.
func TestCountEnds(t *testing.T) {
	r := newTrig(t, "2026-11-20 00:00:00 CET")
	r.add("p", timed(false, `kind: daily, at: "07:15", from: "2026-11-20", count: 3`))
	r.to("2026-12-01 00:00:00 CET")
	r.want("p", "2026-11-20 07:15:00 CET time:1:point 1", "2026-11-21 07:15:00 CET time:1:point 1", "2026-11-22 07:15:00 CET time:1:point 1")
	if in, _ := r.e.Program("p"); in.NextTime != nil {
		t.Errorf("after the last time: %+v", in)
	}
}
