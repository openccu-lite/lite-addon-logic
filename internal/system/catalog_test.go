package system_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"

	"github.com/openccu-lite/lite-addon-logic/internal/fakesys"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// The catalogue lists the devices of every interface but the addon's own, by name, with their
// channels in order and the rooms in the page's language.
func TestCatalogDevices(t *testing.T) {
	fake := fakesys.New()
	defer fake.Close()
	fake.SeedDevices()
	fake.Lite.Interfaces = append(fake.Lite.Interfaces, "Logic", "Down")
	fake.Lite.Devices["Logic"] = []map[string]any{{"ADDRESS": "var1", "TYPE": "VARIABLE"}}
	c := &system.Catalog{Lite: litesource.New(fake.Lite.URL, addonkit.StaticToken("x")), Own: "Logic"}
	devs, err := c.Devices(context.Background(), "de")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range devs {
		got = append(got, d.Interface+" "+d.Address+" "+d.Name+" "+d.Type)
	}
	want := []string{
		"HmIP-RF 000A1B2C3D4E02 Fenster Küche HmIP-SWDO",
		"HmIP-RF 000A1B2C3D4E07 Gong HmIP-MP3P",
		"HmIP-RF 000A1B2C3D4E10 Jalousie Büro HmIP-BBL",
		"HmIP-RF 000A1B2C3D4E09 Lichtband HmIP-RGBW",
		"HmIP-RF 000A1B2C3D4E01 Rollladen Wohnzimmer HmIP-BROLL",
		"HmIP-RF 000A1B2C3D4E08 Sirene HmIP-ASIR",
		"HmIP-RF 000A1B2C3D4E03 Steckdose Flur HmIP-PS",
		"BidCos-RF NEQ0000001 Stehlampe HM-LC-Sw1-Pl",
		"HmIP-RF 000A1B2C3D4E06 Taster Diele HmIP-BSL",
		"HmIP-RF 000A1B2C3D4E04 Thermostat Bad HmIP-STHD",
		"HmIP-RF 000A1B2C3D4E11 Wandtaster Flur HmIP-WRCD",
		"HmIP-RF 000A1B2C3D4E12 Waschmaschine HmIP-PSM",
		"HmIP-RF 000A1B2C3D4E13 Wetterstation HmIP-SWO-PR",
		"HmIP-RF 000A1B2C3D4E05  HmIP-WRC2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("devices:\n%v\nwant\n%v", got, want)
	}
	th := devs[9]
	if len(th.Channels) != 2 || th.Channels[0].Address != "000A1B2C3D4E04:0" || th.Channels[1].Type != "HEATING_CLIMATECONTROL_TRANSCEIVER" {
		t.Fatalf("channels: %+v", th.Channels)
	}
	if ch := th.Channels[1]; ch.Name != "Thermostat Bad" || !reflect.DeepEqual(ch.Rooms, []string{"Bad"}) || !reflect.DeepEqual(ch.Functions, []string{"Heizung"}) {
		t.Errorf("channel: %+v", ch)
	}
	en, _ := c.Devices(context.Background(), "en")
	if r := en[9].Channels[1].Rooms; !reflect.DeepEqual(r, []string{"Bathroom"}) {
		t.Errorf("rooms in English: %v", r)
	}
}

// The datapoints of channels, over several interfaces; an unknown channel is left out.
func TestCatalogParams(t *testing.T) {
	fake := fakesys.New()
	defer fake.Close()
	fake.SeedDevices()
	c := &system.Catalog{Lite: litesource.New(fake.Lite.URL, addonkit.StaticToken("x")), Own: "Logic"}
	got, err := c.Params(context.Background(), []string{"HmIP-RF/000A1B2C3D4E01:2", "HmIP-RF/000A1B2C3D4E02:1", "BidCos-RF/NEQ0000001:1", "HmIP-RF/NOPE:1", "Logic/var:1", "nonsense"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("channels: %v", got)
	}
	b, _ := json.Marshal(got["HmIP-RF/000A1B2C3D4E01:2"])
	if string(b) != `[{"id":"LEVEL","type":"FLOAT","read":true,"write":true,"min":0,"max":1.01,"unit":"100%"},{"id":"STOP","type":"ACTION","read":false,"write":true}]` {
		t.Errorf("shutter: %s", b)
	}
	b, _ = json.Marshal(got["HmIP-RF/000A1B2C3D4E02:1"])
	if string(b) != `[{"id":"STATE","type":"ENUM","read":true,"write":false,"min":0,"max":1,"values":["CLOSED","OPEN"]}]` {
		t.Errorf("contact: %s", b)
	}
}

type brokenSource struct{ ifs []litesource.Interface }

func (b brokenSource) Interfaces(context.Context) ([]litesource.Interface, error) {
	if b.ifs == nil {
		return nil, errors.New("down")
	}
	return b.ifs, nil
}
func (brokenSource) Devices(context.Context, string) ([]litesource.DeviceDescription, error) {
	return []litesource.DeviceDescription{{Address: "B", Type: "T"}, {Address: "A", Type: "T"}, {Address: "A:x", Type: "C", Parent: "A"}, {Address: "Z:1", Type: "C", Parent: "Z"}}, nil
}
func (brokenSource) Names(context.Context) (*litesource.Names, error) { return nil, errors.New("down") }
func (brokenSource) ParamsetsFor(context.Context, string, []string, string) (map[string]string, litesource.ParamsetCache, map[string]string, error) {
	return nil, nil, nil, errors.New("down")
}

// What does not answer: no interface list is an error; without the metadata the devices come by
// address; a channel without its device is left out; descriptions that fail are an error.
func TestCatalogBroken(t *testing.T) {
	c := &system.Catalog{Lite: brokenSource{}}
	if _, err := c.Devices(context.Background(), "de"); err == nil {
		t.Error("no interface list: want an error")
	}
	c = &system.Catalog{Lite: brokenSource{ifs: []litesource.Interface{{Name: "X"}}}}
	devs, err := c.Devices(context.Background(), "de")
	if err != nil || len(devs) != 2 || devs[0].Address != "A" || devs[1].Address != "B" || len(devs[0].Channels) != 1 {
		t.Errorf("devices: %+v %v", devs, err)
	}
	if _, err := c.Params(context.Background(), []string{"X/A:1"}); err == nil {
		t.Error("descriptions down: want an error")
	}
	if got, err := c.Params(context.Background(), nil); err != nil || len(got) != 0 {
		t.Errorf("no channels: %v %v", got, err)
	}
}

// What the editor's forms write fits the devices' own descriptions (internal/fakesys/descriptions,
// each file with its source): a list's entry by its name, a brightness as a share, a duration as
// a number with its unit. And what a description does not have is refused.
func TestFormValuesFitTheDescriptions(t *testing.T) {
	fake := fakesys.New()
	defer fake.Close()
	fake.SeedDevices()
	c := &system.Catalog{Lite: litesource.New(fake.Lite.URL, addonkit.StaticToken("x"))}
	channels := []string{"HmIP-RF/000A1B2C3D4E06:1", "HmIP-RF/000A1B2C3D4E07:1", "HmIP-RF/000A1B2C3D4E07:2", "HmIP-RF/000A1B2C3D4E08:1", "HmIP-RF/000A1B2C3D4E09:1", "HmIP-RF/000A1B2C3D4E10:1"}
	params, err := c.Params(context.Background(), channels)
	if err != nil {
		t.Fatal(err)
	}
	fits := []struct {
		channel string
		values  map[string]any
	}{
		{channels[0], map[string]any{"COLOR": "RED", "LEVEL": 0.5, "COLOR_BEHAVIOUR": "BLINKING_MIDDLE", "DURATION_VALUE": 10.0, "DURATION_UNIT": "M", "RAMP_TIME_VALUE": 2.0, "RAMP_TIME_UNIT": "S"}},
		{channels[1], map[string]any{"SOUNDFILE": "SOUNDFILE_003", "LEVEL": 0.3, "REPETITIONS": "REPETITIONS_002", "DURATION_VALUE": 20.0, "DURATION_UNIT": "S"}},
		{channels[2], map[string]any{"COLOR": "GREEN", "LEVEL": 1.0}},
		{channels[3], map[string]any{"ACOUSTIC_ALARM_SELECTION": "FREQUENCY_RISING", "OPTICAL_ALARM_SELECTION": "DOUBLE_FLASHING_REPEATING", "DURATION_VALUE": 3.0, "DURATION_UNIT": "M"}},
		{channels[4], map[string]any{"HUE": 120.0, "SATURATION": 1.0, "LEVEL": 0.8, "RAMP_TIME_VALUE": 5.0, "RAMP_TIME_UNIT": "S", "EFFECT": "NO_EFFECT"}},
		{channels[4], map[string]any{"COLOR_TEMPERATURE": 2700.0, "LEVEL": 0.4}},
		{channels[5], map[string]any{"LEVEL": 0.5, "LEVEL_2": 0.25}},
	}
	for _, f := range fits {
		if err := system.FitAll(params[f.channel], f.values); err != nil {
			t.Errorf("%s %v: %v", f.channel, f.values, err)
		}
	}
	refused := []struct {
		channel string
		values  map[string]any
		want    string
	}{
		// the light of the HmIP-MP3P has no behaviour, a HmIP-BSL 2.x has
		{channels[2], map[string]any{"COLOR_BEHAVIOUR": "ON"}, "the channel has no datapoint COLOR_BEHAVIOUR"},
		{channels[0], map[string]any{"COLOR": "ORANGE"}, `COLOR has no entry "ORANGE"`},
		{channels[4], map[string]any{"HUE": 400.0}, "HUE: 400 is outside 0 to 361"},
		{channels[4], map[string]any{"COLOR_TEMPERATURE": 500.0}, "COLOR_TEMPERATURE: 500 is outside 1000 to 10200"},
		{channels[5], map[string]any{"LEVEL_2": 1.5}, "LEVEL_2: 1.5 is outside 0 to 1.01"},
		{channels[3], map[string]any{"DURATION_UNIT": "10MS"}, `DURATION_UNIT has no entry "10MS"`},
	}
	for _, r := range refused {
		if err := system.FitAll(params[r.channel], r.values); err == nil || err.Error() != r.want {
			t.Errorf("%s %v: %v, want %s", r.channel, r.values, err, r.want)
		}
	}
}
