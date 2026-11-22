package system

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Source is the slice of the kit's client the feed uses.
type Source interface {
	State(ctx context.Context, f litesource.Filter) (*litesource.Snapshot, error)
	Events(ctx context.Context, f litesource.Filter, lastEventID string) <-chan litesource.Message
}

// Sink is the engine's side: what the programs read, and where values go.
type Sink interface {
	Refs() []model.Ref
	SeedDevice(ref model.Ref, value any)
	DeviceEvent(ref model.Ref, value any)
}

// Feed brings the values of the datapoints the programs read into the engine: the state store
// as a seed - never an event - and then the event stream, filtered to those datapoints' channels
// and parameters. When the stream loses its place it seeds again. The addon's own interface is
// never read this way. Nothing is polled and no device is asked: a datapoint the system has no
// value for stays unknown until it reports.
type Feed struct {
	Lite Source
	Sink Sink
	// Own is the addon's own interface, left out ("Logic").
	Own string
	// Retry is how long the feed waits after a failed seed (10 s when zero).
	Retry time.Duration
	Log   func(format string, args ...any)

	mu     sync.Mutex
	kick   chan struct{}
	seeded int
}

// Kick tells the feed that the programs changed: it looks at what they read now and, when that
// differs, starts over with a new filter.
func (f *Feed) Kick() {
	f.mu.Lock()
	k := f.kick
	f.mu.Unlock()
	if k != nil {
		select {
		case k <- struct{}{}:
		default:
		}
	}
}

// Seeded counts the seeds done (a test's and the page's sign of life).
func (f *Feed) Seeded() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seeded
}

func (f *Feed) logf(format string, args ...any) {
	if f.Log != nil {
		f.Log(format, args...)
	}
}

// filter builds the stream's filter from the datapoints, and the set to test events against.
func (f *Feed) filter() (litesource.Filter, map[model.Ref]bool) {
	want := map[model.Ref]bool{}
	ifaces, addrs, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, r := range f.Sink.Refs() {
		if r.Interface == f.Own {
			continue
		}
		want[r] = true
		ifaces[r.Interface], addrs[r.Address], keys[r.Param] = true, true, true
	}
	list := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	return litesource.Filter{Interfaces: list(ifaces), Addresses: list(addrs), Keys: list(keys), Types: []string{"event", "state", "resync"}}, want
}

func same(a, b map[model.Ref]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func decode(raw json.RawMessage) (any, bool) {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	switch v.(type) {
	case bool, float64, string:
		return v, true
	}
	return nil, false
}

// Run feeds until ctx ends.
func (f *Feed) Run(ctx context.Context) {
	f.mu.Lock()
	f.kick = make(chan struct{}, 1)
	kick := f.kick
	f.mu.Unlock()
	retry := f.Retry
	if retry <= 0 {
		retry = 10 * time.Second
	}
	for ctx.Err() == nil {
		filter, want := f.filter()
		if len(want) == 0 {
			// nothing to read: wait for the programs to change
			select {
			case <-ctx.Done():
			case <-kick:
			}
			continue
		}
		sctx, cancel := context.WithCancel(ctx)
		lastID, ok := f.seed(sctx, filter, want)
		if !ok {
			cancel()
			select {
			case <-ctx.Done():
			case <-kick:
			case <-time.After(retry):
			}
			continue
		}
		events := f.Lite.Events(sctx, filter, lastID)
	stream:
		for {
			select {
			case <-ctx.Done():
				break stream
			case <-kick:
				if _, now := f.filter(); !same(want, now) {
					break stream
				}
			case m, open := <-events:
				if !open {
					break stream
				}
				switch {
				case m.Resync != nil:
					f.logf("the event stream lost its place (%s): reading the values again", m.Resync.Reason)
					if _, ok := f.seed(sctx, filter, want); !ok {
						break stream
					}
				case m.Event != nil:
					ref := model.Ref{Interface: m.Event.Interface, Address: m.Event.Address, Param: m.Event.Key}
					v, ok := decode(m.Event.Value)
					if !want[ref] || !ok {
						continue
					}
					if m.Type == "state" {
						// the store's sweep: what it read, not something that happened
						f.Sink.SeedDevice(ref, v)
					} else {
						f.Sink.DeviceEvent(ref, v)
					}
				}
			}
		}
		cancel()
	}
}

// seed reads the state store for the filter and hands the values to the engine as seeds.
func (f *Feed) seed(ctx context.Context, filter litesource.Filter, want map[model.Ref]bool) (string, bool) {
	snap, err := f.Lite.State(ctx, filter)
	if err != nil {
		if ctx.Err() == nil {
			f.logf("the datapoints' values could not be read: %v", err)
		}
		return "", false
	}
	for _, e := range snap.Entries {
		ref := model.Ref{Interface: e.Interface, Address: e.Address, Param: e.Key}
		if v, ok := decode(e.Value); ok && want[ref] {
			f.Sink.SeedDevice(ref, v)
		}
	}
	f.mu.Lock()
	f.seeded++
	f.mu.Unlock()
	return snap.EventID, true
}
