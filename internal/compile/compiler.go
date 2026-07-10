package compile

import (
	"context"
	"fmt"
	"strings"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/state"
)

// BuildCompilerPrompt builds the prompt for the sessionless Compiler role: a
// disinterested reader of the finished ledger who writes a narrative and
// proposed changes, every claim cited back to the ledger.
func BuildCompilerPrompt(st *state.DebateState, statePath, outPath string, retryErrors []string) string {
	var b strings.Builder
	b.WriteString("You are the COMPILER for a finished two-agent debate. You did not participate and have no stake in the dispute. Read the full debate ledger at ")
	b.WriteString(statePath)
	b.WriteString(" and the target artifact at ")
	b.WriteString(st.TargetArtifact)
	b.WriteString(".\n\nWrite a Markdown document to ")
	b.WriteString(outPath)
	b.WriteString(" with exactly these three sections:\n\n")
	b.WriteString("## Narrative\nA prose account of how the debate evolved: what was contested, what moved, what stuck.\n\n")
	b.WriteString("## Proposed Changes\nBullet list. Each item proposes a concrete edit to the artifact, derived ONLY from consensus_baseline items.\n\n")
	b.WriteString("## Judgment Calls\nBullet list. Each item poses a question the author must decide, with context, derived ONLY from unresolved active_contentions.\n\n")
	b.WriteString("Citation rules (mandatory): every bullet and every narrative claim cites its source as (C<id>, turn <n>), e.g. (C2, turn 4). Cite only contention ids that exist in the ledger. Use plain CommonMark only — no wikilinks or Obsidian syntax. Do not edit the artifact or any other file.\n")
	if len(retryErrors) > 0 {
		b.WriteString("\nYour previous output FAILED citation validation. Fix these errors and rewrite the complete document at the same path:\n")
		for _, e := range retryErrors {
			b.WriteString("- " + e + "\n")
		}
	}
	return b.String()
}

// RunCompiler invokes the compiler binary sessionless, validates citation
// integrity deterministically, retries once with errors, then fails.
func RunCompiler(ctx context.Context, r agent.Runner, binary string, st *state.DebateState, statePath, workDir, outPath string) (string, error) {
	var retryErrors []string
	for attempt := 0; attempt < 2; attempt++ {
		res, err := r.Invoke(ctx, agent.Request{
			Binary:     binary,
			Prompt:     BuildCompilerPrompt(st, statePath, outPath, retryErrors),
			WorkDir:    workDir,
			SessionID:  "", // sessionless by design: no stake, no memory
			OutputPath: outPath,
		})
		if err != nil {
			return "", fmt.Errorf("compiler invocation: %w", err)
		}
		doc := string(res.Output)
		retryErrors = ValidateCitations(doc, st)
		if len(retryErrors) == 0 {
			return doc, nil
		}
	}
	return "", fmt.Errorf("compiler output failed citation validation after retry: %s", strings.Join(retryErrors, "; "))
}
