# Import report

- Source: `ccu` (ReGa database (homematic.regadom))
- Location: not in the source - set it in the settings for astro times

0 programs imported, 0 of them inactive, 1 not imported; 2 system variables imported.

## Programs

| Program | File | Result | Notes |
| --- | --- | --- | --- |
| ruleTmpProgramName | – | not imported | leftover of an unsaved program in the CCU's editor |

## System variables

| System variable | File | Type | Value | Result | Notes |
| --- | --- | --- | --- | --- | --- |
| Anwesenheit | `variables/anwesenheit.yaml` | bool | true | imported | is set to true after every system start, as on the CCU |
| Alarmzone 1 | `variables/alarmzone-1.yaml` | alarm | – | imported | alarm variable: the database holds no alarm state, it starts as "never triggered" |
| Gateway-DP | – | – | – | not imported | a system object of the CCU no program uses |
| Alarmmeldungen | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |
| Servicemeldungen | – | number | – | not imported | a system value: the system maintains it here; programs that use it read that value |

13 service messages of the CCU were not imported (the system shows its own).

## Devices that programs use

none

## What the database holds

- variable: alarm: 1
- variable: bool: 1
