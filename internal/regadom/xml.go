package regadom

import (
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// MaxSize is the largest database Parse reads. A big installation is a few megabytes.
const MaxSize = 64 << 20

// maxSize is MaxSize, lower in a test.
var maxSize int64 = MaxSize

// maxDepth bounds the element nesting; the deepest real path is about twenty elements.
const maxDepth = 100

// ErrTooLarge is returned for input beyond MaxSize.
var ErrTooLarge = errors.New("regadom: the file is larger than 64 MiB")

// latin1Reader turns ISO-8859-1 bytes into UTF-8 for encoding/xml.
type latin1Reader struct {
	r *bufio.Reader
}

func (l *latin1Reader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p)-3 {
		b, err := l.r.ReadByte()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		if b < 0x80 {
			p[n] = b
			n++
		} else {
			p[n] = 0xC0 | b>>6
			p[n+1] = 0x80 | b&0x3F
			n += 2
		}
	}
	return n, nil
}

// printable replaces the control characters XML forbids (everything below U+0020 but tab, line
// feed and carriage return) with a blank. ReGa writes what a script put into a string; one such
// byte in a value must not cost the whole import.
type printable struct {
	r io.Reader
}

func (p printable) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	for i, c := range b[:n] {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			b[i] = ' '
		}
	}
	return n, err
}

// node is one element of the file: its children in order, and for a leaf its text.
type node struct {
	name string
	text string
	kids []*node
}

// child is the first child element of that name, or nil.
func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

// path follows child after child.
func (n *node) path(names ...string) *node {
	for _, name := range names {
		n = n.child(name)
	}
	return n
}

// raw is the text of a child leaf as written (the encoding repaired), "" when it is missing.
func (n *node) raw(name string) string {
	if c := n.child(name); c != nil {
		return fixMojibake(c.text)
	}
	return ""
}

// str is raw without the surrounding white space.
func (n *node) str(name string) string { return strings.TrimSpace(n.raw(name)) }

// num is a child's integer; a missing or malformed one is def.
func (n *node) num(name string, def int64) int64 {
	v, err := strconv.ParseInt(n.str(name), 10, 64)
	if err != nil {
		return def
	}
	return v
}

// oids are the <oid> children of a list element such as <sconds> or <rule-conds>.
func (n *node) oids() []int {
	if n == nil {
		return nil
	}
	var out []int
	for _, k := range n.kids {
		if k.name != "oid" {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(k.text)); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// parseTree reads the whole file into a tree. The character data of an element that has child
// elements is formatting and dropped; a leaf keeps its text as decoded; raw repairs the mixed encoding.
func parseTree(r io.Reader) (*node, error) {
	lr := &io.LimitedReader{R: printable{r}, N: maxSize + 1}
	dec := xml.NewDecoder(lr)
	dec.CharsetReader = func(charset string, in io.Reader) (io.Reader, error) {
		switch strings.ToLower(charset) {
		case "iso-8859-1", "latin1", "iso8859-1":
			return &latin1Reader{r: bufio.NewReader(in)}, nil
		}
		return in, nil
	}
	dec.Strict = false
	root := &node{}
	stack := []*node{root}
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if lr.N <= 0 {
			return nil, ErrTooLarge
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("regadom: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > maxDepth {
				return nil, errors.New("regadom: elements nested too deeply")
			}
			n := &node{name: t.Name.Local}
			top := stack[len(stack)-1]
			top.kids = append(top.kids, n)
			stack = append(stack, n)
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			n := stack[len(stack)-1]
			if len(n.kids) == 0 {
				n.text = text.String()
			}
			text.Reset()
			stack = stack[:len(stack)-1]
		}
	}
	return root, nil
}
