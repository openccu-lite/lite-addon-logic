package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/importer"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// A device programs name is replaced by another of its kind: listed with what the system and
// the import know of it, tried first, refused with the reason when the new one is not there,
// is another type or lacks a datapoint, and done in one step.
func TestReplaceDeviceRoutes(t *testing.T) {
	e := newEnv(t)
	text := "name: Markise\nrules:\n  - when:\n      any:\n        - all:\n            - device: HmIP-RF/ALT0000000001:1/STATE\n              is: 1\n              trigger: change\n    then:\n      - set: HmIP-RF/ALT0000000001:3/LEVEL\n        to: 0\n      - channel: HmIP-RF/HIER000000001:4\n        values:\n          LEVEL: 1\n"
	p, err := model.DecodeProgram("markise.yaml", "markise", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateProgram(p, repo.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	seen := importer.EncodeSeen([]importer.SeenDevice{
		{Channel: "HmIP-RF/ALT0000000001:1", Name: "Markise Status", Device: "Markise Terrasse", Type: "HmIP-BROLL"},
		{Channel: "HmIP-RF/ALT0000000001:3", Name: "Markise Behang", Device: "Markise Terrasse", Type: "HmIP-BROLL"},
		{Channel: "BidCos-RF/XEQ0000001:1", Name: "Bleibt", Device: "Bleibt", Type: "HM-X"},
	})
	if _, err := e.svc.Tree().WriteExtra(importer.SeenPath, seen); err != nil {
		t.Fatal(err)
	}
	dev := func(addr, name, typ string, channels ...string) system.Device {
		d := system.Device{Interface: "HmIP-RF", Address: addr, Name: name, Type: typ}
		for _, ch := range channels {
			d.Channels = append(d.Channels, system.Channel{Address: addr + ":" + ch, Type: "X"})
		}
		return d
	}
	e.sys.devices = []system.Device{
		dev("HIER000000001", "Hier", "HmIP-BBL", "0", "4"),
		dev("NEU0000000001", "Neue Markise", "HmIP-BROLL", "0", "1", "3"),
		dev("ANDERS0000001", "Schalter", "HmIP-PS", "0", "1", "3"),
		dev("KLEIN00000001", "Kleine", "HmIP-BROLL", "0", "1"),
		dev("ARM0000000001", "Arme", "HmIP-BROLL", "0", "1", "3"),
	}
	e.sys.params = map[string][]system.Param{
		"HmIP-RF/NEU0000000001:1": {{ID: "STATE"}}, "HmIP-RF/NEU0000000001:3": {{ID: "LEVEL"}, {ID: "STOP"}},
		"HmIP-RF/ANDERS0000001:1": {{ID: "STATE"}}, "HmIP-RF/ANDERS0000001:3": {{ID: "LEVEL"}},
		"HmIP-RF/KLEIN00000001:1": {{ID: "STATE"}},
		"HmIP-RF/ARM0000000001:1": {{ID: "STATE"}}, "HmIP-RF/ARM0000000001:3": {{ID: "STOP"}},
	}

	out := e.want(http.StatusOK, "", "GET", "devices/used?lang=de", nil)
	used := out["devices"].([]any)
	if len(used) != 2 || out["system"] != true {
		t.Fatalf("used: %v", out)
	}
	old, here := used[0].(map[string]any), used[1].(map[string]any)
	if old["address"] != "ALT0000000001" || old["present"] != false || old["name"] != "Markise Terrasse" || old["type"] != "HmIP-BROLL" || fmt.Sprint(old["datapoints"]) != "[1/STATE 3/LEVEL]" || fmt.Sprint(old["programs"]) != "[map[key:markise name:Markise]]" {
		t.Errorf("the missing device: %v", old)
	}
	if here["address"] != "HIER000000001" || here["present"] != true || here["name"] != "Hier" || here["type"] != "HmIP-BBL" {
		t.Errorf("the device that is here: %v", here)
	}

	from := map[string]any{"interface": "HmIP-RF", "address": "ALT0000000001"}
	to := func(addr string) map[string]any { return map[string]any{"interface": "HmIP-RF", "address": addr} }
	try := func(addr string) map[string]any {
		t.Helper()
		return e.want(http.StatusOK, "", "POST", "devices/replace", map[string]any{"from": from, "to": to(addr), "dry_run": true})
	}
	for addr, want := range map[string]string{
		"NIRGENDS00001": "the system has no device HmIP-RF/NIRGENDS00001",
		"ANDERS0000001": "is a HmIP-PS, the device it would replace a HmIP-BROLL",
		"KLEIN00000001": "lacks what programs name: channel 3",
		"ARM0000000001": "lacks what programs name: 3/LEVEL",
	} {
		got := try(addr)
		if got["can_apply"] != false || !strings.Contains(fmt.Sprint(got["problems"]), want) {
			t.Errorf("%s: %v", addr, got)
		}
		e.want(http.StatusUnprocessableEntity, "invalid", "POST", "devices/replace", map[string]any{"from": from, "to": to(addr)})
	}
	good := try("NEU0000000001")
	if good["can_apply"] != true || good["references"] != float64(2) || fmt.Sprint(good["programs"]) != "[map[key:markise name:Markise]]" || len(good["problems"].([]any)) != 0 {
		t.Errorf("the dry run: %v", good)
	}
	// what the service refuses, and a system that does not answer
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "devices/replace", map[string]any{"from": from, "to": from})
	e.want(http.StatusNotFound, "not-found", "POST", "devices/replace", map[string]any{"from": to("NIEMAND000001"), "to": to("NEU0000000001")})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "devices/replace", map[string]any{"von": from})
	e.sys.fail = errors.New("down")
	e.want(http.StatusBadGateway, "system", "POST", "devices/replace", map[string]any{"from": from, "to": to("NEU0000000001")})
	if out := e.want(http.StatusOK, "", "GET", "devices/used", nil); out["system"] != false || out["devices"].([]any)[1].(map[string]any)["present"] != false {
		t.Errorf("used, the system down: %v", out)
	}
	e.sys.fail = nil

	e.want(http.StatusOK, "", "POST", "devices/replace", map[string]any{"from": from, "to": to("NEU0000000001")})
	prog, _ := e.svc.Program("markise")
	got := string(model.EncodeProgram(prog.Program, nil))
	if !strings.Contains(got, "device: HmIP-RF/NEU0000000001:1/STATE") || !strings.Contains(got, "set: HmIP-RF/NEU0000000001:3/LEVEL") || !strings.Contains(got, "channel: HmIP-RF/HIER000000001:4") || strings.Contains(got, "ALT0000000001") {
		t.Errorf("the program afterwards:\n%s", got)
	}
	// the import's note about the old device is gone, the other one's stays
	data, _ := e.svc.Tree().Read(importer.SeenPath)
	if left, _ := importer.DecodeSeen(data); len(left) != 1 || left[0].Channel != "BidCos-RF/XEQ0000001:1" {
		t.Errorf("devices-seen afterwards: %s", data)
	}
	out = e.want(http.StatusOK, "", "GET", "devices/used", nil)
	if used := out["devices"].([]any); len(used) != 2 || used[1].(map[string]any)["address"] != "NEU0000000001" || used[1].(map[string]any)["present"] != true {
		t.Errorf("used afterwards: %v", out)
	}
	e.want(http.StatusNotFound, "not-found", "POST", "devices/replace", map[string]any{"from": from, "to": to("NEU0000000001")})

	// a device that is here is replaced by one of its type; the params route failing is said
	e.sys.devices = append(e.sys.devices, dev("HIER000000002", "Hier 2", "HmIP-BBL", "0", "4"))
	e.sys.params["HmIP-RF/HIER000000002:4"] = []system.Param{{ID: "LEVEL"}}
	if got := e.want(http.StatusOK, "", "POST", "devices/replace", map[string]any{"from": to("HIER000000001"), "to": to("HIER000000002"), "dry_run": true}); got["can_apply"] != true {
		t.Errorf("a device that is here: %v", got)
	}
	if got := e.want(http.StatusOK, "", "POST", "devices/replace", map[string]any{"from": to("HIER000000001"), "to": to("NEU0000000001"), "dry_run": true}); got["can_apply"] != false || !strings.Contains(fmt.Sprint(got["problems"]), "HmIP-BBL") {
		t.Errorf("another type than the one that is here: %v", got)
	}

	// without a system nothing can be replaced
	bare := &env{t: t, svc: e.svc, now: e.now}
	bare.srv = newServer(t, Options{Service: e.svc})
	bare.want(http.StatusBadGateway, "system", "POST", "devices/replace", map[string]any{"from": to("HIER000000001"), "to": to("HIER000000002")})
	if out := bare.want(http.StatusOK, "", "GET", "devices/used", nil); out["system"] != false {
		t.Errorf("used without a system: %v", out)
	}
}
