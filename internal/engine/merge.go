// Package engine is the deterministic core of the orchestrator: it merges
// validated turn files into state and decides the next action. It never
// judges debate content — agents judge, the engine bookkeeps.
package engine

import (
	"fmt"

	"github.com/allank/dialectic/internal/state"
	"github.com/allank/dialectic/internal/turn"
)

// Merge applies a turn file that has already passed turn.Validate.
func Merge(st *state.DebateState, tf turn.File) error {
	if tf.Agent != st.NextRole {
		return fmt.Errorf("merge: turn from %q but next role is %q (was the file validated?)", tf.Agent, st.NextRole)
	}
	turnNum := st.TurnCount + 1

	// Track per-ID stances within this file for the resolution gate:
	// concur resolves only with no accompanying rebut on the same ID.
	concurRationale := map[string]string{}
	rebutted := map[string]bool{}
	withdrawRationale := map[string]string{}

	for _, e := range tf.Entries {
		id := e.Contention
		switch e.Stance {
		case state.StanceNew:
			id = st.NextContentionID()
			st.ActiveContentions = append(st.ActiveContentions, state.Contention{
				ID:       id,
				Issue:    e.Issue,
				Severity: e.Severity,
				Stances:  map[state.Role]string{tf.Agent: stanceText(e)},
			})
		case state.StanceConcur:
			concurRationale[id] = e.Rationale
		case state.StanceRebut:
			rebutted[id] = true
			updateStanceText(st, id, tf.Agent, e)
		case state.StanceWithdraw:
			withdrawRationale[id] = e.Rationale
		}
		st.ContentionHistory = append(st.ContentionHistory, state.LedgerEntry{
			Turn:       turnNum,
			Agent:      tf.Agent,
			Contention: id,
			Stance:     e.Stance,
			Rationale:  e.Rationale,
		})
	}

	// Resolution gate.
	for id, rationale := range concurRationale {
		if rebutted[id] {
			continue
		}
		if c := removeActive(st, id); c != nil {
			st.ConsensusBaseline = append(st.ConsensusBaseline, state.ConsensusItem{
				ID: c.ID, Issue: c.Issue, ResolvedTurn: turnNum, Rationale: rationale,
			})
		}
	}
	// Withdrawals leave the debate but are not consensus.
	for id, rationale := range withdrawRationale {
		if rebutted[id] {
			continue
		}
		if c := removeActive(st, id); c != nil {
			st.Withdrawn = append(st.Withdrawn, state.ConsensusItem{
				ID: c.ID, Issue: c.Issue, ResolvedTurn: turnNum, Rationale: rationale,
			})
		}
	}

	mergeDirectives(st, tf, turnNum)

	st.TurnCount = turnNum
	if tf.Agent == state.RoleIncumbent {
		st.RoundCount++
	}
	st.NextRole = tf.Agent.Other()
	return nil
}

func updateStanceText(st *state.DebateState, id string, agent state.Role, e turn.Entry) {
	c := st.FindActive(id)
	if c == nil {
		return
	}
	if c.Stances == nil {
		c.Stances = map[state.Role]string{}
	}
	c.Stances[agent] = stanceText(e)
}

// stanceText picks the text to record for a stance: Position when the
// entry supplies one, falling back to Rationale otherwise.
func stanceText(e turn.Entry) string {
	if e.Position != "" {
		return e.Position
	}
	return e.Rationale
}

func removeActive(st *state.DebateState, id string) *state.Contention {
	for i := range st.ActiveContentions {
		if st.ActiveContentions[i].ID == id {
			c := st.ActiveContentions[i]
			st.ActiveContentions = append(st.ActiveContentions[:i], st.ActiveContentions[i+1:]...)
			return &c
		}
	}
	return nil
}

// mergeDirectives processes directive issuance and ignored-directive detection.
// Directives issued by the current agent target the other role; satisfaction
// is detected via field reads (contention citation) rather than judgment.
func mergeDirectives(st *state.DebateState, tf turn.File, turnNum int) {
	cited := map[string]bool{}
	for _, e := range tf.Entries {
		if e.Contention != "" {
			cited[e.Contention] = true
		}
	}
	var remaining []state.Directive
	for _, d := range st.RequiredRebuttals {
		switch {
		case d.Target != tf.Agent:
			remaining = append(remaining, d)
		case cited[d.Contention]:
			// satisfied: drop
		default:
			st.IgnoredDirectives = append(st.IgnoredDirectives, d)
		}
	}
	st.RequiredRebuttals = remaining
	for _, req := range tf.Directives {
		st.RequiredRebuttals = append(st.RequiredRebuttals, state.Directive{
			Target:     tf.Agent.Other(),
			Contention: req.Contention,
			Directive:  req.Directive,
			IssuedTurn: turnNum,
		})
	}
}
