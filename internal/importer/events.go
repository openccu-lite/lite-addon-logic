package importer

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// Triggers lists, per program id, the objects whose events evaluate the program as the importer
// reads its lines: every line that is not "nur prüfen", by the id of its variable, datapoint or
// time module.
func Triggers(db *regadom.DB) map[int][]int {
	out := map[int][]int{}
	for _, p := range db.Programs {
		chain, err := db.Chain(p)
		if err != nil {
			continue
		}
		set := map[int]bool{}
		for _, r := range chain {
			for _, cid := range r.Conditions {
				c := db.Conditions[cid]
				if c == nil {
					continue
				}
				for _, lid := range c.Lines {
					l := db.Lines[lid]
					if l == nil || l.Type2 == regadom.TrigCheck {
						continue
					}
					src := l.Left
					if l.Type1 == regadom.CmpTime || l.Left.Type == regadom.TypeCurrentDate {
						src = l.Right1
					}
					if id, err := strconv.Atoi(src.Text); err == nil {
						set[id] = true
					}
				}
			}
		}
		ids := make([]int, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		out[p.ID] = ids
	}
	return out
}

// Differences to the CCU's own trigger table.
const (
	EventMissing = "event-missing" // a line triggers, the table does not list its source
	EventExtra   = "event-extra"   // the table lists a source no line triggers on
)

// CheckEvents compares the triggers the importer finds with ReGa's own trigger table
// (dom-valueevents) and names every difference (program, object); an empty answer means they
// agree. ReGa lists a program under each of its time modules whatever the line's mode, so a time
// module on a "nur prüfen" line is no difference. A database without the table (a trimmed test
// file) has nothing to compare.
func CheckEvents(db *regadom.DB) []Note {
	if len(db.Events) == 0 {
		return nil
	}
	table := map[int]map[int]bool{}
	for _, e := range db.Events {
		if db.Program(e.Dst) == nil {
			continue // alarms and the WebUI's own mappings
		}
		if table[e.Dst] == nil {
			table[e.Dst] = map[int]bool{}
		}
		table[e.Dst][e.Src] = true
	}
	var out []Note
	found := Triggers(db)
	for _, p := range db.Programs {
		ours := map[int]bool{}
		for _, id := range found[p.ID] {
			ours[id] = true
			if !table[p.ID][id] {
				out = append(out, Note{Code: EventMissing, Args: []string{p.Name, objectName(db, id)}})
			}
		}
		var theirs []int
		for id := range table[p.ID] {
			theirs = append(theirs, id)
		}
		sort.Ints(theirs)
		for _, id := range theirs {
			if !ours[id] && db.Calendars[id] == nil {
				out = append(out, Note{Code: EventExtra, Args: []string{p.Name, objectName(db, id)}})
			}
		}
	}
	return out
}

func objectName(db *regadom.DB, id int) string {
	if n := db.Names[id]; n != "" {
		return fmt.Sprintf("%s (%d)", n, id)
	}
	return strconv.Itoa(id)
}
