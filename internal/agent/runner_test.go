package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStub creates an executable script that echoes its argv to argv.txt in
// its cwd and writes a canned payload to $DIALECTIC_OUTPUT_FILE.
func writeStub(t *testing.T, dir, payload string) string {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > argv.txt\nprintf '%s' '" + payload + "' > \"$DIALECTIC_OUTPUT_FILE\"\necho '{\"session_id\":\"sess-123\"}'\n"
	path := filepath.Join(dir, "stub-agent")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInvokeRunsBinaryInWorkDirAndReadsOutputFile(t *testing.T) {
	workDir := t.TempDir()
	stub := writeStub(t, t.TempDir(), "agent: challenger")
	out := filepath.Join(t.TempDir(), "turn.yaml")

	r := NewExecRunner()
	res, err := r.Invoke(context.Background(), Request{
		Binary: stub, Prompt: "the prompt", WorkDir: workDir, OutputPath: out,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if string(res.Output) != "agent: challenger" {
		t.Errorf("Output: got %q", res.Output)
	}
	argv, err := os.ReadFile(filepath.Join(workDir, "argv.txt"))
	if err != nil {
		t.Fatalf("stub must run with cwd=WorkDir: %v", err)
	}
	if !strings.Contains(string(argv), "the prompt") {
		t.Errorf("prompt must be passed as an argument, argv: %q", argv)
	}
}

func TestInvokeFailsWhenOutputFileMissing(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nexit 0\n"
	stub := filepath.Join(dir, "silent-agent")
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewExecRunner()
	_, err := r.Invoke(context.Background(), Request{
		Binary: stub, Prompt: "p", WorkDir: t.TempDir(), OutputPath: filepath.Join(t.TempDir(), "missing.yaml"),
	})
	if err == nil {
		t.Fatal("Invoke must fail when the agent writes no output file")
	}
}

func TestInvokeIncludesStderrInErrorOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'rate limit exceeded, retry after 30s' >&2\nexit 1\n"
	stub := filepath.Join(dir, "failing-agent")
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := NewExecRunner()
	_, err := r.Invoke(context.Background(), Request{
		Binary: stub, Prompt: "p", WorkDir: t.TempDir(), OutputPath: filepath.Join(t.TempDir(), "turn.yaml"),
	})
	if err == nil {
		t.Fatal("Invoke must fail when the agent exits non-zero")
	}
	if !strings.Contains(err.Error(), "rate limit exceeded, retry after 30s") {
		t.Errorf("error must include the subprocess's stderr for diagnosability, got: %v", err)
	}
}

func TestClaudeSpecArgs(t *testing.T) {
	spec := NewExecRunner().Specs["claude"]
	got := strings.Join(spec.NewArgs("hello"), " ")
	if !strings.Contains(got, "-p hello") || !strings.Contains(got, "--output-format json") {
		t.Errorf("claude new args: %q", got)
	}
	got = strings.Join(spec.ResumeArgs("sess-1", "hello"), " ")
	if !strings.Contains(got, "--resume sess-1") {
		t.Errorf("claude resume args: %q", got)
	}
	if id := spec.ParseSession([]byte(`{"session_id":"abc","result":"..."}`)); id != "abc" {
		t.Errorf("claude session parse: got %q", id)
	}
}

func TestAgySpecArgs(t *testing.T) {
	spec := NewExecRunner().Specs["agy"]
	got := strings.Join(spec.NewArgs("hello"), " ")
	if !strings.Contains(got, "--print hello") {
		t.Errorf("agy new args: %q", got)
	}
	if strings.Contains(got, "-c") {
		t.Errorf("agy turn 1 must NOT continue a prior conversation: %q", got)
	}
	// agy has no addressable conversation id in print mode; -c continues the
	// most recent conversation. The parser returns a sentinel so the loop
	// knows to resume on subsequent turns.
	got = strings.Join(spec.ResumeArgs(AgySessionSentinel, "hello"), " ")
	if !strings.Contains(got, "-c --print hello") {
		t.Errorf("agy resume args: %q", got)
	}
	if id := spec.ParseSession([]byte("any output")); id != AgySessionSentinel {
		t.Errorf("agy session parse: want sentinel %q, got %q", AgySessionSentinel, id)
	}
}

func TestUnknownBinaryGetsGenericSpecAndSessionFromStdout(t *testing.T) {
	stub := writeStub(t, t.TempDir(), "x: y")
	out := filepath.Join(t.TempDir(), "o.yaml")
	r := NewExecRunner()
	res, err := r.Invoke(context.Background(), Request{Binary: stub, Prompt: "p", WorkDir: t.TempDir(), OutputPath: out})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// Generic spec parses claude-style JSON session ids when present.
	if res.SessionID != "sess-123" {
		t.Errorf("SessionID: got %q", res.SessionID)
	}
}
