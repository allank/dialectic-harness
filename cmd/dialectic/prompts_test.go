package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/compile"
)

func TestPromptsCommandHumanOutput(t *testing.T) {
	root := newRootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	// --output text forces the writer's human-text branch (WriteSuccess's
	// humanText argument) regardless of stdout being a bytes.Buffer rather
	// than a real TTY. Without this, murliCobra.NewWriter's TTY detection
	// (isTerminal checks for *os.File) is always false against a
	// bytes.Buffer, so plain "prompts" would silently hit the same JSON
	// branch as --agent and never exercise the human-readable render path.
	root.SetArgs([]string{"prompts", "--output", "text"})
	if err := root.Execute(); err != nil {
		t.Fatalf("prompts: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"Opening Critique", "Turn Loop", "Compiler",
		"=== opening_critique ===", "=== turn ===", "=== schema ===", "=== compiler ===",
		"{{.ArtifactPath}}", "{{.Role", "{{.StatePath}}", "{{.TargetArtifact}}",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("prompts output missing %q, got:\n%s", want, out)
		}
	}
}

func TestPromptsCommandAgentOutput(t *testing.T) {
	root := newRootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"prompts", "--agent"})
	if err := root.Execute(); err != nil {
		t.Fatalf("prompts: %v", err)
	}
	var envelope struct {
		Result struct {
			Diagram   string            `json:"diagram"`
			Templates map[string]string `json:"templates"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("prompts --agent output is not valid JSON: %v\noutput:\n%s", err, stdout.String())
	}
	if envelope.Result.Diagram == "" {
		t.Error("agent-mode payload must include a non-empty diagram")
	}
	for _, name := range []string{"opening_critique", "turn", "schema", "compiler"} {
		if envelope.Result.Templates[name] == "" {
			t.Errorf("agent-mode payload missing template %q", name)
		}
	}
	if len(envelope.Result.Templates) != 4 {
		t.Errorf("agent-mode payload: want exactly 4 templates, got %d: %v", len(envelope.Result.Templates), envelope.Result.Templates)
	}

	// The structured payload must carry the raw template text — unmodified,
	// with no "=== name ===" header prepended — so an agent can write
	// envelope.Result.Templates["opening_critique"] straight to a file for
	// --override-prompt without a stray literal first line.
	want := agent.DefaultTemplates()
	for name, text := range compile.DefaultTemplates() {
		want[name] = text
	}
	for name, wantText := range want {
		if got := envelope.Result.Templates[name]; got != wantText {
			t.Errorf("agent-mode template %q does not match the raw source template.\nwant:\n%s\ngot:\n%s", name, wantText, got)
		}
	}
}
