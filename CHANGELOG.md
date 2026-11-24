# Changelog

## [Unreleased]

## [1.0.0-beta.1] - 2026-11-24

The second pre-release, for openccu-lite 2.0.0-rc.1 and later; the system's tiles for programs and system variables
under Control come with openccu-lite 2.0.0-rc.4. **The first start after this update reports every variable and
program once as deleted and new** to software that follows the interface "Logic" (their description changed, see
below) - in its logs that is no fault, and names, rooms and values stay.

- **No reboot asked for after the first install:** installing the package no longer ends with "an addon asked for a
  reboot to finish its installation" - the system starts Logic right away, and a reboot was never needed.
- **Fewer writes to storage with logged variables:** the kept changes of logged variables are no longer part of the
  state file, which was written whole twice a minute and grew with them (megabytes with many logged variables). They
  have a file of their own (`state/log.jsonl`) that only gets the new changes; the changes kept so far are moved over
  at the first start.
- **Less memory with many programs:** every program kept its last 200 entries of the run history in memory beside
  the 5000 of all programs together - with 300 programs the daemon grew by tens of megabytes within hours. A program
  now keeps its own last 20 beside the common 5000; what the History tab and a program's view show of the last hours
  is the same. The daemon also collects unused memory earlier.
- **The interface says more about variables and programs:** a boolean's own texts for false and true
  (`VALUE_LIST`), *visible* and *internal* (the value channel's `FLAGS`), and a program that is not operable is
  described without a writable `RUN`. The system's Control page uses it for its tiles of programs and system
  variables. After the update every variable and program is reported once as deleted and new to software that follows
  the interface.
- **A client that registers again after an update learns of changed objects:** the interface told a client that
  knew its devices nothing when their description had changed while it was away; it now reports such a device as
  deleted and new, as it does while it runs.

## [1.0.0-beta.0] - 2026-11-22

The first pre-release, for openccu-lite 2.0.0-rc.1 and later. Much of it has been tested only on test systems and
against a simulated system; some of the device forms and the HmIP-WRCD's display have not yet run on a real device, and the device helpers have not yet counted on a real
metering device (their arithmetic follows the CCU's counter scripts line by line).
Please report what behaves differently from the guide.

- **Programs as known from the CCU:** *If … Else if … Else*, AND/OR, every condition with its trigger kind (*when
  changed*, *when updated*, *check only*), negated lines, activities immediately or delayed, *stop running delays for
  these targets first*; time modules with patterns, validity, the clock change and astro (sunrise and sunset, the
  twilights).
- **System variables:** boolean, value list, number, string and alarm, with labels, unit, limits and channel; a value
  at system start; the system values `alarm-messages`, `sun-azimuth` and `sun-elevation`.
- **The import** from a system backup (`.sbk`), the file `homematic.regadom` or the ReGa database the system kept after
  the switch: a dry run with a report first, devices matched by address, every inactive program with its reason, a
  second import updates the same files.
- **Beyond the CCU, without a script:** *holds for* on a condition line, a random delay, *on for* (`ON_TIME` in the
  same command), astro points, several values of a channel in one command, forms for signal lights, sounds, sirens,
  coloured lights and blinds with slats, the HmIP-WRCD's display, webhooks, Wake-on-LAN, computed values (dew point,
  absolute humidity, difference, sum, average, minimum, maximum).
- **Commands** in place of `system.Exec`: a program of the system with a time limit, in the background or waited for,
  its output and exit code into variables, with the addon's rights only - never as root, and never holding up another
  program. A script that is a single `system.Exec` is imported as a webhook, Wake-on-LAN or a command, inactive.
- **Device helpers** count on what the CCU's hidden counter programs counted - energy, feed-in, gas, water, rain and
  sunshine, the total, today and yesterday - and take over the CCU's readings on import.
- **Traceable:** every program and variable is a readable YAML file with a history (versions to compare and bring
  back, optionally pushed to a git remote); for every run the History tab shows what the program saw and did, and why
  it did not run; a dry run per program; the Triggers tab and the Schedule show what triggers what and what is due.
- **For other software:** programs and variables are devices of the interface `Logic`, which listens on the system
  itself only.
- The page in German and English, with a guide in both languages and a chapter for everyone who comes from a CCU.
