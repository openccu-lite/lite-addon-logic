package system

import (
	"context"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// CounterSink is the engine's side of the counters' maxima: which counter datapoints its
// device helpers follow, and where a maximum goes.
type CounterSink interface {
	HelperCounters() []model.Ref
	SetCounterMaximum(ref model.Ref, max float64)
}

// ParamSource answers the datapoints of channels (Catalog.Params).
type ParamSource interface {
	Params(ctx context.Context, channels []string) (map[string][]Param, error)
}

// Maxima tells the engine the maximum of every counter datapoint a device helper follows, from
// the devices' descriptions: what a counter had reached when it began anew. It asks when the
// helpers change (Kick) and again every Retry while a maximum is missing - a device that is not
// there yet, an interface that is still starting.
type Maxima struct {
	Params ParamSource
	Sink   CounterSink
	// Retry is how long it waits before it asks again for what is missing (a minute when zero).
	Retry time.Duration
	Log   func(format string, args ...any)

	mu   sync.Mutex
	kick chan struct{}
	done map[model.Ref]float64
}

// Kick tells it that the helpers may have changed.
func (m *Maxima) Kick() {
	m.mu.Lock()
	k := m.kick
	m.mu.Unlock()
	if k != nil {
		select {
		case k <- struct{}{}:
		default:
		}
	}
}

// Known answers the maxima told so far.
func (m *Maxima) Known() map[model.Ref]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[model.Ref]float64, len(m.done))
	for k, v := range m.done {
		out[k] = v
	}
	return out
}

// resolve asks for the maxima that are missing; it answers how many still are.
func (m *Maxima) resolve(ctx context.Context) int {
	var missing []model.Ref
	channels := map[string]bool{}
	m.mu.Lock()
	for _, ref := range m.Sink.HelperCounters() {
		if _, ok := m.done[ref]; !ok {
			missing = append(missing, ref)
			channels[ref.Channel()] = true
		}
	}
	m.mu.Unlock()
	if len(missing) == 0 {
		return 0
	}
	list := make([]string, 0, len(channels))
	for ch := range channels {
		list = append(list, ch)
	}
	params, err := m.Params.Params(ctx, list)
	if err != nil {
		if ctx.Err() == nil && m.Log != nil {
			m.Log("the counters' maxima could not be read: %v", err)
		}
		return len(missing)
	}
	left := 0
	for _, ref := range missing {
		found := false
		for _, p := range params[ref.Channel()] {
			if p.ID == ref.Param && p.Max != nil {
				m.Sink.SetCounterMaximum(ref, *p.Max)
				m.mu.Lock()
				m.done[ref] = *p.Max
				m.mu.Unlock()
				found = true
			}
		}
		// (a helper of the HmIP-ESI follows two counters and its channel has one of them: a
		// channel that answered without the datapoint is not asked again until the helpers change)
		if !found && params[ref.Channel()] == nil {
			left++
		}
	}
	return left
}

// Run resolves until ctx ends.
func (m *Maxima) Run(ctx context.Context) {
	m.mu.Lock()
	m.kick = make(chan struct{}, 1)
	m.done = map[model.Ref]float64{}
	kick := m.kick
	m.mu.Unlock()
	retry := m.Retry
	if retry <= 0 {
		retry = time.Minute
	}
	for ctx.Err() == nil {
		var again <-chan time.Time
		if m.resolve(ctx) > 0 {
			again = time.After(retry)
		}
		select {
		case <-ctx.Done():
		case <-kick:
		case <-again:
		}
	}
}
