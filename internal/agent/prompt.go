// Package agent handles everything on the far side of the orchestrator's
// process boundary: prompt construction, headless CLI invocation, and the
// clean-room working directory for the challenger.
package agent

import (
	"fmt"
	"strings"

	"github.com/allank/dialectic/internal/state"
)

type PromptInput struct {
	Role           state.Role
	ArtifactPath   string
	StatePath      string // empty on turn 1 (opening critique)
	TurnFilePath   string
	MaxContentions int
	Directives     []state.Directive // directives targeting this role
	RetryErrors    []string          // non-empty on the single validation retry
}

const turnFileSchemaBlock = `Write your turn as a YAML file at the exact path given above, with this schema and nothing else:

agent: %s
entries:
  - contention: C1        # id of an active contention; OMIT for stance new
    stance: rebut         # one of: concur | rebut | withdraw | new
    rationale: "why"      # mandatory on every entry; bare concessions are invalid
    position: "your current position in one sentence (optional, for concur/rebut/withdraw)"
    issue: "one-line issue statement"   # required only for stance: new
    severity: high        # optional, for stance: new (high|medium|low)
directives:               # optional: demand the other agent address a point next turn
  - contention: C1
    directive: "what they must address"

Rules:
- Every entry must cite an active contention id, except stance new (the orchestrator assigns ids to new contentions).
- Do not edit the artifact or any other file. Your only output is the turn file.
- Do not re-litigate resolved contentions.`

func BuildPrompt(in PromptInput) string {
	var b strings.Builder
	if in.Role == state.RoleChallenger && in.StatePath == "" {
		fmt.Fprintf(&b, "You are the CHALLENGER in a structured debate about a document. You have no prior context on it — that is deliberate. Read the artifact at %s with fresh eyes.\n\n", in.ArtifactPath)
		fmt.Fprintf(&b, "Produce an Opening Critique: raise at most %d contentions — the strongest problems with the artifact, ranked by severity. Every entry uses stance: new.\n\n", in.MaxContentions)
	} else {
		fmt.Fprintf(&b, "You are the %s in a structured debate about the artifact at %s.\n\n", strings.ToUpper(string(in.Role)), in.ArtifactPath)
		fmt.Fprintf(&b, "Read the current debate state at %s. It lists active_contentions (with each side's stance), consensus_baseline (settled — do not reopen), and the full contention_history ledger.\n\n", in.StatePath)
		b.WriteString("For each active contention, take a stance: concur | rebut | withdraw | new — with a mandatory rationale. Concur only if you genuinely accept the other side's position. You may raise new contentions if your analysis exposes a fresh problem, but stay focused on the existing dispute.\n\n")
	}
	if len(in.Directives) > 0 {
		b.WriteString("You MUST address these directives this turn (cite the contention id in an entry):\n")
		for _, d := range in.Directives {
			fmt.Fprintf(&b, "- %s: %s\n", d.Contention, d.Directive)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Turn file path: %s\n\n", in.TurnFilePath)
	fmt.Fprintf(&b, turnFileSchemaBlock, in.Role)
	if len(in.RetryErrors) > 0 {
		b.WriteString("\n\nYour previous turn file was INVALID. Fix these errors and rewrite the complete turn file at the same path:\n")
		for _, e := range in.RetryErrors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	return b.String()
}
