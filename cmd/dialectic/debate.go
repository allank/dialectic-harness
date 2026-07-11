package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/murli-cli/murli-go"
	murliCobra "github.com/murli-cli/murli-go/cobra"
	"github.com/spf13/cobra"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/compile"
	"github.com/allank/dialectic/internal/orchestrate"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/runstore"
	"github.com/allank/dialectic/internal/state"
)

func newDebateCmd() *cobra.Command {
	var challenger, incumbent, compiler string
	var maxRounds, maxContentions int
	var overridePromptFlags []string

	cmd := &cobra.Command{
		Use:   "debate <artifact>",
		Short: "Run a two-agent dialectic debate over a target artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			w := murliCobra.NewWriter(cmd)
			reportProgress := func(ev progress.Event) {
				w.WriteProgress(murli.ProgressEvent{
					Stage:   ev.Stage,
					Current: ev.Turn,
					Total:   ev.MaxRounds * 2,
					Message: ev.Message,
				})
			}
			artifact, err := filepath.Abs(args[0])
			if err != nil {
				return murli.NewUserError("bad artifact path: "+err.Error(), "pass a path to a Markdown file")
			}
			if _, err := os.Stat(artifact); err != nil {
				return murli.NewUserError("artifact not found: "+artifact, "pass a path to an existing Markdown file")
			}

			overrides := map[string]string{}
			if len(overridePromptFlags) > 0 {
				valid := map[string]bool{}
				for name := range agent.DefaultTemplates() {
					valid[name] = true
				}
				for name := range compile.DefaultTemplates() {
					valid[name] = true
				}
				for _, spec := range overridePromptFlags {
					name, path, ok := strings.Cut(spec, "=")
					if !ok {
						return murli.NewUserError("invalid --override-prompt value: "+spec, "use the form <name>=<path>, e.g. turn=my-turn.tmpl")
					}
					if !valid[name] {
						return murli.NewUserError("unknown prompt name: "+name, "valid names: opening_critique, turn, schema, compiler")
					}
					content, err := os.ReadFile(path)
					if err != nil {
						return murli.NewUserError("cannot read override file for "+name+": "+err.Error(), "check the file path")
					}
					overrides[name] = string(content)
				}
			}

			for _, bin := range []string{challenger, incumbent, compiler} {
				if _, err := exec.LookPath(bin); err != nil {
					return murli.NewUserError("agent binary not found: "+bin, "install it or pass --challenger/--incumbent/--compiler")
				}
			}
			// Accepted risk, flagged not blocked: agy resumes via -c (most
			// recent conversation), so two roles on agy share one session.
			if filepath.Base(challenger) == "agy" && filepath.Base(incumbent) == "agy" {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: both roles use agy; agy resumes with -c (most recent conversation), so the challenger and incumbent will cross-contaminate sessions")
			}

			paths, err := runstore.NewRun(artifact, time.Now())
			if err != nil {
				return murli.NewToolError(err.Error())
			}
			st := state.New(runstore.Slug(artifact), artifact, maxRounds, map[state.Role]string{
				state.RoleChallenger: challenger,
				state.RoleIncumbent:  incumbent,
			})

			loop := &orchestrate.Loop{
				State:           st,
				StatePath:       paths.StatePath,
				ArtifactPath:    artifact,
				ScratchDir:      paths.ScratchDir,
				TurnsDir:        paths.TurnsDir,
				Runner:          agent.NewExecRunner(),
				MaxContentions:  maxContentions,
				Progress:        reportProgress,
				PromptOverrides: overrides,
			}
			outcome, err := loop.Run(cmd.Context())
			if err != nil {
				return murli.NewToolError(fmt.Sprintf("%v — inspect state at %s", err, paths.StatePath))
			}

			summary := compile.RenderSummary(st, outcome)
			if err := runstore.WriteSummary(paths, summary); err != nil {
				return murli.NewToolError(err.Error())
			}

			doc, err := compile.RunCompiler(cmd.Context(), agent.NewExecRunner(), compiler, st,
				paths.StatePath, filepath.Dir(artifact), filepath.Join(paths.RunDir, "compiler-output.md"), reportProgress, overrides)
			if err != nil {
				return murli.NewToolError(fmt.Sprintf("%v — compiled summary already written to %s", err, paths.SummaryPath))
			}
			if err := runstore.WriteUpdateBrief(paths, st, doc, outcome, time.Now()); err != nil {
				return murli.NewToolError(err.Error())
			}

			w.WriteSuccess(
				fmt.Sprintf("Debate finished (%s) after %d round(s).\n  Summary: %s\n  Update brief: %s\n  Ledger: %s",
					outcome, st.RoundCount, paths.SummaryPath, paths.BriefPath, paths.StatePath),
				map[string]any{
					"outcome":      outcome,
					"rounds":       st.RoundCount,
					"summary_path": paths.SummaryPath,
					"brief_path":   paths.BriefPath,
					"state_path":   paths.StatePath,
					"consensus":    len(st.ConsensusBaseline),
					"unresolved":   len(st.ActiveContentions),
					"ignored_dirs": len(st.IgnoredDirectives),
				},
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&challenger, "challenger", "claude", "binary for the clean-room challenger role")
	cmd.Flags().StringVar(&incumbent, "incumbent", "agy", "binary for the vault-context incumbent role")
	cmd.Flags().StringVar(&compiler, "compiler", "claude", "binary for the sessionless compiler role")
	cmd.Flags().IntVar(&maxRounds, "max-rounds", 3, "circuit breaker: maximum debate rounds")
	cmd.Flags().IntVar(&maxContentions, "max-contentions", 5, "cap on opening critique contentions")
	cmd.Flags().StringArrayVar(&overridePromptFlags, "override-prompt", nil, "override a built-in prompt: --override-prompt <name>=<path> (opening_critique|turn|schema|compiler), repeatable")
	murliCobra.Annotate(cmd, murli.Metadata{
		AgentDescription: "Runs a bounded clean-room-vs-incumbent debate over a Markdown artifact and writes a compiled summary and update brief beside it. Read-only over the artifact.",
		Idempotent:       false,
	})
	return cmd
}
