package runstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBrief(t *testing.T, path, verdict, why, created, slug, outcome string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\narbiter_verdict: " + verdict + "\nverdict_why: \"" + why + "\"\ntopic_slug: " + slug +
		"\ntarget_artifact: prd.md\noutcome: " + outcome + "\nrun_dir: .a2a/x\ncreated: " + created + "\n---\n\n## Narrative\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIndexCollectsBriefsAndSortsByCreatedDesc(t *testing.T) {
	root := t.TempDir()
	writeBrief(t, filepath.Join(root, "a", "prd-update-brief-1.md"), "pending", "", "2026-07-01", "prd", "consensus")
	writeBrief(t, filepath.Join(root, "b", "spec-update-brief-2.md"), "changed-course", "moved to Redis", "2026-07-09", "spec", "round_limit")
	// Non-brief markdown must be ignored.
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# just notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Files under .a2a must be skipped.
	writeBrief(t, filepath.Join(root, ".a2a", "hidden-update-brief.md"), "ignored", "", "2026-07-05", "hidden", "consensus")

	table, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if !strings.Contains(table, "| Created | Artifact | Slug | Outcome | Verdict | Why | Brief |") {
		t.Errorf("missing header:\n%s", table)
	}
	if strings.Contains(table, "hidden") {
		t.Error(".a2a contents must be skipped")
	}
	iSpec := strings.Index(table, "spec")
	iPrd := strings.Index(table, "prd-update-brief")
	if iSpec == -1 || iPrd == -1 || iSpec > iPrd {
		t.Errorf("rows must sort by created desc (spec 2026-07-09 first):\n%s", table)
	}
	if !strings.Contains(table, "changed-course") || !strings.Contains(table, "moved to Redis") {
		t.Errorf("verdict fields missing:\n%s", table)
	}
	if !strings.Contains(table, "](b/spec-update-brief-2.md)") {
		t.Errorf("brief link must be a relative Markdown link:\n%s", table)
	}
}
