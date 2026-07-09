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

func TestDirectiveTargetingOtherAgentIsUntouched(t *testing.T) {
	st := stateWithC1C2(t)
	st.RequiredRebuttals = append(st.RequiredRebuttals, state.Directive{
		Target: state.RoleIncumbent, Contention: "C2", Directive: "Address rollback.", IssuedTurn: 1,
	})
	// Incumbent's own turn: the directive targets the incumbent, and it cites C2 — satisfied.
	_ = Merge(st, turn.File{Agent: state.RoleIncumbent, Entries: []turn.Entry{
		{Contention: "C2", Stance: state.StanceConcur, Rationale: "rollback section agreed"},
	}})
	if len(st.RequiredRebuttals) != 0 || len(st.IgnoredDirectives) != 0 {
		t.Errorf("directive should be satisfied: req=%+v ign=%+v", st.RequiredRebuttals, st.IgnoredDirectives)
	}
}
