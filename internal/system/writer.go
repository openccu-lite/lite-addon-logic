// Package system is the engine's connection to openccu-lite: the device writes go out through
// lite-rpc (writer.go), the datapoints' values come in from the state store and the event stream
// (feed.go). Both through the kit's litesource with the addon's own token.
package system

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Caller is the slice of the kit's client the writer uses.
type Caller interface {
	Call(ctx context.Context, iface, method string, params ...any) (json.RawMessage, error)
	ParamsetsFor(ctx context.Context, iface string, addresses []string, paramset string) (map[string]string, litesource.ParamsetCache, map[string]string, error)
}

// DefaultGap is the pause between two writes on a radio interface.
const DefaultGap = 300 * time.Millisecond

// Writer sends the engine's device writes: one after the other per interface, each awaiting its
// answer, with a pause between two writes on a radio interface - a program's burst must not
// become a burst on air. A write the interface refuses is reported and not tried again (the
// interface processes repeat on air themselves).
type Writer struct {
	// Lite is the client.
	Lite Caller
	// Gap is the pause between writes on a radio interface (DefaultGap when zero); Radio says
	// which interfaces are radio (BidCos-RF and HmIP-RF when nil).
	Gap   time.Duration
	Radio func(iface string) bool
	// Timeout bounds one write (30 s when zero).
	Timeout time.Duration
	// Sleep waits (time.Sleep when nil; a test's hook).
	Sleep func(time.Duration)

	mu     sync.Mutex
	queues map[string]chan job
	types  map[string]string
	wg     sync.WaitGroup
	closed bool
}

type job struct {
	ref   model.Ref
	value any
	// set: several parameters of the channel in one putParamset (ref's parameter is not used)
	set  []model.ParamValue
	done func(error)
}

func radio(iface string) bool { return iface == "BidCos-RF" || iface == "HmIP-RF" }

// Write queues a write and returns; done is called with the interface's answer.
func (w *Writer) Write(ref model.Ref, value any, done func(error)) {
	w.enqueue(job{ref: ref, value: value, done: done})
}

// WriteSet queues one command that writes several parameters of a channel together
// (putParamset on VALUES) - an on-time with the value it belongs to.
func (w *Writer) WriteSet(channel model.Ref, values []model.ParamValue, done func(error)) {
	w.enqueue(job{ref: channel, set: values, done: done})
}

func (w *Writer) enqueue(j job) {
	ref, done := j.ref, j.done
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		go done(fmt.Errorf("the addon is stopping")) // never from the caller's goroutine: it may hold a lock done takes
		return
	}
	if w.queues == nil {
		w.queues = map[string]chan job{}
		w.types = map[string]string{}
	}
	q := w.queues[ref.Interface]
	if q == nil {
		q = make(chan job, 256)
		w.queues[ref.Interface] = q
		w.wg.Add(1)
		go w.run(ref.Interface, q)
	}
	w.mu.Unlock()
	select {
	case q <- j:
	default:
		go done(fmt.Errorf("more than 256 writes are waiting for %s: dropped", ref.Interface))
	}
}

// Close ends the workers after what is queued.
func (w *Writer) Close() {
	w.mu.Lock()
	w.closed = true
	for _, q := range w.queues {
		close(q)
	}
	w.mu.Unlock()
	w.wg.Wait()
}

func (w *Writer) run(iface string, q chan job) {
	defer w.wg.Done()
	isRadio := radio
	if w.Radio != nil {
		isRadio = w.Radio
	}
	sleep := w.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	first := true
	for j := range q {
		if !first && isRadio(iface) {
			sleep(w.gap())
		}
		first = false
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		var err error
		if j.set != nil {
			var values map[string]any
			if values, err = w.fit(ctx, iface, j.ref.Address, j.set); err == nil {
				_, err = w.Lite.Call(ctx, iface, "putParamset", j.ref.Address, "VALUES", values)
			}
		} else {
			_, err = w.Lite.Call(ctx, iface, "setValue", j.ref.Address, j.ref.Param, w.typed(ctx, j.ref, j.value))
		}
		cancel()
		j.done(err)
	}
}

// SetGap changes the pause between writes on a radio interface (the setting); zero or less is
// the default. It holds from the next write on.
func (w *Writer) SetGap(gap time.Duration) {
	w.mu.Lock()
	w.Gap = gap
	w.mu.Unlock()
}

func (w *Writer) gap() time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.Gap <= 0 {
		return DefaultGap
	}
	return w.Gap
}

// fit checks the values of one command against the channel's description as it is now - asked
// anew each time, so a device whose firmware changed its datapoints is refused here and said,
// not sent something it ignores - and gives each the form its datapoint takes: a datapoint
// the channel does not have, one that cannot be written, a number outside its limits, an entry
// its list does not have are errors, and nothing is sent. A channel the system has no
// description for gets the values as they are.
func (w *Writer) fit(ctx context.Context, iface, address string, set []model.ParamValue) (map[string]any, error) {
	values := map[string]any{}
	keys, sets, _, err := w.Lite.ParamsetsFor(ctx, iface, []string{address}, "VALUES")
	desc, known := sets[keys[address]]
	if err != nil || !known || len(desc) == 0 {
		for _, v := range set {
			values[v.Param], _ = Param{ID: v.Param}.Fit(v.Value)
		}
		return values, nil
	}
	for _, v := range set {
		d, ok := desc[v.Param]
		if !ok {
			return nil, fmt.Errorf("the channel has no datapoint %s", v.Param)
		}
		// (a description without operations says nothing about them)
		if d.Operations != 0 && d.Operations&2 == 0 {
			return nil, fmt.Errorf("%s cannot be written", v.Param)
		}
		fitted, err := paramOf(v.Param, d).Fit(v.Value)
		if err != nil {
			return nil, err
		}
		values[v.Param] = fitted
	}
	return values, nil
}

// typed gives a value the type its datapoint has: the interface processes refuse an integer
// for a FLOAT and a double for an INTEGER, and a program's file only says "0". The type comes
// from the channel's paramset description, asked once per channel; without one the value goes
// as it is.
func (w *Writer) typed(ctx context.Context, ref model.Ref, value any) any {
	key := ref.Channel()
	w.mu.Lock()
	_, asked := w.types[key+"/"]
	w.mu.Unlock()
	if !asked {
		found := map[string]string{}
		if keys, sets, _, err := w.Lite.ParamsetsFor(ctx, ref.Interface, []string{ref.Address}, "VALUES"); err == nil {
			for param, d := range sets[keys[ref.Address]] {
				found[param] = d.Type
			}
		}
		w.mu.Lock()
		w.types[key+"/"] = "asked"
		for param, t := range found {
			w.types[key+"/"+param] = t
		}
		w.mu.Unlock()
	}
	w.mu.Lock()
	typ := w.types[key+"/"+ref.Param]
	w.mu.Unlock()
	num, isNum := value.(float64)
	b, isBool := value.(bool)
	switch typ {
	case "FLOAT":
		if isBool {
			num, isNum = map[bool]float64{true: 1, false: 0}[b], true
		}
		if isNum {
			return litesource.Double(num)
		}
	case "INTEGER", "ENUM":
		if isBool {
			return map[bool]int{true: 1, false: 0}[b]
		}
		if isNum {
			return int(math.Round(num))
		}
	case "BOOL", "ACTION":
		if isNum {
			return num != 0
		}
	case "STRING":
		if isNum || isBool {
			return fmt.Sprint(value)
		}
	case "":
		if isNum && num != math.Trunc(num) {
			return litesource.Double(num)
		}
		if isNum {
			return int(num)
		}
	}
	return value
}
