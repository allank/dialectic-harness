package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmdHasDebateAndRunsPlaceholders(t *testing.T) {
	root := newRootCmd()
	if root.Use != "dialectic" {
		t.Fatalf("root.Use: want %q, got %q", "dialectic", root.Use)
	}
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute --help: %v", err)
	}
	if !strings.Contains(buf.String(), "dialectic") {
		t.Errorf("help output should mention dialectic, got:\n%s", buf.String())
	}
}
