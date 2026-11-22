package harness

import "strings"

// The quirk catalogue's entries on triggers, branches, delays and values, one scenario each
// (the time modules are in scenarios_time.go, what needs a restart in scenarios_restart.go).
// They run at the reference's own speed and take well under a minute each.

func listVar(name string, value int, entries ...string) Var {
	return Var{Name: name, Type: "list", Entries: entries, Value: value}
}

// when: a rule with one block of lines (joined by AND) and its DANN.
func when(m string, lines ...Line) Rule {
	return Rule{Groups: [][]Line{lines}, Acts: mark(m)}
}

// otherwise: the final SONST.
func otherwise(m string) Rule { return Rule{Acts: mark(m)} }

// num: a line that compares a number.
func num(name string, cmp int, v float64, mode int) Line {
	return Line{Var: name, Cmp: cmp, V1: v, Mode: mode}
}

// later: an activity that notes the block after a delay.
func later(m string, seconds int) Act { return Act{Mark: m, Delay: seconds} }

// watcher: a program that notes every write of a boolean variable, with the value written.
func watcher(variable string) Program {
	m := strings.ToLower(variable)
	return Program{Name: "watch-" + variable, Rules: []Rule{
		when(m+"-true", is(variable, Update)),
		otherwise(m + "-false"),
	}}
}

func quirkScenarios() []*Scenario {
	return []*Scenario{
		{
			Name: "q01-any-trigger-evaluates-the-chain", Quirk: "Q-1, Q-5", Mark: Reproduce, Start: day,
			Title: `a trigger evaluates the whole chain from the top against the current values: the block that runs need not hold the line that triggered, and a "nur prüfen" line never triggers`,
			Vars:  []Var{boolVar("A", false), boolVar("B", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("then", is("A", Change)),
				when("elseif", is("B", Check)),
				otherwise("else"),
			}}},
			Steps: []Step{
				set(2, "B", true),  // nur prüfen: nothing
				set(4, "A", false), // no flip: nothing
				set(6, "A", true),  // DANN
				set(8, "A", false), // the flip back: SONST WENN, whose line did not trigger
				set(10, "B", false),
				set(12, "A", true),
				set(14, "A", false), // SONST
			},
			Until: sec(18),
		},
		{
			Name: "q03-update-runs-on-every-write", Quirk: "Q-3, Q-4", Mark: Reproduce, Start: day,
			Title: `"bei Aktualisierung auslösen" runs the program on every write, the same value included; with the condition false that is the SONST every time`,
			Vars:  []Var{numVar("N", 5)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("then", num("N", Greater, 10, Update)),
				otherwise("else"),
			}}},
			Steps: []Step{set(2, "N", 5.0), set(4, "N", 5.0), set(6, "N", 12.0), set(8, "N", 12.0), set(10, "N", 5.0)},
			Until: sec(14),
		},
		{
			Name: "q05-check-only-never-runs", Quirk: "Q-5", Mark: Reproduce, Start: day,
			Title: `a program whose lines are all "nur prüfen" never runs by itself`,
			Vars:  []Var{boolVar("A", false), boolVar("B", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("then", is("A", Check), is("B", Check)),
				otherwise("else"),
			}}},
			Steps: []Step{set(2, "A", true), set(4, "B", true), set(6, "A", false), {At: sec(8), Run: "P"}},
			Until: sec(12),
		},
		{
			Name: "q07-two-lines-on-one-variable", Quirk: "Q-7", Mark: Reproduce, Start: day,
			Title: `two trigger lines on the same variable give one run per write, not two`,
			Vars:  []Var{numVar("N", 5)},
			Programs: []Program{
				{Name: "change", Rules: []Rule{
					{Groups: [][]Line{{num("N", Greater, 10, Change)}, {num("N", Greater, 20, Change)}}, Acts: mark("change-then")},
					otherwise("change-else"),
				}},
				{Name: "update", Rules: []Rule{
					{Groups: [][]Line{{num("N", Greater, 10, Update)}, {num("N", Greater, 20, Update)}}, Acts: mark("update-then")},
					otherwise("update-else"),
				}},
			},
			Steps: []Step{set(2, "N", 25.0), set(5, "N", 15.0), set(8, "N", 5.0)},
			Until: sec(12),
		},
		{
			Name: "q08-a-manual-start-checks-nothing", Quirk: "Q-8, Q-9", Mark: Reproduce, Start: day,
			Title: `a start from outside (ProgramExecute) runs the first DANN without looking at the conditions, also of an inactive program`,
			Vars:  []Var{boolVar("A", false)},
			Programs: []Program{
				{Name: "active", Rules: []Rule{when("active-then", is("A", Change)), when("active-elseif", is("A", Check)), otherwise("active-else")}},
				{Name: "inactive", Inactive: true, Rules: []Rule{when("inactive-then", is("A", Change)), otherwise("inactive-else")}},
			},
			Steps: []Step{{At: sec(2), Run: "active"}, {At: sec(5), Run: "inactive"}},
			Until: sec(9),
		},
		{
			Name: "q11-activation", Quirk: "Q-10, Q-11", Mark: Fix, Start: day,
			Title: `switching an inactive program on evaluates it once on the CCU (DANN when true, else SONST); switching an active one on does nothing`,
			Note:  `fixed for all programs (the maintainer's decision on OQ-8): switching a program on or saving it never runs anything; "Jetzt prüfen" is a button`,
			Vars:  []Var{boolVar("A", true)},
			Programs: []Program{
				{Name: "true", Inactive: true, Rules: []Rule{when("true-then", is("A", Change)), otherwise("true-else")}},
				{Name: "false", Inactive: true, Rules: []Rule{when("false-then", Line{Var: "A", V1: false, Mode: Change}), otherwise("false-else")}},
			},
			Steps: []Step{
				{At: sec(2), Activate: "true"}, {At: sec(4), Activate: "false"},
				{At: sec(6), Activate: "true"}, // already on
				{At: sec(8), Deactivate: "true"}, {At: sec(10), Activate: "true"},
			},
			Engine: []string{},
			Until:  sec(14),
		},
		{
			Name: "q15-a-delay-counts-from-the-trigger", Quirk: "Q-15", Mark: Reproduce, Start: day,
			Title: `"verzögert um" counts from the moment the program was triggered, not from the activity above`,
			Vars:  []Var{boolVar("A", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{is("A", Change)}}, Acts: []Act{{Mark: "now"}, later("after-4", 4), later("after-8", 8)}},
			}}},
			Steps: []Step{set(2, "A", true)},
			Until: sec(14),
		},
		{
			Name: "q16-two-triggers-two-delayed-runs", Quirk: "Q-16", Mark: Reproduce, Start: day,
			Title: `without "laufende Verzögerungen beenden" a pending delayed activity stays when the program is triggered again: two triggers, two executions`,
			Vars:  []Var{numVar("N", 5)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{num("N", Greater, 10, Update)}}, Acts: []Act{later("late", 6)}},
			}}},
			Steps: []Step{set(2, "N", 12.0), set(5, "N", 12.0)},
			Until: sec(15),
		},
		{
			Name: "q17-cancel-within-a-program", Quirk: "Q-17", Mark: Reproduce, Start: day,
			Title: `"laufende Verzögerungen beenden" on the SONST cancels the DANN's pending write of the same variable`,
			Vars:  []Var{boolVar("A", false), boolVar("X", false)},
			Programs: []Program{
				watcher("X"),
				{Name: "P", Rules: []Rule{
					{Groups: [][]Line{{is("A", Change)}}, Cancel: true, Acts: []Act{{Var: "X", Value: true, Delay: 6}}},
					{Cancel: true, Acts: []Act{{Var: "X", Value: false}}},
				}},
			},
			Steps: []Step{set(2, "A", true), set(5, "A", false), set(12, "A", true)},
			Until: sec(22),
		},
		{
			Name: "q17-cancel-across-programs", Quirk: "Q-17", Mark: Reproduce, Start: day,
			Title: `the box cancels the pending writes to the variables its block names, whichever program scheduled them; a block that names another variable cancels nothing`,
			Vars:  []Var{boolVar("A", false), boolVar("B", false), boolVar("C", false), boolVar("X", false), boolVar("Y", false)},
			Programs: []Program{
				watcher("X"),
				{Name: "schedules", Rules: []Rule{{Groups: [][]Line{{is("A", Change)}}, Acts: []Act{{Var: "X", Value: true, Delay: 6}}}}},
				{Name: "cancels", Rules: []Rule{{Groups: [][]Line{{is("B", Change)}}, Cancel: true, Acts: []Act{{Var: "X", Value: false}}}}},
				{Name: "other-target", Rules: []Rule{{Groups: [][]Line{{is("C", Change)}}, Cancel: true, Acts: []Act{{Var: "Y", Value: false}}}}},
			},
			Steps: []Step{
				set(2, "A", true), set(5, "B", true), // X: false at 5, nothing at 8
				set(10, "A", false), set(12, "A", true), set(15, "C", true), // X: true at 18
			},
			Until: sec(24),
		},
		{
			Name: "q18-a-delayed-activity-is-not-checked-again", Quirk: "Q-18", Mark: Reproduce, Start: day,
			Title: `a delayed activity runs when it is due although the condition has become false`,
			Vars:  []Var{boolVar("A", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{is("A", Change)}}, Acts: []Act{later("late", 6)}},
			}}},
			Steps: []Step{set(2, "A", true), set(4, "A", false)},
			Until: sec(13),
		},
		{
			Name: "q19-update-restarts-the-delay", Quirk: "Q-19", Mark: Reproduce, Start: day,
			Title: `with the box on and an "bei Aktualisierung" trigger every write starts the delay anew: it runs once, after the last write`,
			Vars:  []Var{numVar("N", 5)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{num("N", Greater, 10, Update)}}, Cancel: true, Acts: []Act{later("late", 6)}},
			}}},
			Steps: []Step{set(2, "N", 12.0), set(5, "N", 12.0), set(8, "N", 12.0)},
			Until: sec(19),
		},
		{
			Name: "q20-switching-off-keeps-the-delay", Quirk: "Q-20", Mark: Reproduce, Start: day,
			Title: `switching a program off does not cancel its pending delayed activity`,
			Vars:  []Var{boolVar("A", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{is("A", Change)}}, Acts: []Act{later("late", 7)}},
			}}},
			Steps: []Step{set(2, "A", true), {At: sec(4), Deactivate: "P"}},
			Until: sec(14),
		},
		{
			Name: "q37-the-same-value-is-no-change", Quirk: "Q-37", Mark: Reproduce, Start: day,
			Title: `writing a value list: only a flip of the line's truth runs the program, the same value or another non-matching one does not`,
			Vars:  []Var{listVar("L", 0, "a", "b", "c")},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("then", Line{Var: "L", V1: 1, Mode: Change}),
				otherwise("else"),
			}}},
			Steps: []Step{set(2, "L", 1), set(4, "L", 1), set(6, "L", 2), set(8, "L", 2), set(10, "L", 0), set(12, "L", 1)},
			Until: sec(16),
		},
		{
			Name: "q38-a-text-under-change", Quirk: "Q-38", Mark: Reproduce, Start: day,
			Title: `a text variable with "bei Änderung" runs the program only when it starts or stops matching the line's text`,
			Vars:  []Var{{Name: "S", Type: "string", Value: ""}},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("then", Line{Var: "S", Cmp: Equal, V1: "x", Mode: Change}),
				otherwise("else"),
			}}},
			Steps: []Step{set(2, "S", "x"), set(4, "S", "x"), set(6, "S", "y"), set(8, "S", "z"), set(10, "S", "x")},
			Until: sec(14),
		},
		{
			Name: "q39-numbers-outside-the-limits", Quirk: "Q-39", Mark: Reproduce, Start: day,
			Title: `a number outside the variable's limits is stored as written and triggers programs, which see it`,
			Vars:  []Var{numVar("N", 50)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("above", num("N", Greater, 120, Change)),
				when("below", num("N", Less, 0, Check)),
				otherwise("inside"),
			}}},
			Steps: []Step{set(2, "N", 150.0), set(5, "N", -5.0), set(8, "N", 100.5), set(11, "N", 200.0), set(14, "N", 50.0)},
			Until: sec(18),
		},
		{
			Name: "q40-an-alarm-variable-in-programs", Quirk: "Q-40", Mark: Reproduce, Start: day,
			Title: `an alarm variable is a boolean to programs: triggered again while it is unacknowledged only "bei Aktualisierung" lines fire; writing false is the acknowledge`,
			Vars:  []Var{{Name: "AL", Type: "alarm", Value: false}},
			Programs: []Program{
				{Name: "change", Rules: []Rule{when("change-then", is("AL", Change)), otherwise("change-else")}},
				{Name: "update", Rules: []Rule{when("update-then", is("AL", Update)), otherwise("update-else")}},
			},
			Steps: []Step{set(2, "AL", true), set(5, "AL", true), set(8, "AL", false), set(11, "AL", false), set(14, "AL", true)},
			Until: sec(18),
		},
		{
			Name: "q44-a-key-line", Quirk: "Q-44", Mark: Reproduce, Start: day,
			Title: `a key line (on a stand-in: a boolean of the subtype "action", with the trigger mode the editor leaves on it) fires on every press, and reads as pressed when another trigger evaluates the program`,
			Vars:  []Var{{Name: "K", Type: "action", Value: false}, boolVar("A", false)},
			Programs: []Program{
				{Name: "key", Rules: []Rule{when("key-then", Line{Var: "K", V1: true}), otherwise("key-else")}},
				{Name: "both", Rules: []Rule{when("both-then", is("A", Change), Line{Var: "K", V1: true}), otherwise("both-else")}},
			},
			Steps: []Step{
				set(2, "A", true),                    // before any press
				set(5, "K", true), set(8, "K", true), // two presses
				set(11, "A", false), set(14, "A", true), // after the presses
			},
			Until: sec(18),
		},
		{
			Name: "q48-an-unreached-change-line", Quirk: "Q-48", Mark: Reproduce, Start: day,
			Title: `a "bei Änderung" line in a SONST WENN the chain did not reach fires on every write of its variable, until the block above it is false again`,
			Vars:  []Var{boolVar("A", false), numVar("N", 5)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				when("then", is("A", Change)),
				when("elseif", num("N", Greater, 10, Change)),
				otherwise("else"),
			}}},
			Steps: []Step{
				set(2, "A", true),                                     // DANN; from now the line below is not reached
				set(4, "N", 5.0), set(6, "N", 5.0), set(8, "N", 12.0), // each write runs DANN again
				set(10, "A", false), // SONST WENN (N is 12)
				set(12, "N", 12.0),  // reached again: no flip, nothing
				set(14, "N", 5.0),   // the flip: SONST
			},
			Until: sec(18),
		},
		{
			Name: "q49-a-delay-of-a-day-or-more", Quirk: "Q-49", Mark: Reproduce, Start: day,
			Title: `a delay of 24 hours or more is stored as none by the CCU: the activity runs at once, and the import reads it so`,
			Vars:  []Var{boolVar("A", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{is("A", Change)}}, Acts: []Act{later("day-and-an-hour", 90000), later("five-seconds", 5)}},
			}}},
			Steps: []Step{set(2, "A", true)},
			Until: sec(12),
		},
		{
			Name: "q51-ranges-of-numbers", Quirk: "B-7", Mark: Reproduce, Start: day,
			Title: `"im Wertebereich": from inclusive, to exclusive; a range whose first value is above its second is never true; and the comparison "outside the range" the editor cannot make`,
			Vars:  []Var{numVar("N", 0)},
			Programs: []Program{
				{Name: "in", Rules: []Rule{when("in-then", Line{Var: "N", Cmp: InRange, V1: 20.0, V2: 50.0, Mode: Update}), otherwise("in-else")}},
				{Name: "turned", Rules: []Rule{when("turned-then", Line{Var: "N", Cmp: InRange, V1: 50.0, V2: 20.0, Mode: Update}), otherwise("turned-else")}},
				{Name: "out", Rules: []Rule{when("out-then", Line{Var: "N", Cmp: OutOfRange, V1: 20.0, V2: 50.0, Mode: Update}), otherwise("out-else")}},
			},
			Steps: []Step{set(2, "N", 10.0), set(4, "N", 20.0), set(6, "N", 30.0), set(8, "N", 50.0), set(10, "N", 60.0)},
			Until: sec(14),
		},
	}
}
