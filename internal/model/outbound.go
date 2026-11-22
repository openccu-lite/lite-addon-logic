package model

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// What a program does beyond the system: a call to a web address (a webhook) and a
// Wake-on-LAN. Neither is a script: a webhook is a form - method, address, headers, a body -
// with a fixed set of placeholders, and its answer is never read.

// The methods and body types a webhook has.
var (
	WebhookMethods   = []string{"GET", "POST", "PUT"}
	WebhookBodyTypes = []string{BodyJSON, BodyForm, BodyText}
)

// Body types: how the body's placeholders are escaped, and its Content-Type.
const (
	BodyJSON = "json"
	BodyForm = "form"
	BodyText = "text"
)

// Limits of a webhook.
const (
	MaxHeaders = 8
	MaxURL     = 2000
	MaxHeader  = 1000
	MaxBody    = 16 << 10
)

// Webhook is ActionWebhook's request.
type Webhook struct {
	// Method: GET, POST or PUT.
	Method string `json:"method"`
	// URL is an http or https address; placeholders in it are escaped for an address.
	URL     string   `json:"url"`
	Headers []Header `json:"headers,omitempty"`
	// Body is sent with POST and PUT; BodyType says how: json, form or text.
	BodyType string `json:"body_type,omitempty"`
	Body     string `json:"body,omitempty"`
	// Insecure: the server's certificate is not verified (a device on the LAN that signs its own).
	Insecure bool `json:"insecure,omitempty"`
}

// Header is one request header.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Wake is ActionWake's target.
type Wake struct {
	// MAC is the hardware address, written AA:BB:CC:DD:EE:FF.
	MAC string `json:"mac"`
	// Broadcast is the IPv4 address the magic packets go to ("" is 255.255.255.255, the
	// local network; a subnet's broadcast address reaches another one).
	Broadcast string `json:"broadcast,omitempty"`
}

var headerName = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+.^_`|~-]{1,64}$")

// headers the request's transport sets itself
var ownHeaders = map[string]bool{"host": true, "content-length": true, "transfer-encoding": true, "connection": true}

// Check says what is wrong with a webhook ("" when nothing is).
func (w *Webhook) Check() string {
	ok := false
	for _, m := range WebhookMethods {
		ok = ok || w.Method == m
	}
	if !ok {
		return fmt.Sprintf("method: %q is not one of %s", w.Method, strings.Join(WebhookMethods, ", "))
	}
	if msg := checkURL(w.URL); msg != "" {
		return msg
	}
	if len(w.Headers) > MaxHeaders {
		return fmt.Sprintf("a webhook has at most %d headers", MaxHeaders)
	}
	seen := map[string]bool{}
	for _, h := range w.Headers {
		lower := strings.ToLower(h.Name)
		switch {
		case !headerName.MatchString(h.Name):
			return fmt.Sprintf("%q is not a header's name", h.Name)
		case ownHeaders[lower]:
			return fmt.Sprintf("the header %s is set by the request itself", h.Name)
		case seen[lower]:
			return fmt.Sprintf("the header %s is there twice", h.Name)
		case len(h.Value) > MaxHeader || strings.ContainsAny(h.Value, "\r\n\x00"):
			return fmt.Sprintf("the value of the header %s is longer than %d bytes or has a line break", h.Name, MaxHeader)
		}
		seen[lower] = true
	}
	if w.Body == "" {
		if w.BodyType != "" {
			return "body_type: only a webhook with a body has it"
		}
		return ""
	}
	if w.Method == "GET" {
		return "a GET has no body"
	}
	if len(w.Body) > MaxBody {
		return fmt.Sprintf("the body is longer than %d bytes", MaxBody)
	}
	ok = false
	for _, t := range WebhookBodyTypes {
		ok = ok || w.BodyType == t
	}
	if !ok {
		return fmt.Sprintf("body_type: %q is not one of %s", w.BodyType, strings.Join(WebhookBodyTypes, ", "))
	}
	return ""
}

// checkURL says what is wrong with a webhook's address. Placeholders stand for text that is
// escaped when it is put in, so the address is checked with a letter in their place.
func checkURL(s string) string {
	if s == "" || len(s) > MaxURL || strings.ContainsAny(s, " \t\r\n") {
		return fmt.Sprintf("the address is empty, longer than %d characters or has a blank in it", MaxURL)
	}
	u, err := url.Parse(placeholder.ReplaceAllString(s, "x"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "the address must be http://… or https://… with a host, and without a user name or a fragment"
	}
	authority := s[strings.Index(s, "://")+3:]
	if i := strings.IndexAny(authority, "/?#"); i >= 0 {
		authority = authority[:i]
	}
	if placeholder.MatchString(authority) {
		return "a placeholder cannot stand in the address' host"
	}
	return ""
}

// CanonicalMAC answers the MAC address written AA:BB:CC:DD:EE:FF ("" when s is none): the six
// bytes with colons, hyphens or nothing between them.
func CanonicalMAC(s string) string {
	hex := strings.ToUpper(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(s)))
	if len(hex) != 12 || strings.Trim(hex, "0123456789ABCDEF") != "" {
		return ""
	}
	if seps := strings.Count(s, ":") + strings.Count(s, "-"); seps != 0 && seps != 5 {
		return ""
	}
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = hex[2*i : 2*i+2]
	}
	return strings.Join(parts, ":")
}

// Check says what is wrong with a Wake-on-LAN ("" when nothing is).
func (w *Wake) Check() string {
	if w.MAC == "" || CanonicalMAC(w.MAC) != w.MAC {
		return fmt.Sprintf("wake: %q is not a hardware address (AA:BB:CC:DD:EE:FF)", w.MAC)
	}
	if w.Broadcast != "" {
		if ip := net.ParseIP(w.Broadcast); ip == nil || ip.To4() == nil || ip.To4().String() != w.Broadcast {
			return fmt.Sprintf("broadcast: %q is not an IPv4 address", w.Broadcast)
		}
	}
	return ""
}

// The placeholders of a webhook - a fixed set, nothing else in braces is touched:
//
//	{program}        the program's name
//	{trigger}        the name of what triggered the run
//	{value}          the value the trigger brought
//	{time}           the time of the run, 2026-11-22T16:30:00+01:00
//	{var:<key>}      the value of a variable
//	{secret:<name>}  a secret from the settings
var placeholder = regexp.MustCompile(`\{(program|trigger|value|time|var:[a-z0-9-]+|secret:[a-z0-9-]+)\}`)

// Expand puts the placeholders' values into s: fill answers the text for a placeholder's name
// ("program", "var:<key>", …) and escape makes it fit where it goes.
func Expand(s string, fill func(name string) string, escape func(string) string) string {
	return placeholder.ReplaceAllStringFunc(s, func(m string) string {
		return escape(fill(m[1 : len(m)-1]))
	})
}

// Placeholders lists the placeholders' names in everything a webhook sends, each once, in the
// order they come.
func (w *Webhook) Placeholders() []string {
	var out []string
	seen := map[string]bool{}
	texts := []string{w.URL}
	for _, h := range w.Headers {
		texts = append(texts, h.Value)
	}
	for _, t := range append(texts, w.Body) {
		for _, m := range placeholder.FindAllStringSubmatch(t, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	return out
}

// uses says whether the webhook names a variable in a placeholder.
func (w *Webhook) uses(variable string) bool {
	for _, p := range w.Placeholders() {
		if p == "var:"+variable {
			return true
		}
	}
	return false
}
