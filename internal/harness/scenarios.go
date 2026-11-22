package harness

import "time"

// Scenarios answers every scenario, in the order of the quirk catalogue.
func Scenarios() []*Scenario {
	var out []*Scenario
	for _, list := range [][]*Scenario{triggerScenarios(), quirkScenarios(), restartScenarios(), timeScenarios()} {
		out = append(out, list...)
	}
	return out
}

// Shorthands the scenario tables use.
const day = "2026-11-10 12:00:00"

func sec(n int) time.Duration { return time.Duration(n) * time.Second }

func boolVar(name string, v bool) Var { return Var{Name: name, Type: "bool", Value: v} }
func numVar(name string, v float64) Var {
	return Var{Name: name, Type: "number", Min: 0, Max: 100, Value: v}
}

// is: a boolean line "variable is true" with a trigger mode.
func is(name string, mode int) Line { return Line{Var: name, V1: true, Mode: mode} }

// mark: an activity that notes the block.
func mark(m string) []Act { return []Act{{Mark: m}} }

func set(at int, name string, v any) Step { return Step{At: sec(at), Set: name, Value: v} }

func triggerScenarios() []*Scenario {
	return []*Scenario{
		{
			Name: "q02-change-flips-both-ways", Quirk: "Q-2, Q-4", Mark: Reproduce, Start: day,
			Title: `"bei Änderung auslösen" runs the program when the line's truth flips, in both directions, not when the value changes`,
			Vars:  []Var{numVar("N", 6)},
			Programs: []Program{{Name: "P", Rules: []Rule{
				{Groups: [][]Line{{{Var: "N", Cmp: Greater, V1: 10.0, Mode: Change}}}, Acts: mark("then")},
				{Acts: mark("else")},
			}}},
			Steps: []Step{set(2, "N", 12.0), set(5, "N", 15.0), set(8, "N", 8.0), set(11, "N", 8.0), set(14, "N", 9.0)},
			Until: sec(18),
		},
	}
}
