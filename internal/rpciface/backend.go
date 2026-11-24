package rpciface

import (
	"errors"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// Author is who the history names for a change made through the interface.
var Author = repo.Author{Name: "Logic interface"}

// systemNames are the system values' device names: what the CCU called them.
var systemNames = map[string]string{
	model.SystemAlarmMessages:   "Alarmmeldungen",
	model.SystemServiceMessages: "Servicemeldungen",
	model.SystemDutyCycle:       "DutyCycle",
	model.SystemSunAzimuth:      "Sonnenazimut",
	model.SystemSunElevation:    "Sonnenhöhe",
}

// ServiceBackend serves the addon's service through the interface: a write from outside enters
// the same door as one on the page.
type ServiceBackend struct {
	Svc *service.Service
}

// Variables are the variables and, as read-only numbers, the system values.
func (b ServiceBackend) Variables() []engine.VariableInfo {
	eng := b.Svc.Engine()
	out := eng.Variables()
	for _, sv := range eng.SystemValues() {
		out = append(out, engine.VariableInfo{Def: &model.Variable{Key: sv.Key, Name: systemNames[sv.Key], Type: model.VarNumber, Visible: true}, Value: sv.Value})
	}
	return out
}

// Programs are the programs with their switch and the block that ran last.
func (b ServiceBackend) Programs() []Program {
	eng := b.Svc.Engine()
	var out []Program
	for _, p := range b.Svc.Programs() {
		out = append(out, Program{Key: p.Program.Key, Name: p.Program.Name, Active: p.Program.Active, Operable: p.Program.Operable, Visible: p.Program.Visible, Internal: p.Program.Internal, LastRun: eng.LastRun(p.Program.Key)})
	}
	return out
}

// SetVariable is a write from outside: the limits hold (OQ-23).
func (b ServiceBackend) SetVariable(key string, value any) error {
	return b.Svc.Engine().SetVariable(key, value, engine.FromRPC)
}

// Acknowledge acknowledges an alarm.
func (b ServiceBackend) Acknowledge(key string) error { return b.Svc.Engine().Acknowledge(key) }

// SetActive switches a program; the change is a file and a history entry like any other.
func (b ServiceBackend) SetActive(key string, active bool) error {
	_, err := b.Svc.SetActive(key, active, Author)
	var se *service.Error
	if errors.As(err, &se) && se.Code == "not-found" {
		return &engine.WriteError{Code: "unknown-variable", Msg: se.Msg}
	}
	return err
}

// Run runs a program: with its conditions, or its first DANN without them.
func (b ServiceBackend) Run(key string, then bool) error {
	mode := model.RunCheck
	if then {
		mode = model.RunThen
	}
	return b.Svc.Engine().RunProgram(key, mode)
}
