package compile

import (
	"os"
	"path/filepath"
	"testing"
)

var compilerGoldenCases = []struct {
	name        string
	retryErrors []string
}{
	{name: "compiler_no_retry", retryErrors: nil},
	{name: "compiler_with_retry", retryErrors: []string{"missing required section: ## Judgment Calls"}},
}

// TestUpdateCompilerGolden captures BuildCompilerPrompt's output into
// internal/compile/testdata/<name>.golden.txt. Run once, with
// UPDATE_GOLDEN=1, BEFORE refactoring BuildCompilerPrompt in Step 4.
func TestUpdateCompilerGolden(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") == "" {
		t.Skip("set UPDATE_GOLDEN=1 to regenerate")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	st := summaryFixture()
	for _, tc := range compilerGoldenCases {
		got, err := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", tc.retryErrors, nil)
		if err != nil {
			t.Fatalf("BuildCompilerPrompt: %v", err)
		}
		path := filepath.Join("testdata", tc.name+".golden.txt")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}
}

func TestBuildCompilerPromptDefaultMatchesGoldenFixtures(t *testing.T) {
	st := summaryFixture()
	for _, tc := range compilerGoldenCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", tc.retryErrors, nil)
			if err != nil {
				t.Fatalf("BuildCompilerPrompt: %v", err)
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

func TestBuildCompilerPromptOverrideTakesPrecedenceOverDefault(t *testing.T) {
	st := summaryFixture()
	got, err := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", nil,
		map[string]string{"compiler": "CUSTOM: read {{.StatePath}} and write to {{.OutPath}}."})
	if err != nil {
		t.Fatalf("BuildCompilerPrompt: %v", err)
	}
	if got != "CUSTOM: read /run/debate-state.yaml and write to /run/compiler-output.md." {
		t.Errorf("override must fully replace the default and render its own placeholders, got:\n%s", got)
	}
}

func TestDefaultTemplatesHasCompilerName(t *testing.T) {
	tmpls := DefaultTemplates()
	if _, ok := tmpls["compiler"]; !ok {
		t.Errorf("DefaultTemplates() missing %q", "compiler")
	}
	if len(tmpls) != 1 {
		t.Errorf("DefaultTemplates(): want exactly 1 entry, got %d: %v", len(tmpls), tmpls)
	}
}
