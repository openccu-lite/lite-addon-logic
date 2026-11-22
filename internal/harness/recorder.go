package harness

import (
	"fmt"
	"strings"
	"time"
)

// Target is a reference system: a ReGaHss whose clock the harness set.
type Target interface {
	// Script runs ReGa script and answers what it wrote.
	Script(text string) (string, error)
	// Regadom answers the saved database.
	Regadom() ([]byte, error)
	// Restart stops the logic and starts it again with its clock showing at.
	Restart(at time.Time, rate int) error
}

// Recorder plays scenarios on a reference.
type Recorder struct {
	Target Target
	// Now and Sleep are the real clock (time.Now and time.Sleep when nil).
	Now   func() time.Time
	Sleep func(time.Duration)
	// Log receives progress lines (nil: silent).
	Log func(format string, args ...any)
	// Poll is how long the trace may wait to be read (20 s when zero).
	Poll time.Duration

	// the clock's mapping: the reference shows refBase at realBase and runs rate times as fast
	refBase, realBase time.Time
	rate              int
}

func (r *Recorder) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Recorder) sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	if r.Sleep != nil {
		r.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (r *Recorder) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(format, args...)
	}
}

// refTime reads the reference's clock.
func (r *Recorder) refTime() (time.Time, error) {
	out, err := r.Target.Script(TimeScript)
	if err != nil {
		return time.Time{}, err
	}
	return parseRef(out)
}

func parseRef(out string) (time.Time, error) {
	t, err := parseWall(strings.TrimSpace(out))
	if err != nil {
		return time.Time{}, fmt.Errorf("the reference answered %q where a time was asked", strings.TrimSpace(out))
	}
	return t, nil
}

// calibrate learns how the reference's clock maps onto the real one: its answers are whole
// seconds, so of several samples the one that shows the latest clock for its moment is closest.
func (r *Recorder) calibrate(rate int) error {
	r.rate = rate
	r.realBase = r.now()
	var best time.Time
	for i := 0; i < 8; i++ {
		before := r.now()
		ref, err := r.refTime()
		if err != nil {
			return err
		}
		mid := before.Add(r.now().Sub(before) / 2)
		base := ref.Add(-time.Duration(rate) * mid.Sub(r.realBase))
		if i == 0 || base.After(best) {
			best = base
		}
		r.sleep(130 * time.Millisecond)
	}
	r.refBase = best
	return nil
}

// realAt answers the real time at which the reference shows ref.
func (r *Recorder) realAt(ref time.Time) time.Time {
	return r.realBase.Add(ref.Sub(r.refBase) / time.Duration(r.rate))
}

// Record plays a scenario and answers what the reference did and the database it saved after
// the programs were created.
func (r *Recorder) Record(sc *Scenario) (*Recording, []byte, error) {
	if err := sc.Check(); err != nil {
		return nil, nil, err
	}
	rate, start := sc.SpeedRate(), sc.StartTime()
	rec := &Recording{Rate: rate}
	version, err := r.Target.Script(`Write(dom.BuildLabel());`)
	if err != nil {
		return nil, nil, err
	}
	rec.Reference = "ReGaHss " + strings.TrimSpace(version)
	if out, err := r.Target.Script(sc.SetupScript()); err != nil || !strings.Contains(out, "setup ok") {
		return nil, nil, fmt.Errorf("the setup failed: %q %v", out, err)
	}
	for i := range sc.Programs {
		p := &sc.Programs[i]
		out, err := r.Target.Script(sc.ProgramScript(p))
		if err != nil {
			return nil, nil, err
		}
		_, when, ok := strings.Cut(strings.TrimSpace(out), "created "+p.Name+" ")
		if !ok {
			return nil, nil, fmt.Errorf("program %s was not created: %q", p.Name, out)
		}
		at, err := parseRef(when)
		if err != nil {
			return nil, nil, err
		}
		rec.Created = append(rec.Created, Created{Program: p.Name, Time: at})
	}
	if out, err := r.Target.Script(SaveScript); err != nil || !strings.Contains(out, "saved") {
		return nil, nil, fmt.Errorf("the database was not saved: %q %v", out, err)
	}
	dom, err := r.Target.Regadom()
	if err != nil {
		return nil, nil, err
	}
	if err := r.calibrate(rate); err != nil {
		return nil, nil, err
	}
	if first := start; r.realAt(first).Before(r.now()) {
		ref, _ := r.refTime()
		return nil, nil, fmt.Errorf("the setup took too long: the reference shows %s, the scenario starts at %s", ref.Format(TimeLayout), sc.Start)
	}
	var raw strings.Builder
	// reading the trace asks for a heartbeat first: a reference that answers scripts but no
	// longer runs programs would otherwise look like one on which nothing ran
	collect := func() error {
		if _, err := r.Target.Script(BeatScript); err != nil {
			return err
		}
		for try := 0; try < 30; try++ {
			out, err := r.Target.Script(TraceScript)
			raw.WriteString(out)
			if err != nil {
				return err
			}
			if strings.Contains(out, BeatVar+"@") {
				return nil
			}
			r.sleep(100 * time.Millisecond)
		}
		return fmt.Errorf("the reference no longer runs programs (the heartbeat was not noted)")
	}
	poll := r.Poll
	if poll <= 0 {
		poll = 20 * time.Second
	}
	wait := func(ref time.Time) error {
		for {
			left := r.realAt(ref).Sub(r.now())
			if left <= 0 {
				return nil
			}
			if left > poll {
				r.sleep(poll)
				if err := collect(); err != nil {
					return err
				}
				continue
			}
			r.sleep(left)
		}
	}
	for i := range sc.Steps {
		st := &sc.Steps[i]
		want := start.Add(st.At)
		if err := wait(want); err != nil {
			return nil, nil, err
		}
		var at time.Time
		if st.Restart {
			if out, err := r.Target.Script(SaveScript); err != nil || !strings.Contains(out, "saved") {
				return nil, nil, fmt.Errorf("the database was not saved: %q %v", out, err)
			}
			if err := collect(); err != nil {
				return nil, nil, err
			}
			if err := r.Target.Restart(want, rate); err != nil {
				return nil, nil, err
			}
			if err := r.calibrate(rate); err != nil {
				return nil, nil, err
			}
			at = want
		} else {
			out, err := r.Target.Script(sc.StepScript(st))
			if err != nil {
				return nil, nil, err
			}
			if at, err = parseRef(out); err != nil {
				return nil, nil, err
			}
			if d := at.Sub(want); d < -sc.SlackTime() || d > sc.SlackTime() {
				return nil, nil, fmt.Errorf("step %d was done at %s, not at %s", i+1, at.Format(TimeLayout), want.Format(TimeLayout))
			}
		}
		rec.Steps = append(rec.Steps, at)
		r.logf("%s: step %d at %s", sc.Name, i+1, at.Format(TimeLayout))
	}
	end := start.Add(sc.Until)
	if err := wait(end.Add(sc.SlackTime())); err != nil {
		return nil, nil, err
	}
	if err := collect(); err != nil {
		return nil, nil, err
	}
	trace, err := ParseTracer(raw.String())
	if err != nil {
		return nil, nil, err
	}
	for _, e := range trace {
		if !e.Time.After(end) {
			rec.Trace = append(rec.Trace, e)
		}
	}
	return rec, dom, nil
}
