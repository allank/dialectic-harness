#!/bin/sh
# Always concurs with C1, so a debate started with --max-rounds 1 reaches
# consensus immediately after this turn.
cat > "$DIALECTIC_OUTPUT_FILE" <<'EOF'
agent: incumbent
entries:
  - contention: C1
    stance: concur
    rationale: "agreed, resolving to consensus so the run completes quickly"
EOF
echo '{"session_id":"stub-sess"}'
