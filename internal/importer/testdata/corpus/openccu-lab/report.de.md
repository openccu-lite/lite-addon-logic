# Importbericht

- Quelle: `ccu` (ReGa-Datenbank (homematic.regadom))
- Standort: nicht in der Quelle – für Astro-Zeiten in den Einstellungen eintragen

Übernommen: 0 Programme (davon 0 inaktiv) und 2 Systemvariablen; nicht übernommen: 1 Programm.

## Programme

| Programm | Datei | Ergebnis | Hinweise |
| --- | --- | --- | --- |
| ruleTmpProgramName | – | nicht übernommen | Rest eines nicht gespeicherten Programms im Editor der Zentrale |

## Systemvariablen

| Systemvariable | Datei | Typ | Wert | Ergebnis | Hinweise |
| --- | --- | --- | --- | --- | --- |
| Anwesenheit | `variables/anwesenheit.yaml` | bool | true | übernommen | wird nach jedem Systemstart auf „wahr“ gesetzt, wie auf der Zentrale |
| Alarmzone 1 | `variables/alarmzone-1.yaml` | alarm | – | übernommen | Alarmvariable: die Datenbank enthält keinen Alarmzustand, sie beginnt als „nie ausgelöst“ |
| Gateway-DP | – | – | – | nicht übernommen | Systemobjekt der Zentrale, von keinem Programm verwendet |
| Alarmmeldungen | – | number | – | nicht übernommen | Systemwert: hier pflegt ihn das System selbst; Programme, die ihn verwenden, lesen diesen Wert |
| Servicemeldungen | – | number | – | nicht übernommen | Systemwert: hier pflegt ihn das System selbst; Programme, die ihn verwenden, lesen diesen Wert |

13 Servicemeldungen der Zentrale wurden nicht übernommen (das System zeigt sie selbst an).

## Geräte, die in Programmen vorkommen

keine

## Was die Datenbank enthält

- variable: alarm: 1
- variable: bool: 1
