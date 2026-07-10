package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run(newRootCmd(), os.Stderr))
}

// run executes cmd and prints any returned error to stderr. This exists
// because murli's TTY-mode error path returns the error to the caller
// instead of printing it (it prints via WriteError only in non-TTY/--agent
// mode) — without this, interactive terminal runs fail silently.
func run(cmd *cobra.Command, stderr io.Writer) int {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}
