package engine

import (
	"testing"

	"github.com/allank/dialectic/internal/state"
)

func TestFreshStateRoutesChallengerForOpeningCritique(t *testing.T) {
	st := openingState()
	got := NextAction(st)
	if got.Type != ActionRoute || got.Role != state.RoleChallenger {
		t.Errorf("want route challenger, got %+v", got)
	}
}

func TestContestedStateRoutesNextRole(t *testing.T) {
	st := openingState()
	st.TurnCount = 1
	st.NextRole = state.RoleIncumbent
	st.ActiveContentions = []state.Contention{{ID: "C1", Issue: "x"}}
	got := NextAction(st)
	if got.Type != ActionRoute || got.Role != state.RoleIncumbent {
		t.Errorf("want route incumbent, got %+v", got)
	}
}

func TestEmptyActiveContentionsCompilesWithConsensusReason(t *testing.T) {
	st := openingState()
	st.TurnCount = 2
	st.RoundCount = 1
	st.NextRole = state.RoleChallenger
	st.ConsensusBaseline = []state.ConsensusItem{{ID: "C1", Issue: "x", ResolvedTurn: 2, Rationale: "agreed"}}
	got := NextAction(st)
	if got.Type != ActionCompile || got.Reason != "consensus" {
		t.Errorf("want compile/consensus, got %+v", got)
	}
}

func TestRoundLimitCompilesWithContentionsPreserved(t *testing.T) {
	st := openingState()
	st.TurnCount = 6
	st.RoundCount = 3 // == MaxRounds
	st.NextRole = state.RoleChallenger
	st.ActiveContentions = []state.Contention{{ID: "C2", Issue: "still contested"}}
	got := NextAction(st)
	if got.Type != ActionCompile || got.Reason != "round_limit" {
		t.Errorf("want compile/round_limit, got %+v", got)
	}
	if len(st.ActiveContentions) != 1 {
		t.Error("circuit breaker must not drop active contentions")
	}
}

func TestMidRoundIncumbentStillRoutesBeforeBreakerFires(t *testing.T) {
	// Breaker counts rounds; incumbent always closes a round. After the
	// challenger's turn in round 3, RoundCount is still 2, so the incumbent
	// must get its closing turn even though this is the final round.
	st := openingState()
	st.TurnCount = 5
	st.RoundCount = 2
	st.NextRole = state.RoleIncumbent
	st.ActiveContentions = []state.Contention{{ID: "C2", Issue: "x"}}
	got := NextAction(st)
	if got.Type != ActionRoute || got.Role != state.RoleIncumbent {
		t.Errorf("want route incumbent, got %+v", got)
	}
}

func TestOpeningCritiqueWithNoContentionsCompilesCleanBill(t *testing.T) {
	st := openingState()
	st.TurnCount = 1
	st.NextRole = state.RoleIncumbent
	// Challenger raised nothing: clean bill of health, nothing to debate.
	got := NextAction(st)
	if got.Type != ActionCompile || got.Reason != "consensus" {
		t.Errorf("want compile/consensus, got %+v", got)
	}
}
