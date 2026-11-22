package importer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
)

// import -> the service stores it -> reload: the tree gives back what was imported, with the
// ledger and the device notes in the same step; a second import of the same source changes
// nothing; the same database as another source lands beside it.
func TestThroughTheService(t *testing.T) {
	dir := t.TempDir()
	svc, err := service.Open(service.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	tree := svc.Tree()
	if l, err := LoadLedger(tree, "ccu"); l != nil || err != nil {
		t.Fatalf("no ledger yet: %+v %v", l, err)
	}
	if _, err := LoadLedger(tree, "../etc"); err == nil {
		t.Fatal("a source's name that is a path")
	}
	known := func(ch string) bool { return ch == "HmIP-RF/00111BE9AB9012:4" }
	apply := func(res *Result) *service.ImportResult {
		t.Helper()
		have, _ := tree.Read(SeenPath)
		out, err := svc.ApplyImport(service.Import{Programs: res.Programs, Variables: res.Variables, Values: res.Values, Files: res.Files(have)}, repo.Author{Name: "Test"})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	res := Import(source(t, "devices"), Options{Location: berlin(t), Known: known})
	if out := apply(res); out.Programs != 3 || out.Variables != 3 || len(out.Deactivated) != 0 || len(out.Values) != 0 {
		t.Errorf("%+v", out)
	}
	set, bad, err := tree.Load()
	if err != nil || len(bad) != 0 || len(set.Programs) != 3 || len(set.Variables) != 3 {
		t.Fatalf("reload: %v %v %d %d", err, bad, len(set.Programs), len(set.Variables))
	}
	for _, p := range res.Programs {
		if !reflect.DeepEqual(set.Programs[p.Key], p) {
			t.Errorf("%s came back different", p.Key)
		}
	}
	if v, ok := svc.Engine().Value("regen"); !ok || v != true {
		t.Errorf("the value came along: %v", v)
	}
	prev, err := LoadLedger(tree, "ccu")
	if err != nil || len(prev.Programs) != 3 || len(prev.Variables) != 3 {
		t.Fatalf("%+v %v", prev, err)
	}
	data, _ := tree.Read(SeenPath)
	if seen, err := DecodeSeen(data); err != nil || len(seen) != 3 {
		t.Errorf("device notes: %v %v", seen, err)
	}
	// again: the ledger is found, the keys are kept, the files are the same bytes
	before := snapshot(t, tree.Dir)
	again := Import(source(t, "devices"), Options{Location: berlin(t), Known: known, Previous: prev, Taken: InStore(set)})
	apply(again)
	if after := snapshot(t, tree.Dir); !reflect.DeepEqual(before, after) {
		t.Errorf("a second import changed the tree:\n%v\n%v", before, after)
	}
	// the same database as another source: its own files, the device notes merged
	other := Import(source(t, "devices"), Options{Location: berlin(t), Source: "ferienhaus", Taken: InStore(set)})
	apply(other)
	set, _, _ = tree.Load()
	if len(set.Programs) != 6 || set.Variables["regen-2"] == nil {
		t.Errorf("another source: %d programs", len(set.Programs))
	}
	if l, err := LoadLedger(tree, "ferienhaus"); err != nil || l.Variables[3000] != "regen-2" {
		t.Errorf("%+v %v", l, err)
	}
	data, _ = tree.Read(SeenPath)
	if seen, err := DecodeSeen(data); err != nil || len(seen) != 4 {
		t.Errorf("merged device notes: %v %v", seen, err)
	}
	// device notes nobody can read are replaced; a damaged ledger is an error, not a fresh start
	files := res.Files([]byte("not: yaml: at: all"))
	if seen, err := DecodeSeen(files[SeenPath]); err != nil || len(seen) != 3 {
		t.Errorf("%v %v", seen, err)
	}
	if err := os.WriteFile(filepath.Join(tree.Dir, "imports", "ccu.yaml"), []byte("source: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLedger(tree, "ccu"); err == nil {
		t.Error("a damaged ledger was read")
	}
	if err := os.Remove(filepath.Join(tree.Dir, "imports", "ccu.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(tree.Dir, "imports", "ccu.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLedger(tree, "ccu"); err == nil {
		t.Error("a directory was read as a ledger")
	}
}

// A program on the CCU's message counters reads the system values: no variable is written for
// them, and the service takes the program as it is.
func TestSystemValuesThroughTheService(t *testing.T) {
	b := newBuild()
	b.db.Variables = append(b.db.Variables, builtin(40, "${sysVarAlarmMessages}", "3"))
	flag := b.variable("Merker", bin, 2, "0", nil)
	b.program("Alarmmeldungen anzeigen", true, ruleSpec{blocks: [][]lineSpec{{on(40, 9, 13, val("1", itg))}}, actions: []actionSpec{set(flag.ID, val("1", bin))}})
	res := b.run(t, Options{})
	svc, err := service.Open(service.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	out, err := svc.ApplyImport(service.Import{Programs: res.Programs, Variables: res.Variables, Values: res.Values, Files: res.Files(nil)}, repo.Author{Name: "Test"})
	if err != nil || len(out.Deactivated) != 0 {
		t.Fatalf("%+v %v", out, err)
	}
	p, ok := svc.Program("alarmmeldungen-anzeigen")
	if !ok || !p.Program.Active || len(p.Problems) != 0 || p.Program.Rules[0].When.Groups[0].Lines[0].Variable != model.SystemAlarmMessages {
		t.Errorf("%+v", p)
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			return nil
		}
		b, _ := os.ReadFile(path)
		rel, _ := filepath.Rel(dir, path)
		out[rel] = string(b)
		return nil
	})
	return out
}
