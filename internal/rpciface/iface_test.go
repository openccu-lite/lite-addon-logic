package rpciface

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// fake is a backend: variables with values, programs, and what was done to them.
type fake struct {
	mu     sync.Mutex
	vars   []engine.VariableInfo
	prgs   []Program
	notify func(engine.Notification)
	calls  []string
	fail   error
}

func (f *fake) Variables() []engine.VariableInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]engine.VariableInfo(nil), f.vars...)
}

func (f *fake) Programs() []Program {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Program(nil), f.prgs...)
}

func (f *fake) did(format string, args ...any) error {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	return f.fail
}

func (f *fake) SetVariable(key string, value any) error {
	f.mu.Lock()
	err := f.did("set %s=%#v", key, value)
	var n *engine.Notification
	if err == nil {
		for i := range f.vars {
			if f.vars[i].Def != nil && f.vars[i].Def.Key == key {
				changed := f.vars[i].Value != value
				f.vars[i].Value = value
				f.vars[i].Time = time.Unix(1800000000, 0)
				if a := f.vars[i].Alarm; a != nil && value == true {
					a.Count++
					a.First = time.Unix(1700000000, 0)
				}
				n = &engine.Notification{Kind: "variable", Key: key, Value: value, Changed: changed}
			}
		}
	}
	notify := f.notify
	f.mu.Unlock()
	if n != nil && notify != nil {
		notify(*n)
	}
	return err
}

func (f *fake) Acknowledge(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.did("acknowledge %s", key)
}

func (f *fake) SetActive(key string, active bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.did("active %s=%v", key, active)
}

func (f *fake) Run(key string, then bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.did("run %s then=%v", key, then)
}

func (f *fake) done() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.calls
	f.calls = nil
	return out
}

func ptr(f float64) *float64 { return &f }

func newFake() *fake {
	return &fake{
		vars: []engine.VariableInfo{
			{Def: &model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool}, Value: false},
			{Def: &model.Variable{Key: "modus", Name: "Modus", Type: model.VarList, Entries: []string{"Auto", "Manuell", "Urlaub"}}, Value: "Manuell"},
			{Def: &model.Variable{Key: "temperatur", Name: "Temperatur", Type: model.VarNumber, Unit: "°C", Min: ptr(-40), Max: ptr(60)}, Value: 21.5, Time: time.Unix(1790000000, 0)},
			{Def: &model.Variable{Key: "hinweis", Name: "Hinweis", Type: model.VarString}, Value: "hallo <welt> & co"},
			{Def: &model.Variable{Key: "alarm-wasser", Name: "Alarm Wasser", Type: model.VarAlarm}, Value: true, Time: time.Unix(1790000100, 0),
				Alarm: &engine.AlarmState{State: engine.AlarmOncoming, Count: 2, First: time.Unix(1790000000, 0)}},
			{Def: &model.Variable{Key: model.SystemAlarmMessages, Name: "Alarmmeldungen", Type: model.VarNumber}, Value: 1.0},
			{Def: &model.Variable{Key: "Not A Key", Name: "x", Type: model.VarBool}},
			{},
		},
		prgs: []Program{
			{Key: "markise", Name: "Markise bei Regen einfahren", Active: true, Operable: true, LastRun: 1},
			{Key: "intern", Name: "Intern", Active: false, Operable: false},
			{Key: "Not A Key"},
		},
	}
}

// client is a subscriber: an XML-RPC server that records what it is called with.
type client struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	calls  []string
	events []string
	knows  []any // what listDevices answers; nil: the method is unknown
	status int
	got    chan struct{}
}

func newClient(t *testing.T) *client {
	c := &client{t: t, got: make(chan struct{}, 1000)}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		method, args, err := DecodeCall(body)
		if err != nil {
			t.Errorf("a callback that is no call: %v", err)
		}
		c.mu.Lock()
		status := c.status
		var res any = ""
		var f *Fault
		switch method {
		case "listDevices":
			if c.knows == nil {
				f = fault(FaultUnknownMethod, "listDevices: unknown method")
			} else {
				res = c.knows
			}
			c.calls = append(c.calls, fmt.Sprintf("listDevices %v", args))
		case "newDevices", "deleteDevices":
			var addrs []string
			for _, e := range args[1].([]any) {
				if m, ok := e.(map[string]any); ok {
					addrs = append(addrs, m["ADDRESS"].(string))
				} else {
					addrs = append(addrs, e.(string))
				}
			}
			c.calls = append(c.calls, fmt.Sprintf("%s %v %s", method, args[0], strings.Join(addrs, ",")))
		case "system.multicall":
			for _, e := range args[0].([]any) {
				m := e.(map[string]any)
				p := m["params"].([]any)
				c.events = append(c.events, fmt.Sprintf("%v %v %v=%#v", p[0], p[1], p[2], p[3]))
			}
			res = []any{}
		default:
			c.calls = append(c.calls, method)
		}
		c.mu.Unlock()
		if status != 0 {
			http.Error(w, "no", status)
		} else {
			_, _ = w.Write(EncodeResponse(res, f))
		}
		c.got <- struct{}{}
	}))
	t.Cleanup(c.srv.Close)
	return c
}

// wait waits for n callbacks.
func (c *client) wait(n int) {
	c.t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-c.got:
		case <-time.After(5 * time.Second):
			c.t.Fatalf("callback %d of %d did not come", i+1, n)
		}
	}
}

func (c *client) take() (calls, events []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	calls, events = c.calls, c.events
	c.calls, c.events = nil, nil
	return
}

// until waits until everything in want was called back (calls and events together), and answers
// what came.
func (c *client) until(want ...string) []string {
	c.t.Helper()
	var all []string
	deadline := time.After(5 * time.Second)
	for {
		calls, events := c.take()
		all = append(append(all, calls...), events...)
		missing := false
		for _, w := range want {
			found := false
			for _, a := range all {
				found = found || a == w
			}
			missing = missing || !found
		}
		if !missing {
			return all
		}
		select {
		case <-c.got:
		case <-deadline:
			c.t.Fatalf("waited for %v, got %v", want, all)
		}
	}
}

// drain forgets every callback so far.
func (c *client) drain() {
	for {
		select {
		case <-c.got:
		case <-time.After(80 * time.Millisecond):
			c.take()
			return
		}
	}
}

// quiet says no callback comes any more.
func (c *client) quiet() {
	c.t.Helper()
	select {
	case <-c.got:
		calls, events := c.take()
		c.t.Fatalf("an unexpected callback: %v %v", calls, events)
	case <-time.After(60 * time.Millisecond):
	}
}

type harness struct {
	t   *testing.T
	b   *fake
	s   *Server
	url string
	log []string
	mu  sync.Mutex
}

func start(t *testing.T, o Options) *harness {
	h := &harness{t: t}
	if o.Backend == nil {
		h.b = newFake()
		o.Backend = h.b
	} else {
		h.b, _ = o.Backend.(*fake)
	}
	o.Log = func(format string, args ...any) {
		h.mu.Lock()
		h.log = append(h.log, fmt.Sprintf(format, args...))
		h.mu.Unlock()
	}
	if o.Version == "" {
		o.Version = "1.2.3"
	}
	h.s = New(o)
	if h.b != nil {
		h.b.notify = h.s.Notify
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); h.s.Run(ctx) }()
	srv := httptest.NewServer(h.s.Handler())
	h.url = srv.URL
	t.Cleanup(func() { srv.Close(); cancel(); <-done })
	return h
}

func (h *harness) logged() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.log, "\n")
}

func (h *harness) call(method string, args ...any) (any, error) {
	h.t.Helper()
	return Call(context.Background(), http.DefaultClient, h.url, method, args...)
}

func (h *harness) ok(method string, args ...any) any {
	h.t.Helper()
	res, err := h.call(method, args...)
	if err != nil {
		h.t.Fatalf("%s %v: %v", method, args, err)
	}
	return res
}

func (h *harness) fault(code int, method string, args ...any) {
	h.t.Helper()
	_, err := h.call(method, args...)
	var f *Fault
	if !errors.As(err, &f) || f.Code != code {
		h.t.Errorf("%s %v: %v, want fault %d", method, args, err, code)
	}
}

func TestDevices(t *testing.T) {
	h := start(t, Options{})
	list := h.ok("listDevices").([]any)
	var addrs []string
	by := map[string]map[string]any{}
	for _, e := range list {
		m := e.(map[string]any)
		addrs = append(addrs, m["ADDRESS"].(string))
		by[m["ADDRESS"].(string)] = m
	}
	want := []string{"prg-intern", "prg-intern:0", "prg-intern:1", "prg-markise", "prg-markise:0", "prg-markise:1",
		"var-alarm-messages", "var-alarm-messages:0", "var-alarm-messages:1", "var-alarm-wasser", "var-alarm-wasser:0", "var-alarm-wasser:1",
		"var-hinweis", "var-hinweis:0", "var-hinweis:1", "var-modus", "var-modus:0", "var-modus:1", "var-regen", "var-regen:0", "var-regen:1",
		"var-temperatur", "var-temperatur:0", "var-temperatur:1"}
	if !reflect.DeepEqual(addrs, want) {
		t.Fatalf("devices: %v", addrs)
	}
	dev := by["var-regen"]
	if dev["TYPE"] != TypeVariable || dev["INTERFACE"] != "Logic" || dev["PARENT"] != "" || !reflect.DeepEqual(dev["CHILDREN"], []any{"var-regen:0", "var-regen:1"}) ||
		!reflect.DeepEqual(dev["PARAMSETS"], []any{"MASTER"}) || !strings.HasPrefix(dev["FIRMWARE"].(string), "1.") {
		t.Errorf("device: %v", dev)
	}
	if ch := by["var-regen:0"]; ch["TYPE"] != "MAINTENANCE" || ch["PARENT"] != "var-regen" || ch["PARENT_TYPE"] != TypeVariable || ch["INDEX"] != 0 {
		t.Errorf("channel 0: %v", ch)
	}
	if ch := by["var-regen:1"]; ch["TYPE"] != "VARIABLE" || !reflect.DeepEqual(ch["PARAMSETS"], []any{"MASTER", "VALUES"}) || ch["INDEX"] != 1 {
		t.Errorf("channel 1: %v", ch)
	}
	if by["var-alarm-wasser:1"]["TYPE"] != "ALARM" || by["prg-markise:1"]["TYPE"] != "PROGRAM" || by["prg-markise"]["TYPE"] != TypeProgram {
		t.Error("channel types")
	}
	// one description per shape: a list and a number have their own, and a program that is not operable
	if by["var-regen"]["FIRMWARE"] == by["var-modus"]["FIRMWARE"] || by["var-regen"]["FIRMWARE"] == by["var-temperatur"]["FIRMWARE"] || by["prg-markise"]["FIRMWARE"] == by["prg-intern"]["FIRMWARE"] {
		t.Error("firmware fingerprints")
	}
	if got := h.ok("getDeviceDescription", "var-regen:1"); !reflect.DeepEqual(got, by["var-regen:1"]) {
		t.Errorf("getDeviceDescription: %v", got)
	}
	if got := h.ok("getDeviceDescription", "prg-markise"); !reflect.DeepEqual(got, by["prg-markise"]) {
		t.Errorf("getDeviceDescription: %v", got)
	}
	h.fault(FaultUnknownDevice, "getDeviceDescription", "var-nichts")
	h.fault(FaultUnknownDevice, "getDeviceDescription", "var-regen:2")
	h.fault(FaultInvalidRequest, "getDeviceDescription", 5)
}

// What Control's panels read (task 53): a boolean's texts, the flags, and whether a program runs.
func TestTextsFlagsAndOperable(t *testing.T) {
	b := &fake{
		vars: []engine.VariableInfo{
			{Def: &model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool, Visible: true}, Value: false},
			{Def: &model.Variable{Key: "anwesend", Name: "Anwesenheit", Type: model.VarBool, Visible: true, Labels: &model.Labels{False: "abwesend", True: "anwesend"}}, Value: true},
			{Def: &model.Variable{Key: "halb", Name: "Halb", Type: model.VarAlarm, Visible: true, Internal: true, Labels: &model.Labels{True: "ja"}}, Value: false},
			{Def: &model.Variable{Key: "leer", Name: "Leer", Type: model.VarBool, Visible: true, Labels: &model.Labels{}}, Value: false},
			{Def: &model.Variable{Key: "versteckt", Name: "Versteckt", Type: model.VarBool}, Value: false},
			{Def: &model.Variable{Key: "intern", Name: "Intern", Type: model.VarBool, Internal: true}, Value: false},
		},
		prgs: []Program{
			{Key: "markise", Name: "Markise", Operable: true, Visible: true},
			{Key: "fest", Name: "Fest", Visible: true},
			{Key: "verborgen", Name: "Verborgen", Operable: true, Internal: true},
		},
	}
	h := start(t, Options{Backend: b})
	by := map[string]map[string]any{}
	for _, x := range h.ok("listDevices").([]any) {
		d := x.(map[string]any)
		by[d["ADDRESS"].(string)] = d
	}
	// the value channel says visible (1) and internal (2); channel 0 is a plain visible one
	for a, flags := range map[string]int{"var-regen:1": 1, "var-halb:1": 3, "var-versteckt:1": 0, "var-intern:1": 2, "prg-markise:1": 1, "prg-verborgen:1": 2,
		"var-versteckt:0": 1, "prg-verborgen:0": 1} {
		if by[a]["FLAGS"] != flags {
			t.Errorf("FLAGS of %s: %v, want %d", a, by[a]["FLAGS"], flags)
		}
	}
	// each of them is a shape of its own: labels that say nothing are none
	fw := func(a string) any { return by[a]["FIRMWARE"] }
	if fw("var-regen") != fw("var-leer") || fw("var-regen") == fw("var-anwesend") || fw("var-regen") == fw("var-versteckt") || fw("var-versteckt") == fw("var-intern") ||
		fw("prg-markise") == fw("prg-fest") || fw("prg-markise") == fw("prg-verborgen") {
		t.Error("firmware fingerprints")
	}
	desc := func(addr string) map[string]any {
		return h.ok("getParamsetDescription", addr, "VALUES").(map[string]any)
	}
	// a boolean's own texts: false first, an empty one where only one is said; still a BOOL
	v := desc("var-anwesend:1")["VALUE"].(map[string]any)
	if v["TYPE"] != "BOOL" || !reflect.DeepEqual(v["VALUE_LIST"], []any{"abwesend", "anwesend"}) {
		t.Errorf("bool with texts: %v", v)
	}
	if v = desc("var-halb:1")["VALUE"].(map[string]any); v["TYPE"] != "BOOL" || !reflect.DeepEqual(v["VALUE_LIST"], []any{"", "ja"}) {
		t.Errorf("alarm with one text: %v", v)
	}
	for _, a := range []string{"var-regen:1", "var-leer:1"} {
		if _, has := desc(a)["VALUE"].(map[string]any)["VALUE_LIST"]; has {
			t.Errorf("%s: no texts are no list", a)
		}
	}
	if got := h.ok("getValue", "var-anwesend:1", "VALUE"); got != true {
		t.Errorf("a boolean with texts reads as a boolean: %v", got)
	}
	// a program takes RUN only when it is operable, and its description says so
	for a, ops := range map[string]int{"prg-markise:1": 2, "prg-fest:1": 0} {
		d := desc(a)
		if d["RUN"].(map[string]any)["OPERATIONS"] != ops || d["RUN_THEN"].(map[string]any)["OPERATIONS"] != ops || d["ACTIVE"].(map[string]any)["OPERATIONS"] != 7 {
			t.Errorf("operations of %s: %v", a, d)
		}
	}
	if _, err := h.call("setValue", "prg-fest:1", "RUN_THEN", true); err == nil || !strings.Contains(err.Error(), "not operable") {
		t.Errorf("the refusal says why: %v", err)
	}
	h.fault(FaultNotSupported, "setValue", "prg-fest:1", "RUN", true)
	h.ok("setValue", "prg-fest:1", "ACTIVE", true)
	h.ok("setValue", "prg-markise:1", "RUN", true)
}

func TestParamsetDescriptions(t *testing.T) {
	h := start(t, Options{})
	desc := func(addr string) map[string]any {
		return h.ok("getParamsetDescription", addr, "VALUES").(map[string]any)
	}
	v := desc("var-regen:1")["VALUE"].(map[string]any)
	if v["TYPE"] != "BOOL" || v["OPERATIONS"] != 7 || v["ID"] != "VALUE" || v["DEFAULT"] != false || v["FLAGS"] != 1 {
		t.Errorf("bool: %v", v)
	}
	v = desc("var-modus:1")["VALUE"].(map[string]any)
	if v["TYPE"] != "ENUM" || v["MIN"] != 0 || v["MAX"] != 2 || !reflect.DeepEqual(v["VALUE_LIST"], []any{"Auto", "Manuell", "Urlaub"}) {
		t.Errorf("list: %v", v)
	}
	v = desc("var-temperatur:1")["VALUE"].(map[string]any)
	if v["TYPE"] != "FLOAT" || v["MIN"] != -40.0 || v["MAX"] != 60.0 || v["UNIT"] != "°C" || v["DEFAULT"] != 0.0 {
		t.Errorf("number: %v", v)
	}
	v = desc("var-hinweis:1")["VALUE"].(map[string]any)
	if v["TYPE"] != "STRING" || v["OPERATIONS"] != 7 {
		t.Errorf("string: %v", v)
	}
	// a system value: a number nobody writes, without limits of its own
	v = desc("var-alarm-messages:1")["VALUE"].(map[string]any)
	if v["TYPE"] != "FLOAT" || v["OPERATIONS"] != 5 || v["MAX"] != float64(math.MaxFloat32) {
		t.Errorf("system value: %v", v)
	}
	a := desc("var-alarm-wasser:1")
	if len(a) != 5 || a["ACKNOWLEDGE"].(map[string]any)["TYPE"] != "ACTION" || a["ACKNOWLEDGE"].(map[string]any)["OPERATIONS"] != 2 ||
		a["COUNT"].(map[string]any)["OPERATIONS"] != 5 || a["FIRST"].(map[string]any)["UNIT"] != "s" || a["LAST"].(map[string]any)["TYPE"] != "INTEGER" {
		t.Errorf("alarm: %v", a)
	}
	p := desc("prg-markise:1")
	if len(p) != 4 || p["ACTIVE"].(map[string]any)["OPERATIONS"] != 7 || p["RUN"].(map[string]any)["TYPE"] != "ACTION" ||
		p["RUN_THEN"].(map[string]any)["OPERATIONS"] != 2 || p["LAST_RUN"].(map[string]any)["OPERATIONS"] != 5 {
		t.Errorf("program: %v", p)
	}
	for _, addr := range []string{"var-regen", "var-regen:0", "var-regen:1"} {
		if got := h.ok("getParamsetDescription", addr, "MASTER"); len(got.(map[string]any)) != 0 {
			t.Errorf("MASTER of %s: %v", addr, got)
		}
	}
	if got := desc("var-regen:0"); len(got) != 0 {
		t.Errorf("the maintenance channel: %v", got)
	}
	h.fault(FaultUnknownSet, "getParamsetDescription", "var-regen", "VALUES")
	h.fault(FaultUnknownSet, "getParamsetDescription", "var-regen:1", "LINK")
	h.fault(FaultUnknownDevice, "getParamsetDescription", "var-nichts:1", "VALUES")
	h.fault(FaultInvalidRequest, "getParamsetDescription", "var-regen:1")
}

func TestReads(t *testing.T) {
	h := start(t, Options{})
	for _, c := range []struct {
		addr, name string
		want       any
	}{
		{"var-regen:1", "VALUE", false},
		{"var-modus:1", "VALUE", 1}, // a list's entry is its index on the wire
		{"var-temperatur:1", "VALUE", 21.5},
		{"var-hinweis:1", "VALUE", "hallo <welt> & co"},
		{"var-alarm-wasser:1", "VALUE", true},
		{"var-alarm-wasser:1", "COUNT", 2},
		{"var-alarm-wasser:1", "FIRST", 1790000000},
		{"var-alarm-wasser:1", "LAST", 1790000100},
		{"var-alarm-messages:1", "VALUE", 1.0},
		{"prg-markise:1", "ACTIVE", true},
		{"prg-markise:1", "LAST_RUN", 1},
		{"prg-intern:1", "ACTIVE", false},
	} {
		if got := h.ok("getValue", c.addr, c.name); got != c.want {
			t.Errorf("%s %s: %#v", c.addr, c.name, got)
		}
	}
	if got := h.ok("getParamset", "var-alarm-wasser:1", "VALUES"); !reflect.DeepEqual(got, map[string]any{"VALUE": true, "COUNT": 2, "FIRST": 1790000000, "LAST": 1790000100}) {
		t.Errorf("getParamset: %v", got)
	}
	if got := h.ok("getParamset", "prg-markise:1", "VALUES"); !reflect.DeepEqual(got, map[string]any{"ACTIVE": true, "LAST_RUN": 1}) {
		t.Errorf("getParamset: %v", got)
	}
	if got := h.ok("getParamset", "prg-markise:0", "VALUES"); len(got.(map[string]any)) != 0 {
		t.Errorf("getParamset of channel 0: %v", got)
	}
	h.fault(FaultUnknownParam, "getValue", "var-regen:1", "LEVEL")
	h.fault(FaultUnknownParam, "getValue", "var-regen:0", "VALUE")
	h.fault(FaultUnknownParam, "getValue", "var-regen", "VALUE")
	h.fault(FaultNotSupported, "getValue", "var-alarm-wasser:1", "ACKNOWLEDGE")
	h.fault(FaultNotSupported, "getValue", "prg-markise:1", "RUN")
	h.fault(FaultUnknownDevice, "getValue", "var-nichts:1", "VALUE")
	h.fault(FaultInvalidRequest, "getValue", "var-regen:1")
	// an alarm that never triggered, and an object that went since the devices were built
	h.b.mu.Lock()
	h.b.vars[4].Alarm = nil
	h.b.vars = h.b.vars[1:]
	h.b.prgs = h.b.prgs[1:]
	h.b.mu.Unlock()
	if got := h.ok("getValue", "var-alarm-wasser:1", "COUNT"); got != 0 {
		t.Errorf("count without a state: %v", got)
	}
	if got := h.ok("getValue", "var-alarm-wasser:1", "FIRST"); got != 0 {
		t.Errorf("first without a state: %v", got)
	}
	h.fault(FaultUnknownDevice, "getValue", "var-regen:1", "VALUE")
	h.fault(FaultUnknownDevice, "getValue", "prg-markise:1", "ACTIVE")
	h.fault(FaultUnknownDevice, "getParamset", "var-regen:1", "VALUES")
}

func TestWrites(t *testing.T) {
	h := start(t, Options{})
	for _, c := range []struct {
		addr, name string
		value      any
		want       string
	}{
		{"var-regen:1", "VALUE", true, "set regen=true"},
		{"var-regen:1", "VALUE", 0, "set regen=false"},
		{"var-regen:1", "VALUE", 1.0, "set regen=true"},
		{"var-regen:1", "VALUE", "false", "set regen=false"},
		{"var-regen:1", "VALUE", "TRUE", "set regen=true"},
		{"var-modus:1", "VALUE", 2, `set modus="Urlaub"`},
		{"var-modus:1", "VALUE", 0.0, `set modus="Auto"`},
		{"var-modus:1", "VALUE", "Manuell", `set modus="Manuell"`},
		{"var-modus:1", "VALUE", " 1 ", `set modus="Manuell"`},
		{"var-temperatur:1", "VALUE", 19.5, "set temperatur=19.5"},
		{"var-temperatur:1", "VALUE", 20, "set temperatur=20"},
		{"var-temperatur:1", "VALUE", " 20.25 ", "set temperatur=20.25"},
		{"var-hinweis:1", "VALUE", "neu", `set hinweis="neu"`},
		{"var-hinweis:1", "VALUE", 5, `set hinweis="5"`},
		{"var-hinweis:1", "VALUE", true, `set hinweis="true"`},
		{"var-alarm-wasser:1", "VALUE", false, "set alarm-wasser=false"},
		{"var-alarm-wasser:1", "ACKNOWLEDGE", true, "acknowledge alarm-wasser"},
		{"prg-markise:1", "ACTIVE", false, "active markise=false"},
		{"prg-intern:1", "ACTIVE", "1", "active intern=true"},
		{"prg-markise:1", "RUN", true, "run markise then=false"},
		{"prg-markise:1", "RUN_THEN", true, "run markise then=true"},
	} {
		if got := h.ok("setValue", c.addr, c.name, c.value); got != "" {
			t.Errorf("setValue answered %v", got)
		}
		if done := h.b.done(); len(done) != 1 || done[0] != c.want {
			t.Errorf("%s %s=%#v: %v", c.addr, c.name, c.value, done)
		}
	}
	// what the interface refuses itself
	for _, c := range []struct {
		code       int
		addr, name string
		value      any
	}{
		{FaultUnknownParam, "var-regen:1", "VALUE", "vielleicht"},
		{FaultUnknownParam, "var-regen:1", "VALUE", []any{}},
		{FaultUnknownParam, "var-modus:1", "VALUE", 3},
		{FaultUnknownParam, "var-modus:1", "VALUE", -1},
		{FaultUnknownParam, "var-modus:1", "VALUE", 1.5},
		{FaultUnknownParam, "var-modus:1", "VALUE", "Winter"},
		{FaultUnknownParam, "var-modus:1", "VALUE", true},
		{FaultUnknownParam, "var-temperatur:1", "VALUE", "warm"},
		{FaultUnknownParam, "var-temperatur:1", "VALUE", true},
		{FaultUnknownParam, "var-hinweis:1", "VALUE", map[string]any{}},
		{FaultUnknownParam, "var-regen:1", "LEVEL", 1},
		{FaultUnknownParam, "prg-markise:1", "ACTIVE", "vielleicht"},
		{FaultNotSupported, "var-alarm-wasser:1", "COUNT", 5},
		{FaultNotSupported, "prg-markise:1", "LAST_RUN", 5},
		{FaultNotSupported, "var-alarm-messages:1", "VALUE", 5.0},
		{FaultNotSupported, "prg-intern:1", "RUN", true},      // not operable
		{FaultNotSupported, "prg-intern:1", "RUN_THEN", true}, // not operable
		{FaultUnknownParam, "var-regen:0", "VALUE", true},
		{FaultUnknownDevice, "var-nichts:1", "VALUE", true},
	} {
		h.fault(c.code, "setValue", c.addr, c.name, c.value)
	}
	h.fault(FaultInvalidRequest, "setValue", "var-regen:1", "VALUE")
	if done := h.b.done(); len(done) != 0 {
		t.Errorf("a refused write reached the backend: %v", done)
	}
	if got := h.ok("setValue", "var-hinweis:1", "VALUE", ""); got != "" || h.b.done()[0] != `set hinweis=""` {
		t.Error("an empty string")
	}
	// what the backend refuses: limits (OQ-23), a read-only value, anything else
	for code, err := range map[int]error{
		FaultUnknownParam:  &engine.WriteError{Code: "out-of-range", Msg: "temperatur takes -40 to 60"},
		FaultNotSupported:  &engine.WriteError{Code: "read-only", Msg: "read-only"},
		FaultUnknownDevice: &engine.WriteError{Code: "unknown-variable", Msg: "gone"},
		FaultGeneral:       errors.New("the disk is full"),
	} {
		h.b.mu.Lock()
		h.b.fail = err
		h.b.mu.Unlock()
		h.fault(code, "setValue", "var-temperatur:1", "VALUE", 99.0)
	}
	h.b.mu.Lock()
	h.b.fail = nil
	h.b.mu.Unlock()
	h.b.done()
	// putParamset writes each value, in the datapoints' order
	h.ok("putParamset", "var-alarm-wasser:1", "VALUES", map[string]any{"VALUE": true, "ACKNOWLEDGE": true})
	if done := h.b.done(); !reflect.DeepEqual(done, []string{"acknowledge alarm-wasser", "set alarm-wasser=true"}) {
		t.Errorf("putParamset: %v", done)
	}
	h.fault(FaultUnknownParam, "putParamset", "var-regen:1", "VALUES", map[string]any{"LEVEL": 1})
	h.fault(FaultUnknownSet, "putParamset", "var-regen:1", "MASTER", map[string]any{})
	h.fault(FaultUnknownDevice, "putParamset", "var-nichts:1", "VALUES", map[string]any{})
	h.fault(FaultInvalidRequest, "putParamset", "var-regen:1", "VALUES", "x")
	h.fault(FaultInvalidRequest, "putParamset", "var-regen:1", "VALUES")
}

func TestSystemMethods(t *testing.T) {
	h := start(t, Options{})
	if got := h.ok("system.listMethods"); len(got.([]any)) != len(Methods) || !sort.StringsAreSorted(Methods) {
		t.Errorf("%v", got)
	}
	if got := h.ok("getVersion"); got != "1.2.3" {
		t.Errorf("%v", got)
	}
	if got := h.ok("getServiceMessages"); len(got.([]any)) != 0 {
		t.Errorf("%v", got)
	}
	h.fault(FaultUnknownMethod, "listBidcosInterfaces")
	got := h.ok("system.multicall", []any{
		map[string]any{"methodName": "getValue", "params": []any{"var-modus:1", "VALUE"}},
		map[string]any{"methodName": "getValue", "params": []any{"var-nichts:1", "VALUE"}},
		map[string]any{"methodName": "system.multicall", "params": []any{[]any{}}},
		"nonsense",
	}).([]any)
	if len(got) != 4 || !reflect.DeepEqual(got[0], []any{1}) || got[1].(map[string]any)["faultCode"] != FaultUnknownDevice ||
		got[2].(map[string]any)["faultCode"] != FaultInvalidRequest || got[3].(map[string]any)["faultCode"] != FaultUnknownMethod {
		t.Errorf("multicall: %v", got)
	}
	h.fault(FaultInvalidRequest, "system.multicall", "x")
	h.fault(FaultInvalidRequest, "system.multicall")
	// not XML-RPC
	res, err := http.Get(h.url)
	if err != nil || res.StatusCode != http.StatusMethodNotAllowed || res.Header.Get("Allow") != "POST" {
		t.Errorf("GET: %v %v", res, err)
	}
	for _, body := range []string{"hello", "<methodCall><params/></methodCall>", "<methodResponse/>", "<methodCall><methodName>x</methodName><params><param><value><i4>x</i4></value></param></params></methodCall>",
		strings.Repeat("x", MaxBody+1)} {
		res, err := http.Post(h.url, "text/xml", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		var f *Fault
		if _, err := DecodeResponse(data); !errors.As(err, &f) || f.Code != FaultInvalidRequest {
			t.Errorf("%.20q: %v", body, err)
		}
	}
}

// init: the interface asks the client what it knows, removes what is gone and announces what is
// new, as rfd does; then every write is an event to every subscriber.
func TestSubscribers(t *testing.T) {
	h := start(t, Options{})
	c := newClient(t)
	c.knows = []any{
		map[string]any{"ADDRESS": "var-regen", "VERSION": 1}, map[string]any{"ADDRESS": "var-regen:0", "VERSION": 1}, map[string]any{"ADDRESS": "var-regen:1", "VERSION": 1},
		map[string]any{"ADDRESS": "var-alt", "VERSION": 1}, map[string]any{"ADDRESS": "var-alt:1", "VERSION": 1},
		map[string]any{"ADDRESS": "prg-markise", "VERSION": 1}, // without its channels: announced again
		"nonsense", map[string]any{"NAME": "x"},
	}
	if got := h.ok("init", c.srv.URL, "occulited_Logic"); got != "" {
		t.Errorf("init answered %v", got)
	}
	c.wait(3)
	calls, _ := c.take()
	want := []string{"listDevices [occulited_Logic]", "deleteDevices occulited_Logic var-alt,var-alt:1",
		"newDevices occulited_Logic prg-intern,prg-intern:0,prg-intern:1,prg-markise,prg-markise:0,prg-markise:1,var-alarm-messages,var-alarm-messages:0,var-alarm-messages:1," +
			"var-alarm-wasser,var-alarm-wasser:0,var-alarm-wasser:1,var-hinweis,var-hinweis:0,var-hinweis:1,var-modus,var-modus:0,var-modus:1,var-temperatur,var-temperatur:0,var-temperatur:1"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("after init:\n%v", strings.Join(calls, "\n"))
	}
	if !reflect.DeepEqual(h.s.Subscribers(), []string{c.srv.URL}) {
		t.Errorf("subscribers: %v", h.s.Subscribers())
	}
	// a client that keeps a device with another FIRMWARE (the addon was updated and the shape changed
	// since it last heard): the device goes and comes; one with the same FIRMWARE stays as it is
	e := newClient(t)
	all := h.ok("listDevices").([]any)
	for _, x := range all {
		m := x.(map[string]any)
		switch m["ADDRESS"] {
		case "var-modus":
			m["FIRMWARE"] = "1.0"
		}
	}
	e.knows = all
	h.ok("init", e.srv.URL, "older")
	e.wait(3)
	if calls, _ := e.take(); len(calls) != 3 || calls[1] != "deleteDevices older var-modus,var-modus:0,var-modus:1" || calls[2] != "newDevices older var-modus,var-modus:0,var-modus:1" {
		t.Errorf("a client with an old FIRMWARE: %v", calls)
	}
	h.ok("init", e.srv.URL, "")
	// a second client that does not know listDevices: everything is announced; the xmlrpc:// spelling
	d := newClient(t)
	h.ok("init", "xmlrpc://"+strings.TrimPrefix(d.srv.URL, "http://"), "hm2mqtt")
	d.wait(2)
	if calls, _ := d.take(); len(calls) != 2 || !strings.HasPrefix(calls[1], "newDevices hm2mqtt prg-intern,") || strings.Count(calls[1], ",") != 23 {
		t.Errorf("a client without listDevices: %v", calls)
	}
	// a write from outside is an event to both, the writer included
	h.ok("setValue", "var-modus:1", "VALUE", 2)
	c.wait(1)
	d.wait(1)
	if _, ev := c.take(); !reflect.DeepEqual(ev, []string{"occulited_Logic var-modus:1 VALUE=2"}) {
		t.Errorf("event: %v", ev)
	}
	if _, ev := d.take(); !reflect.DeepEqual(ev, []string{"hm2mqtt var-modus:1 VALUE=2"}) {
		t.Errorf("event: %v", ev)
	}
	// an alarm's write carries its counter and times
	h.ok("setValue", "var-alarm-wasser:1", "VALUE", true)
	c.wait(1)
	d.wait(1)
	if _, ev := c.take(); !reflect.DeepEqual(ev, []string{"occulited_Logic var-alarm-wasser:1 VALUE=true", "occulited_Logic var-alarm-wasser:1 COUNT=3",
		"occulited_Logic var-alarm-wasser:1 FIRST=1700000000", "occulited_Logic var-alarm-wasser:1 LAST=1800000000"}) {
		t.Errorf("alarm events: %v", ev)
	}
	d.take()
	// what the engine tells about programs, and about values of every type
	active := false
	for _, n := range []engine.Notification{
		{Kind: "program", Key: "markise", Block: 2},
		{Kind: "program", Key: "markise", Active: &active},
		{Kind: "variable", Key: "regen", Value: true},
		{Kind: "variable", Key: "temperatur", Value: 3.5},
		{Kind: "variable", Key: "hinweis", Value: "x"},
		{Kind: "variable", Key: "modus", Value: "gibt es nicht"},
		{Kind: "variable", Key: model.SystemAlarmMessages, Value: 2.0},
		{Kind: "variable", Key: "unbekannt", Value: 1.0}, // no device: nothing
		{Kind: "program", Key: "unbekannt", Block: 1},    // no device: nothing
		{Kind: "program", Key: "markise", Paused: true},  // the loop guard's pause: nothing
		{Kind: "sonstwas", Key: "regen"},
	} {
		h.s.Notify(n)
	}
	c.wait(7)
	d.wait(7)
	if _, ev := c.take(); !reflect.DeepEqual(ev, []string{"occulited_Logic prg-markise:1 LAST_RUN=2", "occulited_Logic prg-markise:1 ACTIVE=false", "occulited_Logic var-regen:1 VALUE=true",
		"occulited_Logic var-temperatur:1 VALUE=3.5", `occulited_Logic var-hinweis:1 VALUE="x"`, "occulited_Logic var-modus:1 VALUE=0", "occulited_Logic var-alarm-messages:1 VALUE=2"}) {
		t.Errorf("events:\n%v", strings.Join(ev, "\n"))
	}
	d.take()
	// ping: a PONG event to everyone
	if got := h.ok("ping", "abc"); got != true {
		t.Errorf("ping: %v", got)
	}
	c.wait(1)
	d.wait(1)
	if _, ev := c.take(); !reflect.DeepEqual(ev, []string{`occulited_Logic CENTRAL PONG="abc"`}) {
		t.Errorf("pong: %v", ev)
	}
	d.take()
	// init with an empty id ends the subscription
	h.ok("init", d.srv.URL, "")
	h.ok("setValue", "var-regen:1", "VALUE", false)
	c.wait(1)
	c.take()
	d.quiet()
	// only this system is called back
	h.fault(FaultGeneral, "init", "http://192.0.2.1:9000", "x")
	h.fault(FaultGeneral, "init", "xmlrpc_bin://127.0.0.1:9000", "x")
	h.fault(FaultGeneral, "init", "http://", "x")
	h.fault(FaultGeneral, "init", "://", "x")
	h.fault(FaultInvalidRequest, "init")
	if _, ok := loopback("http://localhost:1/x"); !ok {
		t.Error("localhost")
	}
	if _, ok := loopback("http://[::1]:1"); !ok {
		t.Error("::1")
	}
	if !strings.Contains(h.logged(), "occulited_Logic registered") {
		t.Errorf("log: %s", h.logged())
	}
}

// Objects come, go and change: the difference goes to the subscribers, and the names follow.
func TestChanges(t *testing.T) {
	names := &namer{}
	h := start(t, Options{Names: names, Retry: 20 * time.Millisecond})
	names.wait(t)
	first := names.take()
	if len(first) != 1 || len(first[0].set) != 24 || first[0].set["Logic.var-regen"] != "Regen" || first[0].set["Logic.var-regen:1"] != "Regen" ||
		first[0].set["Logic.var-regen:0"] != "Regen:0" || first[0].set["Logic.prg-markise:1"] != "Markise bei Regen einfahren" || len(first[0].remove) != 0 {
		t.Fatalf("the first names: %+v", first)
	}
	c := newClient(t)
	c.knows = h.ok("listDevices").([]any)
	h.ok("init", c.srv.URL, "x")
	c.wait(1) // listDevices; nothing to announce
	c.take()
	c.quiet()

	h.b.mu.Lock()
	h.b.vars[0].Def = &model.Variable{Key: "regen", Name: "Regen Terrasse", Type: model.VarBool}                              // renamed
	h.b.vars[1].Def = &model.Variable{Key: "modus", Name: "Modus", Type: model.VarList, Entries: []string{"Auto", "Manuell"}} // another shape
	h.b.vars[2] = engine.VariableInfo{Def: &model.Variable{Key: "neu", Name: "Neu", Type: model.VarString}, Value: ""}        // temperatur went, neu came
	h.b.vars[3].Def = &model.Variable{Key: "hinweis", Name: "Hinweis", Type: model.VarAlarm}                                  // a string became an alarm
	h.b.prgs = h.b.prgs[:1]                                                                                                   // intern went
	h.b.mu.Unlock()
	h.s.Changed()
	c.wait(2)
	calls, _ := c.take()
	want := []string{
		"deleteDevices x prg-intern,prg-intern:0,prg-intern:1,var-hinweis,var-hinweis:0,var-hinweis:1,var-modus,var-modus:0,var-modus:1,var-temperatur,var-temperatur:0,var-temperatur:1",
		"newDevices x var-hinweis,var-hinweis:0,var-hinweis:1,var-modus,var-modus:0,var-modus:1,var-neu,var-neu:0,var-neu:1"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("after the change:\n%s", strings.Join(calls, "\n"))
	}
	names.wait(t)
	second := names.take()
	if len(second) != 1 || !reflect.DeepEqual(second[0].set, map[string]string{"Logic.var-regen": "Regen Terrasse", "Logic.var-regen:0": "Regen Terrasse:0", "Logic.var-regen:1": "Regen Terrasse",
		"Logic.var-neu": "Neu", "Logic.var-neu:0": "Neu:0", "Logic.var-neu:1": "Neu"}) ||
		!reflect.DeepEqual(second[0].remove, []string{"Logic.prg-intern", "Logic.prg-intern:0", "Logic.prg-intern:1", "Logic.var-temperatur", "Logic.var-temperatur:0", "Logic.var-temperatur:1"}) {
		t.Errorf("the names after the change: %+v", second)
	}
	// nothing changed: nothing is announced, no name is written
	h.s.Changed()
	c.quiet()
	if got := names.take(); len(got) != 0 {
		t.Errorf("names without a change: %+v", got)
	}
	// the system does not take the names: tried again until it does
	names.mu.Lock()
	names.fail = 2
	names.mu.Unlock()
	h.b.mu.Lock()
	h.b.prgs[0].Name = "Markise"
	h.b.mu.Unlock()
	h.s.Changed()
	names.wait(t)
	names.wait(t)
	names.wait(t)
	got := names.take()
	if len(got) != 3 || !reflect.DeepEqual(got[2].set, map[string]string{"Logic.prg-markise": "Markise", "Logic.prg-markise:0": "Markise:0", "Logic.prg-markise:1": "Markise"}) {
		t.Errorf("retried names: %+v", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(h.logged(), "the names are written") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if log := h.logged(); strings.Count(log, "could not be written") != 1 || !strings.Contains(log, "the names are written") {
		t.Errorf("log: %s", log)
	}
}

// A name of an object that went away while the addon did not run is still in the metadata: the
// interface reads what is there once after its start, removes what no device has any more and
// writes only what differs.
func TestNamesOfObjectsGoneWhileStopped(t *testing.T) {
	names := &lister{known: map[string]string{
		"Logic.var-regen": "Regen", "Logic.var-regen:0": "Regen:0", "Logic.var-regen:1": "Regen alt", // one differs
		"Logic.var-weg": "Weg", "Logic.var-weg:0": "Weg:0", "Logic.var-weg:1": "Weg", // gone while stopped
		"Logic.prg-alt": "Alt"}}
	h := start(t, Options{Names: names, Retry: 20 * time.Millisecond})
	names.wait(t)
	first := names.take()
	if len(first) != 1 || len(first[0].set) != 22 || first[0].set["Logic.var-regen:1"] != "Regen" || first[0].set["Logic.var-regen"] != "" ||
		!reflect.DeepEqual(first[0].remove, []string{"Logic.prg-alt", "Logic.var-weg", "Logic.var-weg:0", "Logic.var-weg:1"}) {
		t.Fatalf("the first names: %+v", first)
	}
	// read once: a later change asks no more
	h.b.mu.Lock()
	h.b.prgs[0].Name = "Markise"
	h.b.mu.Unlock()
	h.s.Changed()
	names.wait(t)
	if got := names.take(); len(got) != 1 || len(got[0].set) != 3 || len(got[0].remove) != 0 {
		t.Errorf("after a change: %+v", got)
	}
	if n := names.asked(); n != 1 {
		t.Errorf("the list was read %d times", n)
	}
}

// While the list cannot be read the names are written as before, and the reading is tried again:
// what is stale goes once it succeeds.
func TestNamesListNotReadable(t *testing.T) {
	names := &lister{known: map[string]string{"Logic.var-weg": "Weg", "Logic.var-regen": "Regen"}, failRead: 2}
	h := start(t, Options{Names: names, Retry: 20 * time.Millisecond})
	names.wait(t)
	names.wait(t)
	got := names.take()
	// blind: everything, nothing removed; then the list is there: only the stale name is left to do
	if len(got) != 2 || len(got[0].set) != 24 || len(got[0].remove) != 0 || len(got[1].set) != 0 || !reflect.DeepEqual(got[1].remove, []string{"Logic.var-weg"}) {
		t.Fatalf("the names: %+v", got)
	}
	if n := names.asked(); n != 3 {
		t.Errorf("the list was read %d times", n)
	}
	if log := h.logged(); strings.Count(log, "the names the system holds could not be read") != 1 {
		t.Errorf("log: %s", log)
	}
}

// lister is a namer that also knows what the metadata holds.
type lister struct {
	namer
	lmu      sync.Mutex
	known    map[string]string
	failRead int
	reads    int
}

func (l *lister) Known(context.Context) (map[string]string, error) {
	l.lmu.Lock()
	defer l.lmu.Unlock()
	l.reads++
	if l.failRead > 0 {
		l.failRead--
		return nil, errors.New("503")
	}
	out := map[string]string{}
	for k, v := range l.known {
		out[k] = v
	}
	return out, nil
}

// Push keeps the list as the metadata would.
func (l *lister) Push(ctx context.Context, set map[string]string, remove []string) error {
	l.lmu.Lock()
	for k, v := range set {
		l.known[k] = v
	}
	for _, k := range remove {
		delete(l.known, k)
	}
	l.lmu.Unlock()
	return l.namer.Push(ctx, set, remove)
}

func (l *lister) asked() int {
	l.lmu.Lock()
	defer l.lmu.Unlock()
	return l.reads
}

type push struct {
	set    map[string]string
	remove []string
}

type namer struct {
	mu     sync.Mutex
	pushes []push
	fail   int
	got    chan struct{}
	once   sync.Once
}

func (n *namer) Push(_ context.Context, set map[string]string, remove []string) error {
	n.once.Do(func() { n.mu.Lock(); n.got = make(chan struct{}, 100); n.mu.Unlock() })
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pushes = append(n.pushes, push{set, remove})
	n.got <- struct{}{}
	if n.fail > 0 {
		n.fail--
		return errors.New("503")
	}
	return nil
}

func (n *namer) wait(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		n.mu.Lock()
		ch := n.got
		n.mu.Unlock()
		if ch != nil {
			select {
			case <-ch:
				return
			case <-time.After(time.Until(deadline)):
				t.Fatal("no names were written")
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no names were written")
		}
		time.Sleep(time.Millisecond)
	}
}

func (n *namer) take() []push {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.pushes
	n.pushes = nil
	return out
}

// A subscriber that stops answering is dropped after ten failures; one that answers with a fault
// is alive.
func TestDeadSubscriber(t *testing.T) {
	h := start(t, Options{Client: &http.Client{Timeout: time.Second}})
	c := newClient(t)
	c.knows = h.ok("listDevices").([]any)
	h.ok("init", c.srv.URL, "x")
	c.wait(1)
	c.mu.Lock()
	c.status = http.StatusInternalServerError
	c.mu.Unlock()
	for i := 0; i < maxFails-1; i++ {
		h.s.Notify(engine.Notification{Kind: "variable", Key: "regen", Value: true})
	}
	c.wait(maxFails - 1)
	if len(h.s.Subscribers()) != 1 {
		t.Fatal("dropped too early")
	}
	// an answer in between starts the count anew
	c.mu.Lock()
	c.status = 0
	c.mu.Unlock()
	h.s.Notify(engine.Notification{Kind: "variable", Key: "regen", Value: true})
	c.wait(1)
	c.mu.Lock()
	c.status = http.StatusInternalServerError
	c.mu.Unlock()
	for i := 0; i < maxFails; i++ {
		h.s.Notify(engine.Notification{Kind: "variable", Key: "regen", Value: true})
	}
	c.wait(maxFails)
	deadline := time.Now().Add(2 * time.Second)
	for len(h.s.Subscribers()) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(h.s.Subscribers()) != 0 || !strings.Contains(h.logged(), "did not answer 10 times, dropped") {
		t.Errorf("subscribers %v, log: %s", h.s.Subscribers(), h.logged())
	}
	// a subscriber that registered anew while a call to its old self was under way is not counted against
	sub := &subscriber{url: "http://127.0.0.1:1/", id: "old"}
	h.s.mu.Lock()
	h.s.subs[sub.url] = &subscriber{url: sub.url, id: "new"}
	h.s.mu.Unlock()
	if _, err := h.s.call(sub, "event"); err == nil {
		t.Error("a call to nobody succeeded")
	}
	h.s.mu.Lock()
	fails := h.s.subs[sub.url].fails
	h.s.mu.Unlock()
	if fails != 0 {
		t.Errorf("the new subscriber was charged %d failures", fails)
	}
}

// The queue never blocks the engine: beyond its size events are dropped and counted.
func TestQueueOverflow(t *testing.T) {
	b := newFake()
	s := New(Options{Backend: b, Queue: 2, Log: func(string, ...any) {}})
	for i := 0; i < 5; i++ {
		s.Notify(engine.Notification{Kind: "variable", Key: "regen", Value: true})
	}
	if s.Dropped() != 3 {
		t.Errorf("dropped %d", s.Dropped())
	}
	var lines int
	s = New(Options{Backend: b, Queue: 1, Log: func(string, ...any) { lines++ }})
	for i := 0; i < 202; i++ {
		s.Changed()
	}
	if s.Dropped() != 201 || lines != 3 {
		t.Errorf("dropped %d, %d log lines", s.Dropped(), lines)
	}
	// silent without a log, and the defaults
	s = New(Options{Backend: b})
	if cap(s.jobs) != 1024 || s.o.Retry != time.Minute || s.o.Client.Timeout != 5*time.Second {
		t.Errorf("defaults: %d %v", cap(s.jobs), s.o.Retry)
	}
	s.logf("nobody listens")
	// Run stops a pending retry of the names
	n := &namer{fail: 1000}
	s = New(Options{Backend: b, Names: n, Retry: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Run(ctx) }()
	n.wait(t)
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		armed := s.namesTimer != nil
		s.mu.Unlock()
		if armed || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// a second failure while a retry is armed replaces the timer
	s.Changed()
	n.wait(t)
	cancel()
	<-done
}

func TestMetaNames(t *testing.T) {
	var got struct {
		Set    map[string]map[string]string `json:"set"`
		Delete []string                     `json:"delete"`
	}
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/meta/v1/interfaces/Logic/objects:bulk" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		switch status {
		case http.StatusOK:
		case http.StatusNotModified:
			w.WriteHeader(status)
		default:
			http.Error(w, `{"error":{"code":"not-your-interface"}}`, status)
		}
	}))
	defer srv.Close()
	m := MetaNames{Client: srv.Client(), Base: srv.URL + "/"}
	if err := m.Push(context.Background(), map[string]string{"Logic.var-regen": "Regen"}, []string{"Logic.var-alt"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Set, map[string]map[string]string{"Logic.var-regen": {"name": "Regen"}}) || !reflect.DeepEqual(got.Delete, []string{"Logic.var-alt"}) {
		t.Errorf("%+v", got)
	}
	// the system answers 304 when it has exactly these names: that is success
	status = http.StatusNotModified
	if err := m.Push(context.Background(), map[string]string{"Logic.var-regen": "Regen"}, nil); err != nil {
		t.Errorf("304 Not Modified is not an error: %v", err)
	}
	status = http.StatusNotFound
	if err := m.Push(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "not-your-interface") {
		t.Errorf("%v", err)
	}
	srv.Close()
	if err := m.Push(context.Background(), nil, nil); err == nil {
		t.Error("an unreachable system")
	}
	if err := (MetaNames{Client: http.DefaultClient, Base: "http://bad host\x7f"}).Push(context.Background(), nil, nil); err == nil {
		t.Error("a URL that is none")
	}
}

func TestMetaNamesKnown(t *testing.T) {
	answer, status := `{"revision":7,"objects":{"Logic.var-regen":{"name":"Regen"},"Logic.var-regen:1":{"name":"Regen","enums":["rooms/garten"]},"HmIP-RF.0001:1":{"name":"Licht"},"Logical.x":{"name":"other"}}}`, http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/meta/v1/objects" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	}))
	defer srv.Close()
	m := MetaNames{Client: srv.Client(), Base: srv.URL + "/"}
	got, err := m.Known(context.Background())
	if err != nil || !reflect.DeepEqual(got, map[string]string{"Logic.var-regen": "Regen", "Logic.var-regen:1": "Regen"}) {
		t.Errorf("%v %v", got, err)
	}
	answer = `{"objects":`
	if _, err := m.Known(context.Background()); err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Errorf("a cut answer: %v", err)
	}
	answer, status = `{"error":{"code":"forbidden"}}`, http.StatusForbidden
	if _, err := m.Known(context.Background()); err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("%v", err)
	}
	srv.Close()
	if _, err := m.Known(context.Background()); err == nil {
		t.Error("an unreachable system")
	}
	if _, err := (MetaNames{Client: http.DefaultClient, Base: "http://bad host\x7f"}).Known(context.Background()); err == nil {
		t.Error("a URL that is none")
	}
}

func TestCodec(t *testing.T) {
	values := []any{nil, true, false, 0, -7, 2147483647, 1.5, -0.25, "", "a <b> & \"c\" 'd' ü\n", []any{}, []any{1, "x", []any{true}},
		map[string]any{}, map[string]any{"b": 1, "a <": map[string]any{"x": []any{1.5}}}}
	for _, v := range values {
		args := []any{v}
		method, got, err := DecodeCall(EncodeCall("m.x", args...))
		want := v
		if v == nil {
			want = ""
		}
		if err != nil || method != "m.x" || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("%#v: %#v %v", v, got, err)
		}
		res, err := DecodeResponse(EncodeResponse(v, nil))
		if err != nil || !reflect.DeepEqual(res, want) {
			t.Errorf("response %#v: %#v %v", v, res, err)
		}
	}
	// what only the encoder takes
	var b bytes.Buffer
	encodeValue(&b, []string{"a", "b"})
	encodeValue(&b, math.NaN())
	encodeValue(&b, math.Inf(1))
	encodeValue(&b, int64(5))
	if got := b.String(); got != "<value><array><data><value><string>a</string></value><value><string>b</string></value></data></array></value>"+
		"<value><double>0</double></value><value><double>0</double></value><value><string>5</string></value>" {
		t.Errorf("%s", got)
	}
	// what other software writes
	for in, want := range map[string]any{
		"<value>plain</value>":                                                  "plain",
		"<value><int>5</int></value>":                                           5,
		"<value><i8>6</i8></value>":                                             6,
		"<value><boolean>true</boolean></value>":                                true,
		"<value><double> 2.5 </double></value>":                                 2.5,
		"<value><nil/></value>":                                                 nil,
		"<value><base64>QQ==</base64></value>":                                  "QQ==",
		"<value><dateTime.iso8601>20260101T00:00:00</dateTime.iso8601></value>": "20260101T00:00:00",
		"<value><array></array></value>":                                        []any{},
	} {
		_, got, err := DecodeCall([]byte("<methodCall><methodName> x </methodName><params><param>" + in + "</param></params></methodCall>"))
		if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("%s: %#v %v", in, got, err)
		}
	}
	for _, bad := range []string{
		"<value><i4>x</i4></value>", "<value><double>x</double></value>", "<value><double>NaN</double></value>", "<value><unknown/></value>",
		"<value><struct><member><value>1</value></member></struct></value>", "<value><struct><entry/></struct></value>",
		"<value><struct><member><name>a</name><value><i4>x</i4></value></member></struct></value>",
		"<value><array><data><value><i4>x</i4></value></data></array></value>", "<other/>",
		strings.Repeat("<value><array><data>", 40) + strings.Repeat("</data></array></value>", 40),
	} {
		if _, _, err := DecodeCall([]byte("<methodCall><methodName>x</methodName><params><param>" + bad + "</param></params></methodCall>")); err == nil {
			t.Errorf("%.60s was read", bad)
		}
	}
	if _, _, err := DecodeCall([]byte("<methodCall><methodName> </methodName></methodCall>")); err == nil {
		t.Error("a call without a method")
	}
	// answers: a fault, no parameter, damage
	_, err := DecodeResponse(EncodeResponse(nil, fault(-5, "nope <x>")))
	var f *Fault
	if !errors.As(err, &f) || f.Code != -5 || f.Msg != "nope <x>" || f.Error() != "nope <x>" {
		t.Errorf("%v", err)
	}
	if res, err := DecodeResponse([]byte("<methodResponse><params/></methodResponse>")); res != nil || err != nil {
		t.Errorf("%v %v", res, err)
	}
	for _, bad := range []string{"<methodCall/>", "<<<", "<methodResponse><fault><value><i4>x</i4></value></fault></methodResponse>",
		"<methodResponse><params><param><value><i4>x</i4></value></param></params></methodResponse>"} {
		if _, err := DecodeResponse([]byte(bad)); err == nil {
			t.Errorf("%s was read", bad)
		}
	}
}

func TestCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/500":
			http.Error(w, "no", http.StatusInternalServerError)
		case "/cut":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("<methodResponse>"))
		default:
			_, _ = w.Write(EncodeResponse("ok", nil))
		}
	}))
	defer srv.Close()
	if res, err := Call(context.Background(), srv.Client(), srv.URL, "x", 1); res != "ok" || err != nil {
		t.Errorf("%v %v", res, err)
	}
	if _, err := Call(context.Background(), srv.Client(), srv.URL+"/500", "x"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("%v", err)
	}
	if _, err := Call(context.Background(), srv.Client(), srv.URL+"/cut", "x"); err == nil {
		t.Error("a cut answer was read")
	}
	if _, err := Call(context.Background(), srv.Client(), "http://bad host\x7f", "x"); err == nil {
		t.Error("a URL that is none")
	}
	srv.Close()
	if _, err := Call(context.Background(), srv.Client(), srv.URL, "x"); err == nil {
		t.Error("an unreachable server")
	}
}

func TestConversions(t *testing.T) {
	text := &model.Variable{Key: "t", Type: model.VarString}
	if v, err := unwire(text, nil); v != "" || err != nil {
		t.Errorf("nil for a string: %v %v", v, err)
	}
	// values of the wrong Go type never panic on the way out
	for _, v := range []*model.Variable{text, {Type: model.VarBool}, {Type: model.VarNumber}, {Type: model.VarList, Entries: []string{"a"}}} {
		if got := wire(v, struct{}{}); got != "" && got != false && got != 0.0 && got != 0 {
			t.Errorf("%s: %#v", v.Type, got)
		}
	}
	if unix(time.Time{}) != 0 || unix(time.Unix(5, 0)) != 5 {
		t.Error("unix")
	}
}

func FuzzDecodeCall(f *testing.F) {
	f.Add(EncodeCall("setValue", "var-regen:1", "VALUE", true))
	f.Add(EncodeCall("system.multicall", []any{map[string]any{"methodName": "getValue", "params": []any{"var-modus:1", "VALUE"}}}))
	b := newFake()
	s := New(Options{Backend: b})
	f.Fuzz(func(t *testing.T, data []byte) {
		method, args, err := DecodeCall(data)
		if err != nil || method == "init" {
			return
		}
		res, flt := s.dispatch(method, args, 0)
		if _, err := DecodeResponse(EncodeResponse(res, flt)); err != nil && flt == nil {
			t.Fatalf("an answer that cannot be read: %v", err)
		}
	})
}

// The system's XML-RPC client declares ISO-8859-1, as the interface processes do: such a call is
// read, its Latin-1 bytes as the characters they are. (Found on a real system: lite-rpc's
// forwarded calls were all refused.)
func TestLatin1Calls(t *testing.T) {
	call := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><methodCall><methodName>setValue</methodName><params><param><value>var-text:1</value></param><param><value>VALUE</value></param><param><value><string>T\xfcr \xf6ffnen</string></value></param></params></methodCall>")
	method, args, err := DecodeCall(call)
	if err != nil || method != "setValue" || len(args) != 3 || args[2] != "Tür öffnen" {
		t.Fatalf("%s %v %v", method, args, err)
	}
	for _, enc := range []string{"UTF-8", "utf8", "US-ASCII", "latin1", "windows-1252"} {
		if _, _, err := DecodeCall([]byte(`<?xml version="1.0" encoding="` + enc + `"?><methodCall><methodName>ping</methodName><params></params></methodCall>`)); err != nil {
			t.Errorf("%s: %v", enc, err)
		}
	}
	if _, _, err := DecodeCall([]byte(`<?xml version="1.0" encoding="EBCDIC"?><methodCall><methodName>ping</methodName></methodCall>`)); err == nil {
		t.Error("an encoding nobody reads was taken")
	}
	if _, err := DecodeResponse([]byte("<?xml version=\"1.0\" encoding=\"iso-8859-1\"?><methodResponse><params><param><value>\xe4</value></param></params></methodResponse>")); err != nil {
		t.Errorf("a Latin-1 answer: %v", err)
	}
}

// The subscribers survive a restart of the addon: the interface that starts with the file of
// the run before greets them again and sends them the events - the system does not have to
// notice the restart first (it would, after minutes without an answer to its ping; until then
// every event would be lost).
func TestSubscribersSurviveARestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "subscribers.json")
	c := newClient(t)
	first := start(t, Options{SubscribersFile: file})
	first.ok("init", c.srv.URL, "occulited_Logic")
	c.wait(2) // listDevices, newDevices
	c.take()
	data, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(data), `"id":"occulited_Logic"`) {
		t.Fatalf("the file: %s %v", data, err)
	}
	// the "restart": a second interface on the same file
	second := start(t, Options{SubscribersFile: file})
	c.wait(2)
	calls, _ := c.take()
	if len(calls) != 2 || calls[0] != "listDevices [occulited_Logic]" || !strings.HasPrefix(calls[1], "newDevices occulited_Logic ") {
		t.Fatalf("after the restart: %v", calls)
	}
	if !reflect.DeepEqual(second.s.Subscribers(), []string{c.srv.URL}) || !strings.Contains(second.logged(), "from before the restart") {
		t.Errorf("subscribers %v, log %s", second.s.Subscribers(), second.logged())
	}
	second.ok("setValue", "var-regen:1", "VALUE", true)
	c.wait(1)
	if _, events := c.take(); len(events) != 1 || !strings.Contains(events[0], "var-regen:1 VALUE=true") {
		t.Errorf("events after the restart: %v", events)
	}
	// a subscriber that takes itself out is gone from the file too
	second.ok("init", c.srv.URL, "")
	if data, _ := os.ReadFile(file); string(data) != "[]" {
		t.Errorf("the file after the removal: %s", data)
	}
	// a file that is not a list, one with an entry that is no loopback URL, none at all
	_ = os.WriteFile(file, []byte("{"), 0o600)
	if third := start(t, Options{SubscribersFile: file}); len(third.s.Subscribers()) != 0 || !strings.Contains(third.logged(), "not readable") {
		t.Errorf("a torn file: %v", third.s.Subscribers())
	}
	_ = os.WriteFile(file, []byte(`[{"url":"http://example.org/x","id":"a"},{"url":"`+c.srv.URL+`","id":""}]`), 0o600)
	if fourth := start(t, Options{SubscribersFile: file}); len(fourth.s.Subscribers()) != 0 {
		t.Errorf("entries that cannot be subscribers: %v", fourth.s.Subscribers())
	}
	if fifth := start(t, Options{SubscribersFile: filepath.Join(t.TempDir(), "none.json")}); len(fifth.s.Subscribers()) != 0 {
		t.Error("subscribers without a file")
	}
	// a file that cannot be written is logged, the registration holds
	sixth := start(t, Options{SubscribersFile: filepath.Join(t.TempDir(), "no", "dir", "s.json")})
	sixth.ok("init", c.srv.URL, "x")
	if len(sixth.s.Subscribers()) != 1 || !strings.Contains(sixth.logged(), "could not be written") {
		t.Errorf("an unwritable file: %v %s", sixth.s.Subscribers(), sixth.logged())
	}
}

// The event multicall's wire form (B-13): each call's member methodName comes first and holds the
// name as a value without a type. HMIPServer reads nothing else - with the name typed, or params
// first, it answers HTTP 200 with an empty body - so a subscriber that behaves like it stays
// subscribed past maxFails events. An empty body is no answer and counts as a failure.
func TestMulticallWireForm(t *testing.T) {
	const want = `<value><struct><member><name>methodName</name><value>event</value></member><member><name>params</name><value><array><data>`
	var mu sync.Mutex
	var bodies []string
	strict := true
	got := make(chan struct{}, 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		ok := strict && (!strings.Contains(string(body), "system.multicall") || strings.Contains(string(body), want))
		mu.Unlock()
		if ok {
			_, _ = w.Write(EncodeResponse([]any{}, nil))
		}
		got <- struct{}{}
	}))
	t.Cleanup(srv.Close)
	wait := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			select {
			case <-got:
			case <-time.After(5 * time.Second):
				t.Fatalf("callback %d of %d did not come", i+1, n)
			}
		}
	}
	h := start(t, Options{Client: &http.Client{Timeout: time.Second}})
	h.ok("init", srv.URL, "Logic_java")
	wait(1) // the greeting
	for i := 0; i < maxFails+2; i++ {
		h.ok("ping", "p")
	}
	wait(maxFails + 2)
	if len(h.s.Subscribers()) != 1 {
		t.Fatalf("dropped: %s", h.logged())
	}
	mu.Lock()
	last := bodies[len(bodies)-1]
	mu.Unlock()
	exact := `<?xml version="1.0"?><methodCall><methodName>system.multicall</methodName><params><param><value><array><data>` +
		`<value><struct><member><name>methodName</name><value>event</value></member><member><name>params</name>` +
		`<value><array><data><value><string>Logic_java</string></value><value><string>CENTRAL</string></value><value><string>PONG</string></value><value><string>p</string></value></data></array></value>` +
		`</member></struct></value></data></array></value></param></params></methodCall>`
	if last != exact {
		t.Errorf("the wire form:\n got %s\nwant %s", last, exact)
	}
	// a standard reader takes it as it took the typed form: the name is a string
	method, args, err := DecodeCall([]byte(last))
	if err != nil || method != "system.multicall" {
		t.Fatalf("%q %v", method, err)
	}
	if m := args[0].([]any)[0].(map[string]any); m["methodName"] != "event" || len(m["params"].([]any)) != 4 {
		t.Errorf("decoded %#v", m)
	}
	// an empty body with HTTP 200 is no answer
	mu.Lock()
	strict = false
	mu.Unlock()
	for i := 0; i < maxFails; i++ {
		h.ok("ping", "q")
	}
	wait(maxFails)
	deadline := time.Now().Add(2 * time.Second)
	for len(h.s.Subscribers()) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(h.s.Subscribers()) != 0 || !strings.Contains(h.logged(), "did not answer 10 times, dropped (last: xmlrpc: system.multicall answered with an empty body)") {
		t.Errorf("subscribers %v, log: %s", h.s.Subscribers(), h.logged())
	}
}
