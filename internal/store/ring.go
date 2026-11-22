package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
)

// Ring is a file of lines that keeps the newest Max of them: new lines are appended, and once
// the file holds twice as many it is written anew with the newest Max. One writer.
type Ring struct {
	Path string
	Max  int
	// n is the number of lines in the file; torn says the file does not end with a line end
	// (a write cut short): the next line starts on a new one
	n    int
	torn bool
	// written counts the bytes that went to storage
	written int64
}

// Written answers how many bytes went to storage through this ring: the appended lines and
// the files written anew.
func (r *Ring) Written() int64 { return r.written }

// Load answers the newest Max lines, oldest first (none when there is no file yet).
func (r *Ring) Load() ([][]byte, error) {
	b, err := os.ReadFile(r.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.torn = len(b) > 0 && b[len(b)-1] != '\n'
	lines := bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n"))
	if len(b) == 0 {
		lines = nil
	}
	r.n = len(lines)
	if len(lines) > r.Max {
		lines = lines[len(lines)-r.Max:]
	}
	return lines, nil
}

// Append adds lines (none of them holds a line end) and shortens the file when it has grown to
// twice its size.
func (r *Ring) Append(lines [][]byte) error {
	if len(lines) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if r.torn {
		buf.WriteByte('\n')
	}
	for _, l := range lines {
		buf.Write(l)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(r.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
	r.written += int64(buf.Len())
	r.torn = false
	r.n += len(lines)
	if r.n <= 2*r.Max {
		return nil
	}
	kept, err := r.Load()
	if err != nil {
		return err
	}
	whole := append(bytes.Join(kept, []byte("\n")), '\n')
	if err := WriteFile(r.Path, whole, 0o600); err != nil {
		return err
	}
	r.written += int64(len(whole))
	r.n = len(kept)
	return nil
}
