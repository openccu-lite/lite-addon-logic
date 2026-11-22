# Import report

- Source: `ccu` (ReGa database (homematic.regadom))
- Location: not in the source - set it in the settings for astro times

28 programs imported, 0 of them inactive, 0 not imported; 7 system variables imported.

## Programs

| Program | File | Result | Notes |
| --- | --- | --- | --- |
| Bool1OnFalseUpdate | `programs/bool1onfalseupdate.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Bool1OnFalse | `programs/bool1onfalse.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Bool1OnTrue | `programs/bool1ontrue.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Bool1OnTrueUpdate | `programs/bool1ontrueupdate.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeEveryMinute | `programs/timeeveryminute.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0100 | `programs/time0100.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Key16Key17 | `programs/key16key17.yaml` | imported, active | a key press line: triggers at every press; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Key1 | `programs/key1.yaml` | imported, active | a key press line: triggers at every press; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0130 | `programs/time0130.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0155 | `programs/time0155.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0200 | `programs/time0200.yaml` | imported, active | a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0205 | `programs/time0205.yaml` | imported, active | a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0230 | `programs/time0230.yaml` | imported, active | a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0255 | `programs/time0255.yaml` | imported, active | a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0300 | `programs/time0300.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0305 | `programs/time0305.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0330 | `programs/time0330.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0000 | `programs/time0000.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| Time0030 | `programs/time0030.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpan0100-0103 | `programs/timespan0100-0103.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpan0158-0202 | `programs/timespan0158-0202.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpan0230-0200 | `programs/timespan0230-0200.yaml` | imported, active | a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpan0230-0232 | `programs/timespan0230-0232.yaml` | imported, active | a fixed time between 2 and 3 o'clock: shifted, or only once, on the day the clocks change; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpanFullday | `programs/timespanfullday.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpanDay | `programs/timespanday.yaml` | imported, active | astro times are the real sunrise and sunset here; the CCU's are off by up to about ten minutes; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpanNight | `programs/timespannight.yaml` | imported, active | astro times are the real sunrise and sunset here; the CCU's are off by up to about ten minutes; ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeEvery10s | `programs/timeevery10s.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |
| TimeSpan2235-2305 | `programs/timespan2235-2305.yaml` | imported, active | ends running delays for its targets - other programs' too; is evaluated once after every system start, as on the CCU (switch: "evaluate at system start") |

## System variables

| System variable | File | Type | Value | Result | Notes |
| --- | --- | --- | --- | --- | --- |
| Anwesenheit | `variables/anwesenheit.yaml` | bool | true | imported | is set to true after every system start, as on the CCU |
| VarBool1 | `variables/varbool1.yaml` | bool | false | imported | – |
| VarEnum1 | `variables/varenum1.yaml` | list (Wert 1, Wert 2, Wert 3) | Wert 1 | imported | – |
| VarNum1 | `variables/varnum1.yaml` | number | 0 | imported | – |
| VarString1 | `variables/varstring1.yaml` | string | ??? | imported | – |
| DutyCycle | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |
| Alarmzone 1 | `variables/alarmzone-1.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| VarAlarm1 | `variables/varalarm1.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| Gateway-DP | – | – | – | not imported | a system object of the CCU no program uses |
| Alarmmeldungen | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |
| Servicemeldungen | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |

## Devices that programs use

| Channel | Name on the CCU | Device | Rooms | on this system | Programs |
| --- | --- | --- | --- | --- | --- |
| `BidCos-RF/BidCoS-RF:1` | HM-RCV-50 BidCoS-RF:1 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Key1 |
| `BidCos-RF/BidCoS-RF:11` | HM-RCV-50 BidCoS-RF:11 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0100 |
| `BidCos-RF/BidCoS-RF:12` | HM-RCV-50 BidCoS-RF:12 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Bool1OnTrue |
| `BidCos-RF/BidCoS-RF:13` | HM-RCV-50 BidCoS-RF:13 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Bool1OnFalse |
| `BidCos-RF/BidCoS-RF:14` | HM-RCV-50 BidCoS-RF:14 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Bool1OnFalseUpdate |
| `BidCos-RF/BidCoS-RF:15` | HM-RCV-50 BidCoS-RF:15 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Bool1OnTrueUpdate |
| `BidCos-RF/BidCoS-RF:16` | HM-RCV-50 BidCoS-RF:16 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Key16Key17 |
| `BidCos-RF/BidCoS-RF:17` | HM-RCV-50 BidCoS-RF:17 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Key16Key17 |
| `BidCos-RF/BidCoS-RF:2` | HM-RCV-50 BidCoS-RF:2 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Key1 |
| `BidCos-RF/BidCoS-RF:20` | HM-RCV-50 BidCoS-RF:20 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0130 |
| `BidCos-RF/BidCoS-RF:21` | HM-RCV-50 BidCoS-RF:21 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0155 |
| `BidCos-RF/BidCoS-RF:22` | HM-RCV-50 BidCoS-RF:22 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0200 |
| `BidCos-RF/BidCoS-RF:23` | HM-RCV-50 BidCoS-RF:23 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0205 |
| `BidCos-RF/BidCoS-RF:24` | HM-RCV-50 BidCoS-RF:24 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0230 |
| `BidCos-RF/BidCoS-RF:25` | HM-RCV-50 BidCoS-RF:25 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0255 |
| `BidCos-RF/BidCoS-RF:26` | HM-RCV-50 BidCoS-RF:26 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0300 |
| `BidCos-RF/BidCoS-RF:27` | HM-RCV-50 BidCoS-RF:27 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0305 |
| `BidCos-RF/BidCoS-RF:28` | HM-RCV-50 BidCoS-RF:28 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0330 |
| `BidCos-RF/BidCoS-RF:29` | HM-RCV-50 BidCoS-RF:29 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0000 |
| `BidCos-RF/BidCoS-RF:30` | HM-RCV-50 BidCoS-RF:30 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | Time0030 |
| `BidCos-RF/BidCoS-RF:31` | HM-RCV-50 BidCoS-RF:31 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpan0100-0103 |
| `BidCos-RF/BidCoS-RF:32` | HM-RCV-50 BidCoS-RF:32 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpan0158-0202 |
| `BidCos-RF/BidCoS-RF:33` | HM-RCV-50 BidCoS-RF:33 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpan0230-0200 |
| `BidCos-RF/BidCoS-RF:34` | HM-RCV-50 BidCoS-RF:34 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpan0230-0232 |
| `BidCos-RF/BidCoS-RF:35` | HM-RCV-50 BidCoS-RF:35 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpanFullday |
| `BidCos-RF/BidCoS-RF:36` | HM-RCV-50 BidCoS-RF:36 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpanDay |
| `BidCos-RF/BidCoS-RF:37` | HM-RCV-50 BidCoS-RF:37 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpanNight |
| `BidCos-RF/BidCoS-RF:38` | HM-RCV-50 BidCoS-RF:38 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeEvery10s |
| `BidCos-RF/BidCoS-RF:39` | HM-RCV-50 BidCoS-RF:39 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeSpan2235-2305 |
| `BidCos-RF/BidCoS-RF:50` | HM-RCV-50 BidCoS-RF:50 | HM-RCV-50 BidCoS-RF (HM-RCV-50) | – | present | TimeEveryMinute |

## What the database holds

- activity: device: 28
- block: cancels running delays: 28
- comparison: eq: 6
- line: device: 2
- line: time module: 22
- line: variable: 4
- program: 28
- rule: if: 28
- time module: daily: 20
- time module: interval: 2
- trigger: change: 2
- trigger: update: 26
- variable: alarm: 2
- variable: bool: 2
- variable: list: 1
- variable: number: 1
- variable: string: 1
