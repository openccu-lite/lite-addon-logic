package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionsAndRestore(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	defer s.Close()
	if _, err := s.CreateVariable(list("Modus", "Sommer", "Winter"), nil, admin); err != nil {
		t.Fatal(err)
	}
	p := uses("schalter", "Sommer")
	if _, err := s.CreateProgram(p, admin); err != nil {
		t.Fatal(err)
	}
	p2 := p.Clone()
	p2.Note = "zweite Fassung"
	if _, err := s.UpdateProgram("schalter", p2, admin); err != nil {
		t.Fatal(err)
	}
	// a program's versions, the newest first; the file as it is now
	got, err := s.Versions(KindProgram, "schalter", 0)
	if err != nil || len(got) != 2 || got[0].Message != "Program "+p.Name+" changed" || got[1].Message != "Program "+p.Name+" created" || got[0].Author != "Admin" {
		t.Fatalf("versions: %+v, %v", got, err)
	}
	if one, _ := s.Versions(KindProgram, "schalter", 1); len(one) != 1 {
		t.Errorf("limit: %+v", one)
	}
	now, err := s.Current(KindProgram, "schalter")
	if err != nil || !strings.Contains(string(now), "zweite Fassung") {
		t.Fatalf("now: %s, %v", now, err)
	}
	first, err := s.Version(KindProgram, "schalter", got[1].Hash)
	if err != nil || strings.Contains(string(first), "zweite Fassung") || !strings.Contains(string(first), "name: "+p.Name) {
		t.Fatalf("the first version: %s, %v", first, err)
	}
	// restored: the file is the first version's again, as a new entry that says so
	if err := s.Restore(KindProgram, "schalter", got[1].Hash, admin); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Versions(KindProgram, "schalter", 0)
	if len(after) != 3 || after[0].Message != "Program "+p.Name+" restored to its version "+got[1].Hash[:7] {
		t.Fatalf("after the restore: %+v", after)
	}
	if now, _ := s.Current(KindProgram, "schalter"); string(now) != string(first) {
		t.Errorf("the restored file:\n%s\nwant:\n%s", now, first)
	}
	if cur, _ := s.Program("schalter"); cur.Program.Note != "" {
		t.Errorf("the engine's program: %+v", cur.Program)
	}
	// a variable the same way
	v2 := list("Modus", "Sommer", "Winter", "Herbst")
	if _, err := s.UpdateVariable("modus", v2, nil, admin); err != nil {
		t.Fatal(err)
	}
	vs, err := s.Versions(KindVariable, "modus", 0)
	if err != nil || len(vs) != 2 {
		t.Fatalf("the variable's versions: %+v, %v", vs, err)
	}
	if err := s.Restore(KindVariable, "modus", vs[1].Hash, admin); err != nil {
		t.Fatal(err)
	}
	if now, _ := s.Current(KindVariable, "modus"); strings.Contains(string(now), "Herbst") {
		t.Errorf("the restored variable: %s", now)
	}
	if vs, _ = s.Versions(KindVariable, "modus", 0); len(vs) != 3 || !strings.HasPrefix(vs[0].Message, "Variable Modus restored to its version ") {
		t.Errorf("the variable after the restore: %+v", vs)
	}
	// what is not there
	for _, c := range []struct {
		name string
		err  error
	}{
		{"an unknown program", second(s.Versions(KindProgram, "nichts", 0))},
		{"an unknown variable", second(s.Versions(KindVariable, "nichts", 0))},
		{"an unknown kind", second(s.Versions("setting", "modus", 0))},
		{"an unknown program's file", second(s.Current(KindProgram, "nichts"))},
		{"an unknown program's version", second(s.Version(KindProgram, "nichts", got[1].Hash))},
		{"a hash that is none", second(s.Version(KindProgram, "schalter", "nichts"))},
		{"a version of another file", second(s.Version(KindProgram, "schalter", vs[0].Hash[:len(vs[0].Hash)-1]+"0"))},
		{"a restore of nothing", s.Restore(KindProgram, "schalter", "nichts", admin)},
	} {
		if codeOf(c.err) != "not-found" {
			t.Errorf("%s: %v", c.name, c.err)
		}
	}
	// a version that cannot be read any more (the format moved on, or a hand edit): said, nothing saved
	for kind, path := range map[string]string{KindProgram: "programs/schalter.yaml", KindVariable: "variables/modus.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, "tree", filepath.FromSlash(path)), []byte("name: [broken\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.queue.Repo().Commit([]string{path}, admin, "by hand"); err != nil {
			t.Fatal(err)
		}
		key := strings.TrimSuffix(filepath.Base(path), ".yaml")
		broken, _ := s.Versions(kind, key, 1)
		if err := s.Restore(kind, key, broken[0].Hash, admin); codeOf(err) != "invalid" {
			t.Errorf("%s: a broken version: %v", kind, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }
