---
tags: [design, spec]
created: 2026-07-10
status: approved
---

# Design: Progress Streaming for `dialectic debate`

## Problem

`dialectic debate` currently produces zero output between invocation and completion — a real run takes anywhere from a few minutes (consensus reached quickly) to tens of minutes (multiple rounds, each turn a multi-minute LLM call). In interactive/TTY use this looks hung. It also blocks the planned Claude Code skill: a skill orchestrating a background run needs *some* signal to relay to the user besides "still waiting," and today there is none until the process exits.

## Why now

Confirmed via `https://murli.allankent.com/lang/go` and the pinned `murli-go` source (`v1.0.3-0.20260603054825-7a0868c70b51`, `writer.go:245-280`) that `murli.Writer` already has exactly the primitive this needs: `WriteProgress(ProgressEvent)`, which writes to **stderr in both TTY and agent mode** — a human-readable overwriting line in TTY (`\r\033[K`), minified NDJSON in agent mode — and never touches stdout. This means progress output can be added with zero risk to the existing final-result JSON envelope that `dialectic debate --agent | jq` (or a skill parsing stdout) depends on.

`murli.ProgressEvent`:
```go
type ProgressEvent struct {
    Stage   string  `json:"stage,omitempty"`
    Current int     `json:"current,omitempty"`
    Total   int     `json:"total,omitempty"`
    Percent float64 `json:"percent,omitempty"`
    EtaMs   int64   `json:"eta_ms,omitempty"`
    Message string  `json:"message,omitempty"`
}
```

Confirmed murli has no opinion on how business logic *inside* an application reports progress up to the command layer — its docs and source only address the `Writer` obtained in a cobra `RunE` via `murliCobra.NewWriter(cmd)`. `dialectic`'s existing architecture already keeps `murli` confined to `cmd/dialectic/*.go`; every `internal/` package (`state`, `turn`, `engine`, `agent`, `orchestrate`, `compile`, `runstore`) is framework-agnostic on purpose (the final whole-branch review from the original build called this a clean DAG). This design preserves that: the bridge from internal packages to `murli.Writer` is our own thin abstraction, confined to the one file that already imports murli.

## Non-goals

- No heartbeat/ticker during a blocked subprocess call (`claude`/`agy` give no intermediate signal — an "N seconds elapsed" ticker was considered and explicitly rejected as unneeded complexity for this iteration).
- No progress events for preflight checks (`os.Stat`, `exec.LookPath`) or for writing the summary/brief files — both are near-instant, not worth an event.
- No changes to the final JSON result envelope, `WriteSuccess`, or any existing output-format flag behavior.
- No changes to `internal/runstore`.

## Architecture

A new leaf package, `internal/progress`, holds the shared vocabulary — no imports beyond the standard library, so it introduces no new dependency edges into the existing DAG (it sits alongside `internal/state` as a leaf).

```go
// internal/progress/progress.go
package progress

// Event is a single progress update, emitted by internal/orchestrate and
// internal/compile as they run. It carries no murli dependency — the
// command layer (cmd/dialectic/debate.go) is responsible for translating
// Event into murli.ProgressEvent.
type Event struct {
	Stage     string // "turn" | "compile"
	Message   string
	Turn      int // 1-indexed; 0 if not applicable (compile stage)
	Round     int // current round_count at the time of the event
	MaxRounds int // st.MaxRounds; 0 if not applicable (compile stage)
}

// Func reports a single progress event. A nil Func is always safe to call
// through — callers use a package-level helper (report, below) rather than
// nil-checking at every call site.
type Func func(Event)
```

`internal/orchestrate.Loop` gains one new field:

```go
type Loop struct {
	State          *state.DebateState
	StatePath      string
	ArtifactPath   string
	ScratchDir     string
	TurnsDir       string
	Runner         agent.Runner
	MaxContentions int
	Progress       progress.Func // optional; nil is a no-op
}
```

`internal/compile.RunCompiler` gains one new parameter:

```go
func RunCompiler(ctx context.Context, r agent.Runner, binary string, st *state.DebateState,
	statePath, workDir, outPath string, report progress.Func) (string, error)
```

`cmd/dialectic/debate.go` wires both:

```go
reportProgress := func(ev progress.Event) {
	w.WriteProgress(murli.ProgressEvent{
		Stage:   ev.Stage,
		Current: ev.Turn,
		Total:   ev.MaxRounds * 2,
		Message: ev.Message,
	})
}
loop := &orchestrate.Loop{
	// ...existing fields...
	Progress: reportProgress,
}
// ...
doc, err := compile.RunCompiler(cmd.Context(), agent.NewExecRunner(), compiler, st,
	paths.StatePath, filepath.Dir(artifact), filepath.Join(paths.RunDir, "compiler-output.md"),
	reportProgress)
```

`Total = ev.MaxRounds * 2` is a worst-case ceiling (2 turns per round), not a promise — the debate can finish earlier via consensus, in which case the final `Current` value simply never reaches `Total`. This is acceptable: the TTY line only renders `(current/total)` when `Total > 0`, and a bound that's occasionally not reached is still informative context ("turn 3 of at most 6"), not a broken progress bar.

## Event Catalog

| Stage | Trigger | Message template |
|---|---|---|
| `turn` | before first invocation of a turn | `invoking <role> (turn <N>)` |
| `turn` | validation failed after first invocation | `turn <N> (<role>): validation failed — retrying with feedback` |
| `turn` | before the retry invocation | `invoking <role> (turn <N>, retry)` |
| `turn` | turn merged successfully | `turn <N> (<role>) complete: <X> new contentions, <Y> resolved to consensus` |
| `compile` | before first compiler invocation | `invoking compiler (<binary>)` |
| `compile` | citation validation failed after first invocation | `compiler output failed citation validation — retrying with feedback` |
| `compile` | valid document produced | `compiler complete — citations valid` |

A halted run (retry also fails) emits no additional progress event beyond the two above — the returned `ErrHalted` (already surfaced correctly since the `main.go`/stderr fixes earlier this session) carries the failure detail; duplicating that as a progress event would be redundant.

## Data Flow — `internal/orchestrate/loop.go`

In `takeTurn`, immediately before the emit points:

- **Before first `invokeAndValidate` call**: `l.report(progress.Event{Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds, Message: fmt.Sprintf("invoking %s (turn %d)", role, turnNum)})`.
- **After first `invokeAndValidate` returns `errs` non-empty** (before the retry call): emit the "validation failed — retrying" message, then emit the "invoking ... (turn N, retry)" message immediately before the second `invokeAndValidate` call.
- **After `engine.Merge` succeeds**: snapshot `len(l.State.ActiveContentions)` and `len(l.State.ConsensusBaseline)` *before* calling `Merge`, diff against the post-merge lengths, and emit the "complete" message with those deltas. (Consensus count can only increase or stay flat per merge; active-contention count can move either direction — the message reports both raw post-merge counts' deltas from pre-merge, not a signed change, to stay simple: "X new contentions" = new entries added to `ActiveContentions` this turn; "Y resolved to consensus" = new entries added to `ConsensusBaseline` this turn.)

A small private helper `func (l *Loop) report(ev progress.Event)` centralizes the nil-check (`if l.Progress != nil { l.Progress(ev) }`) so call sites never nil-check directly.

## Data Flow — `internal/compile/compiler.go`

In `RunCompiler`'s existing `for attempt := 0; attempt < 2; attempt++` loop:

- **Before each `Invoke` call**: emit `invoking compiler (<binary>)` on the first iteration, or `compiler output failed citation validation — retrying with feedback` immediately after a failed `ValidateCitations` check on the second iteration (this doubles as both "retry announcement" and satisfies the catalog's two `compile`-stage retry-adjacent messages without a third redundant "invoking, retry" line — the compiler has no role/turn number to vary the message, so one combined retry message is clearer than the turn-stage's two-message split).
- **On success** (`len(retryErrors) == 0`): emit `compiler complete — citations valid`.

Same nil-safe pattern: a private `report` helper, or the `progress.Func` called directly with a nil check inline (function is small enough that a helper isn't required — implementer's call, no behavior difference).

## Error Handling

- Progress events never affect control flow. `Progress`/`report` being nil is always valid and exercised by every existing test that doesn't set it (no test needs to change to keep passing).
- No progress event is ever the sole carrier of an error — `ErrHalted`, invocation errors, and citation-validation-exhausted errors all still return through the existing `error` return values, byte-for-byte unchanged by this feature.
- `murli.WriteProgress` itself cannot error (it swallows JSON marshal failures silently, per its own source at `writer.go:275-278`) — no new error path is introduced at the `debate.go` adapter closure either.

## Testing

- **`internal/progress`**: no logic beyond two type declarations — no test file needed (nothing to assert against; this mirrors how `internal/state`'s `Role`/`Stance` constants aren't separately tested beyond their use elsewhere).
- **`internal/orchestrate`**: extend `loop_test.go` with a test that sets `Loop.Progress` to a closure appending into a `[]progress.Event` slice, runs a scripted two-turn debate to consensus (reusing the existing `scriptedRunner` fixture), and asserts the captured events match the expected stage/message/turn sequence exactly (4 events: invoking turn 1, complete turn 1, invoking turn 2, complete turn 2). A second test forces one validation failure then a successful retry (reusing the existing `TestRetrySucceedsAndRunContinues` scenario) and asserts the 6-event sequence including the two retry-related events.
- **`internal/compile`**: extend `compiler_test.go` with a test asserting `RunCompiler` emits `invoking compiler` then `compiler complete` on the success path, and a second test asserting the retry-then-fail path emits `invoking compiler` then the retry message (no second "complete" event, since it still fails after retry).
- **`cmd/dialectic`**: extend `debate_test.go`'s existing `TestDebateEndToEndWithStubs` to assert the captured stderr buffer contains at least one line per stage keyword (`"turn"`, `"compile"`) — a light-touch integration check that events actually reach the real `murli.Writer` through the real wiring, not a full transcript match (which would be brittle against message wording changes).

## Scope check

This is a single, self-contained unit of work — one new leaf package, two call-site integrations, one adapter closure in the existing command file. No decomposition into sub-specs needed.
