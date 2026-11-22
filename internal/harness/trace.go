package harness

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entry is one marker written at a time; Trace what a side did, in order.
type Entry struct {
	Time time.Time
	Mark string
}

// Trace is the list of entries.
type Trace []Entry

// Lines writes a trace as text, "2006-01-02 15:04:05 <mark>" per entry.
func (t Trace) Lines() []string {
	out := make([]string, len(t))
	for i, e := range t {
		out[i] = e.Time.In(Zone).Format(TimeLayout) + " " + e.Mark
	}
	return out
}

// parseWall reads a wall-clock time in Zone; of the two instants a time in the repeated hour of
// an autumn night can mean, it answers the first.
func parseWall(s string) (time.Time, error) {
	t, err := time.ParseInLocation(TimeLayout, s, Zone)
	if err != nil {
		return t, err
	}
	if earlier := t.Add(-time.Hour); earlier.Format(TimeLayout) == s {
		return earlier, nil
	}
	return t, nil
}

// ParseLines reads what Lines wrote.
func ParseLines(lines []string) (Trace, error) {
	var out Trace
	for _, l := range lines {
		if len(l) < len(TimeLayout)+2 {
			return nil, fmt.Errorf("trace line %q", l)
		}
		at, err := parseWall(l[:len(TimeLayout)])
		if err != nil {
			return nil, fmt.Errorf("trace line %q: %v", l, err)
		}
		out = append(out, Entry{Time: at, Mark: l[len(TimeLayout)+1:]})
	}
	return out, nil
}

var tracerEntry = regexp.MustCompile(`(\S+)@(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) `)

// ParseTracer reads the tracer program's notes ("zzM_<mark>@<time> " one after the other; the
// recorder's heartbeats among them are left out). The
// reference writes wall-clock times without the offset: in the repeated hour of an autumn night
// a time that is earlier than the one before it is the second pass.
func ParseTracer(raw string) (Trace, error) {
	var out Trace
	var last time.Time
	rest := raw
	for rest != "" {
		m := tracerEntry.FindStringSubmatchIndex(rest)
		if m == nil || m[0] != 0 {
			return nil, fmt.Errorf("the tracer's note %q is not <marker>@<time>", rest)
		}
		name := rest[m[2]:m[3]]
		if name != BeatVar && !strings.HasPrefix(name, MarkerPrefix) {
			return nil, fmt.Errorf("the tracer noted %q, which is no marker", name)
		}
		at, err := parseWall(rest[m[4]:m[5]])
		if err != nil {
			return nil, err
		}
		if at.Before(last) && last.Sub(at) <= time.Hour {
			at = at.Add(time.Hour)
		}
		last = at
		if name != BeatVar {
			out = append(out, Entry{Time: at, Mark: name[len(MarkerPrefix):]})
		}
		rest = rest[m[1]:]
	}
	return out, nil
}

// Compare says how got differs from want: the same markers in the same order, each within slack
// of its time. Entries of the same instant (within slack) may come in another order.
func Compare(want, got Trace, slack time.Duration) error {
	if len(want) != len(got) {
		return fmt.Errorf("%d entries, want %d", len(got), len(want))
	}
	w, g := normal(want, slack), normal(got, slack)
	for i := range w {
		d := g[i].Time.Sub(w[i].Time)
		if d < 0 {
			d = -d
		}
		if w[i].Mark != g[i].Mark || d > slack {
			return fmt.Errorf("entry %d is %q, want %q", i+1, Trace{g[i]}.Lines()[0], Trace{w[i]}.Lines()[0])
		}
	}
	return nil
}

// normal sorts the entries of one instant by marker.
func normal(t Trace, slack time.Duration) Trace {
	out := append(Trace(nil), t...)
	for i := 0; i < len(out); {
		j := i + 1
		for j < len(out) && out[j].Time.Sub(out[i].Time) <= slack {
			j++
		}
		sort.SliceStable(out[i:j], func(a, b int) bool { return out[i+a].Mark < out[i+b].Mark })
		i = j
	}
	return out
}

// Recording is what the reference did with a scenario.
type Recording struct {
	// Reference names the reference ("ReGaHss R1.00.0388.0254"); Rate is the clock's speed.
	Reference string
	Rate      int
	// Created says when each program was created (and, when active, evaluated once), in order.
	Created []Created
	// Steps says when each step was done.
	Steps []time.Time
	Trace Trace
}

// Created is a program's creation.
type Created struct {
	Program string
	Time    time.Time
}

// Encode writes a recording as the text of rega.trace: a line per event in the order it
// happened, "<time> created <program>", "<time> step <n>", "<time> ran <marker>".
func (r *Recording) Encode() []byte {
	type line struct {
		at   time.Time
		rank int
		text string
	}
	var lines []line
	for _, c := range r.Created {
		lines = append(lines, line{c.Time, 0, "created " + c.Program})
	}
	for i, t := range r.Steps {
		lines = append(lines, line{t, 1, "step " + strconv.Itoa(i+1)})
	}
	for _, e := range r.Trace {
		lines = append(lines, line{e.Time, 2, "ran " + e.Mark})
	}
	sort.SliceStable(lines, func(i, j int) bool {
		if !lines[i].at.Equal(lines[j].at) {
			return lines[i].at.Before(lines[j].at)
		}
		return lines[i].rank < lines[j].rank
	})
	var b bytes.Buffer
	fmt.Fprintf(&b, "# what the reference did (cmd/regaharness wrote this file)\nreference %s\nrate %d\n", r.Reference, r.Rate)
	for _, l := range lines {
		fmt.Fprintf(&b, "%s %s\n", l.at.In(Zone).Format(TimeLayout+" -0700"), l.text)
	}
	return b.Bytes()
}

// DecodeRecording reads what Encode wrote.
func DecodeRecording(data []byte) (*Recording, error) {
	r := &Recording{}
	const stamp = TimeLayout + " -0700"
	for n, l := range strings.Split(string(data), "\n") {
		bad := func() (*Recording, error) { return nil, fmt.Errorf("rega.trace line %d: %q", n+1, l) }
		switch {
		case l == "" || strings.HasPrefix(l, "#"):
		case strings.HasPrefix(l, "reference "):
			r.Reference = l[len("reference "):]
		case strings.HasPrefix(l, "rate "):
			rate, err := strconv.Atoi(l[len("rate "):])
			if err != nil {
				return bad()
			}
			r.Rate = rate
		default:
			if len(l) < len(stamp)+2 {
				return bad()
			}
			at, err := time.ParseInLocation(stamp, l[:len(stamp)], Zone)
			if err != nil {
				return bad()
			}
			kind, arg, _ := strings.Cut(l[len(stamp)+1:], " ")
			switch kind {
			case "created":
				r.Created = append(r.Created, Created{Program: arg, Time: at})
			case "step":
				if i, err := strconv.Atoi(arg); err != nil || i != len(r.Steps)+1 {
					return bad()
				}
				r.Steps = append(r.Steps, at)
			case "ran":
				r.Trace = append(r.Trace, Entry{Time: at, Mark: arg})
			default:
				return bad()
			}
		}
	}
	return r, nil
}
