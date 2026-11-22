package system_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/fakesys"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

const program = `name: Fenster auf, Heizung aus
active: true
rules:
  - when:
      all:
        - all:
            - device: HmIP-RF/0000D3C99C6B41:1/STATE
              is: true
              trigger: change
    then:
      - set: HmIP-RF/000A1BE9A1B2C3:1/SET_POINT_TEMPERATURE
        to: 12
      - set: HmIP-RF/000A1BE9A1B2C3:1/BOOST_MODE
        to: false
      - variable: fenster
        to: true
  - else:
      - variable: fenster
        to: false
`

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The whole way, against the fake system: a device reports, the program runs, the writes go
// out through lite-rpc with the datapoints' types, a variable is set; the file's comments carry
// the devices' names.
func TestDeviceEventToDeviceWrite(t *testing.T) {
	fake := fakesys.New()
	defer fake.Close()
	srv := httptest.NewServer(fake.Handler())
	defer srv.Close()
	fake.Lite.Interfaces = []string{"HmIP-RF"}
	now := time.Now().UTC().Format(time.RFC3339)
	fake.Lite.State = []map[string]any{{"interface": "HmIP-RF", "address": "0000D3C99C6B41:1", "datapoint": "STATE", "value": false, "ts": now, "lc": now, "confirmed": true, "source": "event"}}
	fake.Lite.Devices = map[string][]map[string]any{"HmIP-RF": {
		{"ADDRESS": "000A1BE9A1B2C3", "TYPE": "HmIP-eTRV-2", "FIRMWARE": "2.2.8", "VERSION": 1, "CHILDREN": []string{"000A1BE9A1B2C3:1"}},
		{"ADDRESS": "000A1BE9A1B2C3:1", "TYPE": "HEATING_CLIMATECONTROL_TRANSCEIVER", "PARENT": "000A1BE9A1B2C3", "PARENT_TYPE": "HmIP-eTRV-2", "VERSION": 1},
	}}
	fake.Lite.Paramsets = map[string]map[string]any{"HmIP-RF/HmIP-eTRV-2/2.2.8/1/HEATING_CLIMATECONTROL_TRANSCEIVER/VALUES": {
		"SET_POINT_TEMPERATURE": map[string]any{"TYPE": "FLOAT"}, "BOOST_MODE": map[string]any{"TYPE": "BOOL"}}}
	fake.Lite.SetName("HmIP-RF.0000D3C99C6B41:1", "Fenster Küche")
	fake.Lite.SetName("HmIP-RF.000A1BE9A1B2C3", "Heizung Küche")

	lite := litesource.New(srv.URL, addonkit.StaticToken("t"))
	lite.Reconnect.Min, lite.Reconnect.Max = 10*time.Millisecond, 20*time.Millisecond
	writer := &system.Writer{Lite: lite, Gap: time.Millisecond}
	names := &system.Names{Lite: lite}
	svc, err := service.Open(service.Options{Dir: t.TempDir(), Writer: writer, Names: names.Name})
	if err != nil {
		t.Fatal(err)
	}
	feed := &system.Feed{Lite: lite, Sink: svc.Engine(), Own: "Logic", Retry: 20 * time.Millisecond}
	svc.OnChange(feed.Kick)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go feed.Run(ctx)

	if _, err := svc.CreateVariable(&model.Variable{Name: "Fenster", Type: model.VarBool, Visible: true}, nil, repo.Author{}); err != nil {
		t.Fatal(err)
	}
	p, err := model.DecodeProgram("p.yaml", "p", []byte(program))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProgram(p, repo.Author{}); err != nil {
		t.Fatal(err)
	}
	file, _ := svc.Tree().Read("programs/fenster-auf-heizung-aus.yaml")
	if !strings.Contains(string(file), "STATE  # Fenster Küche\n") || !strings.Contains(string(file), "SET_POINT_TEMPERATURE  # Heizung Küche\n") {
		t.Errorf("the file's comments:\n%s", file)
	}
	waitFor(t, "the seed", func() bool { return feed.Seeded() == 1 })
	waitFor(t, "the stream", func() bool { return fake.Lite.Streams() == 1 })
	// the window opens
	res, err := http.Post(srv.URL+"/dev/event", "application/json", strings.NewReader(`{"interface":"HmIP-RF","address":"0000D3C99C6B41:1","param":"STATE","value":true}`))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("the event: %v %v", res, err)
	}
	waitFor(t, "the writes", func() bool { return len(fake.Writes()) == 2 })
	b, _ := json.Marshal(fake.Writes())
	want := `[{"interface":"HmIP-RF","address":"000A1BE9A1B2C3:1","param":"SET_POINT_TEMPERATURE","value":12},{"interface":"HmIP-RF","address":"000A1BE9A1B2C3:1","param":"BOOST_MODE","value":false}]`
	if string(b) != want {
		t.Errorf("writes: %s", b)
	}
	// the temperature went out as a double
	sent := false
	for _, c := range fake.Lite.Calls {
		sent = sent || strings.Contains(c, "setValue")
	}
	if sent {
		t.Error("the kit's fake saw a setValue: the fake system should have answered it")
	}
	if v, _ := svc.Engine().Value("fenster"); v != true {
		t.Errorf("the variable: %v", v)
	}
	in, _ := svc.Engine().Program("fenster-auf-heizung-aus")
	if in.LastRun == nil || in.LastRun.Block != 1 || in.LastRun.Trigger != "device:HmIP-RF/0000D3C99C6B41:1/STATE" || in.Failures != 0 {
		t.Errorf("the run: %+v", in)
	}
	// and closes: SONST
	res, _ = http.Post(srv.URL+"/dev/event", "application/json", strings.NewReader(`{"interface":"HmIP-RF","address":"0000D3C99C6B41:1","param":"STATE","value":false}`))
	_ = res
	waitFor(t, "SONST", func() bool { v, _ := svc.Engine().Value("fenster"); return v == false })
	// a refused write is counted on the program
	if res, _ := http.Get(srv.URL + "/dev/writes"); res.StatusCode != 200 {
		t.Errorf("/dev/writes: %d", res.StatusCode)
	}
	if res, _ := http.Post(srv.URL+"/dev/event", "application/json", strings.NewReader(`{`)); res.StatusCode != 400 {
		t.Errorf("a torn event: %d", res.StatusCode)
	}
	writer.Close()
	_ = svc.Close()
	var _ engine.Writer = writer
}

type failingNames struct{ calls int }

func (f *failingNames) Names(context.Context) (*litesource.Names, error) {
	f.calls++
	return nil, context.DeadlineExceeded
}

// The names: read at most once a minute, the device's name for a channel without one, and no
// comment when the system does not answer.
func TestNames(t *testing.T) {
	fake := fakesys.New()
	defer fake.Close()
	srv := httptest.NewServer(fake.Handler())
	defer srv.Close()
	fake.Lite.SetName("HmIP-RF.A:1", "Kanal")
	fake.Lite.SetName("HmIP-RF.B", "Gerät")
	lite := litesource.New(srv.URL, addonkit.StaticToken("t"))
	clock := time.Now()
	n := &system.Names{Lite: lite, Now: func() time.Time { return clock }}
	for ref, want := range map[model.Ref]string{
		{Interface: "HmIP-RF", Address: "A:1", Param: "STATE"}: "Kanal",
		{Interface: "HmIP-RF", Address: "B:3", Param: "STATE"}: "Gerät",
		{Interface: "HmIP-RF", Address: "C:1", Param: "STATE"}: "",
		{Interface: "HmIP-RF", Address: "D", Param: "STATE"}:   "",
	} {
		if got := n.Name(ref); got != want {
			t.Errorf("%v: %q, want %q", ref, got, want)
		}
	}
	fake.Lite.SetName("HmIP-RF.C:1", "Neu")
	if got := n.Name(model.Ref{Interface: "HmIP-RF", Address: "C:1"}); got != "" {
		t.Errorf("read again within a minute: %q", got)
	}
	clock = clock.Add(2 * time.Minute)
	if got := n.Name(model.Ref{Interface: "HmIP-RF", Address: "C:1"}); got != "Neu" {
		t.Errorf("after a minute: %q", got)
	}
	f := &failingNames{}
	bad := &system.Names{Lite: f}
	first := bad.Name(model.Ref{Interface: "HmIP-RF", Address: "A:1"})
	second := bad.Name(model.Ref{Interface: "HmIP-RF", Address: "A:1"})
	if first != "" || second != "" || f.calls != 1 {
		t.Errorf("a system that does not answer: %d calls", f.calls)
	}
}
