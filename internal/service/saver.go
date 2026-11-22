package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// What goes to storage while the addon runs, and how often:
//
//   - The state file - every variable's value and time, the programs' line truths: about
//     seventy kilobytes with eight hundred variables and three hundred programs, whatever
//     they did - when something changed, at most every StateEvery, and at the stop.
//   - The run trace's new entries and the logged variables' new changes, appended to their
//     files (trace.jsonl, log.jsonl) at the state's pace: bytes that follow what happened,
//     not what is kept (varlog.go, store.Ring, store.Journal).
//   - The delayed activities, in a small file of their own beside it (pending.json): a delay
//     must survive a power cut, so it cannot wait for the state file's turn - but a system
//     whose programs schedule delays all the time would otherwise write the whole state every
//     few seconds. This file is written PendingEvery after the delays changed, however often
//     they changed meanwhile, and only when its content differs from what is there.
//
// The pending file is always written before the state file, so it is never the older of the
// two: at a start its list is the one that counts. Without it (a first start with this
// version) the state file's own list is used.
const (
	StateEvery   = 30 * time.Second
	PendingEvery = 5 * time.Second
)

// pendingFile is pending.json.
type pendingFile struct {
	Pending []engine.Pending `json:"pending"`
}

// Written counts what went to storage since the start: the files written and their bytes.
type Written struct {
	StateFiles   int   `json:"state_files"`
	StateBytes   int64 `json:"state_bytes"`
	PendingFiles int   `json:"pending_files"`
	PendingBytes int64 `json:"pending_bytes"`
	// LogBytes are the logged variables' journal: the appended lines and its rewrites.
	// TraceBytes the run trace's ring, likewise.
	LogBytes   int64 `json:"log_bytes"`
	TraceBytes int64 `json:"trace_bytes"`
}

// saver is the service's part that writes the two files.
type saver struct {
	mu      sync.Mutex
	last    []byte // the pending file as it was written or read last
	written Written
}

func (s *Service) pendingPath() string { return filepath.Join(s.o.Dir, "state", "pending.json") }

// Written answers what went to storage since the start.
func (s *Service) Written() Written {
	s.saver.mu.Lock()
	defer s.saver.mu.Unlock()
	return s.saver.written
}

// writePending writes the list when it differs from the file's.
func (s *Service) writePending(list []engine.Pending) error {
	if list == nil {
		list = []engine.Pending{}
	}
	data, err := json.Marshal(pendingFile{Pending: list})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.saver.mu.Lock()
	defer s.saver.mu.Unlock()
	if bytes.Equal(data, s.saver.last) {
		return nil
	}
	if err := store.WriteFile(s.pendingPath(), data, 0o600); err != nil {
		return err
	}
	s.saver.last = data
	s.saver.written.PendingFiles++
	s.saver.written.PendingBytes += int64(len(data))
	return nil
}

// SavePending writes the delayed activities that are not due yet into their file, when the
// list changed since it was written.
func (s *Service) SavePending() error {
	return s.writePending(s.eng.PendingActions())
}

// loadPending answers the delayed activities of the pending file; ok is false when there is
// no such file or it cannot be read (the state file's list is used then).
func (s *Service) loadPending() (list []engine.Pending, ok bool) {
	data, err := os.ReadFile(s.pendingPath())
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.logf("the file of the delayed activities cannot be read (%v): using the state file's", err)
		}
		return nil, false
	}
	var f pendingFile
	if err := json.Unmarshal(data, &f); err != nil {
		s.logf("the file of the delayed activities is damaged (%v): using the state file's", err)
		return nil, false
	}
	s.saver.mu.Lock()
	s.saver.last = data
	s.saver.mu.Unlock()
	return f.Pending, true
}

// RunSaver writes to storage until ctx ends: the state when it changed, every stateEvery; the
// delayed activities pendingEvery after they changed. (The stop's own write is Close's.)
func (s *Service) RunSaver(ctx context.Context, stateEvery, pendingEvery time.Duration) {
	tick := time.NewTicker(stateEvery)
	defer tick.Stop()
	var soon <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.saveSoon:
			// (further changes until then are the same write)
			if soon == nil {
				soon = time.After(pendingEvery)
			}
		case <-soon:
			soon = nil
			if err := s.SavePending(); err != nil {
				s.logf("the delayed activities could not be written: %v", err)
			}
		case <-tick.C:
			if err := s.SaveState(false); err != nil {
				s.logf("the state could not be written: %v", err)
			}
		}
	}
}
