// Package addonpkg tests the package files the addon ships.
package addonpkg

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit/fragment"
	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit/manifest"
)

// what openccu-lite 2.0.0-rc.1 added to the manifest and the kit's validator does not know yet:
// checked here by the rules of the system's manifest document, and taken out before the kit's
// strict load checks the rest
var (
	newScopes     = []string{"meta:interface", "regadom:read"}
	reservedNames = []string{"BidCos-RF", "BidCos-Wired", "HmIP-RF", "VirtualDevices", "CUxD", "ReGaHSS", "ReGa", "System", "CCU-Jack"}
	ifaceName     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{1,31}$`)
)

type rpcInterface struct {
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Path     string `json:"path"`
	Protocol string `json:"protocol"`
}

func load(t *testing.T) (*manifest.Manifest, rpcInterface, string, []string) {
	t.Helper()
	b, err := os.ReadFile("../../addon/openccu-lite.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	var ui, rt map[string]json.RawMessage
	_ = json.Unmarshal(raw["ui"], &ui)
	_ = json.Unmarshal(raw["runtime"], &rt)
	var ri rpcInterface
	if err := json.Unmarshal(rt["rpc_interface"], &ri); err != nil {
		t.Fatalf("runtime.rpc_interface: %v", err)
	}
	var importURL string
	_ = json.Unmarshal(ui["regadom_import_url"], &importURL)
	var scopes, old []string
	_ = json.Unmarshal(rt["api_scopes"], &scopes)
	for _, s := range scopes {
		known := true
		for _, n := range newScopes {
			if s == n {
				known = false
			}
		}
		if known {
			old = append(old, s)
		}
	}
	delete(rt, "rpc_interface")
	delete(ui, "regadom_import_url")
	rt["api_scopes"], _ = json.Marshal(old)
	raw["ui"], _ = json.Marshal(ui)
	// (release.prerelease: the system's manifest has it, the kit's not yet - TestPrerelease)
	var rel map[string]json.RawMessage
	_ = json.Unmarshal(raw["release"], &rel)
	delete(rel, "prerelease")
	raw["release"], _ = json.Marshal(rel)
	raw["runtime"], _ = json.Marshal(rt)
	rest, _ := json.Marshal(raw)
	m, err := manifest.Parse(rest)
	if err != nil {
		t.Fatal(err)
	}
	return m, ri, importURL, scopes
}

// The manifest as the system validates it at install.
func TestManifest(t *testing.T) {
	m, ri, importURL, scopes := load(t)
	if m.ID != "logic" || m.Undeclared() || m.Runtime == nil || !m.Runtime.Daemon {
		t.Fatalf("manifest: %+v", m)
	}
	// the name the system shows on the addon's tile and in its lists: short, the same in both languages
	if m.Name != "Logic" {
		t.Errorf("the addon's name is %q, want Logic", m.Name)
	}
	if len(m.Runtime.Capabilities) != 0 || len(m.Runtime.Ports) != 0 {
		t.Errorf("the addon needs no capability and no port: %+v", m.Runtime)
	}
	if strings.Join(scopes, ",") != "rpc:operate,meta:read,meta:interface,regadom:read" {
		t.Errorf("scopes %v", scopes)
	}
	if !ifaceName.MatchString(ri.Name) || ri.Name != "Logic" {
		t.Errorf("rpc_interface.name %q", ri.Name)
	}
	for _, r := range reservedNames {
		if strings.EqualFold(r, ri.Name) {
			t.Errorf("rpc_interface.name %q is the system's", ri.Name)
		}
	}
	if ri.Port < 1024 || ri.Port > 65535 || (ri.Protocol != "" && ri.Protocol != "xmlrpc") {
		t.Errorf("rpc_interface %+v", ri)
	}
	if !strings.HasPrefix(importURL, "/addons/logic/") {
		t.Errorf("ui.regadom_import_url %q", importURL)
	}
	if m.Description["de"] == "" || m.Description["en"] == "" || m.Runtime.Technical["de"] == "" || m.Runtime.Technical["en"] == "" ||
		m.Runtime.Note["de"] == "" || m.Runtime.Note["en"] == "" {
		t.Error("the description, the note and the technical text in both languages")
	}
	for _, scope := range scopes {
		if !strings.Contains(m.Runtime.Technical["en"], scope) || !strings.Contains(m.Runtime.Technical["de"], scope) {
			t.Errorf("the technical text does not explain %s", scope)
		}
	}
	for _, text := range []string{m.Description["de"], m.Description["en"], m.Runtime.Technical["de"], m.Runtime.Note["de"], m.Runtime.Technical["en"], m.Runtime.Note["en"]} {
		if strings.Contains(strings.ToLower(text), " box") {
			t.Error("user-facing text says \"the system\"")
		}
	}
}

// The daemon's default ports are the manifest's and the proxy rule's.
func TestPorts(t *testing.T) {
	_, ri, _, _ := load(t)
	main, err := os.ReadFile("../../cmd/logic/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), `"rpc-port", `+itoa(ri.Port)+`,`) {
		t.Errorf("cmd/logic: the default of -rpc-port is not the manifest's %d", ri.Port)
	}
	if !strings.Contains(string(main), `"port", 8301,`) {
		t.Error("cmd/logic: the default of -port is not the proxy rule's 8301")
	}
	// B-10: below Linux's ephemeral range (32768-60999), where any outgoing connection on the
	// system may hold the port and the daemon then cannot listen
	if ri.Port >= 32768 {
		t.Errorf("rpc_interface.port %d lies in the ephemeral range; take one below 32768", ri.Port)
	}
	m, _, _, _ := load(t)
	for _, lang := range []string{"de", "en"} {
		if !strings.Contains(m.Runtime.Technical[lang], "127.0.0.1:"+itoa(ri.Port)+")") {
			t.Errorf("the %s technical text does not name 127.0.0.1:%d", lang, ri.Port)
		}
	}
}

// B-10: the ports the end-to-end tests and the development server use lie below the ephemeral
// range too, so that a run on a busy machine does not find one taken by an outgoing connection.
func TestDevPortsBelowEphemeralRange(t *testing.T) {
	port := regexp.MustCompile(`PORT['"]?\s*[:=]\s*['"]?(\d{4,5})|127\.0\.0\.1:(\d{4,5})|\$\{\w+:-(\d{4,5})\}`)
	for _, f := range []string{"../../ui/playwright.config.ts", "../../scripts/dev-serve.sh", "../../ui/e2e/interface.spec.ts"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, m := range port.FindAllStringSubmatch(string(b), -1) {
			p, _ := strconv.Atoi(m[1] + m[2] + m[3])
			found++
			if p >= 32768 {
				t.Errorf("%s: port %d lies in the ephemeral range", f, p)
			}
		}
		if found == 0 {
			t.Errorf("%s: no port found - the pattern no longer matches the file", f)
		}
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// The manifest's version is the changelog's newest.
func TestVersionInChangelog(t *testing.T) {
	var m struct {
		Version string `json:"version"`
	}
	b, _ := os.ReadFile("../../addon/openccu-lite.json")
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	cl, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cl), "## ["+m.Version+"]") {
		t.Errorf("CHANGELOG.md has no entry for %s", m.Version)
	}
}

// The manifest's images are tracked at the path the catalogue reads them from.
func TestImagesTracked(t *testing.T) {
	for _, p := range []string{"addon/logic/www/icon.svg", "addon/logic/www/icon-dark.svg", "addon/logic/etc/lighttpd.conf", "addon/logic/www/lighttpd-error-503.html"} {
		if out, err := exec.Command("git", "-C", "../..", "ls-files", "--error-unmatch", "--", p).CombinedOutput(); err != nil {
			t.Errorf("%s not tracked: %v %s", p, err, out)
		}
	}
}

// The proxy rule is what the kit's renderer writes (apart from the comment lines).
func TestFragment(t *testing.T) {
	want, err := fragment.Render(fragment.Fragment{ID: "logic", Port: 8301, ErrorfilePrefix: "/usr/local/addons/logic/www/lighttpd-error-"})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile("../../addon/logic/etc/lighttpd.conf")
	strip := func(s string) string {
		var out []string
		for _, l := range strings.Split(s, "\n") {
			if !strings.HasPrefix(l, "#") && strings.TrimSpace(l) != "" {
				out = append(out, l)
			}
		}
		return strings.Join(out, "\n")
	}
	if strip(string(got)) != strip(want) {
		t.Errorf("etc/lighttpd.conf:\n%s\nwant:\n%s", got, want)
	}
}

// The 503 page names the addon, in the tracked copy and the one the build copies.
func TestErrorPage(t *testing.T) {
	for _, p := range []string{"../../addon/static/lighttpd-error-503.html", "../../addon/logic/www/lighttpd-error-503.html"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "<title>Logic</title>") {
			t.Errorf("%s does not name the addon", p)
		}
	}
}

// The rc.d script and the update_script name no other addon.
func TestScripts(t *testing.T) {
	for _, p := range []string{"../../addon/logic/rc.d", "../../addon/update_script"} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, other := range []string{"notify", "nut", "UPS", "forwarder"} {
			if strings.Contains(string(b), other) {
				t.Errorf("%s names another addon (%s)", p, other)
			}
		}
		if !strings.HasPrefix(string(b), "#!/bin/sh\n") {
			t.Errorf("%s is not a sh script", p)
		}
	}
	// B-9: no reboot asked for - openccu-lite starts the new addon itself, and exit 10 made it
	// offer a reboot nobody needed
	us, _ := os.ReadFile("../../addon/update_script")
	if !strings.HasSuffix(string(us), "\nexit 0\n") || regexp.MustCompile(`(?m)^\s*exit\s+[^0\s]`).Match(us) {
		t.Error("update_script: it ends with exit 0 and never asks for a reboot")
	}
	rc, _ := os.ReadFile("../../addon/logic/rc.d")
	if !strings.Contains(string(rc), "ADDON=logic\n") || !strings.Contains(string(rc), "Config-Url: /addons/$ADDON/") {
		t.Error("rc.d: the addon id or the Config-Url")
	}
}

// A version with a pre-release part (1.0.0-beta.0) is published as a GitHub pre-release, which the
// system installs only when the manifest's release says prerelease - and a release version is not
// marked one.
func TestPrerelease(t *testing.T) {
	var m struct {
		Version string `json:"version"`
		Release struct {
			Prerelease *bool `json:"prerelease"`
		} `json:"release"`
	}
	b, _ := os.ReadFile("../../addon/openccu-lite.json")
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	pre := strings.Contains(m.Version, "-")
	if pre && (m.Release.Prerelease == nil || !*m.Release.Prerelease) {
		t.Errorf("version %s: release.prerelease must be true", m.Version)
	}
	if !pre && m.Release.Prerelease != nil {
		t.Errorf("version %s: release.prerelease must go", m.Version)
	}
}
