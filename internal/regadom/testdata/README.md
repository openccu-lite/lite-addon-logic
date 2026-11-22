# Test data

- `fixture.regadom` - the database of a stock OpenCCU 3.89.8 (ReGaHss R1.00.0388.0254) on which twenty programs, twenty
  time modules and the variables of every type were built with the calls the WebUI's pages make; the Admin password
  hash is blanked. The system had no radio, so the file has no devices: every condition and activity is on a system
  variable or a time module.
- `devices.regadom` - written by hand in the same shape, for what the first file cannot show: devices, channels and
  datapoints on three interfaces (one of them CUxD), rooms, a key line, negated lines, a special value, the
  `<startupexec>` element of OpenCCU 3.89.14 and later, elements this reader does not know, and references to objects
  the file does not hold. All addresses and names are made up.
- `counters.regadom` - written by hand in the same shape, for the device helpers: a measuring actuator and a weather
  sensor (the addresses of the fake system's two) with the counter variables and hidden programs the WebUI creates
  for them (`svEnergyCounter…`, `svHmIPRainCounter…`, `svHmIPSunshineCounter…`, `prg…Counter…`) and the day values
  the WebUI keeps in the channel's metadata.
