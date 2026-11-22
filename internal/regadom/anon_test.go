package regadom

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
)

func anonymise(t testing.TB, in []byte) ([]byte, *Anonymised) {
	t.Helper()
	var out bytes.Buffer
	done, err := Anonymise(bytes.NewReader(in), &out)
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes(), done
}

// The anonymised databases keep every object and lose every word somebody typed; anonymising
// once more changes nothing.
func TestAnonymiseFixtures(t *testing.T) {
	leaks := map[string][]string{
		"fixture": {"Heizperiode", "zzL_", "Wasser im Keller", "Markise", "t.State(", "system.Date", "Regensensor", "Erdgeschoss", "ruft andere", "$src$"},
		"devices": {"LEQ0123456", "00111BE9AB9012", "es regnet", "trocken", "RF_ADDRESS"},
	}
	for name, words := range leaks {
		in, err := os.ReadFile("testdata/" + name + ".regadom")
		if err != nil {
			t.Fatal(err)
		}
		out, done := anonymise(t, in)
		for _, w := range words {
			if !bytes.Contains(in, []byte(w)) && w != "RF_ADDRESS" {
				t.Errorf("%s: the test's word %q is not in the source", name, w)
			}
			if bytes.Contains(out, []byte(w)) {
				t.Errorf("%s: %q is still there", name, w)
			}
		}
		if done.Kind != "regadom" || done.Names == 0 || done.Texts == 0 {
			t.Errorf("%s: %+v", name, done)
		}
		was, err := Parse(bytes.NewReader(in))
		if err != nil {
			t.Fatal(err)
		}
		now, err := Parse(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if len(now.Programs) != len(was.Programs) || len(now.Variables) != len(was.Variables) || len(now.Devices) != len(was.Devices) ||
			len(now.Channels) != len(was.Channels) || len(now.Datapoints) != len(was.Datapoints) || len(now.Lines) != len(was.Lines) ||
			len(now.Actions) != len(was.Actions) || len(now.Calendars) != len(was.Calendars) || len(now.Rooms) != len(was.Rooms) || len(now.Names) != len(was.Names) {
			t.Errorf("%s: objects were lost", name)
		}
		for id, d := range was.Datapoints {
			if now.Datapoints[id] == nil || now.Datapoints[id].Param != d.Param {
				t.Errorf("%s: datapoint %d", name, id)
			}
		}
		again, _ := anonymise(t, out)
		if !bytes.Equal(again, out) {
			t.Errorf("%s: anonymising the anonymised file changes it", name)
		}
	}
}

// A backup gives its database, and its firmware in the account.
func TestAnonymiseBackup(t *testing.T) {
	dev, _ := os.ReadFile("testdata/devices.regadom")
	b := sbk(t, map[string][]byte{"usr/local/etc/config/homematic.regadom": dev, "usr/local/etc/config/time.conf": []byte("LATITUDE=52.5\n")},
		map[string][]byte{"firmware_version": []byte("VERSION=3.89.8.20260719\n")})
	out, done := anonymise(t, b)
	bare, _ := anonymise(t, dev)
	if done.Kind != "sbk" || done.Firmware != "3.89.8.20260719" || !bytes.Equal(out, bare) {
		t.Errorf("%+v", done)
	}
	if _, err := Anonymise(strings.NewReader("<html/>"), &bytes.Buffer{}); err == nil {
		t.Error("no error for a file that is no database")
	}
	if _, err := Read(bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}
}

func obj(id int, name string, meta ...string) string {
	var b strings.Builder
	b.WriteString("<obj>\n<id>" + itoa(id) + "</id>\n<name>" + name + "</name>\n")
	if len(meta) > 0 {
		b.WriteString("<metadata>\n<count>" + itoa(len(meta)/2) + "</count>\n")
		for i := 0; i+1 < len(meta); i += 2 {
			b.WriteString("<property>" + meta[i] + "</property>\n<value>" + meta[i+1] + "</value>\n")
		}
		b.WriteString("</metadata>\n")
	}
	b.WriteString("</obj>\n")
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

// Every rule of the anonymiser, on a hand-written database.
func TestAnonymiseRules(t *testing.T) {
	in := `<?xml version="1.0" encoding="iso-8859-1" ?>
<dom>
` + obj(1, "DOM") + `<objmap>
<enummap>
<enum>` + obj(1100, "Wohnzimmer") + `<entype>1</entype><eninfo>das gro&#223;e</eninfo></enum>
<enum>` + obj(1101, "Licht") + `<entype>3</entype><eninfo></eninfo></enum>
<enum>` + obj(1102, "Sonstiges") + `<entype>7</entype></enum>
<enum>` + obj(101, "Rooms") + `<entype>1</entype></enum>
</enummap>
<favoritemap><favorite><enum>` + obj(1103, "Meine Favoriten") + `<entype>4</entype></enum></favorite></favoritemap>
<interfacemap><ifc>` + obj(1010, "BidCos-RF") + `<ifc-url>xmlrpc_bin://192.168.1.5:32001</ifc-url><ifc-info>BidCos-RF</ifc-info></ifc>
<ifc>` + obj(1011, "VirtualDevices") + `<ifc-url>xmlrpc://ccu.fritz.box:39292/groups</ifc-url><ifc-info>meine Gruppen!</ifc-info></ifc>
<ifc>` + obj(1012, "Odd") + `<ifc-url>not a url</ifc-url></ifc></interfacemap>
<devicemap>
<device>` + obj(1200, "Rollladen K&#252;che MEQ0123456", "DEVDESC", "[ADDRESS:\"MEQ0123456\",RF_ADDRESS:1234567]", "a b&#252;", "x") + `<devadr>MEQ0123456</devadr><devlabel>HM-LC-Bl1-FM</devlabel></device>
<device>` + obj(1201, "Taster") + `<devadr>3014F711A0001F98A9A75A2C</devadr><devlabel>mein Ger&#228;t</devlabel></device>
<device>` + obj(1202, "Wetter") + `<devadr>CUX2801001</devadr><devlabel>HM-WDS40-TH-I</devlabel></device>
<device>` + obj(1203, "Gruppe") + `<devadr>INT0000001</devadr></device>
<device>` + obj(1204, "Fremd") + `<devadr>stra&#223;e 5</devadr></device>
</devicemap>
<channelmap>
<channel>` + obj(1300, "Rollladen K&#252;che") + `<chnadr>MEQ0123456:1</chnadr><chnlabel>BLIND</chnlabel></channel>
<channel>` + obj(1301, "HmIP") + `<chnadr>000A1B2C3D4E5F:3</chnadr><chnlabel>KEY</chnlabel></channel>
</channelmap>
<hssdpmap>
<hss-dp><dp>` + obj(1400, "BidCos-RF.MEQ0123456:1.LEVEL", "VALUE_LIST", "OFF;ON", "UNIT", "100%", "TYPE", "FLOAT", "MIN", "0.000000") + `</dp><hss-adr>MEQ0123456:1</hss-adr><hss-id>LEVEL</hss-id></hss-dp>
<hss-dp><dp>` + obj(1401, "mein Datenpunkt", "VALUE_LIST", "gr&#252;n;rot", "UNIT", "sehr lange Einheit", "SPECIAL", "geheim") + `</dp><hss-adr>MEQ0123456:1</hss-adr><hss-id>mein &#228;</hss-id></hss-dp>
</hssdpmap>
<alarmdpmap>
<alarm-dp><dp>` + obj(1500, "AL-MEQ0123456:1.LOWBAT") + `<dp-info></dp-info></dp></alarm-dp>
<alarm-dp><dp>` + obj(1501, "WatchDog: hmipserver-restart") + `<dp-info>HMIPServer restarted</dp-info></dp></alarm-dp>
<alarm-dp><dp>` + obj(1502, "Alarm Keller", "BV0", "trocken", "BV1", "nass") + `<dp-info>Wasser</dp-info></dp><objcond><condition>` + obj(1503, "AlarmCondition") + `</condition></objcond></alarm-dp>
</alarmdpmap>
<vardpmap>
<var-dp><dp>` + obj(950, "${sysVarPresence}", "BV0", "${sysVarPresenceNotPresent}", "BV1", "${sysVarPresencePresent}") + `<dp-info>${sysVarPresenceMsg}</dp-info></dp><var>1</var><varType>2</varType></var-dp>
<var-dp><dp>` + obj(1600, "svEnergyCounter_1300_MEQ0123456:1") + `<dp-info></dp-info></dp><var>12.500000</var><varType>4</varType></var-dp>
<var-dp><dp>` + obj(1601, "svEnergyCounter_1300_K&#252;che") + `</dp><var>1</var><varType>4</varType></var-dp>
<var-dp><dp>` + obj(1602, "DutyCycle") + `<dp-info>DutyCycle CCU</dp-info></dp><var>3</var><varType>4</varType></var-dp>
<var-dp><dp>` + obj(1603, "Modus", "VALUE_LIST", "Urlaub;Zuhause;;Urlaub;${x}", "UNIT", "&lt;&amp;&gt;\t&#176;C", "MIN", "0", "pos", "a b") + `<dp-info>wie wir leben</dp-info></dp><var>1</var><varType>16</varType></var-dp>
<var-dp><dp>` + obj(1604, "PIN") + `</dp><var>4711</var><varType>20</varType></var-dp>
<var-dp><dp>` + obj(1605, "Nachricht") + `</dp><var>Hallo Welt</var><varType>20</varType></var-dp>
<var-dp><dp>` + obj(1606, "Leer") + `</dp><var></var><varType>20</varType></var-dp>
<var-dp><dp>` + obj(1607, "Nachricht") + `</dp><var>Hallo Welt</var><varType>4</varType></var-dp>
<var-dp><dp>` + obj(1608, "${sysVarAlarmMessages}") + `</dp><var>0</var><varType>16</varType></var-dp>
<var-dp><dp>` + obj(1609, "") + `</dp></var-dp>
</vardpmap>
<programmap>
<program>` + obj(1700, "Licht an") + `<prginfo>abends</prginfo><prgrule><rule>` + obj(1701, "PrgRule") + `</rule></prgrule></program>
<program>` + obj(1702, "Licht an") + `<prginfo>${x}</prginfo></program>
<program>` + obj(1703, "prgEnergyCounter_1300_MEQ0123456:1") + `<prginfo></prginfo></program>
</programmap>
<calendardpmap><calendar-dp><timer-dp><dp>` + obj(1750, "Zeitmodul") + `</dp></timer-dp></calendar-dp>
<calendar-dp><timer-dp><dp>` + obj(1751, "mein Zeitmodul") + `</dp></timer-dp></calendar-dp></calendardpmap>
<sdestmap>
<sdestination>` + obj(1800, "SD0") + `<trgval><val>dom.GetObject("Licht an").ProgramExecute();</val><type>20</type></trgval><trgvalparam><val>00:05:00</val><type>20</type></trgvalparam><param>20</param></sdestination>
<sdestination>` + obj(1801, "SD1") + `<trgval><val>var p = dom.GetObject('Fremdes Programm'); p.Active(true);</val><type>20</type></trgval><param>20</param></sdestination>
<sdestination>` + obj(1802, "SD2") + `<trgval><val>dom.GetObject(1700).Active(false);</val><type>20</type></trgval><param>20</param></sdestination>
<sdestination>` + obj(1803, "SD3") + `<trgval><val>string s = "geheim";
WriteLine(s);</val><type>20</type></trgval><param>20</param></sdestination>
<sdestination>` + obj(1804, "SD4") + `<trgval><val>Hallo Welt</val><type>20</type></trgval><param>5</param></sdestination>
<sdestination>` + obj(1805, "SD5") + `<trgval><val>[SV]</val><type>19</type></trgval><param>5</param></sdestination>
<sdestination>` + obj(1806, "SD6") + `<trgval><val>21.5</val><type>4</type></trgval><param>5</param></sdestination>
</sdestmap>
<scondmap><s-cond>` + obj(1900, "SC0") + `<rval1><val>Hallo Welt</val><type>20</type></rval1></s-cond></scondmap>
<usermap><user>` + obj(1004, "Admin") + `<firstname>Erika</firstname><mail>erika@example.org</mail><pwsha512>abc</pwsha512><pwmd5>123</pwmd5><phone>0123</phone><userlevel>8</userlevel></user></usermap>
</objmap>
<val>frei</val>
<dom-gwcfg><gateway-config>` + obj(20, "Gateway configuration") + `<serial-number>12345678</serial-number><latitude>52.5</latitude><longitude>13.4</longitude><gmtoffset>60</gmtoffset><location>Berlin</location><title>Mein Haus</title></gateway-config></dom-gwcfg>
</dom>
`
	out, done := anonymise(t, []byte(in))
	s := string(out)
	for _, want := range []string{
		"<name>DOM</name>", "<name>Raum 1</name>", "<name>Gewerk 1</name>", "<name>Liste 1</name>", "<name>Rooms</name>", "<name>Favorit 1</name>",
		"<eninfo>Beschreibung</eninfo>", "<eninfo></eninfo>",
		"<name>BidCos-RF</name>", "<ifc-url>xmlrpc_bin://127.0.0.1:32001</ifc-url>", "<ifc-info>BidCos-RF</ifc-info>",
		"<ifc-url>xmlrpc://127.0.0.1:39292/groups</ifc-url>", "<ifc-info></ifc-info>", "<ifc-url></ifc-url>",
		"<name>Ger&#228;t 1</name>", "<property>DEVDESC</property>\n<value></value>", "<property></property>\n<value></value>",
		"<devadr>MEQ0000001</devadr>", "<devlabel>HM-LC-Bl1-FM</devlabel>",
		"<devadr>3014F711A000000000000002</devadr>", "<devlabel></devlabel>", "<devadr>CUX2801001</devadr>", "<devadr>INT0000001</devadr>", "<devadr></devadr>",
		"<name>Kanal 1</name>", "<chnadr>MEQ0000001:1</chnadr>", "<chnadr>000A0000000003:3</chnadr>",
		"<name>BidCos-RF.MEQ0000001:1.LEVEL</name>", "<value>OFF;ON</value>", "<value>100%</value>", "<value>FLOAT</value>", "<value>0.000000</value>",
		"<hss-adr>MEQ0000001:1</hss-adr>", "<hss-id>LEVEL</hss-id>",
		"<name>Datenpunkt 1</name>", "<property>VALUE_LIST</property>\n<value></value>", "<property>UNIT</property>\n<value></value>", "<property>SPECIAL</property>\n<value></value>", "<hss-id></hss-id>",
		"<name>AL-MEQ0000001:1.LOWBAT</name>", "<name>WatchDog: hmipserver-restart</name>", "<dp-info>HMIPServer restarted</dp-info>",
		"<name>Alarm 1</name>", "<value>Wert 0</value>", "<value>Wert 1</value>", "<dp-info>Beschreibung</dp-info>", "<name>AlarmCondition</name>",
		"<name>${sysVarPresence}</name>", "<value>${sysVarPresenceNotPresent}</value>", "<dp-info>${sysVarPresenceMsg}</dp-info>",
		"<name>svEnergyCounter_1300_MEQ0000001:1</name>", "<var>12.500000</var>", "<name>Variable 1</name>",
		"<name>DutyCycle</name>", "<dp-info>DutyCycle CCU</dp-info>",
		"<name>Variable 2</name>", "<value>Wert 1;Wert 2;;Wert 1;${x}</value>", "<value>&lt;&amp;&gt;\t&#176;C</value>", "<property>pos</property>\n<value>a b</value>",
		"<name>Variable 3</name>", "<var>Text 1</var>", "<name>Variable 4</name>", "<var>Text 2</var>", "<var></var>", "<name>${sysVarAlarmMessages}</name>", "<name></name>",
		"<name>Programm 1</name>", "<prginfo>Beschreibung</prginfo>", "<name>PrgRule</name>", "<prginfo>${x}</prginfo>", "<name>prgEnergyCounter_1300_MEQ0000001:1</name>",
		"<name>Zeitmodul</name>", "<name>Objekt 1</name>",
		`<val>dom.GetObject(&quot;Programm 1&quot;).ProgramExecute();</val>`, "<val>00:05:00</val>",
		`<val>dom.GetObject(&quot;Programm 2&quot;).Active(true);</val>`, "<val>dom.GetObject(1700).Active(false);</val>",
		"<val>! anonymised script (lines: 2, characters: 34)</val>", "<val>Text 2</val>", "<val>[SV]</val>", "<val>21.5</val>",
		"<name>Benutzer 1</name>", "<firstname></firstname>", "<mail></mail>", "<pwsha512></pwsha512>", "<pwmd5></pwmd5>", "<phone></phone>", "<userlevel>8</userlevel>",
		"<val>Text 3</val>",
		"<serial-number></serial-number>", "<latitude></latitude>", "<longitude></longitude>", "<gmtoffset>60</gmtoffset>", "<location></location>", "<title></title>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Count(s, "<name>Programm 1</name>") != 2 || strings.Count(s, "<name>Variable 4</name>") != 2 {
		t.Error("two objects of one name must share a pseudonym")
	}
	for _, gone := range []string{"Wohnzimmer", "K&#252;che", "Küche", "Erika", "example.org", "geheim", "Hallo", "4711", "Urlaub", "192.168", "fritz", "Mein Haus", "Berlin", "RF_ADDRESS", "1F98A9"} {
		if strings.Contains(s, gone) {
			t.Errorf("%q is still there", gone)
		}
	}
	if done.Names == 0 || done.Addresses != 3 || done.Scripts != 4 || done.Texts == 0 || done.Dropped == 0 {
		t.Errorf("%+v", done)
	}
	again, _ := anonymise(t, out)
	if !bytes.Equal(again, out) {
		t.Errorf("not stable:\n%s", again)
	}
}

func TestKind(t *testing.T) {
	if kind(nil) != "Objekt" || kind([]*node{{name: "dp"}}) != "Objekt" {
		t.Error("an object without a class")
	}
}

func FuzzAnonymise(f *testing.F) {
	for _, name := range []string{"fixture.regadom", "devices.regadom"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte(`<dom><objmap><vardpmap><var-dp><dp><obj><id>1600</id><name>Modus</name><metadata><property>VALUE_LIST</property><value>a;b</value></metadata></obj></dp><var>x</var><varType>20</varType></var-dp></vardpmap><sdestmap><sdestination><trgval><val>dom.GetObject("Modus").Active(true);</val></trgval><param>20</param></sdestination></sdestmap><devicemap><device><obj><id>1200</id><name>MEQ0123456</name></obj><devadr>MEQ0123456</devadr></device></devicemap></objmap></dom>`))
	f.Fuzz(func(t *testing.T, b []byte) {
		var out bytes.Buffer
		if _, err := Anonymise(bytes.NewReader(b), &out); err != nil {
			return
		}
		// what comes out is a database again, and anonymising it changes nothing
		if _, err := Parse(bytes.NewReader(out.Bytes())); err != nil {
			t.Fatalf("the anonymised file does not parse: %v", err)
		}
		var again bytes.Buffer
		if _, err := Anonymise(bytes.NewReader(out.Bytes()), &again); err != nil || !bytes.Equal(again.Bytes(), out.Bytes()) {
			t.Fatalf("not stable (%v)", err)
		}
	})
}
