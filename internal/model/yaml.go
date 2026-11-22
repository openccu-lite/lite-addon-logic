package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// The files are a strict subset of YAML: block style only (an empty list is [] and an empty
// mapping {}), no anchors, no aliases, no tags, one document, no key twice, no key the format
// does not know. The writer below is the only one of record; the reader holds a hand-edited file
// to the same subset and names the file and the line of what it refuses.

// Error is a file the reader refuses: where and why.
type Error struct {
	File string
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Msg)
}

// --- writing ---

type emitter struct {
	b strings.Builder
}

// words YAML 1.1 readers take for booleans or null; written quoted so every tool reads a string
var yaml11Words = map[string]bool{"y": true, "n": true, "yes": true, "no": true, "on": true, "off": true, "true": true, "false": true, "null": true, "~": true}

// plain says whether s can be written without quotes and still be read as the same string.
func plain(s string) bool {
	if s == "" || s != strings.TrimSpace(s) || yaml11Words[strings.ToLower(s)] || strings.ContainsAny(s, "\n\r\t") || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 || r == 0xfeff {
			return false
		}
	}
	var n yaml.Node
	if err := yaml.Unmarshal([]byte("k: "+s+"\n"), &n); err != nil {
		return false
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) != 1 || n.Content[0].Kind != yaml.MappingNode || len(n.Content[0].Content) != 2 {
		return false
	}
	v := n.Content[0].Content[1]
	return v.Kind == yaml.ScalarNode && v.ShortTag() == "!!str" && v.Value == s && v.Style == 0 && v.LineComment == ""
}

// quoted writes s double-quoted.
func quoted(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r == 0x85 || r == 0x2028 || r == 0x2029 || r == 0xfeff || r == utf8.RuneError:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// str writes a string scalar: plain where that is safe, else double-quoted.
func str(s string) string {
	if plain(s) {
		return s
	}
	return quoted(s)
}

// number writes a number: an integer without a decimal point, anything else the shortest way
// that reads back as the same number.
func number(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// scalar writes a value: a bool, a number or a string.
func scalar(v any) string {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return number(x)
	case string:
		return str(x)
	}
	return quoted(fmt.Sprint(v))
}

// literal says whether s can be written as a block literal ("|-"): several lines of ordinary
// text, as a kept script is.
func literal(s string) bool {
	if !strings.Contains(s, "\n") || strings.HasSuffix(s, "\n") || strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\n") || !utf8.ValidString(s) {
		return false
	}
	for _, line := range strings.Split(s, "\n") {
		if line != strings.TrimRight(line, " \t") {
			return false
		}
		for _, r := range line {
			if (r < 0x20 && r != '\t') || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029 || r == 0xfeff {
				return false
			}
		}
	}
	return true
}

func (e *emitter) line(indent int, text string) {
	e.b.WriteString(strings.Repeat(" ", indent))
	e.b.WriteString(text)
	e.b.WriteByte('\n')
}

// kv writes "key: value" at an indent; first is "- " for the first key of a list item.
func (e *emitter) kv(indent int, first bool, key, value string) {
	if first {
		e.line(indent-2, "- "+key+": "+value)
		return
	}
	e.line(indent, key+": "+value)
}

// key writes "key:" with what follows on the next lines.
func (e *emitter) key(indent int, first bool, key string) {
	if first {
		e.line(indent-2, "- "+key+":")
		return
	}
	e.line(indent, key+":")
}

// text writes a string value that may have several lines.
func (e *emitter) text(indent int, first bool, key, s string) {
	if literal(s) {
		e.kv(indent, first, key, "|-")
		for _, l := range strings.Split(s, "\n") {
			if l == "" {
				e.b.WriteByte('\n')
				continue
			}
			e.line(indent+2, l)
		}
		return
	}
	e.kv(indent, first, key, str(s))
}

// --- reading ---

type decoder struct {
	file string
}

func (d *decoder) errf(n *yaml.Node, format string, args ...any) *Error {
	line := 0
	if n != nil {
		line = n.Line
	}
	return &Error{File: d.file, Line: line, Msg: fmt.Sprintf(format, args...)}
}

// document parses the bytes and answers the one document's root.
func (d *decoder) document(data []byte) (*yaml.Node, error) {
	if !utf8.Valid(data) {
		return nil, &Error{File: d.file, Msg: "the file is not UTF-8"}
	}
	var doc yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&doc); err != nil {
		line := 0
		msg := err.Error()
		if _, e := fmt.Sscanf(msg, "yaml: line %d:", &line); e == nil {
			msg = strings.TrimSpace(msg[strings.Index(msg, ":")+1:])
			msg = strings.TrimSpace(msg[strings.Index(msg, ":")+1:])
		} else {
			msg = strings.TrimPrefix(msg, "yaml: ")
		}
		if msg == "EOF" {
			msg = "the file is empty"
		}
		return nil, &Error{File: d.file, Line: line, Msg: msg}
	}
	var more yaml.Node
	if err := dec.Decode(&more); err == nil {
		return nil, d.errf(&more, "a second document: a file holds one")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, &Error{File: d.file, Msg: "the file is empty"}
	}
	return doc.Content[0], nil
}

// strict refuses what the subset leaves out.
func (d *decoder) strict(n *yaml.Node) error {
	switch {
	case n.Kind == yaml.AliasNode:
		return d.errf(n, "an alias (*%s): the files do not use anchors", n.Value)
	case n.Anchor != "":
		return d.errf(n, "an anchor (&%s): the files do not use anchors", n.Anchor)
	case n.Style&yaml.TaggedStyle != 0:
		return d.errf(n, "a tag (%s): the files do not use tags", n.Tag)
	case n.Style&yaml.FlowStyle != 0 && len(n.Content) > 0:
		return d.errf(n, "flow style ([…] or {…} with content): write one entry per line")
	}
	return nil
}

// fields is a mapping being read: every key must be asked for, or done refuses it.
type fields struct {
	d     *decoder
	node  *yaml.Node
	keys  []*yaml.Node
	vals  map[string]*yaml.Node
	asked map[string]bool
}

func (d *decoder) mapping(n *yaml.Node, what string) (*fields, error) {
	if err := d.strict(n); err != nil {
		return nil, err
	}
	if n.Kind != yaml.MappingNode {
		return nil, d.errf(n, "%s must be a mapping (key: value lines)", what)
	}
	f := &fields{d: d, node: n, vals: map[string]*yaml.Node{}, asked: map[string]bool{}}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if err := d.strict(k); err != nil {
			return nil, err
		}
		if k.Kind != yaml.ScalarNode {
			return nil, d.errf(k, "a key must be a word")
		}
		if _, dup := f.vals[k.Value]; dup {
			return nil, d.errf(k, "the key %q is there twice", k.Value)
		}
		f.keys = append(f.keys, k)
		f.vals[k.Value] = v
	}
	return f, nil
}

func (f *fields) has(key string) bool { return f.vals[key] != nil }

func (f *fields) get(key string) *yaml.Node {
	f.asked[key] = true
	return f.vals[key]
}

// done refuses a key nobody asked for.
func (f *fields) done(what string, known ...string) error {
	for _, k := range f.keys {
		if !f.asked[k.Value] {
			return f.d.errf(k, "unknown key %q in %s (known: %s)", k.Value, what, strings.Join(known, ", "))
		}
	}
	return nil
}

func (d *decoder) scalarNode(n *yaml.Node, what string) error {
	if err := d.strict(n); err != nil {
		return err
	}
	if n.Kind != yaml.ScalarNode {
		return d.errf(n, "%s must be a single value", what)
	}
	return nil
}

func isString(n *yaml.Node) bool {
	t := n.ShortTag()
	return t == "!!str" || (t == "!!timestamp" && n.Style == 0)
}

// str reads a string; a value YAML takes for something else must be quoted.
func (f *fields) str(key string) (string, error) {
	n := f.get(key)
	if n == nil {
		return "", nil
	}
	if err := f.d.scalarNode(n, key); err != nil {
		return "", err
	}
	if !isString(n) {
		return "", f.d.errf(n, "%s must be text: put %s in quotes", key, n.Value)
	}
	return n.Value, nil
}

// word reads a string that must be one of the given words.
func (f *fields) word(key string, words ...string) (string, error) {
	s, err := f.str(key)
	if err != nil || !f.has(key) {
		return s, err
	}
	for _, w := range words {
		if s == w {
			return s, nil
		}
	}
	return "", f.d.errf(f.vals[key], "%s: %q is not one of %s", key, s, strings.Join(words, ", "))
}

// boolean reads true or false; def when the key is absent.
func (f *fields) boolean(key string, def bool) (bool, error) {
	n := f.get(key)
	if n == nil {
		return def, nil
	}
	if err := f.d.scalarNode(n, key); err != nil {
		return false, err
	}
	if n.ShortTag() != "!!bool" || (n.Value != "true" && n.Value != "false") {
		return false, f.d.errf(n, "%s must be true or false", key)
	}
	return n.Value == "true", nil
}

func (d *decoder) float(n *yaml.Node, what string) (float64, error) {
	if err := d.scalarNode(n, what); err != nil {
		return 0, err
	}
	var v float64
	if t := n.ShortTag(); (t != "!!int" && t != "!!float") || n.Decode(&v) != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, d.errf(n, "%s must be a number", what)
	}
	return v, nil
}

// integer reads a whole number in a range; 0 when the key is absent.
func (f *fields) integer(key string, min, max int) (int, error) {
	n := f.get(key)
	if n == nil {
		return 0, nil
	}
	v, err := f.d.float(n, key)
	if err != nil {
		return 0, err
	}
	if v != math.Trunc(v) || v < float64(min) || v > float64(max) {
		return 0, f.d.errf(n, "%s must be a whole number from %d to %d", key, min, max)
	}
	return int(v), nil
}

// value reads a bool, a number or a string; nil when the key is absent.
func (f *fields) value(key string) (any, error) {
	n := f.get(key)
	if n == nil {
		return nil, nil
	}
	if err := f.d.scalarNode(n, key); err != nil {
		return nil, err
	}
	switch t := n.ShortTag(); {
	case t == "!!bool" && (n.Value == "true" || n.Value == "false"):
		return n.Value == "true", nil
	case t == "!!int" || t == "!!float":
		return f.d.float(n, key)
	case isString(n):
		return n.Value, nil
	}
	return nil, f.d.errf(n, "%s must be true, false, a number or text (an empty text is \"\")", key)
}

// duration reads a duration; 0 when the key is absent.
func (f *fields) duration(key string) (Duration, error) {
	n := f.get(key)
	if n == nil {
		return 0, nil
	}
	if err := f.d.scalarNode(n, key); err != nil {
		return 0, err
	}
	v, err := ParseDuration(n.Value)
	if err != nil {
		return 0, f.d.errf(n, "%s: %v", key, err)
	}
	return v, nil
}

// list answers the items of a sequence; none when the key is absent.
func (f *fields) list(key string) ([]*yaml.Node, error) {
	n := f.get(key)
	if n == nil {
		return nil, nil
	}
	return f.d.sequence(n, key)
}

func (d *decoder) sequence(n *yaml.Node, what string) ([]*yaml.Node, error) {
	if err := d.strict(n); err != nil {
		return nil, err
	}
	if n.Kind != yaml.SequenceNode {
		return nil, d.errf(n, "%s must be a list (lines starting with \"- \"; an empty one is [])", what)
	}
	return n.Content, nil
}

// ref reads a datapoint reference.
func (f *fields) ref(key string) (*Ref, error) {
	s, err := f.str(key)
	if err != nil || !f.has(key) {
		return nil, err
	}
	r, err := ParseRef(s)
	if err != nil {
		return nil, f.d.errf(f.vals[key], "%s: %v", key, err)
	}
	return &r, nil
}
