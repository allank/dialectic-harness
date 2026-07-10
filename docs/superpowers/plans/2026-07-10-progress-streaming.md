# Progress Streaming Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add live progress reporting to `dialectic debate` so both interactive (TTY) and agent-mode runs show turn-by-turn and compile-stage progress instead of producing no output until completion.

**Architecture:** A new leaf package `internal/progress` defines a framework-agnostic `Event`/`Func` vocabulary. `internal/orchestrate.Loop` and `internal/compile.RunCompiler` accept an optional `progress.Func` and call it at defined points, with no dependency on `murli`. Only `cmd/dialectic/debate.go` (the sole existing `murli` import site) adapts `progress.Event` to `murli.ProgressEvent` and calls `murli.Writer.WriteProgress`, which writes to stderr in both TTY and agent mode — never stdout, so the final JSON result envelope is untouched.

**Tech Stack:** Go 1.26, `github.com/murli-cli/murli-go` (`Writer.WriteProgress`, confirmed at `writer.go:245-280` of the pinned version), existing `internal/orchestrate` and `internal/compile` packages.

## Global Constraints

- Full design spec: `docs/superpowers/specs/2026-07-10-progress-streaming-design.md` — read it for the "why" behind every decision below; this plan implements it verbatim.
- `progress.Func` is nil-safe: a nil `Progress`/`report` value must never panic and must be a complete no-op. Every existing caller of `Loop` or `RunCompiler` that doesn't set it must keep working unchanged.
- No progress event is ever the sole carrier of an error. Halts and failures continue to return through existing `error` values, byte-for-byte unchanged by this feature.
- Progress messages use these exact templates (verbatim, from the design spec's Event Catalog):
  - `invoking <role> (turn <N>)`
  - `turn <N> (<role>): validation failed — retrying with feedback`
  - `invoking <role> (turn <N>, retry)`
  - `turn <N> (<role>) complete: <X> new contentions, <Y> resolved to consensus`
  - `invoking compiler (<binary>)`
  - `compiler output failed citation validation — retrying with feedback`
  - `compiler complete — citations valid`
- `<X>`/`<Y>` in the turn-complete message are computed as `len(ActiveContentions)`/`len(ConsensusBaseline)` immediately after `engine.Merge` minus their values immediately before, floored at 0 if negative (an approximation — informational only, never used to drive control flow).
- No heartbeat/ticker during a blocked subprocess call. No progress events for preflight checks or for writing summary/brief files.
- `internal/progress` has no test file — it is two type declarations with nothing to assert against independently of their use in Tasks 1–2.

---

### Task 1: Progress events in the orchestration loop

**Files:**
- Create: `internal/progress/progress.go`
- Modify: `internal/orchestrate/loop.go`
- Test: `internal/orchestrate/loop_test.go`

**Interfaces:**
- Consumes: nothing new — `internal/orchestrate` already has `engine`, `state`, `turn`, `agent`.
- Produces: `progress.Event{Stage, Message string; Turn, Round, MaxRounds int}`, `progress.Func func(Event)` — Task 2 and Task 3 both import and use these exact types. `Loop` gains a `Progress progress.Func` field (optional; existing struct literals that omit it keep compiling and behaving exactly as before).

- [ ] **Step 1: Write the failing test**

Add to `internal/orchestrate/loop_test.go` (append after `TestRetrySucceedsAndRunContinues`, before `TestHaltStateSaveFailureIsSurfaced`):

```go
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
```

Add `"github.com/allank/dialectic/internal/progress"` to `loop_test.go`'s import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/orchestrate/ -run "TestRunReportsProgress" -v`
Expected: FAIL — compile error, `undefined: progress` (the package doesn't exist yet) and/or `l.Progress undefined (type *Loop has no field or method Progress)`.

- [ ] **Step 3: Create the progress package**

Create `internal/progress/progress.go`:

```go
// Package progress defines a framework-agnostic vocabulary for reporting
// debate progress. It has no dependency on murli or any CLI framework —
// only cmd/dialectic (the sole murli import site) translates Event into
// murli.ProgressEvent.
package progress

// Event is a single progress update, emitted by internal/orchestrate and
// internal/compile as they run.
type Event struct {
	Stage     string // "turn" | "compile"
	Message   string
	Turn      int // 1-indexed; 0 if not applicable (compile stage)
	Round     int // current round_count at the time of the event
	MaxRounds int // st.MaxRounds; 0 if not applicable (compile stage)
}

// Func reports a single progress event. A nil Func is always safe: callers
// must nil-check before invoking it (there is no default no-op wrapper).
type Func func(Event)
```

- [ ] **Step 4: Wire progress events into the loop**

In `internal/orchestrate/loop.go`, add the import and field:

```go
import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/engine"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/state"
	"github.com/allank/dialectic/internal/turn"
)

var ErrHalted = errors.New("run halted: invalid turn file after retry (state preserved)")

type Loop struct {
	State          *state.DebateState
	StatePath      string
	ArtifactPath   string
	ScratchDir     string // challenger clean-room cwd
	TurnsDir       string
	Runner         agent.Runner
	MaxContentions int
	Progress       progress.Func // optional; nil is a no-op
}

// report calls l.Progress if set. Every emit site in this file goes through
// report rather than nil-checking l.Progress directly.
func (l *Loop) report(ev progress.Event) {
	if l.Progress != nil {
		l.Progress(ev)
	}
}
```

Replace the body of `takeTurn` (everything from the `in := agent.PromptInput{...}` line onward) with:

```go
	in := agent.PromptInput{
		Role:           role,
		ArtifactPath:   artifactRef,
		StatePath:      statePath,
		TurnFilePath:   turnPath,
		MaxContentions: l.MaxContentions,
		Directives:     directivesFor(l.State, role),
	}

	l.report(progress.Event{
		Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
		Message: fmt.Sprintf("invoking %s (turn %d)", role, turnNum),
	})
	tf, sessionID, errs, err := l.invokeAndValidate(ctx, role, in)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		l.report(progress.Event{
			Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
			Message: fmt.Sprintf("turn %d (%s): validation failed — retrying with feedback", turnNum, role),
		})
		in.RetryErrors = errs
		l.report(progress.Event{
			Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
			Message: fmt.Sprintf("invoking %s (turn %d, retry)", role, turnNum),
		})
		tf, sessionID, errs, err = l.invokeAndValidate(ctx, role, in)
		if err != nil {
			return err
		}
		if len(errs) > 0 {
			if serr := l.State.Save(l.StatePath); serr != nil {
				return fmt.Errorf("%w: turn %d (%s): %v (state save also failed: %v)", ErrHalted, turnNum, role, errs, serr)
			}
			return fmt.Errorf("%w: turn %d (%s): %v", ErrHalted, turnNum, role, errs)
		}
	}

	if sessionID != "" {
		l.State.Sessions[role] = sessionID
	}
	preActive := len(l.State.ActiveContentions)
	preConsensus := len(l.State.ConsensusBaseline)
	if err := engine.Merge(l.State, tf); err != nil {
		return err
	}
	newContentions := len(l.State.ActiveContentions) - preActive
	if newContentions < 0 {
		newContentions = 0
	}
	newConsensus := len(l.State.ConsensusBaseline) - preConsensus
	l.report(progress.Event{
		Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
		Message: fmt.Sprintf("turn %d (%s) complete: %d new contentions, %d resolved to consensus", turnNum, role, newContentions, newConsensus),
	})
	return l.State.Save(l.StatePath)
```

(Everything above `in := agent.PromptInput{...}` in `takeTurn` — the `turnNum`, `turnPath`, `artifactRef`/`statePath`/clean-room setup — is unchanged. Only the body from `in := agent.PromptInput{...}` to the final `return l.State.Save(l.StatePath)` is replaced.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/orchestrate/ -v`
Expected: PASS — all tests including the two new ones and every pre-existing test in the package (`TestRunReachesConsensusInOneRound`, `TestRunHitsRoundLimitPreservingContentions`, `TestInvalidTurnRetriesOnceWithErrorsThenHalts`, `TestRetrySucceedsAndRunContinues`, `TestHaltStateSaveFailureIsSurfaced`).

- [ ] **Step 6: Run the full suite to confirm nothing else broke**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: clean build, all packages pass, no vet warnings.

- [ ] **Step 7: Commit**

```bash
git add internal/progress/ internal/orchestrate/loop.go internal/orchestrate/loop_test.go
git commit -m "feat: emit progress events from the orchestration loop"
```

---

### Task 2: Progress events in the Compiler stage

**Files:**
- Modify: `internal/compile/compiler.go`
- Test: `internal/compile/compiler_test.go`

**Interfaces:**
- Consumes: `progress.Event`, `progress.Func` from Task 1.
- Produces: `RunCompiler`'s signature gains a final parameter `report progress.Func` — Task 3 calls it with this new signature.

**Note:** this is a breaking signature change to `RunCompiler` (adds a required 8th positional parameter). Both existing call sites in `compiler_test.go` must be updated in Step 3, in the same commit — otherwise the package won't compile.

- [ ] **Step 1: Write the failing test**

Add to `internal/compile/compiler_test.go` (append at the end of the file):

```go
func TestRunCompilerReportsProgressOnSuccess(t *testing.T) {
	r := &cannedRunner{payloads: []string{validCompilerDoc}}
	out := filepath.Join(t.TempDir(), "brief-body.md")
	var events []progress.Event
	_, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "/runs/debate-state.yaml", t.TempDir(), out,
		func(ev progress.Event) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("RunCompiler: %v", err)
	}
	want := []string{"invoking compiler (claude)", "compiler complete — citations valid"}
	if len(events) != len(want) {
		t.Fatalf("event count: want %d, got %d: %+v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].Message != w {
			t.Errorf("event %d: want %q, got %q", i, w, events[i].Message)
		}
		if events[i].Stage != "compile" {
			t.Errorf("event %d: want stage \"compile\", got %q", i, events[i].Stage)
		}
	}
}

func TestRunCompilerReportsProgressOnRetryThenFail(t *testing.T) {
	bad := "## Narrative\n\nno citations here\n"
	r := &cannedRunner{payloads: []string{bad, bad}}
	out := filepath.Join(t.TempDir(), "brief-body.md")
	var events []progress.Event
	_, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "state.yaml", t.TempDir(), out,
		func(ev progress.Event) { events = append(events, ev) })
	if err == nil {
		t.Fatal("want error after failed retry")
	}
	want := []string{"invoking compiler (claude)", "compiler output failed citation validation — retrying with feedback"}
	if len(events) != len(want) {
		t.Fatalf("event count: want %d, got %d: %+v", len(want), len(events), events)
	}
	for i, w := range want {
		if events[i].Message != w {
			t.Errorf("event %d: want %q, got %q", i, w, events[i].Message)
		}
	}
}
```

Add `"github.com/allank/dialectic/internal/progress"` to `compiler_test.go`'s import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/compile/ -run "TestRunCompilerReportsProgress" -v`
Expected: FAIL — compile error, `undefined: progress` and/or `not enough arguments in call to RunCompiler`.

- [ ] **Step 3: Update RunCompiler and both existing call sites**

In `internal/compile/compiler.go`, add the import and change the function:

```go
import (
	"context"
	"fmt"
	"strings"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/state"
)
```

Replace the `RunCompiler` function body:

```go
// RunCompiler invokes the compiler binary sessionless, validates citation
// integrity deterministically, retries once with errors, then fails.
func RunCompiler(ctx context.Context, r agent.Runner, binary string, st *state.DebateState,
	statePath, workDir, outPath string, report progress.Func) (string, error) {
	var retryErrors []string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 0 {
			reportCompile(report, "invoking compiler ("+binary+")")
		} else {
			reportCompile(report, "compiler output failed citation validation — retrying with feedback")
		}
		res, err := r.Invoke(ctx, agent.Request{
			Binary:     binary,
			Prompt:     BuildCompilerPrompt(st, statePath, outPath, retryErrors),
			WorkDir:    workDir,
			SessionID:  "", // sessionless by design: no stake, no memory
			OutputPath: outPath,
		})
		if err != nil {
			return "", fmt.Errorf("compiler invocation: %w", err)
		}
		doc := string(res.Output)
		retryErrors = ValidateCitations(doc, st)
		if len(retryErrors) == 0 {
			reportCompile(report, "compiler complete — citations valid")
			return doc, nil
		}
	}
	return "", fmt.Errorf("compiler output failed citation validation after retry: %s", strings.Join(retryErrors, "; "))
}

func reportCompile(report progress.Func, message string) {
	if report != nil {
		report(progress.Event{Stage: "compile", Message: message})
	}
}
```

In `internal/compile/compiler_test.go`, update the two existing calls to pass `nil` as the new final argument:

In `TestRunCompilerAcceptsValidDoc`, change:
```go
	doc, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "/runs/debate-state.yaml", t.TempDir(), out)
```
to:
```go
	doc, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "/runs/debate-state.yaml", t.TempDir(), out, nil)
```

In `TestRunCompilerRetriesOnceThenFails`, change:
```go
	_, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "state.yaml", t.TempDir(), out)
```
to:
```go
	_, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "state.yaml", t.TempDir(), out, nil)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/compile/ -v`
Expected: PASS — all four tests in the package (`TestRunCompilerAcceptsValidDoc`, `TestRunCompilerRetriesOnceThenFails`, `TestRunCompilerReportsProgressOnSuccess`, `TestRunCompilerReportsProgressOnRetryThenFail`) plus every existing test in `citations_test.go` and `summary_test.go`.

- [ ] **Step 5: Run the full suite to confirm nothing else broke**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: clean build, all packages pass, no vet warnings. (`cmd/dialectic` will NOT yet compile-fail from this change since `debate.go`'s call to `RunCompiler` still has the old 7-arg signature — that's fixed in Task 3. If `go build ./...` fails here on `cmd/dialectic`, stop and re-check Task 2's Step 3 was applied exactly as shown before proceeding — Task 3 depends on Task 2 being complete first.)

- [ ] **Step 6: Commit**

```bash
git add internal/compile/compiler.go internal/compile/compiler_test.go
git commit -m "feat: emit progress events from the compiler stage"
```

---

### Task 3: Wire progress into the debate command

**Files:**
- Modify: `cmd/dialectic/debate.go`
- Test: `cmd/dialectic/debate_test.go`

**Interfaces:**
- Consumes: `progress.Event`, `progress.Func` (Task 1), `Loop.Progress` field (Task 1), `RunCompiler`'s 8-argument signature (Task 2), `murli.ProgressEvent`, `murli.Writer.WriteProgress` (confirmed at `writer.go:245-280` of the pinned murli-go version).
- Produces: nothing new for later tasks — this is the final integration point.

- [ ] **Step 1: Write the failing test**

In `cmd/dialectic/debate_test.go`, replace `TestDebateEndToEndWithStubs` in full with this version (splits stdout/stderr into separate buffers so progress lines on stderr can be asserted independently of the final JSON result on stdout):

```go
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
```

`TestDebateRejectsMissingArtifact` is unchanged — leave it exactly as it is in the file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/dialectic/ -run TestDebateEndToEndWithStubs -v`
Expected: FAIL — the stderr assertions fail (`stderr should contain turn-stage progress events, got:\n` with empty or non-JSON content), since `debate.go` doesn't emit any progress events yet.

- [ ] **Step 3: Wire the progress adapter into debate.go**

In `cmd/dialectic/debate.go`, add the import:

```go
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/murli-cli/murli-go"
	murliCobra "github.com/murli-cli/murli-go/cobra"
	"github.com/spf13/cobra"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/compile"
	"github.com/allank/dialectic/internal/orchestrate"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/runstore"
	"github.com/allank/dialectic/internal/state"
)
```

Immediately after the `w := murliCobra.NewWriter(cmd)` line, add the adapter closure:

```go
			w := murliCobra.NewWriter(cmd)
			reportProgress := func(ev progress.Event) {
				w.WriteProgress(murli.ProgressEvent{
					Stage:   ev.Stage,
					Current: ev.Turn,
					Total:   ev.MaxRounds * 2,
					Message: ev.Message,
				})
			}
```

In the `loop := &orchestrate.Loop{...}` struct literal, add the `Progress` field:

```go
			loop := &orchestrate.Loop{
				State:          st,
				StatePath:      paths.StatePath,
				ArtifactPath:   artifact,
				ScratchDir:     paths.ScratchDir,
				TurnsDir:       paths.TurnsDir,
				Runner:         agent.NewExecRunner(),
				MaxContentions: maxContentions,
				Progress:       reportProgress,
			}
```

Change the `compile.RunCompiler` call to pass `reportProgress` as the final argument:

```go
			doc, err := compile.RunCompiler(cmd.Context(), agent.NewExecRunner(), compiler, st,
				paths.StatePath, filepath.Dir(artifact), filepath.Join(paths.RunDir, "compiler-output.md"), reportProgress)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/dialectic/ -v`
Expected: PASS — `TestDebateEndToEndWithStubs`, `TestDebateRejectsMissingArtifact`, `TestRootCmdHasDebateAndRunsPlaceholders`, `TestRunsCommandPrintsAndWritesIndex`, `TestRunPrintsErrorToStderrAndReturnsNonZero`, `TestRunReturnsZeroOnSuccess` — every test in the package.

- [ ] **Step 5: Run the full suite, build, vet, and tidy**

Run: `go build ./... && go test ./... && go vet ./... && go mod tidy`
Expected: clean build, all packages pass, no vet warnings, no `go.mod`/`go.sum` diff from `go mod tidy` (no new dependencies were added anywhere in this plan).

- [ ] **Step 6: Manual smoke test with the stub scripts**

Run:
```bash
go build -o /tmp/dialectic-progress-check ./cmd/dialectic
dir=$(mktemp -d)
echo "# Test" > "$dir/artifact.md"
/tmp/dialectic-progress-check debate "$dir/artifact.md" \
  --challenger cmd/dialectic/testdata/stub-agent.sh \
  --incumbent cmd/dialectic/testdata/stub-agent.sh \
  --compiler cmd/dialectic/testdata/stub-compiler.sh \
  --max-rounds 1
```
Expected: in TTY mode (a real terminal, not piped), you should see a human-readable overwriting progress line (e.g. `[turn] invoking challenger (turn 1)`) update in place as the run progresses, followed by the final human-readable success message on stdout. If run piped (e.g. `| cat`, forcing non-TTY), stderr shows one JSON line per progress event and stdout shows only the single final JSON result envelope — confirm with `2>/dev/null | tail -1 | python3 -m json.tool` that stdout still parses as exactly one JSON object.

- [ ] **Step 7: Commit**

```bash
git add cmd/dialectic/debate.go cmd/dialectic/debate_test.go
git commit -m "feat: stream progress events to stderr via murli.Writer.WriteProgress"
```

---

## Verification at the end

After Task 3, progress streaming is complete. Confirm end-to-end with a real (non-stub) run if convenient:

```bash
go build -o dialectic ./cmd/dialectic
./dialectic debate <some-real-artifact.md> --challenger agy --incumbent claude
```

Watch for the overwriting progress line updating between turns instead of the previous total silence. Check that the final summary/brief output and exit behavior are unchanged from before this plan.
