package engine

import (
	"testing"

	"github.com/allank/dialectic/internal/state"
	"github.com/allank/dialectic/internal/turn"
)

func stateWithC1C2(t *testing.T) *state.DebateState {
	t.Helper()
	st := openingState()
	if err := Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Stance: state.StanceNew, Issue: "store choice", Severity: "high", Rationale: "Redis needed"},
		{Stance: state.StanceNew, Issue: "no rollback", Severity: "high", Rationale: "missing section"},
	}}); err != nil {
		t.Fatalf("seed merge: %v", err)
	}
	return st
}

func TestDirectiveIssuedTargetsOtherRole(t *testing.T) {
	st := stateWithC1C2(t)
	_ = Merge(st, turn.File{
		Agent:      state.RoleIncumbent,
		Entries:    []turn.Entry{{Contention: "C1", Stance: state.StanceRebut, Rationale: "latency"}},
		Directives: []turn.DirectiveRequest{{Contention: "C1", Directive: "Provide a latency benchmark at 10k TPS."}},
	})
	if len(st.RequiredRebuttals) != 1 {
		t.Fatalf("required_rebuttals: want 1, got %+v", st.RequiredRebuttals)
	}
	d := st.RequiredRebuttals[0]
	if d.Target != state.RoleChallenger || d.Contention != "C1" || d.IssuedTurn != 2 {
		t.Errorf("directive: %+v", d)
	}
}

func TestDirectiveSatisfiedWhenTargetCitesContention(t *testing.T) {
	st := stateWithC1C2(t)
	_ = Merge(st, turn.File{
		Agent:      state.RoleIncumbent,
		Entries:    []turn.Entry{{Contention: "C1", Stance: state.StanceRebut, Rationale: "latency"}},
		Directives: []turn.DirectiveRequest{{Contention: "C1", Directive: "Provide a benchmark."}},
	})
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceRebut, Rationale: "benchmark: 0.4ms p99 at 10k TPS"},
	}})
	if len(st.RequiredRebuttals) != 0 {
		t.Errorf("satisfied directive should be removed, got %+v", st.RequiredRebuttals)
	}
	if len(st.IgnoredDirectives) != 0 {
		t.Errorf("nothing ignored, got %+v", st.IgnoredDirectives)
	}
}

func TestDirectiveIgnoredWhenTargetDodges(t *testing.T) {
	st := stateWithC1C2(t)
	_ = Merge(st, turn.File{
		Agent:      state.RoleIncumbent,
		Entries:    []turn.Entry{{Contention: "C1", Stance: state.StanceRebut, Rationale: "latency"}},
		Directives: []turn.DirectiveRequest{{Contention: "C1", Directive: "Provide a benchmark."}},
	})
	// Challenger's next turn addresses only C2 — dodging the C1 directive.
	_ = Merge(st, turn.File{Agent: state.RoleChallenger, Entries: []turn.Entry{
		{Contention: "C2", Stance: state.StanceRebut, Rationale: "still no rollback"},
	}})
	if len(st.RequiredRebuttals) != 0 {
		t.Errorf("dodged directive should leave required_rebuttals, got %+v", st.RequiredRebuttals)
	}
	if len(st.IgnoredDirectives) != 1 {
		t.Fatalf("ignored: want 1, got %+v", st.IgnoredDirectives)
	}
	if st.IgnoredDirectives[0].Contention != "C1" || st.IgnoredDirectives[0].Target != state.RoleChallenger {
		t.Errorf("ignored directive: %+v", st.IgnoredDirectives[0])
	}
}

func TestDirectiveTargetingOtherAgentIsUntouchedByThisTurn(t *testing.T) {
	st := stateWithC1C2(t)
	directive := state.Directive{
		Target: state.RoleChallenger, Contention: "C1", Directive: "Provide evidence.", IssuedTurn: 1,
	}
	st.RequiredRebuttals = append(st.RequiredRebuttals, directive)
	// Incumbent's turn cites the SAME contention (C1) the directive targets, but the
	// directive targets the challenger, not the incumbent — satisfaction only applies
	// to directives targeting the CURRENT turn's agent, so this must leave the
	// directive completely unchanged: not satisfied, not moved to ignored.
	_ = Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C1", Stance: state.StanceRebut, Rationale: "latency"},
	}})
	if len(st.RequiredRebuttals) != 1 {
		t.Fatalf("directive targeting the other agent must remain in required_rebuttals, got %+v", st.RequiredRebuttals)
	}
	if st.RequiredRebuttals[0] != directive {
		t.Errorf("directive must be byte-for-byte unchanged: got %+v, want %+v", st.RequiredRebuttals[0], directive)
	}
	if len(st.IgnoredDirectives) != 0 {
		t.Errorf("directive targeting the other agent must not be moved to ignored, got %+v", st.IgnoredDirectives)
	}
}
