#!/bin/sh
# Captures its full argv (the prompt is the last argument for both claude-
# and agy-style invocations) to $PROMPT_CAPTURE_FILE, then writes a turn file
# with two contentions (C1, C2) so a downstream stub-concur-incumbent.sh
# (which concurs only C1) leaves C2 unresolved. That matches what
# stub-compiler.sh's hardcoded output cites, so ValidateCitations passes.
echo "$@" > "$PROMPT_CAPTURE_FILE"
cat > "$DIALECTIC_OUTPUT_FILE" <<'EOF'
agent: challenger
entries:
  - stance: new
    issue: "no rollback plan"
    severity: high
    rationale: "artifact lacks a rollback section"
    position: "A rollback section is required."
  - stance: new
    issue: "store choice unjustified"
    severity: medium
    rationale: "no rationale given for in-memory store"
    position: "Justify or change the store choice."
EOF
echo '{"session_id":"stub-sess"}'
