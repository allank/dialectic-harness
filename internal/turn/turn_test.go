package turn

import (
	"fmt"
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

const validMinimalTurn = "agent: incumbent\nentries:\n  - contention: C1\n    stance: rebut\n    rationale: because\n"

func TestParseStripsTrailingLeakedClosingTag(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"with trailing newline", validMinimalTurn + "</content>\n"},
		{"without trailing newline", validMinimalTurn + "</content>"},
		{"blank line before tag", validMinimalTurn + "\n</content>\n"},
	}
	want, err := Parse([]byte(validMinimalTurn))
	if err != nil {
		t.Fatalf("baseline Parse: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse([]byte(tc.raw))
			if err != nil {
				t.Fatalf("Parse should strip the leaked tag and succeed, got: %v", err)
			}
			if got.Agent != want.Agent || len(got.Entries) != len(want.Entries) {
				t.Errorf("stripped parse result diverges from baseline: got %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseDoesNotStripATagThatIsAllTheContent(t *testing.T) {
	if _, err := Parse([]byte("</content>\n")); err == nil {
		t.Fatal("a file that is nothing but a closing tag should still fail to parse")
	}
}

func TestParseDoesNotStripANonTrailingTag(t *testing.T) {
	raw := "agent: incumbent\nentries:\n  - contention: C1\n    stance: rebut\n    rationale: >\n      </content> mentioned mid-rationale\n"
	got, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(got.Entries[0].Rationale, "</content>") {
		t.Errorf("a tag that is genuinely part of the content must not be stripped, got rationale: %q", got.Entries[0].Rationale)
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
	if errs := Validate(tf, fixtureState(), 5); len(errs) != 0 {
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
			errs := Validate(tc.tf, st, 5)
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

func newEntries(n int) []Entry {
	entries := make([]Entry, n)
	for i := range entries {
		entries[i] = Entry{Stance: state.StanceNew, Issue: fmt.Sprintf("issue %d", i), Rationale: "r"}
	}
	return entries
}

func TestOpeningCritiqueOverCapIsInvalid(t *testing.T) {
	st := state.New("s", "a.md", 3, nil) // TurnCount 0, NextRole challenger (zero values)
	st.NextRole = state.RoleChallenger
	tf := File{Agent: state.RoleChallenger, Entries: newEntries(6)}
	errs := Validate(tf, st, 5)
	joined := strings.Join(errs, "; ")
	if !strings.Contains(joined, "exceeding the cap of 5") {
		t.Errorf("want cap-exceeded error, got: %q", joined)
	}
}

func TestOpeningCritiqueAtCapIsValid(t *testing.T) {
	st := state.New("s", "a.md", 3, nil)
	st.NextRole = state.RoleChallenger
	tf := File{Agent: state.RoleChallenger, Entries: newEntries(5)}
	if errs := Validate(tf, st, 5); len(errs) != 0 {
		t.Errorf("want valid at exactly the cap, got errors: %v", errs)
	}
}

func TestLaterTurnNewContentionsAreUncapped(t *testing.T) {
	st := fixtureState() // TurnCount 1: not the opening critique
	st.NextRole = state.RoleIncumbent
	tf := File{Agent: state.RoleIncumbent, Entries: newEntries(6)}
	errs := Validate(tf, st, 5)
	for _, e := range errs {
		if strings.Contains(e, "exceeding the cap") {
			t.Errorf("cap must not apply past the opening critique, got: %v", errs)
		}
	}
}
