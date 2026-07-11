package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

// goldenCases is the shared input matrix for the golden-fixture capture
// (this test) and the default-rendering regression test (added in Step 6,
// after the refactor). Keeping the cases in one place means both tests stay
// in sync by construction.
var goldenCases = []struct {
	name string
	in   PromptInput
}{
	{
		name: "opening_critique_no_directives",
		in: PromptInput{
			Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md",
			TurnFilePath: "/run/turns/turn-1-challenger.yaml", MaxContentions: 5,
		},
	},
	{
		name: "regular_turn_incumbent_with_directives",
		in: PromptInput{
			Role: state.RoleIncumbent, ArtifactPath: "/vault/doc.md", StatePath: "/run/debate-state.yaml",
			TurnFilePath: "/run/turns/turn-2-incumbent.yaml", MaxContentions: 5,
			Directives: []state.Directive{{Target: state.RoleIncumbent, Contention: "C1", Directive: "Address the latency concern.", IssuedTurn: 1}},
		},
	},
	{
		name: "regular_turn_challenger_with_retry",
		in: PromptInput{
			Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md", StatePath: "/run/debate-state.yaml",
			TurnFilePath: "/run/turns/turn-3-challenger.yaml", MaxContentions: 5,
			RetryErrors: []string{"entries[0]: rationale is mandatory; bare concessions are invalid"},
		},
	},
}

// TestUpdateGolden captures BuildPrompt's output for goldenCases into
// internal/agent/testdata/<name>.golden.txt. Run once, with UPDATE_GOLDEN=1,
// against today's implementation before any template refactor, to prove the
// fixtures reflect real behavior rather than a copy of a new implementation's
// output. The original capture (pre-refactor, single-return BuildPrompt) is
// preserved in the "test: capture BuildPrompt golden fixtures before
// templating refactor" commit; this call is updated post-refactor to the
// new two-return signature so the package keeps compiling and the fixtures
// can still be regenerated deliberately in the future.
func TestUpdateGolden(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") == "" {
		t.Skip("set UPDATE_GOLDEN=1 to regenerate")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range goldenCases {
		got, err := BuildPrompt(tc.in, nil)
		if err != nil {
			t.Fatalf("BuildPrompt: %v", err)
		}
		path := filepath.Join("testdata", tc.name+".golden.txt")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}
}
