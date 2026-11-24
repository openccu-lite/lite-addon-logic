package rpciface

import (
	"errors"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// The interface on the real service: devices of what the service holds, a write from outside
// with the limits of OQ-23, an alarm acknowledged, a program switched, the events on the way.
func TestServiceBackend(t *testing.T) {
	svc, err := service.Open(service.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	who := repo.Author{Name: "Test"}
	min, max := 0.0, 100.0
	for _, v := range []*model.Variable{
		{Key: "zahl", Name: "Zahl", Type: model.VarNumber, Min: &min, Max: &max, Visible: true},
		{Key: "alarm", Name: "Alarm", Type: model.VarAlarm, Visible: true},
	} {
		if _, err := svc.CreateVariable(v, nil, who); err != nil {
			t.Fatal(err)
		}
	}
	prg := &model.Program{Key: "licht", Name: "Licht", Visible: true, Operable: true, Rules: []model.Rule{
		{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "zahl", Op: model.OpGt, Value: 5.0, Trigger: model.TriggerChange}}}}},
			Then: []model.Action{{Kind: model.ActionVariable, Variable: "alarm", To: true}}}}}
	if _, err := svc.CreateProgram(prg, who); err != nil {
		t.Fatal(err)
	}
	h := start(t, Options{Backend: ServiceBackend{Svc: svc}})
	svc.Subscribe(h.s.Notify)
	svc.OnChange(h.s.Changed)
	c := newClient(t)
	c.knows = []any{}
	h.ok("init", c.srv.URL, "x")
	c.wait(2)
	calls, _ := c.take()
	if len(calls) != 2 || calls[1] != "newDevices x prg-licht,prg-licht:0,prg-licht:1,var-alarm,var-alarm:0,var-alarm:1,var-alarm-messages,var-alarm-messages:0,var-alarm-messages:1,"+
		"var-duty-cycle,var-duty-cycle:0,var-duty-cycle:1,var-service-messages,var-service-messages:0,var-service-messages:1,var-sun-azimuth,var-sun-azimuth:0,var-sun-azimuth:1,var-sun-elevation,var-sun-elevation:0,var-sun-elevation:1,var-zahl,var-zahl:0,var-zahl:1" {
		t.Fatalf("%v", calls)
	}
	// the program's and the variables' flags reach the value channel (visible; a system value is visible too)
	for _, a := range []string{"prg-licht:1", "var-zahl:1", "var-duty-cycle:1"} {
		if d := h.ok("getDeviceDescription", a).(map[string]any); d["FLAGS"] != 1 {
			t.Errorf("FLAGS of %s: %v", a, d["FLAGS"])
		}
	}
	// a write inside the limits is taken and comes back as an event; outside them it is refused
	h.ok("setValue", "var-zahl:1", "VALUE", 7.5)
	c.wait(1)
	if _, ev := c.take(); len(ev) == 0 || ev[0] != "x var-zahl:1 VALUE=7.5" {
		t.Errorf("events: %v", ev)
	}
	if got := h.ok("getValue", "var-zahl:1", "VALUE"); got != 7.5 {
		t.Errorf("%v", got)
	}
	h.fault(FaultUnknownParam, "setValue", "var-zahl:1", "VALUE", 150.0)
	h.fault(FaultNotSupported, "setValue", "var-duty-cycle:1", "VALUE", 5.0)
	if got := h.ok("getValue", "var-alarm-messages:1", "VALUE"); got != 0.0 {
		t.Errorf("alarm messages: %v", got)
	}
	// an alarm set from outside counts, and the system value follows; ACKNOWLEDGE ends it
	h.ok("setValue", "var-alarm:1", "VALUE", true)
	if got := h.ok("getValue", "var-alarm:1", "COUNT"); got != 1 {
		t.Errorf("count: %v", got)
	}
	if got := h.ok("getValue", "var-alarm-messages:1", "VALUE"); got != 1.0 {
		t.Errorf("alarm messages: %v", got)
	}
	h.ok("setValue", "var-alarm:1", "ACKNOWLEDGE", true)
	if got := h.ok("getValue", "var-alarm:1", "VALUE"); got != false {
		t.Errorf("acknowledged: %v", got)
	}
	// a program switched from outside is switched in the service
	if got := h.ok("getValue", "prg-licht:1", "ACTIVE"); got != false {
		t.Errorf("active: %v", got)
	}
	h.ok("setValue", "prg-licht:1", "ACTIVE", true)
	if p, _ := svc.Program("licht"); !p.Program.Active {
		t.Error("the program was not switched")
	}
	if got := h.ok("getValue", "prg-licht:1", "LAST_RUN"); got != 0 {
		t.Errorf("last run: %v", got)
	}
	// RUN evaluates the program: zahl is 7.5, above 5, so the first block runs and sets the alarm;
	// the run is an event, and LAST_RUN says which block
	c.drain()
	h.ok("setValue", "prg-licht:1", "RUN", true)
	c.until("x prg-licht:1 LAST_RUN=1", "x var-alarm:1 VALUE=true")
	if got := h.ok("getValue", "prg-licht:1", "LAST_RUN"); got != 1 {
		t.Errorf("last run: %v", got)
	}
	if v, _ := svc.Engine().Value("alarm"); v != true {
		t.Errorf("the run did not act: %v", v)
	}
	// RUN_THEN runs the first block without the conditions, also below the threshold
	h.ok("setValue", "var-alarm:1", "ACKNOWLEDGE", true)
	h.ok("setValue", "var-zahl:1", "VALUE", 1.0)
	h.ok("setValue", "prg-licht:1", "RUN_THEN", true)
	if v, _ := svc.Engine().Value("alarm"); v != true {
		t.Errorf("RUN_THEN did not act: %v", v)
	}
	h.fault(FaultUnknownDevice, "setValue", "prg-nichts:1", "RUN", true)
	// a new variable arrives as a device
	c.drain()
	if _, err := svc.CreateVariable(&model.Variable{Key: "neu", Name: "Neu", Type: model.VarString, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	c.until("newDevices x var-neu,var-neu:0,var-neu:1")
	// what is gone
	b := ServiceBackend{Svc: svc}
	var we *engine.WriteError
	if err := b.SetActive("nichts", true); !errors.As(err, &we) || we.Code != "unknown-variable" {
		t.Errorf("%v", err)
	}
	if err := b.Run("nichts", true); !errors.As(err, &we) || we.Code != "unknown-program" {
		t.Errorf("%v", err)
	}
}

// A computed value is read-only on the interface: its VALUE cannot be written from outside.
func TestComputedIsReadOnlyOutside(t *testing.T) {
	svc, err := service.Open(service.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	who := repo.Author{Name: "Test"}
	for _, v := range []*model.Variable{
		{Name: "A", Type: model.VarNumber, Visible: true},
		{Name: "Doppelt", Type: model.VarNumber, Visible: true, Computed: &model.Computed{Function: model.FnSum, Inputs: []model.Input{{Variable: "a"}, {Variable: "a"}}}},
	} {
		if _, err := svc.CreateVariable(v, nil, who); err != nil {
			t.Fatal(err)
		}
	}
	h := start(t, Options{Backend: ServiceBackend{Svc: svc}})
	h.ok("setValue", "var-a:1", "VALUE", 2.5)
	if got := h.ok("getValue", "var-doppelt:1", "VALUE"); got != 5.0 {
		t.Errorf("the computed value: %v", got)
	}
	h.fault(FaultNotSupported, "setValue", "var-doppelt:1", "VALUE", 1.0)
	desc, _ := h.ok("getParamsetDescription", "var-doppelt:1", "VALUES").(map[string]any)
	if value, _ := desc["VALUE"].(map[string]any); value["OPERATIONS"] != 5 {
		t.Errorf("operations: %v", value["OPERATIONS"])
	}
}
