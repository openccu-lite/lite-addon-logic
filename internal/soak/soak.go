// Package soak makes the programs and variables of the long-running test on hardware
// (scripts/soak): a tree as large as the largest installations known - 300 programs and 800
// system variables by default - in which every kind of line, trigger and activity occurs many
// times, and which a stream of writes to its input variables keeps busy for a week without ever
// running in a circle. Every file is written the canonical way, as a save from the page would
// write it.
package soak

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Options say how large the tree is and which real channels it touches.
type Options struct {
	// Programs and Variables are the numbers of each (300 and 800 when zero). There are never
	// fewer variables than the inputs and outputs the programs need.
	Programs, Variables int
	// Read is a datapoint of a real device some programs trigger on
	// ("Interface/ADDRESS:CHANNEL/PARAMETER") with the value they compare it with; empty: none.
	Read      string
	ReadValue string
	// Write is a datapoint some programs write, with its value - a virtual key's PRESS_SHORT,
	// so that nothing goes out on the radio; empty: none.
	Write      string
	WriteValue string
}

// Tree is the generated tree: file contents by key, and the keys the load writes.
type Tree struct {
	Programs  map[string][]byte
	Variables map[string][]byte
	// Inputs are the variables the load writes, with their type ("bool", "number", "list",
	// "string"), sorted by key.
	Inputs []Input
}

// Input is one variable the load writes.
type Input struct {
	Key  string
	Type string
}

// Entries are the entries of every generated value list.
var Entries = []string{"aus", "halb", "voll"}

// The inputs: the load writes these, programs only read them.
const (
	boolInputs   = 60
	numberInputs = 60
	listInputs   = 20
	textInputs   = 10
)

// templates is the number of program shapes New cycles through.
const templates = 12

// New makes the tree.
func New(o Options) (*Tree, error) {
	if o.Programs <= 0 {
		o.Programs = 300
	}
	if o.Variables <= 0 {
		o.Variables = 800
	}
	t := &Tree{Programs: map[string][]byte{}, Variables: map[string][]byte{}}
	vars := map[string]string{}
	add := func(key, yaml string) { vars[key] = yaml }
	for i := 0; i < boolInputs; i++ {
		k := fmt.Sprintf("soak-in-b-%03d", i)
		add(k, fmt.Sprintf("name: Soak Eingang B %03d\ntype: bool\nlabels:\n  \"false\": zu\n  \"true\": offen\n", i))
		t.Inputs = append(t.Inputs, Input{k, "bool"})
	}
	for i := 0; i < numberInputs; i++ {
		k := fmt.Sprintf("soak-in-n-%03d", i)
		add(k, fmt.Sprintf("name: Soak Eingang N %03d\ntype: number\nunit: \"%%\"\nmin: 0\nmax: 100\n", i))
		t.Inputs = append(t.Inputs, Input{k, "number"})
	}
	for i := 0; i < listInputs; i++ {
		k := fmt.Sprintf("soak-in-l-%03d", i)
		add(k, fmt.Sprintf("name: Soak Eingang L %03d\ntype: list\nentries:\n  - aus\n  - halb\n  - voll\n", i))
		t.Inputs = append(t.Inputs, Input{k, "list"})
	}
	for i := 0; i < textInputs; i++ {
		k := fmt.Sprintf("soak-in-s-%03d", i)
		add(k, fmt.Sprintf("name: Soak Eingang S %03d\ntype: string\n", i))
		t.Inputs = append(t.Inputs, Input{k, "string"})
	}
	// every program has outputs of its own: nothing a program writes is an input of the load
	for i := 0; i < o.Programs; i++ {
		add(fmt.Sprintf("soak-out-b-%03d", i), fmt.Sprintf("name: Soak Ausgang B %03d\ntype: bool\n", i))
		add(fmt.Sprintf("soak-out-n-%03d", i), fmt.Sprintf("name: Soak Ausgang N %03d\ntype: number\nlogged: true\n", i))
	}
	add("soak-out-l", "name: Soak Ausgang L\ntype: list\nentries:\n  - aus\n  - halb\n  - voll\n")
	add("soak-out-s", "name: Soak Ausgang S\ntype: string\n")
	for i := 0; i < 10; i++ {
		add(fmt.Sprintf("soak-alarm-%02d", i), fmt.Sprintf("name: Soak Alarm %02d\ntype: alarm\n", i))
		add(fmt.Sprintf("soak-mittel-%02d", i), fmt.Sprintf("name: Soak Mittel %02d\ntype: number\ncomputed:\n  function: average\n  inputs:\n    - variable: soak-in-n-%03d\n    - variable: soak-in-n-%03d\n", i, i, i+10))
	}
	// the rest, to the number asked for: variables no program uses, as every installation has
	for i := 0; len(vars) < o.Variables; i++ {
		add(fmt.Sprintf("soak-frei-%03d", i), fmt.Sprintf("name: Soak frei %03d\nnote: von keinem Programm verwendet\ntype: number\nstart_value: %d\n", i, i))
	}

	set := model.NewSet()
	for key, text := range vars {
		v, err := model.DecodeVariable(key+".yaml", key, []byte(text))
		if err != nil {
			return nil, fmt.Errorf("variable %s: %w", key, err)
		}
		set.Variables[key] = v
		t.Variables[key] = model.EncodeVariable(v)
	}
	for i := 0; i < o.Programs; i++ {
		key := fmt.Sprintf("soak-%03d", i)
		p, err := model.DecodeProgram(key+".yaml", key, []byte(program(o, i)))
		if err != nil {
			return nil, fmt.Errorf("program %s: %w", key, err)
		}
		set.Programs[key] = p
		t.Programs[key] = model.EncodeProgram(p, nil)
	}
	problems := set.Problems()
	keys := make([]string, 0, len(problems))
	for k := range problems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		return nil, fmt.Errorf("program %s: %s", k, strings.Join(problems[k], "; "))
	}
	sort.Slice(t.Inputs, func(a, b int) bool { return t.Inputs[a].Key < t.Inputs[b].Key })
	return t, nil
}

// program is the text of program i: one of the shapes, on the inputs its number picks.
func program(o Options, i int) string {
	b := fmt.Sprintf("soak-in-b-%03d", i%boolInputs)
	b2 := fmt.Sprintf("soak-in-b-%03d", (i+7)%boolInputs)
	n := fmt.Sprintf("soak-in-n-%03d", i%numberInputs)
	l := fmt.Sprintf("soak-in-l-%03d", i%listInputs)
	s := fmt.Sprintf("soak-in-s-%03d", i%textInputs)
	ob := fmt.Sprintf("soak-out-b-%03d", i)
	on := fmt.Sprintf("soak-out-n-%03d", i)
	head := fmt.Sprintf("name: Soak %03d %%s\nactive: true\n", i)
	shape := i % templates
	if shape == 10 && o.Read == "" || shape == 11 && o.Write == "" {
		shape = 0
	}
	switch shape {
	case 0: // "when changed" with an Else: both edges write
		return fmt.Sprintf(head, "Aenderung") + "at_start: true\nrules:\n  - when:\n      any:\n        - all:\n            - variable: " + b + "\n              is: true\n              trigger: change\n    then:\n      - variable: " + ob + "\n        to: true\n  - else:\n      - variable: " + ob + "\n        to: false\n"
	case 1: // "when updated", a delay the next run ends
		return fmt.Sprintf(head, "Aktualisierung") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + n + "\n              op: gt\n              value: 50\n              trigger: update\n    cancel_pending: true\n    then:\n      - variable: " + on + "\n        to: 1\n        after: 30s\n  - else:\n      - variable: " + on + "\n        to: 0\n"
	case 2: // two groups, AND inside and OR between, a check-only line
		return fmt.Sprintf(head, "Gruppen") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + b + "\n              is: true\n              trigger: change\n            - variable: " + n + "\n              op: lt\n              value: 80\n              trigger: check\n        - all:\n            - variable: " + b2 + "\n              is: false\n              not: true\n              trigger: change\n    then:\n      - variable: " + on + "\n        from_variable: " + n + "\n"
	case 3: // If, Else if, Else on ranges of a number; a value list is written
		return fmt.Sprintf(head, "Kette") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + n + "\n              op: lt\n              value: 30\n              trigger: change\n    then:\n      - variable: soak-out-l\n        to: aus\n  - when:\n      any:\n        - all:\n            - variable: " + n + "\n              op: range\n              from: 30\n              below: 70\n              trigger: change\n    then:\n      - variable: soak-out-l\n        to: halb\n  - else:\n      - variable: soak-out-l\n        to: voll\n      - variable: " + on + "\n        to: 2\n"
	case 4: // a delay nothing ends, and a random one
		return fmt.Sprintf(head, "Verzoegerung") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + l + "\n              is: voll\n              trigger: change\n    then:\n      - variable: " + ob + "\n        to: true\n        after: 10s\n      - variable: " + ob + "\n        to: false\n        after: 20s\n        after_up_to: 2min\n"
	case 5: // an interval: every minute or every five
		every := "1min"
		if i%2 == 1 {
			every = "5min"
		}
		return fmt.Sprintf(head, "Intervall") + "rules:\n  - when:\n      any:\n        - all:\n            - time:\n                kind: interval\n                all_day: true\n                interval: " + every + "\n              trigger: update\n    then:\n      - variable: " + on + "\n        from_variable: " + n + "\n"
	case 6: // holds for
		return fmt.Sprintf(head, "haelt seit") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + b + "\n              is: true\n              trigger: change\n              for: 20s\n    then:\n      - variable: " + on + "\n        to: 6\n"
	case 7: // triggered by what program i-7 (shape 0) writes: a chain of two, never a circle
		return fmt.Sprintf(head, "Folge") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + fmt.Sprintf("soak-out-b-%03d", i-7) + "\n              is: true\n              trigger: change\n    then:\n      - variable: " + on + "\n        to: 7\n      - run: " + fmt.Sprintf("soak-%03d", i-1) + "\n        mode: check\n"
	case 8: // an alarm: set above 95, acknowledged below 5
		a := fmt.Sprintf("soak-alarm-%02d", i%10)
		return fmt.Sprintf(head, "Alarm") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + n + "\n              op: gt\n              value: 95\n              trigger: change\n    then:\n      - variable: " + a + "\n        to: true\n  - when:\n      any:\n        - all:\n            - variable: " + n + "\n              op: lt\n              value: 5\n              trigger: change\n    then:\n      - variable: " + a + "\n        to: false\n"
	case 9: // a daily range and a computed value, and a text at every change of the value
		return fmt.Sprintf(head, "Zeitspanne") + "at_start: true\nrules:\n  - when:\n      all:\n        - any:\n            - time:\n                kind: daily\n                at: \"06:00\"\n                duration: 16h\n              trigger: update\n            - variable: " + s + "\n              op: ne\n              value: \"\"\n              trigger: value\n        - any:\n            - variable: " + fmt.Sprintf("soak-mittel-%02d", i%10) + "\n              op: ge\n              value: 0\n              trigger: check\n    then:\n      - variable: soak-out-s\n        from_variable: " + s + "\n  - else:\n      - variable: " + on + "\n        to: 9\n"
	case 10: // a real device's datapoint, at every report
		return fmt.Sprintf(head, "Geraet") + "rules:\n  - when:\n      any:\n        - all:\n            - device: " + o.Read + "\n              is: " + o.ReadValue + "\n              trigger: update\n    then:\n      - variable: " + ob + "\n        to: true\n  - else:\n      - variable: " + ob + "\n        to: false\n"
	default: // 11: a write to a device, through the system
		return fmt.Sprintf(head, "Schreiben") + "rules:\n  - when:\n      any:\n        - all:\n            - variable: " + b + "\n              is: true\n              trigger: change\n    then:\n      - set: " + o.Write + "\n        to: " + o.WriteValue + "\n      - variable: " + on + "\n        to: 11\n"
	}
}
