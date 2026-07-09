// Package orchestrate drives the debate: route → invoke → validate → merge →
// persist, until the engine says compile. It owns the retry-once-then-halt
// policy for invalid turn files.
package orchestrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/allank/dialectic/internal/agent"
	"github.com/allank/dialectic/internal/engine"
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
			stateCopy := filepath.Join(l.ScratchDir, "debate-state.yaml")
			if err := copyFile(l.StatePath, stateCopy); err != nil {
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

	tf, sessionID, errs, err := l.invokeAndValidate(ctx, role, in)
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		in.RetryErrors = errs
		tf, sessionID, errs, err = l.invokeAndValidate(ctx, role, in)
		if err != nil {
			return err
		}
		if len(errs) > 0 {
			_ = l.State.Save(l.StatePath)
			return fmt.Errorf("%w: turn %d (%s): %v", ErrHalted, turnNum, role, errs)
		}
	}

	if sessionID != "" {
		l.State.Sessions[role] = sessionID
	}
	if err := engine.Merge(l.State, tf); err != nil {
		return err
	}
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
		_ = l.State.Save(l.StatePath)
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

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
