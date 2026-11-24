package service

import (
	"runtime"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/soak"
)

// The daemon's memory under the soak's load (300 programs, 800 variables, two writes a second,
// every logged variable's history full): what it holds settles within the first hour - when
// the global log of runs is full - and stays there. It used to climb for eight hours, to
// thirty megabytes and more: every program kept its last two hundred runs (sixty thousand
// entries with three hundred programs), and every logged variable its five hundred changes.
func TestMemoryBoundedUnderLoad(t *testing.T) {
	r := newSoakRig(t, func(dir string) {
		tree, _ := soak.New(soak.Options{Programs: 300, Variables: 800})
		fullLogs(t, tree, dir)
	})
	held := func() uint64 {
		runtime.GC()
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	hours := func(n int) {
		for m := 0; m < n*60; m++ {
			r.minute(t)
		}
	}
	start := held()
	hours(1)
	first := held()
	if n := len(r.s.Engine().Trace(0)); n < engine.TraceKept {
		t.Fatalf("after an hour the global log has %d entries: the load does not fill it", n)
	}
	hours(3)
	last := held()
	t.Logf("held: %d kB at the start, %d kB after an hour, %d kB after four", start>>10, first>>10, last>>10)
	// the budget for the whole process is 20 MiB; the heap's share of it - with the runtime,
	// the program itself and what a collection leaves lying around beside it - is half
	const most = 10 << 20
	if last > most {
		t.Errorf("%d kB held after four hours, at most %d kB are meant to be", last>>10, most>>10)
	}
	if last > first+first/5 {
		t.Errorf("what is held grew from %d kB after an hour to %d kB after four", first>>10, last>>10)
	}
	runtime.KeepAlive(r)
	_ = r.s.Close()
}
