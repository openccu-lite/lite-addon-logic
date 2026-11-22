package harness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The run trace explains the quirks: for these scenarios the engine's own account of every run
// - the trigger with the value it brought, each condition line with its value and truth, the
// block, what was written, scheduled, ended or skipped - is kept beside the recording
// (engine.runs). A reader of that file alone can say why the program did what the quirk
// describes; a change of the engine that changes the account shows here.
var explained = []string{
	"q03-update-runs-on-every-write",              // Q-3, Q-4: every write, SONST each time
	"q05-check-only-never-runs",                   // Q-5: not triggered, check only
	"q07-two-lines-on-one-variable",               // Q-7: one run for two lines
	"q16-two-triggers-two-delayed-runs",           // Q-16
	"q17-cancel-across-programs",                  // Q-17
	"q18-a-delayed-activity-is-not-checked-again", // Q-18
	"q37-the-same-value-is-no-change",             // Q-2, Q-37: a flip, not a value
	"q48-an-unreached-change-line",                // Q-48
	"t15-two-modules-in-one-program",              // time modules
}

func TestTheRunTraceExplainsTheQuirks(t *testing.T) {
	byName := map[string]*Scenario{}
	for _, sc := range Scenarios() {
		byName[sc.Name] = sc
	}
	for _, name := range explained {
		t.Run(name, func(t *testing.T) {
			sc := byName[name]
			if sc == nil {
				t.Fatal("no such scenario")
			}
			dir := filepath.Join("testdata", name)
			raw, err := os.ReadFile(filepath.Join(dir, "rega.trace"))
			if err != nil {
				t.Fatal(err)
			}
			rec, err := DecodeRecording(raw)
			if err != nil {
				t.Fatal(err)
			}
			packed, err := os.ReadFile(filepath.Join(dir, "homematic.regadom.gz"))
			if err != nil {
				t.Fatal(err)
			}
			dom, err := Gunzip(packed)
			if err != nil {
				t.Fatal(err)
			}
			im, err := Import(sc, dom)
			if err != nil {
				t.Fatal(err)
			}
			_, runs, err := RunEngineRuns(sc, im, rec)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(runs))
			for n := range runs {
				names = append(names, n)
			}
			sort.Strings(names)
			var b strings.Builder
			b.WriteString("# " + sc.Quirk + ": " + sc.Title + "\n")
			for _, n := range names {
				b.WriteString("\nprogram " + n + "\n")
				for _, r := range runs[n] {
					if r.Time.Before(sc.StartTime()) {
						continue // the creation, as on the reference
					}
					b.WriteString(r.Text() + "\n")
				}
			}
			file := filepath.Join(dir, "engine.runs")
			want, _ := os.ReadFile(file)
			if got := b.String(); got != string(want) {
				if os.Getenv("HARNESS_UPDATE") != "" {
					if err := os.WriteFile(file, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				t.Errorf("the engine's account changed (HARNESS_UPDATE=1 rewrites engine.runs):\n%s", got)
			}
		})
	}
}
