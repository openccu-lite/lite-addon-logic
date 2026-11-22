package model

import (
	"errors"
	"strings"
	"testing"
)

const okRule = "rules:\n  - when:\n      all:\n        - all:\n            - variable: v\n              is: true\n              trigger: change\n    then: []\n"

// rule wraps one condition line and one activity (both YAML at their indent) into a program.
func prog(line, action string) string {
	if line == "" {
		line = "            - variable: v\n              is: true\n              trigger: change\n"
	}
	then := "    then: []\n"
	if action != "" {
		then = "    then:\n" + action
	}
	return "name: P\nrules:\n  - when:\n      all:\n        - all:\n" + line + then
}

// Every refusal names the file and the line.
func TestProgramErrors(t *testing.T) {
	for _, c := range []struct {
		name, yaml string
		line       int
		msg        string
	}{
		{"empty", "", 0, "empty"},
		{"only a comment", "# nothing\n", 0, "empty"},
		{"not UTF-8", "name: \xff\n", 0, "UTF-8"},
		{"syntax", "name: P\nrules: [\n", 2, "did not find"},
		{"two documents", "name: P\nrules: []\n---\nname: Q\n", 3, "second document"},
		{"a list", "- a\n", 1, "must be a mapping"},
		{"no name", "rules: []\n", 1, "needs a name"},
		{"blank name", "name: \"  \"\nrules: []\n", 1, "needs a name"},
		{"name is a number", "name: 123\nrules: []\n", 1, "must be text"},
		{"name is a list", "name:\n  - a\nrules: []\n", 2, "single value"},
		{"long name", "name: " + strings.Repeat("x", 201) + "\nrules: []\n", 1, "longer than"},
		{"long note", "name: P\nnote: " + strings.Repeat("x", MaxText+1) + "\nrules: []\n", 2, "note is longer"},
		{"note not text", "name: P\nnote: true\nrules: []\n", 2, "must be text"},
		{"active", "name: P\nactive: yes\nrules: []\n", 2, "true or false"},
		{"active list", "name: P\nactive: [1]\nrules: []\n", 2, "flow style"},
		{"no rules", "name: P\n", 1, "needs rules"},
		{"rules not a list", "name: P\nrules: x\n", 2, "must be a list"},
		{"unknown key", "name: P\ncolour: red\nrules: []\n", 2, "unknown key \"colour\""},
		{"key twice", "name: P\nname: Q\nrules: []\n", 2, "twice"},
		{"anchor", "name: &a P\nrules: []\n", 1, "anchor"},
		{"alias", "name: &a P\nnote: *a\nrules: []\n", 1, "anchor"},
		{"alias value", "x: &a P\nname: *a\nrules: []\n", 2, "alias"},
		{"tag", "name: !!str P\nrules: []\n", 1, "tag"},
		{"flow mapping", "name: P\nrules:\n  - {else: []}\n", 3, "flow style"},
		{"complex key", "? [a]\n: b\n", 1, "flow style"},
		{"mapping key", "? a: b\n: c\n", 1, "a key must be a word"},
		{"rule not a mapping", "name: P\nrules:\n  - x\n", 3, "a rule must be a mapping"},
		{"rule without when", "name: P\nrules:\n  - note: x\n", 3, "needs \"when\""},
		{"when and else", "name: P\nrules:\n  - when:\n      all: []\n    else: []\n", 3, "not both"},
		{"else with then", "name: P\nrules:\n  - else: []\n    then: []\n", 4, "no \"then\""},
		{"when without then", "name: P\nrules:\n  - when:\n      all: []\n", 3, "needs \"then\""},
		{"else not last", "name: P\nrules:\n  - else: []\n  - when:\n      all: []\n    then: []\n", 3, "last one"},
		{"else not a list", "name: P\nrules:\n  - else: x\n", 3, "must be a list"},
		{"rule note", "name: P\nrules:\n  - else: []\n    note: 1\n", 4, "must be text"},
		{"cancel_pending", "name: P\nrules:\n  - else: []\n    cancel_pending: 1\n", 4, "true or false"},
		{"rule unknown key", "name: P\nrules:\n  - else: []\n    when_not: x\n", 4, "unknown key"},
		{"then not a list", "name: P\nrules:\n  - when:\n      all: []\n    then: x\n", 5, "must be a list"},
		{"when without op", "name: P\nrules:\n  - when: {}\n    then: []\n", 3, "exactly one of"},
		{"when both ops", "name: P\nrules:\n  - when:\n      all: []\n      any: []\n    then: []\n", 4, "exactly one of"},
		{"when extra key", "name: P\nrules:\n  - when:\n      all: []\n      none: []\n    then: []\n", 5, "unknown key"},
		{"when not a mapping", "name: P\nrules:\n  - when: x\n    then: []\n", 3, "must be a mapping"},
		{"groups not a list", "name: P\nrules:\n  - when:\n      all: x\n    then: []\n", 4, "must be a list"},
		{"group not a mapping", "name: P\nrules:\n  - when:\n      all:\n        - x\n    then: []\n", 5, "must be a mapping"},
		{"empty group", "name: P\nrules:\n  - when:\n      all:\n        - any: []\n    then: []\n", 5, "at least one line"},
		{"line not a mapping", "name: P\nrules:\n  - when:\n      all:\n        - any:\n            - x\n    then: []\n", 6, "must be a mapping"},
		{"line on nothing", prog("            - is: true\n              trigger: change\n", ""), 6, "exactly one of"},
		{"line on two", prog("            - variable: v\n              device: HmIP-RF/A:1/STATE\n              is: true\n              trigger: change\n", ""), 6, "exactly one of"},
		{"bad device", prog("            - device: HmIP-RF/A/STATE\n              is: true\n              trigger: change\n", ""), 6, "not a datapoint"},
		{"device not text", prog("            - device: 1\n              is: true\n              trigger: change\n", ""), 6, "must be text"},
		{"bad variable key", prog("            - variable: Regen\n              is: true\n              trigger: change\n", ""), 6, "not a variable's key"},
		{"variable not text", prog("            - variable: [a]\n              is: true\n              trigger: change\n", ""), 6, "flow style"},
		{"no comparison", prog("            - variable: v\n              trigger: change\n", ""), 6, "needs \"is\" or \"op\""},
		{"is and op", prog("            - variable: v\n              is: 1\n              op: gt\n              trigger: change\n", ""), 8, "not both"},
		{"is with value", prog("            - variable: v\n              is: 1\n              value: 2\n              trigger: change\n", ""), 8, "has no \"value\""},
		{"is null", prog("            - variable: v\n              is:\n              trigger: change\n", ""), 7, "must be true, false, a number or text"},
		{"is a list", prog("            - variable: v\n              is:\n                - a\n              trigger: change\n", ""), 8, "single value"},
		{"unknown op", prog("            - variable: v\n              op: like\n              value: 1\n              trigger: change\n", ""), 7, "not one of"},
		{"op eq spelled out", prog("            - variable: v\n              op: eq\n              value: 1\n              trigger: change\n", ""), 7, "not one of"},
		{"gt without value", prog("            - variable: v\n              op: gt\n              trigger: change\n", ""), 6, "needs \"value\""},
		{"gt on text", prog("            - variable: v\n              op: gt\n              value: x\n              trigger: change\n", ""), 8, "compares numbers"},
		{"gt value bad", prog("            - variable: v\n              op: gt\n              value: .nan\n              trigger: change\n", ""), 8, "must be a number"},
		{"range without below", prog("            - variable: v\n              op: range\n              from: 1\n              trigger: change\n", ""), 6, "needs \"below\""},
		{"range with to", prog("            - variable: v\n              op: range\n              from: 1\n              below: 2\n              to: 3\n              trigger: change\n", ""), 10, "has no \"to\""},
		{"range on text", prog("            - variable: v\n              op: range\n              from: a\n              below: 2\n              trigger: change\n", ""), 6, "compares numbers"},
		{"range from bad", prog("            - variable: v\n              op: range\n              from:\n              below: 2\n              trigger: change\n", ""), 8, "must be"},
		{"range below bad", prog("            - variable: v\n              op: range\n              from: 1\n              below:\n              trigger: change\n", ""), 9, "must be"},
		{"outside without to", prog("            - variable: v\n              op: outside\n              from: 1\n              trigger: change\n", ""), 6, "needs \"to\""},
		{"outside on text", prog("            - variable: v\n              op: outside\n              from: 1\n              to: b\n              trigger: change\n", ""), 6, "compares numbers"},
		{"outside from bad", prog("            - variable: v\n              op: outside\n              from:\n              to: 2\n              trigger: change\n", ""), 8, "must be"},
		{"outside to bad", prog("            - variable: v\n              op: outside\n              from: 1\n              to:\n              trigger: change\n", ""), 9, "must be"},
		{"not", prog("            - variable: v\n              is: 1\n              not: 1\n              trigger: change\n", ""), 8, "true or false"},
		{"no trigger", prog("            - variable: v\n              is: 1\n", ""), 6, "needs \"trigger\""},
		{"bad trigger", prog("            - variable: v\n              is: 1\n              trigger: always\n", ""), 8, "not one of"},
		{"line note", prog("            - variable: v\n              is: 1\n              trigger: check\n              note: [x]\n", ""), 9, "flow style"},
		{"line unknown key", prog("            - variable: v\n              is: 1\n              trigger: check\n              since: 10min\n", ""), 9, "unknown key"},
		{"bad for", prog("            - variable: v\n              is: 1\n              for: soon\n              trigger: check\n", ""), 8, "not a duration"},
		{"for too long", prog("            - variable: v\n              is: 1\n              for: 200h\n              trigger: check\n", ""), 8, "from 1s to 168h"},
		{"for of nothing", prog("            - variable: v\n              is: 1\n              for: 0s\n              trigger: check\n", ""), 8, "from 1s to"},
		{"for on a time line", prog("            - time:\n                kind: daily\n                at: \"06:30\"\n              for: 5min\n              trigger: update\n", ""), 9, "a time line has none"},
		{"time bad point", prog("            - time:\n                kind: daily\n                astro: sunset\n                point: maybe\n              trigger: update\n", ""), 9, "true or false"},
		{"time bad twilight", prog("            - time:\n                kind: daily\n                astro: sunset\n                twilight: dark\n              trigger: update\n", ""), 9, "not one of"},
		{"point without astro", prog("            - time:\n                kind: daily\n                at: \"06:30\"\n                point: true\n              trigger: update\n", ""), 7, "belong to \"astro\""},
		{"twilight without astro", prog("            - time:\n                kind: daily\n                all_day: true\n                twilight: civil\n              trigger: update\n", ""), 7, "belong to \"astro\""},
		{"point with a duration", prog("            - time:\n                kind: daily\n                astro: sunset\n                point: true\n                duration: 1h\n              trigger: update\n", ""), 7, "a point has no"},
		{"time with is", prog("            - time:\n                kind: daily\n                at: \"06:30\"\n              is: 1\n              trigger: update\n", ""), 9, "a time line has no"},
		{"time trigger change", prog("            - time:\n                kind: daily\n                at: \"06:30\"\n              trigger: change\n", ""), 9, "not one of update, check"},
		{"time not a mapping", prog("            - time: daily\n              trigger: update\n", ""), 6, "must be a mapping"},
		{"time without kind", prog("            - time:\n                at: \"06:30\"\n              trigger: update\n", ""), 7, "needs \"kind\""},
		{"time bad kind", prog("            - time:\n                kind: hourly\n              trigger: update\n", ""), 7, "not one of"},
		{"time at unquoted", prog("            - time:\n                kind: daily\n                at: 630\n              trigger: update\n", ""), 8, "must be text"},
		{"time bad at", prog("            - time:\n                kind: daily\n                at: \"25:00\"\n              trigger: update\n", ""), 8, "not a time of day"},
		{"time bad astro", prog("            - time:\n                kind: daily\n                astro: noon\n              trigger: update\n", ""), 8, "not one of"},
		{"time at and astro", prog("            - time:\n                kind: daily\n                at: \"06:30\"\n                astro: sunset\n              trigger: update\n", ""), 7, "exactly one of"},
		{"time bad offset", prog("            - time:\n                kind: daily\n                offset: soon\n              trigger: update\n", ""), 8, "not a duration"},
		{"time offset list", prog("            - time:\n                kind: daily\n                offset:\n                  - 1s\n              trigger: update\n", ""), 9, "single value"},
		{"time bad duration", prog("            - time:\n                kind: daily\n                duration: 1d\n              trigger: update\n", ""), 8, "not a duration"},
		{"time bad interval", prog("            - time:\n                kind: interval\n                interval: x\n              trigger: update\n", ""), 8, "not a duration"},
		{"interval without interval", prog("            - time:\n                kind: interval\n                all_day: true\n              trigger: update\n", ""), 7, "only that, has an \"interval\""},
		{"daily with interval", prog("            - time:\n                kind: daily\n                at: \"06:30\"\n                interval: 5min\n              trigger: update\n", ""), 7, "only that, has an \"interval\""},
		{"time bad every", prog("            - time:\n                kind: daily\n                every: 0\n              trigger: update\n", ""), 8, "whole number from 1"},
		{"time every fraction", prog("            - time:\n                kind: daily\n                every: 1.5\n              trigger: update\n", ""), 8, "whole number"},
		{"time every text", prog("            - time:\n                kind: daily\n                every: two\n              trigger: update\n", ""), 8, "must be a number"},
		{"time weekdays not a list", prog("            - time:\n                kind: weekly\n                weekdays: mon\n              trigger: update\n", ""), 8, "must be a list"},
		{"time bad weekday", prog("            - time:\n                kind: weekly\n                weekdays:\n                  - monday\n              trigger: update\n", ""), 9, "not one of"},
		{"time weekday twice", prog("            - time:\n                kind: weekly\n                weekdays:\n                  - mon\n                  - mon\n              trigger: update\n", ""), 10, "twice"},
		{"time weekday mapping", prog("            - time:\n                kind: weekly\n                weekdays:\n                  - a: b\n              trigger: update\n", ""), 9, "single value"},
		{"time bad all_day", prog("            - time:\n                kind: daily\n                all_day: maybe\n              trigger: update\n", ""), 8, "true or false"},
		{"time bad nth", prog("            - time:\n                kind: monthly\n                nth: 6\n              trigger: update\n", ""), 8, "whole number"},
		{"time without a when", prog("            - time:\n                kind: daily\n              trigger: update\n", ""), 7, "exactly one of"},
		{"time bad day", prog("            - time:\n                kind: monthly\n                day: 32\n              trigger: update\n", ""), 8, "whole number"},
		{"time bad month", prog("            - time:\n                kind: yearly\n                month: 13\n              trigger: update\n", ""), 8, "whole number"},
		{"time date unquoted", prog("            - time:\n                kind: once\n                date: 20270101\n              trigger: update\n", ""), 8, "must be text"},
		{"time bad date", prog("            - time:\n                kind: once\n                date: \"1.1.2027\"\n              trigger: update\n", ""), 8, "not a date"},
		{"time bad from", prog("            - time:\n                kind: daily\n                from: \"2027-13-01\"\n              trigger: update\n", ""), 8, "not a date"},
		{"time bad until", prog("            - time:\n                kind: daily\n                until: \"2027-01-01 25:00\"\n              trigger: update\n", ""), 8, "not a date"},
		{"time bad count", prog("            - time:\n                kind: daily\n                count: -1\n              trigger: update\n", ""), 8, "whole number"},
		{"time unknown key", prog("            - time:\n                kind: daily\n                zone: UTC\n              trigger: update\n", ""), 8, "unknown key"},
		{"action not a mapping", prog("", "      - x\n"), 10, "must be a mapping"},
		{"action of no kind", prog("", "      - to: 1\n"), 10, "an activity is one of"},
		{"action of two kinds", prog("", "      - set: HmIP-RF/A:1/STATE\n        variable: v\n        to: 1\n"), 11, "exactly one of"},
		{"set bad ref", prog("", "      - set: nowhere\n        to: 1\n"), 10, "not a datapoint"},
		{"set without value", prog("", "      - set: HmIP-RF/A:1/STATE\n"), 10, "needs exactly one of"},
		{"set with two values", prog("", "      - set: HmIP-RF/A:1/STATE\n        to: 1\n        special: X\n"), 10, "needs exactly one of"},
		{"set to null", prog("", "      - set: HmIP-RF/A:1/STATE\n        to:\n"), 11, "must be true, false, a number or text"},
		{"set from bad", prog("", "      - set: HmIP-RF/A:1/STATE\n        from: x\n"), 11, "not a datapoint"},
		{"set from_variable bad", prog("", "      - set: HmIP-RF/A:1/STATE\n        from_variable: X y\n"), 11, "not a variable's key"},
		{"set from_variable list", prog("", "      - set: HmIP-RF/A:1/STATE\n        from_variable:\n          - a\n"), 12, "single value"},
		{"set special empty", prog("", "      - set: HmIP-RF/A:1/STATE\n        special: \"\"\n"), 11, "names the special value"},
		{"set special not text", prog("", "      - set: HmIP-RF/A:1/STATE\n        special: 1\n"), 11, "must be text"},
		{"variable bad key", prog("", "      - variable: Bad Key\n        to: 1\n"), 10, "not a variable's key"},
		{"variable not text", prog("", "      - variable: 1\n        to: 1\n"), 10, "must be text"},
		{"run bad key", prog("", "      - run: Bad Key\n        mode: then\n"), 10, "not a program's key"},
		{"run not text", prog("", "      - run: 1\n        mode: then\n"), 10, "must be text"},
		{"run without mode", prog("", "      - run: p\n"), 10, "needs \"mode\""},
		{"run bad mode", prog("", "      - run: p\n        mode: fast\n"), 11, "not one of"},
		{"run with to", prog("", "      - run: p\n        mode: then\n        to: 1\n"), 12, "only \"set\" and \"variable\""},
		{"activate with mode", prog("", "      - activate: p\n        mode: then\n"), 11, "only a \"run\" activity"},
		{"script not text", prog("", "      - script: 1\n"), 10, "must be text"},
		{"script too long", prog("", "      - script: "+strings.Repeat("x", MaxText+1)+"\n"), 10, "script is longer"},
		{"bad after", prog("", "      - variable: v\n        to: 1\n        after: 5\n"), 12, "not a duration"},
		{"bad on_for", prog("", "      - set: HmIP-RF/A:1/STATE\n        to: true\n        on_for: x\n"), 12, "not a duration"},
		{"on_for on a variable", prog("", "      - variable: v\n        to: 1\n        on_for: 5min\n"), 12, "only a \"set\" activity"},
		{"on_for without to", prog("", "      - set: HmIP-RF/A:1/STATE\n        from_variable: v\n        on_for: 5min\n"), 12, "only a \"set\" activity"},
		{"on_for too long", prog("", "      - set: HmIP-RF/A:1/STATE\n        to: true\n        on_for: 169h\n"), 12, "from 1s to"},
		{"bad after_up_to", prog("", "      - variable: v\n        to: 1\n        after_up_to: x\n"), 12, "not a duration"},
		{"after_up_to not more", prog("", "      - variable: v\n        to: 1\n        after: 5min\n        after_up_to: 5min\n"), 13, "more than \"after\""},
		{"after_up_to on a script", prog("", "      - script: x\n        after_up_to: 5min\n"), 11, "more than \"after\""},
		{"action note", prog("", "      - variable: v\n        to: 1\n        note: 5\n"), 12, "must be text"},
		{"action unknown key", prog("", "      - variable: v\n        to: 1\n        retry: 3\n"), 12, "unknown key"},
	} {
		_, err := DecodeProgram("programs/p.yaml", "p", []byte(c.yaml))
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: no error (%v)", c.name, err)
			continue
		}
		if e.File != "programs/p.yaml" || e.Line != c.line || !strings.Contains(e.Msg, c.msg) {
			t.Errorf("%s: %v, want line %d with %q", c.name, err, c.line, c.msg)
		}
		if c.line > 0 && !strings.HasPrefix(err.Error(), "programs/p.yaml:") {
			t.Errorf("%s: %q does not start with the file and the line", c.name, err.Error())
		}
	}
	if _, err := DecodeProgram("x.yaml", "Bad Key", []byte(okRule)); err == nil || !strings.Contains(err.Error(), "x.yaml: ") || !strings.Contains(err.Error(), "cannot be a program's key") {
		t.Errorf("a bad key: %v", err)
	}
}

func TestVariableErrors(t *testing.T) {
	for _, c := range []struct {
		name, yaml string
		line       int
		msg        string
	}{
		{"empty", "", 0, "empty"},
		{"a list", "- a\n", 1, "must be a mapping"},
		{"no name", "type: bool\n", 1, "needs a name"},
		{"name number", "name: 1\ntype: bool\n", 1, "must be text"},
		{"note", "name: V\nnote: 1\ntype: bool\n", 2, "must be text"},
		{"no type", "name: V\n", 1, "needs a \"type\""},
		{"bad type", "name: V\ntype: float\n", 2, "not one of"},
		{"labels on a number", "name: V\ntype: number\nlabels:\n  \"true\": x\n", 4, "has no \"labels\""},
		{"entries on a bool", "name: V\ntype: bool\nentries:\n  - a\n", 4, "has no \"entries\""},
		{"unit on a string", "name: V\ntype: string\nunit: x\n", 3, "has no \"unit\""},
		{"min on a list", "name: V\ntype: list\nentries:\n  - a\nmin: 1\n", 5, "has no \"min\""},
		{"max on a bool", "name: V\ntype: bool\nmax: 1\n", 3, "has no \"max\""},
		{"labels not a mapping", "name: V\ntype: bool\nlabels: x\n", 3, "must be a mapping"},
		{"label not text", "name: V\ntype: bool\nlabels:\n  \"true\": 1\n", 4, "must be text"},
		{"false label not text", "name: V\ntype: bool\nlabels:\n  \"false\": 1\n", 4, "must be text"},
		{"labels unknown key", "name: V\ntype: bool\nlabels:\n  maybe: x\n", 4, "unknown key"},
		{"list without entries", "name: V\ntype: list\n", 1, "needs \"entries\""},
		{"entries not a list", "name: V\ntype: list\nentries: a\n", 3, "must be a list"},
		{"entry a mapping", "name: V\ntype: list\nentries:\n  - a: b\n", 4, "single value"},
		{"entry a number", "name: V\ntype: list\nentries:\n  - 1\n", 4, "put 1 in quotes"},
		{"entry twice", "name: V\ntype: list\nentries:\n  - a\n  - a\n", 5, "there twice"},
		{"entry empty", "name: V\ntype: list\nentries:\n  - \"\"\n", 4, "is empty"},
		{"entry with semicolon", "name: V\ntype: list\nentries:\n  - \"a;b\"\n", 4, "semicolon"},
		{"unit not text", "name: V\ntype: number\nunit: 1\n", 3, "must be text"},
		{"min text", "name: V\ntype: number\nmin: low\n", 3, "must be a number"},
		{"max list", "name: V\ntype: number\nmax:\n  - 1\n", 4, "single value"},
		{"min above max", "name: V\ntype: number\nmin: 5\nmax: 1\n", 3, "min is above max"},
		{"visible", "name: V\ntype: bool\nvisible: no\n", 3, "true or false"},
		{"internal", "name: V\ntype: bool\ninternal: 1\n", 3, "true or false"},
		{"logged", "name: V\ntype: bool\nlogged: x\n", 3, "true or false"},
		{"channel not text", "name: V\ntype: bool\nchannel: 1\n", 3, "must be text"},
		{"bad channel", "name: V\ntype: bool\nchannel: HmIP-RF/ABC\n", 3, "not a channel"},
		{"start_value null", "name: V\ntype: bool\nstart_value:\n", 3, "must be true"},
		{"start_value of another type", "name: V\ntype: bool\nstart_value: 5\n", 3, "takes true or false"},
		{"start_value not an entry", "name: V\ntype: list\nentries:\n  - a\nstart_value: b\n", 5, "one of its entries"},
		{"unknown key", "name: V\ntype: bool\nvalue: true\n", 3, "unknown key \"value\""},
	} {
		_, err := DecodeVariable("variables/v.yaml", "v", []byte(c.yaml))
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: no error (%v)", c.name, err)
			continue
		}
		if e.File != "variables/v.yaml" || e.Line != c.line || !strings.Contains(e.Msg, c.msg) {
			t.Errorf("%s: %v, want line %d with %q", c.name, err, c.line, c.msg)
		}
	}
	if _, err := DecodeVariable("x.yaml", "", []byte("name: V\ntype: bool\n")); err == nil || !strings.Contains(err.Error(), "cannot be a variable's key") {
		t.Errorf("a bad key: %v", err)
	}
}

// What a hand-written file may do that the writer never does.
func TestLenientReads(t *testing.T) {
	p, err := DecodeProgram("p.yaml", "p", []byte("# my program\nname: P   # trailing\nrules:\n  - when:\n      all:\n        - all:\n            - time:\n                kind: daily\n                at: \"07:00\"\n                from: 2026-12-01\n              trigger: update\n    then: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Rules[0].When.Groups[0].Lines[0].Time.From; got != "2026-12-01" {
		t.Errorf("an unquoted date: %q", got)
	}
	v, err := DecodeVariable("v.yaml", "v", []byte("name: V\ntype: bool\nlabels:\n  false: aus\n  true: an\n"))
	if err != nil || v.Labels.False != "aus" || v.Labels.True != "an" {
		t.Errorf("unquoted label keys: %+v %v", v, err)
	}
}
