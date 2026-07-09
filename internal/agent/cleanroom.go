package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// PrepareCleanRoom copies the target artifact (and nothing else) into the
// scratch directory that becomes the challenger's cwd. No vault CLAUDE.md,
// no project memory, no other vault files — the contamination under test is
// accumulated content, not writing conventions.
func PrepareCleanRoom(scratchDir, artifactPath string) (string, error) {
	body, err := os.ReadFile(artifactPath)
	if err != nil {
		return "", fmt.Errorf("read artifact: %w", err)
	}
	dst := filepath.Join(scratchDir, filepath.Base(artifactPath))
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return "", fmt.Errorf("copy artifact into clean room: %w", err)
	}
	return dst, nil
}
