package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStateFileGenerations(t *testing.T) {
	dir := t.TempDir()
	s := StateFile{Path: filepath.Join(dir, "state.json")}
	valid := func(b []byte) bool { return json.Valid(b) }
	if b, gen, damaged := s.Load(valid); b != nil || gen != -1 || damaged {
		t.Fatalf("a first start: %q %d %v", b, gen, damaged)
	}
	if err := s.Save([]byte(`{"n":1}`)); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(s.Path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	if err := s.Save([]byte(`{"n":2}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Save([]byte(`{"n":3}`)); err != nil {
		t.Fatal(err)
	}
	if b, gen, damaged := s.Load(valid); string(b) != `{"n":3}` || gen != 0 || damaged {
		t.Fatalf("load: %q %d %v", b, gen, damaged)
	}
	if b, _ := os.ReadFile(s.Path + ".1"); string(b) != `{"n":2}` {
		t.Fatalf("the generation before: %q", b)
	}
	// the file is torn: the one before is used, and the caller is told
	if err := os.WriteFile(s.Path, []byte(`{"n":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, gen, damaged := s.Load(valid); string(b) != `{"n":2}` || gen != 1 || !damaged {
		t.Fatalf("a torn file: %q %d %v", b, gen, damaged)
	}
	// both are damaged: nothing, and the caller is told
	if err := os.WriteFile(s.Path+".1", []byte(`x`), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, gen, damaged := s.Load(valid); b != nil || gen != -1 || !damaged {
		t.Fatalf("both damaged: %q %d %v", b, gen, damaged)
	}
	// a save over a damaged file works again
	if err := s.Save([]byte(`{"n":4}`)); err != nil {
		t.Fatal(err)
	}
	if b, gen, _ := s.Load(valid); string(b) != `{"n":4}` || gen != 0 {
		t.Fatalf("after the damage: %q %d", b, gen)
	}
	// a directory where the file should be cannot be read, and a state cannot be saved into
	// a directory that is not there
	bad := StateFile{Path: filepath.Join(dir, "sub")}
	_ = os.Mkdir(bad.Path, 0o755)
	if b, _, damaged := bad.Load(valid); b != nil || !damaged {
		t.Errorf("a directory as the state file: %q %v", b, damaged)
	}
	if err := bad.Save([]byte("{}")); err == nil {
		t.Error("a save over a directory")
	}
	if err := (StateFile{Path: filepath.Join(dir, "nowhere", "state.json")}).Save([]byte("{}")); err == nil {
		t.Error("a save into a directory that is not there")
	}
}
