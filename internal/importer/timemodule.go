package importer

import (
	"strconv"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

var timeKinds = map[int]string{0: "once", 1: "daily", 2: "weekly", 4: "interval", 5: "weekly", 6: "monthly", 7: "yearly", 8: "once", 9: "daily"}

var astroKinds = map[int]string{1: "sunrise", 2: "before_sunrise", 3: "day", 4: "sunset", 5: "before_sunset", 6: "night"}

func seconds(n int64) model.Duration { return model.Duration(time.Duration(n) * time.Second) }

// timeModule maps a time module the way the WebUI's dialog writes one (timemodule.htm,
// system.fn::saveTimeModule; OpenCCU's patch 0073 for the astro offsets):
//
//   - the time of day is the local clock of an epoch value on some base date ("2007-01-01
//     HH:MM:SS"); 0 with no duration and no astro kind is "ganztägig"; with an astro kind it is
//     the offset in plain seconds;
//   - "alle N Tage/Wochen" is the repetition value, not the period; a daily module carries the
//     weekdays 31 ("werktags") or 96 ("am Wochenende");
//   - monthly and yearly are "day <period>" or, with one weekday bit, "the <period>-th such
//     weekday"; the repetition value is "every n months" or the month;
//   - begin, end and the repetition count are the validity window.
//
// why is not empty when the module cannot be run as stored - a timer type this version does
// not know, or fields that contradict each other: the caller takes the program inactive.
// pattern says a field was left out because its value makes no sense for the kind.
func (m *imp) timeModule(c *regadom.Calendar) (t *model.TimeModule, why string, pattern bool) {
	placeholder := &model.TimeModule{Kind: "daily", At: "00:00"}
	kind, ok := timeKinds[c.Type]
	if !ok {
		return placeholder, "timer type " + strconv.Itoa(c.Type), false
	}
	t = &model.TimeModule{Kind: kind}
	at := time.Unix(c.Time, 0).In(m.loc)
	daySeconds := int64(at.Hour()*3600 + at.Minute()*60 + at.Second())
	if c.Time == 0 {
		daySeconds = 0
	}
	if c.Duration > 0 {
		t.Duration = seconds(c.Duration)
	}
	astro, isAstro := astroKinds[c.Sot]
	switch {
	case isAstro:
		offset := daySeconds
		if c.Time >= 0 && c.Time < 86400 {
			offset = c.Time // as the WebUI stores it; a date-based value is a script's doing
		}
		if offset != 0 && astro == "sunrise" {
			astro = "day"
		}
		if offset != 0 && astro == "sunset" {
			astro = "night"
		}
		t.Astro, t.Offset = astro, seconds(offset)
	case c.Time == 0 && c.Duration <= 0:
		pattern = c.Sot != 0
		t.AllDay = true
	default:
		pattern = c.Sot != 0
		t.At = time.Unix(daySeconds, 0).UTC().Format("15:04")
		if daySeconds%60 != 0 {
			t.At = time.Unix(daySeconds, 0).UTC().Format("15:04:05")
		}
	}
	every := func(n int64) {
		if n > 1 && n <= 10000 {
			t.Every = int(n)
		}
	}
	weekdays := func() {
		for i, d := range model.Weekdays {
			if c.Weekdays&(1<<i) != 0 {
				t.Weekdays = append(t.Weekdays, d)
			}
		}
	}
	// dayOrNth is the monthly and yearly choice: a day of the month, or the n-th of a weekday
	dayOrNth := func() {
		if c.Weekdays == 0 {
			if c.Period >= 1 && c.Period <= 31 {
				t.Day = int(c.Period)
			}
			return
		}
		weekdays()
		if c.Period >= 1 && c.Period <= 5 {
			t.Nth = int(c.Period)
		}
	}
	switch kind {
	case "interval":
		if c.Period <= 0 {
			// an interval without a period cannot be written; a daily module is what is left of it
			t.Kind, pattern = "daily", true
			break
		}
		t.Interval = seconds(c.Period)
	case "daily":
		every(c.RepVal)
		weekdays()
	case "weekly":
		weekdays()
		every(c.RepVal)
	case "monthly":
		dayOrNth()
		every(c.RepVal)
	case "yearly":
		dayOrNth()
		if c.RepVal >= 1 && c.RepVal <= 12 {
			t.Month = int(c.RepVal)
		}
	case "once":
		if c.TmRep > 0 {
			t.Date = time.Unix(c.TmRep, 0).In(m.loc).Format("2006-01-02")
		}
	}
	if c.Begin > 0 {
		t.From = m.date(c.Begin)
	}
	if c.End > 0 {
		t.Until = m.date(c.End)
	}
	if c.RepCnt > 0 && c.RepCnt <= 1000000 {
		t.Count = int(c.RepCnt)
	}
	if (t.Every > 1 || t.Count > 0) && t.From == "" {
		t.From = "1970-01-01" // ReGa's "from the start"
	}
	if err := t.Check(); err != nil {
		return placeholder, "time module: " + err.Error(), false
	}
	return t, "", pattern
}

// date writes a stored instant as a local date, with the time when it is not midnight.
func (m *imp) date(unix int64) string {
	d := time.Unix(unix, 0).In(m.loc)
	if d.Hour() == 0 && d.Minute() == 0 && d.Second() == 0 {
		return d.Format("2006-01-02")
	}
	return d.Format("2006-01-02 15:04")
}

// inDSTHour says whether a fixed time lies in the hour the clock skips or repeats.
func inDSTHour(t *model.TimeModule) bool {
	return t.Astro == "" && len(t.At) >= 2 && t.At[:2] == "02"
}
