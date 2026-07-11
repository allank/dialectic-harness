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
