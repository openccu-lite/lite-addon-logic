package service

import (
	"fmt"
	"sort"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
)

// HelperInput is a device helper to make: the kind, the channel, and its variables by role
// with their names. Unit is the counter's unit as the device describes it (the kind's own
// when empty).
type HelperInput struct {
	Kind    string            `json:"kind"`
	Channel string            `json:"channel"`
	Names   map[string]string `json:"names"`
	Unit    string            `json:"unit,omitempty"`
}

// helperOf answers the variables of the helper with this kind and channel, by key (with the
// lock held).
func (s *Service) helperOf(kind, channel string) map[string]*model.Variable {
	out := map[string]*model.Variable{}
	for k, v := range s.set.Variables {
		if v.Helper != nil && v.Helper.Kind == kind && v.Helper.Channel == channel {
			out[k] = v
		}
	}
	return out
}

// CreateHelper makes a device helper: one number variable per role that is named, in one step
// and one history entry. The helper has not seen its device: it counts from the device's
// reading at its next report (or from the value the system has for it now). Refused: a kind
// or a role there is none of, no variable for the kind's leading role, a channel that already
// has a helper of the kind.
func (s *Service) CreateHelper(in HelperInput, author repo.Author) ([]*model.Variable, error) {
	kind := model.HelperKindOf(in.Kind)
	if kind == nil {
		return nil, refuse("invalid", "there is no device helper of the kind %q", in.Kind)
	}
	if err := model.ParseChannel(in.Channel); err != nil {
		return nil, &Error{Code: "invalid", Msg: err.Error()}
	}
	for role := range in.Names {
		if !kind.HasRole(role) {
			return nil, refuse("invalid", "a helper of the kind %s has no %q", kind.Kind, role)
		}
	}
	if in.Names[kind.Lead()] == "" {
		return nil, refuse("invalid", "a helper of the kind %s needs a variable for its %s", kind.Kind, kind.Lead())
	}
	unit := in.Unit
	if unit == "" {
		unit = kind.Unit
	}
	s.mu.Lock()
	if len(s.helperOf(kind.Kind, in.Channel)) > 0 {
		s.mu.Unlock()
		return nil, refuse("exists", "%s already has a helper of the kind %s", in.Channel, kind.Kind)
	}
	taken := map[string]bool{}
	var made []*model.Variable
	for _, role := range kind.Roles {
		name := in.Names[role]
		if name == "" {
			continue
		}
		v := &model.Variable{Name: name, Type: model.VarNumber, Unit: unit, Visible: true, Helper: &model.Helper{Kind: kind.Kind, Channel: in.Channel, Role: role}}
		v.Key = model.UniqueKey(name, func(k string) bool { return taken[k] || s.set.Variables[k] != nil || model.IsSystemValue(k) })
		taken[v.Key] = true
		checked, err := v.Check()
		if err != nil {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: plainError(err)}
		}
		made = append(made, checked)
	}
	var paths []string
	for _, v := range made {
		path, _, err := s.tree.SaveVariable(v)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		paths = append(paths, path)
		s.set.Variables[v.Key] = v
	}
	s.eng.SetVariables(s.set.Variables)
	s.mu.Unlock()
	s.queue.Enqueue(paths, author, fmt.Sprintf("Device helper %s created", made[0].Name))
	s.changed()
	out := make([]*model.Variable, len(made))
	for i, v := range made {
		out[i] = v.Clone()
	}
	return out, nil
}

// DeleteHelper removes a device helper with all its variables, in one step and one history
// entry; key is any of its variables. Refused while a program or a computed value uses one of
// them.
func (s *Service) DeleteHelper(key string, author repo.Author) error {
	s.mu.Lock()
	v := s.set.Variables[key]
	if v == nil || v.Helper == nil {
		s.mu.Unlock()
		return refuse("not-found", "there is no device helper with a variable %s", key)
	}
	members := s.helperOf(v.Helper.Kind, v.Helper.Channel)
	keys := make([]string, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	name := v.Name
	for _, k := range keys {
		if kind := model.HelperKindOf(v.Helper.Kind); members[k].Helper.Role == kind.Lead() {
			name = members[k].Name
		}
		users := append(s.set.VariableUsers(k), s.set.ComputedUsers(k)...)
		if len(users) > 0 {
			s.mu.Unlock()
			return &Error{Code: "in-use", Msg: fmt.Sprintf("%s cannot be removed while programs or computed values use it", members[k].Name), Users: users}
		}
	}
	var paths []string
	for _, k := range keys {
		path, err := s.tree.DeleteVariable(k)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		paths = append(paths, path)
		delete(s.set.Variables, k)
	}
	s.eng.SetVariables(s.set.Variables)
	s.mu.Unlock()
	s.queue.Enqueue(paths, author, fmt.Sprintf("Device helper %s removed", name))
	s.changed()
	return nil
}
