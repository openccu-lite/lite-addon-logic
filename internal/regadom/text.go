package regadom

import (
	"strings"
	"unicode/utf8"
)

// fixMojibake undoes the ISO-8859-1 decode for a string that was UTF-8 to begin with.
//
// The regadom declares `encoding="iso-8859-1"` and is mixed: eQ-3's own strings really are
// Latin-1 (a unit is the single byte 0xB0 for "°C"), while a name a user typed on a current
// firmware is written as UTF-8 into the same file. Decoding the whole stream as Latin-1 - which is
// what the declaration asks for, and what the units need - turns "Büro" into "BÃ¼ro".
//
// The test is exact rather than heuristic in the direction that matters: only a string whose runes
// are all below U+0100 can have come out of a Latin-1 decode, and only if the bytes they stand for
// are valid UTF-8 with a multi-byte sequence in them was the source UTF-8. A genuinely Latin-1
// "°C" fails that test (a lone 0xB0 is not valid UTF-8) and is left alone, and so is anything
// ASCII. (Copied from occulited's internal/regaimport with its tests.)
func fixMojibake(s string) string {
	b := make([]byte, 0, len(s))
	high := false
	for _, r := range s {
		if r > 0xFF {
			return s // not something a Latin-1 decode produced
		}
		if r >= 0x80 {
			high = true
		}
		b = append(b, byte(r))
	}
	if !high || !utf8.Valid(b) || utf8.RuneCount(b) == len(b) {
		return s
	}
	return string(b)
}

// cleanName is what every name coming out of a CCU goes through: the encoding repair above, and
// then the control characters out. An import must not lose a program because its name carries a
// stray tab that nobody here can go and fix: forgiving on the way in from a foreign system.
func cleanName(s string) string {
	s = fixMojibake(s)
	if strings.IndexFunc(s, isControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return ' '
		}
		return r
	}, s)
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
