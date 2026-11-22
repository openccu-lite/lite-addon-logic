package schedule

import (
	"math"
	"time"
)

// Position is where the system stands, for sunrise and sunset: degrees, north and east
// positive. Known is false until somebody set it; an astro time module then never fires.
type Position struct {
	Lat, Lon float64
	Known    bool
}

// zenith is the sun's distance from the zenith at sunrise and sunset: 90 degrees, plus the
// sun's radius and the refraction of the air at the horizon (the usual 50 arc minutes).
const zenith = 90.833

func rad(d float64) float64 { return d * math.Pi / 180 }
func deg(r float64) float64 { return r * 180 / math.Pi }

// solar answers, for an instant, the equation of time in minutes and the sun's declination in
// degrees - the NOAA solar calculator's formulas (after Meeus, Astronomical Algorithms).
func solar(t time.Time) (eqTime, decl float64) {
	jd := float64(t.Unix())/86400 + 2440587.5
	c := (jd - 2451545) / 36525
	l0 := math.Mod(280.46646+c*(36000.76983+c*0.0003032), 360)
	m := 357.52911 + c*(35999.05029-0.0001537*c)
	e := 0.016708634 - c*(0.000042037+0.0000001267*c)
	centre := math.Sin(rad(m))*(1.914602-c*(0.004817+0.000014*c)) +
		math.Sin(rad(2*m))*(0.019993-0.000101*c) +
		math.Sin(rad(3*m))*0.000289
	omega := 125.04 - 1934.136*c
	lambda := l0 + centre - 0.00569 - 0.00478*math.Sin(rad(omega))
	eps := 23 + (26+(21.448-c*(46.815+c*(0.00059-c*0.001813)))/60)/60 + 0.00256*math.Cos(rad(omega))
	decl = deg(math.Asin(math.Sin(rad(eps)) * math.Sin(rad(lambda))))
	y := math.Tan(rad(eps/2)) * math.Tan(rad(eps/2))
	eqTime = 4 * deg(y*math.Sin(2*rad(l0))-2*e*math.Sin(rad(m))+4*e*y*math.Sin(rad(m))*math.Cos(2*rad(l0))-
		0.5*y*y*math.Sin(4*rad(l0))-1.25*e*e*math.Sin(2*rad(m)))
	return eqTime, decl
}

// SunDay is the sun on one solar day at a position.
type SunDay struct {
	// Rise and Set are sunrise and sunset, to the second (the formulas are good for about a
	// minute); HasRise and HasSet are false when the sun does not cross the horizon then.
	Rise, Set       time.Time
	HasRise, HasSet bool
	// Up says the sun neither rises nor sets because it stays above the horizon (the midnight
	// sun); with neither event and Up false it stays below (the polar night).
	Up bool
}

// Zeniths of the twilights: the sun's centre 6, 12 and 18 degrees below the horizon.
var twilightZenith = map[string]float64{"civil": 96, "nautical": 102, "astronomical": 108}

// Sun answers sunrise and sunset of the solar day that belongs to the civil date y-m-d at the
// position.
func Sun(year int, month time.Month, day int, pos Position) SunDay {
	return SunAt(year, month, day, pos, zenith)
}

// SunAt is Sun for another distance of the sun from the zenith: with 96 degrees Rise is the
// civil dawn and Set the civil dusk.
func SunAt(year int, month time.Month, day int, pos Position, zenith float64) SunDay {
	base := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	event := func(sign float64) (at time.Time, has, above bool) {
		// first with the sun as it stands at noon, then twice more as it stands at the event
		at = base.Add(12*time.Hour - time.Duration(pos.Lon*4*float64(time.Minute)))
		for range 3 {
			eq, decl := solar(at)
			cosH := math.Cos(rad(zenith))/(math.Cos(rad(pos.Lat))*math.Cos(rad(decl))) - math.Tan(rad(pos.Lat))*math.Tan(rad(decl))
			if cosH > 1 || cosH < -1 {
				return time.Time{}, false, cosH < -1
			}
			minutes := 720 - 4*pos.Lon - eq + sign*4*deg(math.Acos(cosH))
			at = base.Add(time.Duration(minutes * float64(time.Minute)))
		}
		return at.Truncate(time.Second), true, false
	}
	var d SunDay
	var upRise, upSet bool
	d.Rise, d.HasRise, upRise = event(-1)
	d.Set, d.HasSet, upSet = event(1)
	d.Up = !d.HasRise && !d.HasSet && upRise && upSet
	return d
}

// SunPosition answers where the sun stands at an instant, seen from a position: the azimuth in
// degrees clockwise from north, and the elevation of its centre above the horizon (negative
// below it), without the refraction of the air - at sunrise and sunset it is about -0.8.
func SunPosition(t time.Time, pos Position) (azimuth, elevation float64) {
	eq, decl := solar(t)
	u := t.UTC()
	minutes := float64(u.Hour()*60+u.Minute()) + float64(u.Second())/60
	// the true solar time in minutes, and the hour angle: 0 when the sun is in the south
	solarTime := math.Mod(minutes+eq+4*pos.Lon+1440, 1440)
	hour := solarTime/4 - 180
	cosZ := math.Sin(rad(pos.Lat))*math.Sin(rad(decl)) + math.Cos(rad(pos.Lat))*math.Cos(rad(decl))*math.Cos(rad(hour))
	zen := math.Acos(math.Max(-1, math.Min(1, cosZ)))
	elevation = 90 - deg(zen)
	den := math.Cos(rad(pos.Lat)) * math.Sin(zen)
	if math.Abs(den) < 1e-9 {
		// at a pole, or the sun in the zenith: every direction is south (or north)
		return 180, elevation
	}
	a := deg(math.Acos(math.Max(-1, math.Min(1, (math.Sin(rad(pos.Lat))*math.Cos(zen)-math.Sin(rad(decl)))/den))))
	if hour > 0 {
		return math.Mod(a+180, 360), elevation
	}
	return math.Mod(540-a, 360), elevation
}
