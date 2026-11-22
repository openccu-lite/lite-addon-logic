package store

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Journal is a file of lines that each belong to a key, and keeps the newest Max lines of
// every key: new lines are appended - only they go to storage - and once the file holds more
// than twice what it would keep, it is written anew with what is kept. A line may also say
// that everything before it under its key is void (a clear). Nothing of the file is held in
// memory: a read and the rewrite go through it line by line. One writer.
//
// What it costs: a line is written once when it is appended, and at most about twice more by
// rewrites over its life (a rewrite comes only after as many lines were appended as it keeps,
// plus Slack) - so the bytes that go to storage follow the new lines, never the file's size.
type Journal struct {
	Path string
	// Max is how many lines are kept per key.
	Max int
	// Slack is how many lines beyond twice the kept ones the file may hold before a rewrite
	// (so that a small file is not rewritten all the time).
	Slack int
	// Key says which key a line belongs to and whether it is a clear; ok is false for a line
	// that is none (a write cut short): it is left out.
	Key func(line []byte) (key string, clear, ok bool)
	// n is the number of lines in the file, kept how many of them the last look would have
	// kept; torn says the file does not end with a line end
	n, kept int
	torn    bool
	opened  bool
	// written counts the bytes that went to storage
	written int64
}

// Written answers how many bytes went to storage through this journal: the appended lines
// and the rewrites.
func (j *Journal) Written() int64 { return j.written }

// Exists says whether there is a file.
func (j *Journal) Exists() bool {
	_, err := os.Stat(j.Path)
	return err == nil
}

// each calls fn with every line of the file, in order; the slice is only valid during the
// call. It answers whether the file ends without a line end.
func (j *Journal) each(fn func(line []byte) error) (torn bool, err error) {
	f, err := os.Open(j.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			torn = line[len(line)-1] != '\n'
			if ferr := fn(bytes.TrimSuffix(line, []byte("\n"))); ferr != nil {
				return torn, ferr
			}
		}
		if err == io.EOF {
			return torn, nil
		}
		if err != nil {
			return torn, err
		}
	}
}

// survey reads the file once and answers, per key that keep takes, how many of its lines a
// rewrite would leave out from the front (those before its last clear, and those beyond Max),
// the number of lines in the file and the number a rewrite would keep.
func (j *Journal) survey(keep func(key string) bool) (skip map[string]int, n, kept int, torn bool, err error) {
	// per key: the lines since its last clear, and all of them
	type count struct{ live, all int }
	counts := map[string]*count{}
	torn, err = j.each(func(line []byte) error {
		n++
		key, clear, ok := j.Key(line)
		if !ok || (keep != nil && !keep(key)) {
			return nil
		}
		c := counts[key]
		if c == nil {
			c = &count{}
			counts[key] = c
		}
		if clear {
			c.live = 0
			return nil
		}
		c.live++
		c.all++
		return nil
	})
	if err != nil {
		return nil, 0, 0, torn, err
	}
	skip = make(map[string]int, len(counts))
	for key, c := range counts {
		k := min(c.live, j.Max)
		skip[key] = c.all - k
		kept += k
	}
	return skip, n, kept, torn, nil
}

// open counts what is there, once.
func (j *Journal) open() error {
	if j.opened {
		return nil
	}
	_, n, kept, torn, err := j.survey(nil)
	if err != nil {
		return err
	}
	j.n, j.kept, j.torn, j.opened = n, kept, torn, true
	return nil
}

// Read answers the lines kept under key, oldest first: at most Max, those after its last clear.
func (j *Journal) Read(key string) ([][]byte, error) {
	var out [][]byte
	_, err := j.each(func(line []byte) error {
		k, clear, ok := j.Key(line)
		if !ok || k != key {
			return nil
		}
		if clear {
			out = out[:0]
			return nil
		}
		out = append(out, append([]byte(nil), line...))
		if len(out) >= 2*j.Max {
			out = append(out[:0], out[len(out)-j.Max:]...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) > j.Max {
		out = out[len(out)-j.Max:]
	}
	return out, nil
}

// Append adds lines (none of them holds a line end) and rewrites the file when it has grown
// to more than twice what it keeps: then the lines of keys that keep does not take (nil: all
// are taken) are left out for good, with everything a clear voided and what is beyond Max.
func (j *Journal) Append(lines [][]byte, keep func(key string) bool) error {
	if len(lines) == 0 {
		return nil
	}
	if err := j.open(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if j.torn {
		buf.WriteByte('\n')
	}
	for _, l := range lines {
		buf.Write(l)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(j.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	j.written += int64(buf.Len())
	j.torn = false
	j.n += len(lines)
	if j.n <= 2*j.kept+j.Slack {
		return nil
	}
	return j.rewrite(keep)
}

// rewrite writes the file anew with what is kept: into a temporary file beside it, synced,
// renamed over it - a power cut leaves the old file or the new one.
func (j *Journal) rewrite(keep func(key string) bool) error {
	skip, n, kept, _, err := j.survey(keep)
	if err != nil {
		return err
	}
	if kept == n {
		// nothing to leave out (the count of before was of another set of keys)
		j.n, j.kept = n, kept
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(j.Path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	w := bufio.NewWriterSize(tmp, 64<<10)
	var size int64
	if _, err := j.each(func(line []byte) error {
		key, clear, ok := j.Key(line)
		if !ok || clear {
			return nil
		}
		left, taken := skip[key]
		if !taken {
			return nil
		}
		if left > 0 {
			skip[key] = left - 1
			return nil
		}
		size += int64(len(line)) + 1
		if _, err := w.Write(line); err != nil {
			return err
		}
		return w.WriteByte('\n')
	}); err != nil {
		return fail(err)
	}
	if err := w.Flush(); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, j.Path); err != nil {
		os.Remove(name)
		return err
	}
	j.written += size
	j.n, j.kept, j.torn = kept, kept, false
	return nil
}
