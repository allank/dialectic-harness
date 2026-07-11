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
dialectic [command] [flags]
```

Every command supports `--agent` (force machine-readable JSON regardless of TTY), `--output json|ndjson|text` (explicit format override), `--schema` (print the command's machine-readable flag/subcommand schema instead of running it), and `--protocol-version` (envelope version negotiation, currently `0.2`) — inherited from the [murli](https://murli.allankent.com/lang/go) CLI framework this binary is built on. In a real terminal, output renders as human-readable text; piped or redirected, it auto-detects and switches to JSON.

### `debate` — run the harness

```
dialectic debate <artifact> [flags]
```

Runs the bounded challenger-vs-incumbent debate over a Markdown artifact.

| Flag | Default | Description |
|---|---|---|
| `--challenger` | `agy` | binary for the clean-room challenger role |
| `--incumbent` | `claude` | binary for the vault-context incumbent role |
| `--compiler` | `claude` | binary for the sessionless compiler role |
| `--max-rounds` | `3` | circuit breaker: maximum debate rounds |
| `--max-contentions` | `5` | cap on opening critique contentions |
| `--override-prompt` | — | override a built-in prompt: `--override-prompt <name>=<path>` (`opening_critique`\|`turn`\|`schema`\|`compiler`), repeatable |

Progress streams to stderr as the debate runs: one line per turn, appended (not overwritten), in a real terminal; one JSON event per turn in agent/non-TTY mode.

### `prompts` — introspect the built-in prompts

```
dialectic prompts
```

Prints the ASCII debate-flow diagram and all four built-in prompt templates (`opening_critique`, `turn`, `schema`, `compiler`) raw, with placeholders visible — the binary documents its own behavior. Read-only, no artifact required. Use this to see the exact template text before overriding one with `debate --override-prompt`.

#### Overriding prompts

Extract a template's exact default text with `dialectic prompts`, edit a copy, then point `debate` at it with `--override-prompt <name>=<path>`. Unlisted elements keep using their built-in default — an override replaces one template, not the whole prompt.

Pull the `turn` template out to a file and tweak it (`jq` reads the JSON envelope's `result.templates.<name>` field):

```
dialectic prompts --output json | jq -r '.result.templates.turn' > my-turn.tmpl
# edit my-turn.tmpl, e.g. add a house rule about citing line numbers
dialectic debate <artifact> --override-prompt turn=my-turn.tmpl
```

`--override-prompt` is repeatable, so multiple elements can be overridden in the same run:

```
dialectic debate <artifact> \
  --override-prompt turn=my-turn.tmpl \
  --override-prompt schema=my-schema.tmpl
```

The four valid names are `opening_critique`, `turn`, `schema`, and `compiler` — `debate` rejects any other name before invoking an agent. `schema` is the YAML contract the turn parser depends on (`internal/turn/turn.go`'s strict decoder rejects unknown fields); an override that drops a required field or renames a key will make every subsequent turn fail validation, not fail loudly up front — test a schema override against a short, disposable run first.

### `runs` — regenerate the kill-criterion index

```
dialectic runs [dir] [--write]
```

Scans a directory tree for update briefs (`arbiter_verdict` frontmatter) and regenerates the kill-criterion index table. Pass `--write` to also save it as `a2a-runs.md` in that directory; without it, the table only prints to stdout.

### `describe` — self-describing binary

```
dialectic describe [--agents-md]
```

Prints the full command tree, flags, and capabilities as a single JSON document — lets an orchestrating agent discover what this binary can do without parsing `--help` text. Pass `--agents-md` to generate an `AGENTS.md` stub instead.

### `completion`

Standard shell-completion script generation (bash/zsh/fish/powershell) — see `dialectic completion --help`.

## Trying it out

`examples/office-footprint-review.md` is a short, realistic-sounding office-lease decision memo — not derived from any real project — built to exercise the debate loop end to end. It plants two solid, well-supported claims, one unsubstantiated claim asserted as settled fact, and one hallucinated claim (a specific, invented statistic attached to a real researcher's name — the most common shape hallucinated citations take in practice).

```
dialectic debate examples/office-footprint-review.md
```

Compare the resulting compiled summary and update brief against the answer key below to judge how well the debate caught each planted issue:

| Paragraph | Claim | Type |
|---|---|---|
| 1 | US office vacancy surpassed the 2008 peak in 2023 | Solid, substantiated — widely reported (CBRE, Moody's, others) |
| 2 | Badge-swipe attendance plateaued at ~50% of pre-pandemic levels, midweek-peaked | Solid, substantiated — the widely reported Kastle "Back to Work Barometer" pattern |
| 3 | "Most companies that mandate strict RTO see a measurable satisfaction drop, followed by attrition" | Unsubstantiated — plausible, commonly repeated in commentary, asserted here with no source |
| 4 | "A 2022 Stanford study led by Nicholas Bloom found fully remote employees are 23% more productive" | Hallucinated — Bloom is a real, prominent WFH economist; no such study or figure exists |

Keep this key separate from the artifact when you run it — the incumbent role reads the artifact's own directory for context, so a sibling file could leak the key into its prompt.

## On-disk layout

Each debate run creates a hidden state directory beside the artifact at `.a2a/<slug>-<timestamp>/` (turn files, debate state, challenger scratch dir) — machine state the user never needs to touch. Two human-readable Markdown files are written beside the artifact: a compiled summary and an update brief carrying `arbiter_verdict: pending` frontmatter, which you flip after acting on it.

## Caveat: both roles on agy

If `--challenger` and `--incumbent` both resolve to `agy`, the tool prints a warning and proceeds anyway: agy's `-c` resumes the most recent conversation rather than a specific session ID, so the challenger and incumbent sessions can cross-contaminate. This is an accepted risk, not a blocked configuration.

## Read-only guarantee

The debate never modifies the target artifact. All writes go to the hidden `.a2a/` state directory or to the summary/brief files generated beside it.
