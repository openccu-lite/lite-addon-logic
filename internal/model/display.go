package model

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ActionDisplay writes up to five lines and an optional signal tone to a display channel
// (ACOUSTIC_DISPLAY_RECEIVER: the HmIP-WRCD), in one command: the text the channel takes in
// its COMBINED_PARAMETER, built by Display.Combined.
const ActionDisplay ActionKind = "display"

const (
	// MaxDisplayLines is how many lines the display has.
	MaxDisplayLines = 5
	// MaxDisplayText is how many characters of a line the display shows; what a text is with
	// its placeholders filled is cut there.
	MaxDisplayText = 16
	// MaxDisplayStored bounds a line's text as it is stored, placeholders and all.
	MaxDisplayStored = 64
)

// The lists of a display channel, in the order of the device's own description: an entry is
// stored by its name and sent as its number.
var (
	DisplayIcons = []string{"NO_ICON", "LAMP_OFF", "LAMP_ON", "PADLOCK_OPEN", "PADLOCK_CLOSED", "ERROR", "EVERYTHING_OKAY", "INFORMATION", "NEW_MESSAGE", "SERVICE_MESSAGE",
		"SUN", "MOON", "WIND", "CLOUD", "THUNDERSTORM", "DRIZZLE", "CLOUD_AND_MOOON", "RAIN", "SNOW", "CLOUD_AND_SUN", "CLOUD_SUN_AND_RAIN", "SNOWFLAKE", "RAINDROP", "FLAME",
		"WINDOW_OPEN", "SHUTTERS", "ECO", "PROTECTION_DEACTIVATED", "EXTERNAL_PROTECTION", "INTERNAL_PROTECTION", "BELL", "CLOCK"}
	DisplaySounds = []string{"LOW_BATTERY", "DISARMED", "INTERNALLY_ARMED", "EXTERNALLY_ARMED", "DELAYED_INTERNALLY_ARMED", "DELAYED_EXTERNALLY_ARMED", "EVENT", "ERROR"}
	DisplayAligns = []string{"LEFT", "CENTER", "RIGHT"}
	DisplayColors = []string{"WHITE", "BLACK"}
)

// Display is what an ActionDisplay shows: Lines[i] is line i+1 of the display.
type Display struct {
	Lines []DisplayLine `json:"lines,omitempty"`
	// Sound is the signal tone played with it (an entry of DisplaySounds), "" for none;
	// Repetitions how often it is repeated (0 to 14, 15: without end), Interval the seconds
	// between two (5 to 80 in steps of 5; 0 with no repetition asked for is 5).
	Sound       string `json:"sound,omitempty"`
	Repetitions int    `json:"repetitions,omitempty"`
	Interval    int    `json:"interval,omitempty"`
}

// DisplayLine is one line: a text (with the placeholders {value}, {var:<key>}, {program},
// {trigger} and {time}), an icon, where the text stands and the two colours. A line without
// text and icon is not sent: the display keeps what that line shows.
type DisplayLine struct {
	Text string `json:"text,omitempty"`
	// Icon is an entry of DisplayIcons, "" for none.
	Icon string `json:"icon,omitempty"`
	// Align is LEFT, CENTER or RIGHT; "" is CENTER.
	Align string `json:"align,omitempty"`
	// Color is the text's colour, Background the line's: BLACK or WHITE; "" is a black text
	// on a white line.
	Color      string `json:"color,omitempty"`
	Background string `json:"background,omitempty"`
}

// Empty says whether the line has neither a text nor an icon.
func (l DisplayLine) Empty() bool { return l.Text == "" && l.Icon == "" }

func index(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

// displayChars are what a display shows besides plain ASCII, and the characters that stand for
// them in the text the device takes (the CCU's own table).
var displayChars = map[rune]rune{'Ä': '[', 'Ö': '#', 'Ü': '$', 'ä': '²', 'ö': '|', 'ü': '³', 'ß': '_', '&': ']', '\'': 'µ'}

// displayRune says whether a line's text may hold r as it is stored.
func displayRune(r rune) bool {
	if _, ok := displayChars[r]; ok {
		return true
	}
	return r >= 0x20 && r <= 0x7e && !strings.ContainsRune(",={}", r)
}

// Check says what is wrong with a display, "" when it can be stored.
func (d *Display) Check() string {
	if len(d.Lines) > MaxDisplayLines {
		return fmt.Sprintf("a display has %d lines", MaxDisplayLines)
	}
	shown := false
	for i, l := range d.Lines {
		n := i + 1
		rest := placeholder.ReplaceAllStringFunc(l.Text, func(m string) string {
			if strings.HasPrefix(m, "{secret:") {
				return m
			}
			return ""
		})
		switch {
		case strings.Contains(rest, "{secret:"):
			return fmt.Sprintf("line %d: a secret is not shown on a display", n)
		case utf8.RuneCountInString(l.Text) > MaxDisplayStored:
			return fmt.Sprintf("line %d: the text is longer than %d characters", n, MaxDisplayStored)
		case utf8.RuneCountInString(rest) > MaxDisplayText:
			return fmt.Sprintf("line %d: the display shows %d characters of a line", n, MaxDisplayText)
		case l.Icon != "" && index(DisplayIcons, l.Icon) < 1:
			return fmt.Sprintf("line %d: %q is not an icon of the display", n, l.Icon)
		case l.Align != "" && index(DisplayAligns, l.Align) < 0:
			return fmt.Sprintf("line %d: align is LEFT, CENTER or RIGHT", n)
		case (l.Color != "" && index(DisplayColors, l.Color) < 0) || (l.Background != "" && index(DisplayColors, l.Background) < 0):
			return fmt.Sprintf("line %d: a colour is BLACK or WHITE", n)
		}
		for _, r := range rest {
			if !displayRune(r) {
				return fmt.Sprintf("line %d: the display cannot show %q", n, string(r))
			}
		}
		shown = shown || !l.Empty()
	}
	if n := len(d.Lines); n > 0 && d.Lines[n-1] == (DisplayLine{}) {
		return "the last line listed has nothing in it"
	}
	switch {
	case d.Sound != "" && index(DisplaySounds, d.Sound) < 0:
		return fmt.Sprintf("%q is not a signal tone of the display", d.Sound)
	case d.Sound == "" && (d.Repetitions != 0 || d.Interval != 0):
		return "repetitions and interval belong to a sound"
	case d.Repetitions < 0 || d.Repetitions > 15:
		return "repetitions: 0 to 14, or 15 for without end"
	case d.Interval != 0 && (d.Interval < 5 || d.Interval > 80 || d.Interval%5 != 0):
		return "interval: 5 to 80 seconds in steps of 5"
	case !shown && d.Sound == "":
		return "a display activity shows a line or plays a sound"
	}
	return ""
}

// Placeholders lists the placeholders' names in the lines' texts, each once, in their order.
func (d *Display) Placeholders() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range d.Lines {
		for _, m := range placeholder.FindAllStringSubmatch(l.Text, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

func (d *Display) uses(variable string) bool {
	for _, p := range d.Placeholders() {
		if p == "var:"+variable {
			return true
		}
	}
	return false
}

// Shown answers a line's text as the display gets it: the placeholders filled (fill answers
// the text for a placeholder's name), what the display cannot show replaced by a blank, cut
// to the display's width.
func (l DisplayLine) Shown(fill func(name string) string) string {
	s := Expand(l.Text, fill, func(v string) string { return v })
	out := make([]rune, 0, MaxDisplayText)
	for _, r := range s {
		if len(out) == MaxDisplayText {
			break
		}
		if !displayRune(r) {
			r = ' '
		}
		out = append(out, r)
	}
	return strings.TrimRight(string(out), " ")
}

// Combined builds the text a display channel takes in COMBINED_PARAMETER, the way the CCU
// writes it: one block per line that has a text or an icon -
// {DDBC=<background>,DDTC=<text colour>,DDI=<icon number>,DDA=<alignment>,DDS=<text>,DDID=<line>}
// - the last of them with DDC=true (show it now), then {R=…,IN=…,ANS=…} for a sound.
func (d *Display) Combined(fill func(name string) string) string {
	or := func(s, def string) string {
		if s == "" {
			return def
		}
		return s
	}
	var blocks []string
	for i, l := range d.Lines {
		if l.Empty() {
			continue
		}
		text := []rune(l.Shown(fill))
		for j, r := range text {
			if c, ok := displayChars[r]; ok {
				text[j] = c
			}
		}
		icon := 0
		if l.Icon != "" {
			icon = index(DisplayIcons, l.Icon)
		}
		blocks = append(blocks, fmt.Sprintf("{DDBC=%s,DDTC=%s,DDI=%d,DDA=%s,DDS=%s,DDID=%d}", or(l.Background, "WHITE"), or(l.Color, "BLACK"), icon, or(l.Align, "CENTER"), string(text), i+1))
	}
	if n := len(blocks); n > 0 {
		blocks[n-1] = strings.TrimSuffix(blocks[n-1], "}") + ",DDC=true}"
	}
	if d.Sound != "" {
		interval := d.Interval
		if interval == 0 {
			interval = 5
		}
		blocks = append(blocks, fmt.Sprintf("{R=%d,IN=%d,ANS=%d}", d.Repetitions, interval, index(DisplaySounds, d.Sound)))
	}
	return strings.Join(blocks, ",")
}
