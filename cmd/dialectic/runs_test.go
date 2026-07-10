package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunsCommandPrintsAndWritesIndex(t *testing.T) {
	dir := t.TempDir()
	brief := "---\narbiter_verdict: confirmed-course\nverdict_why: \"kept scope\"\ntopic_slug: prd\ntarget_artifact: prd.md\noutcome: consensus\nrun_dir: .a2a/x\ncreated: 2026-07-09\n---\n\n## Narrative\n"
	if err := os.WriteFile(filepath.Join(dir, "prd-update-brief-x.md"), []byte(brief), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"runs", dir, "--write"})
	if err := root.Execute(); err != nil {
		t.Fatalf("runs: %v", err)
	}
	if !strings.Contains(buf.String(), "confirmed-course") {
		t.Errorf("stdout should contain the table:\n%s", buf.String())
	}
	idx, err := os.ReadFile(filepath.Join(dir, "a2a-runs.md"))
	if err != nil || !strings.Contains(string(idx), "confirmed-course") {
		t.Errorf("--write must write a2a-runs.md: %v\n%s", err, idx)
	}
}
