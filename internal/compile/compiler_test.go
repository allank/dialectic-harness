package compile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/agent"
)

type cannedRunner struct {
	payloads []string
	requests []agent.Request
}

func (c *cannedRunner) Invoke(_ context.Context, req agent.Request) (agent.Result, error) {
	c.requests = append(c.requests, req)
	if len(c.payloads) == 0 {
		return agent.Result{}, errors.New("out of payloads")
	}
	p := c.payloads[0]
	c.payloads = c.payloads[1:]
	if err := os.WriteFile(req.OutputPath, []byte(p), 0o644); err != nil {
		return agent.Result{}, err
	}
	return agent.Result{Output: []byte(p), SessionID: "should-be-ignored"}, nil
}

func TestRunCompilerAcceptsValidDoc(t *testing.T) {
	r := &cannedRunner{payloads: []string{validCompilerDoc}}
	out := filepath.Join(t.TempDir(), "brief-body.md")
	doc, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "/runs/debate-state.yaml", t.TempDir(), out)
	if err != nil {
		t.Fatalf("RunCompiler: %v", err)
	}
	if doc != validCompilerDoc {
		t.Error("returned doc should be the validated output")
	}
	if c := c1SessionOf(r); c != "" {
		t.Errorf("compiler must be sessionless, got session %q", c)
	}
	if !strings.Contains(r.requests[0].Prompt, "/runs/debate-state.yaml") {
		t.Error("compiler prompt must point at the ledger")
	}
}

func c1SessionOf(r *cannedRunner) string { return r.requests[0].SessionID }

func TestRunCompilerRetriesOnceThenFails(t *testing.T) {
	bad := "## Narrative\n\nno citations here\n"
	r := &cannedRunner{payloads: []string{bad, bad}}
	out := filepath.Join(t.TempDir(), "brief-body.md")
	_, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "state.yaml", t.TempDir(), out)
	if err == nil {
		t.Fatal("want error after failed retry")
	}
	if len(r.requests) != 2 {
		t.Fatalf("want exactly 2 invocations, got %d", len(r.requests))
	}
	if !strings.Contains(r.requests[1].Prompt, "missing required section") {
		t.Errorf("retry prompt must include validation errors:\n%s", r.requests[1].Prompt)
	}
}
