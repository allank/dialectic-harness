# Prompt Introspection and Override Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `dialectic`'s four internal prompts (opening critique, turn, schema, compiler) describable via a new `dialectic prompts` subcommand and independently overridable via `--override-prompt <name>=<path>` on `dialectic debate`, with zero change to default behavior.

**Architecture:** Each prompt becomes an embedded Go `text/template` file instead of a hardcoded `fmt.Sprintf`/`strings.Builder` call, living in the package that already owns its logic (`internal/agent` for opening_critique/turn/schema, `internal/compile` for compiler). `BuildPrompt` and `BuildCompilerPrompt`/`RunCompiler` gain an `overrides map[string]string` parameter that takes precedence over the embedded default per element. `cmd/dialectic` threads `--override-prompt` flags into both, and a new `prompts` subcommand merges both packages' template catalogs for display.

**Tech Stack:** Go 1.26, `embed` (stdlib), `text/template` (stdlib) — no new external dependencies.

## Global Constraints

- Full design spec: `docs/superpowers/specs/2026-07-11-prompt-introspection-design.md` — read it for the "why" behind every decision below; this plan implements it, with one deviation noted next.
- **Task ordering deviates from the spec's Scope Check section.** The spec suggested (3) prompts subcommand before (4) override wiring. This plan reverses that: override wiring must land first, because `internal/compile.RunCompiler`'s signature change (Task 2) breaks `cmd/dialectic/debate.go`'s existing call site — the package won't build again until that call site is fixed, which only happens as part of override wiring. Adding a new subcommand to a package that doesn't build yet is not a valid task boundary. This plan's task order is: (1) `internal/agent`, (2) `internal/compile`, (3) override wiring into `debate` (fixes the build, adds the flag), (4) `prompts` subcommand.
- Four catalog names, exactly: `opening_critique`, `turn`, `schema`, `compiler`. No others recognized anywhere in this feature.
- `opening_critique` variables: `.ArtifactPath`, `.MaxContentions`. `turn` variables: `.Role` (raw, e.g. `"challenger"`), `.ArtifactPath`, `.StatePath`, plus the registered template function `upper` for display casing (`{{.Role | upper}}`). `schema` variables: `.Role` (raw). `compiler` variables: `.StatePath`, `.TargetArtifact`, `.OutPath`.
- Directives and the retry-errors block are never templated and never overridable — they stay exactly as today, built fresh from live data and concatenated in Go code around whichever template rendered.
- No content/semantic validation of override files beyond what Go template parsing itself requires. A parse error (e.g. unclosed `{{`) surfaces before any LLM call. A render-clean-but-semantically-broken override (e.g. drops the schema instructions) surfaces via the existing retry-once-then-halt / citation-retry-then-fail paths, unchanged.
- Default rendering must be **byte-identical** to today's `BuildPrompt`/`BuildCompilerPrompt` output. This plan verifies that via runtime golden-fixture capture (matching this codebase's existing pattern at `internal/compile/testdata/summary.golden.md`), never by hand-transcribing expected output into test code.
- `dialectic prompts` follows the same murli dual-audience pattern as `debate`/`runs`: human text in TTY mode, `murli.Writer.WriteSuccess` with a structured `{"diagram": "...", "templates": {...}}` payload in agent mode.

---

### Task 1: Externalize `internal/agent`'s templates

**Files:**
- Create: `internal/agent/templates/opening_critique.tmpl`
- Create: `internal/agent/templates/turn.tmpl`
- Create: `internal/agent/templates/schema.tmpl`
- Create: `internal/agent/templates.go`
- Modify: `internal/agent/prompt.go`
- Modify: `internal/orchestrate/loop.go:152-159` (`invokeAndValidate`'s call to `agent.BuildPrompt`)
- Test: `internal/agent/prompt_test.go`
- Test: `internal/agent/templates_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `func BuildPrompt(in PromptInput, overrides map[string]string) (string, error)` (signature change — was `func BuildPrompt(in PromptInput) string`) and `func DefaultTemplates() map[string]string` (new) in package `agent`. Task 3 and Task 4 both call `DefaultTemplates()`; Task 3 threads a real `overrides` value into `BuildPrompt` (this task passes `nil` at the one call site it fixes).

- [ ] **Step 1: Write a test that captures today's exact `BuildPrompt` output as golden fixtures**

Create `internal/agent/testdata/` directory (via the test itself, using `os.MkdirAll`) is not needed — commit fixtures directly under `internal/agent/testdata/`.

Create `internal/agent/templates_test.go`:

```go
package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

// goldenCases is the shared input matrix for the golden-fixture capture
// (this test) and the default-rendering regression test (added in Step 6,
// after the refactor). Keeping the cases in one place means both tests stay
// in sync by construction.
var goldenCases = []struct {
	name string
	in   PromptInput
}{
	{
		name: "opening_critique_no_directives",
		in: PromptInput{
			Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md",
			TurnFilePath: "/run/turns/turn-1-challenger.yaml", MaxContentions: 5,
		},
	},
	{
		name: "regular_turn_incumbent_with_directives",
		in: PromptInput{
			Role: state.RoleIncumbent, ArtifactPath: "/vault/doc.md", StatePath: "/run/debate-state.yaml",
			TurnFilePath: "/run/turns/turn-2-incumbent.yaml", MaxContentions: 5,
			Directives: []state.Directive{{Target: state.RoleIncumbent, Contention: "C1", Directive: "Address the latency concern.", IssuedTurn: 1}},
		},
	},
	{
		name: "regular_turn_challenger_with_retry",
		in: PromptInput{
			Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md", StatePath: "/run/debate-state.yaml",
			TurnFilePath: "/run/turns/turn-3-challenger.yaml", MaxContentions: 5,
			RetryErrors: []string{"entries[0]: rationale is mandatory; bare concessions are invalid"},
		},
	},
}

// TestUpdateGolden captures BuildPrompt's output for goldenCases into
// internal/agent/testdata/<name>.golden.txt. Run once, with UPDATE_GOLDEN=1,
// BEFORE refactoring BuildPrompt in Step 4 — it must run against today's
// fmt.Sprintf-based implementation so the fixtures are proof of current
// behavior, not a copy of the new implementation's output.
func TestUpdateGolden(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") == "" {
		t.Skip("set UPDATE_GOLDEN=1 to regenerate")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range goldenCases {
		got := BuildPrompt(tc.in)
		path := filepath.Join("testdata", tc.name+".golden.txt")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}
}
```

Note: `BuildPrompt(tc.in)` above is a single-return-value call — this is deliberate, matching **today's** signature before Step 4's refactor. Step 6 rewrites this same file's regression test to call the new two-return-value signature.

- [ ] **Step 2: Generate the golden fixtures from today's actual code**

Run:
```bash
cd /Users/allank/Dev/dialectic-harness
UPDATE_GOLDEN=1 go test ./internal/agent/ -run TestUpdateGolden -v
```
Expected: `PASS`, and three new files exist: `internal/agent/testdata/opening_critique_no_directives.golden.txt`, `internal/agent/testdata/regular_turn_incumbent_with_directives.golden.txt`, `internal/agent/testdata/regular_turn_challenger_with_retry.golden.txt`.

Read each generated file to sanity-check it looks like a real prompt (starts with "You are the...", contains the YAML schema block, the retry-case file ends with the validation-error line) — not empty or garbled.

- [ ] **Step 3: Commit the golden fixtures as a baseline, before any refactor**

```bash
git add internal/agent/templates_test.go internal/agent/testdata/
git commit -m "test: capture BuildPrompt golden fixtures before templating refactor"
```

- [ ] **Step 4: Create the template files**

Create `internal/agent/templates/opening_critique.tmpl`:
```
You are the CHALLENGER in a structured debate about a document. You have no prior context on it — that is deliberate. Read the artifact at {{.ArtifactPath}} with fresh eyes.

Produce an Opening Critique: raise at most {{.MaxContentions}} contentions — the strongest problems with the artifact, ranked by severity. Every entry uses stance: new.
```

Create `internal/agent/templates/turn.tmpl`:
```
You are the {{.Role | upper}} in a structured debate about the artifact at {{.ArtifactPath}}.

Read the current debate state at {{.StatePath}}. It lists active_contentions (with each side's stance), consensus_baseline (settled — do not reopen), and the full contention_history ledger.

For each active contention, take a stance: concur | rebut | withdraw | new — with a mandatory rationale. Concur only if you genuinely accept the other side's position. You may raise new contentions if your analysis exposes a fresh problem, but stay focused on the existing dispute.
```

Create `internal/agent/templates/schema.tmpl`:
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

- [ ] **Step 5: Create the embed + default-templates accessor**

Create `internal/agent/templates.go`:

```go
package agent

import (
	"embed"
	"fmt"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

var templateNames = []string{"opening_critique", "turn", "schema"}

var defaultTemplates = mustLoadTemplates()

func mustLoadTemplates() map[string]string {
	out := make(map[string]string, len(templateNames))
	for _, name := range templateNames {
		data, err := templatesFS.ReadFile("templates/" + name + ".tmpl")
		if err != nil {
			panic(fmt.Sprintf("agent: missing embedded template %q: %v", name, err))
		}
		out[name] = string(data)
	}
	return out
}

// DefaultTemplates returns the built-in prompt templates owned by this
// package, keyed by name. Used for introspection (dialectic prompts) and
// for validating --override-prompt names at the CLI layer. Returns a copy;
// callers may not mutate the package's own defaults.
func DefaultTemplates() map[string]string {
	out := make(map[string]string, len(defaultTemplates))
	for k, v := range defaultTemplates {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 6: Refactor `BuildPrompt` to render via template, with overrides, and update its regression test**

Replace `internal/agent/prompt.go` in full:

```go
// Package agent handles everything on the far side of the orchestrator's
// process boundary: prompt construction, headless CLI invocation, and the
// clean-room working directory for the challenger.
package agent

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/allank/dialectic/internal/state"
)

type PromptInput struct {
	Role           state.Role
	ArtifactPath   string
	StatePath      string // empty on turn 1 (opening critique)
	TurnFilePath   string
	MaxContentions int
	Directives     []state.Directive // directives targeting this role
	RetryErrors    []string          // non-empty on the single validation retry
}

type templateData struct {
	Role           string
	ArtifactPath   string
	StatePath      string
	MaxContentions int
}

// BuildPrompt renders the prompt for one agent turn. overrides maps a
// template name (opening_critique | turn | schema) to raw template text
// that takes precedence over the embedded default for that element only;
// a nil or empty map (or a map missing a given name) uses the default.
func BuildPrompt(in PromptInput, overrides map[string]string) (string, error) {
	data := templateData{
		Role:           string(in.Role),
		ArtifactPath:   in.ArtifactPath,
		StatePath:      in.StatePath,
		MaxContentions: in.MaxContentions,
	}

	framingName := "turn"
	if in.Role == state.RoleChallenger && in.StatePath == "" {
		framingName = "opening_critique"
	}
	framing, err := renderNamed(framingName, overrides, data)
	if err != nil {
		return "", err
	}

	pieces := []string{strings.TrimSpace(framing)}

	if len(in.Directives) > 0 {
		var db strings.Builder
		db.WriteString("You MUST address these directives this turn (cite the contention id in an entry):\n")
		for _, d := range in.Directives {
			fmt.Fprintf(&db, "- %s: %s\n", d.Contention, d.Directive)
		}
		pieces = append(pieces, strings.TrimSpace(db.String()))
	}

	pieces = append(pieces, fmt.Sprintf("Turn file path: %s", in.TurnFilePath))

	schema, err := renderNamed("schema", overrides, data)
	if err != nil {
		return "", err
	}
	pieces = append(pieces, strings.TrimSpace(schema))

	result := strings.Join(pieces, "\n\n")

	if len(in.RetryErrors) > 0 {
		var rb strings.Builder
		rb.WriteString("Your previous turn file was INVALID. Fix these errors and rewrite the complete turn file at the same path:\n")
		for _, e := range in.RetryErrors {
			fmt.Fprintf(&rb, "- %s\n", e)
		}
		result += "\n\n" + rb.String()
	}

	return result, nil
}

func renderNamed(name string, overrides map[string]string, data templateData) (string, error) {
	text, ok := overrides[name]
	if !ok {
		text = defaultTemplates[name]
	}
	tmpl, err := template.New(name).Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse %s template: %w", name, err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render %s template: %w", name, err)
	}
	return buf.String(), nil
}
```

Replace `internal/agent/prompt_test.go` in full:

```go
package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allank/dialectic/internal/state"
)

func TestBuildPromptDefaultMatchesGoldenFixtures(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildPrompt(tc.in, nil)
			if err != nil {
				t.Fatalf("BuildPrompt: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".golden.txt"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output diverges from golden fixture (captured from pre-refactor code)\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func TestBuildPromptOverrideTakesPrecedenceOverDefault(t *testing.T) {
	got, err := BuildPrompt(PromptInput{
		Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md",
		TurnFilePath: "/run/turns/turn-1-challenger.yaml", MaxContentions: 5,
	}, map[string]string{"opening_critique": "CUSTOM: look at {{.ArtifactPath}}."})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(got, "CUSTOM: look at /vault/doc.md.") {
		t.Errorf("override must be rendered with its own placeholders substituted, got:\n%s", got)
	}
	if strings.Contains(got, "You have no prior context") {
		t.Errorf("built-in default text must not appear when overridden, got:\n%s", got)
	}
}

func TestBuildPromptRejectsMalformedOverride(t *testing.T) {
	_, err := BuildPrompt(PromptInput{
		Role: state.RoleChallenger, ArtifactPath: "/vault/doc.md",
		TurnFilePath: "/run/turns/turn-1-challenger.yaml", MaxContentions: 5,
	}, map[string]string{"opening_critique": "unclosed {{ .ArtifactPath"})
	if err == nil {
		t.Fatal("want an error for a malformed override template, got nil")
	}
}

func TestDefaultTemplatesHasAllThreeNames(t *testing.T) {
	tmpls := DefaultTemplates()
	for _, name := range []string{"opening_critique", "turn", "schema"} {
		if _, ok := tmpls[name]; !ok {
			t.Errorf("DefaultTemplates() missing %q", name)
		}
	}
	if len(tmpls) != 3 {
		t.Errorf("DefaultTemplates(): want exactly 3 entries, got %d: %v", len(tmpls), tmpls)
	}
}
```

- [ ] **Step 7: Run the agent package's tests to verify they pass**

Run: `go test ./internal/agent/ -v`
Expected: FAIL at this point — `internal/orchestrate` isn't part of this test run, but `internal/agent` itself should already compile and pass, since Step 6 only touched files inside `internal/agent`. If it fails, re-check Step 6's `prompt.go`/`prompt_test.go` content was applied exactly as shown.

- [ ] **Step 8: Fix `internal/orchestrate/loop.go`'s call site**

`BuildPrompt`'s signature changed from `func BuildPrompt(in PromptInput) string` to `func BuildPrompt(in PromptInput, overrides map[string]string) (string, error)`. `internal/orchestrate/loop.go`'s `invokeAndValidate` method calls it. This step makes the minimal fix to keep `internal/orchestrate` compiling — it passes `nil` (no overrides yet); Task 3 replaces `nil` with a real field once `Loop` gains one.

In `internal/orchestrate/loop.go`, find `invokeAndValidate` (currently starting around line 152):

```go
func (l *Loop) invokeAndValidate(ctx context.Context, role state.Role, in agent.PromptInput) (turn.File, string, []string, error) {
	res, err := l.Runner.Invoke(ctx, agent.Request{
		Binary:     l.State.Roles[role],
		Prompt:     agent.BuildPrompt(in),
		WorkDir:    workDirFor(l, role),
		SessionID:  l.State.Sessions[role],
		OutputPath: in.TurnFilePath,
	})
```

Replace with:

```go
func (l *Loop) invokeAndValidate(ctx context.Context, role state.Role, in agent.PromptInput) (turn.File, string, []string, error) {
	prompt, err := agent.BuildPrompt(in, nil)
	if err != nil {
		return turn.File{}, "", nil, fmt.Errorf("build prompt: %w", err)
	}
	res, err := l.Runner.Invoke(ctx, agent.Request{
		Binary:     l.State.Roles[role],
		Prompt:     prompt,
		WorkDir:    workDirFor(l, role),
		SessionID:  l.State.Sessions[role],
		OutputPath: in.TurnFilePath,
	})
```

(The rest of `invokeAndValidate`, from the `if err != nil {` guard on the `l.Runner.Invoke` result onward, is unchanged — only the `Prompt:` line's source changed, plus the two new lines before it.)

- [ ] **Step 9: Run the full suite, build, and vet**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: clean build, every package's tests pass (including `internal/orchestrate`'s existing tests, unaffected in behavior — they never set overrides, and `nil` overrides renders identically to today), no vet warnings.

- [ ] **Step 10: Commit**

```bash
git add internal/agent/templates/ internal/agent/templates.go internal/agent/prompt.go internal/agent/prompt_test.go internal/orchestrate/loop.go
git commit -m "feat: externalize internal/agent prompts into overridable templates"
```

---

### Task 2: Externalize `internal/compile`'s compiler template

**Files:**
- Create: `internal/compile/templates/compiler.tmpl`
- Create: `internal/compile/templates.go`
- Modify: `internal/compile/compiler.go`
- Test: `internal/compile/compiler_test.go`
- Test: `internal/compile/templates_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `func BuildCompilerPrompt(st *state.DebateState, statePath, outPath string, retryErrors []string, overrides map[string]string) (string, error)` (signature change) and `func RunCompiler(ctx context.Context, r agent.Runner, binary string, st *state.DebateState, statePath, workDir, outPath string, report progress.Func, overrides map[string]string) (string, error)` (signature change — adds a 9th parameter) and `func DefaultTemplates() map[string]string` (new) in package `compile`. Task 3 threads a real `overrides` value into `RunCompiler`'s call in `cmd/dialectic/debate.go`; this task updates the two existing test call sites to pass `nil`. **`cmd/dialectic` will not build after this task** — its call to `compile.RunCompiler` still has the old 8-argument signature. This is expected; Task 3 fixes it.

- [ ] **Step 1: Write a test that captures today's exact `BuildCompilerPrompt` output as golden fixtures**

Create `internal/compile/templates_test.go`:

```go
package compile

import (
	"os"
	"path/filepath"
	"testing"
)

var compilerGoldenCases = []struct {
	name        string
	retryErrors []string
}{
	{name: "compiler_no_retry", retryErrors: nil},
	{name: "compiler_with_retry", retryErrors: []string{"missing required section: ## Judgment Calls"}},
}

// TestUpdateGolden captures BuildCompilerPrompt's output into
// internal/compile/testdata/<name>.golden.txt. Run once, with
// UPDATE_GOLDEN=1, BEFORE refactoring BuildCompilerPrompt in Step 4.
func TestUpdateGolden(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") == "" {
		t.Skip("set UPDATE_GOLDEN=1 to regenerate")
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	st := summaryFixture()
	for _, tc := range compilerGoldenCases {
		got := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", tc.retryErrors)
		path := filepath.Join("testdata", tc.name+".golden.txt")
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
	}
}
```

`summaryFixture()` is already defined in this package (used by `summary_test.go`) — do not redefine it.

- [ ] **Step 2: Generate the golden fixtures from today's actual code**

Run:
```bash
UPDATE_GOLDEN=1 go test ./internal/compile/ -run TestUpdateGolden -v
```
Expected: `PASS`, and two new files exist: `internal/compile/testdata/compiler_no_retry.golden.txt`, `internal/compile/testdata/compiler_with_retry.golden.txt`. Read each to confirm it looks like a real Compiler prompt (mentions Narrative/Proposed Changes/Judgment Calls; the retry case ends with the validation-error line).

- [ ] **Step 3: Commit the golden fixtures as a baseline, before any refactor**

```bash
git add internal/compile/templates_test.go internal/compile/testdata/
git commit -m "test: capture BuildCompilerPrompt golden fixtures before templating refactor"
```

- [ ] **Step 4: Create the template file**

Create `internal/compile/templates/compiler.tmpl`:
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

- [ ] **Step 5: Create the embed + default-templates accessor**

Create `internal/compile/templates.go`:

```go
package compile

import (
	"embed"
	"fmt"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

var defaultTemplates = mustLoadTemplates()

func mustLoadTemplates() map[string]string {
	data, err := templatesFS.ReadFile("templates/compiler.tmpl")
	if err != nil {
		panic(fmt.Sprintf("compile: missing embedded template %q: %v", "compiler", err))
	}
	return map[string]string{"compiler": string(data)}
}

// DefaultTemplates returns the built-in prompt templates owned by this
// package, keyed by name. Used for introspection (dialectic prompts) and
// for validating --override-prompt names at the CLI layer. Returns a copy;
// callers may not mutate the package's own defaults.
func DefaultTemplates() map[string]string {
	out := make(map[string]string, len(defaultTemplates))
	for k, v := range defaultTemplates {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 6: Refactor `BuildCompilerPrompt` and `RunCompiler`, and update all call sites**

In `internal/compile/compiler.go`, add the `text/template` import:

```go
import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/state"
)
```

Replace `BuildCompilerPrompt` and `RunCompiler` in full:

```go
// BuildCompilerPrompt builds the prompt for the sessionless Compiler role: a
// disinterested reader of the finished ledger who writes a narrative and
// proposed changes, every claim cited back to the ledger. overrides maps
// "compiler" to raw template text that takes precedence over the embedded
// default; a nil or empty map (or one missing "compiler") uses the default.
func BuildCompilerPrompt(st *state.DebateState, statePath, outPath string, retryErrors []string, overrides map[string]string) (string, error) {
	text, ok := overrides["compiler"]
	if !ok {
		text = defaultTemplates["compiler"]
	}
	tmpl, err := template.New("compiler").Parse(text)
	if err != nil {
		return "", fmt.Errorf("parse compiler template: %w", err)
	}
	var buf strings.Builder
	data := struct{ StatePath, TargetArtifact, OutPath string }{
		StatePath: statePath, TargetArtifact: st.TargetArtifact, OutPath: outPath,
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render compiler template: %w", err)
	}
	result := buf.String()
	if len(retryErrors) > 0 {
		var rb strings.Builder
		rb.WriteString("\nYour previous output FAILED citation validation. Fix these errors and rewrite the complete document at the same path:\n")
		for _, e := range retryErrors {
			rb.WriteString("- " + e + "\n")
		}
		result += rb.String()
	}
	return result, nil
}

// RunCompiler invokes the compiler binary sessionless, validates citation
// integrity deterministically, retries once with errors, then fails.
func RunCompiler(ctx context.Context, r agent.Runner, binary string, st *state.DebateState,
	statePath, workDir, outPath string, report progress.Func, overrides map[string]string) (string, error) {
	var retryErrors []string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt == 0 {
			reportCompile(report, "invoking compiler ("+binary+")")
		} else {
			reportCompile(report, "compiler output failed citation validation — retrying with feedback")
		}
		prompt, err := BuildCompilerPrompt(st, statePath, outPath, retryErrors, overrides)
		if err != nil {
			return "", fmt.Errorf("build compiler prompt: %w", err)
		}
		res, err := r.Invoke(ctx, agent.Request{
			Binary:     binary,
			Prompt:     prompt,
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

In `internal/compile/compiler_test.go`, update all four `RunCompiler` calls to add `nil` as the new final argument. Change:
```go
	doc, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "/runs/debate-state.yaml", t.TempDir(), out, nil)
```
to:
```go
	doc, err := RunCompiler(context.Background(), r, "claude", summaryFixture(), "/runs/debate-state.yaml", t.TempDir(), out, nil, nil)
```
(in `TestRunCompilerAcceptsValidDoc`), and similarly append `, nil` to the existing `RunCompiler(...)` calls in `TestRunCompilerRetriesOnceThenFails`, `TestRunCompilerReportsProgressOnSuccess` (which passes a closure, not literal `nil`, as the `report` argument — append `, nil` after that closure argument), and `TestRunCompilerReportsProgressOnRetryThenFail` (same pattern). All four calls need exactly one more trailing `nil` argument; none need any other change.

- [ ] **Step 7: Add golden-regression and override tests**

Append to `internal/compile/templates_test.go`:

```go
func TestBuildCompilerPromptDefaultMatchesGoldenFixtures(t *testing.T) {
	st := summaryFixture()
	for _, tc := range compilerGoldenCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", tc.retryErrors, nil)
			if err != nil {
				t.Fatalf("BuildCompilerPrompt: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".golden.txt"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if got != string(want) {
				t.Errorf("output diverges from golden fixture (captured from pre-refactor code)\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

func TestBuildCompilerPromptOverrideTakesPrecedenceOverDefault(t *testing.T) {
	st := summaryFixture()
	got, err := BuildCompilerPrompt(st, "/run/debate-state.yaml", "/run/compiler-output.md", nil,
		map[string]string{"compiler": "CUSTOM: read {{.StatePath}} and write to {{.OutPath}}."})
	if err != nil {
		t.Fatalf("BuildCompilerPrompt: %v", err)
	}
	if got != "CUSTOM: read /run/debate-state.yaml and write to /run/compiler-output.md." {
		t.Errorf("override must fully replace the default and render its own placeholders, got:\n%s", got)
	}
}

func TestDefaultTemplatesHasCompilerName(t *testing.T) {
	tmpls := DefaultTemplates()
	if _, ok := tmpls["compiler"]; !ok {
		t.Errorf("DefaultTemplates() missing %q", "compiler")
	}
	if len(tmpls) != 1 {
		t.Errorf("DefaultTemplates(): want exactly 1 entry, got %d: %v", len(tmpls), tmpls)
	}
}
```

- [ ] **Step 8: Run the compile package's tests to verify they pass**

Run: `go test ./internal/compile/ -v`
Expected: PASS, all tests in the package.

- [ ] **Step 9: Confirm the expected (temporary) breakage in `cmd/dialectic`**

Run: `go build ./internal/... && echo "INTERNAL BUILD OK"`
Expected: `INTERNAL BUILD OK` — every package under `internal/` builds cleanly.

Run: `go build ./cmd/... 2>&1`
Expected: a build failure in `cmd/dialectic/debate.go`, specifically a "not enough arguments in call to compile.RunCompiler" error. This is expected — Task 3 fixes it. Do not attempt to fix `cmd/dialectic` in this task.

- [ ] **Step 10: Run internal package tests and vet**

Run: `go test ./internal/... && go vet ./internal/...`
Expected: all internal packages pass, no vet warnings. (Skip `./cmd/...` — it doesn't build yet, by design.)

- [ ] **Step 11: Commit**

```bash
git add internal/compile/templates/ internal/compile/templates.go internal/compile/compiler.go internal/compile/compiler_test.go
git commit -m "feat: externalize internal/compile prompt into overridable template"
```

---

### Task 3: Wire `--override-prompt` into the `debate` command

**Files:**
- Modify: `cmd/dialectic/debate.go`
- Modify: `internal/orchestrate/loop.go` (add `PromptOverrides` field, fix the `nil` placeholder from Task 1)
- Test: `cmd/dialectic/debate_test.go`
- Create: `cmd/dialectic/testdata/stub-capture-challenger.sh`
- Create: `cmd/dialectic/testdata/stub-concur-incumbent.sh`

**Interfaces:**
- Consumes: `agent.BuildPrompt`'s overrides parameter, `compile.RunCompiler`'s overrides parameter (Tasks 1-2); `agent.DefaultTemplates()`, `compile.DefaultTemplates()` (Tasks 1-2, used here only for name validation — Task 4 uses them for display).
- Produces: nothing new for Task 4 beyond `cmd/dialectic` building again — Task 4 is additive (a new subcommand file) and does not depend on anything this task adds beyond a working build.

- [ ] **Step 1: Add the `PromptOverrides` field to `Loop` and use it**

In `internal/orchestrate/loop.go`, find the `Loop` struct (around line 21):

```go
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
```

Replace with:

```go
type Loop struct {
	State           *state.DebateState
	StatePath       string
	ArtifactPath    string
	ScratchDir      string // challenger clean-room cwd
	TurnsDir        string
	Runner          agent.Runner
	MaxContentions  int
	Progress        progress.Func     // optional; nil is a no-op
	PromptOverrides map[string]string // optional; nil uses agent's built-in defaults
}
```

Find the line added in Task 1, Step 8 (`invokeAndValidate`'s prompt build):

```go
	prompt, err := agent.BuildPrompt(in, nil)
```

Replace with:

```go
	prompt, err := agent.BuildPrompt(in, l.PromptOverrides)
```

- [ ] **Step 2: Run orchestrate's tests to verify nothing broke**

Run: `go test ./internal/orchestrate/ -v`
Expected: PASS — every existing test leaves `Loop.PromptOverrides` unset (nil), which behaves identically to the explicit `nil` it replaces.

- [ ] **Step 3: Write the failing tests for `--override-prompt`**

Create `cmd/dialectic/testdata/stub-capture-challenger.sh`:

```bash
#!/bin/sh
# Captures its full argv (the prompt is the last argument for both claude-
# and agy-style invocations) to $PROMPT_CAPTURE_FILE, then writes a single
# minimal valid turn file so the debate can proceed.
echo "$@" > "$PROMPT_CAPTURE_FILE"
cat > "$DIALECTIC_OUTPUT_FILE" <<'EOF'
agent: challenger
entries:
  - stance: new
    issue: "placeholder issue for override test"
    severity: low
    rationale: "placeholder rationale for override test"
EOF
echo '{"session_id":"stub-sess"}'
```

Create `cmd/dialectic/testdata/stub-concur-incumbent.sh`:

```bash
#!/bin/sh
# Always concurs with C1, so a debate started with --max-rounds 1 reaches
# consensus immediately after this turn.
cat > "$DIALECTIC_OUTPUT_FILE" <<'EOF'
agent: incumbent
entries:
  - contention: C1
    stance: concur
    rationale: "agreed, resolving to consensus so the run completes quickly"
EOF
echo '{"session_id":"stub-sess"}'
```

Make both executable:
```bash
chmod +x cmd/dialectic/testdata/stub-capture-challenger.sh cmd/dialectic/testdata/stub-concur-incumbent.sh
```

Append to `cmd/dialectic/debate_test.go`:

```go
func TestDebateOverridePromptChangesAgentInput(t *testing.T) {
	wd, _ := os.Getwd()
	captureStub := filepath.Join(wd, "testdata", "stub-capture-challenger.sh")
	incumbentStub := filepath.Join(wd, "testdata", "stub-concur-incumbent.sh")
	compilerStub := filepath.Join(wd, "testdata", "stub-compiler.sh")

	dir := t.TempDir()
	artifact := filepath.Join(dir, "Test PRD.md")
	if err := os.WriteFile(artifact, []byte("# Test PRD\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	captureFile := filepath.Join(t.TempDir(), "captured-prompt.txt")
	t.Setenv("PROMPT_CAPTURE_FILE", captureFile)

	overrideFile := filepath.Join(t.TempDir(), "override-opening-critique.txt")
	overrideText := "CUSTOM OVERRIDE MARKER: raise your concerns about this document."
	if err := os.WriteFile(overrideFile, []byte(overrideText), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs([]string{"debate", artifact,
		"--challenger", captureStub,
		"--incumbent", incumbentStub,
		"--compiler", compilerStub,
		"--max-rounds", "1",
		"--override-prompt", "opening_critique=" + overrideFile,
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("debate: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	captured, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("read captured prompt: %v", err)
	}
	if !strings.Contains(string(captured), "CUSTOM OVERRIDE MARKER") {
		t.Errorf("challenger's prompt must contain the override text, got:\n%s", captured)
	}
	if strings.Contains(string(captured), "You have no prior context") {
		t.Errorf("challenger's prompt must NOT contain the built-in default text when overridden, got:\n%s", captured)
	}
}

func TestDebateRejectsUnknownOverridePromptName(t *testing.T) {
	origExit := murli.ExitFunc
	var capturedExit int
	murli.ExitFunc = func(code int) { capturedExit = code }
	defer func() { murli.ExitFunc = origExit }()

	dir := t.TempDir()
	artifact := filepath.Join(dir, "Test.md")
	if err := os.WriteFile(artifact, []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	overrideFile := filepath.Join(t.TempDir(), "override.txt")
	if err := os.WriteFile(overrideFile, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	errBuf := &bytes.Buffer{}
	root.SetOut(&bytes.Buffer{})
	root.SetErr(errBuf)
	root.SetArgs([]string{"debate", artifact, "--override-prompt", "bogus_name=" + overrideFile})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if capturedExit != murli.ExitUserError {
		t.Errorf("exit code: want %d (ExitUserError), got %d\nstderr:\n%s", murli.ExitUserError, capturedExit, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "unknown prompt name") {
		t.Errorf("stderr should mention unknown prompt name, got:\n%s", errBuf.String())
	}
}
```

Note: `TestDebateRejectsUnknownOverridePromptName` deliberately omits `--challenger`/`--incumbent`/`--compiler`, leaving them at their `claude`/`agy`/`claude` defaults, and does not require those binaries to be installed — Step 4 places override-prompt validation *before* the binary-exists preflight check, so this test never reaches that check.

- [ ] **Step 4: Run the new tests to verify they fail**

Run: `go test ./cmd/dialectic/ -run "TestDebateOverridePromptChangesAgentInput|TestDebateRejectsUnknownOverridePromptName" -v`
Expected: FAIL — `flag needs an argument: --override-prompt` or `unknown flag: --override-prompt` (the flag doesn't exist yet), or (once the flag doesn't error) the capture-file assertions failing because no override wiring exists yet. Either failure mode confirms the feature is missing.

- [ ] **Step 5: Add the `--override-prompt` flag and wire it through**

In `cmd/dialectic/debate.go`, add `"strings"` to the import block:

```go
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

Add a new local variable alongside the existing flag variables (find `var challenger, incumbent, compiler string` and `var maxRounds, maxContentions int`):

```go
	var challenger, incumbent, compiler string
	var maxRounds, maxContentions int
	var overridePromptFlags []string
```

Immediately after the existing artifact checks and BEFORE the binary `exec.LookPath` loop, insert override-prompt parsing and validation. Find:

```go
			artifact, err := filepath.Abs(args[0])
			if err != nil {
				return murli.NewUserError("bad artifact path: "+err.Error(), "pass a path to a Markdown file")
			}
			if _, err := os.Stat(artifact); err != nil {
				return murli.NewUserError("artifact not found: "+artifact, "pass a path to an existing Markdown file")
			}
			for _, bin := range []string{challenger, incumbent, compiler} {
```

Replace with:

```go
			artifact, err := filepath.Abs(args[0])
			if err != nil {
				return murli.NewUserError("bad artifact path: "+err.Error(), "pass a path to a Markdown file")
			}
			if _, err := os.Stat(artifact); err != nil {
				return murli.NewUserError("artifact not found: "+artifact, "pass a path to an existing Markdown file")
			}

			overrides := map[string]string{}
			if len(overridePromptFlags) > 0 {
				valid := map[string]bool{}
				for name := range agent.DefaultTemplates() {
					valid[name] = true
				}
				for name := range compile.DefaultTemplates() {
					valid[name] = true
				}
				for _, spec := range overridePromptFlags {
					name, path, ok := strings.Cut(spec, "=")
					if !ok {
						return murli.NewUserError("invalid --override-prompt value: "+spec, "use the form <name>=<path>, e.g. turn=my-turn.tmpl")
					}
					if !valid[name] {
						return murli.NewUserError("unknown prompt name: "+name, "valid names: opening_critique, turn, schema, compiler")
					}
					content, err := os.ReadFile(path)
					if err != nil {
						return murli.NewUserError("cannot read override file for "+name+": "+err.Error(), "check the file path")
					}
					overrides[name] = string(content)
				}
			}

			for _, bin := range []string{challenger, incumbent, compiler} {
```

Add `PromptOverrides: overrides` to the `orchestrate.Loop{...}` literal. Find:

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

Replace with:

```go
			loop := &orchestrate.Loop{
				State:           st,
				StatePath:       paths.StatePath,
				ArtifactPath:    artifact,
				ScratchDir:      paths.ScratchDir,
				TurnsDir:        paths.TurnsDir,
				Runner:          agent.NewExecRunner(),
				MaxContentions:  maxContentions,
				Progress:        reportProgress,
				PromptOverrides: overrides,
			}
```

Add `overrides` as the final argument to the `compile.RunCompiler` call. Find:

```go
			doc, err := compile.RunCompiler(cmd.Context(), agent.NewExecRunner(), compiler, st,
				paths.StatePath, filepath.Dir(artifact), filepath.Join(paths.RunDir, "compiler-output.md"), reportProgress)
```

Replace with:

```go
			doc, err := compile.RunCompiler(cmd.Context(), agent.NewExecRunner(), compiler, st,
				paths.StatePath, filepath.Dir(artifact), filepath.Join(paths.RunDir, "compiler-output.md"), reportProgress, overrides)
```

Register the new flag alongside the existing ones. Find:

```go
	cmd.Flags().StringVar(&challenger, "challenger", "claude", "binary for the clean-room challenger role")
	cmd.Flags().StringVar(&incumbent, "incumbent", "agy", "binary for the vault-context incumbent role")
	cmd.Flags().StringVar(&compiler, "compiler", "claude", "binary for the sessionless compiler role")
	cmd.Flags().IntVar(&maxRounds, "max-rounds", 3, "circuit breaker: maximum debate rounds")
	cmd.Flags().IntVar(&maxContentions, "max-contentions", 5, "cap on opening critique contentions")
```

Replace with:

```go
	cmd.Flags().StringVar(&challenger, "challenger", "claude", "binary for the clean-room challenger role")
	cmd.Flags().StringVar(&incumbent, "incumbent", "agy", "binary for the vault-context incumbent role")
	cmd.Flags().StringVar(&compiler, "compiler", "claude", "binary for the sessionless compiler role")
	cmd.Flags().IntVar(&maxRounds, "max-rounds", 3, "circuit breaker: maximum debate rounds")
	cmd.Flags().IntVar(&maxContentions, "max-contentions", 5, "cap on opening critique contentions")
	cmd.Flags().StringArrayVar(&overridePromptFlags, "override-prompt", nil, "override a built-in prompt: --override-prompt <name>=<path> (opening_critique|turn|schema|compiler), repeatable")
```

- [ ] **Step 6: Run the new tests to verify they pass**

Run: `go test ./cmd/dialectic/ -run "TestDebateOverridePromptChangesAgentInput|TestDebateRejectsUnknownOverridePromptName" -v`
Expected: PASS.

- [ ] **Step 7: Run the full suite, build, and vet**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: clean build (cmd/dialectic builds again), every test passes, no vet warnings.

- [ ] **Step 8: Commit**

```bash
git add internal/orchestrate/loop.go cmd/dialectic/debate.go cmd/dialectic/debate_test.go cmd/dialectic/testdata/stub-capture-challenger.sh cmd/dialectic/testdata/stub-concur-incumbent.sh
git commit -m "feat: add --override-prompt flag to debate, wire through orchestrate and compile"
```

---

### Task 4: Add the `dialectic prompts` subcommand

**Files:**
- Create: `cmd/dialectic/prompts.go`
- Modify: `cmd/dialectic/root.go`
- Test: `cmd/dialectic/prompts_test.go`

**Interfaces:**
- Consumes: `agent.DefaultTemplates()`, `compile.DefaultTemplates()` (Tasks 1-2).
- Produces: `newPromptsCmd() *cobra.Command`, registered on the root — final integration point of this feature.

- [ ] **Step 1: Write the failing tests**

Create `cmd/dialectic/prompts_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPromptsCommandHumanOutput(t *testing.T) {
	root := newRootCmd()
	stdout := &bytes.Buffer{}
	root.SetOut(stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"prompts"})
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
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/dialectic/ -run "TestPromptsCommand" -v`
Expected: FAIL — `unknown command "prompts" for "dialectic"`.

- [ ] **Step 3: Implement the `prompts` subcommand**

Create `cmd/dialectic/prompts.go`:

```go
package main

import (
	"fmt"
	"strings"

	"github.com/murli-cli/murli-go"
	murliCobra "github.com/murli-cli/murli-go/cobra"
	"github.com/spf13/cobra"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/compile"
)

const promptFlowDiagram = `[Opening Critique]  challenger, turn 1, clean room
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
  compiled summary + update brief`

// promptCatalogOrder is the fixed display order for both human and agent
// output — matches the design's Template Catalog table.
var promptCatalogOrder = []string{"opening_critique", "turn", "schema", "compiler"}

func allDefaultTemplates() map[string]string {
	out := agent.DefaultTemplates()
	for name, text := range compile.DefaultTemplates() {
		out[name] = text
	}
	return out
}

func newPromptsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompts",
		Short: "Print the harness's built-in prompt templates and debate flow diagram",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := murliCobra.NewWriter(cmd)
			templates := allDefaultTemplates()

			var human strings.Builder
			human.WriteString(promptFlowDiagram)
			human.WriteString("\n\n")
			for _, name := range promptCatalogOrder {
				fmt.Fprintf(&human, "=== %s ===\n%s\n\n", name, templates[name])
			}

			w.WriteSuccess(strings.TrimRight(human.String(), "\n"), map[string]any{
				"diagram":   promptFlowDiagram,
				"templates": templates,
			})
			return nil
		},
	}
	murliCobra.Annotate(cmd, murli.Metadata{
		AgentDescription: "Prints the debate flow diagram and all four built-in prompt templates (opening_critique, turn, schema, compiler), raw with placeholders visible. Read-only, no artifact required.",
		Idempotent:       true,
	})
	return cmd
}
```

`promptCatalogOrder` is a fixed, deliberate display order (not derived by sorting), so no `sort` import is needed anywhere in this file.

- [ ] **Step 4: Register the command in `root.go`**

In `cmd/dialectic/root.go`, find:

```go
	root.AddCommand(newDebateCmd())
	root.AddCommand(newRunsCmd())
	murliCobra.Enable(root)
```

Replace with:

```go
	root.AddCommand(newDebateCmd())
	root.AddCommand(newRunsCmd())
	root.AddCommand(newPromptsCmd())
	murliCobra.Enable(root)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/dialectic/ -v`
Expected: PASS — `TestPromptsCommandHumanOutput`, `TestPromptsCommandAgentOutput`, and every other existing test in the package.

- [ ] **Step 6: Run the full suite, build, vet, and tidy**

Run: `go build ./... && go test ./... && go vet ./... && go mod tidy`
Expected: clean build, every test passes, no vet warnings, no `go.mod`/`go.sum` diff (no new external dependencies were added anywhere in this plan).

- [ ] **Step 7: Manual smoke test**

Run:
```bash
go build -o /tmp/dialectic-prompts-check ./cmd/dialectic
/tmp/dialectic-prompts-check prompts
```
Expected: the ASCII diagram followed by all four `=== name ===` sections with readable prompt text, placeholders visible (e.g. `{{.ArtifactPath}}`).

Run:
```bash
/tmp/dialectic-prompts-check prompts --agent | python3 -m json.tool | head -20
```
Expected: valid JSON with `result.diagram` and `result.templates` (4 keys).

- [ ] **Step 8: Commit**

```bash
git add cmd/dialectic/prompts.go cmd/dialectic/prompts_test.go cmd/dialectic/root.go
git commit -m "feat: add dialectic prompts subcommand for template introspection"
```

---

## Verification at the end

After Task 4, run a real debate with an override to confirm end-to-end behavior against real agents (attended, per the project's no-unattended-operation convention):

```bash
go build -o dialectic ./cmd/dialectic
echo "You are the CHALLENGER. Read {{.ArtifactPath}} and raise your single biggest concern." > /tmp/my-opening-critique.tmpl
./dialectic debate <some-real-artifact.md> --challenger agy --incumbent claude --override-prompt opening_critique=/tmp/my-opening-critique.tmpl --max-rounds 1
```

Confirm the challenger's actual behavior reflects the overridden framing (a single concern, not the built-in "at most 5, ranked by severity" framing), and that `dialectic prompts` output still matches what shipped in Task 4 (defaults are unaffected by a run-time override — `dialectic prompts` always shows the built-ins).
