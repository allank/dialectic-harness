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

func TestBuildIndexEscapesPipesInCellValues(t *testing.T) {
	root := t.TempDir()
	writeBrief(t, filepath.Join(root, "c", "prd-update-brief-3.md"), "changed-course",
		"scope split into A | B", "2026-07-08", "prd", "round_limit")

	table, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if strings.Contains(table, "A | B") {
		t.Errorf("unescaped pipe in cell value breaks table structure:\n%s", table)
	}
	if !strings.Contains(table, `A \| B`) {
		t.Errorf("expected the literal pipe to be escaped as \\|:\n%s", table)
	}
	// Every data row (skip header + separator) must have exactly 8 pipes:
	// 7 columns delimited by 8 `|` characters, with the escaped pipe not
	// counted as a delimiter.
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "|---") {
			continue
		}
		unescaped := strings.ReplaceAll(line, `\|`, "")
		if got := strings.Count(unescaped, "|"); got != 8 {
			t.Errorf("row has %d unescaped pipe delimiters, want 8: %q", got, line)
		}
	}
}
