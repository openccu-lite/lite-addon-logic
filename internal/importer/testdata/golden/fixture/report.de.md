# Importbericht

- Quelle: `ccu` (ReGa-Datenbank (homematic.regadom))
- Standort: nicht in der Quelle – für Astro-Zeiten in den Einstellungen eintragen

Übernommen: 18 Programme (davon 7 inaktiv) und 38 Systemvariablen; nicht übernommen: 2 Programme.

## Programme

| Programm | Datei | Ergebnis | Hinweise |
| --- | --- | --- | --- |
| zzL_Ia5 | `programs/zzl-ia5.yaml` | übernommen, inaktiv | enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Ib31 | `programs/zzl-ib31.yaml` | übernommen, inaktiv | enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Ic5r | `programs/zzl-ic5r.yaml` | übernommen, inaktiv | enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Id5p | `programs/zzl-id5p.yaml` | übernommen, inaktiv | enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Ie7 | `programs/zzl-ie7.yaml` | übernommen, inaktiv | enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_If31r | `programs/zzl-if31r.yaml` | übernommen, inaktiv | enthält ein Script – Scripts laufen hier nicht; das Script bleibt als Text erhalten. Für solche Aufgaben: Node-RED (RedMatic), Home Assistant, ioBroker |
| Markise bei Regen einfahren | `programs/markise-bei-regen-einfahren.yaml` | übernommen, aktiv | beendet laufende Verzögerungen für seine Ziele – auch die anderer Programme; mehrere Aktivitäten: hier immer in der angegebenen Reihenfolge, eine nach der anderen; „bei Aktualisierung“ mit „Sonst“: „Sonst“ läuft bei jeder Meldung, solange die Bedingung nicht erfüllt ist; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Heizperiode schalten | `programs/heizperiode-schalten.yaml` | übernommen, aktiv | „bei Änderung“ in einem „Sonst, wenn“: löst bei jedem Schreiben aus, solange ein früherer Block erfüllt ist (wie auf der Zentrale); Vergleich „außerhalb des Wertebereichs“: im Editor der Zentrale nicht einstellbar, Verhalten dort nicht gemessen; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Vergleiche alle | `programs/vergleiche-alle.yaml` | übernommen, aktiv | mehrere Aktivitäten: hier immer in der angegebenen Reihenfolge, eine nach der anderen; „bei Aktualisierung“ mit „Sonst“: „Sonst“ läuft bei jeder Meldung, solange die Bedingung nicht erfüllt ist; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Alarm bei Wasser | `programs/alarm-bei-wasser.yaml` | übernommen, aktiv | wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Rollläden morgens | `programs/rolllaeden-morgens.yaml` | übernommen, aktiv | wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Rollläden abends | `programs/rolllaeden-abends.yaml` | übernommen, aktiv | Astro-Zeiten sind hier der wirkliche Sonnenauf- und -untergang; die der Zentrale liegen um bis zu etwa zehn Minuten daneben; beendet laufende Verzögerungen für seine Ziele – auch die anderer Programme; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Nachtabsenkung | `programs/nachtabsenkung.yaml` | übernommen, inaktiv wie auf der Zentrale | – |
| Zisterne prüfen | `programs/zisterne-pruefen.yaml` | übernommen, aktiv | Zeitmodul mit „nur prüfen“: löst das Programm trotzdem zu seinen Zeitpunkten aus, wie auf der Zentrale; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Kalender | `programs/kalender.yaml` | übernommen, aktiv | Astro-Zeiten sind hier der wirkliche Sonnenauf- und -untergang; die der Zentrale liegen um bis zu etwa zehn Minuten daneben; Zeitmodul mit „nur prüfen“: löst das Programm trotzdem zu seinen Zeitpunkten aus, wie auf der Zentrale; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Szene Abend | `programs/szene-abend.yaml` | übernommen, aktiv | mehrere Aktivitäten: hier immer in der angegebenen Reihenfolge, eine nach der anderen; wird nach jedem Systemstart einmal geprüft, wie auf der Zentrale (abschaltbar: „beim Systemstart prüfen“) |
| Wetter holen | `programs/wetter-holen.yaml` | übernommen, inaktiv | ein `system.Exec` wurde zur Aktivität „Befehl“: der Befehl läuft hier mit den Rechten des Addons, nicht als root. Bitte den Befehl prüfen, dann das Programm aktivieren |
| Inaktiv und unsichtbar | `programs/inaktiv-und-unsichtbar.yaml` | übernommen, inaktiv wie auf der Zentrale | hat keine auslösende Zeile: läuft nie von selbst |
| prgEnergyCounter_4711_000A1B2C3D4E5F:6 | – | nicht übernommen | Zählerprogramm der Zentrale (Energie, Regen, Sonnenschein): ein Gerätehelfer ersetzt es und zählt mit den übernommenen Ständen weiter |
| prgDailySunshineRainCounter_4730 | – | nicht übernommen | Zählerprogramm der Zentrale (Energie, Regen, Sonnenschein): ein Gerätehelfer ersetzt es und zählt mit den übernommenen Ständen weiter |

## Systemvariablen

| Systemvariable | Datei | Typ | Wert | Ergebnis | Hinweise |
| --- | --- | --- | --- | --- | --- |
| Anwesenheit | `variables/anwesenheit.yaml` | bool | false | übernommen | wird nach jedem Systemstart auf „wahr“ gesetzt, wie auf der Zentrale |
| DutyCycle | – | number | – | nicht übernommen | Systemwert: hier pflegt ihn das System selbst; Programme, die ihn verwenden, lesen diesen Wert |
| zzL_A | `variables/zzl-a.yaml` | bool | true | übernommen | – |
| zzL_B | `variables/zzl-b.yaml` | bool | true | übernommen | – |
| zzL_N | `variables/zzl-n.yaml` | number | 50 | übernommen | – |
| zzL_E | `variables/zzl-e.yaml` | number | 11 | übernommen | – |
| zzL_L | `variables/zzl-l.yaml` | list (a, b, c) | a | übernommen | – |
| zzL_S | `variables/zzl-s.yaml` | string | idle | übernommen | – |
| zzL_T | `variables/zzl-t.yaml` | string | x3:$src$@22:17:20  | übernommen | – |
| zzL_TT | `variables/zzl-tt.yaml` | string | Ia5E@21 21:56:18 Ib31E@21 21:56:18 Ic5rE@21 21:56:18 Id5pE@2… | übernommen | – |
| zzL_K | `variables/zzl-k.yaml` | bool | true | übernommen | Logikwert vom Untertyp „Aktion“ |
| Regen | `variables/regen.yaml` | bool | false | übernommen | – |
| Wassermelder Keller | `variables/wassermelder-keller.yaml` | bool | false | übernommen | – |
| Heizperiode | `variables/heizperiode.yaml` | list (Sommer, Übergang, Winter) | Übergang | übernommen | – |
| Rollladen Status | `variables/rollladen-status.yaml` | list (offen, geschlossen, Beschattung) | offen | übernommen | – |
| Außentemperatur Mittel | `variables/aussentemperatur-mittel.yaml` | number | 12.5 | übernommen | – |
| Zähler Türklingel | `variables/zaehler-tuerklingel.yaml` | number | 3 | übernommen | – |
| Markise-Hinweis | `variables/markise-hinweis.yaml` | string | – | übernommen | – |
| intern Merker | `variables/intern-merker.yaml` | bool | false | übernommen | – |
| svEnergyCounter_4711_000A1B2C3D4E5F:6 | `variables/svenergycounter-4711-000a1b2c3d4e5f-6.yaml` | number | 100.5 | übernommen | Zählervariable der Zentrale (EnergyCounter); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterOldVal_4711 | `variables/svenergycounteroldval-4711.yaml` | number | 211.75 | übernommen | Zählervariable der Zentrale (EnergyCounterOldVal); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterFeedIn_4711_000A1B2C3D4E5F:6 | `variables/svenergycounterfeedin-4711-000a1b2c3d4e5f-6.yaml` | number | 323 | übernommen | Zählervariable der Zentrale (EnergyCounterFeedIn); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterOldValFeedIn_4711 | `variables/svenergycounteroldvalfeedin-4711.yaml` | number | 434.25 | übernommen | Zählervariable der Zentrale (EnergyCounterOldValFeedIn); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterGas_4720_NEQ0123456:1 | `variables/svenergycountergas-4720-neq0123456-1.yaml` | number | 545.5 | übernommen | Zählervariable der Zentrale (EnergyCounterGas); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterGasOldVal_4720 | `variables/svenergycountergasoldval-4720.yaml` | number | 656.75 | übernommen | Zählervariable der Zentrale (EnergyCounterGasOldVal); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterIEC_4721_NEQ0123456:2 | `variables/svenergycounteriec-4721-neq0123456-2.yaml` | number | 768 | übernommen | Zählervariable der Zentrale (EnergyCounterIEC); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svEnergyCounterIECOldVal_4721 | `variables/svenergycounteriecoldval-4721.yaml` | number | 879.25 | übernommen | Zählervariable der Zentrale (EnergyCounterIECOldVal); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPRainCounter_4730_000B1B2C3D4E5F:1 | `variables/svhmipraincounter-4730-000b1b2c3d4e5f-1.yaml` | number | 990.5 | übernommen | Zählervariable der Zentrale (HmIPRainCounter); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPRainCounterOldVal_4730 | `variables/svhmipraincounteroldval-4730.yaml` | number | 1101.75 | übernommen | Zählervariable der Zentrale (HmIPRainCounterOldVal); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPRainCounterYesterday_4730 | `variables/svhmipraincounteryesterday-4730.yaml` | number | 1213 | übernommen | Zählervariable der Zentrale (HmIPRainCounterYesterday); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPRainCounterToday_4730 | `variables/svhmipraincountertoday-4730.yaml` | number | 1324.25 | übernommen | Zählervariable der Zentrale (HmIPRainCounterToday); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPSunshineCounter_4730_000B1B2C3D4E5F:1 | `variables/svhmipsunshinecounter-4730-000b1b2c3d4e5f-1.yaml` | number | 1435.5 | übernommen | Zählervariable der Zentrale (HmIPSunshineCounter); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPSunshineCounterOldVal_4730 | `variables/svhmipsunshinecounteroldval-4730.yaml` | number | 1546.75 | übernommen | Zählervariable der Zentrale (HmIPSunshineCounterOldVal); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPSunshineCounterYesterday_4730 | `variables/svhmipsunshinecounteryesterday-4730.yaml` | number | 1658 | übernommen | Zählervariable der Zentrale (HmIPSunshineCounterYesterday); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| svHmIPSunshineCounterToday_4730 | `variables/svhmipsunshinecountertoday-4730.yaml` | number | 1546.75 | übernommen | Zählervariable der Zentrale (HmIPSunshineCounterToday); ihr Kanal steht nicht in der Datenbank: als gewöhnliche Variable übernommen, niemand zählt sie weiter |
| Alarmzone 1 | `variables/alarmzone-1.yaml` | alarm | – | übernommen | Alarmvariable: die Datenbank enthält keinen Alarmzustand, sie beginnt als „nie ausgelöst“ |
| WatchDog: hmipserver-highfd | – | – | – | nicht übernommen | Überwachungsalarm von OpenCCU |
| WatchDog: no-internet | – | – | – | nicht übernommen | Überwachungsalarm von OpenCCU |
| zzL_AL | `variables/zzl-al.yaml` | alarm | – | übernommen | Alarmvariable: die Datenbank enthält keinen Alarmzustand, sie beginnt als „nie ausgelöst“ |
| Alarm Wasser | `variables/alarm-wasser.yaml` | alarm | – | übernommen | Alarmvariable: die Datenbank enthält keinen Alarmzustand, sie beginnt als „nie ausgelöst“ |
| Alarm Einbruch | `variables/alarm-einbruch.yaml` | alarm | – | übernommen | Alarmvariable: die Datenbank enthält keinen Alarmzustand, sie beginnt als „nie ausgelöst“ |
| Gateway-DP | – | – | – | nicht übernommen | Systemobjekt der Zentrale, von keinem Programm verwendet |
| Alarmmeldungen | – | number | – | nicht übernommen | Systemwert: hier pflegt ihn das System selbst; Programme, die ihn verwenden, lesen diesen Wert |
| Servicemeldungen | – | number | – | nicht übernommen | Systemwert: hier pflegt ihn das System selbst; Programme, die ihn verwenden, lesen diesen Wert |

## Geräte, die in Programmen vorkommen

keine

## Was die Datenbank enthält

- activity: delayed: 5
- activity: program (activate): 2
- activity: program (deactivate): 1
- activity: program (run): 1
- activity: script: 12
- activity: system.Exec (command): 1
- activity: variable: 18
- block: cancels running delays: 3
- comparison: eq: 10
- comparison: ge: 2
- comparison: gt: 2
- comparison: le: 1
- comparison: lt: 2
- comparison: outside: 1
- comparison: range: 1
- line: time module: 19
- line: variable: 19
- program: 18
- rule: else: 12
- rule: else if: 2
- rule: if: 18
- time module: daily: 6
- time module: interval: 9
- time module: monthly: 1
- time module: once: 1
- time module: weekly: 1
- time module: yearly: 1
- trigger: change: 12
- trigger: check: 7
- trigger: update: 19
- variable: alarm: 4
- variable: bool: 7
- variable: list: 3
- variable: number: 20
- variable: string: 4
