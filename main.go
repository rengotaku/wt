package main

import (
	"os"
	"slices"

	"wt/cmd"
	"wt/internal/cli/clierr"
)

func main() {
	if err := cmd.Execute(); err != nil {
		clierr.Write(os.Stderr, err, slices.Contains(os.Args[1:], "--json"))
		os.Exit(clierr.ExitCode(err))
	}
}
