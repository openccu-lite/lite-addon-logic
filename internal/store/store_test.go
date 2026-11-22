package store

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

func variable(key string) *model.Variable {
	return &model.Variable{Key: key, Name: strings.ToUpper(key), Type: model.VarBool, Visible: true}
}

func program(key string) *model.Program {
	return &model.Program{Key: key, Name: "Program " + key, Visible: true, Operable: true, Rules: []model.Rule{{Else: true, Then: []model.Action{{Kind: model.ActionVariable, Variable: "regen", To: true}}}}}
}

func TestSaveLoadDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tree")
	tr, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	path, changed, err := tr.SaveVariable(variable("regen"))
	if err != nil || !changed || path != "variables/regen.yaml" {
		t.Fatalf("save: %s %v %v", path, changed, err)
	}
	if _, changed, err = tr.SaveVariable(variable("regen")); err != nil || changed {
		t.Fatalf("the same bytes again: %v %v", changed, err)
	}
	if path, changed, err = tr.SaveProgram(program("markise"), nil); err != nil || !changed || path != "programs/markise.yaml" {
		t.Fatalf("save: %s %v %v", path, changed, err)
	}
	if _, changed, _ = tr.SaveProgram(program("markise"), nil); changed {
		t.Fatal("the same bytes again")
	}
	if data, err := tr.Read("programs/markise.yaml"); err != nil || !strings.HasPrefix(string(data), "name: Program markise\n") {
		t.Fatalf("read: %q %v", data, err)
	}
	if st, _ := os.Stat(filepath.Join(dir, "programs", "markise.yaml")); st.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", st.Mode())
	}
	// no temporary file stays behind
	entries, _ := os.ReadDir(filepath.Join(dir, "programs"))
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
	set, bad, err := tr.Load()
	if err != nil || len(bad) != 0 || len(set.Programs) != 1 || len(set.Variables) != 1 || set.Programs["markise"].Name != "Program markise" {
		t.Fatalf("load: %+v %v %v", set, bad, err)
	}
	// an invalid object is refused before anything is written
	if _, _, err := tr.SaveProgram(&model.Program{Key: "x"}, nil); err == nil {
		t.Error("a program without a name was saved")
	}
	if _, _, err := tr.SaveVariable(&model.Variable{Key: "x", Name: "X"}); err == nil {
		t.Error("a variable without a type was saved")
	}
	if _, err := os.Stat(filepath.Join(dir, "programs", "x.yaml")); err == nil {
		t.Error("the refused program has a file")
	}
	if p, err := tr.DeleteProgram("markise"); err != nil || p != "programs/markise.yaml" {
		t.Fatalf("delete: %v", err)
	}
	if _, err := tr.DeleteProgram("markise"); err != nil {
		t.Errorf("deleting what is gone: %v", err)
	}
	if _, err := tr.DeleteVariable("regen"); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.DeleteVariable("../x"); err == nil {
		t.Error("a path as a key")
	}
	if set, _, _ := tr.Load(); len(set.Programs)+len(set.Variables) != 0 {
		t.Errorf("after the deletes: %+v", set)
	}
}

// A broken file stops only its own object; what is not a .yaml file is not looked at.
func TestLoadReportsBadFiles(t *testing.T) {
	dir := t.TempDir()
	tr, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = tr.SaveVariable(variable("regen"))
	_, _, _ = tr.SaveProgram(program("good"), nil)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "programs", "broken.yaml"), []byte("name: X\nrules: nope\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "programs", "Bad Name.yaml"), []byte("name: X\nrules: []\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "variables", "broken.yaml"), []byte("name: X\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "programs", "README.md"), []byte("not ours"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "programs", ".hidden.yaml"), []byte("not ours"), 0o644))
	must(os.Mkdir(filepath.Join(dir, "programs", "folder.yaml"), 0o755))
	must(os.Symlink("/nonexistent", filepath.Join(dir, "variables", "dangling.yaml")))
	must(os.Symlink("/nonexistent", filepath.Join(dir, "programs", "dangling.yaml")))
	set, bad, err := tr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Programs) != 1 || set.Programs["good"] == nil || len(set.Variables) != 1 {
		t.Errorf("loaded: %v %v", set.Programs, set.Variables)
	}
	var got []string
	for _, b := range bad {
		got = append(got, b.Path+": "+b.Error())
	}
	want := []string{"variables/broken.yaml: variables/broken.yaml:1: a variable needs a \"type\"", "variables/dangling.yaml: ", "programs/Bad Name.yaml: programs/Bad Name.yaml: \"Bad Name\" cannot be a program's key", "programs/broken.yaml: programs/broken.yaml:2: rules must be a list", "programs/dangling.yaml: "}
	if len(got) != len(want) {
		t.Fatalf("bad files: %q", got)
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || strings.HasPrefix(g, w)
		}
		if !found {
			t.Errorf("no report starting %q in %q", w, got)
		}
	}
}

func TestMissingAndUnreadableDirectories(t *testing.T) {
	dir := t.TempDir()
	// a tree whose folders are gone is empty, not an error
	tr := &Tree{Dir: dir}
	if set, bad, err := tr.Load(); err != nil || len(bad) != 0 || len(set.Programs) != 0 {
		t.Fatalf("empty: %v %v", bad, err)
	}
	// a save makes the folder again
	if _, _, err := tr.SaveVariable(variable("v")); err != nil {
		t.Fatal(err)
	}
	// a file where a folder should be
	if err := os.WriteFile(filepath.Join(dir, "programs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tr.Load(); err == nil {
		t.Error("programs is a file and Load says nothing")
	}
	if _, _, err := tr.SaveProgram(program("p"), nil); err == nil {
		t.Error("programs is a file and a save worked")
	}
	if _, err := Open(filepath.Join(dir, "programs", "below")); err == nil {
		t.Error("Open below a file")
	}
	if err := os.Remove(filepath.Join(dir, "variables", "v.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "variables")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "variables"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tr.Load(); err == nil {
		t.Error("variables is a file and Load says nothing")
	}
	if err := WriteFile(filepath.Join(dir, "nowhere", "f"), []byte("x"), 0o600); err == nil {
		t.Error("WriteFile into a directory that is not there")
	}
	// renaming over a directory fails and leaves no temporary file
	target := filepath.Join(dir, "target")
	_ = os.MkdirAll(filepath.Join(target, "sub"), 0o755)
	if err := WriteFile(target, []byte("x"), 0o600); err == nil {
		t.Error("WriteFile over a directory")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("left behind: %s", e.Name())
		}
	}
}

func TestExtraFiles(t *testing.T) {
	tr, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := tr.WriteExtra("imports/ccu-keller.yaml", []byte("a: 1\n")); err != nil || !changed {
		t.Fatalf("write: %v %v", changed, err)
	}
	if changed, _ := tr.WriteExtra("imports/ccu-keller.yaml", []byte("a: 1\n")); changed {
		t.Error("the same bytes again")
	}
	if _, err := tr.WriteExtra("devices-seen.yaml", []byte("x\n")); err != nil {
		t.Fatal(err)
	}
	if b, _ := tr.Read("imports/ccu-keller.yaml"); string(b) != "a: 1\n" {
		t.Errorf("read: %q", b)
	}
	for _, p := range []string{"", "/etc/passwd", "../x", "a/../b", "a//b", ".git/config", "imports/.hidden", "programs/x.yaml", "variables/x.yaml", "programs", "a\\b"} {
		if _, err := tr.WriteExtra(p, []byte("x")); err == nil {
			t.Errorf("the path %q was taken", p)
		}
	}
}

// What a write that was cut off left behind is removed: WriteFile's temporary files, wherever
// below the folder - and nothing else, and nothing of the history's own.
func TestRemoveStale(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	keep := []string{"state/state.json", "state/state.json.1", "tree/programs/a.yaml", "tree/.git/.tmp-123", "tree/.git/objects/.tmp-9",
		"state/.tmp-", "state/.tmp-12a", "state/x.tmp-12", "tree/variables/.tmpfile", "secrets.json"}
	gone := []string{".tmp-77", "state/.tmp-2350104642", "tree/programs/.tmp-1", "tree/variables/.tmp-0042"}
	for _, f := range append(append([]string{}, keep...), gone...) {
		write(f)
	}
	// a folder of the name is no file of a write
	if err := os.MkdirAll(filepath.Join(dir, "state", ".tmp-555"), 0o755); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveStale(dir)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, gone) {
		t.Errorf("removed %v, want %v", removed, gone)
	}
	for _, f := range keep {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s is gone", f)
		}
	}
	for _, f := range gone {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err == nil {
			t.Errorf("%s is still there", f)
		}
	}
	if again, err := RemoveStale(dir); err != nil || len(again) != 0 {
		t.Errorf("a second time: %v %v", again, err)
	}
	// a folder that is not there has nothing; one that cannot be read says so
	if none, err := RemoveStale(filepath.Join(dir, "nowhere")); err != nil || none != nil {
		t.Errorf("no folder: %v %v", none, err)
	}
	if os.Getuid() != 0 {
		locked := filepath.Join(dir, "state", ".tmp-555")
		write("state/.tmp-555/.tmp-1")
		if err := os.Chmod(locked, 0); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(locked, 0o755)
		if _, err := RemoveStale(dir); err == nil {
			t.Error("a folder that cannot be read is no error")
		}
		// a file that cannot be removed says so too
		os.Chmod(locked, 0o755)
		os.Chmod(locked, 0o555)
		if _, err := RemoveStale(dir); err == nil {
			t.Error("a file that cannot be removed is no error")
		}
	}
}
