package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

func TestBuildPromptDefaultMatchesGoldenFixtures(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildPrompt(tc.in, nil)
			if err != nil {
				t.Fatalf("BuildPrompt: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".golden.txt"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output diverges from golden fixture (captured from pre-refactor code)\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func TestBuildPromptOverrideTakesPrecedenceOverDefault(t *testing.T) {
	got, err := BuildPrompt(PromptInput{
		Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md",
		TurnFilePath: "/run/turns/turn-1-challenger.yaml", MaxContentions: 5,
	}, map[string]string{"opening_critique": "CUSTOM: look at {{.ArtifactPath}}."})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(got, "CUSTOM: look at /vault/doc.md.") {
		t.Errorf("override must be rendered with its own placeholders substituted, got:\n%s", got)
	}
	if strings.Contains(got, "You have no prior context") {
		t.Errorf("built-in default text must not appear when overridden, got:\n%s", got)
	}
}

func TestBuildPromptRejectsMalformedOverride(t *testing.T) {
	_, err := BuildPrompt(PromptInput{
		Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md",
		TurnFilePath: "/run/turns/turn-1-challenger.yaml", MaxContentions: 5,
	}, map[string]string{"opening_critique": "unclosed {{ .ArtifactPath"})
	if err == nil {
		t.Fatal("want an error for a malformed override template, got nil")
	}
}

func TestDefaultTemplatesHasAllThreeNames(t *testing.T) {
	tmpls := DefaultTemplates()
	for _, name := range []string{"opening_critique", "turn", "schema"} {
		if _, ok := tmpls[name]; !ok {
			t.Errorf("DefaultTemplates() missing %q", name)
		}
	}
	if len(tmpls) != 3 {
		t.Errorf("DefaultTemplates(): want exactly 3 entries, got %d: %v", len(tmpls), tmpls)
	}
}
