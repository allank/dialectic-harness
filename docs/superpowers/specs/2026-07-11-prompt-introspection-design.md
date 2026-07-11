---
tags: [design, spec]
created: 2026-07-11
status: approved
---

# Design: Prompt Introspection and Override

## Problem

`dialectic`'s four internal prompts (the challenger's opening critique, the shared turn-framing text, the turn-file schema block, and the Compiler's prompt) are opaque — hardcoded `fmt.Sprintf`/`strings.Builder` calls in `internal/agent/prompt.go` and `internal/compile/compiler.go`. There is no way to see what the harness actually tells each agent, and no way to customize it. This blocks both understanding ("what is this tool actually doing under the hood?") and experimentation (a user or an agent invoking `dialectic` wanting to try a different framing without forking the binary).

## Goals

1. A `dialectic prompts` subcommand that prints an ASCII flow diagram of the debate's stages plus all four built-in prompt templates, verbatim — the binary becomes self-describing.
2. Per-element `--override-prompt <name>=<path>` flags on `dialectic debate`, so any of the four elements can be swapped independently, with everything not overridden falling back to its built-in default.

## Non-goals

- No architecture change. The four elements map exactly onto what the code already generates (see Template Catalog below) — this is a templating-mechanism swap, not a redesign of when/how each piece fires.
- Directives and the retry-errors block stay non-template, non-overridable — they're dynamically injected facts from live debate state (which contentions, which validation errors), not authored prompt text.
- No content/semantic validation of override files (e.g. checking an override still contains schema instructions). If an override breaks turn validation, that surfaces through the existing retry-once-then-halt path exactly as a misbehaving agent would today.
- No change to `internal/orchestrate`, `internal/runstore`, or the `runs` subcommand.

## Architecture

Each template stays in the package that already owns its logic:
- `opening_critique`, `turn`, `schema` — `internal/agent`, alongside `BuildPrompt`.
- `compiler` — `internal/compile`, alongside `BuildCompilerPrompt`.

Each package embeds its own template files via `go:embed` and exposes `DefaultTemplates() map[string]string`. `cmd/dialectic`'s new `prompts` subcommand merges both maps for display. `BuildPrompt` and `BuildCompilerPrompt` both gain an `overrides map[string]string` parameter (name → raw template text; empty map when no `--override-prompt` flags are passed) and render every element — default or overridden — through Go's `text/template` instead of `fmt.Sprintf`.

## Template Catalog

| Name | Package | Fires when | Template variables |
|---|---|---|---|
| `opening_critique` | `internal/agent` | challenger, turn 1 only | `.ArtifactPath`, `.MaxContentions` |
| `turn` | `internal/agent` | every other turn, both roles | `.Role`, `.ArtifactPath`, `.StatePath` |
| `schema` | `internal/agent` | appended after either framing above, every turn | `.Role` |
| `compiler` | `internal/compile` | the Compiler stage | `.StatePath`, `.TargetArtifact`, `.OutPath` |

`.Role` is the raw role value (`"challenger"` / `"incumbent"`, matching `state.Role`'s string form — same value used in the schema block's `agent: {{.Role}}` line today). Where the turn-framing text needs it uppercased for display, the template uses a registered `upper` function: `{{.Role | upper}}`. This keeps `.Role` itself uncased and reusable across both templates that need it, rather than carrying two differently-cased fields.

**Byte-identical defaults, verified by golden fixture, not hand-derived.** Each embedded default template carries today's exact prompt text with `%s`/`%d` substitutions replaced 1:1 by `{{.Field}}` placeholders. The "Turn file path: {{.TurnFilePath}}\n\n" line that precedes the schema block in `BuildPrompt` today stays a separate, always-appended, non-template line (like directives/retry) rather than being folded into the `schema` template — this keeps `schema`'s variable set to just `.Role` (matching the catalog table) and avoids any wording change.

The current code's exact whitespace between concatenated pieces (framing, optional directives, the turn-file-path line, schema, optional retry block) has subtle differences — e.g. the directives block ends with a blank line, the retry block does not — that are easy to get wrong by hand-deriving a join algorithm in prose. The implementation plan must NOT hand-derive this: before refactoring, capture `BuildPrompt`'s and `BuildCompilerPrompt`'s actual current output for a representative set of inputs (with/without directives, with/without retry errors, challenger/incumbent, opening critique/regular turn) as golden fixtures — the same pattern already used in this codebase at `internal/compile/testdata/summary.golden.md` — then assert the templated version's default-path output matches those fixtures byte-for-byte. This is the source of truth for correct assembly, not this document.

**Default template file contents** (exact text for each `.tmpl` file, blank lines are literal):

`internal/agent/templates/opening_critique.tmpl`:
```
You are the CHALLENGER in a structured debate about a document. You have no prior context on it — that is deliberate. Read the artifact at {{.ArtifactPath}} with fresh eyes.

Produce an Opening Critique: raise at most {{.MaxContentions}} contentions — the strongest problems with the artifact, ranked by severity. Every entry uses stance: new.
```

`internal/agent/templates/turn.tmpl`:
```
You are the {{.Role | upper}} in a structured debate about the artifact at {{.ArtifactPath}}.

Read the current debate state at {{.StatePath}}. It lists active_contentions (with each side's stance), consensus_baseline (settled — do not reopen), and the full contention_history ledger.

For each active contention, take a stance: concur | rebut | withdraw | new — with a mandatory rationale. Concur only if you genuinely accept the other side's position. You may raise new contentions if your analysis exposes a fresh problem, but stay focused on the existing dispute.
```

`internal/agent/templates/schema.tmpl`:
```
Write your turn as a YAML file at the exact path given above, with this schema and nothing else:

agent: {{.Role}}
entries:
  - contention: C1        # id of an active contention; OMIT for stance new
    stance: rebut         # one of: concur | rebut | withdraw | new
    rationale: "why"      # mandatory on every entry; bare concessions are invalid
    position: "your current position in one sentence (optional, for concur/rebut/withdraw)"
    issue: "one-line issue statement"   # required only for stance: new
    severity: high        # optional, for stance: new (high|medium|low)
directives:               # optional: demand the other agent address a point next turn
  - contention: C1
    directive: "what they must address"

Rules:
- Every entry must cite an active contention id, except stance new (the orchestrator assigns ids to new contentions).
- Do not edit the artifact or any other file. Your only output is the turn file.
- Do not re-litigate resolved contentions.
```

`internal/compile/templates/compiler.tmpl`:
```
You are the COMPILER for a finished two-agent debate. You did not participate and have no stake in the dispute. Read the full debate ledger at {{.StatePath}} and the target artifact at {{.TargetArtifact}}.

Write a Markdown document to {{.OutPath}} with exactly these three sections:

## Narrative
A prose account of how the debate evolved: what was contested, what moved, what stuck.

## Proposed Changes
Bullet list. Each item proposes a concrete edit to the artifact, derived ONLY from consensus_baseline items.

## Judgment Calls
Bullet list. Each item poses a question the author must decide, with context, derived ONLY from unresolved active_contentions.

Citation rules (mandatory): every bullet and every narrative claim cites its source as (C<id>, turn <n>), e.g. (C2, turn 4). Cite only contention ids that exist in the ledger. Use plain CommonMark only — no wikilinks or Obsidian syntax. Do not edit the artifact or any other file.
```

## CLI Surface

**`dialectic prompts`** — new subcommand. No artifact argument, no debate execution. Follows the same murli dual-audience pattern as `debate`/`runs`: in TTY mode, human text — the diagram followed by each of the 4 templates verbatim under a `=== name ===` header, in catalog order (`opening_critique`, `turn`, `schema`, `compiler`); in agent mode (`--agent` or non-TTY), `murli.Writer.WriteSuccess` with a structured payload `{"diagram": "...", "templates": {"opening_critique": "...", "turn": "...", "schema": "...", "compiler": "..."}}`. This is explicit because an agent invoking `dialectic` (the stated use case for the override feature too) needs machine-readable access to the same catalog a human sees, not just a human-formatted dump.

ASCII flow diagram (plain ASCII only, no Unicode box-drawing — portable across any terminal):

```
[Opening Critique]  challenger, turn 1, clean room
  uses: opening_critique + schema
        |
        v
[Turn Loop]  challenger <-> incumbent, alternating
  uses: turn + schema, every turn
        |
        +-- invalid turn file --> retry once (same prompt + errors)
        |                              |
        |                    still invalid --> HALT (state preserved)
        v
  consensus reached OR round limit hit
        |
        v
[Compiler]  sessionless, reads full ledger
  uses: compiler
        |
        v
  compiled summary + update brief
```

**`dialectic debate <artifact> --override-prompt <name>=<path>`** — repeatable flag on the existing `debate` command. `<name>` must be one of `opening_critique`, `turn`, `schema`, `compiler`; an unrecognized name is a `murli.NewUserError` (same classification pattern as `debate.go`'s existing bad-input checks — e.g. artifact-not-found, binary-not-found). `<path>` is read from disk and used as that element's template text for the entire run, in place of the embedded default. Elements not named by any `--override-prompt` flag keep their default. Multiple `--override-prompt` flags for the same name: last one wins (standard flag-repetition behavior, no special handling needed).

## Data Flow

`BuildPrompt(in PromptInput, overrides map[string]string) string` and `BuildCompilerPrompt(st *state.DebateState, statePath, outPath string, retryErrors []string, overrides map[string]string) string` each resolve, per element they need: `text, ok := overrides[name]; if !ok { text = defaultTemplates[name] }`, then `template.New(name).Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(text)` and `.Execute(&buf, data)` where `data` is a small anonymous or named struct carrying exactly the variables listed in the Template Catalog for that element. `cmd/dialectic/debate.go` reads any `--override-prompt` flags into a single `map[string]string` (validating names against the catalog at parse time) and passes it through to both `BuildPrompt` (via a small threading change in the orchestrator's prompt-construction path) and `BuildCompilerPrompt`.

## Error Handling

- Unknown `--override-prompt` name → `murli.NewUserError`, caught at flag-processing time, before any run starts.
- Override file doesn't exist / can't be read → `murli.NewUserError`, same timing.
- Override file fails to parse as a Go template (e.g. unclosed `{{`) → surfaces as an error before any LLM call is spent (template parsing is synchronous and local).
- Override renders fine but produces content that breaks downstream turn validation or citation checking → no special handling; the existing retry-once-then-halt (`internal/orchestrate`) or citation-retry-then-fail (`internal/compile`) paths catch it exactly as they would a misbehaving agent, with the existing error messages.

## Testing

- `internal/agent`: capture `BuildPrompt`'s current (pre-refactor) output as golden fixtures for a representative input matrix (opening critique / regular turn, challenger / incumbent, with / without directives, with / without retry errors), per the golden-fixture strategy above. After refactoring, assert the templated version's default-path (no overrides) output matches each fixture byte-for-byte — the regression guard for the templating swap. Separately, assert that supplying an override in the `overrides` map produces that override's rendered text (with its own placeholders substituted) instead of the default.
- `internal/compile`: same two-part guard for `compiler` — golden-fixture-verified byte-identical default, override takes precedence when supplied.
- `cmd/dialectic`: in TTY mode, `dialectic prompts` output contains all 4 `=== name ===` headers and the diagram's key stage labels (`Opening Critique`, `Turn Loop`, `Compiler`); in agent mode, the JSON payload's `templates` object has exactly the 4 catalog keys and a non-empty `diagram` string. An end-to-end test (extending the existing stub-script harness) confirms `--override-prompt turn=<file>` actually changes the prompt text a stub agent receives, and that an unknown `--override-prompt` name produces a `murli.NewUserError` before any stub binary is invoked.

## Scope check

This is one cohesive feature — introspection and override share the same template catalog and the same underlying templating-mechanism swap — but it touches three packages (`internal/agent`, `internal/compile`, `cmd/dialectic`) and both a new subcommand and new flags on an existing command. The implementation plan should sequence it as: (1) template externalization in `internal/agent` with the byte-identical-default regression tests, (2) same for `internal/compile`, (3) the `prompts` subcommand, (4) `--override-prompt` wiring into `debate`. No further decomposition into separate specs needed — all four steps are small and share one design.
