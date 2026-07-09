package turn

import (
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

func fixtureState() *state.DebateState {
	st := state.New("s", "a.md", 3, nil)
	st.TurnCount = 1
	st.NextRole = state.RoleIncumbent
	st.ContentionCounter = 2
	st.ActiveContentions = []state.Contention{{ID: "C1", Issue: "store choice", Severity: "high"}}
	st.ConsensusBaseline = []state.ConsensusItem{{ID: "C2", Issue: "language", ResolvedTurn: 1, Rationale: "agreed"}}
	return st
}

func TestParseStrictRejectsUnknownFields(t *testing.T) {
	raw := []byte("agent: incumbent\nentries:\n  - contention: C1\n    stance: rebut\n    rationale: because\nhistory_rewrite: [1, 2]\n")
	if _, err := Parse(raw); err == nil {
		t.Fatal("Parse should reject unknown top-level fields (state-write attempts)")
	}
}

func TestParseRejectsMalformedYAML(t *testing.T) {
	if _, err := Parse([]byte("agent: [unclosed")); err == nil {
		t.Fatal("Parse should fail on unparseable YAML")
	}
}

func TestValidTurnFilePasses(t *testing.T) {
	tf := File{
		Agent: state.RoleIncumbent,
		Entries: []Entry{
			{Contention: "C1", Stance: state.StanceRebut, Rationale: "latency budget rules Redis out", Position: "In-memory is faster."},
			{Stance: state.StanceNew, Issue: "missing SLO section", Severity: "medium", Rationale: "artifact defines no SLOs"},
		},
		Directives: []DirectiveRequest{{Contention: "C1", Directive: "Provide a latency benchmark."}},
	}
	if errs := Validate(tf, fixtureState()); len(errs) != 0 {
		t.Fatalf("want valid, got errors: %v", errs)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		tf      File
		wantSub string
	}{
		{"wrong agent", File{Agent: state.RoleChallenger, Entries: []Entry{{Contention: "C1", Stance: state.StanceRebut, Rationale: "r"}}}, "expected turn from incumbent"},
		{"no entries", File{Agent: state.RoleIncumbent}, "at least one entry"},
		{"bad stance", File{Agent: state.RoleIncumbent, Entries: []Entry{{Contention: "C1", Stance: "agree", Rationale: "r"}}}, "invalid stance"},
		{"missing rationale", File{Agent: state.RoleIncumbent, Entries: []Entry{{Contention: "C1", Stance: state.StanceConcur}}}, "rationale"},
		{"unknown contention", File{Agent: state.RoleIncumbent, Entries: []Entry{{Contention: "C99", Stance: state.StanceRebut, Rationale: "r"}}}, "unknown contention"},
		{"relitigate consensus", File{Agent: state.RoleIncumbent, Entries: []Entry{{Contention: "C2", Stance: state.StanceRebut, Rationale: "r"}}}, "already resolved"},
		{"new without issue", File{Agent: state.RoleIncumbent, Entries: []Entry{{Stance: state.StanceNew, Rationale: "r"}}}, "issue"},
		{"new with id", File{Agent: state.RoleIncumbent, Entries: []Entry{{Contention: "C7", Stance: state.StanceNew, Issue: "x", Rationale: "r"}}}, "orchestrator assigns"},
		{"directive unknown id", File{Agent: state.RoleIncumbent, Entries: []Entry{{Contention: "C1", Stance: state.StanceRebut, Rationale: "r"}}, Directives: []DirectiveRequest{{Contention: "C99", Directive: "d"}}}, "unknown contention"},
	}
	st := fixtureState()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := Validate(tc.tf, st)
			if len(errs) == 0 {
				t.Fatal("want validation errors, got none")
			}
			joined := strings.Join(errs, "; ")
			if !strings.Contains(joined, tc.wantSub) {
				t.Errorf("errors %q should contain %q", joined, tc.wantSub)
			}
		})
	}
}
