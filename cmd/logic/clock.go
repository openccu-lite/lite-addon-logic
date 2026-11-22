package main

import "syscall"

// clockSynchronised says whether the system's clock is set: the kernel knows whether a time
// service (NTP, whichever) has synchronised it. A system without a battery clock starts with
// the time of its last shutdown, or 1970, until the network is up - a time module must not
// fire on that. Where the kernel cannot be asked the clock is taken as it is.
func clockSynchronised() bool {
	var tx syscall.Timex
	if _, err := syscall.Adjtimex(&tx); err != nil {
		return true
	}
	const unsynchronised = 0x40 // STA_UNSYNC
	return tx.Status&unsynchronised == 0
}
