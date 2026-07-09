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
			stanceText := e.Position
			if stanceText == "" {
				stanceText = e.Rationale
			}
			st.ActiveContentions = append(st.ActiveContentions, state.Contention{
				ID:       id,
				Issue:    e.Issue,
				Severity: e.Severity,
				Stances:  map[state.Role]string{tf.Agent: stanceText},
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
	text := e.Position
	if text == "" {
		text = e.Rationale
	}
	c.Stances[agent] = text
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

// mergeDirectives is completed in the directives task; for now it is a stub
// so Merge compiles.
func mergeDirectives(st *state.DebateState, tf turn.File, turnNum int) {}
