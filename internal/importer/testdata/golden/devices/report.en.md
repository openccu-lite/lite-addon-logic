# Import report

- Source: `ccu` (ReGa database (homematic.regadom))
- Location: 52.52, 13.40

3 programs imported, 3 of them inactive, 1 not imported; 3 system variables imported.

## Programs

| Program | File | Result | Notes |
| --- | --- | --- | --- |
| Markise bei Wind | `programs/markise-bei-wind.yaml` | imported, inactive | holds a special value ("last value" or similar); the activity is skipped until it is edited; devices missing on this system: BidCos-RF/LEQ0123456:1, HmIP-RF/00111BE9AB9012:3; is not evaluated at system start (as set on the CCU); a key press line: triggers at every press; holds negated lines ("not when"); ends running delays for its targets - other programs' too; several activities: always in their listed order here, one after the other; "on update" with an "else": the "else" runs at every report while the condition is false |
| Frostwarnung | `programs/frostwarnung.yaml` | imported, inactive | uses CUxD (CUxD/CUX2801001:1), which does not exist here |
| Neues Feld | `programs/neues-feld.yaml` | imported, inactive | the database holds a field this version does not know: program/runlimit; the database holds a field this version does not know: rule/rule-prio; the database holds a field this version does not know: s-cond/hysteresis |
| Verwaist | – | not imported | refers to an object the database does not hold: 2999 |

## System variables

| System variable | File | Type | Value | Result | Notes |
| --- | --- | --- | --- | --- | --- |
| Regen | `variables/regen.yaml` | bool | true | imported | – |
| Küche Licht | `variables/kueche-licht.yaml` | bool | false | imported | – |
| Modus | `variables/modus.yaml` | list (Auto, Manuell, Urlaub) | Urlaub | imported | – |

## Devices that programs use

| Channel | Name on the CCU | Device | Rooms | on this system | Programs |
| --- | --- | --- | --- | --- | --- |
| `BidCos-RF/LEQ0123456:1` | Taster Flur oben | Taster Flur (HM-PB-2-WM55) | Flur | missing | Markise bei Wind |
| `CUxD/CUX2801001:1` | Wetter außen | Wetter (HM-WDS40-TH-I) | – | missing | Frostwarnung |
| `HmIP-RF/00111BE9AB9012:3` | Markise Terrasse:3 | Markise Terrasse (HmIP-BROLL) | Terrasse | missing | Markise bei Wind |
| `HmIP-RF/00111BE9AB9012:4` | Markise Behang | Markise Terrasse (HmIP-BROLL) | Terrasse, Flur | present | Markise bei Wind |

## Repaired names

| before | now |
| --- | --- |
| KÃ¼che Licht | Küche Licht |

## What the database holds

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
