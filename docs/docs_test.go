package docs

import (
	"bytes"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// Both languages have the same sections under the same anchors, and each renders with them.
func TestGuide(t *testing.T) {
	de, err := Anchors("de")
	if err != nil {
		t.Fatal(err)
	}
	en, err := Anchors("en")
	if err != nil {
		t.Fatal(err)
	}
	if len(de) == 0 || !reflect.DeepEqual(de, en) {
		t.Fatalf("anchors differ:\nde %v\nen %v", de, en)
	}
	for lang := range Languages {
		page, err := HTML(lang)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range de {
			if !strings.Contains(string(page), `id="`+a+`"`) {
				t.Errorf("%s: no element with id %q", lang, a)
			}
		}
		if !strings.Contains(string(page), `<html lang="`+lang+`">`) || strings.Contains(string(page), "{#") || strings.Contains(string(page), "<a id=") {
			t.Errorf("%s: the page's language or a raw anchor", lang)
		}
		src, _ := Source(lang)
		// every heading below the title has its anchor, written the way GitHub keeps it, each once;
		// and the anchors are the only raw HTML (the page renders the guide's HTML as it is)
		seen := map[string]bool{}
		for _, line := range strings.Split(string(src), "\n") {
			if m := regexp.MustCompile(`^#{2,6} (<a id="([a-z0-9-]+)"></a>)?`).FindStringSubmatch(line); m != nil {
				if m[1] == "" {
					t.Errorf("%s: a heading without its anchor: %s", lang, line)
				} else if seen[m[2]] {
					t.Errorf("%s: the anchor %q twice", lang, m[2])
				}
				seen[m[2]] = true
			}
		}
		// (pictures are the other raw HTML: a <picture> with its dark <source> and its <img>)
		pictures := regexp.MustCompile(`<picture><source media="\(prefers-color-scheme: dark\)" srcset="[^"]+"><img alt="[^"]+" src="[^"]+"></picture>`)
		prose := pictures.ReplaceAll(regexp.MustCompile("(?s)```.*?```|`[^`\n]*`").ReplaceAll(src, nil), nil)
		if html := regexp.MustCompile(`<[a-zA-Z/][^>]*>`).FindAllString(string(prose), -1); len(html) != 2*len(de) {
			t.Errorf("%s: raw HTML besides the anchors and pictures: %d tags for %d anchors", lang, len(html), len(de))
		}
		for _, a := range de {
			if !regexp.MustCompile(`<h[2-6] id="` + a + `">`).Match(page) {
				t.Errorf("%s: the anchor %q is not on its heading", lang, a)
			}
		}
		if strings.Contains(strings.ToLower(string(src)), " box") {
			t.Errorf("%s: user-facing text says \"the system\"", lang)
		}
	}
	if _, err := HTML("fr"); err == nil {
		t.Error("a language without a guide")
	}
	if _, err := Anchors("fr"); err == nil {
		t.Error("a language without a guide")
	}
}

// The guide's program and variable examples are files the reader takes, in both languages, and
// they are written the canonical way.
func TestExamplesAreValid(t *testing.T) {
	block := regexp.MustCompile("(?s)```yaml\n(.*?)```")
	for lang := range Languages {
		src, _ := Source(lang)
		blocks := block.FindAllSubmatch(src, -1)
		if len(blocks) != 6 {
			t.Fatalf("%s: %d yaml examples, want the several-values program, the display program, the webhook program, the command program, the program and the variable", lang, len(blocks))
		}
		// (the examples of the activities beyond the CCU come first in the guide)
		for _, b := range blocks[:4] {
			if more, err := model.DecodeProgram("example.yaml", "example", b[1]); err != nil {
				t.Errorf("%s: an activity's example: %v", lang, err)
			} else if got := model.EncodeProgram(more, nil); string(got) != string(b[1]) {
				t.Errorf("%s: an activity's example is not canonical:\n%s", lang, got)
			}
		}
		blocks = blocks[4:]
		p, err := model.DecodeProgram("example.yaml", "example", blocks[0][1])
		if err != nil {
			t.Errorf("%s: the program example: %v", lang, err)
		} else if got := model.EncodeProgram(p, func(model.Ref) string { return "Markise Terrasse" }); string(got) != string(blocks[0][1]) {
			t.Errorf("%s: the program example is not canonical:\n%s", lang, got)
		}
		v, err := model.DecodeVariable("example.yaml", "example", blocks[1][1])
		if err != nil {
			t.Errorf("%s: the variable example: %v", lang, err)
		} else if got := model.EncodeVariable(v); string(got) != string(blocks[1][1]) {
			t.Errorf("%s: the variable example is not canonical:\n%s", lang, got)
		}
	}
}

// Every link inside the guide leads to a section the guide has, in both languages.
func TestLinksResolve(t *testing.T) {
	link := regexp.MustCompile(`\]\(#([^)]+)\)`)
	for lang := range Languages {
		anchors, _ := Anchors(lang)
		have := map[string]bool{}
		for _, a := range anchors {
			have[a] = true
		}
		src, _ := Source(lang)
		links := link.FindAllSubmatch(src, -1)
		if len(links) == 0 {
			t.Errorf("%s: no links found", lang)
		}
		for _, m := range links {
			if !have[string(m[1])] {
				t.Errorf("%s: a link to #%s, which the guide does not have", lang, m[1])
			}
		}
	}
}

// Someone who comes from a CCU finds the chapter for it, and the quirks each under what the
// addon does with it.
func TestFromACCU(t *testing.T) {
	anchors, _ := Anchors("en")
	have := map[string]bool{}
	for _, a := range anchors {
		have[a] = true
	}
	for _, a := range []string{"from-ccu", "from-ccu-steps", "from-ccu-where", "from-ccu-not", "quirks", "quirks-kept", "quirks-fixed", "quirks-gone"} {
		if !have[a] {
			t.Errorf("the guide has no section #%s", a)
		}
	}
}

// The page lists its chapters before the first one, each a link to it, in the guide's order and
// the same ones in both languages.
func TestChapters(t *testing.T) {
	en, err := Chapters("en")
	if err != nil || len(en) < 8 {
		t.Fatalf("chapters: %d, %v", len(en), err)
	}
	de, _ := Chapters("de")
	if len(de) != len(en) {
		t.Fatalf("chapters: %d in German, %d in English", len(de), len(en))
	}
	for i := range en {
		if de[i].Anchor != en[i].Anchor {
			t.Errorf("chapter %d: #%s in German, #%s in English", i, de[i].Anchor, en[i].Anchor)
		}
	}
	if en[0] != (Chapter{Title: "Overview", Anchor: "overview"}) || en[1].Anchor != "from-ccu" || en[2].Anchor != "quirks" {
		t.Errorf("the first chapters: %v", en[:3])
	}
	if _, err := Chapters("fr"); err == nil {
		t.Error("a language without a guide")
	}
	for lang, word := range map[string]string{"de": "Inhalt", "en": "Contents"} {
		page, _ := HTML(lang)
		p := string(page)
		toc, first := strings.Index(p, `<div class="toc" id="contents"><strong>`+word+`</strong>`), strings.Index(p, "<h2")
		if toc < 0 || toc > first {
			t.Errorf("%s: the contents are not before the first chapter (%d, %d)", lang, toc, first)
		}
		chapters, _ := Chapters(lang)
		for _, c := range chapters {
			if !strings.Contains(p, `<li><a href="#`+c.Anchor+`">`) {
				t.Errorf("%s: the contents do not link #%s", lang, c.Anchor)
			}
		}
		if strings.Contains(p, "%!") {
			t.Errorf("%s: a formatting verb went wrong", lang)
		}
	}
}

// Every picture the guides show is there, light and dark, in the guide's own language, with a text
// for those who cannot see it; both guides show the same pictures, and every picture is shown.
func TestPictures(t *testing.T) {
	picture := regexp.MustCompile(`<picture><source media="\(prefers-color-scheme: dark\)" srcset="\.\./screenshots/([a-z]+)/([0-9a-z-]+)-dark\.png"><img alt="([^"]+)" src="\.\./screenshots/([a-z]+)/([0-9a-z-]+)\.png"></picture>`)
	shown := map[string][]string{}
	for lang := range Languages {
		src, _ := Source(lang)
		if n := strings.Count(string(src), "<picture>"); n == 0 || n != len(picture.FindAll(src, -1)) {
			t.Errorf("%s: %d pictures, not all of them in the one form", lang, n)
		}
		for _, m := range picture.FindAllStringSubmatch(string(src), -1) {
			if m[1] != lang || m[4] != lang || m[2] != m[5] {
				t.Errorf("%s: a picture of another language or two pictures in one: %s", lang, m[0])
			}
			for _, name := range []string{m[5] + ".png", m[5] + "-dark.png"} {
				if b, err := Picture(lang, name); err != nil || !bytes.HasPrefix(b, []byte("\x89PNG")) {
					t.Errorf("%s: the picture %s is not there: %v", lang, name, err)
				}
			}
			shown[lang] = append(shown[lang], m[5])
		}
		page, _ := HTML(lang)
		if !strings.Contains(string(page), `<img alt="`) {
			t.Errorf("%s: the page shows no picture", lang)
		}
	}
	if !reflect.DeepEqual(shown["de"], shown["en"]) {
		t.Errorf("the guides show different pictures:\nde %v\nen %v", shown["de"], shown["en"])
	}
	// and nothing in screenshots/ that neither a guide nor a README shows
	readme := ""
	for _, f := range []string{"../README.md", "../README.en.md"} {
		b, _ := os.ReadFile(f)
		readme += string(b)
	}
	for lang := range Languages {
		entries, _ := files.ReadDir("screenshots/" + lang)
		for _, e := range entries {
			name := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".png"), "-dark")
			if !slices.Contains(shown[lang], name) && !strings.Contains(readme, "screenshots/"+lang+"/"+name+".png") {
				t.Errorf("screenshots/%s/%s is shown nowhere", lang, e.Name())
			}
		}
	}
	if _, err := Picture("de", "../de/handbuch.md"); err == nil {
		t.Error("a path that is no picture")
	}
	if _, err := Picture("fr", "01-programs.png"); err == nil {
		t.Error("a language without pictures")
	}
}
