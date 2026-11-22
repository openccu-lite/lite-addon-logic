package engine

import (
	"sort"
	"sync"
	"time"
)

// FakeClock is a Clock a test moves by hand: time stands still until Advance or Set, and a
// timer's function runs inside the Advance that reaches it, on the caller's goroutine, in the
// order the timers are due. Deterministic to the nanosecond, which is what the tests of delays
// - and of the time modules' scheduler - need.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	seq    int
}

type fakeTimer struct {
	c    *FakeClock
	at   time.Time
	seq  int
	f    func()
	done bool
}

// NewFakeClock makes a clock that shows t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{now: t} }

// Now is the time the clock shows.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// AfterFunc arms a timer.
func (c *FakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	t := &fakeTimer{c: c, at: c.now.Add(d), seq: c.seq, f: f}
	c.timers = append(c.timers, t)
	return t
}

// Stop disarms the timer; false when it ran already or was stopped before.
func (t *fakeTimer) Stop() bool {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if t.done {
		return false
	}
	t.done = true
	return true
}

// Advance moves the clock forward by d and runs every timer that comes due on the way, each at
// its own time (Now inside the function is the timer's time), earliest first; a timer armed by
// such a function runs too when it is due within d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	for {
		var next *fakeTimer
		live := c.timers[:0]
		for _, t := range c.timers {
			if !t.done {
				live = append(live, t)
			}
		}
		c.timers = live
		sort.SliceStable(c.timers, func(i, j int) bool {
			if !c.timers[i].at.Equal(c.timers[j].at) {
				return c.timers[i].at.Before(c.timers[j].at)
			}
			return c.timers[i].seq < c.timers[j].seq
		})
		if len(c.timers) > 0 && !c.timers[0].at.After(target) {
			next = c.timers[0]
		}
		if next == nil {
			break
		}
		next.done = true
		if next.at.After(c.now) {
			c.now = next.at
		}
		c.mu.Unlock()
		next.f()
		c.mu.Lock()
	}
	c.now = target
	c.mu.Unlock()
}

// Set makes the clock show t without running anything: a jump of the clock, as an NTP
// correction is. Timers keep the time they were armed for; the next Advance runs those that are
// due by then.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.mu.Unlock()
}

// Jump moves the clock by d - forward or back - the way a correction of the system's clock
// does: nothing runs, and every armed timer keeps the time it still had to wait (a real timer
// counts on a clock that does not jump), so it is now due at another reading of the clock.
func (c *FakeClock) Jump(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		t.at = t.at.Add(d)
	}
	c.mu.Unlock()
}

// Pending counts the armed timers.
func (c *FakeClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.done {
			n++
		}
	}
	return n
}
