package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// "logic anonymise" end to end: a database in, an anonymised database out, never over a file
// that is there, nothing left behind when it fails.
func TestAnonymise(t *testing.T) {
	dir := t.TempDir()
	in := "../../internal/regadom/testdata/fixture.regadom"
	out := filepath.Join(dir, "out.regadom")
	var stdout, stderr bytes.Buffer
	if rc := anonymise([]string{in, out}, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc %d: %s", rc, stderr.String())
	}
	if !strings.Contains(stdout.String(), "names") || !strings.Contains(stdout.String(), "Look through the file") {
		t.Errorf("the account: %s", stdout.String())
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	src, err := regadom.Read(bytes.NewReader(b))
	if err != nil || len(src.DB.Programs) != 20 || bytes.Contains(b, []byte("Heizperiode")) {
		t.Errorf("the output: %v", err)
	}
	if info, _ := os.Stat(out); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode())
	}
	for name, c := range map[string]struct {
		args []string
		rc   int
		want string
	}{
		"usage":               {[]string{in}, 2, "usage: logic anonymise"},
		"no input":            {[]string{filepath.Join(dir, "nope"), filepath.Join(dir, "x")}, 1, "no such file"},
		"the output is there": {[]string{in, out}, 1, "file exists"},
	} {
		stderr.Reset()
		if rc := anonymise(c.args, &stdout, &stderr); rc != c.rc || !strings.Contains(stderr.String(), c.want) {
			t.Errorf("%s: rc %d, %s", name, rc, stderr.String())
		}
	}
	bad := filepath.Join(dir, "bad")
	_ = os.WriteFile(bad, []byte("<html/>"), 0o600)
	left := filepath.Join(dir, "left")
	stderr.Reset()
	if rc := anonymise([]string{bad, left}, &stdout, &stderr); rc != 1 || !strings.Contains(stderr.String(), "not a ReGa database") {
		t.Errorf("a file that is no database: rc %d, %s", rc, stderr.String())
	}
	if _, err := os.Stat(left); err == nil {
		t.Error("a failed run leaves its output behind")
	}
}
