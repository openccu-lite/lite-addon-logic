package main

import (
	"fmt"
	"io"
	"os"

	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// anonymise is "logic anonymise <backup or database> <output>": the database without names,
// serial numbers, texts and scripts, for a report or for the import tests of this addon.
func anonymise(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: logic anonymise <backup.sbk or homematic.regadom> <output file>")
		return 2
	}
	in, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "logic:", err)
		return 1
	}
	defer in.Close()
	out, err := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		fmt.Fprintln(stderr, "logic:", err)
		return 1
	}
	done, err := regadom.Anonymise(in, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(args[1])
		fmt.Fprintln(stderr, "logic:", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s written: %d names, %d serial numbers, %d texts and %d scripts replaced, %d other entries emptied.\n"+
		"Numbers, device types and the structure of the programs are kept. Look through the file before you pass it on.\n",
		args[1], done.Names, done.Addresses, done.Texts, done.Scripts, done.Dropped)
	return 0
}
