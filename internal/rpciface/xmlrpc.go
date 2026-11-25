package rpciface

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The XML-RPC the interface processes speak, as far as this interface needs it: values are nil
// (an empty string on the wire), bool, int, float64, string, []any and map[string]any. No
// dependency: the codec is two hundred lines.

// MaxBody bounds a request or an answer.
const MaxBody = 4 << 20

// Fault is an XML-RPC fault: what a call is answered with when it cannot be done.
type Fault struct {
	Code int
	Msg  string
}

func (f *Fault) Error() string { return f.Msg }

// Fault codes, as rfd and hmipserver use them.
const (
	FaultGeneral        = -1
	FaultUnknownDevice  = -2
	FaultUnknownSet     = -3
	FaultUnknownParam   = -5
	FaultNotSupported   = -6
	FaultUnknownMethod  = -32601
	FaultInvalidRequest = -32600
)

func fault(code int, format string, args ...any) *Fault {
	return &Fault{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// multicallEntry is one call of a system.multicall: a struct whose member methodName comes
// first and holds its name as a value without a type (`<value>event</value>`), params after it.
// Both forms are a string to XML-RPC; HMIPServer reads an event multicall only in this one: with
// the name typed (`<value><string>event</string></value>`) or params first it answers HTTP 200
// with an empty body, so its subscription was dropped after ten events (B-13).
type multicallEntry struct {
	method string
	params []any
}

func encodeValue(b *bytes.Buffer, v any) {
	if e, ok := v.(multicallEntry); ok {
		b.WriteString("<value><struct><member><name>methodName</name><value>")
		_ = xml.EscapeText(b, []byte(e.method))
		b.WriteString("</value></member><member><name>params</name>")
		encodeValue(b, e.params)
		b.WriteString("</member></struct></value>")
		return
	}
	b.WriteString("<value>")
	switch x := v.(type) {
	case nil:
		b.WriteString("<string></string>")
	case bool:
		if x {
			b.WriteString("<boolean>1</boolean>")
		} else {
			b.WriteString("<boolean>0</boolean>")
		}
	case int:
		fmt.Fprintf(b, "<i4>%d</i4>", x)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			x = 0
		}
		b.WriteString("<double>" + strconv.FormatFloat(x, 'f', -1, 64) + "</double>")
	case string:
		b.WriteString("<string>")
		_ = xml.EscapeText(b, []byte(x))
		b.WriteString("</string>")
	case []any:
		b.WriteString("<array><data>")
		for _, e := range x {
			encodeValue(b, e)
		}
		b.WriteString("</data></array>")
	case []string:
		b.WriteString("<array><data>")
		for _, e := range x {
			encodeValue(b, e)
		}
		b.WriteString("</data></array>")
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("<struct>")
		for _, k := range keys {
			b.WriteString("<member><name>")
			_ = xml.EscapeText(b, []byte(k))
			b.WriteString("</name>")
			encodeValue(b, x[k])
			b.WriteString("</member>")
		}
		b.WriteString("</struct>")
	default:
		b.WriteString("<string>")
		_ = xml.EscapeText(b, []byte(fmt.Sprint(x)))
		b.WriteString("</string>")
	}
	b.WriteString("</value>")
}

// EncodeCall writes a method call.
func EncodeCall(method string, params ...any) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodCall><methodName>`)
	_ = xml.EscapeText(&b, []byte(method))
	b.WriteString("</methodName><params>")
	for _, p := range params {
		b.WriteString("<param>")
		encodeValue(&b, p)
		b.WriteString("</param>")
	}
	b.WriteString("</params></methodCall>")
	return b.Bytes()
}

// EncodeResponse writes an answer: the result, or the fault.
func EncodeResponse(result any, f *Fault) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodResponse>`)
	if f != nil {
		b.WriteString("<fault>")
		encodeValue(&b, map[string]any{"faultCode": f.Code, "faultString": f.Msg})
		b.WriteString("</fault>")
	} else {
		b.WriteString("<params><param>")
		encodeValue(&b, result)
		b.WriteString("</param></params>")
	}
	b.WriteString("</methodResponse>")
	return b.Bytes()
}

// node is an element with its children and text, enough to read a call.
type node struct {
	XMLName xml.Name
	Text    string `xml:",chardata"`
	Kids    []node `xml:",any"`
}

func (n *node) kid(name string) *node {
	for i := range n.Kids {
		if n.Kids[i].XMLName.Local == name {
			return &n.Kids[i]
		}
	}
	return nil
}

var errValue = errors.New("xmlrpc: a value that is none")

func decodeValue(n *node, depth int) (any, error) {
	if n == nil || n.XMLName.Local != "value" || depth > 32 {
		return nil, errValue
	}
	if len(n.Kids) == 0 {
		return n.Text, nil // a value without a type is a string
	}
	t := &n.Kids[0]
	text := strings.TrimSpace(t.Text)
	switch t.XMLName.Local {
	case "string", "base64", "dateTime.iso8601":
		return t.Text, nil
	case "boolean":
		return text == "1" || text == "true", nil
	case "i4", "int", "i8":
		i, err := strconv.Atoi(text)
		if err != nil {
			return nil, errValue
		}
		return i, nil
	case "double":
		f, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errValue
		}
		return f, nil
	case "nil":
		return nil, nil
	case "array":
		out := []any{}
		if data := t.kid("data"); data != nil {
			for i := range data.Kids {
				v, err := decodeValue(&data.Kids[i], depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
		}
		return out, nil
	case "struct":
		out := map[string]any{}
		for i := range t.Kids {
			m := &t.Kids[i]
			name := m.kid("name")
			if m.XMLName.Local != "member" || name == nil {
				return nil, errValue
			}
			v, err := decodeValue(m.kid("value"), depth+1)
			if err != nil {
				return nil, err
			}
			out[name.Text] = v
		}
		return out, nil
	}
	return nil, errValue
}

func parse(data []byte, root string) (*node, error) {
	var n node
	dec := xml.NewDecoder(bytes.NewReader(data))
	// the system's own XML-RPC client, like the interface processes, declares ISO-8859-1
	dec.CharsetReader = charsetReader
	if err := dec.Decode(&n); err != nil {
		return nil, err
	}
	if n.XMLName.Local != root {
		return nil, fmt.Errorf("xmlrpc: not a %s", root)
	}
	return &n, nil
}

func params(n *node) ([]any, error) {
	var out []any
	if ps := n.kid("params"); ps != nil {
		for i := range ps.Kids {
			v, err := decodeValue(ps.Kids[i].kid("value"), 0)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// DecodeCall reads a method call.
func DecodeCall(data []byte) (method string, args []any, err error) {
	n, err := parse(data, "methodCall")
	if err != nil {
		return "", nil, err
	}
	name := n.kid("methodName")
	if name == nil || strings.TrimSpace(name.Text) == "" {
		return "", nil, errors.New("xmlrpc: a call without a method")
	}
	args, err = params(n)
	return strings.TrimSpace(name.Text), args, err
}

// DecodeResponse reads an answer; a fault comes back as the error.
func DecodeResponse(data []byte) (any, error) {
	n, err := parse(data, "methodResponse")
	if err != nil {
		return nil, err
	}
	if f := n.kid("fault"); f != nil {
		v, err := decodeValue(f.kid("value"), 0)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		code, _ := m["faultCode"].(int)
		msg, _ := m["faultString"].(string)
		return nil, &Fault{Code: code, Msg: msg}
	}
	ps, err := params(n)
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, nil
	}
	return ps[0], nil
}

// Call sends a method call to an XML-RPC server and answers its result.
func Call(ctx context.Context, client *http.Client, url, method string, args ...any) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(EncodeCall(method, args...)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBody))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("xmlrpc: %s answered %s", method, res.Status)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		// no methodResponse is no answer (the spec has none without one): counted as a failure
		return nil, fmt.Errorf("xmlrpc: %s answered with an empty body", method)
	}
	return DecodeResponse(body)
}

// charsetReader lets the XML decoder read what declares another encoding than UTF-8: Latin-1
// (ISO-8859-1, which the system's XML-RPC client and the interface processes declare) is turned
// into UTF-8; anything else that is not UTF-8 or ASCII is refused.
func charsetReader(charset string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return in, nil
	case "iso-8859-1", "iso8859-1", "latin1", "latin-1", "iso-8859-15", "windows-1252":
		raw, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		out := make([]rune, len(raw))
		for i, c := range raw {
			out[i] = rune(c)
		}
		return strings.NewReader(string(out)), nil
	}
	return nil, fmt.Errorf("the encoding %q is not supported", charset)
}
