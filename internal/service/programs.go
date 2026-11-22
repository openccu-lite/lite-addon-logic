package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// ProgramInfo is a program with what is wrong with it.
type ProgramInfo struct {
	Program *model.Program `json:"program"`
	// Problems are broken references (a variable or program that is gone, a value that does not
	// fit); a program with problems does not run.
	Problems []string `json:"problems,omitempty"`
}

// Programs lists the programs, sorted by name.
func (s *Service) Programs() []ProgramInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProgramInfo, 0, len(s.set.Programs))
	for _, p := range s.set.Programs {
		out = append(out, ProgramInfo{Program: p.Clone(), Problems: s.set.ProgramProblems(p)})
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Program.Name), strings.ToLower(out[j].Program.Name); a != b {
			return a < b
		}
		return out[i].Program.Key < out[j].Program.Key
	})
	return out
}

// Program answers one program.
func (s *Service) Program(key string) (ProgramInfo, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.set.Programs[key]
	if p == nil {
		return ProgramInfo{}, false
	}
	return ProgramInfo{Program: p.Clone(), Problems: s.set.ProgramProblems(p)}, true
}

// ProgramUsers lists the programs whose activities run or switch a program.
func (s *Service) ProgramUsers(key string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.set.ProgramUsers(key)
}

// checkProgram validates a program and its references against the set (with the lock held). An
// active program must be sound: one with a broken reference or a kept script cannot be active.
func (s *Service) checkProgram(p *model.Program) (*model.Program, error) {
	checked, err := p.Check()
	if err != nil {
		return nil, &Error{Code: "invalid", Msg: plainError(err)}
	}
	if checked.Active {
		with := &model.Set{Programs: map[string]*model.Program{}, Variables: s.set.Variables}
		for k, q := range s.set.Programs {
			with.Programs[k] = q
		}
		with.Programs[checked.Key] = checked
		if pr := with.ProgramProblems(checked); len(pr) > 0 {
			return nil, &Error{Code: "invalid", Msg: "the program cannot be active: " + strings.Join(pr, "; ")}
		}
	}
	return checked, nil
}

// CreateProgram makes a new program; its key is made from the name.
func (s *Service) CreateProgram(def *model.Program, author repo.Author) (*model.Program, error) {
	s.mu.Lock()
	p := def.Clone()
	p.Key = model.UniqueKey(p.Name, func(k string) bool { return s.set.Programs[k] != nil })
	checked, err := s.checkProgram(p)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	path, _, err := s.tree.SaveProgram(checked, s.o.Names)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.set.Programs[checked.Key] = checked
	s.programsChanged()
	s.mu.Unlock()
	s.queue.Enqueue([]string{path}, author, "Program "+checked.Name+" created")
	s.changed()
	return checked.Clone(), nil
}

// UpdateProgram replaces a program. Saving never runs anything.
func (s *Service) UpdateProgram(key string, def *model.Program, author repo.Author) (*model.Program, error) {
	return s.updateProgram(key, def, author, "changed")
}

// updateProgram is UpdateProgram with what the history says happened to the program.
func (s *Service) updateProgram(key string, def *model.Program, author repo.Author, what string) (*model.Program, error) {
	s.mu.Lock()
	if s.set.Programs[key] == nil {
		s.mu.Unlock()
		return nil, refuse("not-found", "there is no program %s", key)
	}
	p := def.Clone()
	p.Key = key
	checked, err := s.checkProgram(p)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	path, _, err := s.tree.SaveProgram(checked, s.o.Names)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.set.Programs[key] = checked
	s.programsChanged()
	s.mu.Unlock()
	s.queue.Enqueue([]string{path}, author, "Program "+checked.Name+" "+what)
	s.changed()
	return checked.Clone(), nil
}

// SetActive switches a program on or off. Switching on never runs it.
func (s *Service) SetActive(key string, active bool, author repo.Author) (*model.Program, error) {
	s.mu.Lock()
	old := s.set.Programs[key]
	if old == nil {
		s.mu.Unlock()
		return nil, refuse("not-found", "there is no program %s", key)
	}
	p := old.Clone()
	p.Active = active
	s.mu.Unlock()
	if old.Active == active {
		return p, nil
	}
	return s.UpdateProgram(key, p, author)
}

// SetAtStart switches "beim Systemstart prüfen" of a program: whether it is evaluated once
// after a boot of the system.
func (s *Service) SetAtStart(key string, atStart bool, author repo.Author) (*model.Program, error) {
	s.mu.Lock()
	old := s.set.Programs[key]
	if old == nil {
		s.mu.Unlock()
		return nil, refuse("not-found", "there is no program %s", key)
	}
	p := old.Clone()
	p.AtStart = atStart
	s.mu.Unlock()
	if old.AtStart == atStart {
		return p, nil
	}
	return s.UpdateProgram(key, p, author)
}

// DeleteProgram removes a program; one that other programs run or switch cannot be removed.
func (s *Service) DeleteProgram(key string, author repo.Author) error {
	s.mu.Lock()
	old := s.set.Programs[key]
	if old == nil {
		s.mu.Unlock()
		return refuse("not-found", "there is no program %s", key)
	}
	if users := s.set.ProgramUsers(key); len(users) > 0 {
		s.mu.Unlock()
		return &Error{Code: "in-use", Msg: fmt.Sprintf("%s cannot be removed while other programs run or switch it", old.Name), Users: users}
	}
	path, err := s.tree.DeleteProgram(key)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	delete(s.set.Programs, key)
	s.programsChanged()
	s.mu.Unlock()
	s.queue.Enqueue([]string{path}, author, "Program "+old.Name+" removed")
	s.changed()
	return nil
}

// Import is what an import brings: programs and variables under the keys the importer chose
// (an existing key is replaced - the importer's ledger decides what is the same object), and
// the variables' values.
type Import struct {
	Programs  []*model.Program
	Variables []*model.Variable
	// Values are the variables' values by key; they are set without an event.
	Values map[string]any
	// Helpers is what the device helpers among the variables remember, by their leading
	// variable's key: the CCU's last counted reading.
	Helpers map[string]model.HelperState
	// Files are other files of the tree the import keeps (its ledger, the devices it saw), by
	// their path in the tree; they are written with the rest and belong to the same history
	// entry. Not below programs/ or variables/.
	Files map[string][]byte
	// Location is where the CCU stood (nil: the source did not say). It becomes the addon's
	// location when none is set yet - a location the user set is never replaced.
	Location *model.Location
	// Message is the history entry's text.
	Message string
}

// ImportResult says what an import did.
type ImportResult struct {
	Programs  int `json:"programs"`
	Variables int `json:"variables"`
	// Deactivated are programs that came active and were stored inactive because a reference
	// of theirs does not hold (key: the reasons).
	Deactivated map[string][]string `json:"deactivated,omitempty"`
	// Values are values that did not fit their variable and were left out (key: the reason).
	Values map[string]string `json:"values,omitempty"`
	// Location says the import's location became the addon's.
	Location bool `json:"location,omitempty"`
}

// ApplyImport stores an import as one step and one history entry. A variable under a system
// value's key (alarm-messages, service-messages, duty-cycle) is left out: those exist anyway. Every object is validated
// first; if one is refused, nothing is written. A program whose references do not hold is
// stored inactive, never refused: an import must not lose a program.
func (s *Service) ApplyImport(in Import, author repo.Author) (*ImportResult, error) {
	s.mu.Lock()
	next := &model.Set{Programs: map[string]*model.Program{}, Variables: map[string]*model.Variable{}}
	for k, v := range s.set.Variables {
		next.Variables[k] = v
	}
	for k, p := range s.set.Programs {
		next.Programs[k] = p
	}
	vars := make([]*model.Variable, 0, len(in.Variables))
	for _, v := range in.Variables {
		checked, err := v.Check()
		if err != nil {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: err.Error()}
		}
		if model.IsSystemValue(checked.Key) {
			// a system value exists without a variable: an importer that brings one for the
			// CCU's built-in counters is not refused, the variable is just not needed
			continue
		}
		vars = append(vars, checked)
		next.Variables[checked.Key] = checked
	}
	progs := make([]*model.Program, 0, len(in.Programs))
	for _, p := range in.Programs {
		checked, err := p.Check()
		if err != nil {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: err.Error()}
		}
		progs = append(progs, checked)
		next.Programs[checked.Key] = checked
	}
	for path := range in.Files {
		if err := store.CheckExtraPath(path); err != nil {
			s.mu.Unlock()
			return nil, &Error{Code: "invalid", Msg: err.Error()}
		}
	}
	res := &ImportResult{Programs: len(progs), Variables: len(vars)}
	for _, p := range progs {
		if !p.Active {
			continue
		}
		if pr := next.ProgramProblems(p); len(pr) > 0 {
			p.Active = false
			if res.Deactivated == nil {
				res.Deactivated = map[string][]string{}
			}
			res.Deactivated[p.Key] = pr
		}
	}
	var paths []string
	for _, v := range vars {
		path, _, err := s.tree.SaveVariable(v)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		paths = append(paths, path)
	}
	for _, p := range progs {
		path, _, err := s.tree.SaveProgram(p, s.o.Names)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		paths = append(paths, path)
	}
	for _, path := range sortedFiles(in.Files) {
		if _, err := s.tree.WriteExtra(path, in.Files[path]); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		paths = append(paths, path)
	}
	if in.Location != nil && s.settings.Location == nil {
		st := model.Settings{Location: in.Location}
		if st.Check() == nil {
			if _, err := s.tree.WriteExtra(model.SettingsPath, model.EncodeSettings(&st)); err != nil {
				s.mu.Unlock()
				return nil, err
			}
			paths = append(paths, model.SettingsPath)
			s.settings, res.Location = &st, true
			s.eng.SetPosition(position(&st))
		}
	}
	s.set = next
	s.eng.SetVariables(s.set.Variables)
	for _, k := range sortedValues(in.Values) {
		if err := s.eng.SetVariable(k, in.Values[k], engine.FromImport); err != nil {
			if res.Values == nil {
				res.Values = map[string]string{}
			}
			res.Values[k] = err.Error()
		}
	}
	for k, st := range in.Helpers {
		s.eng.SetHelperState(k, st)
	}
	s.programsChanged()
	s.mu.Unlock()
	msg := in.Message
	if msg == "" {
		msg = fmt.Sprintf("Import: %d programs, %d variables", len(progs), len(vars))
	}
	if len(paths) > 0 {
		s.queue.Enqueue(paths, author, msg)
	}
	s.changed()
	return res, nil
}

func sortedFiles(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedValues(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
