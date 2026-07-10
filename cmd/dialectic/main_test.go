package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunPrintsErrorToStderrAndReturnsNonZero(t *testing.T) {
	cmd := &cobra.Command{
		Use: "stub",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("agent invocation failed (state preserved): boom")
		},
	}
	cmd.SetArgs([]string{})
	stderr := &bytes.Buffer{}

	code := run(cmd, stderr)

	if code == 0 {
		t.Fatal("run should return non-zero exit code when Execute() returns an error")
	}
	if !strings.Contains(stderr.String(), "agent invocation failed") {
		t.Errorf("run must print the error to stderr, got: %q", stderr.String())
	}
}

func TestRunReturnsZeroOnSuccess(t *testing.T) {
	cmd := &cobra.Command{
		Use: "stub",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.SetArgs([]string{})
	stderr := &bytes.Buffer{}

	code := run(cmd, stderr)

	if code != 0 {
		t.Errorf("run should return 0 on success, got %d", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("run should print nothing to stderr on success, got: %q", stderr.String())
	}
}
