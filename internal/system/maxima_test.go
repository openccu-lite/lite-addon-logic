package system

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

type counterSink struct {
	mu   sync.Mutex
	refs []model.Ref
	got  map[string]float64
}

func (s *counterSink) HelperCounters() []model.Ref {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.Ref(nil), s.refs...)
}

func (s *counterSink) SetCounterMaximum(ref model.Ref, max float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got[ref.String()] = max
}

func (s *counterSink) maxima() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]float64{}
	for k, v := range s.got {
		out[k] = v
	}
	return out
}

type paramSource struct {
	mu     sync.Mutex
	params map[string][]Param
	err    error
	asked  [][]string
}

func (p *paramSource) Params(_ context.Context, channels []string) (map[string][]Param, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sort.Strings(channels)
	p.asked = append(p.asked, channels)
	if p.err != nil {
		return nil, p.err
	}
	out := map[string][]Param{}
	for _, ch := range channels {
		if ps, ok := p.params[ch]; ok {
			out[ch] = ps
		}
	}
	return out, nil
}

func (p *paramSource) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.asked)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("never: %s", what)
}

// The maxima of the counters the helpers follow come from the devices' descriptions: asked when
// the helpers change, and again while a device does not answer.
func TestMaxima(t *testing.T) {
	ref := func(s string) model.Ref {
		r, err := model.ParseRef(s)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	f := 838859.1
	g := 10995116277.59
	src := &paramSource{err: errors.New("down"), params: map[string][]Param{
		"HmIP-RF/000A:6": {{ID: "ENERGY_COUNTER", Max: &f}, {ID: "POWER"}},
		"HmIP-RF/000E:2": {{ID: "GAS_VOLUME", Max: &g}, {ID: "ENERGY_COUNTER"}},
	}}
	sink := &counterSink{got: map[string]float64{}, refs: []model.Ref{ref("HmIP-RF/000A:6/ENERGY_COUNTER")}}
	var logs []string
	var logMu sync.Mutex
	m := &Maxima{Params: src, Sink: sink, Retry: 5 * time.Millisecond, Log: func(format string, args ...any) {
		logMu.Lock()
		logs = append(logs, format)
		logMu.Unlock()
	}}
	m.Kick() // before it runs: nothing to tell
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	// the system does not answer: it asks again
	eventually(t, "asked twice", func() bool { return src.calls() >= 2 })
	logMu.Lock()
	if len(logs) == 0 {
		t.Error("nothing logged while the system is down")
	}
	logMu.Unlock()
	src.mu.Lock()
	src.err = nil
	src.mu.Unlock()
	eventually(t, "the maximum", func() bool { return sink.maxima()["HmIP-RF/000A:6/ENERGY_COUNTER"] == f })
	// everything known: nothing is asked until the helpers change
	n := src.calls()
	time.Sleep(30 * time.Millisecond)
	if src.calls() != n {
		t.Error("asked again with nothing missing")
	}
	// a helper on an HmIP-ESI (its channel has one counter with a maximum, the other without)
	// and one on a device that is not there
	sink.mu.Lock()
	sink.refs = append(sink.refs, ref("HmIP-RF/000E:2/ENERGY_COUNTER"), ref("HmIP-RF/000E:2/GAS_VOLUME"), ref("HmIP-RF/000F:1/RAIN_COUNTER"))
	sink.mu.Unlock()
	m.Kick()
	eventually(t, "the gas maximum", func() bool { return sink.maxima()["HmIP-RF/000E:2/GAS_VOLUME"] == g })
	// the missing device is asked for again and again, and found once it is there
	n = src.calls()
	eventually(t, "asked again for the missing device", func() bool { return src.calls() > n+1 })
	src.mu.Lock()
	last := src.asked[len(src.asked)-1]
	r := 1638.0
	src.params["HmIP-RF/000F:1"] = []Param{{ID: "RAIN_COUNTER", Max: &r}}
	src.mu.Unlock()
	if !reflect.DeepEqual(last, []string{"HmIP-RF/000E:2", "HmIP-RF/000F:1"}) && !reflect.DeepEqual(last, []string{"HmIP-RF/000F:1"}) {
		t.Errorf("asked for %v", last)
	}
	eventually(t, "the rain maximum", func() bool { return sink.maxima()["HmIP-RF/000F:1/RAIN_COUNTER"] == r })
	if known := m.Known(); len(known) != 3 || known[ref("HmIP-RF/000A:6/ENERGY_COUNTER")] != f {
		t.Errorf("known: %v", known)
	}
	if got := sink.maxima(); len(got) != 3 {
		t.Errorf("told: %v", got)
	}
	cancel()
	<-done
	// a cancelled question is not logged
	m2 := &Maxima{Params: &paramSource{err: errors.New("down")}, Sink: sink, Log: func(string, ...any) { t.Error("logged after the end") }}
	m2.done = map[model.Ref]float64{}
	gone, stop := context.WithCancel(context.Background())
	stop()
	if left := m2.resolve(gone); left != 4 {
		t.Errorf("left: %d", left)
	}
	m2.Run(gone)
}
