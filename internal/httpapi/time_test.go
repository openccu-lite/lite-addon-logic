package httpapi

import (
	"strconv"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

const timedProgram = `name: Morgens
active: true
rules:
  - when:
      any:
        - all:
            - time:
                kind: daily
                at: "06:30"
              trigger: update
        - all:
            - time:
                kind: daily
                astro: day
              trigger: check
    then: []
`

// The settings, a program's time lines and the development clock, through the API.
func TestTimeAPI(t *testing.T) {
	clock := engine.NewFakeClock(time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC))
	svc, err := service.Open(service.Options{Dir: t.TempDir(), Clock: clock, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e := &env{t: t, svc: svc, sys: &fakeSystem{}}
	e.srv = newServer(t, Options{Service: svc, System: e.sys, DevClock: clock})
	p, err := model.DecodeProgram("programs/morgens.yaml", "morgens", []byte(timedProgram))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProgram(p, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}

	// the settings: no location at first, and the astro line says so
	out := e.want(200, "", "GET", "settings", nil)
	if out["location"] != nil {
		t.Errorf("settings: %v", out)
	}
	ov := e.want(200, "", "GET", "overview", nil)
	state := ov["programs"].([]any)[0].(map[string]any)["state"].(map[string]any)
	if state["no_position"] != true || state["next_time"] != "2026-11-21T06:30:00Z" || ov["now"] != "2026-11-21T06:00:00Z" || ov["settings"].(map[string]any)["location"] != nil {
		t.Errorf("overview: %v %v %v", state, ov["now"], ov["settings"])
	}
	e.want(422, "invalid", "PUT", "settings", map[string]any{"location": map[string]any{"latitude": 95, "longitude": 13.405}})
	e.want(400, "invalid-body", "PUT", "settings", map[string]any{"position": 1})
	out = e.want(200, "", "PUT", "settings", map[string]any{"location": map[string]any{"latitude": 52.52, "longitude": 13.405}})
	if out["location"].(map[string]any)["latitude"] != 52.52 {
		t.Errorf("settings: %v", out)
	}
	ov = e.want(200, "", "GET", "overview", nil)
	if state := ov["programs"].([]any)[0].(map[string]any)["state"].(map[string]any); state["no_position"] != nil {
		t.Errorf("with a location: %v", state)
	}
	if svc.Settings().Location.Longitude != 13.405 || !svc.Engine().Position().Known {
		t.Error("the location did not reach the service and the engine")
	}
	// and it can be taken away again
	e.want(200, "", "PUT", "settings", map[string]any{})
	if svc.Settings().Location != nil || svc.Engine().Position().Known {
		t.Error("the location is still there")
	}
	e.want(200, "", "PUT", "settings", map[string]any{"location": map[string]any{"latitude": 52.52, "longitude": 13.405}})

	// the time lines
	e.want(404, "not-found", "GET", "programs/nothing/times", nil)
	out = e.want(200, "", "GET", "programs/morgens/times", nil)
	lines := out["lines"].([]any)
	first, second := lines[0].(map[string]any), lines[1].(map[string]any)
	if len(lines) != 2 || first["line"] != float64(1) || first["triggers"] != true || first["active"] != false || len(first["next"].([]any)) != 5 ||
		first["next"].([]any)[0].(map[string]any)["at"] != "2026-11-21T06:30:00Z" || first["last"] != nil || second["triggers"] != true {
		t.Errorf("times: %v", out)
	}

	// the development clock
	out = e.want(200, "", "GET", "dev/clock", nil)
	if out["now"] != "2026-11-21T06:00:00Z" {
		t.Errorf("the clock: %v", out)
	}
	e.want(400, "invalid-body", "POST", "dev/clock", map[string]any{"advance": "soon"})
	out = e.want(200, "", "POST", "dev/clock", map[string]any{"advance": "45min"})
	if out["now"] != "2026-11-21T06:45:00Z" {
		t.Errorf("the clock: %v", out)
	}
	out = e.want(200, "", "GET", "programs/morgens/times", nil)
	first = out["lines"].([]any)[0].(map[string]any)
	if first["last"].(map[string]any)["at"] != "2026-11-21T06:30:00Z" || first["last"].(map[string]any)["kind"] != "point" {
		t.Errorf("after 06:30: %v", first)
	}
	// (the second line is a sunrise on "nur prüfen": a time module triggers whatever its mode)
	if runs := svc.Engine().Runs("morgens"); len(runs) != 2 || runs[0].Trigger != "time:1:point" || runs[1].Trigger != "time:2:begin" {
		t.Errorf("runs: %+v", runs)
	}
	out = e.want(200, "", "POST", "dev/clock", map[string]any{"jump_seconds": -60})
	if out["now"] != "2026-11-21T06:44:00Z" {
		t.Errorf("the clock after a step back: %v", out)
	}

	// without a development clock there is no such route
	plain := newEnv(t)
	plain.want(404, "", "GET", "dev/clock", nil)
}

func TestImportLocation(t *testing.T) {
	if l := location(" 52.52", "13.405 "); l == nil || l.Latitude != 52.52 || l.Longitude != 13.405 {
		t.Errorf("a location: %+v", l)
	}
	for _, c := range [][2]string{{"", ""}, {"52.52", ""}, {"north", "13"}, {"99", "13"}, {"52", "400"}} {
		if l := location(c[0], c[1]); l != nil {
			t.Errorf("%v is a location: %+v", c, l)
		}
	}
}

const scheduledProgram = `name: Nacht
active: true
rules:
  - when:
      any:
        - all:
            - variable: regen
              is: true
              trigger: change
            - time:
                kind: daily
                at: "22:00"
                duration: 8h
              trigger: update
    then:
      - variable: zahl
        to: 5
        after: 10min
      - variable: zahl
        to: 7
        after: 20min
`

// The schedule: every time module line with its next and last firing and why one is not armed,
// and the delayed activities, which can be run now or cancelled.
func TestScheduleAPI(t *testing.T) {
	clock := engine.NewFakeClock(time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC))
	svc, err := service.Open(service.Options{Dir: t.TempDir(), Clock: clock, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e := &env{t: t, svc: svc, sys: &fakeSystem{}}
	e.srv = newServer(t, Options{Service: svc, System: e.sys, DevClock: clock})
	who := repo.Author{Name: "t"}
	for _, v := range []*model.Variable{{Name: "Regen", Type: model.VarBool, Visible: true}, {Name: "Zahl", Type: model.VarNumber, Visible: true}} {
		if _, err := svc.CreateVariable(v, nil, who); err != nil {
			t.Fatal(err)
		}
	}
	for key, text := range map[string]string{"morgens": timedProgram, "nacht": scheduledProgram} {
		p, err := model.DecodeProgram("programs/"+key+".yaml", key, []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateProgram(p, who); err != nil {
			t.Fatal(err)
		}
	}
	timers := func() map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, x := range e.want(200, "", "GET", "schedule", nil)["timers"].([]any) {
			m := x.(map[string]any)
			out[m["program"].(string)+"/"+strconv.Itoa(int(m["line"].(float64)))] = m
		}
		return out
	}
	got := timers()
	if len(got) != 3 {
		t.Fatalf("timers: %v", got)
	}
	if m := got["morgens/1"]; m["armed"] != true || m["next"].(map[string]any)["at"] != "2026-11-21T06:30:00Z" || m["time"].(map[string]any)["at"] != "06:30" || m["last"] != nil {
		t.Errorf("morgens/1: %v", m)
	}
	if m := got["morgens/2"]; m["armed"] != false || m["off"] != "no-location" || m["next"] != nil {
		t.Errorf("morgens/2: %v", m)
	}
	if m := got["nacht/2"]; m["armed"] != true || m["active"] != false || m["next"].(map[string]any)["kind"] != "begin" {
		t.Errorf("nacht/2: %v", m)
	}
	// it fires: the last firing is there, the range is under way
	clock.Advance(17 * time.Hour)
	got = timers()
	if m := got["nacht/2"]; m["active"] != true || m["last"].(map[string]any)["at"] != "2026-11-21T22:00:00Z" || m["next"].(map[string]any)["kind"] != "end" {
		t.Errorf("nacht/2 at night: %v", m)
	}
	if m := got["morgens/1"]; m["last"].(map[string]any)["at"] != "2026-11-21T06:30:00Z" || m["next"].(map[string]any)["at"] != "2026-11-22T06:30:00Z" {
		t.Errorf("morgens/1 later: %v", m)
	}
	// an inactive program's lines are not armed, and neither is true
	if _, err := svc.SetActive("nacht", false, who); err != nil {
		t.Fatal(err)
	}
	if m := timers()["nacht/2"]; m["armed"] != false || m["off"] != "inactive" || m["active"] != false || m["next"] != nil {
		t.Errorf("inactive: %v", m)
	}
	if _, err := svc.SetActive("nacht", true, who); err != nil {
		t.Fatal(err)
	}
	// a faulty program's lines are not armed either: its variable is gone
	if _, err := svc.SetActive("nacht", false, who); err != nil {
		t.Fatal(err)
	}
	faulty, _ := svc.Program("nacht")
	faulty.Program.Rules[0].When.Groups[0].Lines[0].Variable = "weg"
	if _, err := svc.UpdateProgram("nacht", faulty.Program, who); err != nil {
		t.Fatal(err)
	}
	if m := timers()["nacht/2"]; m["armed"] != false || m["off"] != "faulty" {
		t.Errorf("faulty: %v", m)
	}
	faulty.Program.Rules[0].When.Groups[0].Lines[0].Variable = "regen"
	faulty.Program.Active = true
	if _, err := svc.UpdateProgram("nacht", faulty.Program, who); err != nil {
		t.Fatal(err)
	}

	// the delays: scheduled by a run, the next one first
	if err := svc.Engine().SetVariable("regen", true, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	out := e.want(200, "", "GET", "schedule", nil)
	pend := out["pending"].([]any)
	if len(pend) != 2 || out["now"] != "2026-11-21T23:00:00Z" {
		t.Fatalf("pending: %v", out)
	}
	first, second := pend[0].(map[string]any), pend[1].(map[string]any)
	if first["due"] != "2026-11-21T23:10:00Z" || first["set"] != "2026-11-21T23:00:00Z" || first["program"] != "nacht" || first["action"].(map[string]any)["to"] != 5.0 {
		t.Errorf("first: %v", first)
	}
	id1, id2 := strconv.Itoa(int(first["id"].(float64))), strconv.Itoa(int(second["id"].(float64)))
	// run now: it happened; cancel: it never will
	e.want(200, "", "POST", "pending/"+id1+"/run", nil)
	if v, _ := svc.Engine().Value("zahl"); v != 5.0 {
		t.Errorf("zahl after run now: %v", v)
	}
	e.want(404, "not-found", "POST", "pending/"+id1+"/run", nil)
	e.want(200, "", "DELETE", "pending/"+id2, nil)
	e.want(404, "not-found", "DELETE", "pending/"+id2, nil)
	e.want(400, "invalid-id", "DELETE", "pending/x", nil)
	e.want(400, "invalid-id", "POST", "pending/x/run", nil)
	clock.Advance(time.Hour)
	if v, _ := svc.Engine().Value("zahl"); v != 5.0 {
		t.Errorf("zahl after the cancelled delay's time: %v", v)
	}
	if out := e.want(200, "", "GET", "schedule", nil); len(out["pending"].([]any)) != 0 {
		t.Errorf("pending: %v", out)
	}
}

// The schedule lists the lines that count how long their comparison has held: since when, when
// they are (or were) true, and that the count goes on in an inactive program.
func TestScheduleHolds(t *testing.T) {
	clock := engine.NewFakeClock(time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC))
	svc, err := service.Open(service.Options{Dir: t.TempDir(), Clock: clock, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e := &env{t: t, svc: svc, sys: &fakeSystem{}}
	e.srv = newServer(t, Options{Service: svc, System: e.sys, DevClock: clock})
	who := repo.Author{Name: "t"}
	if _, err := svc.CreateVariable(&model.Variable{Name: "Zahl", Type: model.VarNumber, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	const text = "name: Lange\nactive: true\nrules:\n  - when:\n      all:\n        - all:\n            - variable: zahl\n              is: 1\n              trigger: check\n            - variable: zahl\n              op: gt\n              value: 5\n              for: 10min\n              trigger: change\n    then:\n      - variable: zahl\n        to: 7\n"
	p, err := model.DecodeProgram("programs/lange.yaml", "lange", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProgram(p, who); err != nil {
		t.Fatal(err)
	}
	holds := func() []any { return e.want(200, "", "GET", "schedule", nil)["holds"].([]any) }
	if got := holds(); len(got) != 0 {
		t.Fatalf("nothing holds yet: %v", got)
	}
	if err := svc.Engine().SetVariable("zahl", 9.0, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	clock.Advance(4 * time.Minute)
	got := holds()
	if len(got) != 1 {
		t.Fatalf("holds: %v", got)
	}
	h := got[0].(map[string]any)
	if h["program"] != "lange" || h["line"] != 2.0 || h["for"] != "10min" || h["since"] != "2026-11-21T06:00:00Z" || h["due"] != "2026-11-21T06:10:00Z" || h["true"] != false || h["inactive"] != nil ||
		h["of"].(map[string]any)["variable"] != "zahl" {
		t.Errorf("counting: %v", h)
	}
	clock.Advance(6 * time.Minute)
	if h := holds()[0].(map[string]any); h["true"] != true || h["since"] != "2026-11-21T06:00:00Z" {
		t.Errorf("held long enough: %v", h)
	}
	// an inactive program's line counts too, from the moment it was switched off
	if _, err := svc.SetActive("lange", false, who); err != nil {
		t.Fatal(err)
	}
	if h := holds()[0].(map[string]any); h["true"] != false || h["inactive"] != true || h["since"] != "2026-11-21T06:10:00Z" {
		t.Errorf("inactive: %v", h)
	}
	// the comparison ends: the line is gone from the list
	if err := svc.Engine().SetVariable("zahl", 2.0, engine.FromUser); err != nil {
		t.Fatal(err)
	}
	if got := holds(); len(got) != 0 {
		t.Errorf("after the end: %v", got)
	}
}

// While the system's clock is not set no time module is armed, and the schedule says so.
func TestScheduleWithoutClock(t *testing.T) {
	clock := engine.NewFakeClock(time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC))
	svc, err := service.Open(service.Options{Dir: t.TempDir(), Clock: clock, Location: time.UTC, ClockReady: func() bool { return false }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	e := &env{t: t, svc: svc, sys: &fakeSystem{}}
	e.srv = newServer(t, Options{Service: svc, System: e.sys})
	p, err := model.DecodeProgram("programs/morgens.yaml", "morgens", []byte(timedProgram))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProgram(p, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	first := e.want(200, "", "GET", "schedule", nil)["timers"].([]any)[0].(map[string]any)
	if first["armed"] != false || first["off"] != "no-clock" || first["next"] != nil {
		t.Errorf("without a clock: %v", first)
	}
}
