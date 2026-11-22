package service

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/docs"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

const everyKind = `name: Alles
rules:
  - when:
      any:
        - all:
            - device: HmIP-RF/000A1B2C3D4E03:3/STATE
              is: true
              trigger: change
            - device: HmIP-RF/000A1B2C3D4E03:0/LOW_BAT
              is: false
              trigger: check
            - device: HmIP-RF/000AAAAAAAAAAA:1/STATE
              is: 1
              trigger: update
    then:
      - set: HmIP-RF/000A1B2C3D4E03:3/STATE
        to: true
        on_for: 5min
      - set: BidCos-RF/NEQ0000001:1/STATE
        from: HmIP-RF/000A1B2C3D4E03:2/STATE
      - channel: HmIP-RF/000A1B2C3D4E03:3
        values:
          STATE: true
      - webhook: "http://127.0.0.1/HmIP-RF/000A1B2C3D4E03:3/STATE"
        method: GET
  - else:
      - variable: text
        to: HmIP-RF/000A1B2C3D4E03:3/STATE
`

func decode(t *testing.T, key, text string) *model.Program {
	t.Helper()
	p, err := model.DecodeProgram(key+".yaml", key, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// After a device was swapped, everything that names the old one names the new one: lines,
// activities of every kind, a computed value's inputs, a variable's channel - and nothing else
// changes: a webhook's address and a text that happen to hold the address stay.
func TestReplaceDevice(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	defer s.Close()
	old, neu := Device{"HmIP-RF", "000A1B2C3D4E03"}, Device{"HmIP-RF", "000F00000000FF"}
	if _, err := s.CreateVariable(&model.Variable{Name: "Text", Type: model.VarString, Visible: true}, nil, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProgram(decode(t, "alles", everyKind), admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProgram(decode(t, "andere", "name: Andere\nrules:\n  - when:\n      all: []\n    then:\n      - set: HmIP-RF/000AAAAAAAAAAA:1/STATE\n        to: true\n"), admin); err != nil {
		t.Fatal(err)
	}
	mean, err := model.DecodeVariable("mittel.yaml", "mittel", []byte("name: Mittel\ntype: number\ncomputed:\n  function: average\n  inputs:\n    - device: HmIP-RF/000A1B2C3D4E03:1/ACTUAL_TEMPERATURE\n    - device: HmIP-RF/000AAAAAAAAAAA:1/ACTUAL_TEMPERATURE\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVariable(mean, nil, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateVariable(&model.Variable{Name: "Am Kanal", Type: model.VarBool, Visible: true, Channel: "HmIP-RF/000A1B2C3D4E03:3"}, nil, admin); err != nil {
		t.Fatal(err)
	}

	uses := s.DeviceUses()
	if len(uses) != 3 || uses[0].Device != (Device{"BidCos-RF", "NEQ0000001"}) || uses[1].Device != old {
		t.Fatalf("uses: %+v", uses)
	}
	if u := uses[1]; !reflect.DeepEqual(u.Datapoints, []string{"0/LOW_BAT", "1/ACTUAL_TEMPERATURE", "2/STATE", "3", "3/STATE"}) || !reflect.DeepEqual(u.Programs, []string{"alles"}) || !reflect.DeepEqual(u.Variables, []string{"am-kanal", "mittel"}) {
		t.Errorf("the old device's use: %+v", u)
	}
	if u := uses[2]; !reflect.DeepEqual(u.Programs, []string{"alles", "andere"}) || !reflect.DeepEqual(u.Variables, []string{"mittel"}) {
		t.Errorf("the other device's use: %+v", u)
	}

	// what is refused
	for _, c := range []struct {
		from, to Device
		code     string
	}{
		{old, old, "invalid"},
		{Device{"HmIP-RF", ""}, neu, "invalid"},
		{old, Device{"Hm IP", "X"}, "invalid"},
		{old, Device{"HmIP-RF", "000F:1"}, "invalid"},
		{Device{"HmIP-RF", "000NOBODY"}, neu, "not-found"},
	} {
		if _, err := s.ReplaceDevice(c.from, c.to, nil, admin, false); codeOf(err) != c.code {
			t.Errorf("%v -> %v: %v", c.from, c.to, err)
		}
	}
	if _, err := s.ReplaceDevice(old, neu, map[string][]byte{"../x": nil}, admin, false); codeOf(err) != "invalid" {
		t.Errorf("an extra file outside the tree: %v", err)
	}

	// a dry run says what would change and changes nothing
	res, err := s.ReplaceDevice(old, neu, nil, admin, true)
	if err != nil || !reflect.DeepEqual(res.Programs, []string{"alles"}) || !reflect.DeepEqual(res.Variables, []string{"am-kanal", "mittel"}) || res.References != 7 {
		t.Fatalf("dry: %+v %v", res, err)
	}
	if len(s.DeviceUses()) != 3 || s.DeviceUses()[1].Device != old {
		t.Fatal("the dry run changed something")
	}
	s.Flush()
	entries, _ := s.History("", 0)

	res, err = s.ReplaceDevice(old, neu, map[string][]byte{"devices-seen.yaml": []byte("devices: []\n")}, admin, false)
	if err != nil || res.References != 7 {
		t.Fatalf("replace: %+v %v", res, err)
	}
	s.Flush()
	text, _ := os.ReadFile(filepath.Join(dir, "tree", "programs", "alles.yaml"))
	for _, want := range []string{
		"device: HmIP-RF/000F00000000FF:3/STATE", "device: HmIP-RF/000F00000000FF:0/LOW_BAT", "set: HmIP-RF/000F00000000FF:3/STATE", "from: HmIP-RF/000F00000000FF:2/STATE",
		"channel: HmIP-RF/000F00000000FF:3", "device: HmIP-RF/000AAAAAAAAAAA:1/STATE", "set: BidCos-RF/NEQ0000001:1/STATE",
		// free text stays
		`webhook: "http://127.0.0.1/HmIP-RF/000A1B2C3D4E03:3/STATE"`, "to: HmIP-RF/000A1B2C3D4E03:3/STATE",
	} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the program lacks %q:\n%s", want, text)
		}
	}
	if v, _ := os.ReadFile(filepath.Join(dir, "tree", "variables", "mittel.yaml")); !strings.Contains(string(v), "000F00000000FF:1/ACTUAL_TEMPERATURE") || !strings.Contains(string(v), "000AAAAAAAAAAA:1/ACTUAL_TEMPERATURE") {
		t.Errorf("the computed value: %s", v)
	}
	if v, _ := os.ReadFile(filepath.Join(dir, "tree", "variables", "am-kanal.yaml")); !strings.Contains(string(v), "channel: HmIP-RF/000F00000000FF:3") {
		t.Errorf("the variable's channel: %s", v)
	}
	if seen, _ := os.ReadFile(filepath.Join(dir, "tree", "devices-seen.yaml")); string(seen) != "devices: []\n" {
		t.Errorf("the extra file: %q", seen)
	}
	uses = s.DeviceUses()
	if len(uses) != 3 || uses[2].Device != neu || len(uses[2].Datapoints) != 5 {
		t.Errorf("uses afterwards: %+v", uses)
	}
	// the engine listens to the new device, not the old one
	refs := map[string]bool{}
	for _, r := range s.Engine().Refs() {
		refs[r.String()] = true
	}
	if !refs["HmIP-RF/000F00000000FF:3/STATE"] || refs["HmIP-RF/000A1B2C3D4E03:3/STATE"] {
		t.Errorf("the engine's references: %v", refs)
	}
	// one entry in the history, with all the files
	after, _ := s.History("", 0)
	if len(after) != len(entries)+1 || after[0].Message != "Device HmIP-RF/000A1B2C3D4E03 replaced by HmIP-RF/000F00000000FF" || after[0].Author != "Admin" {
		t.Errorf("the history: %+v", after[0])
	}
	for _, path := range []string{"programs/alles.yaml", "variables/mittel.yaml", "variables/am-kanal.yaml", "devices-seen.yaml"} {
		if h, _ := s.History(path, 1); len(h) != 1 || h[0].Hash != after[0].Hash {
			t.Errorf("%s is not in the replacement's entry", path)
		}
	}
	// onto a device on another interface whose address the format refuses for nobody: fine;
	// onto one that makes a file unreadable: nothing is written
	if _, err := s.ReplaceDevice(neu, Device{"BidCos-RF", "NEQ0000002"}, nil, admin, false); err != nil {
		t.Errorf("onto another interface: %v", err)
	}
}

// Every example program of the repository - the guide's in both languages, the specs' seeds,
// the importer's - names its devices the way the replacement finds them: after each device was
// replaced, the old address is nowhere left but in free text. An activity kind that names its
// device another way fails here.
func TestReplaceFindsEveryReference(t *testing.T) {
	var texts []string
	block := regexp.MustCompile("(?s)```yaml\n(.*?)```")
	for lang := range docs.Languages {
		src, _ := docs.Source(lang)
		for _, m := range block.FindAllSubmatch(src, -1) {
			texts = append(texts, string(m[1]))
		}
	}
	for _, glob := range []string{"../../ui/e2e/*/programs/*.yaml", "../importer/testdata/golden/*/programs/*.yaml", "../importer/testdata/golden/*/variables/*.yaml", "../../ui/e2e/*/variables/*.yaml"} {
		files, _ := filepath.Glob(glob)
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			texts = append(texts, string(data))
		}
	}
	texts = append(texts, everyKind)
	free := regexp.MustCompile(`^\s*(- )?(name|note|webhook|body|value|to|script|text): `)
	address := regexp.MustCompile(`[A-Za-z][A-Za-z0-9_-]*/[A-Za-z0-9_.-]+:[0-9]+`)
	to := Device{"Neu-RF", "NEU0000001"}
	devices, programs := 0, 0
	for _, text := range texts {
		var canon []byte
		if p, err := model.DecodeProgram("x.yaml", "x", []byte(text)); err == nil {
			canon = model.EncodeProgram(p, nil)
			programs++
		} else if v, err := model.DecodeVariable("x.yaml", "x", []byte(text)); err == nil {
			canon = model.EncodeVariable(v)
		} else {
			continue // a seed that is broken on purpose
		}
		found := map[Device]bool{}
		references(canon, func(d Device, _ string) { found[d] = true })
		out := canon
		for d := range found {
			devices++
			var n int
			if out, n = repoint(out, d, to); n == 0 {
				t.Errorf("%v was found and not replaced", d)
			}
		}
		for _, line := range strings.Split(string(out), "\n") {
			if free.MatchString(line) {
				continue
			}
			for _, a := range address.FindAllString(line, -1) {
				if !strings.HasPrefix(a, "Neu-RF/NEU0000001:") {
					t.Errorf("a device reference the replacement does not find: %q in\n%s", line, text)
				}
			}
		}
	}
	if programs < 15 || devices < 10 {
		t.Errorf("only %d programs with %d devices looked at", programs, devices)
	}
}
