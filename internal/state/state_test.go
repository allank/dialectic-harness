package state

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewStateDefaults(t *testing.T) {
	st := New("zaru-order-book", "drafts/zaru-order-book.md", 3,
		map[Role]string{RoleChallenger: "agy", RoleIncumbent: "claude"})
	if st.NextRole != RoleChallenger {
		t.Errorf("NextRole: want challenger, got %s", st.NextRole)
	}
	if st.MaxRounds != 3 || st.RoundCount != 0 || st.TurnCount != 0 {
		t.Errorf("counters: got max=%d round=%d turn=%d", st.MaxRounds, st.RoundCount, st.TurnCount)
	}
	if st.Roles[RoleChallenger] != "agy" {
		t.Errorf("challenger binary: want agy, got %s", st.Roles[RoleChallenger])
	}
}

func TestContentionIDsAreStableAndSequential(t *testing.T) {
	st := New("s", "a.md", 3, nil)
	if id := st.NextContentionID(); id != "C1" {
		t.Errorf("first id: want C1, got %s", id)
	}
	if id := st.NextContentionID(); id != "C2" {
		t.Errorf("second id: want C2, got %s", id)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	st := New("slug", "a.md", 3, map[Role]string{RoleChallenger: "claude", RoleIncumbent: "agy"})
	st.ActiveContentions = append(st.ActiveContentions, Contention{
		ID: st.NextContentionID(), Issue: "In-memory vs Redis", Severity: "high",
		Stances: map[Role]string{RoleChallenger: "Redis is required."},
	})
	st.ContentionHistory = append(st.ContentionHistory, LedgerEntry{
		Turn: 1, Agent: RoleChallenger, Contention: "C1", Stance: StanceNew,
		Rationale: "In-memory fails DR expectations.",
	})
	st.RequiredRebuttals = append(st.RequiredRebuttals, Directive{
		Target: RoleIncumbent, Contention: "C1", Directive: "Address DR.", IssuedTurn: 1,
	})
	st.Sessions[RoleChallenger] = "conv-abc123"
	st.TurnCount = 1
	st.NextRole = RoleIncumbent

	path := filepath.Join(t.TempDir(), "debate-state.yaml")
	if err := st.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(st, got) {
		t.Errorf("round trip mismatch:\nsaved:  %+v\nloaded: %+v", st, got)
	}
}

func TestFindActiveAndIsConsensus(t *testing.T) {
	st := New("s", "a.md", 3, nil)
	st.ActiveContentions = []Contention{{ID: "C1", Issue: "x"}}
	st.ConsensusBaseline = []ConsensusItem{{ID: "C2", Issue: "y", ResolvedTurn: 2, Rationale: "agreed"}}
	if st.FindActive("C1") == nil {
		t.Error("FindActive(C1) should be non-nil")
	}
	if st.FindActive("C2") != nil {
		t.Error("FindActive(C2) should be nil")
	}
	if !st.IsConsensus("C2") || st.IsConsensus("C1") {
		t.Error("IsConsensus wrong")
	}
}

func TestValidStance(t *testing.T) {
	for _, s := range []Stance{StanceConcur, StanceRebut, StanceWithdraw, StanceNew} {
		if !ValidStance(s) {
			t.Errorf("ValidStance(%s) should be true", s)
		}
	}
	if ValidStance("agree") {
		t.Error(`ValidStance("agree") should be false`)
	}
}
