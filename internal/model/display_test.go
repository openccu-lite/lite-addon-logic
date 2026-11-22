package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const shown = `name: Anzeige
rules:
  - when:
      all:
        - all:
            - variable: v
              is: true
              trigger: change
    then:
      - display: HmIP-RF/000A1B2C3D4E11:3  # Wandtaster Flur
        lines:
          - text: "Fenster {var:v}"
            icon: WINDOW_OPEN
            align: LEFT
            color: WHITE
            background: BLACK
          - {}
          - text: "Tür & Tor ä"
          - icon: BELL
        sound: EVENT
        repetitions: 2
        interval: 10
        after: 5s
      - display: HmIP-RF/000A1B2C3D4E11:3
        sound: ERROR
      - display: HmIP-RF/000A1B2C3D4E11:3
        lines:
          - text: "{value}"
            align: CENTER
        note: der Wert
`

func oneName(r Ref) string {
	if r.Address == "000A1B2C3D4E11:3" && r.Param == "" {
		return "Wandtaster Flur"
	}
	return ""
}

// A display activity is written and read back the same, says what it targets, what it names
// and the text the device takes.
func TestDisplayRoundTrip(t *testing.T) {
	p, err := DecodeProgram("anzeige.yaml", "anzeige", []byte(shown))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(shown, "  # Wandtaster Flur", "", 1)
	if got := string(EncodeProgram(p, nil)); got != want {
		t.Fatalf("written:\n%s", got)
	}
	if got := string(EncodeProgram(p, oneName)); !strings.Contains(got, "- display: HmIP-RF/000A1B2C3D4E11:3  # Wandtaster Flur\n") {
		t.Errorf("the name as a comment:\n%s", got)
	}
	then := p.Rules[0].Then
	d := then[0].Display
	if then[0].Kind != ActionDisplay || then[0].Channel != "HmIP-RF/000A1B2C3D4E11:3" || len(d.Lines) != 4 || d.Sound != "EVENT" || d.Repetitions != 2 || d.Interval != 10 {
		t.Fatalf("display %+v", then[0])
	}
	if d.Lines[0] != (DisplayLine{Text: "Fenster {var:v}", Icon: "WINDOW_OPEN", Align: "LEFT", Color: "WHITE", Background: "BLACK"}) || !d.Lines[1].Empty() || d.Lines[3].Empty() {
		t.Errorf("lines %+v", d.Lines)
	}
	if got := then[0].Target(); got != "ch:HmIP-RF/000A1B2C3D4E11:3" {
		t.Errorf("target %s", got)
	}
	if got := d.Placeholders(); !reflect.DeepEqual(got, []string{"var:v"}) {
		t.Errorf("placeholders %v", got)
	}
	fill := func(name string) string {
		return map[string]string{"var:v": "offen, {ja}=1", "value": "21.5 °C und noch viel mehr Text"}[name]
	}
	// the CCU's text: a block per line with something in it, the last with DDC=true, then the sound;
	// what a value brings that the display cannot show is a blank; ä, & as the device's characters
	if got, want := d.Combined(fill), "{DDBC=BLACK,DDTC=WHITE,DDI=24,DDA=LEFT,DDS=Fenster offen,DDID=1},{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=T³r ] Tor ²,DDID=3},{DDBC=WHITE,DDTC=BLACK,DDI=30,DDA=CENTER,DDS=,DDID=4,DDC=true},{R=2,IN=10,ANS=6}"; got != want {
		t.Errorf("combined:\n%s\nwant\n%s", got, want)
	}
	if got := then[1].Display.Combined(fill); got != "{R=0,IN=5,ANS=7}" {
		t.Errorf("a sound alone: %s", got)
	}
	// cut to the display's width
	if got := then[2].Display.Combined(fill); got != "{DDBC=WHITE,DDTC=BLACK,DDI=0,DDA=CENTER,DDS=21.5  C und noch,DDID=1,DDC=true}" {
		t.Errorf("a long value: %s", got)
	}
	if got := (DisplayLine{Text: "a{value}"}).Shown(func(string) string { return "   " }); got != "a" {
		t.Errorf("blanks at the end: %q", got)
	}
	// a copy shares nothing
	c := p.Clone()
	c.Rules[0].Then[0].Display.Lines[0].Text = "x"
	if d.Lines[0].Text == "x" {
		t.Error("the copy shares the lines")
	}
	// one without its display is written so that the reader says what is missing
	bare := &Program{Key: "p", Name: "P", Rules: []Rule{{When: p.Rules[0].When, Then: []Action{{Kind: ActionDisplay, Channel: "HmIP-RF/A:1"}}}}}
	if _, err := bare.Check(); err == nil || !strings.Contains(err.Error(), "shows a line or plays a sound") {
		t.Errorf("a display with nothing: %v", err)
	}
	// the set: a placeholder's variable must exist, and the variable is in use
	s := NewSet()
	s.Programs["anzeige"] = p
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

func TestDisplayErrors(t *testing.T) {
	head := "      - display: HmIP-RF/A:3\n"
	for _, c := range []struct{ name, action, msg string }{
		{"channel not text", "      - display: 5\n        sound: EVENT\n", "must be text"},
		{"no channel", "      - display: \"Flur\"\n        sound: EVENT\n", "display:"},
		{"nothing", head, "shows a line or plays a sound"},
		{"only empty lines", head + "        lines:\n          - {}\n", "the last line listed has nothing in it"},
		{"an empty line at the end", head + "        lines:\n          - text: a\n          - {}\n", "the last line listed has nothing in it"},
		{"six lines", head + "        lines:\n" + strings.Repeat("          - text: a\n", 6), "a display has 5 lines"},
		{"lines not a list", head + "        lines: x\n", "must be a list"},
		{"line not a mapping", head + "        lines:\n          - x\n", "must be a mapping"},
		{"line key", head + "        lines:\n          - txt: a\n", "unknown key"},
		{"text not text", head + "        lines:\n          - text: 5\n", "must be text"},
		{"icon not text", head + "        lines:\n          - icon: 5\n", "must be text"},
		{"align not text", head + "        lines:\n          - text: a\n            align: 5\n", "must be text"},
		{"color not text", head + "        lines:\n          - text: a\n            color: 5\n", "must be text"},
		{"background not text", head + "        lines:\n          - text: a\n            background: 5\n", "must be text"},
		{"long text", head + "        lines:\n          - text: \"12345678901234567\"\n", "line 1: the display shows 16 characters"},
		{"long stored text", head + "        lines:\n          - text: a\n          - text: \"" + strings.Repeat("{value}", 10) + "\"\n", "line 2: the text is longer than 64"},
		{"a comma", head + "        lines:\n          - text: \"a, b\"\n", "the display cannot show \",\""},
		{"a brace", head + "        lines:\n          - text: \"{wert}\"\n", "the display cannot show \"{\""},
		{"an equals sign", head + "        lines:\n          - text: \"a=b\"\n", "the display cannot show \"=\""},
		{"a euro", head + "        lines:\n          - text: \"5 €\"\n", "the display cannot show \"€\""},
		{"a secret", head + "        lines:\n          - text: \"{secret:pin}\"\n", "a secret is not shown"},
		{"icon", head + "        lines:\n          - icon: HOUSE\n", "is not an icon of the display"},
		{"no icon by name", head + "        lines:\n          - text: a\n            icon: NO_ICON\n", "is not an icon of the display"},
		{"align", head + "        lines:\n          - text: a\n            align: MIDDLE\n", "align is LEFT, CENTER or RIGHT"},
		{"color", head + "        lines:\n          - text: a\n            color: RED\n", "a colour is BLACK or WHITE"},
		{"background", head + "        lines:\n          - text: a\n            background: RED\n", "a colour is BLACK or WHITE"},
		{"sound not text", head + "        sound: 5\n", "must be text"},
		{"sound", head + "        sound: GONG\n", "is not a signal tone"},
		{"repetitions without a sound", head + "        lines:\n          - text: a\n        repetitions: 2\n", "belong to a sound"},
		{"interval without a sound", head + "        lines:\n          - text: a\n        interval: 10\n", "belong to a sound"},
		{"repetitions", head + "        sound: EVENT\n        repetitions: 16\n", "whole number from 0 to 15"},
		{"interval", head + "        sound: EVENT\n        interval: 4\n", "whole number from 5 to 80"},
		{"interval between steps", head + "        sound: EVENT\n        interval: 12\n", "in steps of 5"},
		{"lines elsewhere", "      - variable: v\n        to: true\n        lines: []\n", "only a \"display\" activity has \"lines\""},
		{"sound elsewhere", "      - variable: v\n        to: true\n        sound: EVENT\n", "only a \"display\" activity has \"sound\""},
		{"two kinds", head + "        channel: HmIP-RF/A:3\n", "exactly one of"},
		{"to on a display", head + "        sound: EVENT\n        to: true\n", "only \"set\" and \"variable\""},
	} {
		_, err := DecodeProgram("p.yaml", "p", []byte(prog("", c.action)))
		var e *Error
		if !errors.As(err, &e) || !strings.Contains(e.Msg, c.msg) || e.Line == 0 {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// what the reader's ranges do not reach
	for _, d := range []Display{{Sound: "EVENT", Repetitions: -1}, {Sound: "EVENT", Repetitions: 16}} {
		if got := d.Check(); !strings.Contains(got, "repetitions: 0 to 14") {
			t.Errorf("%+v: %s", d, got)
		}
	}
	if (Action{Kind: ActionDisplay}).Target() != "ch:" {
		t.Error("the target of a display without a channel")
	}
}

// The lists are the device's own: the order of its description decides the number that is sent.
func TestDisplayLists(t *testing.T) {
	if len(DisplayIcons) != 32 || DisplayIcons[0] != "NO_ICON" || DisplayIcons[31] != "CLOCK" || len(DisplaySounds) != 8 {
		t.Errorf("icons %d, sounds %d", len(DisplayIcons), len(DisplaySounds))
	}
	if index(DisplayIcons, "nope") != -1 {
		t.Error("an icon that is not one")
	}
}
