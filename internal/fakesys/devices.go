package fakesys

import (
	"embed"
	"encoding/json"
)

// The VALUES descriptions of real devices, each file with its source: what the forms of the
// program editor are built and tested against (signal light, sound, siren, coloured light,
// blind with slats, display), and the counters the device helpers follow (a measuring
// actuator, a weather sensor).
//
//go:embed descriptions/*.json
var descriptions embed.FS

// Description answers a device's VALUES description from descriptions/<name>.json.
func Description(name string) map[string]any {
	raw, err := descriptions.ReadFile("descriptions/" + name + ".json")
	if err != nil {
		panic(err)
	}
	var file struct {
		Values map[string]any `json:"values"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		panic(err)
	}
	return file.Values
}

// SeedDevices gives the fake system a handful of devices with names, rooms and datapoints: what
// the editor's device picker lists in the page's development and the Playwright tests. None of
// them is a device the import fixture uses.
func (s *Server) SeedDevices() {
	l := s.Lite
	l.Interfaces = []string{"HmIP-RF", "BidCos-RF"}
	dev := func(iface, address, typ string, channels ...string) {
		children := []string{}
		for i := range channels {
			children = append(children, address+":"+string(rune('0'+i)))
		}
		l.Devices[iface] = append(l.Devices[iface], map[string]any{"ADDRESS": address, "TYPE": typ, "PARENT": "", "CHILDREN": children, "FIRMWARE": "1.0.0", "VERSION": 1, "PARAMSETS": []string{"MASTER"}})
		for i, ch := range channels {
			l.Devices[iface] = append(l.Devices[iface], map[string]any{"ADDRESS": children[i], "TYPE": ch, "PARENT": address, "PARENT_TYPE": typ, "VERSION": 1, "PARAMSETS": []string{"MASTER", "VALUES"}})
		}
	}
	values := func(iface, typ, channel string, params map[string]any) {
		l.Paramsets[iface+"/"+typ+"/1.0.0/1/"+channel+"/VALUES"] = params
	}
	p := func(typ string, ops int, more ...any) map[string]any {
		out := map[string]any{"TYPE": typ, "OPERATIONS": ops, "FLAGS": 1}
		for i := 0; i+1 < len(more); i += 2 {
			out[more[i].(string)] = more[i+1]
		}
		return out
	}
	maintenance := map[string]any{"LOW_BAT": p("BOOL", 5), "UNREACH": p("BOOL", 5)}

	dev("HmIP-RF", "000A1B2C3D4E01", "HmIP-BROLL", "MAINTENANCE", "SHUTTER_TRANSMITTER", "SHUTTER_VIRTUAL_RECEIVER")
	values("HmIP-RF", "HmIP-BROLL", "MAINTENANCE", maintenance)
	values("HmIP-RF", "HmIP-BROLL", "SHUTTER_TRANSMITTER", map[string]any{
		"LEVEL":          p("FLOAT", 5, "MIN", 0.0, "MAX", 1.01, "UNIT", "100%"),
		"ACTIVITY_STATE": p("ENUM", 5, "MIN", 0, "MAX", 3, "VALUE_LIST", []string{"UNKNOWN", "UP", "DOWN", "STABLE"}),
	})
	values("HmIP-RF", "HmIP-BROLL", "SHUTTER_VIRTUAL_RECEIVER", map[string]any{
		"LEVEL": p("FLOAT", 7, "MIN", 0.0, "MAX", 1.01, "UNIT", "100%"),
		"STOP":  p("ACTION", 2),
	})
	dev("HmIP-RF", "000A1B2C3D4E02", "HmIP-SWDO", "MAINTENANCE", "SHUTTER_CONTACT")
	values("HmIP-RF", "HmIP-SWDO", "MAINTENANCE", maintenance)
	values("HmIP-RF", "HmIP-SWDO", "SHUTTER_CONTACT", map[string]any{
		"STATE": p("ENUM", 5, "MIN", 0, "MAX", 1, "VALUE_LIST", []string{"CLOSED", "OPEN"}),
	})
	dev("HmIP-RF", "000A1B2C3D4E03", "HmIP-PS", "MAINTENANCE", "SWITCH_TRANSMITTER", "SWITCH_VIRTUAL_RECEIVER")
	values("HmIP-RF", "HmIP-PS", "MAINTENANCE", maintenance)
	values("HmIP-RF", "HmIP-PS", "SWITCH_TRANSMITTER", map[string]any{"STATE": p("BOOL", 5)})
	values("HmIP-RF", "HmIP-PS", "SWITCH_VIRTUAL_RECEIVER", map[string]any{
		"STATE":   p("BOOL", 7),
		"ON_TIME": p("FLOAT", 2, "MIN", 0.0, "MAX", 327680.0, "UNIT", "s"),
	})
	dev("HmIP-RF", "000A1B2C3D4E04", "HmIP-STHD", "MAINTENANCE", "HEATING_CLIMATECONTROL_TRANSCEIVER")
	values("HmIP-RF", "HmIP-STHD", "MAINTENANCE", maintenance)
	values("HmIP-RF", "HmIP-STHD", "HEATING_CLIMATECONTROL_TRANSCEIVER", map[string]any{
		"ACTUAL_TEMPERATURE":    p("FLOAT", 5, "MIN", -3276.8, "MAX", 3276.7, "UNIT", "°C"),
		"SET_POINT_TEMPERATURE": p("FLOAT", 7, "MIN", 4.5, "MAX", 30.5, "UNIT", "°C"),
		"HUMIDITY":              p("INTEGER", 5, "MIN", 0, "MAX", 99, "UNIT", "%"),
		// what a thermostat takes together: the mode with its set point, the boost, the window
		"CONTROL_MODE": p("INTEGER", 2, "MIN", 0, "MAX", 3),
		"BOOST_MODE":   p("BOOL", 7),
		"WINDOW_STATE": p("ENUM", 7, "MIN", 0, "MAX", 1, "VALUE_LIST", []string{"CLOSED", "OPEN"}),
	})
	dev("BidCos-RF", "NEQ0000001", "HM-LC-Sw1-Pl", "MAINTENANCE", "SWITCH")
	values("BidCos-RF", "HM-LC-Sw1-Pl", "MAINTENANCE", maintenance)
	values("BidCos-RF", "HM-LC-Sw1-Pl", "SWITCH", map[string]any{"STATE": p("BOOL", 7), "INFO": p("STRING", 5)})
	// a device nobody named yet
	dev("HmIP-RF", "000A1B2C3D4E05", "HmIP-WRC2", "MAINTENANCE", "KEY_TRANSCEIVER")
	values("HmIP-RF", "HmIP-WRC2", "KEY_TRANSCEIVER", map[string]any{"PRESS_SHORT": p("ACTION", 4), "PRESS_LONG": p("ACTION", 4)})

	// the devices with a form of their own, described as real ones are (descriptions/)
	dev("HmIP-RF", "000A1B2C3D4E06", "HmIP-BSL", "MAINTENANCE", "DIMMER_VIRTUAL_RECEIVER")
	values("HmIP-RF", "HmIP-BSL", "DIMMER_VIRTUAL_RECEIVER", Description("bsl-2"))
	dev("HmIP-RF", "000A1B2C3D4E07", "HmIP-MP3P", "MAINTENANCE", "ACOUSTIC_SIGNAL_VIRTUAL_RECEIVER", "DIMMER_VIRTUAL_RECEIVER")
	values("HmIP-RF", "HmIP-MP3P", "ACOUSTIC_SIGNAL_VIRTUAL_RECEIVER", Description("mp3p-sound"))
	values("HmIP-RF", "HmIP-MP3P", "DIMMER_VIRTUAL_RECEIVER", Description("mp3p-light"))
	dev("HmIP-RF", "000A1B2C3D4E08", "HmIP-ASIR", "MAINTENANCE", "ALARM_SWITCH_VIRTUAL_RECEIVER")
	values("HmIP-RF", "HmIP-ASIR", "ALARM_SWITCH_VIRTUAL_RECEIVER", Description("asir"))
	dev("HmIP-RF", "000A1B2C3D4E09", "HmIP-RGBW", "MAINTENANCE", "UNIVERSAL_LIGHT_RECEIVER")
	values("HmIP-RF", "HmIP-RGBW", "UNIVERSAL_LIGHT_RECEIVER", Description("rgbw"))
	dev("HmIP-RF", "000A1B2C3D4E10", "HmIP-BBL", "MAINTENANCE", "BLIND_VIRTUAL_RECEIVER")
	values("HmIP-RF", "HmIP-BBL", "BLIND_VIRTUAL_RECEIVER", Description("bbl"))
	dev("HmIP-RF", "000A1B2C3D4E11", "HmIP-WRCD", "MAINTENANCE", "ACOUSTIC_DISPLAY_RECEIVER")
	values("HmIP-RF", "HmIP-WRCD", "ACOUSTIC_DISPLAY_RECEIVER", Description("wrcd"))

	// the devices with a counter a device helper follows
	dev("HmIP-RF", "000A1B2C3D4E12", "HmIP-PSM", "MAINTENANCE", "ENERGIE_METER_TRANSMITTER")
	values("HmIP-RF", "HmIP-PSM", "ENERGIE_METER_TRANSMITTER", Description("psm-meter"))
	dev("HmIP-RF", "000A1B2C3D4E13", "HmIP-SWO-PR", "MAINTENANCE", "WEATHER_TRANSMIT")
	values("HmIP-RF", "HmIP-SWO-PR", "WEATHER_TRANSMIT", Description("swo-pr"))

	l.Meta["enums"] = map[string]any{
		"room": map[string]any{"name": map[string]string{"de": "Räume", "en": "Rooms"}, "tree": []map[string]any{
			{"id": "wohnzimmer", "name": map[string]string{"de": "Wohnzimmer", "en": "Living room"}},
			{"id": "kueche", "name": map[string]string{"de": "Küche", "en": "Kitchen"}},
			{"id": "bad", "name": map[string]string{"de": "Bad", "en": "Bathroom"}},
			{"id": "flur", "name": map[string]string{"de": "Flur", "en": "Hallway"}},
		}},
		"function": map[string]any{"name": map[string]string{"de": "Gewerke", "en": "Functions"}, "tree": []map[string]any{
			{"id": "heizung", "name": map[string]string{"de": "Heizung", "en": "Heating"}},
		}},
	}
	name := func(ref, name string, enums ...string) { l.SetName(ref, name, enums...) }
	name("HmIP-RF.000A1B2C3D4E01", "Rollladen Wohnzimmer")
	name("HmIP-RF.000A1B2C3D4E01:1", "Rollladen Wohnzimmer Status", "room/wohnzimmer")
	name("HmIP-RF.000A1B2C3D4E01:2", "Rollladen Wohnzimmer", "room/wohnzimmer")
	name("HmIP-RF.000A1B2C3D4E02", "Fenster Küche")
	name("HmIP-RF.000A1B2C3D4E02:1", "Fenster Küche", "room/kueche")
	name("HmIP-RF.000A1B2C3D4E03", "Steckdose Flur")
	name("HmIP-RF.000A1B2C3D4E03:1", "Steckdose Flur Status", "room/flur")
	name("HmIP-RF.000A1B2C3D4E03:2", "Steckdose Flur", "room/flur")
	name("HmIP-RF.000A1B2C3D4E04", "Thermostat Bad")
	name("HmIP-RF.000A1B2C3D4E04:1", "Thermostat Bad", "room/bad", "function/heizung")
	name("BidCos-RF.NEQ0000001", "Stehlampe")
	name("BidCos-RF.NEQ0000001:1", "Stehlampe", "room/wohnzimmer")
	name("HmIP-RF.000A1B2C3D4E06", "Taster Diele")
	name("HmIP-RF.000A1B2C3D4E06:1", "Taster Diele Licht")
	name("HmIP-RF.000A1B2C3D4E07", "Gong")
	name("HmIP-RF.000A1B2C3D4E07:1", "Gong Ton")
	name("HmIP-RF.000A1B2C3D4E07:2", "Gong Licht")
	name("HmIP-RF.000A1B2C3D4E08", "Sirene")
	name("HmIP-RF.000A1B2C3D4E08:1", "Sirene")
	name("HmIP-RF.000A1B2C3D4E09", "Lichtband")
	name("HmIP-RF.000A1B2C3D4E09:1", "Lichtband")
	name("HmIP-RF.000A1B2C3D4E10", "Jalousie Büro")
	name("HmIP-RF.000A1B2C3D4E10:1", "Jalousie Büro")
	name("HmIP-RF.000A1B2C3D4E11", "Wandtaster Flur")
	name("HmIP-RF.000A1B2C3D4E11:1", "Wandtaster Flur Anzeige")
	name("HmIP-RF.000A1B2C3D4E12", "Waschmaschine")
	name("HmIP-RF.000A1B2C3D4E12:1", "Waschmaschine Messung")
	name("HmIP-RF.000A1B2C3D4E13", "Wetterstation")
	name("HmIP-RF.000A1B2C3D4E13:1", "Wetterstation")
}
