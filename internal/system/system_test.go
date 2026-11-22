package system

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource/fakelite"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

type fakeCaller struct {
	mu    sync.Mutex
	calls []string
	asked []string
	fail  map[string]error
	types map[string]litesource.Paramset
	block chan struct{}
}

func (c *fakeCaller) Call(ctx context.Context, iface, method string, params ...any) (json.RawMessage, error) {
	if c.block != nil {
		select {
		case <-c.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	b, _ := json.Marshal(params)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, fmt.Sprintf("%s %s %s", iface, method, b))
	return nil, c.fail[fmt.Sprint(params[0])]
}

func (c *fakeCaller) ParamsetsFor(ctx context.Context, iface string, addresses []string, paramset string) (map[string]string, litesource.ParamsetCache, map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.asked = append(c.asked, iface+"/"+addresses[0])
	set, ok := c.types[addresses[0]]
	if !ok {
		return nil, nil, nil, errors.New("unknown")
	}
	return map[string]string{addresses[0]: "k"}, litesource.ParamsetCache{"k": set}, nil, nil
}

// "ein für": the on-time and the value in one putParamset, each with its datapoint's type.
func TestWriteSet(t *testing.T) {
	c := &fakeCaller{fail: map[string]error{"BAD:1": errors.New("-1 Failure")}, types: map[string]litesource.Paramset{
		"A:1": {"ON_TIME": {Type: "FLOAT"}, "STATE": {Type: "BOOL"}},
	}}
	w := &Writer{Lite: c, Sleep: func(time.Duration) {}}
	var wg sync.WaitGroup
	var errs []error
	wg.Add(2)
	done := func(err error) { errs = append(errs, err); wg.Done() }
	w.WriteSet(ref("BidCos-RF/A:1/STATE"), []model.ParamValue{{Param: "ON_TIME", Value: 300.0}, {Param: "STATE", Value: 1.0}}, done)
	w.WriteSet(ref("BidCos-RF/BAD:1/STATE"), []model.ParamValue{{Param: "STATE", Value: true}}, done)
	wg.Wait()
	want := []string{`BidCos-RF putParamset ["A:1","VALUES",{"ON_TIME":{"double":300},"STATE":true}]`, `BidCos-RF putParamset ["BAD:1","VALUES",{"STATE":true}]`}
	if !reflect.DeepEqual(c.calls, want) || errs[0] != nil || errs[1] == nil {
		t.Errorf("calls %v, errors %v", c.calls, errs)
	}
	w.Close()
}

func ref(s string) model.Ref {
	r, err := model.ParseRef(s)
	if err != nil {
		panic(err)
	}
	return r
}

// One write after the other per interface, each with the type its datapoint has, a pause
// between two on a radio interface, and a refused write reported, not retried.
func TestWriter(t *testing.T) {
	c := &fakeCaller{fail: map[string]error{"BAD:1": errors.New("-1 Failure")}, types: map[string]litesource.Paramset{
		"A:1": {"LEVEL": {Type: "FLOAT"}, "STATE": {Type: "BOOL"}, "MODE": {Type: "ENUM"}, "COUNT": {Type: "INTEGER"}, "TEXT": {Type: "STRING"}, "PRESS": {Type: "ACTION"}},
	}}
	var slept []time.Duration
	var mu sync.Mutex
	w := &Writer{Lite: c, Sleep: func(d time.Duration) { mu.Lock(); slept = append(slept, d); mu.Unlock() }}
	var wg sync.WaitGroup
	results := map[string]error{}
	send := func(r string, v any) {
		wg.Add(1)
		w.Write(ref(r), v, func(err error) { mu.Lock(); results[r+fmt.Sprint(v)] = err; mu.Unlock(); wg.Done() })
	}
	send("HmIP-RF/A:1/LEVEL", 0.0)
	send("HmIP-RF/A:1/LEVEL", true)
	send("HmIP-RF/A:1/STATE", 1.0)
	send("HmIP-RF/A:1/STATE", false)
	send("HmIP-RF/A:1/MODE", 2.0)
	send("HmIP-RF/A:1/MODE", true)
	send("HmIP-RF/A:1/COUNT", 2.6)
	send("HmIP-RF/A:1/TEXT", 5.0)
	send("HmIP-RF/A:1/TEXT", "x")
	send("HmIP-RF/A:1/PRESS", 1.0)
	send("HmIP-RF/A:1/LEVEL", "text")
	send("HmIP-RF/BAD:1/STATE", true)
	send("HmIP-RF/NOTYPE:1/LEVEL", 1.0)
	send("HmIP-RF/NOTYPE:1/LEVEL", 0.5)
	send("HmIP-RF/NOTYPE:1/STATE", true)
	wg.Wait()
	want := []string{
		`HmIP-RF setValue ["A:1","LEVEL",{"double":0}]`, `HmIP-RF setValue ["A:1","LEVEL",{"double":1}]`,
		`HmIP-RF setValue ["A:1","STATE",true]`, `HmIP-RF setValue ["A:1","STATE",false]`,
		`HmIP-RF setValue ["A:1","MODE",2]`, `HmIP-RF setValue ["A:1","MODE",1]`, `HmIP-RF setValue ["A:1","COUNT",3]`,
		`HmIP-RF setValue ["A:1","TEXT","5"]`, `HmIP-RF setValue ["A:1","TEXT","x"]`, `HmIP-RF setValue ["A:1","PRESS",true]`,
		`HmIP-RF setValue ["A:1","LEVEL","text"]`, `HmIP-RF setValue ["BAD:1","STATE",true]`,
		`HmIP-RF setValue ["NOTYPE:1","LEVEL",1]`, `HmIP-RF setValue ["NOTYPE:1","LEVEL",{"double":0.5}]`, `HmIP-RF setValue ["NOTYPE:1","STATE",true]`,
	}
	if !reflect.DeepEqual(c.calls, want) {
		t.Errorf("calls:\n%v\nwant\n%v", c.calls, want)
	}
	// the descriptions were asked once per channel
	if !reflect.DeepEqual(c.asked, []string{"HmIP-RF/A:1", "HmIP-RF/BAD:1", "HmIP-RF/NOTYPE:1"}) {
		t.Errorf("asked: %v", c.asked)
	}
	if results["HmIP-RF/BAD:1/STATEtrue"] == nil || results["HmIP-RF/A:1/LEVEL0"] != nil {
		t.Errorf("results: %v", results)
	}
	// a pause before every write but the first, on the radio
	if len(slept) != len(want)-1 || slept[0] != DefaultGap {
		t.Errorf("pauses: %v", slept)
	}
	// another interface has its own queue and, when it is no radio, no pause
	slept = nil
	send("VirtualDevices/V:1/STATE", true)
	send("VirtualDevices/V:1/STATE", false)
	wg.Wait()
	if len(slept) != 0 {
		t.Errorf("a pause on a virtual interface: %v", slept)
	}
	w.Close()
	// the answer never comes on the caller's goroutine: the engine calls with its lock held
	after := make(chan error, 1)
	w.Write(ref("HmIP-RF/A:1/STATE"), true, func(err error) { after <- err })
	if err := <-after; err == nil {
		t.Error("a write after the close")
	}
}

func TestWriterSettingsTimeoutAndOverflow(t *testing.T) {
	c := &fakeCaller{block: make(chan struct{})}
	var slept []time.Duration
	var mu sync.Mutex
	w := &Writer{Lite: c, Gap: time.Millisecond, Radio: func(string) bool { return true }, Timeout: 20 * time.Millisecond, Sleep: func(d time.Duration) { mu.Lock(); slept = append(slept, d); mu.Unlock() }}
	var wg sync.WaitGroup
	var errs []error
	var dropped int
	for i := 0; i < 300; i++ {
		wg.Add(1)
		w.Write(ref("Any/A:1/STATE"), true, func(err error) {
			mu.Lock()
			if err != nil && errors.Is(err, context.DeadlineExceeded) {
				errs = append(errs, err)
			} else if err != nil {
				dropped++
			}
			mu.Unlock()
			wg.Done()
		})
	}
	wg.Wait()
	w.Close()
	if dropped == 0 || len(errs) == 0 || len(errs)+dropped != 300 {
		t.Errorf("%d timed out, %d dropped", len(errs), dropped)
	}
	if len(slept) == 0 || slept[0] != time.Millisecond {
		t.Errorf("pauses: %v", slept)
	}
	// the pause is a setting: it holds from the next write on, and zero is the default again
	w.SetGap(700 * time.Millisecond)
	if w.gap() != 700*time.Millisecond {
		t.Errorf("the gap that was set: %v", w.gap())
	}
	w.SetGap(0)
	if w.gap() != DefaultGap {
		t.Errorf("no gap is the default: %v", w.gap())
	}
}

type sink struct {
	mu     sync.Mutex
	refs   []model.Ref
	seeds  []string
	events []string
	ch     chan string
}

func (s *sink) Refs() []model.Ref {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]model.Ref(nil), s.refs...)
}
func (s *sink) SeedDevice(r model.Ref, v any) {
	s.mu.Lock()
	s.seeds = append(s.seeds, fmt.Sprintf("%s=%v", r, v))
	s.mu.Unlock()
	s.ch <- "seed"
}
func (s *sink) DeviceEvent(r model.Ref, v any) {
	s.mu.Lock()
	s.events = append(s.events, fmt.Sprintf("%s=%v", r, v))
	s.mu.Unlock()
	s.ch <- "event"
}

func (s *sink) wait(t *testing.T, what string) {
	t.Helper()
	select {
	case got := <-s.ch:
		if got != what {
			t.Fatalf("got a %s, want a %s", got, what)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no %s", what)
	}
}

func entry(iface, addr, dp string, v any) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]any{"interface": iface, "address": addr, "datapoint": dp, "value": v, "ts": now, "lc": now, "confirmed": true, "source": "event"}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The feed against the kit's fake system: the state store as a seed, the stream's events as
// events, only for what the programs read; a resync seeds again; changed programs change the
// filter.
func TestFeed(t *testing.T) {
	fake := fakelite.New()
	defer fake.Close()
	fake.Interfaces = []string{"HmIP-RF", "Logic"}
	fake.EventID = fake.BootID + "-0"
	fake.State = []map[string]any{entry("HmIP-RF", "A:1", "STATE", true), entry("HmIP-RF", "A:1", "LEVEL", 0.5), entry("HmIP-RF", "B:1", "STATE", false), entry("HmIP-RF", "A:1", "OBJECT", map[string]any{"x": 1})}
	lite := litesource.New(fake.URL, addonkit.StaticToken("t"))
	lite.Reconnect.Min, lite.Reconnect.Max = 10*time.Millisecond, 20*time.Millisecond
	s := &sink{ch: make(chan string, 100)}
	var logs []string
	var lmu sync.Mutex
	f := &Feed{Lite: lite, Sink: s, Own: "Logic", Retry: 20 * time.Millisecond, Log: func(format string, a ...any) {
		lmu.Lock()
		logs = append(logs, fmt.Sprintf(format, a...))
		lmu.Unlock()
	}}
	f.Kick() // before Run: nothing to tell
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.Run(ctx); close(done) }()
	// nothing to read yet: no stream
	time.Sleep(30 * time.Millisecond)
	if fake.Streams() != 0 || f.Seeded() != 0 {
		t.Fatalf("a stream without anything to read")
	}

	s.mu.Lock()
	s.refs = []model.Ref{ref("HmIP-RF/A:1/STATE"), ref("Logic/var-x:1/VALUE")}
	s.mu.Unlock()
	f.Kick()
	s.wait(t, "seed")
	waitFor(t, "the stream", func() bool { return fake.Streams() == 1 })
	if q := fake.Queries(); len(q) != 1 {
		t.Fatalf("queries: %v", q)
	}
	fake.PushEvent("HmIP-RF", "A:1", "STATE", false)
	fake.PushEvent("HmIP-RF", "A:1", "LEVEL", 0.7)                    // not read by a program
	fake.PushEvent("HmIP-RF", "A:1", "STATE", map[string]any{"x": 1}) // not a value
	fake.PushEvent("HmIP-RF", "A:1", "STATE", 1)
	s.wait(t, "event")
	s.wait(t, "event")
	// the store's sweep is a seed, not an event
	fake.Push("state", entry("HmIP-RF", "A:1", "STATE", true))
	s.wait(t, "seed")
	// a kick without a change keeps the stream
	f.Kick()
	time.Sleep(30 * time.Millisecond)
	if len(fake.Queries()) != 1 {
		t.Errorf("an unchanged set of datapoints opened a new stream")
	}
	// the stream loses its place: seeded again
	fake.Resync("overflow")
	s.wait(t, "seed")
	// the programs change: a new filter, a new seed
	s.mu.Lock()
	s.refs = append(s.refs, ref("HmIP-RF/B:1/STATE"))
	s.mu.Unlock()
	f.Kick()
	s.wait(t, "seed")
	s.wait(t, "seed")
	waitFor(t, "the second stream", func() bool { return len(fake.Queries()) == 2 && fake.Streams() == 1 })
	fake.PushEvent("HmIP-RF", "B:1", "STATE", true)
	s.wait(t, "event")
	s.mu.Lock()
	seeds, events := append([]string(nil), s.seeds...), append([]string(nil), s.events...)
	s.mu.Unlock()
	wantSeeds := []string{"HmIP-RF/A:1/STATE=true", "HmIP-RF/A:1/STATE=true", "HmIP-RF/A:1/STATE=true", "HmIP-RF/A:1/STATE=true", "HmIP-RF/B:1/STATE=false"}
	wantEvents := []string{"HmIP-RF/A:1/STATE=false", "HmIP-RF/A:1/STATE=1", "HmIP-RF/B:1/STATE=true"}
	if !reflect.DeepEqual(seeds, wantSeeds) || !reflect.DeepEqual(events, wantEvents) {
		t.Errorf("seeds %v\nevents %v", seeds, events)
	}
	if f.Seeded() != 3 {
		t.Errorf("%d seeds", f.Seeded())
	}
	lmu.Lock()
	if len(logs) != 1 {
		t.Errorf("logs: %v", logs)
	}
	lmu.Unlock()
	cancel()
	<-done
}

// The system does not answer: the feed tries again, and gets its values when it does.
func TestFeedRetries(t *testing.T) {
	fake := fakelite.New()
	defer fake.Close()
	fake.Interfaces = []string{"HmIP-RF"}
	fake.State = []map[string]any{entry("HmIP-RF", "A:1", "STATE", true)}
	fake.SetStarting(3)
	lite := litesource.New(fake.URL, addonkit.StaticToken("t"))
	lite.Reconnect.Min, lite.Reconnect.Max = 10*time.Millisecond, 20*time.Millisecond
	s := &sink{ch: make(chan string, 10), refs: []model.Ref{ref("HmIP-RF/A:1/STATE")}}
	var logs int
	var lmu sync.Mutex
	f := &Feed{Lite: lite, Sink: s, Retry: 10 * time.Millisecond, Log: func(string, ...any) { lmu.Lock(); logs++; lmu.Unlock() }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.Run(ctx); close(done) }()
	s.wait(t, "seed")
	lmu.Lock()
	if logs == 0 {
		t.Error("the failed seeds were not logged")
	}
	lmu.Unlock()
	cancel()
	<-done
	// without a log function, and stopped while it waits to retry
	fake.SetStarting(-1)
	f2 := &Feed{Lite: lite, Sink: s}
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() { f2.Run(ctx2); close(done2) }()
	time.Sleep(30 * time.Millisecond)
	cancel2()
	<-done2
}

func f64(f float64) *float64 { return &f }

// A value fits its datapoint or it is said why not; an ENUM's entry is named by its text.
func TestFit(t *testing.T) {
	for _, c := range []struct {
		p     Param
		value any
		want  any
		err   string
	}{
		{Param{ID: "STATE", Type: "BOOL"}, true, true, ""},
		{Param{ID: "STATE", Type: "BOOL"}, 1.0, true, ""},
		{Param{ID: "STATE", Type: "BOOL"}, 0.0, false, ""},
		{Param{ID: "STATE", Type: "BOOL"}, 2.0, nil, "STATE takes true or false"},
		{Param{ID: "PRESS", Type: "ACTION"}, "on", nil, "PRESS takes true or false"},
		{Param{ID: "LEVEL", Type: "FLOAT", Min: f64(0), Max: f64(1.01)}, 0.5, litesource.Double(0.5), ""},
		{Param{ID: "LEVEL", Type: "FLOAT", Min: f64(0), Max: f64(1.01)}, true, litesource.Double(1), ""},
		{Param{ID: "LEVEL", Type: "FLOAT", Min: f64(0), Max: f64(1.01)}, 1.5, litesource.Double(1.5), "LEVEL: 1.5 is outside 0 to 1.01"},
		{Param{ID: "LEVEL", Type: "FLOAT", Min: f64(0)}, -1.0, litesource.Double(-1), "LEVEL: -1 is outside 0 to …"},
		{Param{ID: "LEVEL", Type: "FLOAT", Max: f64(1)}, 2.0, litesource.Double(2), "LEVEL: 2 is outside … to 1"},
		{Param{ID: "LEVEL", Type: "FLOAT"}, "half", nil, "LEVEL takes a number"},
		{Param{ID: "COUNT", Type: "INTEGER", Min: f64(0), Max: f64(10)}, 2.6, 3, ""},
		{Param{ID: "COUNT", Type: "INTEGER", Min: f64(0), Max: f64(10)}, 11.0, 11, "COUNT: 11 is outside 0 to 10"},
		{Param{ID: "COUNT", Type: "INTEGER"}, "x", nil, "COUNT takes a whole number"},
		{Param{ID: "COLOR", Type: "ENUM", Values: []string{"BLACK", "", "RED"}}, "RED", 2, ""},
		{Param{ID: "COLOR", Type: "ENUM", Values: []string{"BLACK", "", "RED"}}, "GREEN", nil, `COLOR has no entry "GREEN"`},
		{Param{ID: "COLOR", Type: "ENUM", Values: []string{"BLACK", "", "RED"}}, "", nil, `COLOR has no entry ""`},
		{Param{ID: "COLOR", Type: "ENUM", Values: []string{"BLACK", "", "RED"}}, 1.0, 1, ""},
		{Param{ID: "COLOR", Type: "ENUM", Values: []string{"BLACK", "", "RED"}}, true, 1, ""},
		{Param{ID: "COLOR", Type: "ENUM", Values: []string{"BLACK", "", "RED"}}, 3.0, nil, "COLOR has no entry number 3"},
		{Param{ID: "COLOR", Type: "ENUM"}, -1.0, nil, "COLOR has no entry number -1"},
		{Param{ID: "COLOR", Type: "ENUM"}, 7.0, 7, ""},
		{Param{ID: "TEXT", Type: "STRING"}, "x", "x", ""},
		{Param{ID: "TEXT", Type: "STRING"}, 5.0, "5", ""},
		{Param{ID: "TEXT", Type: "STRING"}, true, "true", ""},
		{Param{ID: "X"}, 2.0, 2, ""},
		{Param{ID: "X"}, 2.5, litesource.Double(2.5), ""},
		{Param{ID: "X"}, true, true, ""},
		{Param{ID: "X"}, "t", "t", ""},
	} {
		got, err := c.p.Fit(c.value)
		text := ""
		if err != nil {
			text = err.Error()
		}
		if text != c.err || (err == nil && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("%s %s = %v: %v, %q", c.p.ID, c.p.Type, c.value, got, text)
		}
	}
	params := []Param{{ID: "LEVEL", Type: "FLOAT", Write: true, Max: f64(1)}, {ID: "STATE", Type: "BOOL"}}
	for _, c := range []struct {
		values map[string]any
		err    string
	}{
		{map[string]any{"LEVEL": 0.5}, ""},
		{map[string]any{"LEVEL": 5.0}, "LEVEL: 5 is outside … to 1"},
		{map[string]any{"STATE": true}, "STATE cannot be written"},
		{map[string]any{"COLOR": 1.0}, "the channel has no datapoint COLOR"},
	} {
		err := FitAll(params, c.values)
		if (err == nil) != (c.err == "") || (err != nil && err.Error() != c.err) {
			t.Errorf("%v: %v", c.values, err)
		}
	}
}

// Several values of a channel in one command: checked against the channel's description as it
// is at that moment, an entry by its name; what does not fit is refused and nothing is sent.
func TestWriteSetIsChecked(t *testing.T) {
	c := &fakeCaller{types: map[string]litesource.Paramset{
		"A:8": {
			"COLOR":            {Type: "ENUM", Operations: 7, ValueList: []string{"BLACK", "BLUE", "RED"}},
			"LEVEL":            {Type: "FLOAT", Operations: 7, Min: json.RawMessage("0"), Max: json.RawMessage("1.01")},
			"DURATION_VALUE":   {Type: "INTEGER", Operations: 2},
			"ACTUAL_SOMETHING": {Type: "FLOAT", Operations: 5},
		},
		"EMPTY:1": {},
	}}
	w := &Writer{Lite: c, Sleep: func(time.Duration) {}}
	send := func(channel string, values ...model.ParamValue) string {
		done := make(chan error, 1)
		w.WriteSet(model.ChannelRef(channel), values, func(err error) { done <- err })
		if err := <-done; err != nil {
			return err.Error()
		}
		return ""
	}
	v := func(param string, value any) model.ParamValue { return model.ParamValue{Param: param, Value: value} }
	for _, tc := range []struct {
		channel string
		values  []model.ParamValue
		err     string
	}{
		{"HmIP-RF/A:8", []model.ParamValue{v("COLOR", "RED"), v("LEVEL", 0.5), v("DURATION_VALUE", 10.0)}, ""},
		{"HmIP-RF/A:8", []model.ParamValue{v("COLOR", "GREEN"), v("LEVEL", 0.5)}, `COLOR has no entry "GREEN"`},
		{"HmIP-RF/A:8", []model.ParamValue{v("LEVEL", 2.0)}, "LEVEL: 2 is outside 0 to 1.01"},
		{"HmIP-RF/A:8", []model.ParamValue{v("COLOR_BEHAVIOUR", 1.0)}, "the channel has no datapoint COLOR_BEHAVIOUR"},
		{"HmIP-RF/A:8", []model.ParamValue{v("ACTUAL_SOMETHING", 1.0)}, "ACTUAL_SOMETHING cannot be written"},
		{"HmIP-RF/EMPTY:1", []model.ParamValue{v("LEVEL", 2.0), v("ON", true)}, ""},
		{"HmIP-RF/NOTYPE:1", []model.ParamValue{v("LEVEL", 0.5)}, ""},
	} {
		if got := send(tc.channel, tc.values...); got != tc.err {
			t.Errorf("%s %v: %q", tc.channel, tc.values, got)
		}
	}
	want := []string{
		`HmIP-RF putParamset ["A:8","VALUES",{"COLOR":2,"DURATION_VALUE":10,"LEVEL":{"double":0.5}}]`,
		`HmIP-RF putParamset ["EMPTY:1","VALUES",{"LEVEL":2,"ON":true}]`,
		`HmIP-RF putParamset ["NOTYPE:1","VALUES",{"LEVEL":{"double":0.5}}]`,
	}
	if !reflect.DeepEqual(c.calls, want) {
		t.Errorf("calls %v", c.calls)
	}
	// the description is asked each time: a firmware that changed its datapoints is seen
	c.mu.Lock()
	c.types["A:8"] = litesource.Paramset{"LEVEL": {Type: "FLOAT", Operations: 7}}
	c.mu.Unlock()
	if got := send("HmIP-RF/A:8", v("COLOR", "RED")); got != "the channel has no datapoint COLOR" {
		t.Errorf("after the change: %q", got)
	}
	w.Close()
}
