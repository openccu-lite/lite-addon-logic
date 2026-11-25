package importer

import (
	"fmt"
	"sort"
	"strings"
)

// texts are the report's words: code -> German, English. %s takes the note's arguments joined
// by ", ".
var texts = map[string][2]string{
	"title":           {"Importbericht", "Import report"},
	"source":          {"Quelle", "Source"},
	"kind-sbk":        {"Systemsicherung (.sbk)", "system backup (.sbk)"},
	"kind-regadom":    {"ReGa-Datenbank (homematic.regadom)", "ReGa database (homematic.regadom)"},
	"firmware":        {"Firmware der Zentrale", "The CCU's firmware"},
	"location":        {"Standort", "Location"},
	"location-none":   {"nicht in der Quelle – für Astro-Zeiten in den Einstellungen eintragen", "not in the source - set it in the settings for astro times"},
	"timezone":        {"Zeitzone", "Time zone"},
	"summary":         {"Übernommen: %s (davon %d inaktiv) und %s; nicht übernommen: %s.", "Imported: %s (%d of them inactive) and %s; not imported: %s."},
	"n-programs":      {"%d Programm|%d Programme", "%d program|%d programs"},
	"n-variables":     {"%d Systemvariable|%d Systemvariablen", "%d system variable|%d system variables"},
	"all-inactive-on": {"Gewählt: alle Programme inaktiv übernehmen.", "Chosen: import every program inactive."},
	"programs":        {"Programme", "Programs"},
	"variables":       {"Systemvariablen", "System variables"},
	"devices":         {"Geräte, die in Programmen vorkommen", "Devices that programs use"},
	"repairs":         {"Reparierte Namen", "Repaired names"},
	"histogram":       {"Was die Datenbank enthält", "What the database holds"},
	"events":          {"Abweichungen von der Auslösertabelle der Zentrale", "Differences to the CCU's own trigger table"},
	"services":        {"%d Servicemeldungen der Zentrale wurden nicht übernommen (das System zeigt sie selbst an).", "%d service messages of the CCU were not imported (the system shows its own)."},
	"none":            {"keine", "none"},
	"col-program":     {"Programm", "Program"},
	"col-file":        {"Datei", "File"},
	"col-result":      {"Ergebnis", "Result"},
	"col-notes":       {"Hinweise", "Notes"},
	"col-variable":    {"Systemvariable", "System variable"},
	"col-type":        {"Typ", "Type"},
	"col-value":       {"Wert", "Value"},
	"col-channel":     {"Kanal", "Channel"},
	"col-name":        {"Name auf der Zentrale", "Name on the CCU"},
	"col-device":      {"Gerät", "Device"},
	"col-rooms":       {"Räume", "Rooms"},
	"col-here":        {"auf diesem System", "on this system"},
	"col-was":         {"vorher", "before"},
	"col-now":         {"jetzt", "now"},
	"yes":             {"vorhanden", "present"},
	"no":              {"fehlt", "missing"},
	"st-imported":     {"übernommen", "imported"},
	"st-active":       {"übernommen, aktiv", "imported, active"},
	"st-off":          {"übernommen, inaktiv wie auf der Zentrale", "imported, inactive as on the CCU"},
	"st-inactive":     {"übernommen, inaktiv", "imported, inactive"},
	"st-skipped":      {"nicht übernommen", "not imported"},
	"updated":         {"aktualisiert die Datei eines früheren Imports", "updates the file of an earlier import"},

	ReasonCommand:      {"ein `system.Exec` wurde zur Aktivität „Befehl“: der Befehl läuft hier mit den Rechten des Addons, nicht als root. Bitte den Befehl prüfen, dann das Programm aktivieren", "a `system.Exec` became the activity \"command\": the command runs with the addon's rights here, not as root. Check the command, then switch the program on"},
	ReasonScript:       {"enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker", "holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker"},
	ReasonCUxD:         {"verwendet CUxD (%s), das es hier nicht gibt", "uses CUxD (%s), which does not exist here"},
	ReasonMissing:      {"Geräte fehlen auf diesem System: %s", "devices missing on this system: %s"},
	ReasonSpecial:      {"enthält einen Sonderwert („Letzter Wert“ o. ä.); die Aktivität wird übersprungen, bis sie bearbeitet ist", "holds a special value (\"last value\" or similar); the activity is skipped until it is edited"},
	ReasonUnknownField: {"die Datenbank enthält ein Feld, das diese Version nicht kennt: %s", "the database holds a field this version does not know: %s"},
	ReasonComparison:   {"unbekannter Vergleich (Typ %s)", "unknown comparison (type %s)"},
	ReasonTrigger:      {"unbekannte Auslöseart (Typ %s)", "unknown trigger mode (type %s)"},
	ReasonValue:        {"ein Wert passt nicht zu seinem Ziel: %s", "a value does not fit its target: %s"},
	ReasonOperators:    {"UND und ODER sind in einer Ebene gemischt", "AND and OR are mixed in one level"},
	ReasonDelay:        {"eine Verzögerung ist nicht lesbar: %s", "a delay cannot be read: %s"},
	ReasonAllInactive:  {"auf Wunsch inaktiv (war auf der Zentrale aktiv)", "inactive as chosen (was active on the CCU)"},
	ReasonListIndex:    {"ein Wertelisten-Eintrag existiert nicht: %s", "a value list entry does not exist: %s"},

	SkipCounter:    {"Zählerprogramm der Zentrale (Energie, Regen, Sonnenschein): ein Gerätehelfer ersetzt es und zählt mit den übernommenen Ständen weiter", "one of the CCU's counter programs (energy, rain, sunshine): a device helper replaces it and counts on from the values taken over"},
	SkipLeftover:   {"Rest eines nicht gespeicherten Programms im Editor der Zentrale", "leftover of an unsaved program in the CCU's editor"},
	SkipUnreadable: {"nicht lesbar: %s", "cannot be read: %s"},
	SkipDangling:   {"verweist auf ein Objekt, das die Datenbank nicht enthält: %s", "refers to an object the database does not hold: %s"},
	SkipInvalid:    {"passt nicht ins Dateiformat: %s", "does not fit the file format: %s"},

	NoteAtStart:      {"wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“)", "is evaluated once after every system start, as on the CCU (switch: \"evaluate at system start\")"},
	NoteNotAtStart:   {"wird beim Systemstart nicht geprüft (so auf der Zentrale eingestellt)", "is not evaluated at system start (as set on the CCU)"},
	NoteElseUpdate:   {"„bei Aktualisierung“ mit „Sonst“: „Sonst“ läuft bei jeder Meldung, solange die Bedingung nicht erfüllt ist", "\"on update\" with an \"else\": the \"else\" runs at every report while the condition is false"},
	NoteCancels:      {"beendet laufende Verzögerungen für seine Ziele – auch die anderer Programme", "ends running delays for its targets - other programs' too"},
	NoteOrder:        {"mehrere Aktivitäten: hier immer in der angegebenen Reihenfolge, eine nach der anderen", "several activities: always in their listed order here, one after the other"},
	NoteDST:          {"feste Uhrzeit zwischen 2 und 3 Uhr: am Tag der Zeitumstellung verschoben bzw. nur einmal", "a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change"},
	NoteAstro:        {"Astro-Zeiten sind hier der wirkliche Sonnenauf- und -untergang; die der Zentrale liegen um bis zu etwa zehn Minuten daneben", "astro times are the real sunrise and sunset here; the CCU's are off by up to about ten minutes"},
	NoteKeyLine:      {"Tastendruck-Zeile: löst bei jedem Tastendruck aus", "a key press line: triggers at every press"},
	NoteOutside:      {"Vergleich „außerhalb des Wertebereichs“: im Editor der Zentrale nicht einstellbar, Verhalten dort nicht gemessen", "the comparison \"outside the range\": the CCU's editor cannot make it, its behaviour there was not measured"},
	NoteZeroDelay:    {"eine Verzögerung von 24 Stunden oder mehr hat die Zentrale als null gespeichert: die Aktivität läuft sofort, wie dort", "a delay of 24 hours or more was stored as zero by the CCU: the activity runs at once, as there"},
	NoteElseIfChange: {"„bei Änderung“ in einem „Sonst, wenn“: löst bei jedem Schreiben aus, solange ein früherer Block erfüllt ist (wie auf der Zentrale)", "\"on change\" in an \"else if\": triggers at every write while an earlier block is true (as on the CCU)"},
	NoteNoTrigger:    {"hat keine auslösende Zeile: läuft nie von selbst", "has no triggering line: never runs by itself"},
	NoteTimeCheck:    {"Zeitmodul mit „nur prüfen“: löst das Programm trotzdem zu seinen Zeitpunkten aus, wie auf der Zentrale", "a time module on \"check only\": it triggers the program at its times all the same, as on the CCU"},
	NoteNegated:      {"enthält verneinte Zeilen („nicht bei“)", "holds negated lines (\"not when\")"},
	NoteExecWebhook:  {"ein `system.Exec` mit wget oder curl wurde zum Webhook: die Antwort wird nicht gelesen, und das Programm wartet nicht darauf", "a `system.Exec` with wget or curl became a webhook: the answer is not read, and the program does not wait for it"},
	NoteExecWake:     {"ein `system.Exec` mit ether-wake wurde zu Wake-on-LAN", "a `system.Exec` with ether-wake became a Wake-on-LAN"},
	NoteCommandRoot:  {"der Befehl nennt einen Systempfad oder ein Werkzeug, das root braucht: das gelingt mit den Rechten des Addons nicht", "the command names a system path or a tool that needs root: that fails with the addon's rights"},
	NotePattern:      {"ein Zeitmodul hat ein Muster, das so übernommen wurde, wie es gespeichert ist; bitte prüfen", "a time module has a pattern that was taken as stored; please check it"},

	VarSystemValue:  {"Systemwert: hier pflegt ihn das System selbst; Programme, die ihn verwenden, lesen diesen Wert", "a system value: the system maintains it here; programs that use it read that value"},
	VarPresence:     {"wird nach jedem Systemstart auf „wahr“ gesetzt, wie auf der Zentrale", "is set to true after every system start, as on the CCU"},
	VarCounter:      {"Zählervariable der Zentrale (%s)", "one of the CCU's counter variables (%s)"},
	VarCounterPlain: {"ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter", "its channel is not in the database: taken over as a plain variable, nobody counts it on"},
	VarHelper:       {"ein Gerätehelfer zählt sie weiter (%s)", "a device helper counts it on (%s)"},
	VarHelperState:  {"letzter Zählerstand des Geräts: im Gerätehelfer aufgehoben, keine eigene Variable", "the device's last reading: kept inside the device helper, no variable of its own"},
	VarHelperUnused: {"Hilfsvariable des Zähler-Scripts der Zentrale: nicht mehr nötig", "a scratch variable of the CCU's counter script: no longer needed"},
	VarHelperDay:    {"Tageswert des Gerätehelfers %s (auf der Zentrale in den Kanaldaten)", "a day's amount of the device helper %s (kept with the channel on the CCU)"},
	VarCounterAddr:  {"Name und Datenbank nennen verschiedene Kanäle: %s", "name and database give different channels: %s"},
	VarAlarm:        {"Alarmvariable: die Datenbank enthält keinen Alarmzustand, sie beginnt als „nie ausgelöst“", "alarm variable: the database holds no alarm state, it starts as \"never triggered\""},
	VarEntries:      {"leere oder doppelte Einträge der Werteliste wurden umbenannt", "empty or double entries of the value list were renamed"},
	VarListValue:    {"der gespeicherte Wert (%s) ist kein Eintrag der Liste: erster Eintrag gesetzt", "the stored value (%s) is no entry of the list: set to the first entry"},
	VarAction:       {"Logikwert vom Untertyp „Aktion“", "a boolean of the subtype \"action\""},
	VarLimits:       {"Minimum über Maximum: Grenzen weggelassen", "minimum above maximum: limits left out"},
	SkipWatchdog:    {"Überwachungsalarm von OpenCCU", "one of OpenCCU's watchdog alarms"},
	SkipSystem:      {"Systemobjekt der Zentrale, von keinem Programm verwendet", "a system object of the CCU no program uses"},
	EventMissing:    {"%s: eine Zeile löst darauf aus, die Tabelle der Zentrale nennt es nicht", "%s: a line triggers on it, the CCU's table does not list it"},
	EventExtra:      {"%s: die Tabelle der Zentrale nennt es, keine Zeile löst darauf aus", "%s: the CCU's table lists it, no line triggers on it"},
	SkipUnknownType: {"unbekannter Typ (%s)", "unknown type (%s)"},
}

// count is a number with its noun: the text's two forms, singular|plural, are split at the bar.
func count(lang, code string, n int) string {
	t := texts[code]
	s := t[0]
	if lang == "en" {
		s = t[1]
	}
	one, other, _ := strings.Cut(s, "|")
	if n == 1 {
		return fmt.Sprintf(one, n)
	}
	return fmt.Sprintf(other, n)
}

// Text is a code's words in a language ("de" or "en"), the arguments filled in.
func Text(lang, code string, args ...string) string {
	t, ok := texts[code]
	if !ok {
		return strings.TrimSpace(code + " " + strings.Join(args, ", "))
	}
	s := t[0]
	if lang == "en" {
		s = t[1]
	}
	if strings.Contains(s, "%s") {
		var given []string
		for _, a := range args {
			if a != "" {
				given = append(given, a)
			}
		}
		return fmt.Sprintf(s, strings.Join(given, ", "))
	}
	return s
}

// Template is a code's words with the %s for the arguments left in ("" for a code nobody knows):
// the page fills them itself.
func Template(lang, code string) string {
	t := texts[code]
	if lang == "en" {
		return t[1]
	}
	return t[0]
}

// Codes lists every code the report can carry, sorted: the page checks it has words for each.
func Codes() []string {
	out := make([]string, 0, len(texts))
	for k := range texts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cell(s string) string {
	s = strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(s)
	if s == "" {
		return "–"
	}
	return s
}

func notes(lang string, list ...[]Note) string {
	var out []string
	for _, l := range list {
		for _, n := range l {
			out = append(out, Text(lang, n.Code, n.Args...))
		}
	}
	return strings.Join(out, "; ")
}

// Markdown writes the report for keeping: German or English.
func (r *Report) Markdown(lang string) string {
	if lang != "en" {
		lang = "de"
	}
	t := func(code string, args ...string) string { return Text(lang, code, args...) }
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", t("title"))
	fmt.Fprintf(&b, "- %s: `%s` (%s)\n", t("source"), r.Source, t("kind-"+r.Kind))
	if r.Firmware != "" {
		fmt.Fprintf(&b, "- %s: %s\n", t("firmware"), r.Firmware)
	}
	if r.Location.Latitude != "" || r.Location.Longitude != "" {
		fmt.Fprintf(&b, "- %s: %s, %s\n", t("location"), r.Location.Latitude, r.Location.Longitude)
	} else {
		fmt.Fprintf(&b, "- %s: %s\n", t("location"), t("location-none"))
	}
	if r.Location.Timezone != "" {
		fmt.Fprintf(&b, "- %s: %s\n", t("timezone"), r.Location.Timezone)
	}
	imported, inactive, skipped, variables := r.Totals()
	summary := texts["summary"][0]
	if lang == "en" {
		summary = texts["summary"][1]
	}
	fmt.Fprintf(&b, "\n"+summary+"\n", count(lang, "n-programs", imported+inactive), inactive, count(lang, "n-variables", variables), count(lang, "n-programs", skipped))
	if r.AllInactive {
		fmt.Fprintf(&b, "%s\n", t("all-inactive-on"))
	}

	fmt.Fprintf(&b, "\n## %s\n\n", t("programs"))
	if len(r.Programs) == 0 {
		fmt.Fprintf(&b, "%s\n", t("none"))
	} else {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n| --- | --- | --- | --- |\n", t("col-program"), t("col-file"), t("col-result"), t("col-notes"))
		for _, p := range r.Programs {
			status := t("st-skipped")
			switch {
			case p.Status == Inactive:
				status = t("st-inactive")
			case p.Status == Imported && p.Active:
				status = t("st-active")
			case p.Status == Imported:
				status = t("st-off")
			}
			file := ""
			if p.Key != "" {
				file = "`programs/" + p.Key + ".yaml`"
			}
			n := notes(lang, p.Reasons, p.Notes)
			if p.Updated {
				n = strings.TrimPrefix(t("updated")+"; "+n, "; ")
				n = strings.TrimSuffix(n, "; ")
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", cell(p.Name), cell(file), status, cell(n))
		}
	}

	fmt.Fprintf(&b, "\n## %s\n\n", t("variables"))
	if len(r.Variables) == 0 {
		fmt.Fprintf(&b, "%s\n", t("none"))
	} else {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n| --- | --- | --- | --- | --- | --- |\n", t("col-variable"), t("col-file"), t("col-type"), t("col-value"), t("col-result"), t("col-notes"))
		for _, v := range r.Variables {
			status, file := t("st-skipped"), ""
			if v.Status == Imported {
				status, file = t("st-imported"), "`variables/"+v.Key+".yaml`"
			}
			typ := v.Type
			if len(v.Entries) > 0 {
				typ += " (" + strings.Join(v.Entries, ", ") + ")"
			}
			n := notes(lang, v.Notes)
			if v.Updated {
				n = strings.TrimSuffix(t("updated")+"; "+n, "; ")
			}
			value := v.Value
			if len([]rune(value)) > 60 {
				value = string([]rune(value)[:60]) + "…"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", cell(v.Name), cell(file), cell(typ), cell(value), status, cell(n))
		}
	}
	if r.Services > 0 {
		s := texts["services"][0]
		if lang == "en" {
			s = texts["services"][1]
		}
		fmt.Fprintf(&b, "\n"+s+"\n", r.Services)
	}

	fmt.Fprintf(&b, "\n## %s\n\n", t("devices"))
	if len(r.Devices) == 0 {
		fmt.Fprintf(&b, "%s\n", t("none"))
	} else {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n| --- | --- | --- | --- | --- | --- |\n", t("col-channel"), t("col-name"), t("col-device"), t("col-rooms"), t("col-here"), t("programs"))
		for _, d := range r.Devices {
			here := t("no")
			if d.Known {
				here = t("yes")
			}
			dev := strings.TrimSpace(d.Device + " (" + d.Type + ")")
			if d.Type == "" {
				dev = d.Device
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s |\n", d.Channel, cell(d.Name), cell(dev), cell(strings.Join(d.Rooms, ", ")), here, cell(strings.Join(d.Programs, ", ")))
		}
	}

	if len(r.Repairs) > 0 {
		fmt.Fprintf(&b, "\n## %s\n\n| %s | %s |\n| --- | --- |\n", t("repairs"), t("col-was"), t("col-now"))
		for _, rp := range r.Repairs {
			fmt.Fprintf(&b, "| %s | %s |\n", cell(rp.Before), cell(rp.After))
		}
	}
	if len(r.Events) > 0 {
		fmt.Fprintf(&b, "\n## %s\n\n", t("events"))
		for _, e := range r.Events {
			fmt.Fprintf(&b, "- %s\n", t(e.Code, e.Args...))
		}
	}
	fmt.Fprintf(&b, "\n## %s\n\n", t("histogram"))
	for _, c := range r.Histogram {
		fmt.Fprintf(&b, "- %s: %d\n", c.What, c.N)
	}
	return b.String()
}
