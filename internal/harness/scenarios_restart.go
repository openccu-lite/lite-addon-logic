package harness

// What happens at a restart of the logic. The reference is stopped with its database saved
// and started again with its clock where it was; the engine side stops with its state and
// starts as after a boot of the system.

func restartScenarios() []*Scenario {
	return []*Scenario{
		{
			Name: "r12-the-start-evaluates-every-program", Quirk: "Q-12, Q-14, Q-50", Mark: Reproduce, Needs: "restart", Start: day,
			Title: `at a start every active program is evaluated once, with its conditions: the first true block runs, else the SONST - also with an empty condition, with "nur prüfen" lines only, with a time module only; and the evaluation sets what a "bei Änderung" line compares the next write with`,
			Vars:  []Var{boolVar("A", true), boolVar("B", false)},
			Programs: []Program{
				{Name: "empty", Rules: []Rule{otherwise("empty-then")}},
				{Name: "check", Rules: []Rule{when("check-then", is("A", Check)), otherwise("check-else")}},
				{Name: "change", Rules: []Rule{when("change-then", is("A", Change)), otherwise("change-else")}},
				{Name: "update", Rules: []Rule{when("update-then", is("A", Update)), otherwise("update-else")}},
				{Name: "false-alone", Rules: []Rule{when("false-alone-then", is("B", Change))}},
				{Name: "false", Rules: []Rule{when("false-then", is("B", Change)), otherwise("false-else")}},
				{Name: "off", Inactive: true, Rules: []Rule{when("off-then", is("A", Change)), otherwise("off-else")}},
				timed("timer", &TimeModule{Type: Daily, Time: "2007-01-01 03:00:00"}),
			},
			Steps: []Step{
				{At: sec(10), Restart: true},
				set(40, "A", true),  // unchanged: "bei Änderung" does not run, "bei Aktualisierung" does
				set(45, "B", false), // unchanged
				set(50, "B", true),  // the flip
			},
			Until: sec(58),
		},
		{
			Name: "r21-a-delay-across-a-restart", Quirk: "Q-21", Mark: Fix, Needs: "restart", Start: day,
			Title:  `a delayed activity that is pending when the logic is restarted is lost on the CCU`,
			Note:   `fixed for all programs: pending delays are kept across a restart and run when they are due`,
			Engine: []string{"2026-11-10 12:00:48 late"},
			Vars:   []Var{boolVar("A", false)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{is("A", Change)}}, Acts: []Act{later("late", 45)}},
			}}},
			// the condition is false again before the restart, so the start's own evaluation
			// (which would run the DANN and start a new delay) does nothing
			Steps: []Step{set(3, "A", true), set(6, "A", false), {At: sec(10), Restart: true}},
			Until: sec(70),
		},
	}
}
