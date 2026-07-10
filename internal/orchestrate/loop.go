// Package orchestrate drives the debate: route → invoke → validate → merge →
// persist, until the engine says compile. It owns the retry-once-then-halt
// policy for invalid turn files.
package orchestrate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/engine"
	"github.com/allank/dialectic/internal/progress"
	"github.com/allank/dialectic/internal/state"
	"github.com/allank/dialectic/internal/turn"
)

var ErrHalted = errors.New("run halted: invalid turn file after retry (state preserved)")

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

// report calls l.Progress if set. Every emit site in this file goes through
// report rather than nil-checking l.Progress directly.
func (l *Loop) report(ev progress.Event) {
	if l.Progress != nil {
		l.Progress(ev)
	}
}

func (l *Loop) Run(ctx context.Context) (string, error) {
	for {
		act := engine.NextAction(l.State)
		if act.Type == engine.ActionCompile {
			return act.Reason, nil
		}
		if err := l.takeTurn(ctx, act.Role); err != nil {
			return "", err
		}
	}
}

func (l *Loop) takeTurn(ctx context.Context, role state.Role) error {
	turnNum := l.State.TurnCount + 1
	turnPath := filepath.Join(l.TurnsDir, fmt.Sprintf("turn-%d-%s.yaml", turnNum, role))

	artifactRef := l.ArtifactPath
	statePath := "" // turn 1 is the opening critique: no prior state to read
	if turnNum > 1 {
		if err := l.State.Save(l.StatePath); err != nil {
			return err
		}
		statePath = l.StatePath
	}
	if role == state.RoleChallenger {
		// Clean room: scratch dir holds a copy of the artifact plus the
		// debate state; the challenger never reads vault paths.
		copyPath, err := agent.PrepareCleanRoom(l.ScratchDir, l.ArtifactPath)
		if err != nil {
			return err
		}
		artifactRef = copyPath
		if statePath != "" {
			// Load a fresh copy (never mutate l.State) and rewrite
			// target_artifact to the scratch copy's own path before saving
			// it into the clean room, so the challenger's scratch state
			// never reveals the real vault-adjacent artifact path.
			scratchState, err := state.Load(l.StatePath)
			if err != nil {
				return err
			}
			scratchState.TargetArtifact = artifactRef
			stateCopy := filepath.Join(l.ScratchDir, "debate-state.yaml")
			if err := scratchState.Save(stateCopy); err != nil {
				return err
			}
			statePath = stateCopy
		}
	}

	in := agent.PromptInput{
		Role:           role,
		ArtifactPath:   artifactRef,
		StatePath:      statePath,
		TurnFilePath:   turnPath,
		MaxContentions: l.MaxContentions,
		Directives:     directivesFor(l.State, role),
	}

	l.report(progress.Event{
		Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
		Message: fmt.Sprintf("invoking %s (turn %d)", role, turnNum),
	})
	tf, sessionID, errs, err := l.invokeAndValidate(ctx, role, in)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		l.report(progress.Event{
			Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
			Message: fmt.Sprintf("turn %d (%s): validation failed — retrying with feedback", turnNum, role),
		})
		in.RetryErrors = errs
		l.report(progress.Event{
			Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
			Message: fmt.Sprintf("invoking %s (turn %d, retry)", role, turnNum),
		})
		tf, sessionID, errs, err = l.invokeAndValidate(ctx, role, in)
		if err != nil {
			return err
		}
		if len(errs) > 0 {
			if serr := l.State.Save(l.StatePath); serr != nil {
				return fmt.Errorf("%w: turn %d (%s): %v (state save also failed: %v)", ErrHalted, turnNum, role, errs, serr)
			}
			return fmt.Errorf("%w: turn %d (%s): %v", ErrHalted, turnNum, role, errs)
		}
	}

	if sessionID != "" {
		l.State.Sessions[role] = sessionID
	}
	preActive := len(l.State.ActiveContentions)
	preConsensus := len(l.State.ConsensusBaseline)
	if err := engine.Merge(l.State, tf); err != nil {
		return err
	}
	newContentions := len(l.State.ActiveContentions) - preActive
	if newContentions < 0 {
		newContentions = 0
	}
	newConsensus := len(l.State.ConsensusBaseline) - preConsensus
	l.report(progress.Event{
		Stage: "turn", Turn: turnNum, Round: l.State.RoundCount, MaxRounds: l.State.MaxRounds,
		Message: fmt.Sprintf("turn %d (%s) complete: %d new contentions, %d resolved to consensus", turnNum, role, newContentions, newConsensus),
	})
	return l.State.Save(l.StatePath)
}

func (l *Loop) invokeAndValidate(ctx context.Context, role state.Role, in agent.PromptInput) (turn.File, string, []string, error) {
	res, err := l.Runner.Invoke(ctx, agent.Request{
		Binary:     l.State.Roles[role],
		Prompt:     agent.BuildPrompt(in),
		WorkDir:    workDirFor(l, role),
		SessionID:  l.State.Sessions[role],
		OutputPath: in.TurnFilePath,
	})
	if err != nil {
		if serr := l.State.Save(l.StatePath); serr != nil {
			return turn.File{}, "", nil, fmt.Errorf("agent invocation failed AND state save also failed: %w (save error: %v)", err, serr)
		}
		return turn.File{}, "", nil, fmt.Errorf("agent invocation failed (state preserved): %w", err)
	}
	tf, perr := turn.Parse(res.Output)
	if perr != nil {
		return turn.File{}, res.SessionID, []string{perr.Error()}, nil
	}
	return tf, res.SessionID, turn.Validate(tf, l.State), nil
}

func workDirFor(l *Loop, role state.Role) string {
	if role == state.RoleChallenger {
		return l.ScratchDir
	}
	return filepath.Dir(l.ArtifactPath)
}

func directivesFor(st *state.DebateState, role state.Role) []state.Directive {
	var out []state.Directive
	for _, d := range st.RequiredRebuttals {
		if d.Target == role {
			out = append(out, d)
		}
	}
	return out
}
