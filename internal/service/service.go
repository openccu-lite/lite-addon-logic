// Package service is the addon's one place where things change: it keeps the tree of files, the
// history, the engine and the state file in step. The page's API, the interface and the import
// all go through it; nothing else writes a file or loads the engine.
//
// A change is: validate, write the file, load the engine with the new set, hand the paths to
// the history. The file is written before anything else knows; the history comes last and in
// the background, and a save never fails with it.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/schedule"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// Options configure a service.
type Options struct {
	// Dir is the data directory: tree/ (the files and their history) and state/ below it.
	Dir string
	// Clock is the engine's time (the system's when nil); Location the time zone of the time
	// modules (the system's when nil); ClockReady says whether the system's clock is set (see
	// engine.Options).
	Clock      engine.Clock
	Location   *time.Location
	ClockReady func() bool
	// Notify is the engine's listener (see engine.Options).
	Notify func(engine.Notification)
	// Writer sends the programs' device writes (see engine.Options).
	Writer engine.Writer
	// Caller carries out the programs' webhooks and Wake-on-LAN (see engine.Options); it
	// asks Service.Secret for the webhooks' secrets.
	Caller engine.Caller
	// Names answers a device's name for the comments in program files (nil: no comments).
	Names model.Namer
	// Pacer is told the pause between two write commands on a radio interface when the
	// settings are read or changed (nil: nobody to tell).
	Pacer func(gap time.Duration)
	// Log receives a line per noteworthy event (nil: silent).
	Log func(format string, args ...any)
}

// closeWait is how long a stop waits for a push that is under way.
var closeWait = 5 * time.Second

// outside is the history's text for what was not changed through the service.
const outside = "Changed outside the page"

// Error is a change that was refused, with a code the page can act on.
type Error struct {
	// Code: not-found, exists, invalid, in-use, read-only.
	Code string
	Msg  string
	// Users are the programs that stand in the way (in-use).
	Users []string
}

func (e *Error) Error() string { return e.Msg }

func refuse(code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Service is the addon's state behind one lock.
type Service struct {
	o     Options
	mu    sync.Mutex
	tree  *store.Tree
	queue *repo.Queue
	eng   *engine.Engine
	state store.StateFile
	// trace is the run trace's ring on disk; traceSaved the number of the newest entry in it
	trace      store.Ring
	traceMu    sync.Mutex
	traceSaved uint64
	// logs is the journal of the logged variables' changes (varlog.go); logWait holds lines
	// that could not be written
	logs    store.Journal
	logMu   sync.Mutex
	logWait [][]byte
	set     *model.Set
	bad     []store.FileError
	// settings are the tree's settings.yaml
	settings *model.Settings
	// secrets are the webhooks' secrets, by name (behind their own lock: the webhooks'
	// workers ask for them)
	secMu   sync.Mutex
	secrets map[string]string
	// remote is the history's remote (remote.json), nil when there is none; the rest is its
	// state. Behind its own lock: the push worker and the host key callback use it.
	remMu         sync.Mutex
	remote        *remoteFile
	remotePending bool
	remoteWanted  uint64
	remotePushed  time.Time
	remoteErr     string
	remoteErrCode string
	remoteRetry   time.Duration
	pushWake      chan struct{}
	pushDone      chan struct{}
	// notices: what the user should know about the data (a damaged state file, a history
	// that could not be written), newest last
	stateNotice   string
	historyNotice string
	// onChange is told after a change to the set of programs or variables (a variable or
	// program came, went, or was renamed): the interface announces devices from it.
	onChange []func()
	subMu    sync.Mutex
	subs     []func(engine.Notification)
	// saveSoon is signalled when the delayed activities changed
	saveSoon chan struct{}
	// saver writes the state file and the file of the delayed activities (saver.go)
	saver saver
	// switches are programs a program's activity switched on or off: stored by a worker,
	// since the engine tells it while it is locked
	switches chan switchReq
	switched chan struct{}
}

type switchReq struct {
	key    string
	active bool
	// done is closed when the switch is stored (a test's and Flush's way to wait)
	done chan struct{}
}

// applySwitches stores what programs' activities switched.
func (s *Service) applySwitches() {
	defer close(s.switched)
	for req := range s.switches {
		if req.done != nil {
			close(req.done)
			continue
		}
		if _, err := s.SetActive(req.key, req.active, repo.Author{Name: "program"}); err != nil {
			s.logf("a program's activity could not switch %s: %v", req.key, err)
		}
	}
}

// Open loads the tree, the history and the state, and makes the engine.
func Open(o Options) (*Service, error) {
	// what a power cut in the middle of a write left behind goes first: nothing reads it
	stale, err := store.RemoveStale(o.Dir)
	if o.Log != nil {
		if err != nil {
			o.Log("looking for files a cut-off write left: %v", err)
		}
		if len(stale) > 0 {
			o.Log("removed %d temporary files a cut-off write left (%s)", len(stale), strings.Join(stale, ", "))
		}
	}
	tree, err := store.Open(filepath.Join(o.Dir, "tree"))
	if err != nil {
		return nil, err
	}
	s := &Service{o: o, tree: tree, state: store.StateFile{Path: filepath.Join(o.Dir, "state", "state.json")},
		trace: store.Ring{Path: filepath.Join(o.Dir, "state", "trace.jsonl"), Max: engine.TraceKept}}
	s.logs = newLogJournal(s.logPath())
	if _, err := store.Open(filepath.Join(o.Dir, "state")); err != nil {
		return nil, err
	}
	rp, err := repo.Open(tree.Dir)
	if err != nil {
		// the files are the truth, the history a convenience: a repository that does not
		// open is moved aside and started anew
		aside := filepath.Join(o.Dir, "history-broken-"+time.Now().UTC().Format("20060102-150405"))
		if mvErr := renameGit(tree.Dir, aside); mvErr != nil {
			return nil, fmt.Errorf("%w (and it could not be moved aside: %v)", err, mvErr)
		}
		s.logf("the history could not be opened (%v): moved aside to %s and started anew", err, filepath.Base(aside))
		s.historyNotice = "The history could not be opened and was started anew; the old one is kept beside it."
		if rp, err = repo.Open(tree.Dir); err != nil {
			return nil, err
		}
	}
	if o.Clock != nil {
		rp.Now = o.Clock.Now
	}
	s.queue = repo.NewQueue(rp)
	s.queue.OnError = func(err error) {
		s.mu.Lock()
		s.historyNotice = "The last change could not be written to the history: " + err.Error()
		s.mu.Unlock()
		s.logf("history: %v", err)
	}
	s.queue.OnCommit = func(string, string) {
		s.mu.Lock()
		s.historyNotice = ""
		s.mu.Unlock()
		s.pushSoon()
	}
	s.remoteRetry = RemoteRetry
	s.pushWake, s.pushDone = make(chan struct{}, 1), make(chan struct{})
	s.loadRemote()
	go s.pushWorker()
	s.saveSoon = make(chan struct{}, 1)
	s.eng = engine.New(engine.Options{Clock: o.Clock, Location: o.Location, ClockReady: o.ClockReady, Writer: o.Writer, Caller: o.Caller, Notify: s.fanOut, Log: o.Log})
	s.loadSecrets()
	s.switches = make(chan switchReq, 64)
	s.switched = make(chan struct{})
	go s.applySwitches()
	if err := s.reload(); err != nil {
		return nil, err
	}
	// the run trace first: what the state brings along (a delay that came due while the addon
	// was stopped) is filed after it
	s.loadTrace()
	data, gen, damaged := s.state.Load(func(b []byte) bool { _, err := engine.UnmarshalState(b); return err == nil })
	switch {
	case data != nil:
		st, _ := engine.UnmarshalState(data)
		// the delayed activities have a file of their own, never older than the state's
		if list, ok := s.loadPending(); ok {
			st.Pending = list
		}
		s.moveLogs(st)
		s.eng.Import(st)
		if gen > 0 {
			s.stateNotice = "The state file was damaged; the one before it was used. Values written since then are lost."
			s.logf("the state file is damaged: using the generation before")
		}
	case damaged:
		s.stateNotice = "The state file was damaged and could not be read; the variables have their default values."
		s.logf("the state file and the one before it are damaged: starting with default values")
		fallthrough
	default:
		// no state to read: the delayed activities still have their own file
		if list, ok := s.loadPending(); ok && len(list) > 0 {
			s.eng.Import(engine.State{Pending: list})
		}
	}
	// what is in the tree and not in the history yet (files put there by hand while the addon
	// was stopped): recorded now, before anything else changes the tree
	if _, err := rp.Commit(nil, repo.Author{}, outside); err != nil {
		s.historyNotice = "The last change could not be written to the history: " + err.Error()
		s.logf("history: %v", err)
	}
	return s, nil
}

func (s *Service) logf(format string, args ...any) {
	if s.o.Log != nil {
		s.o.Log(format, args...)
	}
}

// reload reads the tree and loads the engine; called with the lock held or before the service
// is shared.
func (s *Service) reload() error {
	set, bad, err := s.tree.Load()
	if err != nil {
		return err
	}
	s.set, s.bad = set, bad
	for _, b := range bad {
		s.logf("%v", b.Err)
	}
	s.settings = &model.Settings{}
	if data, err := s.tree.Read(model.SettingsPath); err == nil {
		if st, err := model.DecodeSettings(data); err != nil {
			s.logf("%v: the settings are not used", err)
		} else {
			s.settings = st
		}
	}
	s.applySettings(s.settings)
	for _, k := range sortedVariables(set.Variables) {
		if problem := set.ComputedProblem(set.Variables[k]); problem != "" {
			s.logf("variable %s is not computed: %s", k, problem)
		}
		if problem := set.HelperProblem(set.Variables[k]); problem != "" {
			s.logf("variable %s is not kept by its device helper: %s", k, problem)
		}
	}
	s.eng.SetVariables(set.Variables)
	s.programsChanged()
	return nil
}

func position(st *model.Settings) schedule.Position {
	if st.Location == nil {
		return schedule.Position{}
	}
	return schedule.Position{Lat: st.Location.Latitude, Lon: st.Location.Longitude, Known: true}
}

// applySettings hands the settings to who runs by them: the location to the time modules, the
// loop guard to the engine, the pause between write commands to the writer.
func (s *Service) applySettings(st *model.Settings) {
	s.eng.SetPosition(position(st))
	s.eng.UpdateSun()
	g := st.Guard()
	s.eng.SetLoopGuard(engine.LoopGuard{Runs: g.Runs, Window: time.Duration(g.WindowSeconds) * time.Second, Pause: time.Duration(g.PauseSeconds) * time.Second})
	if s.o.Pacer != nil {
		s.o.Pacer(time.Duration(st.Pacing()) * time.Millisecond)
	}
}

// Settings answers the addon's settings.
func (s *Service) Settings() model.Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.settings
}

// SetSettings stores the settings; they apply at once: the astro time modules follow a new
// location, the loop guard and the pause between write commands their new figures.
func (s *Service) SetSettings(st model.Settings, author repo.Author) error {
	if err := st.Check(); err != nil {
		return &Error{Code: "invalid", Msg: err.Error()}
	}
	s.mu.Lock()
	changed, err := s.tree.WriteExtra(model.SettingsPath, model.EncodeSettings(&st))
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.settings = &st
	s.applySettings(&st)
	s.mu.Unlock()
	if changed {
		s.queue.Enqueue([]string{model.SettingsPath}, author, "Settings changed")
	}
	return nil
}

// Reload reads the tree again: after files were changed by hand, or pulled.
func (s *Service) Reload() error {
	s.mu.Lock()
	err := s.reload()
	s.mu.Unlock()
	if err == nil {
		s.queue.Enqueue(nil, repo.Author{}, outside)
		s.changed()
	}
	return err
}

// OnChange registers a function called after the set of programs or variables changed. It may
// be called from the service's own worker: it must not wait for the service (no Flush).
func (s *Service) OnChange(f func()) {
	s.mu.Lock()
	s.onChange = append(s.onChange, f)
	s.mu.Unlock()
}

func (s *Service) changed() {
	s.mu.Lock()
	fs := append([]func(){}, s.onChange...)
	s.mu.Unlock()
	for _, f := range fs {
		f()
	}
}

// Subscribe registers a listener for what the engine tells: a variable was written
// (Kind "variable": Key, Value, Changed), a program ran or was switched (Kind "program"). The
// listener is called in order, with the engine locked: it must hand the notification on (a
// channel, a queue) and return - never call the engine or the service from it.
func (s *Service) Subscribe(f func(engine.Notification)) {
	s.subMu.Lock()
	s.subs = append(s.subs, f)
	s.subMu.Unlock()
}

func (s *Service) fanOut(n engine.Notification) {
	if n.Kind == "pending" {
		// the state file should follow soon: a delay must survive a power cut
		select {
		case s.saveSoon <- struct{}{}:
		default:
		}
		return
	}
	if n.Kind == "program" && n.Active != nil {
		// a program's activity switched a program: the file must say so too
		select {
		case s.switches <- switchReq{key: n.Key, active: *n.Active}:
		default:
			s.logf("too many program switches at once: the switch of %s is lost", n.Key)
		}
	}
	if s.o.Notify != nil {
		s.o.Notify(n)
	}
	s.subMu.Lock()
	subs := s.subs
	s.subMu.Unlock()
	for _, f := range subs {
		f(n)
	}
}

// Engine is the running engine: values, writes, alarms.
func (s *Service) Engine() *engine.Engine { return s.eng }

// Tree is the tree of files.
func (s *Service) Tree() *store.Tree { return s.tree }

// Flush waits for what programs switched to be stored and for the history to have tried what
// is queued (tests, and before a stop).
func (s *Service) Flush() {
	done := make(chan struct{})
	s.switches <- switchReq{done: done}
	<-done
	s.queue.Flush()
}

// History answers the history of the tree, or of one file in it.
func (s *Service) History(path string, limit int) ([]repo.Entry, error) {
	return s.queue.Repo().History(path, limit)
}

// Notices lists what the user should know about the data: files that were not taken, a damaged
// state file, a history that could not be written.
type Notices struct {
	// BadFiles are files of the tree the reader refused, with the reason.
	BadFiles []BadFile `json:"bad_files,omitempty"`
	State    string    `json:"state,omitempty"`
	History  string    `json:"history,omitempty"`
	// Remote says the last push to the history's remote did not work.
	Remote string `json:"remote,omitempty"`
}

// BadFile is one refused file.
type BadFile struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Notices answers them.
func (s *Service) Notices() Notices {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := Notices{State: s.stateNotice, History: s.historyNotice}
	s.remMu.Lock()
	if s.remote != nil && s.remotePending {
		n.Remote = s.remoteErr
	}
	s.remMu.Unlock()
	for _, b := range s.bad {
		n.BadFiles = append(n.BadFiles, BadFile{Path: b.Path, Error: b.Error()})
	}
	return n
}

// SaveSoon is signalled when the delayed activities changed: RunSaver then writes their file
// within seconds instead of at its next regular turn, so that a delay survives a power cut.
func (s *Service) SaveSoon() <-chan struct{} { return s.saveSoon }

// SaveState writes the state file when the state changed, or always with force (at a stop).
func (s *Service) SaveState(force bool) error {
	terr := errors.Join(s.saveTrace(force), s.saveLogs())
	st, dirty := s.eng.Export()
	if !dirty && !force {
		return terr
	}
	b, err := engine.MarshalState(st)
	if err != nil {
		return err
	}
	// the delayed activities first: their file is never the older of the two
	perr := s.writePending(st.Pending)
	err = s.state.Save(b)
	if err == nil {
		s.saver.mu.Lock()
		s.saver.written.StateFiles++
		s.saver.written.StateBytes += int64(len(b))
		s.saver.mu.Unlock()
	}
	return errors.Join(err, perr, terr)
}

// traceSettle is how old an entry of the run trace is before it goes to disk: by then the
// interface has answered its writes, and the entry says so.
const traceSettle = 10 * time.Second

// loadTrace gives the engine the run trace the ring on disk kept. A line that is no entry (a
// write cut short) is left out.
func (s *Service) loadTrace() {
	lines, err := s.trace.Load()
	if err != nil {
		s.logf("the run trace could not be read: %v", err)
		return
	}
	runs := make([]engine.Run, 0, len(lines))
	for _, l := range lines {
		var r engine.Run
		if json.Unmarshal(l, &r) == nil && r.Seq > s.traceSaved {
			runs = append(runs, r)
			s.traceSaved = r.Seq
		}
	}
	s.eng.ImportTrace(runs)
}

// saveTrace appends the run trace's new entries to the ring on disk: those that are settled,
// or all of them (at a stop).
func (s *Service) saveTrace(all bool) error {
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	cut := s.eng.Now().Add(-traceSettle)
	var lines [][]byte
	last := s.traceSaved
	for _, r := range s.eng.Trace(s.traceSaved) {
		if !all && r.Time.After(cut) {
			break
		}
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		lines = append(lines, b)
		last = r.Seq
	}
	before := s.trace.Written()
	err := s.trace.Append(lines)
	s.saver.mu.Lock()
	s.saver.written.TraceBytes += s.trace.Written() - before
	s.saver.mu.Unlock()
	if err != nil {
		return err
	}
	s.traceSaved = last
	return nil
}

// SystemStart tells the service the system's boot id (the kernel's, new at every boot). At the
// first start of the addon after a boot of the system - not at a restart or an update of the
// addon - the variables with a "Wert beim Systemstart" get it. Answers whether this was such a
// start; an empty id is never one.
func (s *Service) SystemStart(bootID string) bool {
	if bootID == "" {
		return false
	}
	file := filepath.Join(s.o.Dir, "state", "boot-id")
	if last, err := os.ReadFile(file); err == nil && strings.TrimSpace(string(last)) == bootID {
		return false
	}
	s.eng.ApplyStartValues()
	if err := store.WriteFile(file, []byte(bootID+"\n"), 0o600); err != nil {
		s.logf("the boot id could not be stored: %v", err)
	}
	return true
}

// StartEvaluation is the evaluation after a boot of the system (concept §6, Q-12): once per
// boot id - not at a restart or an update of the addon - every active program whose "beim
// Systemstart prüfen" is on is evaluated, as the CCU evaluates its programs when it starts.
// It waits until ready says yes (the devices' values are read, the clock is set) or wait has
// passed, asking every poll; then it evaluates, notes the boot id as done and answers the
// programs' keys. An addon stopped while it waited evaluates at its next start. first is
// what SystemStart answered for this boot id: a start that is not the first one after the boot
// and finds no note of any evaluation is an update from a version that kept none - the system
// did not start, so nothing is evaluated (and the id is noted). Answers nil and false when
// this boot's evaluation was done already or is not due, the id is empty, or ctx ended.
func (s *Service) StartEvaluation(ctx context.Context, bootID string, first bool, ready func() bool, wait, poll time.Duration) ([]string, bool) {
	if bootID == "" {
		return nil, false
	}
	file := filepath.Join(s.o.Dir, "state", "start-evaluated")
	last, err := os.ReadFile(file)
	if err == nil && strings.TrimSpace(string(last)) == bootID {
		return nil, false
	}
	if err != nil && !first {
		if err := store.WriteFile(file, []byte(bootID+"\n"), 0o600); err != nil {
			s.logf("the boot id could not be stored: %v", err)
		}
		return nil, false
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	late := false
	for !late && !ready() {
		select {
		case <-ctx.Done():
			return nil, false
		case <-deadline.C:
			late = true
		case <-tick.C:
		}
	}
	if late {
		unknown := s.eng.StartUnknown()
		more := ""
		if len(unknown) > 5 {
			more, unknown = fmt.Sprintf(" and %d more", len(unknown)-5), unknown[:5]
		}
		s.logf("after the system's start: not everything was ready after %v (the clock, the devices' values; without a value: %s%s) - the programs are evaluated with what is known", wait, strings.Join(unknown, ", "), more)
	}
	keys := s.eng.EvaluateAtStart()
	if err := store.WriteFile(file, []byte(bootID+"\n"), 0o600); err != nil {
		s.logf("the boot id could not be stored: %v", err)
	}
	s.logf("after the system's start: %d programs evaluated once", len(keys))
	return keys, true
}

// Close writes the state and ends the history's worker after what is queued.
func (s *Service) Close() error {
	s.Flush()
	close(s.switches)
	<-s.switched
	err := s.SaveState(true)
	s.queue.Close()
	// a push under way to a remote that does not answer is not waited for: the next start
	// pushes what is left
	close(s.pushWake)
	select {
	case <-s.pushDone:
	case <-time.After(closeWait):
	}
	return err
}

// --- variables ---

// Variables lists the definitions, sorted by name.
func (s *Service) Variables() []*model.Variable {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*model.Variable, 0, len(s.set.Variables))
	for _, v := range s.set.Variables {
		out = append(out, v.Clone())
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name); a != b {
			return a < b
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// VariableUsers lists the programs that use a variable.
func (s *Service) VariableUsers(key string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set.VariableUsers(key)
}

// CreateVariable makes a new variable; its key is made from the name. initial is its first
// value (nil: the type's default).
func (s *Service) CreateVariable(def *model.Variable, initial any, author repo.Author) (*model.Variable, error) {
	s.mu.Lock()
	v := def.Clone()
	v.Key = model.UniqueKey(v.Name, func(k string) bool {
		return s.set.Variables[k] != nil || model.IsSystemValue(k)
	})
	checked, err := v.Check()
	if err != nil {
		s.mu.Unlock()
		return nil, &Error{Code: "invalid", Msg: plainError(err)}
	}
	if initial != nil {
		if err := checked.CheckValue(initial); err != nil {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: err.Error()}
		}
	}
	if checked.Computed != nil {
		initial = nil
		if problem := s.set.ComputedProblem(checked); problem != "" {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: problem}
		}
	}
	if checked.Helper != nil {
		tentative := &model.Set{Variables: map[string]*model.Variable{checked.Key: checked}}
		for k, v := range s.set.Variables {
			tentative.Variables[k] = v
		}
		if problem := tentative.HelperProblem(checked); problem != "" {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: problem}
		}
	}
	path, _, err := s.tree.SaveVariable(checked)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.set.Variables[checked.Key] = checked
	s.eng.SetVariables(s.set.Variables)
	if initial != nil {
		_ = s.eng.SetVariable(checked.Key, initial, engine.FromImport)
	}
	s.mu.Unlock()
	s.queue.Enqueue([]string{path}, author, "Variable "+checked.Name+" created")
	s.changed()
	return checked.Clone(), nil
}

// UpdateVariable replaces a variable's definition. The CCU lets a changed value list or type
// silently change what existing program lines mean; here a type that is in use cannot be
// changed, an entry that is in use cannot be removed, and an entry renamed through renames
// (old label to new) is renamed in the programs that name it and in the variable's own value.
func (s *Service) UpdateVariable(key string, def *model.Variable, renames map[string]string, author repo.Author) (*model.Variable, error) {
	return s.updateVariable(key, def, renames, author, "changed")
}

// updateVariable is UpdateVariable with what the history says happened to the variable.
func (s *Service) updateVariable(key string, def *model.Variable, renames map[string]string, author repo.Author, what string) (*model.Variable, error) {
	s.mu.Lock()
	old := s.set.Variables[key]
	if old == nil {
		s.mu.Unlock()
		return nil, refuse("not-found", "there is no variable %s", key)
	}
	v := def.Clone()
	v.Key = key
	checked, err := v.Check()
	if err != nil {
		s.mu.Unlock()
		return nil, &Error{Code: "invalid", Msg: plainError(err)}
	}
	if users := s.set.VariableUsers(key); checked.Type != old.Type && len(users) > 0 {
		s.mu.Unlock()
		return nil, &Error{Code: "in-use", Msg: fmt.Sprintf("the type of %s cannot be changed while programs use it", old.Name), Users: users}
	}
	// what the change would do to the computed values: this one's own inputs, and the ones it feeds
	tentative := &model.Set{Programs: s.set.Programs, Variables: map[string]*model.Variable{}}
	for k, v := range s.set.Variables {
		tentative.Variables[k] = v
	}
	tentative.Variables[key] = checked
	if problem := tentative.ComputedProblem(checked); problem != "" {
		s.mu.Unlock()
		return nil, &Error{Code: "invalid", Msg: problem}
	}
	for _, k := range sortedVariables(tentative.Variables) {
		if problem := tentative.ComputedProblem(tentative.Variables[k]); problem != "" && s.set.ComputedProblem(s.set.Variables[k]) == "" {
			s.mu.Unlock()
			return nil, &Error{Code: "in-use", Msg: problem, Users: tentative.ComputedUsers(key)}
		}
	}
	// and to the device helpers: this variable's own place in one, and the ones it leads
	for _, k := range sortedVariables(tentative.Variables) {
		if problem := tentative.HelperProblem(tentative.Variables[k]); problem != "" && (k == key || s.set.HelperProblem(s.set.Variables[k]) == "") {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: problem}
		}
	}
	changedPrograms := map[string]*model.Program{}
	if old.Type == model.VarList && checked.Type == model.VarList {
		for from, to := range renames {
			if old.EntryIndex(from) < 0 || checked.EntryIndex(to) < 0 || checked.EntryIndex(from) >= 0 {
				s.mu.Unlock()
				return nil, refuse("invalid", "the entry %q cannot be renamed to %q: the old one must be gone and the new one there", from, to)
			}
		}
		for _, e := range old.Entries {
			if checked.EntryIndex(e) >= 0 || renames[e] != "" {
				continue
			}
			if users := s.set.EntryUsers(key, e); len(users) > 0 {
				s.mu.Unlock()
				return nil, &Error{Code: "in-use", Msg: fmt.Sprintf("the entry %q of %s cannot be removed while programs use it", e, old.Name), Users: users}
			}
		}
		// apply the renames one after the other on a working copy of the set
		work := &model.Set{Programs: map[string]*model.Program{}, Variables: s.set.Variables}
		for k, p := range s.set.Programs {
			work.Programs[k] = p
		}
		for _, from := range sortedKeys(renames) {
			for k, p := range work.RenameEntry(key, from, renames[from]) {
				work.Programs[k] = p
				changedPrograms[k] = p
			}
		}
	}
	paths := []string{}
	for _, k := range sortedPrograms(changedPrograms) {
		path, _, err := s.tree.SaveProgram(changedPrograms[k], s.o.Names)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		paths = append(paths, path)
		s.set.Programs[k] = changedPrograms[k]
	}
	path, _, err := s.tree.SaveVariable(checked)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	paths = append(paths, path)
	s.set.Variables[key] = checked
	for from, to := range renames {
		s.eng.RenameEntry(key, from, to)
	}
	s.eng.SetVariables(s.set.Variables)
	s.programsChanged()
	s.mu.Unlock()
	s.queue.Enqueue(paths, author, "Variable "+checked.Name+" "+what)
	s.changed()
	return checked.Clone(), nil
}

// DeleteVariable removes a variable; one that programs use cannot be removed.
func (s *Service) DeleteVariable(key string, author repo.Author) error {
	s.mu.Lock()
	old := s.set.Variables[key]
	if old == nil {
		s.mu.Unlock()
		return refuse("not-found", "there is no variable %s", key)
	}
	if users := s.set.VariableUsers(key); len(users) > 0 {
		s.mu.Unlock()
		return &Error{Code: "in-use", Msg: fmt.Sprintf("%s cannot be removed while programs use it", old.Name), Users: users}
	}
	if users := s.set.ComputedUsers(key); len(users) > 0 {
		s.mu.Unlock()
		return &Error{Code: "in-use", Msg: fmt.Sprintf("%s cannot be removed while computed values use it", old.Name), Users: users}
	}
	if users := s.set.HelperMembers(key); len(users) > 0 {
		s.mu.Unlock()
		return &Error{Code: "in-use", Msg: fmt.Sprintf("%s cannot be removed while its device helper has other variables", old.Name), Users: users}
	}
	path, err := s.tree.DeleteVariable(key)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	delete(s.set.Variables, key)
	s.eng.SetVariables(s.set.Variables)
	s.mu.Unlock()
	s.queue.Enqueue([]string{path}, author, "Variable "+old.Name+" removed")
	s.changed()
	return nil
}

// programsChanged loads the engine's programs from the set (with the lock held).
func (s *Service) programsChanged() { s.eng.SetPrograms(s.set.Programs) }

// plainError is a reader's error without the file and the line: the page shows a form, not a file.
func plainError(err error) string {
	if e, ok := err.(*model.Error); ok {
		return e.Msg
	}
	return err.Error()
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedPrograms(m map[string]*model.Program) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedVariables(m map[string]*model.Variable) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
