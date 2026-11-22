package regadom

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// The anonymiser makes a database that can be shown to others out of a real one: the same
// objects with the same ids, types, flags, conditions, time modules and numbers, so that it
// imports exactly as the original does - and none of what was typed or what names a home.
//
// The rule is "blank unless known harmless": every text is dropped except
//
//   - numbers (ids, types, flags, times, values of number and list variables);
//   - what the firmware wrote and the import reads: datapoint names (STATE), device and channel
//     types (HM-LC-Sw1-Pl), the interfaces' names, the names of built-in objects, the ${…} keys
//     of the WebUI's texts, the names of eQ-3's counter variables and programs;
//   - names, which become "Programm 1", "Variable 2", "Raum 3" … - the same name the same
//     pseudonym, so that two objects of one name stay two objects of one name;
//   - serial numbers in addresses, which are numbered anew with their kind kept
//     (MEQ0123456:1 becomes MEQ0000001:1);
//   - texts a variable holds or a program compares and writes ("Text 1"), entries of value lists
//     ("Wert 1"), descriptions ("Beschreibung");
//   - scripts: the two one-liners the import knows are written anew with the pseudonym, one
//     that is a single system.Exec as a command of its kind without its words, every
//     other script becomes a comment that says how long it was.
//
// Passwords, their hashes, mail, phone, the location, the serial number of the CCU and the
// devices' descriptions (radio addresses, firmware) are dropped. The result is for the import
// of this addon, not for a CCU: ReGa would miss its settings.

// Anonymised says what Anonymise did.
type Anonymised struct {
	// Kind and Firmware are the source's ("sbk", "3.89.8.20260719").
	Kind     string
	Firmware string
	// Names, Addresses, Texts and Scripts count what was replaced, Dropped what was blanked.
	Names, Addresses, Texts, Scripts, Dropped int
}

// Anonymise reads a backup or a bare database and writes the anonymised database.
func Anonymise(r io.Reader, w io.Writer) (*Anonymised, error) {
	src, err := read(r, true)
	if err != nil {
		return nil, err
	}
	a := &anon{out: &Anonymised{Kind: src.Kind, Firmware: src.Firmware}, names: map[string]string{}, perKind: map[string]int{},
		serials: map[string]string{}, texts: map[string]string{}, kept: map[*node]bool{}}
	a.objects(src.root, nil)
	a.leaves(src.root, nil)
	bw := bufio.NewWriter(w)
	bw.WriteString(`<?xml version="1.0" encoding="iso-8859-1" ?>` + "\n")
	for _, k := range src.root.kids {
		writeNode(bw, k)
	}
	return a.out, bw.Flush()
}

type anon struct {
	out     *Anonymised
	names   map[string]string // kind + "\x00" + name -> pseudonym
	perKind map[string]int
	serials map[string]string
	texts   map[string]string
	kept    map[*node]bool // <obj> elements whose name stayed
}

// The pseudonym's word for an object, by the element that holds its <obj> (for a datapoint, the
// element around its <dp>).
var kindWords = map[string]string{
	"program": "Programm", "var-dp": "Variable", "alarm-dp": "Alarm", "device": "Gerät", "channel": "Kanal",
	"user": "Benutzer", "hss-dp": "Datenpunkt", "enum": "Liste", "ifc": "Schnittstelle",
}

// kind answers the word for an object. "Objekt" is everything ReGa names itself: rules,
// conditions, activities, the timers of time modules.
func kind(parents []*node) string {
	at := len(parents) - 1
	if at > 0 && parents[at].name == "dp" {
		at--
	}
	if at < 0 {
		return "Objekt"
	}
	p := parents[at]
	w, ok := kindWords[p.name]
	if !ok {
		return "Objekt"
	}
	if p.name == "enum" {
		switch {
		case at > 0 && parents[at-1].name == "favorite":
			return "Favorit"
		case p.str("entype") == "1":
			return "Raum"
		case p.str("entype") == "3":
			return "Gewerk"
		}
	}
	return w
}

var (
	numeric     = regexp.MustCompile(`^-?[0-9]+([.,][0-9]+)?([eE][-+]?[0-9]+)?$`)
	placeholder = regexp.MustCompile(`^(\$\{[A-Za-z0-9_]+\})+$`)
	harmless    = regexp.MustCompile(`^[A-Za-z0-9_. +:/%-]{1,48}$`)
	counterVar  = regexp.MustCompile(`^sv(EnergyCounter|HmIPRainCounter|HmIPSunshineCounter|Counter)[A-Za-z]*_[0-9]+(_[A-Za-z0-9-]+:[0-9]+)?$`)
	counterPrg  = regexp.MustCompile(`^prg(EnergyCounter|GasCounter|RainCounter|SunshineCounter|FlowCounter|DailySunshineRainCounter|SetEnergyValuesAtMidnight|SetCounterAtMidnight)[A-Za-z]*(_[0-9]+)?(_[A-Za-z0-9-]+:[0-9]+)?$`)
	systemName  = regexp.MustCompile(`^(DutyCycle(-[A-Za-z0-9-]+)?|WatchDog: [a-z0-9-]+|(AL-|[A-Za-z0-9-]+\.)[A-Za-z0-9_*-]+(:[0-9]+)?\.[A-Z0-9_]+)$`)
	// what ReGa calls the objects it makes itself
	ownName = regexp.MustCompile(`^(DOM|Root|PrgTimerDp|Zeitmodul|AlarmCondition|MapCondition|PrgCnd|PrgMapDp|HM_AUTOCONF|PrgRule|SubRule|RuleDestination|History data point|(Condition|SC|SD)[0-9]*|Gateway configuration|Runtime configuration|WEB configuration|Gateway-IPDP|Eventing|ValueEventing|SMTPServer|POPClient)$`)
	// serial numbers: BidCos and Wired (three letters, seven digits), HmIP (fourteen hex digits),
	// the 24 digits of an SGTIN
	serial     = regexp.MustCompile(`[0-9A-F]{24}|[0-9A-F]{14}|[A-Z]{3}[0-9]{7}`)
	special    = regexp.MustCompile(`^\[SV\][A-Za-z0-9_ ]*$`)
	anonymised = regexp.MustCompile(`^! anonymised script \(lines: [0-9]+, characters: [0-9]+\)$`)
	ifcURL     = regexp.MustCompile(`^([a-z_]+)://[^/:]+:([0-9]+)((?:/[A-Za-z0-9_-]+)*)$`)
	clock      = regexp.MustCompile(`^[0-9]+:[0-9]+:[0-9]+$`)
)

// firstDynamic is the first id ReGa gives an object somebody made; below it are its built-ins.
const firstDynamic = 1000

// address numbers the serials in a text anew. Addresses of CUxD and of the virtual group
// devices are the user's own numbering of nothing and stay.
func (a *anon) address(s string) string {
	return serial.ReplaceAllStringFunc(s, func(m string) string {
		if strings.HasPrefix(m, "CUX") || strings.HasPrefix(m, "INT") {
			return m
		}
		if r, ok := a.serials[m]; ok {
			return r
		}
		a.out.Addresses++
		n := a.out.Addresses
		var r string
		switch len(m) {
		case 10:
			r = m[:3] + fmt.Sprintf("%07d", n)
		case 14:
			r = m[:4] + fmt.Sprintf("%010d", n)
		default:
			r = m[:10] + fmt.Sprintf("%014d", n)
		}
		a.serials[m] = r
		a.serials[r] = r // an anonymised file stays as it is
		return r
	})
}

// pseudonym answers the name that stands for a name of a kind.
func (a *anon) pseudonym(kind, name string) string {
	key := kind + "\x00" + name
	if p, ok := a.names[key]; ok {
		return p
	}
	a.out.Names++
	a.perKind[kind]++
	p := kind + " " + strconv.Itoa(a.perKind[kind])
	a.names[key] = p
	return p
}

// text answers what stands for a text a variable holds or a program compares or writes.
func (a *anon) text(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	if t, ok := a.texts[s]; ok {
		return t
	}
	a.out.Texts++
	t := "Text " + strconv.Itoa(len(a.texts)+1)
	a.texts[s] = t
	return t
}

func (a *anon) drop(n *node) {
	if n.text != "" {
		a.out.Dropped++
	}
	n.text = ""
}

// objects is the first pass: every object's name and metadata.
func (a *anon) objects(n *node, parents []*node) {
	if n.name == "obj" {
		a.object(n, kind(parents))
		return
	}
	parents = append(parents, n)
	for _, k := range n.kids {
		a.objects(k, parents)
	}
}

func (a *anon) object(o *node, kind string) {
	id := int(o.num("id", firstDynamic))
	name := o.child("name")
	keep := true
	if name != nil {
		was := strings.TrimSpace(cleanName(name.text))
		switch {
		case was == "" || id < firstDynamic || kind == "Schnittstelle" || placeholder.MatchString(was):
		case kind == "Datenpunkt" && systemName.MatchString(was),
			(kind == "Variable" || kind == "Alarm") && (counterVar.MatchString(was) || systemName.MatchString(was)),
			kind == "Programm" && counterPrg.MatchString(was):
			name.text = a.address(was)
		case kind == "Objekt" && ownName.MatchString(was):
		default:
			name.text = a.pseudonym(kind, was)
			keep = false
		}
	}
	a.kept[o] = keep
	md := o.child("metadata")
	if md == nil {
		return
	}
	prop := ""
	for _, k := range md.kids {
		switch k.name {
		case "property":
			prop = strings.TrimSpace(k.text)
			if !harmless.MatchString(prop) {
				prop = ""
				a.drop(k)
			}
		case "value":
			a.meta(k, prop, kind)
			prop = ""
		}
	}
}

// Metadata whose values are the firmware's own short words.
var plainMeta = map[string]bool{"MIN": true, "MAX": true, "TYPE": true, "CONTROL": true, "TAB_ORDER": true, "LINKCOUNT": true,
	"operateGroupOnly": true, "AUTOCONF": true, "inHeatingGroup": true, "pos": true, "channelMode": true, "pwdSet": true}

// meta rewrites one metadata value. The value names and lists of a variable are somebody's
// words unless they are the WebUI's keys; the unit is kept (a short one), the devices' descriptions go.
func (a *anon) meta(v *node, prop string, kind string) {
	was := fixMojibake(v.text)
	own := (kind == "Variable" || kind == "Alarm") && !placeholders(was)
	switch {
	case was == "" || numeric.MatchString(was):
	case plainMeta[prop] && harmless.MatchString(was):
	case prop == "UNIT" && len([]rune(was)) <= 8:
		v.text = was
	case prop == "BV0" || prop == "BV1" || prop == "VALUE_LIST":
		if !own {
			if !valueWords(was) {
				a.drop(v)
			}
			return
		}
		a.out.Texts++
		if prop != "VALUE_LIST" {
			v.text = "Wert " + prop[2:]
			return
		}
		seen := map[string]string{}
		parts := strings.Split(was, ";")
		for i, p := range parts {
			t := strings.TrimSpace(p)
			if t == "" || placeholder.MatchString(t) {
				continue
			}
			if seen[t] == "" {
				seen[t] = "Wert " + strconv.Itoa(len(seen)+1)
			}
			parts[i] = seen[t]
		}
		v.text = strings.Join(parts, ";")
	default:
		a.drop(v)
	}
}

// placeholders says every entry of a value list is one of the WebUI's ${…} keys, or empty.
func placeholders(s string) bool {
	for _, p := range strings.Split(s, ";") {
		if p = strings.TrimSpace(p); p != "" && !placeholder.MatchString(p) {
			return false
		}
	}
	return true
}

// valueWords says a value list is the firmware's: its entries are plain words or ${…} keys.
func valueWords(s string) bool {
	for _, p := range strings.Split(s, ";") {
		if p != "" && !harmless.MatchString(p) && !placeholder.MatchString(p) {
			return false
		}
	}
	return true
}

// Leaves that go whatever they hold - a number, too.
var secrets = map[string]bool{"pwsha512": true, "pwmd5": true, "password": true, "pwd": true, "pop3pwd": true, "pop3name": true,
	"pop3svr": true, "phone": true, "mail": true, "firstname": true, "latitude": true, "longitude": true, "location": true,
	"serial-number": true, "knx-ind-addr": true, "ir-address": true, "register-url": true, "register-sa": true, "register-ns": true}

// leaves is the second pass: everything that is not an object's header.
func (a *anon) leaves(n *node, parents []*node) {
	if n.name == "obj" {
		return
	}
	if len(n.kids) > 0 {
		parents = append(parents, n)
		for _, k := range n.kids {
			a.leaves(k, parents)
		}
		return
	}
	var parent, grand *node
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	}
	if len(parents) > 1 {
		grand = parents[len(parents)-2]
	}
	was := n.text
	switch {
	case secrets[n.name]:
		a.drop(n)
	case n.name == "var":
		// a variable's value: a text when the variable holds texts, else a number
		if parent.str("varType") == "20" || !numeric.MatchString(strings.TrimSpace(was)) {
			n.text = a.text(was)
		}
	case n.name == "val":
		a.value(n, parent, grand)
	case strings.TrimSpace(was) == "" || numeric.MatchString(strings.TrimSpace(was)):
	case n.name == "hss-id" || n.name == "devlabel" || n.name == "chnlabel" || n.name == "ifc-info":
		if !harmless.MatchString(was) {
			a.drop(n)
		}
	case n.name == "hss-adr" || n.name == "chnadr" || n.name == "devadr":
		n.text = a.address(was)
		if !harmless.MatchString(n.text) {
			a.drop(n)
		}
	case n.name == "ifc-url":
		m := ifcURL.FindStringSubmatch(strings.TrimSpace(was))
		if m == nil {
			a.drop(n)
			return
		}
		n.text = m[1] + "://127.0.0.1:" + m[2] + m[3]
	case n.name == "dp-info" || n.name == "prginfo" || n.name == "eninfo":
		if placeholder.MatchString(strings.TrimSpace(was)) || (a.kept[parent.child("obj")] && harmless.MatchString(was)) {
			return
		}
		a.out.Texts++
		n.text = "Beschreibung"
	default:
		a.drop(n)
	}
}

// value rewrites a <val>: what a line compares with, what an activity writes, a default.
func (a *anon) value(n, parent, grand *node) {
	was := n.text
	trimmed := strings.TrimSpace(was)
	script := parent != nil && parent.name == "trgval" && grand != nil && grand.str("param") == "20"
	switch {
	case script:
		a.out.Scripts++
		n.text = a.script(was)
	case trimmed == "" || numeric.MatchString(trimmed) || clock.MatchString(trimmed) || special.MatchString(trimmed):
	default:
		n.text = a.text(was)
	}
}

// script writes the two known one-liners anew and turns any other script into a comment.
func (a *anon) script(s string) string {
	name, id, call, ok := OneLiner(s)
	if !ok && anonymised.MatchString(s) {
		return s
	}
	if exec, isExec := Exec(s); !ok && isExec {
		return a.exec(exec)
	}
	if !ok {
		return fmt.Sprintf("! anonymised script (lines: %d, characters: %d)", strings.Count(strings.TrimSpace(s), "\n")+1, len([]rune(s)))
	}
	target := strconv.Itoa(id)
	if name != "" || id == 0 {
		target = `"` + a.pseudonym("Programm", strings.TrimSpace(cleanName(name))) + `"`
	}
	switch call {
	case CallRun:
		return "dom.GetObject(" + target + ").ProgramExecute();"
	case CallActivate:
		return "dom.GetObject(" + target + ").Active(true);"
	}
	return "dom.GetObject(" + target + ").Active(false);"
}

// writeNode writes an element as ReGa does: one per line, a leaf with its text on its line.
func writeNode(w *bufio.Writer, n *node) {
	w.WriteString("<" + n.name + ">")
	if len(n.kids) == 0 {
		escape(w, n.text)
	} else {
		w.WriteString("\n")
		for _, k := range n.kids {
			writeNode(w, k)
		}
	}
	w.WriteString("</" + n.name + ">\n")
}

// escape writes a text for a file that says it is ISO-8859-1: everything beyond ASCII as a
// character reference.
func escape(w *bufio.Writer, s string) {
	for _, r := range s {
		switch {
		case r == '&':
			w.WriteString("&amp;")
		case r == '<':
			w.WriteString("&lt;")
		case r == '>':
			w.WriteString("&gt;")
		case r == '"':
			w.WriteString("&quot;")
		case r == '\n' || r == '\t' || (r >= 0x20 && r < 0x7f):
			w.WriteRune(r)
		default:
			fmt.Fprintf(w, "&#%d;", r)
		}
	}
}

// exec writes a script that is one system.Exec anew: a command of the same kind - one that
// fetches an address, one that wakes a machine, one that needs root or does not - with nothing
// of the original in it, and the variable its output goes to by its pseudonym.
func (a *anon) exec(c ExecCall) string {
	if !c.Output {
		if _, ok := c.Fetch(); ok {
			return `system.Exec("wget -q -O - http://anonymised.invalid/");`
		}
		if _, _, ok := c.Wake(); ok {
			return `system.Exec("ether-wake 02:00:00:00:00:01");`
		}
	}
	command := "anonymised-command"
	if c.NeedsRoot() {
		command = "/etc/anonymised-command"
	}
	if !c.Output {
		return `system.Exec("` + command + `");`
	}
	target := strconv.Itoa(c.OutID)
	if c.OutName != "" {
		target = `"` + a.pseudonym("Variable", strings.TrimSpace(cleanName(c.OutName))) + `"`
	}
	return `string out; string err; system.Exec("` + command + `", &out, &err); dom.GetObject(` + target + `).State(out);`
}
