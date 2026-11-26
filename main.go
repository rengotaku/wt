package main

import (
	"os"
	"strconv"
	"strings"

	"wt/cmd"
	"wt/internal/cli/clierr"
)

// wantsJSON mirrors how cobra reads the --json bool flag: "--json",
// "--json=true" (or 1/t/T/TRUE/True), ignoring anything after "--".
func wantsJSON(args []string) bool {
	on := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" {
			on = true
			continue
		}
		if v, ok := strings.CutPrefix(a, "--json="); ok {
			b, err := strconv.ParseBool(v)
			on = err == nil && b
		}
	}
	return on
}

func main() {
	if err := cmd.Execute(); err != nil {
		clierr.Write(os.Stderr, err, wantsJSON(os.Args[1:]))
		os.Exit(clierr.ExitCode(err))
	}
}
