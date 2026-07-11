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
	root.AddCommand(newPromptsCmd())
	murliCobra.Enable(root)
	removeProfileCommand(root)
	return root
}

// removeProfileCommand strips the profile command group murli.Enable
// auto-mounts. No dialectic flag is marked Profileable, so every
// subcommand under it is a dead end ("no profileable flags were set") —
// better to not ship a command that can't do anything. The --profile
// persistent flag it would have populated is hidden for the same reason:
// with no save command, it can never resolve to a real profile.
func removeProfileCommand(root *cobra.Command) {
	for _, c := range root.Commands() {
		if c.Name() == "profile" {
			root.RemoveCommand(c)
			break
		}
	}
	_ = root.PersistentFlags().MarkHidden("profile")
}
