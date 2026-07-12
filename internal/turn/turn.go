// Package turn defines the small structured file an agent writes each turn.
// The orchestrator validates it here and merges it into state in the engine;
// agents never touch .debate-state.yaml directly.
package turn

import (
	"bytes"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/allank/dialectic/internal/state"
)

type Entry struct {
	Contention string       `yaml:"contention,omitempty"`
	Stance     state.Stance `yaml:"stance"`
	Rationale  string       `yaml:"rationale"`
	Issue      string       `yaml:"issue,omitempty"`
	Severity   string       `yaml:"severity,omitempty"`
	Position   string       `yaml:"position,omitempty"`
}

type DirectiveRequest struct {
	Contention string `yaml:"contention"`
	Directive  string `yaml:"directive"`
}

type File struct {
	Agent      state.Role         `yaml:"agent"`
	Entries    []Entry            `yaml:"entries"`
	Directives []DirectiveRequest `yaml:"directives,omitempty"`
}

// trailingClosingTag matches a lone XML/HTML-style closing tag (e.g.
// "</content>") on its own line. Observed leaking onto the last line of an
// otherwise well-formed turn file — an artifact of the underlying agent
// CLI's own tool-call formatting on a long generation, not part of the
// document itself.
var trailingClosingTag = regexp.MustCompile(`</[A-Za-z_][\w:-]*>\s*$`)

// stripTrailingLeakedClosingTag removes a single stray closing tag from the
// end of raw, if present, leaving everything else untouched. Only the very
// last line is eligible, and only when real content precedes it — this
// targets the one observed failure shape without papering over genuinely
// malformed turn files.
func stripTrailingLeakedClosingTag(raw []byte) []byte {
	trimmed := bytes.TrimRight(raw, "\n\r\t ")
	loc := trailingClosingTag.FindIndex(trimmed)
	if loc == nil || loc[1] != len(trimmed) {
		return raw
	}
	before := trimmed[:loc[0]]
	if len(bytes.TrimSpace(before)) == 0 {
		return raw
	}
	return before
}

// Parse strictly decodes a turn file. Unknown fields are rejected so a turn
// file cannot smuggle in state mutations (e.g. rewritten history).
func Parse(raw []byte) (File, error) {
	raw = stripTrailingLeakedClosingTag(raw)
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var tf File
	if err := dec.Decode(&tf); err != nil {
		return File{}, fmt.Errorf("parse turn file: %w", err)
	}
	return tf, nil
}

// Validate returns one message per violation, phrased so it can be fed back
// to the agent verbatim on the single retry. maxContentions caps how many
// stance-new entries the opening critique (the first turn, st.TurnCount==0)
// may raise; it is ignored on every later turn, where new contentions are
// uncapped.
func Validate(tf File, st *state.DebateState, maxContentions int) []string {
	var errs []string
	if tf.Agent != st.NextRole {
		errs = append(errs, fmt.Sprintf("agent is %q but the orchestrator expected turn from %s", tf.Agent, st.NextRole))
	}
	if len(tf.Entries) == 0 {
		errs = append(errs, "turn file must contain at least one entry")
	}
	newCount := 0
	for i, e := range tf.Entries {
		at := fmt.Sprintf("entries[%d]", i)
		if !state.ValidStance(e.Stance) {
			errs = append(errs, fmt.Sprintf("%s: invalid stance %q (must be concur|rebut|withdraw|new)", at, e.Stance))
		}
		if e.Rationale == "" {
			errs = append(errs, fmt.Sprintf("%s: rationale is mandatory; bare concessions are invalid", at))
		}
		if e.Stance == state.StanceNew {
			newCount++
			if e.Issue == "" {
				errs = append(errs, fmt.Sprintf("%s: stance 'new' requires an issue statement", at))
			}
			if e.Contention != "" {
				errs = append(errs, fmt.Sprintf("%s: stance 'new' must omit contention id; the orchestrator assigns ids", at))
			}
			continue
		}
		if st.IsConsensus(e.Contention) {
			errs = append(errs, fmt.Sprintf("%s: contention %s is already resolved; consensus items are not re-litigated", at, e.Contention))
		} else if st.FindActive(e.Contention) == nil {
			errs = append(errs, fmt.Sprintf("%s: unknown contention id %q; cite an active contention or use stance 'new'", at, e.Contention))
		}
	}
	if st.TurnCount == 0 && maxContentions > 0 && newCount > maxContentions {
		errs = append(errs, fmt.Sprintf("opening critique raised %d contentions, exceeding the cap of %d", newCount, maxContentions))
	}
	for i, d := range tf.Directives {
		if st.FindActive(d.Contention) == nil {
			errs = append(errs, fmt.Sprintf("directives[%d]: unknown contention id %q; directives must target an active contention", i, d.Contention))
		}
	}
	return errs
}
