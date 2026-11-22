package regadom

import "testing"

// The regadom declares iso-8859-1 and is mixed: eQ-3's own strings are Latin-1, a name a user
// typed on a current firmware is UTF-8 in the same file. Both spellings have to come out as the
// same name. (occulited's test, with the parser.)
func TestFixMojibake(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		// what a Latin-1 decode makes of UTF-8 bytes: undone
		{"BÃ¼ro", "Büro"},
		{"KÃ¼che", "Küche"},
		// the Latin-1 decode of UTF-8 "Ö" and "ß" is C3 + a C1 control, not the CP1252 glyphs a
		// terminal shows for them - write the runes, not what they look like
		{"Ã\u0096lheizung", "Ölheizung"},
		{"StraÃ\u009fe", "Straße"},
		// a name that really was Latin-1: the decode was right, leave it
		{"Büro", "Büro"},
		{"°C", "°C"},
		{"Kinderzimmer 1", "Kinderzimmer 1"},
		{"", ""},
		// already beyond Latin-1, so not something a Latin-1 decode produced
		{"Küche ☂", "Küche ☂"},
	} {
		if got := fixMojibake(c.in); got != c.want {
			t.Errorf("fixMojibake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A CCU's own names go through cleanName, so one stray character cannot cost a whole import.
func TestCleanName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Wohnzimmer", "Wohnzimmer"},
		{"BÃ¼ro", "Büro"},           // the encoding repair still happens
		{"Bad \u0000 1", "Bad   1"}, // a NUL becomes a space
		{"Flur\u001bX", "Flur X"},   // so does an ESC
		{"a\tb", "a b"},             // and a tab
		{"StraÃ\u009fe", "Straße"},  // repaired first, then cleaned
	} {
		if got := cleanName(c.in); got != c.want {
			t.Errorf("cleanName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
