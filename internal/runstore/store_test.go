package runstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/allank/dialectic/internal/state"
)

var testNow = time.Date(2026, 7, 9, 14, 30, 0, 0, time.UTC)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"drafts/Zaru Order Book.md":  "zaru-order-book",
		"PRD - A2A Harness.md":       "prd-a2a-harness",
		"/abs/path/simple.md":        "simple",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q): want %q, got %q", in, want, got)
		}
	}
}

func TestNewRunCreatesHiddenRunDirBesideArtifact(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "My PRD.md")
	if err := os.WriteFile(artifact, []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := NewRun(artifact, testNow)
	if err != nil {
		t.Fatalf("NewRun: %v", err)
	}
	wantRun := filepath.Join(dir, ".a2a", "my-prd-20260709T143000")
	if p.RunDir != wantRun {
		t.Errorf("RunDir: want %s, got %s", wantRun, p.RunDir)
	}
	for _, d := range []string{p.RunDir, p.TurnsDir, p.ScratchDir} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Errorf("dir %s should exist: %v", d, err)
		}
	}
	if p.StatePath != filepath.Join(wantRun, "debate-state.yaml") {
		t.Errorf("StatePath: %s", p.StatePath)
	}
	if p.SummaryPath != filepath.Join(dir, "my-prd-debate-summary-20260709T143000.md") {
		t.Errorf("SummaryPath: %s", p.SummaryPath)
	}
	if p.BriefPath != filepath.Join(dir, "my-prd-update-brief-20260709T143000.md") {
		t.Errorf("BriefPath: %s", p.BriefPath)
	}
}

func TestWriteUpdateBriefFrontmatter(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "prd.md")
	if err := os.WriteFile(artifact, []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := NewRun(artifact, testNow)
	if err != nil {
		t.Fatal(err)
	}
	st := state.New("prd", artifact, 3, nil)
	doc := "## Narrative\n\nbody\n\n## Proposed Changes\n\nNone.\n\n## Judgment Calls\n\nNone.\n"
	if err := WriteUpdateBrief(p, st, doc, "consensus", testNow); err != nil {
		t.Fatalf("WriteUpdateBrief: %v", err)
	}
	raw, err := os.ReadFile(p.BriefPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		"---\n",
		"arbiter_verdict: pending",
		"verdict_why: \"\"",
		"topic_slug: prd",
		"outcome: consensus",
		"created: 2026-07-09",
		"## Narrative",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("brief missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "---\n") {
		t.Error("brief must start with YAML frontmatter")
	}
}

func TestWriteSummary(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "prd.md")
	if err := os.WriteFile(artifact, []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _ := NewRun(artifact, testNow)
	if err := WriteSummary(p, "# Debate Summary: prd\n"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p.SummaryPath)
	if err != nil || !strings.HasPrefix(string(raw), "# Debate Summary") {
		t.Errorf("summary: %q err=%v", raw, err)
	}
}
