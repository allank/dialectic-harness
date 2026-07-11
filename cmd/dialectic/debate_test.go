package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	murli "github.com/murli-cli/murli-go"
)

func TestDebateEndToEndWithStubs(t *testing.T) {
	wd, _ := os.Getwd()
	stubAgent := filepath.Join(wd, "testdata", "stub-agent.sh")
	stubCompiler := filepath.Join(wd, "testdata", "stub-compiler.sh")

	dir := t.TempDir()
	artifact := filepath.Join(dir, "Test PRD.md")
	artifactBody := "# Test PRD\n\nA document with no rollback plan.\n"
	if err := os.WriteFile(artifact, []byte(artifactBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("STUB_COUNT_FILE", filepath.Join(t.TempDir(), "count"))

	root := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"debate", artifact,
		"--challenger", stubAgent,
		"--incumbent", stubAgent,
		"--compiler", stubCompiler,
		"--max-rounds", "2",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("debate: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	// Progress events land on stderr, one per stage, distinct from the final
	// JSON result envelope on stdout.
	if !strings.Contains(stderr.String(), `"stage":"turn"`) {
		t.Errorf("stderr should contain turn-stage progress events, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), `"stage":"compile"`) {
		t.Errorf("stderr should contain compile-stage progress events, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "invoking") {
		t.Errorf("stderr should contain human-readable invoking messages, got:\n%s", stderr.String())
	}

	// Artifact untouched: the debate is read-only.
	after, _ := os.ReadFile(artifact)
	if string(after) != artifactBody {
		t.Error("target artifact must never be modified")
	}

	// Run dir with state exists.
	runs, err := filepath.Glob(filepath.Join(dir, ".a2a", "test-prd-*", "debate-state.yaml"))
	if err != nil || len(runs) != 1 {
		t.Fatalf("expected one run state file, got %v (err=%v)", runs, err)
	}

	// Compiled summary beside the artifact, with C2 unresolved.
	summaries, _ := filepath.Glob(filepath.Join(dir, "test-prd-debate-summary-*.md"))
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %v", summaries)
	}
	sum, _ := os.ReadFile(summaries[0])
	for _, want := range []string{"## Decisions Made and Why", "C1", "## Unresolved Tensions", "C2"} {
		if !strings.Contains(string(sum), want) {
			t.Errorf("summary missing %q:\n%s", want, sum)
		}
	}

	// Update brief with pending verdict.
	briefs, _ := filepath.Glob(filepath.Join(dir, "test-prd-update-brief-*.md"))
	if len(briefs) != 1 {
		t.Fatalf("expected one brief, got %v", briefs)
	}
	brief, _ := os.ReadFile(briefs[0])
	for _, want := range []string{"arbiter_verdict: pending", "## Proposed Changes", "(C1, turn 2)", "## Judgment Calls", "(C2, turn 2)"} {
		if !strings.Contains(string(brief), want) {
			t.Errorf("brief missing %q:\n%s", want, brief)
		}
	}
}

// TestDebateRejectsMissingArtifact exercises the preflight validation for a
// missing artifact. Note: murliCobra wraps every RunE so that, outside a TTY
// (as here, since SetOut/SetErr point at bytes.Buffers, not *os.File), an
// *AgentError returned from RunE is translated into a stderr envelope and
// Execute() returns nil — the error never propagates as a Go error. Instead
// murli.ExitFunc (normally os.Exit) is invoked with the AgentError's code, so
// we mock it here to capture the code instead of terminating the test binary.
// This matches the pattern used in murli-go's own cobra test suite.
func TestDebateRejectsMissingArtifact(t *testing.T) {
	origExit := murli.ExitFunc
	var capturedExit int
	murli.ExitFunc = func(code int) { capturedExit = code }
	defer func() { murli.ExitFunc = origExit }()

	root := newRootCmd()
	errBuf := &bytes.Buffer{}
	root.SetOut(&bytes.Buffer{})
	root.SetErr(errBuf)
	root.SetArgs([]string{"debate", "/nonexistent/thing.md"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedExit != murli.ExitUserError {
		t.Errorf("exit code: want %d (ExitUserError), got %d\nstderr:\n%s", murli.ExitUserError, capturedExit, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "artifact not found") {
		t.Errorf("stderr should mention the missing artifact, got:\n%s", errBuf.String())
	}
}

func TestDebateOverridePromptChangesAgentInput(t *testing.T) {
	wd, _ := os.Getwd()
	captureStub := filepath.Join(wd, "testdata", "stub-capture-challenger.sh")
	incumbentStub := filepath.Join(wd, "testdata", "stub-concur-incumbent.sh")
	compilerStub := filepath.Join(wd, "testdata", "stub-compiler.sh")

	dir := t.TempDir()
	artifact := filepath.Join(dir, "Test PRD.md")
	if err := os.WriteFile(artifact, []byte("# Test PRD\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	captureFile := filepath.Join(t.TempDir(), "captured-prompt.txt")
	t.Setenv("PROMPT_CAPTURE_FILE", captureFile)

	overrideFile := filepath.Join(t.TempDir(), "override-opening-critique.txt")
	overrideText := "CUSTOM OVERRIDE MARKER: raise your concerns about this document."
	if err := os.WriteFile(overrideFile, []byte(overrideText), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"debate", artifact,
		"--challenger", captureStub,
		"--incumbent", incumbentStub,
		"--compiler", compilerStub,
		"--max-rounds", "1",
		"--override-prompt", "opening_critique=" + overrideFile,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("debate: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	captured, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("read captured prompt: %v", err)
	}
	if !strings.Contains(string(captured), "CUSTOM OVERRIDE MARKER") {
		t.Errorf("challenger's prompt must contain the override text, got:\n%s", captured)
	}
	if strings.Contains(string(captured), "You have no prior context") {
		t.Errorf("challenger's prompt must NOT contain the built-in default text when overridden, got:\n%s", captured)
	}
}

func TestDebateRejectsUnknownOverridePromptName(t *testing.T) {
	origExit := murli.ExitFunc
	var capturedExit int
	murli.ExitFunc = func(code int) { capturedExit = code }
	defer func() { murli.ExitFunc = origExit }()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "Test.md")
	if err := os.WriteFile(artifact, []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overrideFile := filepath.Join(t.TempDir(), "override.txt")
	if err := os.WriteFile(overrideFile, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	errBuf := &bytes.Buffer{}
	root.SetOut(&bytes.Buffer{})
	root.SetErr(errBuf)
	root.SetArgs([]string{"debate", artifact, "--override-prompt", "bogus_name=" + overrideFile})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedExit != murli.ExitUserError {
		t.Errorf("exit code: want %d (ExitUserError), got %d\nstderr:\n%s", murli.ExitUserError, capturedExit, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "unknown prompt name") {
		t.Errorf("stderr should mention unknown prompt name, got:\n%s", errBuf.String())
	}
}
