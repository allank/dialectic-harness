package main

import (
	"os"
	"path/filepath"

	"github.com/murli-cli/murli-go"
	murliCobra "github.com/murli-cli/murli-go/cobra"
	"github.com/spf13/cobra"

	"github.com/allank/dialectic/internal/runstore"
)

func newRunsCmd() *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "runs [dir]",
		Short: "Regenerate the kill-criterion index of debate runs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := murliCobra.NewWriter(cmd)
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			table, err := runstore.BuildIndex(root)
			if err != nil {
				return murli.NewToolError("scan runs: " + err.Error())
			}
			if write {
				if err := os.WriteFile(filepath.Join(root, "a2a-runs.md"), []byte(table), 0o644); err != nil {
					return murli.NewToolError("write index: " + err.Error())
				}
			}
			w.WriteSuccess(table, map[string]any{"index": table, "written": write})
			return nil
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "also write the table to <dir>/a2a-runs.md")
	murliCobra.Annotate(cmd, murli.Metadata{
		AgentDescription: "Scans update briefs for arbiter_verdict frontmatter and renders the kill-criterion index table.",
		Idempotent:       true,
	})
	return cmd
}
