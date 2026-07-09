package agent

import (
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

func TestOpeningCritiquePrompt(t *testing.T) {
	p := BuildPrompt(PromptInput{
		Role:           state.RoleChallenger,
		ArtifactPath:   "artifact.md",
		TurnFilePath:   "/runs/turns/turn-1-challenger.yaml",
		MaxContentions: 5,
	})
	for _, want := range []string{
		"artifact.md",
		"/runs/turns/turn-1-challenger.yaml",
		"at most 5",
		"stance: new",
		"Do not edit the artifact",
		"agent: challenger",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("opening critique prompt missing %q\n---\n%s", want, p)
		}
	}
	if strings.Contains(p, "debate-state") {
		t.Error("opening critique must not reference prior state — there is none")
	}
}

func TestTurnPromptIncludesStatePointerAndDirectives(t *testing.T) {
	p := BuildPrompt(PromptInput{
		Role:         state.RoleIncumbent,
		ArtifactPath: "drafts/zaru.md",
		StatePath:    "/runs/debate-state.yaml",
		TurnFilePath: "/runs/turns/turn-2-incumbent.yaml",
		Directives: []state.Directive{
			{Target: state.RoleIncumbent, Contention: "C1", Directive: "Provide a latency benchmark.", IssuedTurn: 1},
		},
	})
	for _, want := range []string{
		"/runs/debate-state.yaml",
		"drafts/zaru.md",
		"agent: incumbent",
		"C1",
		"Provide a latency benchmark.",
		"concur | rebut | withdraw | new",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("turn prompt missing %q\n---\n%s", want, p)
		}
	}
}

func TestRetryPromptFeedsBackSpecificErrors(t *testing.T) {
	p := BuildPrompt(PromptInput{
		Role:         state.RoleChallenger,
		ArtifactPath: "a.md",
		StatePath:    "/runs/debate-state.yaml",
		TurnFilePath: "/runs/turns/turn-3-challenger.yaml",
		RetryErrors:  []string{`entries[0]: rationale is mandatory; bare concessions are invalid`},
	})
	if !strings.Contains(p, "rationale is mandatory") {
		t.Errorf("retry prompt must contain the specific validation error:\n%s", p)
	}
	if !strings.Contains(p, "rewrite the complete turn file") {
		t.Errorf("retry prompt must ask for a full rewrite:\n%s", p)
	}
}
