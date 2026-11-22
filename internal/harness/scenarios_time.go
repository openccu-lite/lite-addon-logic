package harness

import "time"

// Time modules. Their clocks run fast on the reference, so their times are compared by the
// minute, not the second.

func clock(h, m int) time.Duration {
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute
}

// timed: a program with one triggering time module, a DANN and a SONST.
func timed(name string, tm *TimeModule) Program {
	return Program{Name: name, Rules: []Rule{
		{Groups: [][]Line{{{Time: tm}}}, Acts: mark(name + "-then")},
		{Acts: mark(name + "-else")},
	}}
}

// checked: a variable "bei Aktualisierung" AND a time module "nur prüfen".
func checked(name, variable string, tm *TimeModule) Program {
	return Program{Name: name, Rules: []Rule{
		{Groups: [][]Line{{is(variable, Update), {Time: tm, Mode: Check}}}, Acts: mark(name + "-then")},
		{Acts: mark(name + "-else")},
	}}
}

func timeScenarios() []*Scenario {
	return []*Scenario{
		{
			Name: "t01-all-day-daily", Quirk: "Q-34, Q-50", Mark: Reproduce, Needs: "scheduler", Rate: 60,
			Title: `an all-day daily module ("ganztägig", "täglich"): when it triggers, and what it is when another trigger checks it`,
			Start: "2026-11-10 23:30:00",
			Vars:  []Var{boolVar("V", true)},
			Programs: []Program{
				timed("a", &TimeModule{Type: Daily}),
				checked("b", "V", &TimeModule{Type: Daily}),
			},
			// written before the first midnight, a minute after it, in the night and at noon
			Steps: []Step{{At: clock(0, 10), Set: "V", Value: true}, {At: clock(0, 31), Set: "V", Value: true}, {At: clock(3, 30), Set: "V", Value: true}, {At: clock(12, 30), Set: "V", Value: true}},
			Until: clock(12, 45),
		},
		{
			Name: "t02-all-day-weekend", Quirk: "Q-34, Q-50", Mark: Reproduce, Needs: "scheduler", Rate: 60,
			Title: `an all-day daily module "am Wochenende" from Friday night to Monday: the runs at each midnight`,
			Start: "2026-11-13 23:30:00",
			Vars:  []Var{boolVar("V", true)},
			Programs: []Program{
				timed("a", &TimeModule{Type: Daily, Wd: Weekend}),
				checked("b", "V", &TimeModule{Type: Daily, Wd: Weekend}),
			},
			// Friday 23:45, Saturday noon, Sunday noon, Monday 00:30
			Steps: []Step{{At: clock(0, 15), Set: "V", Value: true}, {At: clock(12, 30), Set: "V", Value: true}, {At: clock(36, 30), Set: "V", Value: true}, {At: clock(49, 0), Set: "V", Value: true}},
			Until: clock(49, 30),
		},
		{
			Name: "t03-validity", Quirk: "Q-34", Mark: Reproduce, Needs: "scheduler", Rate: 60,
			Title: `the validity of a daily module at 12:00: an end date today and tomorrow, a begin today, "endet nach 2 Terminen" with a begin two days ago`,
			Start: "2026-11-10 11:40:00",
			Programs: []Program{
				timed("end-today", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Begin: "2026-11-09", End: "2026-11-10"}),
				timed("end-tomorrow", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Begin: "2026-11-09", End: "2026-11-11"}),
				timed("begin-today", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Begin: "2026-11-10"}),
				timed("begin-tomorrow", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Begin: "2026-11-11"}),
				timed("twice", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Begin: "2026-11-08", Calcount: 2}),
				timed("twice-from-today", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Begin: "2026-11-10", Calcount: 2}),
			},
			Until: clock(73, 0),
		},
		{
			Name: "t04-range-dst-spring", Quirk: "Q-28, Q-31", Mark: Fix, Needs: "scheduler", Rate: 60,
			Note:   `the CCU has the times of the hour the clock skips an hour early (02:00 at 01:00 CET, 02:30 at 01:30, the range 02:10-02:30 at 01:10 and 01:30 CET); here a time in the gap happens at the moment the clock jumps, 03:00:00 CEST, and a range wholly inside it begins then and ends a second later (correct scheduling across the change, task 1)`,
			Engine: []string{"2026-03-28 22:00:00 night-then", "2026-03-29 03:00:00 at-half-past-two-then", "2026-03-29 03:00:00 at-two-then", "2026-03-29 03:00:00 skipped-then", "2026-03-29 03:00:01 skipped-else", "2026-03-29 06:00:00 night-else"},
			Title:  `ranges in the night the clock goes forward (2026-03-29): 22:00 for eight hours, and 02:10 for twenty minutes inside the skipped hour`,
			Start:  "2026-03-28 21:45:00",
			Programs: []Program{
				timed("night", &TimeModule{Type: Daily, Time: "2007-01-01 22:00:00", Caldur: 28800}),
				timed("skipped", &TimeModule{Type: Daily, Time: "2007-01-01 02:10:00", Caldur: 1200}),
				timed("at-two", &TimeModule{Type: Daily, Time: "2007-01-01 02:00:00"}),
				timed("at-half-past-two", &TimeModule{Type: Daily, Time: "2007-01-01 02:30:00"}),
			},
			Until: clock(8, 30), // 07:15 CEST
		},
		{
			Name: "t05-range-dst-autumn", Quirk: "Q-28, Q-31", Mark: Reproduce, Needs: "scheduler", Rate: 60,
			Title: `ranges in the night the clock goes back (2026-10-25): 22:00 for eight hours, and 02:10 for twenty minutes inside the repeated hour`,
			Start: "2026-10-24 21:45:00",
			Programs: []Program{
				timed("night", &TimeModule{Type: Daily, Time: "2007-01-01 22:00:00", Caldur: 28800}),
				timed("repeated", &TimeModule{Type: Daily, Time: "2007-01-01 02:10:00", Caldur: 1200}),
				timed("at-half-past-two", &TimeModule{Type: Daily, Time: "2007-01-01 02:30:00"}),
			},
			Until: clock(10, 30), // 07:15 CET
		},
		{
			Name: "t06-interval-dst-spring", Quirk: "Q-30, Q-31", Mark: Fix, Needs: "scheduler", Rate: 20,
			Note:   `the interval goes on by elapsed time across the change as on the CCU (03:06, 03:13 CEST); not reproduced are two fires the CCU adds on this night - one at the jump itself, 03:00:00 CEST, and one at 01:42:00, seven minutes after the program was made`,
			Engine: []string{"2026-03-29 01:45:00 seven-then", "2026-03-29 01:52:00 seven-then", "2026-03-29 01:59:00 seven-then", "2026-03-29 03:06:00 seven-then", "2026-03-29 03:13:00 seven-then", "2026-03-29 03:20:00 seven-then"},
			Title:  `an all-day interval of seven minutes across the skipped hour (2026-03-29)`,
			Start:  "2026-03-29 01:40:00",
			Programs: []Program{
				timed("seven", &TimeModule{Type: Periodic, Period: 420}),
			},
			Until: clock(0, 45), // 03:25 CEST
		},
		{
			Name: "t07-interval-dst-autumn", Quirk: "Q-30, Q-31", Mark: Reproduce, Needs: "scheduler", Rate: 20,
			Title: `an all-day interval of seven minutes across the repeated hour (2026-10-25)`,
			Start: "2026-10-25 01:40:00",
			Programs: []Program{
				timed("seven", &TimeModule{Type: Periodic, Period: 420}),
			},
			Until: clock(2, 35), // 03:15 CET
		},
		{
			Name: "t08-interval-phase", Quirk: "Q-30", Mark: Reproduce, Needs: "scheduler", Rate: 20,
			Title: `an all-day interval of seven minutes counts from midnight, whenever the program was made`,
			Start: "2026-11-10 12:10:00",
			Programs: []Program{
				timed("seven", &TimeModule{Type: Periodic, Period: 420}),
			},
			Until: clock(0, 50),
		},
		{
			Name: "t09-interval-minute", Quirk: "Q-30", Mark: Reproduce, Needs: "scheduler",
			Title: `an all-day interval of one minute fires on the minute`,
			Start: day,
			Programs: []Program{
				timed("minute", &TimeModule{Type: Periodic, Period: 60}),
			},
			Until: sec(200),
		},
		{
			Name: "t10-check-only-modules", Quirk: "Q-5, Q-34, Q-50", Mark: Reproduce, Needs: "scheduler", Rate: 30,
			Title: `time modules as "nur prüfen" beside a variable "bei Aktualisierung": a point, a range, an all-day interval, an all-day daily module - whether they trigger, and what they are when the variable is written`,
			Start: "2026-11-10 11:50:00",
			Vars:  []Var{boolVar("V", true)},
			Programs: []Program{
				checked("point", "V", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00"}),
				checked("range", "V", &TimeModule{Type: Daily, Time: "2007-01-01 12:00:00", Caldur: 1800}),
				checked("interval", "V", &TimeModule{Type: Periodic, Period: 600}),
				checked("allday", "V", &TimeModule{Type: Daily}),
			},
			// written before the point, at it (a second after), inside the range and after it
			Steps: []Step{{At: clock(0, 5), Set: "V", Value: true}, {At: clock(0, 10) + time.Second, Set: "V", Value: true}, {At: clock(0, 25), Set: "V", Value: true}, {At: clock(0, 50), Set: "V", Value: true}},
			Until: clock(1, 0),
		},
		{
			Name: "t11-interval-over-midnight", Quirk: "Q-30", Mark: Reproduce, Needs: "scheduler", Rate: 20,
			Title: `an interval of seven minutes inside a range that crosses midnight (23:40 for forty minutes): whether the series starts again at 00:00`,
			Start: "2026-11-10 23:30:00",
			Programs: []Program{
				timed("seven", &TimeModule{Type: Periodic, Time: "2007-01-01 23:40:00", Caldur: 2400, Period: 420}),
			},
			Until: clock(1, 5),
		},
		{
			Name: "t12-month-without-the-day", Quirk: "Q-34", Mark: Reproduce, Needs: "scheduler", Rate: 60,
			Title: `monthly modules at 12:00 at the end of April 2026: the 30th, the 31st the month does not have, the fifth Friday it does not have, the first Friday`,
			Start: "2026-04-30 11:50:00",
			Programs: []Program{
				timed("day-30", &TimeModule{Type: Monthly, Time: "2007-01-01 12:00:00", Period: 30, Calrep: 1, Begin: "2026-01-01"}),
				timed("day-31", &TimeModule{Type: Monthly, Time: "2007-01-01 12:00:00", Period: 31, Calrep: 1, Begin: "2026-01-01"}),
				timed("fifth-friday", &TimeModule{Type: Monthly, Time: "2007-01-01 12:00:00", Wd: Fri, Period: 5, Calrep: 1, Begin: "2026-01-01"}),
				timed("first-friday", &TimeModule{Type: Monthly, Time: "2007-01-01 12:00:00", Wd: Fri, Period: 1, Calrep: 1, Begin: "2026-01-01"}),
			},
			Until: clock(24, 30), // 1 May 12:20
		},
		{
			Name: "t13-interval-with-a-point-and-a-range", Quirk: "Q-30", Mark: Reproduce, Needs: "scheduler", Rate: 20,
			Title: `an interval pattern with a time point fires once, at the point; with a range it fires from the range's begin every interval, and the range's end runs the SONST`,
			Start: day,
			Programs: []Program{
				timed("at-point", &TimeModule{Type: Periodic, Time: "2007-01-01 12:05:00", Period: 300}),
				timed("in-range", &TimeModule{Type: Periodic, Time: "2007-01-01 12:03:00", Caldur: 900, Period: 240}),
			},
			Until: clock(0, 25),
		},
		{
			Name: "t14-weekend-range-over-midnight", Quirk: "Q-29", Mark: Reproduce, Needs: "scheduler", Rate: 60,
			Title: `"am Wochenende" from 20:00 for fourteen hours: the weekday is that of the day the range begins on - Sunday's range ends on Monday at 10:00, and none begins on Monday`,
			Start: "2026-11-15 19:50:00",
			Programs: []Program{
				timed("weekend", &TimeModule{Type: Daily, Wd: Weekend, Time: "2007-01-01 20:00:00", Caldur: 50400}),
			},
			Until: clock(24, 30), // Monday 20:20
		},
		{
			Name: "t15-two-modules-in-one-program", Quirk: "Q-33", Mark: Reproduce, Needs: "scheduler", Rate: 20,
			Title: `two triggering time modules in one program: each fires for itself - a point, and a range with its begin and its end`,
			Start: day,
			Programs: []Program{{Name: "both", Rules: []Rule{
				{Groups: [][]Line{
					{{Time: &TimeModule{Type: Daily, Time: "2007-01-01 12:03:00"}}},
					{{Time: &TimeModule{Type: Daily, Time: "2007-01-01 12:05:00", Caldur: 120}}},
				}, Acts: mark("both-then")},
				{Acts: mark("both-else")},
			}}},
			Until: clock(0, 12),
		},
		{
			Name: "t16-two-identical-modules", Quirk: "Q-7", Mark: Fix, Needs: "scheduler", Rate: 20,
			Title:  `two identical time modules in one program run it twice in the same second on the CCU, once per module`,
			Note:   `fixed for all programs: the triggers of one program at one instant are one evaluation, so the block runs once`,
			Engine: []string{"2026-11-10 12:03:00 twins-then"},
			Start:  day,
			Programs: []Program{{Name: "twins", Rules: []Rule{
				{Groups: [][]Line{
					{{Time: &TimeModule{Type: Daily, Time: "2007-01-01 12:03:00"}}},
					{{Time: &TimeModule{Type: Daily, Time: "2007-01-01 12:03:00"}}},
				}, Acts: mark("twins-then")},
				{Acts: mark("twins-else")},
			}}},
			Until: clock(0, 8),
		},
		{
			Name: "t17-astro", Quirk: "Q-32", Mark: Reproduce, Needs: "scheduler", Rate: 60, Slack: 12 * time.Minute,
			Title: `astro modules at 48.78 N, 9.18 E from a morning to the next: from sunrise to sunset and from sunset to sunrise, with offsets before and after, with a duration, and what they are when a variable is written - the kinds and their order are the CCU's; its sunrise and sunset are up to ten minutes off the real ones, which the engine uses, hence the slack`,
			Start: "2026-10-10 05:30:00", Position: &Position{Lat: 48.78, Lon: 9.18},
			Vars: []Var{boolVar("V", true)},
			Programs: []Program{
				timed("rise", &TimeModule{Type: Daily, Sot: Sunrise}),
				timed("set", &TimeModule{Type: Daily, Sot: Sunset}),
				timed("day", &TimeModule{Type: Daily, Sot: AfterSunrise}),
				timed("night", &TimeModule{Type: Daily, Sot: AfterSunset}),
				timed("day-early", &TimeModule{Type: Daily, Sot: BeforeSunrise, Offset: 3600}),
				timed("night-late", &TimeModule{Type: Daily, Sot: AfterSunset, Offset: 5400}),
				timed("morning", &TimeModule{Type: Daily, Sot: AfterSunrise, Offset: 1800, Caldur: 7200}),
				timed("evening", &TimeModule{Type: Daily, Sot: BeforeSunset, Offset: 2400, Caldur: 3600}),
				checked("by-day", "V", &TimeModule{Type: Daily, Sot: AfterSunrise}),
			},
			// written before sunrise, at noon, in the evening
			Steps: []Step{{At: clock(0, 30), Set: "V", Value: true}, {At: clock(6, 30), Set: "V", Value: true}, {At: clock(17, 0), Set: "V", Value: true}},
			Until: clock(27, 0), // 08:30 the next morning
		},
	}
}
