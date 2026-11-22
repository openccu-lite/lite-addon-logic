// Package docs holds the user guide, German and English, as the daemon serves it: docs/de/ and
// docs/en/ below the page's path, where the page's ?-icons link a section by its anchor. The
// anchors are the same in both languages: an HTML anchor at the start of a heading,
// `## <a id="name"></a>Title`, which GitHub's Markdown keeps as well (it shows the files there);
// the served page moves the id onto the heading itself.
package docs

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

//go:embed de/handbuch.md en/guide.md screenshots
var files embed.FS

// Languages maps a language to its file.
var Languages = map[string]string{"de": "de/handbuch.md", "en": "en/guide.md"}

var (
	anchor = regexp.MustCompile(`(?m)^#{1,6} <a id="([a-z0-9-]+)"></a>`)
	title  = regexp.MustCompile(`(?m)^# (.+)$`)
	// (a chapter: a heading of the second level with its anchor)
	chapter = regexp.MustCompile(`(?m)^## <a id="([a-z0-9-]+)"></a>(.+?)\s*$`)
	// (a rendered heading that starts with its anchor)
	headingAnchor = regexp.MustCompile(`<h([1-6])><a id="([a-z0-9-]+)"></a>`)
)

// Chapter is one chapter of a guide: its heading and its anchor.
type Chapter struct {
	Title  string
	Anchor string
}

// Chapters lists the chapters of a language's guide in their order.
func Chapters(lang string) ([]Chapter, error) {
	src, err := Source(lang)
	if err != nil {
		return nil, err
	}
	var out []Chapter
	for _, m := range chapter.FindAllSubmatch(src, -1) {
		out = append(out, Chapter{Title: string(m[2]), Anchor: string(m[1])})
	}
	return out, nil
}

// contentsWord heads the list of chapters.
var contentsWord = map[string]string{"de": "Inhalt", "en": "Contents"}

// Source answers the Markdown of a language's guide.
func Source(lang string) ([]byte, error) {
	f, ok := Languages[lang]
	if !ok {
		return nil, fmt.Errorf("no guide in %q", lang)
	}
	return files.ReadFile(f)
}

// pictureName is a picture's file name in screenshots/<lang>/.
var pictureName = regexp.MustCompile(`^[0-9a-z-]+\.png$`)

// Picture answers a picture of the guide, screenshots/<lang>/<name> (the pages link it as
// ../screenshots/<lang>/<name>, which is the same path beside the Markdown files).
func Picture(lang, name string) ([]byte, error) {
	if _, ok := Languages[lang]; !ok || !pictureName.MatchString(name) {
		return nil, fmt.Errorf("no picture %s/%s", lang, name)
	}
	return files.ReadFile("screenshots/" + lang + "/" + name)
}

// Anchors lists the anchors of a language's guide, sorted.
func Anchors(lang string) ([]string, error) {
	src, err := Source(lang)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range anchor.FindAllSubmatch(src, -1) {
		out = append(out, string(m[1]))
	}
	sort.Strings(out)
	return out, nil
}

const page = `<!doctype html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>%s</title>
<style>
  :root { color-scheme: light dark; }
  body { margin: 0; font: 15px/1.55 system-ui, sans-serif; background: Canvas; color: CanvasText; }
  main { max-width: 52em; margin: 0 auto; padding: 16px 20px 64px; }
  h1 { font-size: 1.5em; } h2 { font-size: 1.25em; margin-top: 2em; } h3 { font-size: 1.05em; margin-top: 1.5em; }
  h2:target, h3:target { outline: 2px solid Highlight; outline-offset: 4px; }
  img { max-width: 100%%; height: auto; border: 1px solid GrayText; border-radius: 4px; }
  code { font-family: ui-monospace, monospace; font-size: .92em; }
  pre { overflow: auto; padding: 10px 12px; border: 1px solid GrayText; border-radius: 4px; }
  table { border-collapse: collapse; display: block; max-width: 100%%; overflow-x: auto; } th, td { text-align: left; padding: 4px 10px; border-bottom: 1px solid GrayText; vertical-align: top; }
  nav { font-size: .9em; opacity: .8; }
  .toc { margin: 1em 0; padding: 8px 12px; border: 1px solid GrayText; border-radius: 4px; }
  .toc ol { margin: 4px 0 0; padding-left: 1.6em; }
</style>
</head>
<body>
<main>
<nav><a href="../de/">Deutsch</a> · <a href="../en/">English</a></nav>
%s
%s
</main>
</body>
</html>
`

// HTML renders a language's guide as a page.
func HTML(lang string) ([]byte, error) {
	src, err := Source(lang)
	if err != nil {
		return nil, err
	}
	// (the guide is the repository's own text: its anchors are the only raw HTML in it)
	md := goldmark.New(goldmark.WithExtensions(extension.Table), goldmark.WithRendererOptions(gmhtml.WithUnsafe()))
	var body bytes.Buffer
	if err := md.Convert(src, &body); err != nil {
		return nil, err
	}
	body = *bytes.NewBuffer(headingAnchor.ReplaceAll(body.Bytes(), []byte(`<h$1 id="$2">`)))
	name := lang
	if m := title.FindSubmatch(src); m != nil {
		name = string(m[1])
	}
	// the list of chapters stands between the guide's first paragraph and its first chapter
	chapters, _ := Chapters(lang)
	var toc bytes.Buffer
	fmt.Fprintf(&toc, `<div class="toc" id="contents"><strong>%s</strong><ol>`, contentsWord[lang])
	for _, c := range chapters {
		fmt.Fprintf(&toc, `<li><a href="#%s">%s</a></li>`, c.Anchor, html.EscapeString(c.Title))
	}
	toc.WriteString("</ol></div>\n")
	head, rest := body.String(), ""
	if i := strings.Index(head, "<h2"); i >= 0 {
		head, rest = head[:i], head[i:]
	}
	return []byte(fmt.Sprintf(page, lang, html.EscapeString(name), head+toc.String(), rest)), nil
}
