package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRing(t *testing.T) {
	dir := t.TempDir()
	r := &Ring{Path: filepath.Join(dir, "ring"), Max: 3}
	if lines, err := r.Load(); err != nil || lines != nil {
		t.Fatalf("no file yet: %q, %v", lines, err)
	}
	if err := r.Append(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.Path); err == nil {
		t.Error("nothing appended made a file")
	}
	text := func() string {
		b, err := os.ReadFile(r.Path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for i := 1; i <= 6; i++ {
		if err := r.Append([][]byte{[]byte(fmt.Sprint(i))}); err != nil {
			t.Fatal(err)
		}
	}
	if got := text(); got != "1\n2\n3\n4\n5\n6\n" {
		t.Errorf("up to twice the size everything stays: %q", got)
	}
	if err := r.Append([][]byte{[]byte("7"), []byte("8")}); err != nil {
		t.Fatal(err)
	}
	if got := text(); got != "6\n7\n8\n" {
		t.Errorf("shortened to the newest: %q", got)
	}
	// another start reads the newest, and a file cut short gets its line end first
	if err := os.WriteFile(r.Path, []byte("a\nb\nc\nd\ne"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = &Ring{Path: r.Path, Max: 3}
	lines, err := r.Load()
	if err != nil || len(lines) != 3 || string(lines[0]) != "c" || string(lines[2]) != "e" {
		t.Fatalf("loaded %q, %v", lines, err)
	}
	if err := r.Append([][]byte{[]byte("f")}); err != nil {
		t.Fatal(err)
	}
	if got := text(); got != "a\nb\nc\nd\ne\nf\n" {
		t.Errorf("after a torn line: %q", got)
	}
	// an empty file is no line
	if err := os.WriteFile(r.Path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if lines, err := r.Load(); err != nil || len(lines) != 0 {
		t.Errorf("empty: %q, %v", lines, err)
	}
	// what cannot be read or written says so
	bad := &Ring{Path: dir, Max: 3}
	if _, err := bad.Load(); err == nil {
		t.Error("a directory was read")
	}
	if err := bad.Append([][]byte{[]byte("x")}); err == nil {
		t.Error("a directory was written")
	}
	gone := &Ring{Path: filepath.Join(dir, "none", "ring"), Max: 1}
	if err := gone.Append([][]byte{[]byte("x")}); err == nil || !strings.Contains(err.Error(), "none") {
		t.Errorf("no folder: %v", err)
	}
}
