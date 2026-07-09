package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Request struct {
	Binary     string
	Prompt     string
	WorkDir    string
	SessionID  string // empty: start a new session
	OutputPath string // file the agent is instructed to write
}

type Result struct {
	Output    []byte // contents of OutputPath
	SessionID string // parsed session/conversation id; empty if unknown
}

type Runner interface {
	Invoke(ctx context.Context, req Request) (Result, error)
}

type BinarySpec struct {
	NewArgs      func(prompt string) []string
	ResumeArgs   func(sessionID, prompt string) []string
	ParseSession func(stdout []byte) string
}

// AgySessionSentinel marks that an agy session exists but has no
// addressable id: agy resumes via -c (most recent conversation).
const AgySessionSentinel = "agy-most-recent"

func parseJSONSessionID(stdout []byte) string {
	var v struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(stdout, &v); err == nil {
		return v.SessionID
	}
	return ""
}

type ExecRunner struct {
	Specs map[string]BinarySpec
}

func NewExecRunner() *ExecRunner {
	return &ExecRunner{Specs: map[string]BinarySpec{
		"claude": {
			NewArgs: func(p string) []string { return []string{"-p", p, "--output-format", "json"} },
			ResumeArgs: func(id, p string) []string {
				return []string{"-p", "--resume", id, p, "--output-format", "json"}
			},
			ParseSession: parseJSONSessionID,
		},
		"agy": {
			NewArgs: func(p string) []string { return []string{"--print", p} },
			// agy resumes the MOST RECENT conversation via -c; there is no
			// addressable id, so the sentinel stands in for one. Known risk
			// (accepted, handled at the CLI layer in a later task): if both
			// debate roles run on agy, -c cannot tell the two conversations
			// apart.
			ResumeArgs: func(_, p string) []string {
				return []string{"-c", "--print", p}
			},
			ParseSession: func([]byte) string { return AgySessionSentinel },
		},
	}}
}

func (r *ExecRunner) spec(binary string) BinarySpec {
	if s, ok := r.Specs[filepath.Base(binary)]; ok {
		return s
	}
	return BinarySpec{
		NewArgs:      func(p string) []string { return []string{p} },
		ResumeArgs:   func(_, p string) []string { return []string{p} },
		ParseSession: parseJSONSessionID,
	}
}

func (r *ExecRunner) Invoke(ctx context.Context, req Request) (Result, error) {
	spec := r.spec(req.Binary)
	var args []string
	if req.SessionID != "" {
		args = spec.ResumeArgs(req.SessionID, req.Prompt)
	} else {
		args = spec.NewArgs(req.Prompt)
	}
	cmd := exec.CommandContext(ctx, req.Binary, args...)
	cmd.Dir = req.WorkDir
	cmd.Env = append(os.Environ(), "DIALECTIC_OUTPUT_FILE="+req.OutputPath)
	stdout, err := cmd.Output()
	if err != nil {
		return Result{}, fmt.Errorf("invoke %s: %w", req.Binary, err)
	}
	out, err := os.ReadFile(req.OutputPath)
	if err != nil {
		return Result{}, fmt.Errorf("agent %s exited but wrote no output file at %s: %w", req.Binary, req.OutputPath, err)
	}
	return Result{Output: out, SessionID: spec.ParseSession(stdout)}, nil
}
