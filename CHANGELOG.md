# Changelog

## [Unreleased]

## [1.0.0-beta.3] - 2026-11-25

The fourth pre-release, for openccu-lite 2.0.0-rc.1 and later. **HmIPServer now stays subscribed to the interface
"Logic"** - before, it was dropped ten events after each of its starts.

- **HmIPServer stays subscribed to the interface "Logic":** it could not read the events Logic sent it (every event's
  method name was written as a typed string), answered each with nothing, and after ten events Logic dropped it until
  HmIPServer's next start - ten error messages in HmIPServer's log after every start. Events now name their method in
  the form HmIPServer reads; other software reads both forms. Logic's log line for a dropped subscriber now says why
  the last call failed.
- **The import report names a CCU's built-in rooms as the CCU shows them:** "Wohnzimmer" (or "Living room"), not
  `roomLivingRoom`, the name the CCU keeps them by.
- **The import's summaries count in the singular where a number is one:** "Übernommen: 3 Programme (davon 1 inaktiv)
  und 6 Systemvariablen" instead of "1 der Programme sind inaktiv"; the dry run and the report file say it the same
  way.
- **German texts write "Script"**, not "Skript" - on the page, in the guide, the import report and the catalogue
  description.

## [1.0.0-beta.2] - 2026-11-25

The third pre-release, for openccu-lite 2.0.0-rc.1 and later. **The interface "Logic" listens on port 31310 now**
(see below). An update straight from 1.0.0-beta.0 also brings 1.0.0-beta.1's changed descriptions: its first start
then reports every variable and program once as deleted and new to software that follows the interface - no fault,
names, rooms and values stay. An update from 1.0.0-beta.1 leaves the descriptions as they are.

- **The interface "Logic" moves to port 31310** (it was 39310): that port lies in the range the system takes
  ports for outgoing connections from, and while another program's connection held it, Logic did not start - after a
  fresh install it failed for minutes. Software that reaches Logic through the system's programming interface
  (lite-rpc) by its name notices nothing: the system lists the interface at its new port after the update. Only a
  script that called 127.0.0.1:39310 directly needs the new port.
- **A push of the history no longer fails now and then while a change is saved:** with a remote set, a push that
  ran while the next change was written could end with "the remote did not answer as expected: ref file is empty"
  or "… object not found" until the next push. The history's references are now written whole, and the temporary
  files of new entries no longer lie where a push looks for the history's stored entries.

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
