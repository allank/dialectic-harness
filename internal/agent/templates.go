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
