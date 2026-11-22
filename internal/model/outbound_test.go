package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const hook = `name: Melden
rules:
  - when:
      all:
        - all:
            - variable: v
              is: true
              trigger: change
    cancel_pending: true
    then:
      - webhook: "https://example.org/hook/{var:v}?t={time}"
        method: POST
        headers:
          - name: Authorization
            value: "Bearer {secret:token}"
          - name: X-Empty
            value: ""
        body_type: json
        body: |-
          {"program": "{program}",
           "value": {value}}
        insecure: true
        after: 5s
      - webhook: "http://127.0.0.1:8080/"
        method: GET
      - wake: "AA:BB:CC:00:11:22"
        broadcast: "192.168.1.255"
        note: der Rechner
      - wake: "AA:BB:CC:00:11:22"
`

// A webhook and a Wake-on-LAN are written and read back the same, and say what they target.
func TestOutboundRoundTrip(t *testing.T) {
	p, err := DecodeProgram("melden.yaml", "melden", []byte(hook))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(EncodeProgram(p, nil)); got != hook {
		t.Fatalf("written:\n%s", got)
	}
	then := p.Rules[0].Then
	w := then[0].Webhook
	if then[0].Kind != ActionWebhook || w.Method != "POST" || !w.Insecure || w.BodyType != BodyJSON || len(w.Headers) != 2 || w.Headers[0] != (Header{"Authorization", "Bearer {secret:token}"}) {
		t.Errorf("webhook %+v", w)
	}
	if got := w.Placeholders(); !reflect.DeepEqual(got, []string{"var:v", "time", "secret:token", "program", "value"}) {
		t.Errorf("placeholders %v", got)
	}
	if then[2].Kind != ActionWake || *then[2].Wake != (Wake{"AA:BB:CC:00:11:22", "192.168.1.255"}) || then[2].Note != "der Rechner" {
		t.Errorf("wake %+v", then[2])
	}
	for i, want := range []string{"url:https://example.org/hook/{var:v}?t={time}", "url:http://127.0.0.1:8080/", "mac:AA:BB:CC:00:11:22", "mac:AA:BB:CC:00:11:22"} {
		if got := then[i].Target(); got != want {
			t.Errorf("target %d: %s", i, got)
		}
	}
	if (Action{Kind: ActionWebhook}).Target() != "" || (Action{Kind: ActionWake}).Target() != "" {
		t.Error("a target without a request")
	}
	// a copy shares nothing
	c := p.Clone()
	c.Rules[0].Then[0].Webhook.Headers[0].Value = "x"
	c.Rules[0].Then[2].Wake.MAC = "x"
	if w.Headers[0].Value == "x" || then[2].Wake.MAC == "x" {
		t.Error("the copy shares the request")
	}
	// one without its request is written so that the reader says what is missing
	for _, a := range []Action{{Kind: ActionWebhook}, {Kind: ActionWake}} {
		bare := &Program{Key: "p", Name: "P", Rules: []Rule{{When: p.Rules[0].When, Then: []Action{a}}}}
		if _, err := bare.Check(); err == nil {
			t.Errorf("%s without its request is taken", a.Kind)
		}
	}
	// the set: a placeholder's variable must exist, and the variable is in use
	s := NewSet()
	s.Programs["melden"] = p
	if got := s.ProgramProblems(p); len(got) != 2 || !strings.Contains(got[1], "the variable v does not exist") {
		t.Errorf("problems %v", got)
	}
	s.Variables["v"] = &Variable{Key: "v", Name: "V", Type: VarBool}
	if got := s.ProgramProblems(p); len(got) != 0 {
		t.Errorf("problems %v", got)
	}
	p.Rules[0].When = nil
	if got := s.VariableUsers("v"); len(got) != 1 {
		t.Errorf("users %v", got)
	}
	if got := s.VariableUsers("w"); len(got) != 0 {
		t.Errorf("users of w %v", got)
	}
}

func TestOutboundErrors(t *testing.T) {
	long := strings.Repeat("x", MaxURL)
	for _, c := range []struct{ name, action, msg string }{
		{"no method", "      - webhook: \"http://h/\"\n", "needs a \"method\""},
		{"method", "      - webhook: \"http://h/\"\n        method: DELETE\n", "is not one of GET, POST, PUT"},
		{"method not text", "      - webhook: \"http://h/\"\n        method: [GET]\n", "flow style"},
		{"url not text", "      - webhook: 5\n        method: GET\n", "must be text"},
		{"empty url", "      - webhook: \"\"\n        method: GET\n", "the address is empty"},
		{"long url", "      - webhook: \"http://h/" + long + "\"\n        method: GET\n", "longer than"},
		{"blank in url", "      - webhook: \"http://h/a b\"\n        method: GET\n", "has a blank"},
		{"scheme", "      - webhook: \"ftp://h/\"\n        method: GET\n", "must be http://"},
		{"no host", "      - webhook: \"http:///x\"\n        method: GET\n", "must be http://"},
		{"user", "      - webhook: \"http://u:p@h/\"\n        method: GET\n", "without a user name"},
		{"fragment", "      - webhook: \"http://h/#x\"\n        method: GET\n", "or a fragment"},
		{"unparsable", "      - webhook: \"http://h:x/\"\n        method: GET\n", "must be http://"},
		{"placeholder as host", "      - webhook: \"http://{var:host}/x\"\n        method: GET\n", "cannot stand in the address' host"},
		{"placeholder in host", "      - webhook: \"http://a{value}?x=1\"\n        method: GET\n", "cannot stand in the address' host"},
		{"headers not a list", "      - webhook: \"http://h/\"\n        method: GET\n        headers: x\n", "must be a list"},
		{"header not a mapping", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - x\n", "must be a mapping"},
		{"header name not text", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: 1\n            value: x\n", "must be text"},
		{"header value not text", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: A\n            value: 1\n", "must be text"},
		{"header key", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: A\n            wert: x\n", "unknown key"},
		{"header name", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: \"A B\"\n            value: x\n", "is not a header's name"},
		{"own header", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: Host\n            value: x\n", "set by the request itself"},
		{"header twice", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: A\n            value: x\n          - name: a\n            value: y\n", "is there twice"},
		{"header break", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n          - name: A\n            value: \"x\\ny\"\n", "has a line break"},
		{"nine headers", "      - webhook: \"http://h/\"\n        method: GET\n        headers:\n" + strings.Repeat("          - name: A\n            value: x\n", 9), "at most 8 headers"},
		{"body type without body", "      - webhook: \"http://h/\"\n        method: POST\n        body_type: json\n", "only a webhook with a body"},
		{"body type not text", "      - webhook: \"http://h/\"\n        method: POST\n        body_type: 1\n", "must be text"},
		{"body not text", "      - webhook: \"http://h/\"\n        method: POST\n        body: 1\n", "must be text"},
		{"GET with a body", "      - webhook: \"http://h/\"\n        method: GET\n        body_type: text\n        body: x\n", "a GET has no body"},
		{"long body", "      - webhook: \"http://h/\"\n        method: PUT\n        body_type: text\n        body: " + strings.Repeat("x", MaxBody+1) + "\n", "the body is longer"},
		{"no body type", "      - webhook: \"http://h/\"\n        method: PUT\n        body: x\n", "is not one of json, form, text"},
		{"insecure", "      - webhook: \"http://h/\"\n        method: GET\n        insecure: ja\n", "true or false"},
		{"method elsewhere", "      - variable: v\n        to: true\n        method: GET\n", "only a \"webhook\" activity has \"method\""},
		{"broadcast elsewhere", "      - webhook: \"http://h/\"\n        method: GET\n        broadcast: \"1.2.3.4\"\n", "only a \"wake\" activity has \"broadcast\""},
		{"two kinds", "      - webhook: \"http://h/\"\n        wake: \"AA:BB:CC:00:11:22\"\n", "exactly one of"},
		{"mac not text", "      - wake: 5\n", "must be text"},
		{"mac", "      - wake: \"AA:BB:CC:00:11\"\n", "is not a hardware address"},
		{"mac not canonical", "      - wake: \"aa-bb-cc-00-11-22\"\n", "is not a hardware address"},
		{"broadcast not text", "      - wake: \"AA:BB:CC:00:11:22\"\n        broadcast: 5\n", "must be text"},
		{"broadcast", "      - wake: \"AA:BB:CC:00:11:22\"\n        broadcast: \"fe80::1\"\n", "is not an IPv4 address"},
		{"broadcast spelled oddly", "      - wake: \"AA:BB:CC:00:11:22\"\n        broadcast: \"::ffff:1.2.3.4\"\n", "is not an IPv4 address"},
		{"to on a wake", "      - wake: \"AA:BB:CC:00:11:22\"\n        to: true\n", "only \"set\" and \"variable\""},
	} {
		_, err := DecodeProgram("p.yaml", "p", []byte(prog("", c.action)))
		var e *Error
		if !errors.As(err, &e) || !strings.Contains(e.Msg, c.msg) || e.Line == 0 {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestCanonicalMAC(t *testing.T) {
	for in, want := range map[string]string{
		"aa:bb:cc:00:11:22":   "AA:BB:CC:00:11:22",
		" AA-BB-CC-00-11-22 ": "AA:BB:CC:00:11:22",
		"aabbcc001122":        "AA:BB:CC:00:11:22",
		"aa:bb:cc:00:11":      "",
		"aa:bbcc:00:11:22":    "",
		"gg:bb:cc:00:11:22":   "",
		"":                    "",
	} {
		if got := CanonicalMAC(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// Only the fixed placeholders are touched: a JSON body's braces stay.
func TestExpand(t *testing.T) {
	fill := func(name string) string { return "<" + name + ">" }
	got := Expand(`{"a": "{program}", "b": {value}, "c": {"d": "{var:x-1}{secret:t}{trigger}{time}"}, "e": "{nope}{var:}{VAR:x}"}`, fill, strings.ToUpper)
	want := `{"a": "<PROGRAM>", "b": <VALUE>, "c": {"d": "<VAR:X-1><SECRET:T><TRIGGER><TIME>"}, "e": "{nope}{var:}{VAR:x}"}`
	if got != want {
		t.Errorf("got %s", got)
	}
}

const several = `name: Licht
rules:
  - when:
      all: []
    then:
      - channel: HmIP-RF/000855699C1234:8  # Taster Flur
        values:
          COLOR: RED
          LEVEL: 0.5
          "ON": true
          DURATION_VALUE: 10
          TEXT: ""
        after: 5s
      - channel: BidCos-RF/NEQ1:1
        values:
          STATE: false
`

// Several values of a channel in one activity: written and read back the same, in their order.
func TestChannelValuesRoundTrip(t *testing.T) {
	p, err := DecodeProgram("licht.yaml", "licht", []byte(several))
	if err != nil {
		t.Fatal(err)
	}
	names := func(r Ref) string {
		if r.Address == "000855699C1234:8" && r.Param == "" {
			return "Taster Flur"
		}
		return ""
	}
	if got := string(EncodeProgram(p, names)); got != several {
		t.Fatalf("written:\n%s", got)
	}
	a := p.Rules[0].Then[0]
	want := []ParamValue{{"COLOR", "RED"}, {"LEVEL", 0.5}, {"ON", true}, {"DURATION_VALUE", 10.0}, {"TEXT", ""}}
	if a.Kind != ActionChannel || a.Channel != "HmIP-RF/000855699C1234:8" || !reflect.DeepEqual(a.Values, want) || a.Target() != "ch:HmIP-RF/000855699C1234:8" {
		t.Errorf("activity %+v", a)
	}
	c := p.Clone()
	c.Rules[0].Then[0].Values[0].Value = "BLUE"
	if a.Values[0].Value != "RED" {
		t.Error("the copy shares the values")
	}
	if (ChannelRef("HmIP-RF/A:1") != Ref{Interface: "HmIP-RF", Address: "A:1"}) || !ChannelRef("nope").IsZero() {
		t.Error("a channel as a reference")
	}
	// one without values is written so that the reader says what is missing
	bare := &Program{Key: "p", Name: "P", Rules: []Rule{{When: p.Rules[0].When, Then: []Action{{Kind: ActionChannel, Channel: "HmIP-RF/A:1"}}}}}
	if _, err := bare.Check(); err == nil || !strings.Contains(err.Error(), "1 to 32 datapoints") {
		t.Errorf("without values: %v", err)
	}
}

func TestChannelValuesErrors(t *testing.T) {
	many := ""
	for i := 0; i <= MaxValues; i++ {
		many += "          P" + string(rune('A'+i%26)) + string(rune('A'+i/26)) + ": 1\n"
	}
	for _, c := range []struct{ name, action, msg string }{
		{"not text", "      - channel: 5\n        values:\n          A: 1\n", "must be text"},
		{"no channel", "      - channel: HmIP-RF/A:1/STATE\n        values:\n          A: 1\n", "is not a channel"},
		{"no values", "      - channel: HmIP-RF/A:1\n", "needs \"values\""},
		{"values not a mapping", "      - channel: HmIP-RF/A:1\n        values: x\n", "must be a mapping"},
		{"values in flow style", "      - channel: HmIP-RF/A:1\n        values: {A: 1}\n", "flow style"},
		{"empty", "      - channel: HmIP-RF/A:1\n        values: {}\n", "1 to 32 datapoints"},
		{"too many", "      - channel: HmIP-RF/A:1\n        values:\n" + many, "1 to 32 datapoints"},
		{"a name that is none", "      - channel: HmIP-RF/A:1\n        values:\n          \"A B\": 1\n", "is not a datapoint's name"},
		{"twice", "      - channel: HmIP-RF/A:1\n        values:\n          A: 1\n          A: 2\n", "twice"},
		{"a list as a value", "      - channel: HmIP-RF/A:1\n        values:\n          A:\n            - 1\n", "single value"},
		{"null", "      - channel: HmIP-RF/A:1\n        values:\n          A: null\n", "must be true, false, a number or text"},
		{"values elsewhere", "      - variable: v\n        to: true\n        values:\n          A: 1\n", "only a \"channel\" activity has \"values\""},
		{"to on a channel", "      - channel: HmIP-RF/A:1\n        values:\n          A: 1\n        to: 1\n", "only \"set\" and \"variable\""},
		{"on_for on a channel", "      - channel: HmIP-RF/A:1\n        values:\n          A: 1\n        on_for: 5s\n", "only a \"set\" activity"},
	} {
		_, err := DecodeProgram("p.yaml", "p", []byte(prog("", c.action)))
		var e *Error
		if !errors.As(err, &e) || !strings.Contains(e.Msg, c.msg) || e.Line == 0 {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}
