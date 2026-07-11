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
		got := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", tc.retryErrors)
		path := filepath.Join("testdata", tc.name+".golden.txt")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}
}
