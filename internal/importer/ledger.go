package importer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Ledger remembers, per source, which object of the CCU became which file, so a second import of
// the same source updates the same programs and variables instead of adding them again. The
// CCU's object ids stay here and never enter a program file.
type Ledger struct {
	Source   string
	Serial   string
	Firmware string
	// Programs and Variables: the CCU's object id to the key.
	Programs  map[int]string
	Variables map[int]string
}

// ValidSource says whether a name can be a ledger's: a key, as a program's.
func ValidSource(s string) bool { return model.ValidKey(s) }

// Encode writes the ledger as imports/<source>.yaml has it: sorted by id, the same bytes for the
// same ledger.
func (l *Ledger) Encode() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "source: %s\n", l.Source)
	if l.Serial != "" {
		fmt.Fprintf(&b, "serial: %s\n", strconv.Quote(l.Serial))
	}
	if l.Firmware != "" {
		fmt.Fprintf(&b, "firmware: %s\n", strconv.Quote(l.Firmware))
	}
	for _, part := range []struct {
		name string
		m    map[int]string
	}{{"programs", l.Programs}, {"variables", l.Variables}} {
		if len(part.m) == 0 {
			fmt.Fprintf(&b, "%s: {}\n", part.name)
			continue
		}
		fmt.Fprintf(&b, "%s:\n", part.name)
		ids := make([]int, 0, len(part.m))
		for id := range part.m {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			fmt.Fprintf(&b, "  %d: %s\n", id, part.m[id])
		}
	}
	return []byte(b.String())
}

// DecodeLedger reads a ledger file.
func DecodeLedger(data []byte) (*Ledger, error) {
	var raw struct {
		Source    string         `yaml:"source"`
		Serial    string         `yaml:"serial"`
		Firmware  string         `yaml:"firmware"`
		Programs  map[int]string `yaml:"programs"`
		Variables map[int]string `yaml:"variables"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("import ledger: %w", err)
	}
	if !ValidSource(raw.Source) {
		return nil, fmt.Errorf("import ledger: %q is not a source's name", raw.Source)
	}
	l := &Ledger{Source: raw.Source, Serial: raw.Serial, Firmware: raw.Firmware, Programs: raw.Programs, Variables: raw.Variables}
	if l.Programs == nil {
		l.Programs = map[int]string{}
	}
	if l.Variables == nil {
		l.Variables = map[int]string{}
	}
	for _, m := range []map[int]string{l.Programs, l.Variables} {
		for id, key := range m {
			if !model.ValidKey(key) {
				return nil, fmt.Errorf("import ledger: object %d has the key %q, which is none", id, key)
			}
		}
	}
	return l, nil
}

// Merge takes over what an earlier ledger of the source knew and this import did not see again
// (a program removed on the CCU keeps its entry, so its file is still this source's).
func (l *Ledger) Merge(prev *Ledger) {
	if prev == nil {
		return
	}
	for id, k := range prev.Programs {
		if _, ok := l.Programs[id]; !ok {
			l.Programs[id] = k
		}
	}
	for id, k := range prev.Variables {
		if _, ok := l.Variables[id]; !ok {
			l.Variables[id] = k
		}
	}
}

// SeenDevice is what the database knew about a channel programs use: enough to show a reference
// by its old name while the device is not on this system.
type SeenDevice struct {
	Channel     string   `yaml:"channel" json:"channel"` // Interface/ADDRESS:CH
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Device      string   `yaml:"device,omitempty" json:"device,omitempty"`             // the device's name
	Type        string   `yaml:"type,omitempty" json:"type,omitempty"`                 // the device type
	ChannelType string   `yaml:"channel_type,omitempty" json:"channel_type,omitempty"` // the channel type
	Rooms       []string `yaml:"rooms,omitempty" json:"rooms,omitempty"`
	Params      []string `yaml:"params,omitempty" json:"params,omitempty"` // the datapoints programs use
}

// EncodeSeen writes devices-seen.yaml: sorted by channel.
func EncodeSeen(list []SeenDevice) []byte {
	sorted := append([]SeenDevice(nil), list...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Channel < sorted[j].Channel })
	if len(sorted) == 0 {
		return []byte("devices: []\n")
	}
	out, _ := yaml.Marshal(struct {
		Devices []SeenDevice `yaml:"devices"`
	}{sorted})
	return out
}

// DecodeSeen reads devices-seen.yaml.
func DecodeSeen(data []byte) ([]SeenDevice, error) {
	var raw struct {
		Devices []SeenDevice `yaml:"devices"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("devices-seen: %w", err)
	}
	for _, d := range raw.Devices {
		if err := model.ParseChannel(d.Channel); err != nil {
			return nil, fmt.Errorf("devices-seen: %w", err)
		}
	}
	return raw.Devices, nil
}

// MergeSeen adds an import's devices to the ones already noted; a channel seen again is replaced.
func MergeSeen(have, add []SeenDevice) []SeenDevice {
	by := map[string]SeenDevice{}
	for _, d := range have {
		by[d.Channel] = d
	}
	for _, d := range add {
		by[d.Channel] = d
	}
	out := make([]SeenDevice, 0, len(by))
	for _, d := range by {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Channel < out[j].Channel })
	return out
}
