# Import report

- Source: `ccu` (ReGa database (homematic.regadom))
- Location: not in the source - set it in the settings for astro times

18 programs imported, 7 of them inactive, 2 not imported; 38 system variables imported.

## Programs

| Program | File | Result | Notes |
| --- | --- | --- | --- |
| zzL_Ia5 | `programs/zzl-ia5.yaml` | imported, inactive | holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Ib31 | `programs/zzl-ib31.yaml` | imported, inactive | holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Ic5r | `programs/zzl-ic5r.yaml` | imported, inactive | holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Id5p | `programs/zzl-id5p.yaml` | imported, inactive | holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_Ie7 | `programs/zzl-ie7.yaml` | imported, inactive | holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker |
| zzL_If31r | `programs/zzl-if31r.yaml` | imported, inactive | holds a script - scripts do not run here; the script is kept as text. For such tasks: Node-RED (RedMatic), Home Assistant, ioBroker |
| Markise bei Regen einfahren | `programs/markise-bei-regen-einfahren.yaml` | imported, active | ends running delays for its targets - other programs' too; several activities: always in their listed order here, one after the other; "on update" with an "else": the "else" runs at every report while the condition is false; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Heizperiode schalten | `programs/heizperiode-schalten.yaml` | imported, active | "on change" in an "else if": triggers at every write while an earlier block is true (as on the CCU); the comparison "outside the range": the CCU's editor cannot make it, its behaviour there was not measured; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Vergleiche alle | `programs/vergleiche-alle.yaml` | imported, active | several activities: always in their listed order here, one after the other; "on update" with an "else": the "else" runs at every report while the condition is false; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Alarm bei Wasser | `programs/alarm-bei-wasser.yaml` | imported, active | is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Rollläden morgens | `programs/rolllaeden-morgens.yaml` | imported, active | is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Rollläden abends | `programs/rolllaeden-abends.yaml` | imported, active | astro times are the real sunrise and sunset here; the CCU's are off by up to about ten minutes; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Nachtabsenkung | `programs/nachtabsenkung.yaml` | imported, inactive as on the CCU | – |
| Zisterne prüfen | `programs/zisterne-pruefen.yaml` | imported, active | a time module on "check only": it triggers the program at its times all the same, as on the CCU; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Kalender | `programs/kalender.yaml` | imported, active | astro times are the real sunrise and sunset here; the CCU's are off by up to about ten minutes; a time module on "check only": it triggers the program at its times all the same, as on the CCU; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Szene Abend | `programs/szene-abend.yaml` | imported, active | several activities: always in their listed order here, one after the other; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Wetter holen | `programs/wetter-holen.yaml` | imported, inactive | a `system.Exec` became the activity "command": the command runs with the addon's rights here, not as root. Check the command, then switch the program on |
| Inaktiv und unsichtbar | `programs/inaktiv-und-unsichtbar.yaml` | imported, inactive as on the CCU | has no triggering line: never runs by itself |
| prgEnergyCounter_4711_000A1B2C3D4E5F:6 | – | not imported | one of the CCU's counter programs (energy, rain, sunshine): a device helper replaces it and counts on from the values taken over |
| prgDailySunshineRainCounter_4730 | – | not imported | one of the CCU's counter programs (energy, rain, sunshine): a device helper replaces it and counts on from the values taken over |

## System variables

| System variable | File | Type | Value | Result | Notes |
| --- | --- | --- | --- | --- | --- |
| Anwesenheit | `variables/anwesenheit.yaml` | bool | false | imported | is set to true after every system start, as on the CCU |
| DutyCycle | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |
| zzL_A | `variables/zzl-a.yaml` | bool | true | imported | – |
| zzL_B | `variables/zzl-b.yaml` | bool | true | imported | – |
| zzL_N | `variables/zzl-n.yaml` | number | 50 | imported | – |
| zzL_E | `variables/zzl-e.yaml` | number | 11 | imported | – |
| zzL_L | `variables/zzl-l.yaml` | list (a, b, c) | a | imported | – |
| zzL_S | `variables/zzl-s.yaml` | string | idle | imported | – |
| zzL_T | `variables/zzl-t.yaml` | string | x3:$src$@22:17:20  | imported | – |
| zzL_TT | `variables/zzl-tt.yaml` | string | Ia5E@21 21:56:18 Ib31E@21 21:56:18 Ic5rE@21 21:56:18 Id5pE@2… | imported | – |
| zzL_K | `variables/zzl-k.yaml` | bool | true | imported | a boolean of the subtype "action" |
| Regen | `variables/regen.yaml` | bool | false | imported | – |
| Wassermelder Keller | `variables/wassermelder-keller.yaml` | bool | false | imported | – |
| Heizperiode | `variables/heizperiode.yaml` | list (Sommer, Übergang, Winter) | Übergang | imported | – |
| Rollladen Status | `variables/rollladen-status.yaml` | list (offen, geschlossen, Beschattung) | offen | imported | – |
| Außentemperatur Mittel | `variables/aussentemperatur-mittel.yaml` | number | 12.5 | imported | – |
| Zähler Türklingel | `variables/zaehler-tuerklingel.yaml` | number | 3 | imported | – |
| Markise-Hinweis | `variables/markise-hinweis.yaml` | string | – | imported | – |
| intern Merker | `variables/intern-merker.yaml` | bool | false | imported | – |
| svEnergyCounter_4711_000A1B2C3D4E5F:6 | `variables/svenergycounter-4711-000a1b2c3d4e5f-6.yaml` | number | 100.5 | imported | one of the CCU's counter variables (EnergyCounter); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterOldVal_4711 | `variables/svenergycounteroldval-4711.yaml` | number | 211.75 | imported | one of the CCU's counter variables (EnergyCounterOldVal); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterFeedIn_4711_000A1B2C3D4E5F:6 | `variables/svenergycounterfeedin-4711-000a1b2c3d4e5f-6.yaml` | number | 323 | imported | one of the CCU's counter variables (EnergyCounterFeedIn); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterOldValFeedIn_4711 | `variables/svenergycounteroldvalfeedin-4711.yaml` | number | 434.25 | imported | one of the CCU's counter variables (EnergyCounterOldValFeedIn); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterGas_4720_NEQ0123456:1 | `variables/svenergycountergas-4720-neq0123456-1.yaml` | number | 545.5 | imported | one of the CCU's counter variables (EnergyCounterGas); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterGasOldVal_4720 | `variables/svenergycountergasoldval-4720.yaml` | number | 656.75 | imported | one of the CCU's counter variables (EnergyCounterGasOldVal); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterIEC_4721_NEQ0123456:2 | `variables/svenergycounteriec-4721-neq0123456-2.yaml` | number | 768 | imported | one of the CCU's counter variables (EnergyCounterIEC); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svEnergyCounterIECOldVal_4721 | `variables/svenergycounteriecoldval-4721.yaml` | number | 879.25 | imported | one of the CCU's counter variables (EnergyCounterIECOldVal); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPRainCounter_4730_000B1B2C3D4E5F:1 | `variables/svhmipraincounter-4730-000b1b2c3d4e5f-1.yaml` | number | 990.5 | imported | one of the CCU's counter variables (HmIPRainCounter); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPRainCounterOldVal_4730 | `variables/svhmipraincounteroldval-4730.yaml` | number | 1101.75 | imported | one of the CCU's counter variables (HmIPRainCounterOldVal); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPRainCounterYesterday_4730 | `variables/svhmipraincounteryesterday-4730.yaml` | number | 1213 | imported | one of the CCU's counter variables (HmIPRainCounterYesterday); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPRainCounterToday_4730 | `variables/svhmipraincountertoday-4730.yaml` | number | 1324.25 | imported | one of the CCU's counter variables (HmIPRainCounterToday); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPSunshineCounter_4730_000B1B2C3D4E5F:1 | `variables/svhmipsunshinecounter-4730-000b1b2c3d4e5f-1.yaml` | number | 1435.5 | imported | one of the CCU's counter variables (HmIPSunshineCounter); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPSunshineCounterOldVal_4730 | `variables/svhmipsunshinecounteroldval-4730.yaml` | number | 1546.75 | imported | one of the CCU's counter variables (HmIPSunshineCounterOldVal); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPSunshineCounterYesterday_4730 | `variables/svhmipsunshinecounteryesterday-4730.yaml` | number | 1658 | imported | one of the CCU's counter variables (HmIPSunshineCounterYesterday); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| svHmIPSunshineCounterToday_4730 | `variables/svhmipsunshinecountertoday-4730.yaml` | number | 1546.75 | imported | one of the CCU's counter variables (HmIPSunshineCounterToday); its channel is not in the database: taken over as a plain variable, nobody counts it on |
| Alarmzone 1 | `variables/alarmzone-1.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| WatchDog: hmipserver-highfd | – | – | – | not imported | one of OpenCCU's watchdog alarms |
| WatchDog: no-internet | – | – | – | not imported | one of OpenCCU's watchdog alarms |
| zzL_AL | `variables/zzl-al.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| Alarm Wasser | `variables/alarm-wasser.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| Alarm Einbruch | `variables/alarm-einbruch.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| Gateway-DP | – | – | – | not imported | a system object of the CCU no program uses |
| Alarmmeldungen | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |
| Servicemeldungen | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |

## Devices that programs use

none

## What the database holds

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
