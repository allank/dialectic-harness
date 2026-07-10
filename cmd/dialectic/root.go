package main

import (
	murliCobra "github.com/murli-cli/murli-go/cobra"
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "dialectic",
		Short: "Two-agent dialectic harness for PM artifacts",
		Long:  "dialectic runs a bounded, ledger-backed debate between a clean-room challenger and a vault-context incumbent over a target Markdown artifact.",
	}
	root.AddCommand(newDebateCmd())
	root.AddCommand(newRunsCmd())
	murliCobra.Enable(root)
	return root
}
