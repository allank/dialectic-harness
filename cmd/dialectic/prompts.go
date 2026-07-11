package main

import (
	"fmt"
	"strings"

	"github.com/murli-cli/murli-go"
	murliCobra "github.com/murli-cli/murli-go/cobra"
	"github.com/spf13/cobra"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/compile"
)

const promptFlowDiagram = `[Opening Critique]  challenger, turn 1, clean room
  uses: opening_critique + schema
        |
        v
[Turn Loop]  challenger <-> incumbent, alternating
  uses: turn + schema, every turn
        |
        +-- invalid turn file --> retry once (same prompt + errors)
        |                              |
        |                    still invalid --> HALT (state preserved)
        v
  consensus reached OR round limit hit
        |
        v
[Compiler]  sessionless, reads full ledger
  uses: compiler
        |
        v
  compiled summary + update brief`

// promptCatalogOrder is the fixed display order for both human and agent
// output — matches the design's Template Catalog table.
var promptCatalogOrder = []string{"opening_critique", "turn", "schema", "compiler"}

func allDefaultTemplates() map[string]string {
	out := agent.DefaultTemplates()
	for name, text := range compile.DefaultTemplates() {
		out[name] = text
	}
	return out
}

func newPromptsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompts",
		Short: "Print the harness's built-in prompt templates and debate flow diagram",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := murliCobra.NewWriter(cmd)
			templates := allDefaultTemplates()

			var human strings.Builder
			human.WriteString(promptFlowDiagram)
			human.WriteString("\n\n")
			for _, name := range promptCatalogOrder {
				fmt.Fprintf(&human, "=== %s ===\n%s\n\n", name, templates[name])
			}

			w.WriteSuccess(strings.TrimRight(human.String(), "\n"), map[string]any{
				"diagram":   promptFlowDiagram,
				"templates": templates,
			})
			return nil
		},
	}
	murliCobra.Annotate(cmd, murli.Metadata{
		AgentDescription: "Prints the debate flow diagram and all four built-in prompt templates (opening_critique, turn, schema, compiler), raw with placeholders visible. Read-only, no artifact required.",
		Idempotent:       true,
	})
	return cmd
}
