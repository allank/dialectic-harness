package compile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/allank/dialectic/internal/state"
)

var citeRe = regexp.MustCompile(`\((C\d+), turn (\d+)\)`)

// ValidateCitations deterministically checks the Compiler's output against
// the ledger: every claim cited, every cited ID real, categories matched.
// Semantic faithfulness rests on citations making spot-checks cheap.
func ValidateCitations(doc string, st *state.DebateState) []string {
	var errs []string

	sections := splitSections(doc)
	for _, name := range []string{"Narrative", "Proposed Changes", "Judgment Calls"} {
		if _, ok := sections[name]; !ok {
			errs = append(errs, fmt.Sprintf("missing required section: ## %s", name))
		}
	}

	known := map[string]bool{}
	consensus := map[string]bool{}
	unresolved := map[string]bool{}
	for _, c := range st.ConsensusBaseline {
		known[c.ID], consensus[c.ID] = true, true
	}
	for _, c := range st.Withdrawn {
		known[c.ID] = true
	}
	for _, c := range st.ActiveContentions {
		known[c.ID], unresolved[c.ID] = true, true
	}
	for _, e := range st.ContentionHistory {
		known[e.Contention] = true
	}

	for _, m := range citeRe.FindAllStringSubmatch(doc, -1) {
		id := m[1]
		turnNum, _ := strconv.Atoi(m[2])
		if !known[id] {
			errs = append(errs, fmt.Sprintf("unknown contention id %s cited", id))
		}
		if turnNum < 1 || turnNum > st.TurnCount {
			errs = append(errs, fmt.Sprintf("citation (%s, turn %d) is out of range: debate had %d turns", id, turnNum, st.TurnCount))
		}
	}

	checkBullets := func(section string, allowed map[string]bool, requirement string) {
		for _, line := range strings.Split(sections[section], "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "- ") {
				continue
			}
			cites := citeRe.FindAllStringSubmatch(line, -1)
			if len(cites) == 0 {
				errs = append(errs, fmt.Sprintf("%s item has no citation: %q", section, strings.TrimSpace(line)))
				continue
			}
			for _, m := range cites {
				if known[m[1]] && !allowed[m[1]] {
					errs = append(errs, fmt.Sprintf("%s item cites %s %s: %q", section, m[1], requirement, strings.TrimSpace(line)))
				}
			}
		}
	}
	checkBullets("Proposed Changes", consensus, "which is non-consensus — proposals derive only from consensus_baseline")
	checkBullets("Judgment Calls", unresolved, "but must cite unresolved tensions only")

	return errs
}

func splitSections(doc string) map[string]string {
	out := map[string]string{}
	var current string
	var body strings.Builder
	flush := func() {
		if current != "" {
			out[current] = body.String()
		}
		body.Reset()
	}
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		body.WriteString(line + "\n")
	}
	flush()
	return out
}
