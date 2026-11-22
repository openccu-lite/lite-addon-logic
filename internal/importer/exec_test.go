package importer

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// A script that is one system.Exec becomes a webhook, a Wake-on-LAN or a command; anything
// else stays a script.
func TestExecScripts(t *testing.T) {
	b := newBuild()
	flag := b.variable("Merker", bin, regadom.SubBool, "0", nil)
	text := b.variable("Antwort", str, 0, "", nil)
	b.variable("Doppelt", str, 0, "", nil)
	b.variable("Doppelt", str, 0, "", nil)
	alarm := b.variable("Alarm", bin, regadom.SubAlarm, "0", nil)
	line := on(flag.ID, regadom.CmpEquals, regadom.TrigChange, val("1", bin))
	prog := func(name, text string) {
		b.program(name, true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{script(text)}})
	}
	prog("Holen", `system.Exec("wget -q -O - 'http://h/x?a=1'");`)
	prog("Senden", `system.Exec("curl -s -k -X POST -H 'Content-Type: application/json' -d '{\"a\":1}' https://h/api");`)
	prog("Formular", `system.Exec("curl -s -d a=1 -d b=2 http://h/");`)
	prog("Text", `system.Exec("curl -s -H 'Content-Type: text/plain' -d hallo http://h/");`)
	prog("Wecken", `system.Exec("ether-wake -i eth0 aa:bb:cc:00:11:22");`)
	prog("Wecken dort", `system.Exec("wakeonlan -i 192.168.1.255 AA:BB:CC:00:11:22");`)
	prog("Befehl", `system.Exec("/usr/local/addons/mein/skript.sh an");`)
	prog("Root", `system.Exec("echo 1 > /etc/config/x");`)
	prog("Ausgabe", `string o; string e; system.Exec("cat /tmp/wert", &o, &e); dom.GetObject("Antwort").State(o);`)
	prog("Ausgabe per ID", `string o; system.Exec("cat /tmp/wert", &o); dom.GetObject(`+strconv.Itoa(text.ID)+`).State(o);`)
	prog("Ausgabe doppelt", `string o; system.Exec("cat /tmp/wert", &o); dom.GetObject("Doppelt").State(o);`)
	// what a webhook cannot say: a login, a placeholder's look, a file
	prog("Login", `system.Exec("wget -q -O - http://user:pass@h/");`)
	prog("Klammern", `system.Exec("curl -s 'http://h/?v={value}'");`)
	prog("Klammern im Kopf", `system.Exec("curl -s -H 'X: {time}' http://h/");`)
	prog("Datei", `system.Exec("wget -q -O /tmp/x http://h/");`)
	prog("Methode", `system.Exec("curl -X DELETE http://h/");`)
	prog("Mac kaputt", `system.Exec("ether-wake 00:00:00:00:00:0g");`)
	// what stays a script
	prog("In einen Alarm", `string o; system.Exec("cat x", &o); dom.GetObject(`+strconv.Itoa(alarm.ID)+`).State(o);`)
	prog("In nichts", `string o; system.Exec("cat x", &o); dom.GetObject("Gibt es nicht").State(o);`)
	prog("Zu lang", `system.Exec("`+strings.Repeat("x", model.MaxCommandLine+1)+`");`)
	prog("Zwei", `system.Exec("a"); system.Exec("b");`)
	// a delayed one keeps its delay
	b.program("Später", true, ruleSpec{blocks: [][]regadom.Line{{line}}, actions: []regadom.Action{{Param: regadom.DestScript, TargetID: regadom.None, Value: val(`system.Exec("ether-wake AA:BB:CC:00:11:22");`, str), Delay: val("00:00:30", regadom.TypeDelay)}}})
	res := b.run(t, Options{})

	first := func(name string) (*model.Action, *ProgramReport) {
		t.Helper()
		r := programReport(res, name)
		if r == nil || r.Key == "" {
			t.Fatalf("%s: %+v", name, r)
		}
		return &program(res, r.Key).Rules[0].Then[0], r
	}
	for name, want := range map[string]model.Webhook{
		"Holen":    {Method: "GET", URL: "http://h/x?a=1"},
		"Senden":   {Method: "POST", URL: "https://h/api", BodyType: model.BodyJSON, Body: `{"a":1}`, Insecure: true},
		"Formular": {Method: "POST", URL: "http://h/", BodyType: model.BodyForm, Body: "a=1&b=2"},
		"Text":     {Method: "POST", URL: "http://h/", BodyType: model.BodyText, Body: "hallo", Headers: []model.Header{{Name: "Content-Type", Value: "text/plain"}}},
	} {
		a, r := first(name)
		if a.Kind != model.ActionWebhook || a.Webhook.Method != want.Method || a.Webhook.URL != want.URL || a.Webhook.Body != want.Body || a.Webhook.BodyType != want.BodyType || a.Webhook.Insecure != want.Insecure || len(a.Webhook.Headers) != len(want.Headers) {
			t.Errorf("%s: %+v", name, a.Webhook)
		}
		if r.Status != Imported || !r.Active || !has(r.Notes, NoteExecWebhook) {
			t.Errorf("%s: %+v", name, r)
		}
	}
	for name, want := range map[string]model.Wake{"Wecken": {MAC: "AA:BB:CC:00:11:22"}, "Wecken dort": {MAC: "AA:BB:CC:00:11:22", Broadcast: "192.168.1.255"}, "Später": {MAC: "AA:BB:CC:00:11:22"}} {
		a, r := first(name)
		if a.Kind != model.ActionWake || *a.Wake != want || r.Status != Imported || !has(r.Notes, NoteExecWake) {
			t.Errorf("%s: %+v %+v", name, a, r)
		}
	}
	if a, _ := first("Später"); a.After != model.Duration(30*time.Second) {
		t.Errorf("the delay is gone: %+v", a)
	}
	for name, want := range map[string]model.Command{
		"Befehl":           {Line: "/usr/local/addons/mein/skript.sh an"},
		"Root":             {Line: "echo 1 > /etc/config/x"},
		"Ausgabe":          {Line: "cat /tmp/wert", Output: "antwort"},
		"Ausgabe per ID":   {Line: "cat /tmp/wert", Output: "antwort"},
		"Ausgabe doppelt":  {Line: "cat /tmp/wert", Output: "doppelt"},
		"Login":            {Line: "wget -q -O - http://user:pass@h/"},
		"Klammern":         {Line: "curl -s 'http://h/?v={value}'"},
		"Klammern im Kopf": {Line: "curl -s -H 'X: {time}' http://h/"},
		"Datei":            {Line: "wget -q -O /tmp/x http://h/"},
		"Methode":          {Line: "curl -X DELETE http://h/"},
		"Mac kaputt":       {Line: "ether-wake 00:00:00:00:00:0g"},
	} {
		a, r := first(name)
		if a.Kind != model.ActionCommand || !a.Command.Shell || !a.Command.Wait || a.Command.Line != want.Line || a.Command.Output != want.Output || len(a.Command.Args) != 0 {
			t.Errorf("%s: %+v %+v", name, a, a.Command)
		}
		if r.Status != Inactive || r.Active || codes(r.Reasons) != ReasonCommand || has(r.Notes, NoteCommandRoot) != (name == "Root") {
			t.Errorf("%s: %+v", name, r)
		}
	}
	for _, name := range []string{"In einen Alarm", "In nichts", "Zu lang", "Zwei"} {
		a, r := first(name)
		if a.Kind != model.ActionScript || codes(r.Reasons) != ReasonScript {
			t.Errorf("%s: %+v %+v", name, a, r)
		}
	}
	md := res.Report.Markdown("de") + res.Report.Markdown("en")
	for _, want := range []string{"wurde zur Aktivität „Befehl“", "became the activity \"command\"", "wurde zum Webhook", "became a webhook", "wurde zu Wake-on-LAN", "became a Wake-on-LAN", "das root braucht", "that needs root", "activity: system.Exec (command): 11", "activity: system.Exec (webhook): 4", "activity: system.Exec (Wake-on-LAN): 3"} {
		if !strings.Contains(md, want) {
			t.Errorf("the report lacks %q", want)
		}
	}
}
