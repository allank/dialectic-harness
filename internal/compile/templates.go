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
