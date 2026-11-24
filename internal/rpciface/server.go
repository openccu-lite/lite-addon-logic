package rpciface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
)

// Options configure the interface.
type Options struct {
	Backend Backend
	// Version is the addon's, for getVersion.
	Version string
	// Names writes the devices' names into the system's metadata (nil: nobody does).
	Names Namer
	// Client calls the subscribers back (a client with a 5 s timeout when nil).
	Client *http.Client
	// Log receives a line per noteworthy event (nil: silent).
	Log func(format string, args ...any)
	// Queue bounds what waits for the subscribers (1024 when 0); beyond it events are dropped
	// and counted, never the caller blocked.
	Queue int
	// SubscribersFile is where the subscribers are kept across a restart ("": not kept). The
	// interface processes of the system do the same: a client that called init is called back
	// again after the process restarted, without having to notice the restart first.
	SubscribersFile string
	// Retry is how long after a failed write of the names the next one is tried (1 min when 0).
	Retry time.Duration
}

// Namer writes names: set by reference ("Logic.var-regen:1"), and the references to remove.
type Namer interface {
	Push(ctx context.Context, set map[string]string, remove []string) error
}

// Lister is a Namer that can also say which names of this interface the metadata holds, by
// reference. The interface asks once after its start: an object that went away while the addon
// did not run - a tree restored or emptied, a file deleted by hand, a pull with deletions - left
// its names there, and only what the metadata is known to hold can be removed.
type Lister interface {
	Known(ctx context.Context) (map[string]string, error)
}

// subscriber is a client that called init.
type subscriber struct {
	url   string
	id    string
	fails int
}

// maxFails is how many callbacks in a row may fail before a subscriber is dropped; it calls init
// again when it is back.
const maxFails = 10

// Server is the interface process.
type Server struct {
	o    Options
	jobs chan func()

	mu      sync.Mutex
	devices map[string]*device
	subs    map[string]*subscriber
	dropped int

	// names: what the metadata has from us, and whether a write is owed
	named      map[string]string
	namesRead  bool // the metadata's own list was read (or cannot be): named is complete
	namesBlind bool // reading it failed and was said so
	namesOwed  bool
	namesTimer *time.Timer
}

// New makes the interface; Run serves its queue, Handler its calls.
func New(o Options) *Server {
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 5 * time.Second}
	}
	if o.Queue <= 0 {
		o.Queue = 1024
	}
	if o.Retry <= 0 {
		o.Retry = time.Minute
	}
	s := &Server{o: o, jobs: make(chan func(), o.Queue), subs: map[string]*subscriber{}}
	s.devices = build(o.Backend)
	s.restoreSubscribers()
	return s
}

type storedSubscriber struct {
	URL string `json:"url"`
	ID  string `json:"id"`
}

// restoreSubscribers reads the subscribers a run before this one had and greets each as after
// its init: it is told the devices it lacks, and the events go to it again.
func (s *Server) restoreSubscribers() {
	if s.o.SubscribersFile == "" {
		return
	}
	data, err := os.ReadFile(s.o.SubscribersFile)
	if err != nil {
		return
	}
	var stored []storedSubscriber
	if json.Unmarshal(data, &stored) != nil {
		s.logf("interface: the list of subscribers is not readable; they register again by themselves")
		return
	}
	for _, st := range stored {
		u, local := loopback(st.URL)
		if !local || st.ID == "" {
			continue
		}
		sub := &subscriber{url: u, id: st.ID}
		s.subs[u] = sub
		c := *sub
		s.enqueue(func() { s.greet(&c) })
		s.logf("interface: %s is a subscriber from before the restart (%s)", st.ID, u)
	}
}

// saveSubscribers writes the subscribers (with the lock held).
func (s *Server) saveSubscribers() {
	if s.o.SubscribersFile == "" {
		return
	}
	stored := make([]storedSubscriber, 0, len(s.subs))
	for _, sub := range s.subs {
		stored = append(stored, storedSubscriber{URL: sub.url, ID: sub.id})
	}
	sort.Slice(stored, func(i, j int) bool { return stored[i].URL < stored[j].URL })
	data, _ := json.Marshal(stored)
	tmp := s.o.SubscribersFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		s.logf("interface: the list of subscribers could not be written: %v", err)
		return
	}
	if err := os.Rename(tmp, s.o.SubscribersFile); err != nil {
		s.logf("interface: the list of subscribers could not be written: %v", err)
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.o.Log != nil {
		s.o.Log(format, args...)
	}
}

// enqueue hands work to the queue without ever blocking the caller.
func (s *Server) enqueue(f func()) {
	select {
	case s.jobs <- f:
	default:
		s.mu.Lock()
		s.dropped++
		n := s.dropped
		s.mu.Unlock()
		if n == 1 || n%100 == 0 {
			s.logf("interface: the queue is full, %d events dropped so far", n)
		}
	}
}

// Dropped is how many events did not fit the queue.
func (s *Server) Dropped() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

// Run delivers what is queued, in order, until the context ends. It writes the names once at
// the start.
func (s *Server) Run(ctx context.Context) {
	s.enqueue(s.names)
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			if s.namesTimer != nil {
				s.namesTimer.Stop()
			}
			s.mu.Unlock()
			return
		case f := <-s.jobs:
			f()
		}
	}
}

// Notify takes what the engine tells (service.Subscribe): it only queues.
func (s *Server) Notify(n engine.Notification) {
	s.enqueue(func() { s.event(n) })
}

// Changed is told when programs or variables came, went or were renamed (service.OnChange): the
// devices are compared with what was announced and the difference goes to the subscribers.
func (s *Server) Changed() {
	s.enqueue(s.sync)
}

// Subscribers lists the callback URLs registered now, sorted.
func (s *Server) Subscribers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.subs))
	for u := range s.subs {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

func (s *Server) snapshot() []*subscriber {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*subscriber, 0, len(s.subs))
	for _, sub := range s.subs {
		c := *sub
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].url < out[j].url })
	return out
}

// call calls a subscriber back and keeps count of its failures.
func (s *Server) call(sub *subscriber, method string, args ...any) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := Call(ctx, s.o.Client, sub.url, method, args...)
	var f *Fault
	answered := err == nil || errors.As(err, &f)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.subs[sub.url]
	if cur == nil || cur.id != sub.id {
		return res, err
	}
	if answered {
		cur.fails = 0
		return res, err
	}
	cur.fails++
	if cur.fails >= maxFails {
		delete(s.subs, sub.url)
		s.saveSubscribers()
		s.logf("interface: %s (%s) did not answer %d times, dropped", sub.id, sub.url, cur.fails)
	}
	return res, err
}

// multicall sends events to every subscriber: [address, datapoint, value] each.
func (s *Server) multicall(events [][3]any) {
	if len(events) == 0 {
		return
	}
	for _, sub := range s.snapshot() {
		calls := make([]any, 0, len(events))
		for _, e := range events {
			calls = append(calls, map[string]any{"methodName": "event", "params": []any{sub.id, e[0], e[1], e[2]}})
		}
		_, _ = s.call(sub, "system.multicall", calls)
	}
}

// event turns a notification into datapoint events.
func (s *Server) event(n engine.Notification) {
	var events [][3]any
	switch n.Kind {
	case "variable":
		addr := prefixVar + n.Key
		s.mu.Lock()
		d := s.devices[addr]
		s.mu.Unlock()
		if d == nil || d.variable == nil {
			return
		}
		events = append(events, [3]any{addr + ":1", "VALUE", wire(d.variable, n.Value)})
		if d.param("COUNT") != nil {
			for _, v := range s.o.Backend.Variables() {
				if v.Def != nil && v.Def.Key == n.Key && v.Alarm != nil {
					events = append(events, [3]any{addr + ":1", "COUNT", v.Alarm.Count}, [3]any{addr + ":1", "FIRST", unix(v.Alarm.First)}, [3]any{addr + ":1", "LAST", unix(v.Time)})
				}
			}
		}
	case "program":
		addr := prefixPrg + n.Key
		s.mu.Lock()
		d := s.devices[addr]
		s.mu.Unlock()
		if d == nil {
			return
		}
		switch {
		case n.Paused:
			return // the loop guard's pause is the page's to show; no datapoint carries it
		case n.Active != nil:
			events = append(events, [3]any{addr + ":1", "ACTIVE", *n.Active})
		default:
			events = append(events, [3]any{addr + ":1", "LAST_RUN", n.Block})
		}
	}
	s.multicall(events)
}

// sync rebuilds the devices and announces what came and went. A device whose descriptions
// changed (another type, other limits or entries) goes and comes.
func (s *Server) sync() {
	next := build(s.o.Backend)
	s.mu.Lock()
	prev := s.devices
	s.devices = next
	s.mu.Unlock()
	var gone []any
	var come []any
	for _, d := range sorted(prev) {
		if n := next[d.address]; n == nil || n.firmware != d.firmware || n.channelType() != d.channelType() {
			gone = append(gone, d.address, d.address+":0", d.address+":1")
		}
	}
	for _, d := range sorted(next) {
		if p := prev[d.address]; p == nil || p.firmware != d.firmware || p.channelType() != d.channelType() {
			come = append(come, d.descriptions()...)
		}
	}
	for _, sub := range s.snapshot() {
		if len(gone) > 0 {
			_, _ = s.call(sub, "deleteDevices", sub.id, gone)
		}
		if len(come) > 0 {
			_, _ = s.call(sub, "newDevices", sub.id, come)
		}
	}
	s.names()
}

// greet answers an init as rfd does: ask the client what it knows, remove what is gone, announce
// what it lacks. A client without listDevices gets everything.
func (s *Server) greet(sub *subscriber) {
	s.mu.Lock()
	devices := sorted(s.devices)
	s.mu.Unlock()
	// address -> the FIRMWARE the client keeps for it ("" for a channel, or a client that keeps none)
	known := map[string]string{}
	if res, err := s.call(sub, "listDevices", sub.id); err == nil {
		if list, ok := res.([]any); ok {
			for _, e := range list {
				if m, ok := e.(map[string]any); ok {
					if a, ok := m["ADDRESS"].(string); ok {
						fw, _ := m["FIRMWARE"].(string)
						known[a] = fw
					}
				}
			}
		}
	}
	ours := map[string]bool{}
	var come []any
	var changed []any
	for _, d := range devices {
		lacks := false
		for _, a := range append([]string{d.address}, d.channels()...) {
			ours[a] = true
			if _, ok := known[a]; !ok {
				lacks = true
			}
		}
		// a device the client keeps with another FIRMWARE has another shape since the client last
		// heard of it (an update of the addon changed its description): it goes and comes, as sync
		// reports a change while the interface runs
		if fw := known[d.address]; !lacks && fw != "" && fw != d.firmware {
			changed = append(changed, d.address, d.address+":0", d.address+":1")
			lacks = true
		}
		if lacks {
			come = append(come, d.descriptions()...)
		}
	}
	gone := changed
	for a := range known {
		if !ours[a] {
			gone = append(gone, a)
		}
	}
	sort.Slice(gone, func(i, j int) bool { return gone[i].(string) < gone[j].(string) })
	if len(gone) > 0 {
		_, _ = s.call(sub, "deleteDevices", sub.id, gone)
	}
	if len(come) > 0 {
		_, _ = s.call(sub, "newDevices", sub.id, come)
	}
}

// readNames asks the metadata once which names of the interface it holds: from then on "what
// was written" is what is there, so the names of objects that are gone are removed and only
// names that differ are written. While the list cannot be read the names are written as before
// (all of them, nothing removed) and the reading is tried again.
func (s *Server) readNames(ctx context.Context) {
	s.mu.Lock()
	read := s.namesRead
	s.mu.Unlock()
	if read {
		return
	}
	l, ok := s.o.Names.(Lister)
	if !ok {
		s.mu.Lock()
		s.namesRead = true
		s.mu.Unlock()
		return
	}
	known, err := l.Known(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		if !s.namesBlind {
			s.logf("interface: the names the system holds could not be read, trying again: %v", err)
		}
		s.namesBlind = true
		s.retryNames()
		return
	}
	s.namesRead = true
	s.named = known
}

// retryNames has names run again after the retry time; s.mu is held.
func (s *Server) retryNames() {
	if s.namesTimer != nil {
		s.namesTimer.Stop()
	}
	s.namesTimer = time.AfterFunc(s.o.Retry, func() { s.enqueue(s.names) })
}

// names writes the devices' names into the metadata when they differ from what is there, and
// removes the names of devices that are gone.
func (s *Server) names() {
	if s.o.Names == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s.readNames(ctx)
	s.mu.Lock()
	want := map[string]string{}
	for _, d := range s.devices {
		want[Interface+"."+d.address] = d.name
		want[Interface+"."+d.address+":0"] = d.name + ":0"
		want[Interface+"."+d.address+":1"] = d.name
	}
	set := map[string]string{}
	for ref, name := range want {
		if s.named[ref] != name {
			set[ref] = name
		}
	}
	var remove []string
	for ref := range s.named {
		if _, ok := want[ref]; !ok {
			remove = append(remove, ref)
		}
	}
	sort.Strings(remove)
	s.mu.Unlock()
	if len(set) == 0 && len(remove) == 0 {
		return
	}
	err := s.o.Names.Push(ctx, set, remove)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		// the system may not be up yet, or the addon's token not minted: try again later
		if !s.namesOwed {
			s.logf("interface: the names could not be written, trying again: %v", err)
		}
		s.namesOwed = true
		s.retryNames()
		return
	}
	if s.namesOwed {
		s.logf("interface: the names are written")
	}
	s.namesOwed = false
	s.named = want
}

// loopback says whether a callback URL points at this machine: the interface answers nobody else.
func loopback(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(raw, "xmlrpc://"); ok {
		raw = "http://" + rest
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Host == "" {
		return "", false
	}
	host := u.Hostname()
	if host == "localhost" {
		return u.String(), true
	}
	ip := net.ParseIP(host)
	return u.String(), ip != nil && ip.IsLoopback()
}

// Handler serves the XML-RPC calls (POST, any path).
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "XML-RPC: POST", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
		w.Header().Set("Content-Type", "text/xml")
		if err != nil || len(body) > MaxBody {
			_, _ = w.Write(EncodeResponse(nil, fault(FaultInvalidRequest, "the request cannot be read or is too large")))
			return
		}
		method, args, err := DecodeCall(body)
		if err != nil {
			_, _ = w.Write(EncodeResponse(nil, fault(FaultInvalidRequest, "not an XML-RPC call: %v", err)))
			return
		}
		res, f := s.dispatch(method, args, 0)
		_, _ = w.Write(EncodeResponse(res, f))
	})
}

// Methods are the calls the interface answers.
var Methods = []string{"getDeviceDescription", "getParamset", "getParamsetDescription", "getServiceMessages", "getValue", "getVersion", "init",
	"listDevices", "ping", "putParamset", "setValue", "system.listMethods", "system.multicall"}

func text(args []any, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	s, ok := args[i].(string)
	return s, ok
}

func (s *Server) dispatch(method string, args []any, depth int) (any, *Fault) {
	switch method {
	case "system.listMethods":
		return Methods, nil
	case "system.multicall":
		list, ok := []any(nil), false
		if len(args) == 1 {
			list, ok = args[0].([]any)
		}
		if !ok || depth > 0 {
			return nil, fault(FaultInvalidRequest, "system.multicall takes one list of calls")
		}
		out := make([]any, 0, len(list))
		for _, c := range list {
			m, _ := c.(map[string]any)
			name, _ := m["methodName"].(string)
			ps, _ := m["params"].([]any)
			res, f := s.dispatch(name, ps, depth+1)
			if f != nil {
				out = append(out, map[string]any{"faultCode": f.Code, "faultString": f.Msg})
			} else {
				out = append(out, []any{res})
			}
		}
		return out, nil
	case "getVersion":
		return s.o.Version, nil
	case "getServiceMessages":
		return []any{}, nil
	case "init":
		return s.init(args)
	case "ping":
		id, _ := text(args, 0)
		s.enqueue(func() { s.multicall([][3]any{{"CENTRAL", "PONG", id}}) })
		return true, nil
	case "listDevices":
		s.mu.Lock()
		defer s.mu.Unlock()
		out := []any{}
		for _, d := range sorted(s.devices) {
			out = append(out, d.descriptions()...)
		}
		return out, nil
	case "getDeviceDescription":
		addr, ok := text(args, 0)
		if !ok {
			return nil, fault(FaultInvalidRequest, "getDeviceDescription takes an address")
		}
		d, ch, f := s.find(addr)
		if f != nil {
			return nil, f
		}
		return d.descriptions()[ch+1], nil
	case "getParamsetDescription", "getParamset":
		addr, ok1 := text(args, 0)
		set, ok2 := text(args, 1)
		if !ok1 || !ok2 {
			return nil, fault(FaultInvalidRequest, "%s takes an address and a paramset", method)
		}
		d, ch, f := s.find(addr)
		if f != nil {
			return nil, f
		}
		out := map[string]any{}
		switch {
		case set == "MASTER":
			return out, nil
		case set != "VALUES" || ch < 0:
			return nil, fault(FaultUnknownSet, "%s has no paramset %s", addr, set)
		case ch == 0:
			return out, nil
		}
		for _, p := range d.params {
			if method == "getParamsetDescription" {
				out[p.name] = p.description()
			} else if p.ops&opRead != 0 {
				v, f := s.read(d, p.name)
				if f != nil {
					return nil, f
				}
				out[p.name] = v
			}
		}
		return out, nil
	case "getValue":
		addr, ok1 := text(args, 0)
		name, ok2 := text(args, 1)
		if !ok1 || !ok2 {
			return nil, fault(FaultInvalidRequest, "getValue takes an address and a datapoint")
		}
		d, f := s.valueChannel(addr)
		if f != nil {
			return nil, f
		}
		return s.read(d, name)
	case "setValue":
		addr, ok1 := text(args, 0)
		name, ok2 := text(args, 1)
		if !ok1 || !ok2 || len(args) < 3 {
			return nil, fault(FaultInvalidRequest, "setValue takes an address, a datapoint and a value")
		}
		d, f := s.valueChannel(addr)
		if f != nil {
			return nil, f
		}
		if f := s.write(d, name, args[2]); f != nil {
			return nil, f
		}
		return "", nil
	case "putParamset":
		addr, ok1 := text(args, 0)
		set, ok2 := text(args, 1)
		var values map[string]any
		if len(args) >= 3 {
			values, _ = args[2].(map[string]any)
		}
		if !ok1 || !ok2 || values == nil {
			return nil, fault(FaultInvalidRequest, "putParamset takes an address, a paramset and the values")
		}
		if set != "VALUES" {
			return nil, fault(FaultUnknownSet, "%s has no writable paramset %s", addr, set)
		}
		d, f := s.valueChannel(addr)
		if f != nil {
			return nil, f
		}
		names := make([]string, 0, len(values))
		for k := range values {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			if f := s.write(d, k, values[k]); f != nil {
				return nil, f
			}
		}
		return "", nil
	}
	return nil, fault(FaultUnknownMethod, "%s: unknown method", method)
}

// init registers a subscriber (a URL and an id), or with an empty id removes it.
func (s *Server) init(args []any) (any, *Fault) {
	raw, ok := text(args, 0)
	if !ok {
		return nil, fault(FaultInvalidRequest, "init takes a URL and an interface id")
	}
	id, _ := text(args, 1)
	u, local := loopback(raw)
	if id == "" {
		s.mu.Lock()
		delete(s.subs, u)
		delete(s.subs, strings.TrimSpace(raw))
		s.saveSubscribers()
		s.mu.Unlock()
		return "", nil
	}
	if !local {
		return nil, fault(FaultGeneral, "init: %q is not an http URL on this system (the interface calls back on the loopback only)", raw)
	}
	sub := &subscriber{url: u, id: id}
	s.mu.Lock()
	s.subs[u] = sub
	s.saveSubscribers()
	s.mu.Unlock()
	s.logf("interface: %s registered (%s)", id, u)
	c := *sub
	s.enqueue(func() { s.greet(&c) })
	return "", nil
}

// find resolves an address: the device, and the channel (-1 for the device itself).
func (s *Server) find(addr string) (*device, int, *Fault) {
	base, ch := addr, -1
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		base = addr[:i]
		switch addr[i+1:] {
		case "0":
			ch = 0
		case "1":
			ch = 1
		default:
			return nil, 0, fault(FaultUnknownDevice, "unknown channel %s", addr)
		}
	}
	s.mu.Lock()
	d := s.devices[base]
	s.mu.Unlock()
	if d == nil {
		return nil, 0, fault(FaultUnknownDevice, "unknown device %s", addr)
	}
	return d, ch, nil
}

func (s *Server) valueChannel(addr string) (*device, *Fault) {
	d, ch, f := s.find(addr)
	if f != nil {
		return nil, f
	}
	if ch != 1 {
		return nil, fault(FaultUnknownParam, "%s has no datapoints (they are on channel 1)", addr)
	}
	return d, nil
}

// read answers a datapoint's current value.
func (s *Server) read(d *device, name string) (any, *Fault) {
	p := d.param(name)
	if p == nil {
		return nil, fault(FaultUnknownParam, "%s has no datapoint %s", d.address, name)
	}
	if p.ops&opRead == 0 {
		return nil, fault(FaultNotSupported, "%s of %s cannot be read", name, d.address)
	}
	if d.variable == nil {
		for _, prg := range s.o.Backend.Programs() {
			if prg.Key == d.key {
				if name == "ACTIVE" {
					return prg.Active, nil
				}
				return prg.LastRun, nil
			}
		}
		return nil, fault(FaultUnknownDevice, "%s is gone", d.address)
	}
	for _, v := range s.o.Backend.Variables() {
		if v.Def == nil || v.Def.Key != d.key {
			continue
		}
		switch name {
		case "VALUE":
			return wire(d.variable, v.Value), nil
		case "LAST":
			return unix(v.Time), nil
		}
		a := engine.AlarmState{}
		if v.Alarm != nil {
			a = *v.Alarm
		}
		if name == "COUNT" {
			return a.Count, nil
		}
		return unix(a.First), nil
	}
	return nil, fault(FaultUnknownDevice, "%s is gone", d.address)
}

// write does a write from outside. It enters the same door as a write on the page: the engine
// checks it and tells every listener, this interface's subscribers among them.
func (s *Server) write(d *device, name string, value any) *Fault {
	p := d.param(name)
	if p == nil {
		return fault(FaultUnknownParam, "%s has no datapoint %s", d.address, name)
	}
	if d.variable == nil && p.typ == "ACTION" && !d.operable {
		return fault(FaultNotSupported, "%s is not operable: it cannot be run from outside", d.address)
	}
	if p.ops&opWrite == 0 {
		return fault(FaultNotSupported, "%s of %s cannot be written", name, d.address)
	}
	var err error
	switch {
	case d.variable != nil && name == "ACKNOWLEDGE":
		err = s.o.Backend.Acknowledge(d.key)
	case d.variable != nil:
		v, cerr := unwire(d.variable, value)
		if cerr != nil {
			return fault(FaultUnknownParam, "%s of %s: %v", name, d.address, cerr)
		}
		err = s.o.Backend.SetVariable(d.key, v)
	case name == "ACTIVE":
		b, ok := truth(value)
		if !ok {
			return fault(FaultUnknownParam, "ACTIVE of %s takes true or false", d.address)
		}
		err = s.o.Backend.SetActive(d.key, b)
	default:
		err = s.o.Backend.Run(d.key, name == "RUN_THEN")
	}
	if err == nil {
		return nil
	}
	var we *engine.WriteError
	if errors.As(err, &we) {
		switch we.Code {
		case "unknown-variable", "unknown-program":
			return fault(FaultUnknownDevice, "%s", we.Msg)
		case "read-only":
			return fault(FaultNotSupported, "%s", we.Msg)
		}
		return fault(FaultUnknownParam, "%s", we.Msg)
	}
	return fault(FaultGeneral, "%s", fmt.Sprint(err))
}
