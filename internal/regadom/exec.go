package regadom

import (
	"net"
	"path"
	"regexp"
	"strings"
)

// What a recognised system.Exec (ExecCall) runs, read as far as the import needs it: a wget
// or curl that only fetches an address, an ether-wake, a line that needs root.

// words splits a command line as a shell would - when it holds nothing but words: quotes and
// backslashes are read, and anything a shell would act on (a pipe, a redirection, a variable,
// a second command) makes it no plain list of words.
func words(line string) ([]string, bool) {
	var out []string
	var cur []byte
	in := false // inside a word
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t':
			if in {
				out, cur, in = append(out, string(cur)), nil, false
			}
		case c == '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, false
			}
			cur, in = append(cur, line[i+1:i+1+end]...), true
			i += end + 1
		case c == '"':
			in = true
			for i++; ; i++ {
				if i >= len(line) {
					return nil, false
				}
				d := line[i]
				if d == '"' {
					break
				}
				if d == '$' || d == '`' {
					return nil, false
				}
				if d == '\\' && i+1 < len(line) && strings.IndexByte("\"\\$`", line[i+1]) >= 0 {
					i++
					d = line[i]
				}
				cur = append(cur, d)
			}
		case c == '\\':
			if i+1 >= len(line) {
				return nil, false
			}
			i++
			cur, in = append(cur, line[i]), true
		case strings.IndexByte("|&;<>()$`\n\r", c) >= 0, c == '#' && !in, c == '~' && !in:
			return nil, false
		default:
			cur, in = append(cur, c), true
		}
	}
	if in {
		out = append(out, string(cur))
	}
	return out, len(out) > 0
}

// flags reads a tool's options: plain names the ones without a value, valued the ones with
// one (short options as their letter, long ones without the dashes). The answer is the
// options with their values in their order, and what is no option.
type option struct{ name, value string }

func flags(args []string, plain, valued string) (opts []option, rest []string, ok bool) {
	has := func(list, name string) bool {
		for _, n := range strings.Fields(list) {
			if n == name {
				return true
			}
		}
		return false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "--") && len(a) > 2:
			name, value, eq := strings.Cut(a[2:], "=")
			switch {
			case has(plain, name) && !eq:
				opts = append(opts, option{name, ""})
			case has(valued, name) && eq:
				opts = append(opts, option{name, value})
			case has(valued, name) && i+1 < len(args):
				i++
				opts = append(opts, option{name, args[i]})
			default:
				return nil, nil, false
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			for j := 1; j < len(a); j++ {
				name := a[j : j+1]
				if has(plain, name) {
					opts = append(opts, option{name, ""})
					continue
				}
				if !has(valued, name) {
					return nil, nil, false
				}
				value := a[j+1:]
				if value == "" {
					if i+1 >= len(args) {
						return nil, nil, false
					}
					i++
					value = args[i]
				}
				opts = append(opts, option{name, value})
				break
			}
		default:
			rest = append(rest, a)
		}
	}
	return opts, rest, true
}

// Fetch is a wget or curl line that only fetches an address.
type Fetch struct {
	Method, URL string
	// Headers are name and value.
	Headers [][2]string
	// Body is sent as it is written; JSON says the line names it as JSON, Typed that the line
	// names another type in a Content-Type header.
	Body        string
	JSON, Typed bool
	Insecure    bool
}

// tool answers the program a command line runs, without its directory, and its arguments.
func (c ExecCall) tool() (string, []string, bool) {
	w, ok := words(c.Command)
	if !ok {
		return "", nil, false
	}
	return path.Base(w[0]), w[1:], true
}

// Fetch reads the command as a wget or curl that only fetches an address; false when it is
// another program or does anything more (a file is written, a login is sent, a pipe, …).
func (c ExecCall) Fetch() (*Fetch, bool) {
	tool, args, ok := c.tool()
	if !ok || (tool != "wget" && tool != "curl") {
		return nil, false
	}
	w := &Fetch{Method: "GET"}
	var data []string
	method := ""
	var opts []option
	var rest []string
	if tool == "wget" {
		opts, rest, ok = flags(args, "q quiet nv no-verbose no-check-certificate 4 no-cache", "O output-document T timeout t tries post-data header U user-agent method")
	} else {
		opts, rest, ok = flags(args, "s silent S show-error k insecure L location f fail g globoff 4 N", "o output m max-time connect-timeout X request d data data-raw data-ascii H header A user-agent url")
	}
	if !ok {
		return nil, false
	}
	for _, o := range opts {
		switch o.name {
		case "no-check-certificate", "k", "insecure":
			w.Insecure = true
		case "O", "output-document", "o", "output":
			if o.value != "-" && o.value != "/dev/null" {
				return nil, false
			}
		case "post-data", "d", "data", "data-raw", "data-ascii":
			if strings.HasPrefix(o.value, "@") && tool == "curl" && o.name != "data-raw" {
				return nil, false
			}
			data = append(data, o.value)
		case "X", "request", "method":
			method = strings.ToUpper(o.value)
		case "header", "H":
			name, value, found := strings.Cut(o.value, ":")
			if !found {
				return nil, false
			}
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if strings.EqualFold(name, "Content-Type") {
				if strings.HasPrefix(strings.ToLower(value), "application/json") {
					w.JSON = true
					continue
				}
				w.Typed = true
			}
			w.Headers = append(w.Headers, [2]string{name, value})
		case "U", "A", "user-agent":
			w.Headers = append(w.Headers, [2]string{"User-Agent", o.value})
		case "url":
			rest = append(rest, o.value)
		}
	}
	if len(rest) != 1 {
		return nil, false
	}
	w.URL = rest[0]
	if !strings.Contains(w.URL, "://") {
		w.URL = "http://" + w.URL
	}
	if len(data) > 0 {
		w.Method, w.Body = "POST", strings.Join(data, "&")
	}
	if method != "" {
		w.Method = method
	}
	return w, true
}

var execMAC = regexp.MustCompile(`^[0-9A-Fa-f]{2}([:-]?[0-9A-Fa-f]{2}){5}$`)

// Wake reads the command as an ether-wake or wakeonlan: the hardware address as written, and
// the IPv4 address the packets go to when the line names one.
func (c ExecCall) Wake() (mac, broadcast string, ok bool) {
	tool, args, ok := c.tool()
	if !ok || (tool != "ether-wake" && tool != "etherwake" && tool != "wakeonlan" && tool != "wol") {
		return "", "", false
	}
	opts, rest, ok := flags(args, "b D", "i p")
	if !ok || len(rest) != 1 || !execMAC.MatchString(rest[0]) {
		return "", "", false
	}
	for _, o := range opts {
		// ether-wake's -i names a network interface, wakeonlan's the address to send to
		if ip := net.ParseIP(o.value); o.name == "i" && tool != "ether-wake" && tool != "etherwake" && ip != nil && ip.To4() != nil {
			broadcast = ip.To4().String()
		}
		if o.name == "p" && o.value != "9" {
			return "", "", false
		}
	}
	return rest[0], broadcast, true
}

// rootish says a command line names something only root could do on a CCU: a path of the
// system, or one of its administration tools.
var rootish = regexp.MustCompile("(^|[\\s>'\"=])/(etc|sys|boot|root|usr/local/etc)/|(^|[\\s;&|/])(reboot|halt|poweroff|mount|umount|killall|kill|monit|crontab|ifconfig|iptables|date\\s+-s|hwclock|insmod|modprobe|chown|chmod|passwd)(\\s|$)")

// NeedsRoot says the command names something only root could do on a CCU.
func (c ExecCall) NeedsRoot() bool {
	return rootish.MatchString(c.Command)
}
