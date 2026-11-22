package regadom

import "testing"

func TestExec(t *testing.T) {
	for _, c := range []struct {
		script string
		want   ExecCall
		ok     bool
	}{
		{`system.Exec("wget -q -O - 'http://h/x?a=1'");`, ExecCall{Command: "wget -q -O - 'http://h/x?a=1'"}, true},
		{"  system . Exec ( 'ether-wake AA:BB:CC:00:11:22' )  \n", ExecCall{Command: "ether-wake AA:BB:CC:00:11:22"}, true},
		{`var x=system.Exec("/bin/hm_autoconf");`, ExecCall{Command: "/bin/hm_autoconf"}, true},
		{`x = system.Exec("a");`, ExecCall{Command: "a"}, true},
		{`system.Exec("echo \"a\\b\"\n\t\r");`, ExecCall{Command: "echo \"a\\b\"\n\t\r"}, true},
		{"! holt das Wetter\nstring out; string err;\nsystem.Exec(\"curl -s http://h/\", &out, &err);\n", ExecCall{Command: "curl -s http://h/"}, true},
		{`string out; string err; system.Exec("wget -q -O - http://example.invalid/wetter", &out, &err); dom.GetObject("Markise-Hinweis").State(out);`,
			ExecCall{Command: "wget -q -O - http://example.invalid/wetter", Output: true, OutName: "Markise-Hinweis"}, true},
		{`var o = ""; system.Exec("x", &o); (dom.GetObject(1234)).State(o)`, ExecCall{Command: "x", Output: true, OutID: 1234}, true},
		{`string a; string b; system.Exec("x", &a, &b); dom.GetObject('V').State(a);`, ExecCall{Command: "x", Output: true, OutName: "V"}, true},

		{`system.Exec("");`, ExecCall{}, false},
		{`system.Exec("a" # b);`, ExecCall{}, false},
		{`system.Exec(cmd);`, ExecCall{}, false},
		{`system.Exec("a"); system.Exec("b");`, ExecCall{}, false},
		{`system.Exec("a"); WriteLine("x");`, ExecCall{}, false},
		{`if (x) { system.Exec("a"); }`, ExecCall{}, false},
		{`string a; string b; system.Exec("x", &a, &b); dom.GetObject("V").State(b);`, ExecCall{}, false},
		{`system.Exec("x"); dom.GetObject("V").State(a);`, ExecCall{}, false},
		{`string a; system.Exec("x", &a); (dom.GetObject("V").State(a);`, ExecCall{}, false},
		{`dom.GetObject("CUxD.CUX2801001:1.CMD_EXEC").State("x");`, ExecCall{}, false},
		{`WriteLine(1);`, ExecCall{}, false},
	} {
		got, ok := Exec(c.script)
		if ok != c.ok || got != c.want {
			t.Errorf("%s:\n  %+v %v", c.script, got, ok)
		}
	}
}
