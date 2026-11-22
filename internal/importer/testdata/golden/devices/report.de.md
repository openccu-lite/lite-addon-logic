# Importbericht

- Quelle: `ccu` (ReGa-Datenbank (homematic.regadom))
- Standort: 52.52, 13.40

3 Programme übernommen, 3 davon inaktiv, 1 nicht übernommen; 3 Systemvariablen übernommen.

## Programme

| Programm | Datei | Ergebnis | Hinweise |
| --- | --- | --- | --- |
| Markise bei Wind | `programs/markise-bei-wind.yaml` | übernommen, inaktiv | enthält einen Sonderwert („Letzter Wert“ o. ä.); die Aktivität wird übersprungen, bis sie bearbeitet ist; Geräte fehlen auf diesem System: BidCos-RF/LEQ0123456:1, HmIP-RF/00111BE9AB9012:3; wird beim Systemstart nicht geprüft (so auf der Zentrale eingestellt); Tastendruck-Zeile: löst bei jedem Tastendruck aus; enthält verneinte Zeilen („nicht bei“); beendet laufende Verzögerungen für seine Ziele – auch die anderer Programme; mehrere Aktivitäten: hier immer in der angegebenen Reihenfolge, eine nach der anderen; „bei Aktualisierung“ mit „Sonst“: „Sonst“ läuft bei jeder Meldung, solange die Bedingung nicht erfüllt ist |
| Frostwarnung | `programs/frostwarnung.yaml` | übernommen, inaktiv | verwendet CUxD (CUxD/CUX2801001:1), das es hier nicht gibt |
| Neues Feld | `programs/neues-feld.yaml` | übernommen, inaktiv | die Datenbank enthält ein Feld, das diese Version nicht kennt: program/runlimit; die Datenbank enthält ein Feld, das diese Version nicht kennt: rule/rule-prio; die Datenbank enthält ein Feld, das diese Version nicht kennt: s-cond/hysteresis |
| Verwaist | – | nicht übernommen | verweist auf ein Objekt, das die Datenbank nicht enthält: 2999 |

## Systemvariablen

| Systemvariable | Datei | Typ | Wert | Ergebnis | Hinweise |
| --- | --- | --- | --- | --- | --- |
| Regen | `variables/regen.yaml` | bool | true | übernommen | – |
| Küche Licht | `variables/kueche-licht.yaml` | bool | false | übernommen | – |
| Modus | `variables/modus.yaml` | list (Auto, Manuell, Urlaub) | Urlaub | übernommen | – |

## Geräte, die in Programmen vorkommen

| Kanal | Name auf der Zentrale | Gerät | Räume | auf diesem System | Programme |
| --- | --- | --- | --- | --- | --- |
| `BidCos-RF/LEQ0123456:1` | Taster Flur oben | Taster Flur (HM-PB-2-WM55) | Flur | fehlt | Markise bei Wind |
| `CUxD/CUX2801001:1` | Wetter außen | Wetter (HM-WDS40-TH-I) | – | fehlt | Frostwarnung |
| `HmIP-RF/00111BE9AB9012:3` | Markise Terrasse:3 | Markise Terrasse (HmIP-BROLL) | Terrasse | fehlt | Markise bei Wind |
| `HmIP-RF/00111BE9AB9012:4` | Markise Behang | Markise Terrasse (HmIP-BROLL) | Terrasse, Flur | vorhanden | Markise bei Wind |

## Reparierte Namen

| vorher | jetzt |
| --- | --- |
| KÃ¼che Licht | Küche Licht |

## Was die Datenbank enthält

- activity: delayed: 1
- activity: device: 3
- activity: variable: 3
- block: cancels running delays: 1
- comparison: eq: 4
- comparison: gt: 1
- comparison: lt: 1
- comparison: range: 1
- line: device: 4
- line: negated: 2
- line: variable: 2
- program: 3
- rule: else: 1
- rule: if: 4
- trigger: change: 3
- trigger: check: 1
- trigger: update: 3
- variable: bool: 2
- variable: list: 1
