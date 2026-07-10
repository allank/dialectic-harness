package compile

import (
	"os"
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

func summaryFixture() *state.DebateState {
	st := state.New("zaru-order-book", "drafts/zaru-order-book.md", 3, map[state.Role]string{
		state.RoleChallenger: "agy", state.RoleIncumbent: "claude",
	})
	st.TurnCount = 6
	st.RoundCount = 3
	st.ConsensusBaseline = []state.ConsensusItem{
		{ID: "C1", Issue: "The matching engine must be written in Go.", ResolvedTurn: 3, Rationale: "Concurrence: team skill set and murli reuse outweigh alternatives."},
	}
	st.Withdrawn = []state.ConsensusItem{
		{ID: "C3", Issue: "Missing glossary", ResolvedTurn: 4, Rationale: "Incumbent cited CONTEXT.md; contention withdrawn."},
	}
	st.ActiveContentions = []state.Contention{
		{ID: "C2", Issue: "In-memory vs Redis state store", Severity: "high", Stances: map[state.Role]string{
			state.RoleChallenger: "Redis is required for fault tolerance.",
			state.RoleIncumbent:  "In-memory via channel concurrency is faster.",
		}},
	}
	st.IgnoredDirectives = []state.Directive{
		{Target: state.RoleChallenger, Contention: "C2", Directive: "Provide a latency benchmark for Redis at 10k TPS.", IssuedTurn: 2},
	}
	return st
}

func TestRenderSummaryMatchesGolden(t *testing.T) {
	got := RenderSummary(summaryFixture(), "round_limit")
	golden, err := os.ReadFile("testdata/summary.golden.md")
	if err != nil {
		t.Fatalf("read golden: %v (generate with UPDATE_GOLDEN=1)", err)
	}
	if got != string(golden) {
		t.Errorf("summary differs from golden:\n--- got ---\n%s\n--- want ---\n%s", got, golden)
	}
}

func TestRenderSummaryIsDeterministic(t *testing.T) {
	a := RenderSummary(summaryFixture(), "round_limit")
	b := RenderSummary(summaryFixture(), "round_limit")
	if a != b {
		t.Error("same fixture in, byte-identical summary out — render must be pure")
	}
}

func TestRenderSummaryPortableMarkdown(t *testing.T) {
	got := RenderSummary(summaryFixture(), "round_limit")
	if strings.Contains(got, "[[") || strings.Contains(got, "> [!") {
		t.Error("summary must be portable CommonMark: no wikilinks, no Obsidian callouts")
	}
}

func TestUpdateGolden(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") == "" {
		t.Skip("set UPDATE_GOLDEN=1 to regenerate")
	}
	if err := os.WriteFile("testdata/summary.golden.md", []byte(RenderSummary(summaryFixture(), "round_limit")), 0o644); err != nil {
		t.Fatal(err)
	}
}
