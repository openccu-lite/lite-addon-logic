package importer

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// check is what must hold for any database: every program and variable written reads back, the
// set has no problem, the report renders in both languages.
func check(t testing.TB, res *Result) {
	t.Helper()
	set := model.NewSet()
	for _, v := range res.Variables {
		back, err := model.DecodeVariable(v.Key+".yaml", v.Key, model.EncodeVariable(v))
		if err != nil {
			t.Fatalf("variable %s: %v", v.Key, err)
		}
		set.Variables[v.Key] = back
		if x, ok := res.Values[v.Key]; ok {
			if err := v.CheckValue(x); err != nil {
				t.Fatalf("value of %s: %v", v.Key, err)
			}
		}
	}
	for _, p := range res.Programs {
		data := model.EncodeProgram(p, nil)
		back, err := model.DecodeProgram(p.Key+".yaml", p.Key, data)
		if err != nil {
			t.Fatalf("program %s: %v\n%s", p.Key, err, data)
		}
		if !bytes.Equal(model.EncodeProgram(back, nil), data) {
			t.Fatalf("program %s is not stable", p.Key)
		}
		set.Programs[p.Key] = back
	}
	if problems := set.Problems(); len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if _, err := DecodeLedger(res.Ledger.Encode()); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSeen(EncodeSeen(res.Seen)); err != nil {
		t.Fatal(err)
	}
	if res.Report.Markdown("de") == "" || res.Report.Markdown("en") == "" {
		t.Fatal("no report")
	}
}

// shape is what an import made of a database, without a single name: per program its id, state
// and the codes of its reasons and notes, per variable its type and notes, the devices, the
// histogram, the trigger table's differences. An anonymised database must give the shape of
// its original.
func shape(res *Result) string {
	var b strings.Builder
	r := res.Report
	for _, p := range r.Programs {
		fmt.Fprintf(&b, "program %d %s was=%t active=%t reasons=%s notes=%s\n", p.ID, p.Status, p.WasActive, p.Active, codes(p.Reasons), codes(p.Notes))
	}
	for _, v := range r.Variables {
		fmt.Fprintf(&b, "variable %d %s %s entries=%d notes=%s\n", v.ID, v.Type, v.Status, len(v.Entries), codes(v.Notes))
	}
	for _, d := range r.Devices {
		fmt.Fprintf(&b, "device %s %s known=%t programs=%d\n", d.Type, d.ChannelType, d.Known, len(d.Programs))
	}
	for _, c := range r.Histogram {
		fmt.Fprintf(&b, "count %s %d\n", c.What, c.N)
	}
	fmt.Fprintf(&b, "events %s\n", codes(r.Events))
	for _, p := range res.Programs {
		// the file without its words: what is left of a program when names, notes and texts go
		p := *p
		fmt.Fprintf(&b, "file active=%t rules=%d at_start=%t\n", p.Active, len(p.Rules), p.AtStart)
	}
	return b.String()
}

// anonymous reads a database and its anonymised twin.
func anonymous(t testing.TB, raw []byte) (was, now *regadom.Source, anon []byte) {
	t.Helper()
	was, err := regadom.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := regadom.Anonymise(bytes.NewReader(raw), &out); err != nil {
		t.Fatal(err)
	}
	now, err = regadom.Read(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	return was, now, out.Bytes()
}

func known(string) bool { return true }

// An anonymised database imports exactly as its original does.
func TestAnonymisedImportsAlike(t *testing.T) {
	for _, name := range []string{"fixture", "devices"} {
		raw, err := os.ReadFile("../regadom/testdata/" + name + ".regadom")
		if err != nil {
			t.Fatal(err)
		}
		was, now, _ := anonymous(t, raw)
		for _, opt := range []Options{{Known: known, Location: berlin(t)}, {Location: berlin(t)}, {AllInactive: true, Known: known, Location: berlin(t)}} {
			a, b := Import(was, opt), Import(now, opt)
			check(t, b)
			if shape(a) != shape(b) {
				t.Errorf("%s: the anonymised database imports differently:\n--- original\n%s--- anonymised\n%s", name, shape(a), shape(b))
			}
		}
	}
}

// The corpus: real databases, each with what the import makes of it. testdata/corpus/<name>/
// holds homematic.regadom.gz and the golden report, programs and variables. Nobody's database
// is in the repository as it was: every file but the ones named public here is what the
// anonymiser writes (logic anonymise) - the test fails for a file the anonymiser would still
// change.
var publicCorpus = map[string]bool{
	"regahss-test": true, // OpenCCU/ReGaHss-Test's synthetic database (MIT)
}

func TestCorpusFiles(t *testing.T) {
	dirs, err := os.ReadDir("testdata/corpus")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		n++
		name := d.Name()
		t.Run(name, func(t *testing.T) {
			raw := gunzip(t, "testdata/corpus/"+name+"/homematic.regadom.gz")
			src, now, anon := anonymous(t, raw)
			if !publicCorpus[name] && !bytes.Equal(anon, raw) {
				t.Fatalf("the database is not anonymised: run it through \"logic anonymise\" before it goes into the corpus")
			}
			res := Import(src, Options{Known: known, Location: berlin(t)})
			check(t, res)
			if len(res.Report.Events) != 0 {
				t.Errorf("the CCU's trigger table disagrees: %v", res.Report.Events)
			}
			if a, b := shape(res), shape(Import(now, Options{Known: known, Location: berlin(t)})); a != b {
				t.Errorf("anonymised, it imports differently:\n--- original\n%s--- anonymised\n%s", a, b)
			}
			var programs, variables bytes.Buffer
			for _, p := range res.Programs {
				fmt.Fprintf(&programs, "# programs/%s.yaml\n%s\n", p.Key, model.EncodeProgram(p, nil))
			}
			for _, v := range res.Variables {
				fmt.Fprintf(&variables, "# variables/%s.yaml\n%s\n", v.Key, model.EncodeVariable(v))
			}
			golden(t, "../corpus/"+name+"/programs.yaml", programs.Bytes())
			golden(t, "../corpus/"+name+"/variables.yaml", variables.Bytes())
			golden(t, "../corpus/"+name+"/values.txt", values(res))
			golden(t, "../corpus/"+name+"/report.de.md", []byte(res.Report.Markdown("de")))
			golden(t, "../corpus/"+name+"/report.en.md", []byte(res.Report.Markdown("en")))
			// with no device known and with everything inactive it still writes valid files
			check(t, Import(src, Options{Location: berlin(t)}))
			check(t, Import(src, Options{AllInactive: true, Known: known, Lang: "en"}))
		})
	}
	// ccu3-home is in the development repository only, not in the published one: two at least
	if n < 2 {
		t.Fatalf("%d databases in the corpus", n)
	}
}

func gunzip(t testing.TB, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(z)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The databases the reference of the comparison saved (internal/harness/testdata): every
// construct the scenarios build with the editor's calls, written by a real ReGaHss.
func TestHarnessDatabases(t *testing.T) {
	files, _ := filepath.Glob("../harness/testdata/*/homematic.regadom.gz")
	if len(files) < 40 {
		t.Fatalf("%d recorded databases", len(files))
	}
	for _, path := range files {
		src, err := regadom.Read(bytes.NewReader(gunzip(t, path)))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		res := Import(src, Options{Known: known, Location: berlin(t)})
		check(t, res)
		if len(res.Report.Events) != 0 {
			t.Errorf("%s: the CCU's trigger table disagrees: %v", path, res.Report.Events)
		}
	}
}

// More real databases, where there are some: REGADOM_CORPUS names a directory of .sbk and
// .regadom files (never committed: they are somebody's home). Each must import cleanly, and
// import the same once anonymised.
func TestCorpus(t *testing.T) {
	dir := os.Getenv("REGADOM_CORPUS")
	if dir == "" {
		t.Skip("REGADOM_CORPUS is not set")
	}
	n := 0
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Size() == 0 || !(strings.HasSuffix(path, ".sbk") || strings.HasSuffix(path, ".regadom")) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			return nil
		}
		if _, err := regadom.Read(bytes.NewReader(raw)); err != nil {
			t.Logf("%s: %v", path, err)
			return nil
		}
		n++
		src, now, _ := anonymous(t, raw)
		res := Import(src, Options{Known: known})
		check(t, res)
		if a, b := shape(res), shape(Import(now, Options{Known: known})); a != b {
			t.Errorf("%s: anonymised, it imports differently:\n--- original\n%s--- anonymised\n%s", filepath.Base(path), a, b)
		}
		imported, inactive, skipped, vars := res.Report.Totals()
		t.Logf("%s: %d programs imported, %d inactive, %d skipped, %d variables, %d devices used, trigger table differences: %v",
			filepath.Base(path), imported, inactive, skipped, vars, len(res.Report.Devices), res.Report.Events)
		return nil
	})
	if n == 0 {
		t.Errorf("no database under %s", dir)
	}
}

// Whatever the reader accepts, the importer maps without a panic and writes only valid files.
func FuzzImport(f *testing.F) {
	for _, name := range []string{"fixture.regadom", "devices.regadom"} {
		b, err := os.ReadFile("../regadom/testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		src, err := regadom.Read(bytes.NewReader(b))
		if err != nil {
			return
		}
		check(t, Import(src, Options{Known: func(string) bool { return true }}))
		check(t, Import(src, Options{AllInactive: true, Lang: "en"}))
	})
}
