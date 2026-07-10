package compile

import (
	"strings"
	"testing"
)

// summaryFixture (from summary_test.go) has consensus C1, withdrawn C3,
// active C2, TurnCount 6.

const validCompilerDoc = `## Narrative

The challenger opened with two contentions. The language choice (C1, turn 3) resolved quickly; the state-store dispute (C2, turn 2) ran the full three rounds.

## Proposed Changes

- State explicitly that the matching engine will be written in Go (C1, turn 3).

## Judgment Calls

- In-memory vs Redis: fault tolerance vs latency — no benchmark was produced (C2, turn 6). Which risk do you own?
`

func TestValidCompilerDocPasses(t *testing.T) {
	if errs := ValidateCitations(validCompilerDoc, summaryFixture()); len(errs) != 0 {
		t.Fatalf("want valid, got %v", errs)
	}
}

func TestCitationValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		wantSub string
	}{
		{"missing section", "## Narrative\n\nx (C1, turn 1)\n\n## Proposed Changes\n\n- y (C1, turn 1).\n", "missing required section"},
		{"uncited proposal", strings.Replace(validCompilerDoc, "(C1, turn 3).", ".", 1), "no citation"},
		{"unknown id", strings.Replace(validCompilerDoc, "(C2, turn 6)", "(C9, turn 6)", 1), "unknown contention"},
		{"proposal cites non-consensus", strings.Replace(validCompilerDoc, "Go (C1, turn 3)", "Go (C2, turn 3)", 1), "non-consensus"},
		{"question cites consensus", strings.Replace(validCompilerDoc, "produced (C2, turn 6)", "produced (C1, turn 6)", 1), "must cite unresolved"},
		{"turn out of range", strings.Replace(validCompilerDoc, "(C2, turn 6)", "(C2, turn 99)", 1), "turn 99"},
	}
	st := summaryFixture()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateCitations(tc.doc, st)
			if len(errs) == 0 {
				t.Fatal("want errors, got none")
			}
			if !strings.Contains(strings.Join(errs, "; "), tc.wantSub) {
				t.Errorf("errors %v should mention %q", errs, tc.wantSub)
			}
		})
	}
}
