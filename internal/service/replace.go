package service

import (
	"regexp"
	"sort"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// "Gerät ersetzen": after a device was swapped for another one of its kind, everything that
// names the old device - condition lines, activities of every kind, a computed value's inputs,
// the channel a variable is assigned to - names the new one, in one step.
//
// Where a device is named is not listed here kind by kind: a reference is found in the file
// as it is written. In the canonical file a device is always named by a value of its own -
// "Interface/ADDRESS:CHANNEL" with or without "/DATAPOINT" as the whole value of a key or of
// a list's entry - whatever the key is called. So an activity kind added later is covered
// without a change here, as long as it writes its device that way (a test holds every example
// program of the repository to it). Free text - a name, a note, a webhook's address, a value
// that is written or compared - is left alone.

// Device is a device's address: its interface and the address without a channel.
type Device struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
}

func (d Device) String() string { return d.Interface + "/" + d.Address }

// valid says whether the two parts can be a device's.
func (d Device) valid() bool {
	return model.ParseChannel(d.String()+":0") == nil
}

// DeviceUse is a device programs or variables name, with where.
type DeviceUse struct {
	Device
	// Datapoints are what is named of it, "CHANNEL/DATAPOINT" or, where a whole channel is
	// named, "CHANNEL"; sorted.
	Datapoints []string `json:"datapoints"`
	// Programs and Variables are the keys of the objects that name it, sorted.
	Programs  []string `json:"programs"`
	Variables []string `json:"variables"`
}

// reference is a line of a canonical file whose whole value is a device reference: what stands
// before it, the interface, the device's address, the channel, the datapoint with its slash.
var reference = regexp.MustCompile(`^(\s*(?:- )?(?:([A-Za-z_][A-Za-z0-9_]*): )?"?)([A-Za-z][A-Za-z0-9_-]{0,31})/([A-Za-z0-9_.-]{1,64}):([0-9]{1,3})((?:/[A-Za-z0-9_.]{1,64})?)("?)$`)

// freeText are the keys whose value is a text or a value somebody chose - what a variable is
// set to, what a line compares with, a name - and never a device, whatever it looks like.
var freeText = map[string]bool{"name": true, "note": true, "to": true, "value": true, "is": true, "body": true, "script": true, "webhook": true, "command": true}

// parse reads a line of a canonical file as a device reference: what stands before it, the
// device, the channel, the datapoint with its slash, and what stands after it.
func parse(line string) (before string, dev Device, channel, datapoint, after string, ok bool) {
	m := reference.FindStringSubmatch(line)
	if m == nil || freeText[m[2]] {
		return "", Device{}, "", "", "", false
	}
	return m[1], Device{m[3], m[4]}, m[5], m[6], m[7], true
}

// references calls fn for every device reference of a canonical file.
func references(text []byte, fn func(dev Device, datapoint string)) {
	for _, line := range strings.Split(string(text), "\n") {
		if _, dev, channel, datapoint, _, ok := parse(line); ok {
			fn(dev, channel+datapoint)
		}
	}
}

// repoint rewrites every reference to one device into a reference to another; n says how many.
func repoint(text []byte, from, to Device) (out []byte, n int) {
	lines := strings.Split(string(text), "\n")
	for i, line := range lines {
		if before, dev, channel, datapoint, after, ok := parse(line); ok && dev == from {
			lines[i] = before + to.String() + ":" + channel + datapoint + after
			n++
		}
	}
	return []byte(strings.Join(lines, "\n")), n
}

// DeviceUses lists the devices programs and variables name, sorted by interface and address.
func (s *Service) DeviceUses() []DeviceUse {
	s.mu.Lock()
	defer s.mu.Unlock()
	type use struct {
		dps, programs, variables map[string]bool
	}
	uses := map[Device]*use{}
	at := func(d Device) *use {
		if uses[d] == nil {
			uses[d] = &use{map[string]bool{}, map[string]bool{}, map[string]bool{}}
		}
		return uses[d]
	}
	for key, p := range s.set.Programs {
		references(model.EncodeProgram(p, nil), func(d Device, dp string) {
			u := at(d)
			u.dps[dp], u.programs[key] = true, true
		})
	}
	for key, v := range s.set.Variables {
		references(model.EncodeVariable(v), func(d Device, dp string) {
			u := at(d)
			u.dps[dp], u.variables[key] = true, true
		})
	}
	out := make([]DeviceUse, 0, len(uses))
	for d, u := range uses {
		out = append(out, DeviceUse{Device: d, Datapoints: sortedSet(u.dps), Programs: sortedSet(u.programs), Variables: sortedSet(u.variables)})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Device.String() < out[b].Device.String() })
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Replaced is what a replacement changed.
type Replaced struct {
	// Programs and Variables are the keys of the objects that were changed, sorted;
	// References is the number of places.
	Programs   []string `json:"programs"`
	Variables  []string `json:"variables"`
	References int      `json:"references"`
}

// ReplaceDevice makes every reference to one device a reference to another - the same channel
// and datapoint of it. Whether the other device has those is the caller's to check (it knows
// the system); here every changed file is read back and checked like a save, all of them
// before the first one is written, so that nothing is replaced by halves. extra are other
// files of the tree to write with it (the import's record of devices, without the old one).
// With dry set nothing is written: the answer says what would change. Nothing runs.
func (s *Service) ReplaceDevice(from, to Device, extra map[string][]byte, author repo.Author, dry bool) (*Replaced, error) {
	if !from.valid() || !to.valid() {
		return nil, refuse("invalid", "a device is named by its interface and its address")
	}
	if from == to {
		return nil, refuse("invalid", "the two devices are the same one")
	}
	for path := range extra {
		if err := store.CheckExtraPath(path); err != nil {
			return nil, refuse("invalid", "%v", err)
		}
	}
	s.mu.Lock()
	res := &Replaced{Programs: []string{}, Variables: []string{}}
	programs := map[string]*model.Program{}
	variables := map[string]*model.Variable{}
	fail := func(err error) (*Replaced, error) {
		s.mu.Unlock()
		return nil, err
	}
	for _, key := range sortedPrograms(s.set.Programs) {
		text, n := repoint(model.EncodeProgram(s.set.Programs[key], nil), from, to)
		if n == 0 {
			continue
		}
		def, err := model.DecodeProgram(store.ProgramPath(key), key, text)
		if err != nil {
			return fail(refuse("invalid", "program %s cannot name %s: %s", s.set.Programs[key].Name, to, plainError(err)))
		}
		programs[key] = def
		res.Programs = append(res.Programs, key)
		res.References += n
	}
	for _, key := range sortedVariables(s.set.Variables) {
		text, n := repoint(model.EncodeVariable(s.set.Variables[key]), from, to)
		if n == 0 {
			continue
		}
		def, err := model.DecodeVariable(store.VariablePath(key), key, text)
		if err != nil {
			return fail(refuse("invalid", "variable %s cannot name %s: %s", s.set.Variables[key].Name, to, plainError(err)))
		}
		variables[key] = def
		res.Variables = append(res.Variables, key)
		res.References += n
	}
	if res.References == 0 {
		return fail(refuse("not-found", "nothing names the device %s", from))
	}
	// the whole as it would be must hold together as well as it does now
	with := &model.Set{Programs: map[string]*model.Program{}, Variables: map[string]*model.Variable{}}
	for k, p := range s.set.Programs {
		with.Programs[k] = p
	}
	for k, v := range s.set.Variables {
		with.Variables[k] = v
	}
	for k, p := range programs {
		with.Programs[k] = p
	}
	for k, v := range variables {
		with.Variables[k] = v
	}
	before, after := s.set.Problems(), with.Problems()
	for _, k := range sortedPrograms(programs) {
		if strings.Join(before[k], "\n") != strings.Join(after[k], "\n") && len(after[k]) > 0 {
			return fail(refuse("invalid", "program %s: %s", programs[k].Name, strings.Join(after[k], "; ")))
		}
	}
	for _, k := range sortedVariables(variables) {
		if problem := with.ComputedProblem(variables[k]); problem != "" && s.set.ComputedProblem(s.set.Variables[k]) == "" {
			return fail(refuse("invalid", "variable %s: %s", variables[k].Name, problem))
		}
	}
	if dry {
		s.mu.Unlock()
		return res, nil
	}
	paths := []string{}
	for _, k := range res.Programs {
		path, _, err := s.tree.SaveProgram(programs[k], s.o.Names)
		if err != nil {
			return fail(err)
		}
		paths = append(paths, path)
		s.set.Programs[k] = programs[k]
	}
	for _, k := range res.Variables {
		path, _, err := s.tree.SaveVariable(variables[k])
		if err != nil {
			return fail(err)
		}
		paths = append(paths, path)
		s.set.Variables[k] = variables[k]
	}
	for _, path := range sortedFiles(extra) {
		changed, err := s.tree.WriteExtra(path, extra[path])
		if err != nil {
			return fail(err)
		}
		if changed {
			paths = append(paths, path)
		}
	}
	s.eng.SetVariables(s.set.Variables)
	s.programsChanged()
	s.mu.Unlock()
	s.queue.Enqueue(paths, author, "Device "+from.String()+" replaced by "+to.String())
	s.changed()
	s.logf("device %s replaced by %s: %d references in %d programs and %d variables", from, to, res.References, len(res.Programs), len(res.Variables))
	return res, nil
}
