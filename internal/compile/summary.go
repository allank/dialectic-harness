// Package compile renders debate outputs: the deterministic compiled summary
// (a pure function of ledger fields) and, separately, the Compiler stage
// that produces the narrative and update brief.
package compile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/allank/dialectic/internal/state"
)

// RenderSummary is deterministic: same state in, byte-identical output.
// It reads only ledger fields — no model in the loop.
func RenderSummary(st *state.DebateState, outcome string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Debate Summary: %s\n\n", st.TopicSlug)
	fmt.Fprintf(&b, "- Target artifact: %s\n", st.TargetArtifact)
	fmt.Fprintf(&b, "- Outcome: %s\n", outcome)
	fmt.Fprintf(&b, "- Rounds: %d of %d; turns: %d\n", st.RoundCount, st.MaxRounds, st.TurnCount)
	fmt.Fprintf(&b, "- Roles: challenger=%s (clean room), incumbent=%s\n\n", st.Roles[state.RoleChallenger], st.Roles[state.RoleIncumbent])

	b.WriteString("## Decisions Made and Why\n\n")
	if len(st.ConsensusBaseline) == 0 {
		b.WriteString("None.\n")
	}
	items := append([]state.ConsensusItem(nil), st.ConsensusBaseline...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for _, c := range items {
		fmt.Fprintf(&b, "- **%s — %s** (resolved turn %d): %s\n", c.ID, c.Issue, c.ResolvedTurn, c.Rationale)
	}
	if len(st.Withdrawn) > 0 {
		b.WriteString("\nWithdrawn:\n")
		w := append([]state.ConsensusItem(nil), st.Withdrawn...)
		sort.Slice(w, func(i, j int) bool { return w[i].ID < w[j].ID })
		for _, c := range w {
			fmt.Fprintf(&b, "- **%s — %s** (withdrawn turn %d): %s\n", c.ID, c.Issue, c.ResolvedTurn, c.Rationale)
		}
	}

	b.WriteString("\n## Unresolved Tensions\n\n")
	if len(st.ActiveContentions) == 0 {
		b.WriteString("None.\n")
	}
	active := append([]state.Contention(nil), st.ActiveContentions...)
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	for _, c := range active {
		fmt.Fprintf(&b, "- **%s — %s** (severity: %s)\n", c.ID, c.Issue, c.Severity)
		fmt.Fprintf(&b, "  - challenger: %s\n", c.Stances[state.RoleChallenger])
		fmt.Fprintf(&b, "  - incumbent: %s\n", c.Stances[state.RoleIncumbent])
	}

	b.WriteString("\n## Ignored Directives\n\n")
	if len(st.IgnoredDirectives) == 0 {
		b.WriteString("None.\n")
	}
	ign := append([]state.Directive(nil), st.IgnoredDirectives...)
	sort.Slice(ign, func(i, j int) bool {
		if ign[i].IssuedTurn != ign[j].IssuedTurn {
			return ign[i].IssuedTurn < ign[j].IssuedTurn
		}
		return ign[i].Contention < ign[j].Contention
	})
	for _, d := range ign {
		fmt.Fprintf(&b, "- %s ignored a directive on **%s** (issued turn %d): %s\n", d.Target, d.Contention, d.IssuedTurn, d.Directive)
	}
	return b.String()
}
