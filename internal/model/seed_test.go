package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The programs the page's time tests start with (ui/e2e/time-seed) are files the reader takes,
// written the canonical way.
func TestTimeSeedPrograms(t *testing.T) {
	files, _ := filepath.Glob("../../ui/e2e/time-seed/programs/*.yaml")
	if len(files) < 5 {
		t.Fatalf("%d seed programs", len(files))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := DecodeProgram(f, strings.TrimSuffix(filepath.Base(f), ".yaml"), data)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		if got := EncodeProgram(p, nil); string(got) != string(data) {
			t.Errorf("%s is not canonical:\n%s", f, got)
		}
	}
}
