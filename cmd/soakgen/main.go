// soakgen writes the tree of the long-running test (scripts/soak, internal/soak): programs/ and
// variables/ below a folder, and inputs.txt beside them - the variables the load writes, one
// "key type" per line.
//
//	soakgen [-programs 300] [-variables 800] [-read Interface/ADDRESS:CH/PARAM=value] [-write Interface/ADDRESS:CH/PARAM=value] <folder>
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/soak"
)

func main() {
	var o soak.Options
	var read, write string
	flag.IntVar(&o.Programs, "programs", 300, "the number of programs")
	flag.IntVar(&o.Variables, "variables", 800, "the number of system variables")
	flag.StringVar(&read, "read", "", "a real datapoint some programs trigger on, with the value they compare: Interface/ADDRESS:CH/PARAM=value")
	flag.StringVar(&write, "write", "", "a datapoint some programs write, with its value: Interface/ADDRESS:CH/PARAM=value")
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	o.Read, o.ReadValue, _ = strings.Cut(read, "=")
	o.Write, o.WriteValue, _ = strings.Cut(write, "=")
	if err := run(o, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "soakgen:", err)
		os.Exit(1)
	}
}

func run(o soak.Options, dir string) error {
	tree, err := soak.New(o)
	if err != nil {
		return err
	}
	for sub, files := range map[string]map[string][]byte{"programs": tree.Programs, "variables": tree.Variables} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
		for key, data := range files {
			if err := os.WriteFile(filepath.Join(dir, sub, key+".yaml"), data, 0o644); err != nil {
				return err
			}
		}
	}
	var inputs strings.Builder
	for _, in := range tree.Inputs {
		fmt.Fprintf(&inputs, "%s %s\n", in.Key, in.Type)
	}
	if err := os.WriteFile(filepath.Join(dir, "inputs.txt"), []byte(inputs.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("%d programs, %d variables, %d inputs in %s\n", len(tree.Programs), len(tree.Variables), len(tree.Inputs), dir)
	return nil
}
