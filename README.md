# dialectic

A two-agent clean-room debate harness for detecting context-anchoring bias in AI-assisted document review: a challenger agent critiques an artifact with no visibility into the vault it lives in, while an incumbent agent reviews it with full context, and the two debate to consensus or a round limit.

## Build

```
go build -o dialectic ./cmd/dialectic
```

## Prerequisites

`dialectic` invokes `claude` and `agy` as subprocesses to run each debate turn. Both must be installed, authenticated, and resolvable on `PATH` before running `dialectic debate` (or pass `--challenger`/`--incumbent`/`--compiler` with explicit paths to the binaries).

## Usage

```
dialectic debate <artifact> [--challenger claude] [--incumbent agy] [--compiler claude] [--max-rounds 3] [--max-contentions 5]
dialectic runs [dir] [--write]
```

`debate` runs the bounded challenger-vs-incumbent debate over a Markdown artifact. `runs` scans a directory tree for update briefs and regenerates the kill-criterion index table (pass `--write` to also save it as `a2a-runs.md` in that directory).

## On-disk layout

Each debate run creates a hidden state directory beside the artifact at `.a2a/<slug>-<timestamp>/` (turn files, debate state, challenger scratch dir) — machine state the user never needs to touch. Two human-readable Markdown files are written beside the artifact: a compiled summary and an update brief carrying `arbiter_verdict: pending` frontmatter, which you flip after acting on it.

## Caveat: both roles on agy

If `--challenger` and `--incumbent` both resolve to `agy`, the tool prints a warning and proceeds anyway: agy's `-c` resumes the most recent conversation rather than a specific session ID, so the challenger and incumbent sessions can cross-contaminate. This is an accepted risk, not a blocked configuration.

## Read-only guarantee

The debate never modifies the target artifact. All writes go to the hidden `.a2a/` state directory or to the summary/brief files generated beside it.
