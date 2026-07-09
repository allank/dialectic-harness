package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareCleanRoomCopiesArtifactOnly(t *testing.T) {
	vault := t.TempDir()
	artifact := filepath.Join(vault, "prd.md")
	if err := os.WriteFile(artifact, []byte("# PRD\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "CLAUDE.md"), []byte("vault rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()

	copyPath, err := PrepareCleanRoom(scratch, artifact)
	if err != nil {
		t.Fatalf("PrepareCleanRoom: %v", err)
	}
	if filepath.Dir(copyPath) != scratch {
		t.Errorf("copy must live in scratch dir, got %s", copyPath)
	}
	body, err := os.ReadFile(copyPath)
	if err != nil || string(body) != "# PRD\nbody\n" {
		t.Errorf("copy content: %q err=%v", body, err)
	}
	entries, _ := os.ReadDir(scratch)
	if len(entries) != 1 {
		t.Errorf("scratch must contain only the artifact copy, got %d entries", len(entries))
	}
}
