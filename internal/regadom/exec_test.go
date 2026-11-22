package regadom

import (
	"reflect"
	"testing"
)

func TestWords(t *testing.T) {
	for _, c := range []struct {
		line string
		want []string
		ok   bool
	}{
		{`wget -q -O - 'http://h/x?a=1&b=2'`, []string{"wget", "-q", "-O", "-", "http://h/x?a=1&b=2"}, true},
		{`  curl	"http://h/a b"  ''  a\ b  x"y"'z' `, []string{"curl", "http://h/a b", "", "a b", "xyz"}, true},
		{`echo "a \"b\" \\ \x"`, []string{"echo", `a "b" \ \x`}, true},
		{`echo a#b ~x x~`, nil, false},
		{`echo a~b a#b`, []string{"echo", "a~b", "a#b"}, true},
		{``, nil, false},
		{`   `, nil, false},
		{`echo 'a`, nil, false},
		{`echo "a`, nil, false},
		{`echo a\`, nil, false},
		{`echo "$HOME"`, nil, false},
		{"echo \"`id`\"", nil, false},
		{`echo $HOME`, nil, false},
		{`a | b`, nil, false},
		{`a > /tmp/x`, nil, false},
		{`a; b`, nil, false},
		{`a && b`, nil, false},
		{`a #b`, nil, false},
		{"a\nb", nil, false},
	} {
		got, ok := words(c.line)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %q %v", c.line, got, ok)
		}
	}
}

func TestExecFetch(t *testing.T) {
	for _, c := range []struct {
		line string
		want *Fetch
	}{
		{`wget -q -O - 'http://h/x?a=1&b=2'`, &Fetch{Method: "GET", URL: "http://h/x?a=1&b=2"}},
		{`/usr/bin/wget -qO- http://h/`, &Fetch{Method: "GET", URL: "http://h/"}},
		{`wget --quiet --no-check-certificate -T 5 --tries=1 -O /dev/null https://h/`, &Fetch{Method: "GET", URL: "https://h/", Insecure: true}},
		{`wget -q -O - --post-data='a=1&b=2' --header='X-Key: abc' -U Mozilla http://h/`, &Fetch{Method: "POST", URL: "http://h/", Body: "a=1&b=2", Headers: [][2]string{{"X-Key", "abc"}, {"User-Agent", "Mozilla"}}}},
		{`wget --method PUT --post-data x h/p`, &Fetch{Method: "PUT", URL: "http://h/p", Body: "x"}},
		{`curl -s http://h/`, &Fetch{Method: "GET", URL: "http://h/"}},
		{`/usr/local/addons/cuxd/curl -sSkL -m 5 --connect-timeout 2 -o /dev/null "https://h/a"`, &Fetch{Method: "GET", URL: "https://h/a", Insecure: true}},
		{`curl -X POST -H "Content-Type: application/json" -d '{"a": 1}' http://h/`, &Fetch{Method: "POST", URL: "http://h/", Body: `{"a": 1}`, JSON: true}},
		{`curl -H 'Content-Type: text/xml' --data-raw '@x' -d y --url http://h/`, &Fetch{Method: "POST", URL: "http://h/", Body: "@x&y", Typed: true, Headers: [][2]string{{"Content-Type", "text/xml"}}}},
		{`curl --request=put --data=x -A agent http://h/`, &Fetch{Method: "PUT", URL: "http://h/", Body: "x", Headers: [][2]string{{"User-Agent", "agent"}}}},

		{`wget -q -O /tmp/file http://h/`, nil},
		{`curl -o out.html http://h/`, nil},
		{`curl -d @file http://h/`, nil},
		{`curl -u user:pass http://h/`, nil},
		{`curl -H Kaputt http://h/`, nil},
		{`curl http://h/ http://i/`, nil},
		{`curl -s`, nil},
		{`curl -m`, nil},
		{`curl --max-time`, nil},
		{`curl --silent=1 http://h/`, nil},
		{`curl --nix http://h/`, nil},
		{`wget -q -O - http://h/ | grep x`, nil},
		{`ls http://h/`, nil},
	} {
		got, ok := ExecCall{Command: c.line}.Fetch()
		if ok != (c.want != nil) || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v %v", c.line, got, ok)
		}
	}
}

func TestExecWake(t *testing.T) {
	for _, c := range []struct {
		line, mac, broadcast string
		ok                   bool
	}{
		{"ether-wake AA:BB:CC:00:11:22", "AA:BB:CC:00:11:22", "", true},
		{"/usr/sbin/ether-wake -b -i eth0 aa:bb:cc:00:11:22", "aa:bb:cc:00:11:22", "", true},
		{"etherwake -D -i 192.168.1.255 AABBCC001122", "AABBCC001122", "", true},
		{"wakeonlan -i 192.168.1.255 -p 9 AA-BB-CC-00-11-22", "AA-BB-CC-00-11-22", "192.168.1.255", true},
		{"wol -i fe80::1 AA:BB:CC:00:11:22", "AA:BB:CC:00:11:22", "", true},
		{"wakeonlan -p 7 AA:BB:CC:00:11:22", "", "", false},
		{"ether-wake", "", "", false},
		{"ether-wake -x AA:BB:CC:00:11:22", "", "", false},
		{"ether-wake rechner", "", "", false},
		{"ether-wake AA:BB:CC:00:11:22 && echo ok", "", "", false},
		{"ping AA:BB:CC:00:11:22", "", "", false},
	} {
		mac, broadcast, ok := ExecCall{Command: c.line}.Wake()
		if ok != c.ok || mac != c.mac || broadcast != c.broadcast {
			t.Errorf("%s: %s %s %v", c.line, mac, broadcast, ok)
		}
	}
}

func TestExecNeedsRoot(t *testing.T) {
	for line, want := range map[string]bool{
		"/usr/bin/curl -s http://h/":             false,
		"wget -q -O /dev/null http://h/":         false,
		"echo 1 > /tmp/x":                        false,
		"/usr/local/addons/tool/run.sh":          false,
		"echo 1 > /usr/local/etc/config/x":       true,
		"cat /etc/config/homematic.regadom":      true,
		"echo 1 > /sys/class/leds/x/brightness":  true,
		"reboot":                                 true,
		"/sbin/reboot":                           true,
		"sleep 5; killall rfd":                   true,
		"mount -o remount,rw /":                  true,
		"/bin/sh /usr/local/addons/x/update.sh":  false,
		"date -s 12:00":                          true,
		"date":                                   false,
		"echo \"$(date)\" >> /usr/local/tmp/log": false,
	} {
		if got := (ExecCall{Command: line}).NeedsRoot(); got != want {
			t.Errorf("%s: %v", line, got)
		}
	}
}

// A script that is one system.Exec is written anew as a command of its kind.
func TestAnonymisedExec(t *testing.T) {
	a := &anon{out: &Anonymised{}, names: map[string]string{}, perKind: map[string]int{}}
	for script, want := range map[string]string{
		`system.Exec("wget -q -O - http://fritz.box/geheim");`:                                    `system.Exec("wget -q -O - http://anonymised.invalid/");`,
		`system.Exec("ether-wake AA:BB:CC:00:11:22");`:                                            `system.Exec("ether-wake 02:00:00:00:00:01");`,
		`system.Exec("/usr/local/addons/mein/skript.sh geheim");`:                                 `system.Exec("anonymised-command");`,
		`system.Exec("echo geheim > /etc/config/x");`:                                             `system.Exec("/etc/anonymised-command");`,
		`string o; system.Exec("curl -s http://h/geheim", &o); dom.GetObject("Wetter").State(o);`: `string out; string err; system.Exec("anonymised-command", &out, &err); dom.GetObject("Variable 1").State(out);`,
		`string o; system.Exec("reboot", &o); dom.GetObject(4711).State(o);`:                      `string out; string err; system.Exec("/etc/anonymised-command", &out, &err); dom.GetObject(4711).State(out);`,
		"WriteLine(1);": "! anonymised script (lines: 1, characters: 13)",
	} {
		got := a.script(script)
		if got != want {
			t.Errorf("%s:\n  %s", script, got)
		}
		// what it wrote is recognised as the same kind again
		was, _ := Exec(script)
		now, ok := Exec(got)
		_, wasFetch := was.Fetch()
		_, nowFetch := now.Fetch()
		_, _, wasWake := was.Wake()
		_, _, nowWake := now.Wake()
		if script != "WriteLine(1);" && (!ok || was.Output != now.Output || (!was.Output && (wasFetch != nowFetch || wasWake != nowWake)) || was.NeedsRoot() != now.NeedsRoot()) {
			t.Errorf("%s: another kind after the anonymisation: %s", script, got)
		}
	}
}
