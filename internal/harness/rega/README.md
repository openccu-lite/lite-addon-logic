# The reference side of the differential harness

`internal/harness` compares this addon's engine with a real ReGaHss. The reference is the old
CCU behaviour: ReGaHss R1.00.0388.0254 of OpenCCU 3.89.8 (before the changes of 3.89.14). Its
side is recorded with `cmd/regaharness` and committed as `testdata/<scenario>/rega.trace`; the
tests on every commit play the engine side against those recordings. Nobody needs this folder
to run the tests - only to record a new scenario or to record again.

## The image

The image runs the reference's ReGaHss under libfaketime, in the time zone Europe/Berlin, with
no interface process and no device. Its clock is the real time plus what `/clock/offset` says
(`+<seconds>s x<rate>`) when the container starts, so the harness chooses the date and how fast
time passes - a night with a change of the daylight saving time takes a few minutes.

Three things that cost recordings before they were found:

- **The clock must not be read from a file on every call.** ReGaHss asks for the time from many
  threads. With the plain `libfaketime.so.1` and `FAKETIME_TIMESTAMP_FILE` + `FAKETIME_NO_CACHE`
  about one call in ten thousand answered the real time: a jump of months there and back, after
  which ReGaHss answered scripts but no longer ran any program. `start.sh` therefore preloads
  `libfaketimeMT.so.1` and hands the offset over once, in `FAKETIME`. (The recorder's heartbeat
  still watches for a reference that no longer runs programs.)
- **One connection per script.** ReGaHss now and then ends a connection that is used again
  without an answer - and without running the script (three in a thousand). `cmd/regaharness`
  switches keep-alive off.
- **Rates.** x20 to x60 give times exact to the second. Faster was not proven.

eQ-3's binary is not part of this repository. Put into this folder, beside the Dockerfile:

| File | From the reference system (a stock OpenCCU 3.89.8) |
| --- | --- |
| `ReGaHss` | `/bin/ReGaHss` |
| `libXmlRpc.so`, `libxmlparser.so` | `/lib/` |
| `homematic.regadom` | `/etc/config/homematic.regadom` of a system without programs (blank the users' `pwsha512`) |

then `docker build -t logic-rega:0254 internal/harness/rega`. The four files are ignored by git.

## Recording

    go run ./cmd/regaharness -list                 # the scenarios
    go run ./cmd/regaharness -compare              # record what has no recording yet, and compare
    go run ./cmd/regaharness -compare <name> ...   # record these again
    go run ./cmd/regaharness -all -compare         # record everything again

One container per scenario, several at a time (`-j`). A scenario at rate 1 takes as long as it
says; time-module scenarios run their clock 30 to 60 times as fast, and their times are compared
with a slack of two seconds of the reference's clock per rate.

## What a recording holds

- `rega.trace` - when each program was created, when each step was done, and which block of
  which program ran when (the markers), in order.
- `homematic.regadom.gz` - the database the reference saved after the programs were created: the
  engine side imports it, so the importer is part of what is compared.
- `programs.yaml` - the programs as the import stored them, for the reader.
