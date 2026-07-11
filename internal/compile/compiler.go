package compile

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/state"
)

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
