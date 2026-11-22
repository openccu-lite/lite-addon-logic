package schedule

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

func near(a, b time.Time, tolerance time.Duration) bool {
	diff := a.Sub(b)
	return diff <= tolerance && diff >= -tolerance
}

// Sunrise and sunset against reference times (sunrise-sunset.org, UTC, fetched 2026-11-22):
// both hemispheres, the equator, high latitudes. A minute's difference is what the formulas
// are good for; close to the polar day, where the sun grazes the horizon, it is more.
func TestSunAgainstReference(t *testing.T) {
	for _, c := range []struct {
		name      string
		lat, lon  float64
		y, m, d   int
		rise, set string
		tolerance time.Duration
	}{
		{"Berlin, winter", 52.52, 13.405, 2017, 12, 1, "2017-12-01T06:54:40Z", "2017-12-01T14:56:15Z", time.Minute},
		{"Berlin, summer", 52.52, 13.405, 2024, 6, 15, "2024-06-15T02:42:38Z", "2024-06-15T19:31:21Z", time.Minute},
		{"New York, summer", 40.71, -74.01, 2024, 6, 15, "2024-06-15T09:24:15Z", "2024-06-16T00:29:10Z", time.Minute},
		{"Sydney, its summer", -33.8688, 151.2093, 2026, 12, 21, "2026-12-20T18:40:40Z", "2026-12-21T09:05:22Z", time.Minute},
		{"Quito, the equinox", -0.18, -78.47, 2026, 3, 20, "2026-03-20T11:17:52Z", "2026-03-20T23:24:40Z", time.Minute},
		{"Reykjavik, midsummer", 64.15, -21.94, 2026, 6, 21, "2026-06-21T02:54:51Z", "2026-06-22T00:04:20Z", 3 * time.Minute},
		{"Ushuaia, its midwinter", -54.8, -68.3, 2026, 6, 21, "2026-06-21T12:58:43Z", "2026-06-21T20:11:24Z", time.Minute},
		{"Tromsø, days before the midnight sun", 69.65, 18.96, 2026, 5, 16, "2026-05-15T23:24:32Z", "2026-05-16T21:56:32Z", 8 * time.Minute},
		{"Tromsø, days after the midnight sun", 69.65, 18.96, 2026, 7, 27, "2026-07-26T23:35:22Z", "2026-07-27T22:06:03Z", 8 * time.Minute},
	} {
		sun := Sun(c.y, time.Month(c.m), c.d, Position{Lat: c.lat, Lon: c.lon, Known: true})
		rise, set, hasRise, hasSet := sun.Rise, sun.Set, sun.HasRise, sun.HasSet
		wantRise, _ := time.Parse(time.RFC3339, c.rise)
		wantSet, _ := time.Parse(time.RFC3339, c.set)
		if !hasRise || !hasSet || !near(rise, wantRise, c.tolerance) || !near(set, wantSet, c.tolerance) {
			t.Errorf("%s: rise %s set %s (%v %v), want %s and %s within %v", c.name, rise.UTC().Format(time.RFC3339), set.UTC().Format(time.RFC3339), hasRise, hasSet, c.rise, c.set, c.tolerance)
		}
	}
}

// The times the real ReGaHss switched "tagsüber" and "nachts" at in OpenCCU's ReGaHss-Test
// (07a, Europe/Berlin): ours may differ from ReGa's by a minute or two (Q-32) - three here. On
// the day the clock changes in autumn ReGa itself is seven minutes off the almanac (sunrise
// 06:52, sunset 16:46 on 2019-10-27): those two are held to eight.
func TestSunAgainstReGa(t *testing.T) {
	l := zone(t, "Europe/Berlin")
	for _, c := range []struct {
		astro, after, want string
		tolerance          time.Duration
	}{
		{"day", "2017-12-01 07:40:00 CET", "2017-12-01 07:56:00 CET", 3 * time.Minute},
		{"night", "2017-12-01 15:40:00 CET", "2017-12-01 15:56:00 CET", 3 * time.Minute},
		{"day", "2019-03-31 06:30:00 CEST", "2019-03-31 06:42:00 CEST", 3 * time.Minute},
		{"night", "2019-03-31 19:20:00 CEST", "2019-03-31 19:36:00 CEST", 3 * time.Minute},
		{"day", "2019-04-01 06:20:00 CEST", "2019-04-01 06:39:00 CEST", 3 * time.Minute},
		{"night", "2019-04-01 19:20:00 CEST", "2019-04-01 19:39:00 CEST", 3 * time.Minute},
		{"day", "2019-10-27 06:40:00 CET", "2019-10-27 07:00:00 CET", 8 * time.Minute},
		{"night", "2019-10-27 16:20:00 CET", "2019-10-27 16:40:00 CET", 8 * time.Minute},
	} {
		m := compile(t, model.TimeModule{Kind: "daily", Astro: c.astro}, l, berlin)
		e, ok := m.Next(at(t, l, c.after))
		if !ok || e.Kind != Begin || !near(e.At, at(t, l, c.want), c.tolerance) {
			t.Errorf("%s after %s: %s %v, want %s within %v", c.astro, c.after, e.At.Format(stamp), e.Kind, c.want, c.tolerance)
		}
	}
}

// minute cuts an edge to the minute: an astro test names times to the minute it trusts.
func minutes(edges []Edge) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		out[i] = e.At.Format("2006-01-02 15:04 MST") + " " + e.Kind.String()
	}
	return out
}

func TestAstroModules(t *testing.T) {
	l := zone(t, "Europe/Berlin")
	// Berlin, 2026-06-15: sunrise 04:43 CEST, sunset 21:31 CEST (the formulas'; the tests
	// above tie them to the references)
	rise, set, nextRise := Sun(2026, 6, 15, berlin).Rise.In(l), Sun(2026, 6, 15, berlin).Set.In(l), Sun(2026, 6, 16, berlin).Rise.In(l)
	f := func(t time.Time, k EdgeKind) string { return t.Format(stamp) + " " + k.String() }
	midnight, noon := at(t, l, "2026-06-15 00:00:00 CEST"), at(t, l, "2026-06-15 12:00:00 CEST")
	for _, c := range []struct {
		name string
		def  model.TimeModule
		want []string
	}{
		{"day: sunrise to sunset", model.TimeModule{Kind: "daily", Astro: "day"}, []string{f(rise, Begin), f(set, End)}},
		{"sunrise, as an old CCU stored it", model.TimeModule{Kind: "daily", Astro: "sunrise"}, []string{f(rise, Begin), f(set, End)}},
		{"night: sunset to the next sunrise", model.TimeModule{Kind: "daily", Astro: "night"}, []string{f(set, Begin), f(nextRise, End)}},
		{"sunset, as an old CCU stored it", model.TimeModule{Kind: "daily", Astro: "sunset"}, []string{f(set, Begin), f(nextRise, End)}},
		{"30 minutes after sunrise, to sunset", model.TimeModule{Kind: "daily", Astro: "day", Offset: d("30min")}, []string{f(rise.Add(30*time.Minute), Begin), f(set, End)}},
		{"30 minutes before sunrise, to sunset", model.TimeModule{Kind: "daily", Astro: "before_sunrise", Offset: d("30min")}, []string{f(rise.Add(-30*time.Minute), Begin), f(set, End)}},
		{"30 minutes before sunset, for an hour (the fixture's Rollläden abends)", model.TimeModule{Kind: "daily", Astro: "before_sunset", Offset: d("30min"), Duration: d("1h")},
			[]string{f(set.Add(-30*time.Minute), Begin), f(set.Add(30*time.Minute), End)}},
		{"an hour after sunset, to sunrise", model.TimeModule{Kind: "daily", Astro: "night", Offset: d("1h")}, []string{f(set.Add(time.Hour), Begin), f(nextRise, End)}},
		{"after sunrise for ten minutes", model.TimeModule{Kind: "daily", Astro: "day", Duration: d("10min")}, []string{f(rise, Begin), f(rise.Add(10*time.Minute), End)}},
		{"an interval from sunset for an hour", model.TimeModule{Kind: "interval", Astro: "night", Duration: d("1h"), Interval: d("25min")},
			[]string{f(set, Begin), f(set.Add(25*time.Minute), Repeat), f(set.Add(50*time.Minute), Repeat), f(set.Add(time.Hour), End)}},
		{"an offset beyond the sunset: not that day", model.TimeModule{Kind: "daily", Astro: "day", Offset: d("20h")}, nil},
	} {
		m := compile(t, c.def, l, berlin)
		after := midnight
		if !m.byDay() {
			after = noon // (before noon the night that began the evening before ends first)
		}
		var got []string
		for _, e := range m.Upcoming(after, len(c.want)+1) {
			if e.At.Before(midnight.Add(34 * time.Hour)) {
				got = append(got, f(e.At, e.Kind))
			}
		}
		if len(c.want) > 0 {
			got = got[:min(len(got), len(c.want))]
		}
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("%s:\n got %v\nwant %v", c.name, got, c.want)
		}
	}
	// the truth: inside the range, begin included, end not
	day := compile(t, model.TimeModule{Kind: "daily", Astro: "day"}, l, berlin)
	night := compile(t, model.TimeModule{Kind: "daily", Astro: "night"}, l, berlin)
	for _, c := range []struct {
		when       time.Time
		day, night bool
	}{
		{rise.Add(-time.Second), false, true},
		{rise, true, false},
		{set.Add(-time.Second), true, false},
		{set, false, true},
		{at(t, l, "2026-06-15 02:00:00 CEST"), false, true},
		{at(t, l, "2026-06-15 13:00:00 CEST"), true, false},
	} {
		if day.Active(c.when) != c.day || night.Active(c.when) != c.night {
			t.Errorf("at %s: day %v night %v, want %v %v", c.when.Format(stamp), day.Active(c.when), night.Active(c.when), c.day, c.night)
		}
	}
	// weekdays are the day the range begins on: Saturday's night ends on Sunday morning
	sat := compile(t, model.TimeModule{Kind: "weekly", Astro: "night", Weekdays: []string{"sat"}}, l, berlin)
	got := minutes(sat.Upcoming(at(t, l, "2026-11-20 12:00:00 CET"), 3))
	want := []string{"2026-11-21 16:05 CET begin", "2026-11-22 07:40 CET end", "2026-11-28 15:58 CET begin"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Saturday night: %v, want %v", got, want)
	}
	if !sat.Active(at(t, l, "2026-11-22 05:00:00 CET")) || sat.Active(at(t, l, "2026-11-22 17:00:00 CET")) {
		t.Error("Saturday night is true on Sunday at 05:00 and false on Sunday evening")
	}
}

// An astro point fires once and is true at its instant only; a twilight moves the sun's events
// (the civil dawn and dusk of Berlin on 2026-06-15 against timeanddate's 03:53 and 22:21).
func TestAstroPointAndTwilight(t *testing.T) {
	l := zone(t, "Europe/Berlin")
	rise, set := Sun(2026, 6, 15, berlin).Rise.In(l), Sun(2026, 6, 15, berlin).Set.In(l)
	midnight := at(t, l, "2026-06-15 00:00:00 CEST")
	for _, c := range []struct {
		name string
		def  model.TimeModule
		want time.Time
	}{
		{"at sunrise", model.TimeModule{Kind: "daily", Astro: "sunrise", Point: true}, rise},
		{"20 minutes after sunrise", model.TimeModule{Kind: "daily", Astro: "day", Offset: d("20min"), Point: true}, rise.Add(20 * time.Minute)},
		{"at sunset", model.TimeModule{Kind: "daily", Astro: "sunset", Point: true}, set},
		{"30 minutes before sunset", model.TimeModule{Kind: "daily", Astro: "before_sunset", Offset: d("30min"), Point: true}, set.Add(-30 * time.Minute)},
		{"an hour after sunset", model.TimeModule{Kind: "daily", Astro: "night", Offset: d("1h"), Point: true}, set.Add(time.Hour)},
	} {
		m := compile(t, c.def, l, berlin)
		got := m.Upcoming(midnight, 2)
		if len(got) != 2 || got[0].Kind != Point || !got[0].At.Equal(c.want) || got[1].Kind != Point || got[1].At.Sub(c.want) < 23*time.Hour {
			t.Errorf("%s: %v, want a point at %s and the next a day later", c.name, minutes(got), c.want.Format(stamp))
		}
		if !m.Active(c.want) || m.Active(c.want.Add(time.Second)) || m.Active(c.want.Add(-time.Second)) || m.Active(c.want.Add(time.Hour)) {
			t.Errorf("%s: true at its instant only", c.name)
		}
	}
	dawn := compile(t, model.TimeModule{Kind: "daily", Astro: "sunrise", Point: true, Twilight: "civil"}, l, berlin)
	dusk := compile(t, model.TimeModule{Kind: "daily", Astro: "sunset", Point: true, Twilight: "civil"}, l, berlin)
	e1, _ := dawn.Next(midnight)
	e2, _ := dusk.Next(midnight)
	if !near(e1.At, at(t, l, "2026-06-15 03:53:00 CEST"), 2*time.Minute) || !near(e2.At, at(t, l, "2026-06-15 22:21:00 CEST"), 2*time.Minute) {
		t.Errorf("civil dawn %s, dusk %s", e1.At.Format(stamp), e2.At.Format(stamp))
	}
	// a range between the twilights is longer than the day; the deeper twilights are earlier
	// still (in Berlin's June the sun never gets 18 degrees below the horizon: no such dawn)
	bright := compile(t, model.TimeModule{Kind: "daily", Astro: "day", Twilight: "civil"}, l, berlin)
	if got := bright.Upcoming(midnight, 2); len(got) != 2 || !got[0].At.Equal(e1.At) || got[0].Kind != Begin || !got[1].At.Equal(e2.At) || got[1].Kind != End {
		t.Errorf("day between the civil twilights: %v", minutes(got))
	}
	nautical := compile(t, model.TimeModule{Kind: "daily", Astro: "sunrise", Point: true, Twilight: "nautical"}, l, berlin)
	if e, ok := nautical.Next(midnight); !ok || !e.At.Before(e1.At) {
		t.Errorf("the nautical dawn is before the civil one: %s", e.At.Format(stamp))
	}
	astronomical := compile(t, model.TimeModule{Kind: "daily", Astro: "sunrise", Point: true, Twilight: "astronomical", Until: "2026-06-20"}, l, berlin)
	if e, ok := astronomical.Next(midnight); ok {
		t.Errorf("no astronomical dawn in Berlin's June, got %s", e.At.Format(stamp))
	}
}

// Without a position an astro module never fires and is never true.
func TestAstroWithoutPosition(t *testing.T) {
	l := zone(t, "Europe/Berlin")
	m := compile(t, model.TimeModule{Kind: "daily", Astro: "day"}, l, Position{})
	if !m.NeedsPosition() {
		t.Error("NeedsPosition")
	}
	if e, ok := m.Next(at(t, l, "2026-06-15 00:00:00 CEST")); ok {
		t.Errorf("fires at %v", e)
	}
	if m.Active(at(t, l, "2026-06-15 13:00:00 CEST")) {
		t.Error("true at noon")
	}
	if compile(t, daily("06:30"), l, Position{}).NeedsPosition() || compile(t, model.TimeModule{Kind: "daily", Astro: "day"}, l, berlin).NeedsPosition() {
		t.Error("NeedsPosition for a clock time, or with a position")
	}
}

// Beyond the polar circles: Tromsø (69.65 N) has no sunset from about 18 May to 25 July and no
// sunrise from about 27 November to 15 January; McMurdo (77.85 S) the other way round.
func TestPolar(t *testing.T) {
	tromso := Position{Lat: 69.65, Lon: 18.96, Known: true}
	l := zone(t, "Europe/Oslo")
	if s := Sun(2026, 6, 15, tromso); s.HasRise || s.HasSet || !s.Up {
		t.Error("Tromsø in June: the sun neither rises nor sets, it stays up")
	}
	if s := Sun(2026, 12, 15, tromso); s.HasRise || s.HasSet || s.Up {
		t.Error("Tromsø in December: the sun neither rises nor sets, it stays down")
	}
	day := compile(t, model.TimeModule{Kind: "daily", Astro: "day"}, l, tromso)
	night := compile(t, model.TimeModule{Kind: "daily", Astro: "night"}, l, tromso)

	// the midnight sun: "day" began with the last sunrise and stays true until the sun sets again
	if !day.Active(at(t, l, "2026-06-15 01:00:00 CEST")) || night.Active(at(t, l, "2026-06-15 01:00:00 CEST")) {
		t.Error("midnight sun: day is true and night false, also at one in the morning")
	}
	e, ok := day.Next(at(t, l, "2026-06-15 12:00:00 CEST"))
	if !ok || e.Kind != End || e.At.Before(at(t, l, "2026-07-20 00:00:00 CEST")) || e.At.After(at(t, l, "2026-07-31 00:00:00 CEST")) {
		t.Errorf("midnight sun: day ends with the first sunset, late in July; got %s %v %v", e.At.Format(stamp), e.Kind, ok)
	}
	e, ok = night.Next(at(t, l, "2026-06-15 12:00:00 CEST"))
	if !ok || e.Kind != Begin || e.At.Before(at(t, l, "2026-07-20 00:00:00 CEST")) || e.At.After(at(t, l, "2026-07-31 00:00:00 CEST")) {
		t.Errorf("midnight sun: night begins again late in July; got %s %v %v", e.At.Format(stamp), e.Kind, ok)
	}
	// the polar night: "night" stays true, "day" false, and day begins again in January
	if day.Active(at(t, l, "2026-12-15 12:00:00 CET")) || !night.Active(at(t, l, "2026-12-15 12:00:00 CET")) {
		t.Error("polar night: night is true and day false, also at noon")
	}
	e, ok = day.Next(at(t, l, "2026-12-15 12:00:00 CET"))
	if !ok || e.Kind != Begin || e.At.Before(at(t, l, "2027-01-10 00:00:00 CET")) || e.At.After(at(t, l, "2027-01-20 00:00:00 CET")) {
		t.Errorf("polar night: day begins again in the middle of January; got %s %v %v", e.At.Format(stamp), e.Kind, ok)
	}
	e, ok = night.Next(at(t, l, "2026-12-15 12:00:00 CET"))
	if !ok || e.Kind != End || e.At.Before(at(t, l, "2027-01-10 00:00:00 CET")) || e.At.After(at(t, l, "2027-01-20 00:00:00 CET")) {
		t.Errorf("polar night: night ends in the middle of January; got %s %v %v", e.At.Format(stamp), e.Kind, ok)
	}
	// a range with a length does not wait for the sun: it simply does not begin
	short := compile(t, model.TimeModule{Kind: "daily", Astro: "before_sunset", Offset: d("30min"), Duration: d("1h")}, l, tromso)
	if short.Active(at(t, l, "2026-06-15 21:00:00 CEST")) {
		t.Error("midnight sun: before sunset for an hour is not true")
	}
	if e, ok := short.Next(at(t, l, "2026-06-15 12:00:00 CEST")); !ok || e.At.Before(at(t, l, "2026-07-20 00:00:00 CEST")) {
		t.Errorf("midnight sun: before sunset fires again late in July; got %s %v", e.At.Format(stamp), ok)
	}

	// the southern polar day is in December
	mcmurdo := Position{Lat: -77.85, Lon: 166.67, Known: true}
	nz := zone(t, "Antarctica/McMurdo")
	south := compile(t, model.TimeModule{Kind: "daily", Astro: "day"}, nz, mcmurdo)
	if !south.Active(at(t, nz, "2026-12-21 00:30:00 NZDT")) {
		t.Error("McMurdo in December: day is true at half past midnight")
	}
	if south.Active(at(t, nz, "2026-06-21 12:00:00 NZST")) {
		t.Error("McMurdo in June: day is false at noon")
	}
	// exactly at the pole the formulas have no sunrise at all: nothing fires, nothing is true
	pole := compile(t, model.TimeModule{Kind: "daily", Astro: "day"}, time.UTC, Position{Lat: 90, Lon: 0, Known: true})
	if e, ok := pole.Next(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)); ok || pole.Active(time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("the pole: fires at %v", e)
	}
}

// The sun's position: in the south at solar noon, as high as the latitude and the season say;
// on the horizon at sunrise and sunset, in the north-east and the north-west in June; in the
// north at solar midnight; the same hold south of the equator, mirrored.
func TestSunPosition(t *testing.T) {
	day := Sun(2026, 6, 15, berlin)
	noon := day.Rise.Add(day.Set.Sub(day.Rise) / 2)
	az, el := SunPosition(noon, berlin)
	if math.Abs(az-180) > 1 || math.Abs(el-(90-52.5+23.3)) > 0.3 {
		t.Errorf("Berlin, solar noon in June: azimuth %.1f, elevation %.1f", az, el)
	}
	az, el = SunPosition(day.Rise, berlin)
	if math.Abs(el+0.833) > 0.2 || az < 45 || az > 55 {
		t.Errorf("Berlin, sunrise in June: azimuth %.1f, elevation %.1f", az, el)
	}
	az, el = SunPosition(day.Set, berlin)
	if math.Abs(el+0.833) > 0.2 || az < 305 || az > 315 {
		t.Errorf("Berlin, sunset in June: azimuth %.1f, elevation %.1f", az, el)
	}
	az, el = SunPosition(noon.Add(12*time.Hour), berlin)
	if (az > 2 && az < 358) || el > -13 || el < -15 {
		t.Errorf("Berlin, solar midnight in June: azimuth %.1f, elevation %.1f", az, el)
	}
	// Sydney in its summer: the sun passes through the north, high up
	sydney := Position{Lat: -33.8688, Lon: 151.2093, Known: true}
	d := Sun(2026, 12, 21, sydney)
	az, el = SunPosition(d.Rise.Add(d.Set.Sub(d.Rise)/2), sydney)
	if (az > 3 && az < 357) || math.Abs(el-(90-33.87+23.44)) > 0.3 {
		t.Errorf("Sydney, solar noon in December: azimuth %.1f, elevation %.1f", az, el)
	}
	// at the pole there is no direction: the answer is still a number
	az, el = SunPosition(noon, Position{Lat: 90, Lon: 0, Known: true})
	if az != 180 || math.Abs(el-23.3) > 0.3 {
		t.Errorf("the north pole in June: azimuth %.1f, elevation %.1f", az, el)
	}
}
