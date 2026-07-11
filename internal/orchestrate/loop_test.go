package orchestrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/state"
)

// scriptedRunner returns canned turn-file payloads in sequence and records
// every request it receives.
type scriptedRunner struct {
	payloads []string
	requests []agent.Request
}

func (s *scriptedRunner) Invoke(_ context.Context, req agent.Request) (agent.Result, error) {
	s.requests = append(s.requests, req)
	if len(s.payloads) == 0 {
		return agent.Result{}, errors.New("scriptedRunner: out of payloads")
	}
	p := s.payloads[0]
	s.payloads = s.payloads[1:]
	if err := os.WriteFile(req.OutputPath, []byte(p), 0o644); err != nil {
		return agent.Result{}, err
	}
	return agent.Result{Output: []byte(p), SessionID: "sess-" + fmt.Sprint(len(s.requests))}, nil
}

func newTestLoop(t *testing.T, r agent.Runner) *Loop {
	t.Helper()
	dir := t.TempDir()
	artifact := filepath.Join(dir, "prd.md")
	if err := os.WriteFile(artifact, []byte("# PRD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(dir, ".a2a", "prd-20260709T120000")
	for _, d := range []string{filepath.Join(runDir, "turns"), filepath.Join(runDir, "scratch")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	st := state.New("prd", artifact, 3, map[state.Role]string{
		state.RoleChallenger: "stub-challenger", state.RoleIncumbent: "stub-incumbent",
	})
	return &Loop{
		State:          st,
		StatePath:      filepath.Join(runDir, "debate-state.yaml"),
		ArtifactPath:   artifact,
		ScratchDir:     filepath.Join(runDir, "scratch"),
		TurnsDir:       filepath.Join(runDir, "turns"),
		Runner:         r,
		MaxContentions: 5,
	}
}

const openingPayload = `agent: challenger
entries:
  - stance: new
    issue: "no rollback plan"
    severity: high
    rationale: "artifact lacks a rollback section"
`

const concurPayload = `agent: incumbent
entries:
  - contention: C1
    stance: concur
    rationale: "agreed, rollback section needed"
`

func TestRunReachesConsensusInOneRound(t *testing.T) {
	r := &scriptedRunner{payloads: []string{openingPayload, concurPayload}}
	l := newTestLoop(t, r)
	reason, err := l.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != "consensus" {
		t.Errorf("reason: want consensus, got %s", reason)
	}
	if len(l.State.ConsensusBaseline) != 1 || l.State.TurnCount != 2 {
		t.Errorf("state: %+v", l.State)
	}
	// Challenger's turn 1 must run clean-room: cwd is the scratch dir and the
	// artifact reference is the scratch copy, not the vault path.
	first := r.requests[0]
	if first.WorkDir != l.ScratchDir {
		t.Errorf("challenger cwd: want scratch %s, got %s", l.ScratchDir, first.WorkDir)
	}
	if !strings.Contains(first.Prompt, filepath.Base(l.ArtifactPath)) || strings.Contains(first.Prompt, l.ArtifactPath) {
		t.Errorf("challenger prompt must point at the scratch copy, got:\n%s", first.Prompt)
	}
	// Incumbent runs in the artifact's own directory with vault context.
	second := r.requests[1]
	if second.WorkDir != filepath.Dir(l.ArtifactPath) {
		t.Errorf("incumbent cwd: want %s, got %s", filepath.Dir(l.ArtifactPath), second.WorkDir)
	}
	// Sessions captured for resume.
	if l.State.Sessions[state.RoleChallenger] != "sess-1" {
		t.Errorf("challenger session: %q", l.State.Sessions[state.RoleChallenger])
	}
	// State persisted after final merge.
	saved, err := state.Load(l.StatePath)
	if err != nil || saved.TurnCount != 2 {
		t.Errorf("saved state: %+v err=%v", saved, err)
	}
}

func TestRunHitsRoundLimitPreservingContentions(t *testing.T) {
	rebutC := `agent: challenger
entries:
  - contention: C1
    stance: rebut
    rationale: "still unconvinced"
`
	rebutI := `agent: incumbent
entries:
  - contention: C1
    stance: rebut
    rationale: "still disagree"
`
	r := &scriptedRunner{payloads: []string{openingPayload, rebutI, rebutC, rebutI, rebutC, rebutI}}
	l := newTestLoop(t, r)
	l.State.MaxRounds = 3
	reason, err := l.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != "round_limit" {
		t.Errorf("reason: want round_limit, got %s", reason)
	}
	if len(l.State.ActiveContentions) != 1 {
		t.Error("unresolved tension must be preserved, not discarded")
	}
	if l.State.RoundCount != 3 {
		t.Errorf("RoundCount: want 3, got %d", l.State.RoundCount)
	}
	// Challenger turns after turn 1 read the state COPY inside the scratch
	// dir, never the run-dir path (clean room holds artifact + state).
	turn3 := r.requests[2]
	if !strings.Contains(turn3.Prompt, filepath.Join(l.ScratchDir, "debate-state.yaml")) {
		t.Errorf("challenger turn 3 must point at the scratch state copy:\n%s", turn3.Prompt)
	}
	// The scratch state copy must NOT leak the real vault-adjacent artifact
	// path: target_artifact in the copy must be rewritten to the scratch
	// artifact's own path, never l.ArtifactPath. A raw byte copy of the real
	// state file would fail this assertion.
	scratchStatePath := filepath.Join(l.ScratchDir, "debate-state.yaml")
	scratchState, err := state.Load(scratchStatePath)
	if err != nil {
		t.Fatalf("load scratch state copy: %v", err)
	}
	if scratchState.TargetArtifact == l.ArtifactPath {
		t.Errorf("scratch state copy leaks the real vault artifact path: target_artifact=%q", scratchState.TargetArtifact)
	}
	wantScratchArtifact := filepath.Join(l.ScratchDir, filepath.Base(l.ArtifactPath))
	if scratchState.TargetArtifact != wantScratchArtifact {
		t.Errorf("scratch state target_artifact: want scratch copy path %q, got %q", wantScratchArtifact, scratchState.TargetArtifact)
	}
}

func TestInvalidTurnRetriesOnceWithErrorsThenHalts(t *testing.T) {
	missingRationale := `agent: challenger
entries:
  - stance: new
    issue: "x"
`
	r := &scriptedRunner{payloads: []string{missingRationale, missingRationale}}
	l := newTestLoop(t, r)
	_, err := l.Run(context.Background())
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("want ErrHalted, got %v", err)
	}
	if len(r.requests) != 2 {
		t.Fatalf("want exactly 2 invocations (original + one retry), got %d", len(r.requests))
	}
	if !strings.Contains(r.requests[1].Prompt, "rationale is mandatory") {
		t.Errorf("retry prompt must feed back the specific error:\n%s", r.requests[1].Prompt)
	}
	// Halted, not corrupted: state file exists and is loadable.
	if _, err := state.Load(l.StatePath); err != nil {
		t.Errorf("state must be preserved on halt: %v", err)
	}
}

func TestOpeningCritiqueOverCapRetriesOnceThenHalts(t *testing.T) {
	tooMany := `agent: challenger
entries:
  - stance: new
    issue: "issue 1"
    rationale: "r"
  - stance: new
    issue: "issue 2"
    rationale: "r"
  - stance: new
    issue: "issue 3"
    rationale: "r"
  - stance: new
    issue: "issue 4"
    rationale: "r"
  - stance: new
    issue: "issue 5"
    rationale: "r"
  - stance: new
    issue: "issue 6"
    rationale: "r"
`
	r := &scriptedRunner{payloads: []string{tooMany, tooMany}}
	l := newTestLoop(t, r) // MaxContentions: 5
	_, err := l.Run(context.Background())
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("want ErrHalted, got %v", err)
	}
	if len(r.requests) != 2 {
		t.Fatalf("want exactly 2 invocations (original + one retry), got %d", len(r.requests))
	}
	if !strings.Contains(r.requests[1].Prompt, "exceeding the cap of 5") {
		t.Errorf("retry prompt must feed back the cap violation:\n%s", r.requests[1].Prompt)
	}
}

func TestRetrySucceedsAndRunContinues(t *testing.T) {
	missingRationale := `agent: challenger
entries:
  - stance: new
    issue: "x"
`
	r := &scriptedRunner{payloads: []string{missingRationale, openingPayload, concurPayload}}
	l := newTestLoop(t, r)
	reason, err := l.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if reason != "consensus" {
		t.Errorf("reason: want consensus, got %s", reason)
	}
}

func TestRunReportsProgressForEachTurn(t *testing.T) {
	r := &scriptedRunner{payloads: []string{openingPayload, concurPayload}}
	l := newTestLoop(t, r)
	var events []progress.Event
	l.Progress = func(ev progress.Event) { events = append(events, ev) }

	_, err := l.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"invoking challenger (turn 1)",
		"turn 1 (challenger) complete: 1 new contentions, 0 resolved to consensus",
		"invoking incumbent (turn 2)",
		"turn 2 (incumbent) complete: 0 new contentions, 1 resolved to consensus",
	}
	if len(events) != len(want) {
		t.Fatalf("event count: want %d, got %d: %+v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].Message != w {
			t.Errorf("event %d: want %q, got %q", i, w, events[i].Message)
		}
		if events[i].Stage != "turn" {
			t.Errorf("event %d: want stage \"turn\", got %q", i, events[i].Stage)
		}
	}
}

func TestRunReportsProgressForRetry(t *testing.T) {
	missingRationale := `agent: challenger
entries:
  - stance: new
    issue: "x"
`
	r := &scriptedRunner{payloads: []string{missingRationale, openingPayload, concurPayload}}
	l := newTestLoop(t, r)
	var events []progress.Event
	l.Progress = func(ev progress.Event) { events = append(events, ev) }

	_, err := l.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"invoking challenger (turn 1)",
		"turn 1 (challenger): validation failed — retrying with feedback",
		"invoking challenger (turn 1, retry)",
		"turn 1 (challenger) complete: 1 new contentions, 0 resolved to consensus",
		"invoking incumbent (turn 2)",
		"turn 2 (incumbent) complete: 0 new contentions, 1 resolved to consensus",
	}
	if len(events) != len(want) {
		t.Fatalf("event count: want %d, got %d: %+v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].Message != w {
			t.Errorf("event %d: want %q, got %q", i, w, events[i].Message)
		}
	}
}

func TestHaltStateSaveFailureIsSurfaced(t *testing.T) {
	missingRationale := `agent: challenger
entries:
  - stance: new
    issue: "x"
`
	r := &scriptedRunner{payloads: []string{missingRationale, missingRationale}}
	l := newTestLoop(t, r)
	// Point StatePath at a directory that doesn't exist so the save-on-halt
	// write fails; the returned error must say so rather than silently
	// claiming "state preserved".
	l.StatePath = filepath.Join(l.ScratchDir, "no-such-dir", "debate-state.yaml")
	_, err := l.Run(context.Background())
	if !errors.Is(err, ErrHalted) {
		t.Fatalf("want ErrHalted, got %v", err)
	}
	if !strings.Contains(err.Error(), "state save also failed") {
		t.Errorf("error must surface the save failure, got: %v", err)
	}
}
