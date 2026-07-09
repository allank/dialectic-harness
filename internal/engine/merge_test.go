package engine

import (
	"testing"

	"github.com/allank/dialectic/internal/state"
	"github.com/allank/dialectic/internal/turn"
)

func openingState() *state.DebateState {
	return state.New("s", "a.md", 3, map[state.Role]string{
		state.RoleChallenger: "agy", state.RoleIncumbent: "claude",
	})
}

func TestMergeOpeningCritiqueCreatesContentions(t *testing.T) {
	st := openingState()
	tf := turn.File{
		Agent: state.RoleChallenger,
		Entries: []turn.Entry{
			{Stance: state.StanceNew, Issue: "no rollback plan", Severity: "high", Rationale: "doc lacks rollback", Position: "A rollback section is required."},
			{Stance: state.StanceNew, Issue: "vague success metric", Severity: "medium", Rationale: "metric unmeasurable"},
		},
	}
	if err := Merge(st, tf); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(st.ActiveContentions) != 2 {
		t.Fatalf("active: want 2, got %d", len(st.ActiveContentions))
	}
	if st.ActiveContentions[0].ID != "C1" || st.ActiveContentions[1].ID != "C2" {
		t.Errorf("ids: got %s, %s", st.ActiveContentions[0].ID, st.ActiveContentions[1].ID)
	}
	if got := st.ActiveContentions[0].Stances[state.RoleChallenger]; got != "A rollback section is required." {
		t.Errorf("stance text: got %q", got)
	}
	// Position omitted falls back to rationale.
	if got := st.ActiveContentions[1].Stances[state.RoleChallenger]; got != "metric unmeasurable" {
		t.Errorf("fallback stance text: got %q", got)
	}
	if len(st.ContentionHistory) != 2 || st.ContentionHistory[0].Turn != 1 || st.ContentionHistory[0].Contention != "C1" {
		t.Errorf("ledger: %+v", st.ContentionHistory)
	}
	if st.TurnCount != 1 || st.RoundCount != 0 || st.NextRole != state.RoleIncumbent {
		t.Errorf("bookkeeping: turn=%d round=%d next=%s", st.TurnCount, st.RoundCount, st.NextRole)
	}
}

func TestResolutionGateConcurMovesToConsensus(t *testing.T) {
	st := openingState()
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Stance: state.StanceNew, Issue: "no rollback plan", Severity: "high", Rationale: "doc lacks rollback"},
	}})
	if err := Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceConcur, Rationale: "agreed, rollback section needed"},
	}}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(st.ActiveContentions) != 0 {
		t.Fatalf("active should be empty, got %+v", st.ActiveContentions)
	}
	if len(st.ConsensusBaseline) != 1 {
		t.Fatalf("consensus: want 1, got %d", len(st.ConsensusBaseline))
	}
	c := st.ConsensusBaseline[0]
	if c.ID != "C1" || c.ResolvedTurn != 2 || c.Rationale != "agreed, rollback section needed" {
		t.Errorf("consensus item: %+v", c)
	}
	if st.RoundCount != 1 {
		t.Errorf("incumbent closes round: RoundCount want 1, got %d", st.RoundCount)
	}
}

func TestRebutKeepsContentionActiveAndRecordsStance(t *testing.T) {
	st := openingState()
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Stance: state.StanceNew, Issue: "store choice", Severity: "high", Rationale: "Redis needed", Position: "Redis is required for fault tolerance."},
	}})
	_ = Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceRebut, Rationale: "latency budget rules Redis out", Position: "In-memory is faster."},
	}})
	if st.FindActive("C1") == nil {
		t.Fatal("C1 must stay active after rebut")
	}
	if got := st.FindActive("C1").Stances[state.RoleIncumbent]; got != "In-memory is faster." {
		t.Errorf("incumbent stance: got %q", got)
	}
}

func TestConcurWithAccompanyingRebutDoesNotResolve(t *testing.T) {
	st := openingState()
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Stance: state.StanceNew, Issue: "x", Rationale: "r"},
	}})
	// Same turn file both concurs and rebuts C1: the gate must NOT resolve it.
	_ = Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceConcur, Rationale: "agree in part"},
		{Contention: "C1", Stance: state.StanceRebut, Rationale: "but the premise is wrong"},
	}})
	if st.FindActive("C1") == nil {
		t.Fatal("C1 must remain active when concur is accompanied by rebut on the same id")
	}
	if len(st.ConsensusBaseline) != 0 {
		t.Fatal("nothing should reach consensus")
	}
}

func TestWithdrawRemovesFromActiveIntoWithdrawn(t *testing.T) {
	st := openingState()
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Stance: state.StanceNew, Issue: "x", Rationale: "r"},
	}})
	_ = Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceRebut, Rationale: "reference answers this"},
	}})
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceWithdraw, Rationale: "incumbent's citation resolves it"},
	}})
	if st.FindActive("C1") != nil {
		t.Fatal("withdrawn contention must leave active_contentions")
	}
	if len(st.Withdrawn) != 1 || st.Withdrawn[0].ResolvedTurn != 3 {
		t.Fatalf("withdrawn: %+v", st.Withdrawn)
	}
	if len(st.ConsensusBaseline) != 0 {
		t.Fatal("withdraw is not consensus")
	}
}

func TestLedgerIsAppendOnlyAcrossMerges(t *testing.T) {
	st := openingState()
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Stance: state.StanceNew, Issue: "x", Rationale: "r1"},
	}})
	first := st.ContentionHistory[0]
	_ = Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceConcur, Rationale: "r2"},
	}})
	if len(st.ContentionHistory) != 2 {
		t.Fatalf("history length: want 2, got %d", len(st.ContentionHistory))
	}
	if st.ContentionHistory[0] != first {
		t.Error("existing ledger entries must never change")
	}
}
