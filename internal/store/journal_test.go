package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testKey reads "key value", "key !" (a clear); a line without a blank is none.
func testKey(line []byte) (string, bool, bool) {
	k, v, ok := strings.Cut(string(line), " ")
	return k, v == "!", ok
}

func journalLines(n int, key string, from int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = []byte(fmt.Sprintf("%s %d", key, from+i))
	}
	return out
}

func TestJournal(t *testing.T) {
	dir := t.TempDir()
	j := &Journal{Path: filepath.Join(dir, "log"), Max: 3, Slack: 2, Key: testKey}
	text := func() string {
		b, err := os.ReadFile(j.Path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	read := func(key string) string {
		t.Helper()
		lines, err := j.Read(key)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, l := range lines {
			out = append(out, string(l))
		}
		return strings.Join(out, ",")
	}
	add := func(lines ...string) {
		t.Helper()
		var b [][]byte
		for _, l := range lines {
			b = append(b, []byte(l))
		}
		if err := j.Append(b, func(key string) bool { return key != "gone" }); err != nil {
			t.Fatal(err)
		}
	}
	if j.Exists() || read("a") != "" {
		t.Fatal("something before anything was written")
	}
	if err := j.Append(nil, nil); err != nil || j.Exists() {
		t.Fatalf("nothing appended made a file: %v", err)
	}
	add("a 1", "b 1", "a 2")
	if !j.Exists() || text() != "a 1\nb 1\na 2\n" || read("a") != "a 1,a 2" || read("b") != "b 1" || read("c") != "" {
		t.Fatalf("after three lines: %q", text())
	}
	if j.Written() != 12 {
		t.Errorf("written: %d", j.Written())
	}
	// more than Max of a key: a read answers the newest, the file keeps all until it is
	// rewritten - at more than twice the kept lines and the slack
	add("a 3", "a 4", "a 5", "gone 1", "junk")
	if read("a") != "a 3,a 4,a 5" || !strings.HasPrefix(text(), "a 1\n") {
		t.Fatalf("before the rewrite: %q, a: %s", text(), read("a"))
	}
	add("b 2")
	if got := text(); got != "b 1\na 3\na 4\na 5\nb 2\n" {
		t.Fatalf("rewritten with what is kept: %q", got)
	}
	if j.Written() != 12+24+4+20 {
		t.Errorf("written: %d", j.Written())
	}
	// a clear voids what was there under its key; the clear itself goes with the next rewrite
	add("a !", "a 6")
	if read("a") != "a 6" || read("b") != "b 1,b 2" {
		t.Fatalf("after a clear: a %s, b %s", read("a"), read("b"))
	}
	add("b 3", "b 4", "b 5", "b 6", "b 7", "b 8", "b 9")
	if got := text(); got != "a 6\nb 7\nb 8\nb 9\n" {
		t.Fatalf("the second rewrite: %q", got)
	}
	// a long run of one key between rewrites is read without holding it all
	big := &Journal{Path: filepath.Join(dir, "big"), Max: 3, Slack: 1000, Key: testKey}
	if err := big.Append(journalLines(50, "a", 0), nil); err != nil {
		t.Fatal(err)
	}
	if lines, err := big.Read("a"); err != nil || len(lines) != 3 || string(lines[0]) != "a 47" || string(lines[2]) != "a 49" {
		t.Fatalf("the newest of fifty: %q, %v", lines, err)
	}
	// another start: a file cut short gets its line end first, the torn line is not read
	if err := os.WriteFile(j.Path, []byte("a 1\na 2\nb 1\nb"), 0o600); err != nil {
		t.Fatal(err)
	}
	j = &Journal{Path: j.Path, Max: 3, Slack: 2, Key: testKey}
	add("a 3")
	if got := text(); got != "a 1\na 2\nb 1\nb\na 3\n" || read("a") != "a 1,a 2,a 3" || read("b") != "b 1" {
		t.Fatalf("after a torn line: %q", got)
	}
	// a file that needs no rewrite although it is long: every line is kept
	keep := &Journal{Path: filepath.Join(dir, "keep"), Max: 100, Key: testKey}
	if err := keep.Append(journalLines(5, "a", 0), nil); err != nil {
		t.Fatal(err)
	}
	keep = &Journal{Path: keep.Path, Max: 100, Key: testKey}
	keep.opened, keep.n, keep.kept = true, 5, 1 // as if most of it had been another key's
	if err := keep.Append(journalLines(1, "a", 5), nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(keep.Path); strings.Count(string(b), "\n") != 6 || keep.kept != 6 {
		t.Errorf("rewritten without need: %q", b)
	}
	// an empty file is no line
	if err := os.WriteFile(j.Path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	j = &Journal{Path: j.Path, Max: 3, Key: testKey}
	if read("a") != "" {
		t.Error("an empty file has lines")
	}
	// what cannot be read or written says so
	bad := &Journal{Path: dir, Max: 3, Key: testKey}
	if _, err := bad.Read("a"); err == nil {
		t.Error("a directory was read")
	}
	if err := bad.Append(journalLines(1, "a", 0), nil); err == nil {
		t.Error("a directory was written")
	}
	bad.opened = true
	if err := bad.Append(journalLines(1, "a", 0), nil); err == nil {
		t.Error("a directory was written")
	}
	if err := bad.rewrite(nil); err == nil {
		t.Error("a directory was rewritten")
	}
	gone := &Journal{Path: filepath.Join(dir, "none", "log"), Max: 1, Key: testKey}
	if err := gone.Append(journalLines(1, "a", 0), nil); err == nil || !strings.Contains(err.Error(), "none") {
		t.Errorf("no folder: %v", err)
	}
	// a folder that takes no new file: the rewrite fails, the file stays whole
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	locked := &Journal{Path: filepath.Join(ro, "log"), Max: 1, Key: testKey}
	if err := locked.Append(journalLines(2, "a", 0), nil); err != nil {
		t.Fatal(err)
	}
	if os.Getuid() != 0 {
		if err := os.Chmod(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(ro, 0o755)
		if err := locked.Append(journalLines(2, "a", 2), nil); err == nil {
			t.Error("rewritten in a folder that takes no file")
		}
		if lines, err := locked.Read("a"); err != nil || len(lines) != 1 || string(lines[0]) != "a 3" {
			t.Errorf("after a failed rewrite: %q, %v", lines, err)
		}
	}
}

// What goes to storage follows the new lines, not what the file keeps: with every key's ring
// full, a line costs its own bytes and at most twice more through the rewrites - and the file
// never holds more than about twice what is kept.
func TestJournalCost(t *testing.T) {
	const keys, max, slack = 20, 50, 10
	j := &Journal{Path: filepath.Join(t.TempDir(), "log"), Max: max, Slack: slack, Key: testKey}
	line := func(i int) []byte { return []byte(fmt.Sprintf("k%02d %06d", i%keys, i)) }
	i := 0
	for ; i < 3*keys*max; i++ { // until every ring is full and rewrites have happened
		if err := j.Append([][]byte{line(i)}, nil); err != nil {
			t.Fatal(err)
		}
	}
	before := j.Written()
	const more = 10 * keys * max
	var appended, largest int64
	for end := i + more; i < end; i += 7 {
		var batch [][]byte
		for k := 0; k < 7; k++ {
			batch = append(batch, line(i+k))
			appended += int64(len(line(i+k))) + 1
		}
		if err := j.Append(batch, nil); err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(j.Path)
		if err != nil {
			t.Fatal(err)
		}
		largest = max64(largest, st.Size())
	}
	written := j.Written() - before
	t.Logf("%d bytes of new lines, %d bytes went to storage (%.2f times); the file's largest size %d bytes, kept are %d", appended, written, float64(written)/float64(appended), largest, keys*max*11)
	if written > 3*appended {
		t.Errorf("%d bytes written for %d bytes of new lines", written, appended)
	}
	if limit := int64((2*keys*max + slack + 7) * 11); largest > limit {
		t.Errorf("the file grew to %d bytes, twice the kept lines and the slack are %d", largest, limit)
	}
	for k := 0; k < keys; k++ {
		lines, err := j.Read(fmt.Sprintf("k%02d", k))
		if err != nil || len(lines) != max {
			t.Fatalf("key %d: %d lines, %v", k, len(lines), err)
		}
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
