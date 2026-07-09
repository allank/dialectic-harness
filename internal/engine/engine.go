package engine

import (
	"github.com/allank/dialectic/internal/state"
)

type ActionType string

const (
	ActionRoute   ActionType = "route"
	ActionCompile ActionType = "compile"
)

type Action struct {
	Type   ActionType
	Role   state.Role // set when Type == ActionRoute
	Reason string     // "consensus" | "round_limit" when Type == ActionCompile
}

// NextAction is the orchestrator's control flow as a pure function:
// circuit breaker, then resolution check, then routing.
func NextAction(st *state.DebateState) Action {
	if st.TurnCount == 0 {
		return Action{Type: ActionRoute, Role: state.RoleChallenger}
	}
	if len(st.ActiveContentions) == 0 {
		return Action{Type: ActionCompile, Reason: "consensus"}
	}
	if st.RoundCount >= st.MaxRounds {
		return Action{Type: ActionCompile, Reason: "round_limit"}
	}
	return Action{Type: ActionRoute, Role: st.NextRole}
}
