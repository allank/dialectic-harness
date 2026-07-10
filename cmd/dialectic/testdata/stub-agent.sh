#!/bin/sh
# Plays both roles across a two-turn debate. Counts invocations via
# $STUB_COUNT_FILE, writes the scripted turn file to $DIALECTIC_OUTPUT_FILE.
n=$(cat "$STUB_COUNT_FILE" 2>/dev/null || echo 0)
n=$((n+1))
echo "$n" > "$STUB_COUNT_FILE"
case "$n" in
1) cat > "$DIALECTIC_OUTPUT_FILE" <<'EOF'
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
;;
2) cat > "$DIALECTIC_OUTPUT_FILE" <<'EOF'
agent: incumbent
entries:
  - contention: C1
    stance: concur
    rationale: "agreed, rollback section needed"
  - contention: C2
    stance: rebut
    rationale: "latency budget rules out alternatives"
    position: "In-memory is deliberate; latency budget."
EOF
;;
*) cat > "$DIALECTIC_OUTPUT_FILE" <<EOF
agent: $([ $((n % 2)) -eq 1 ] && echo challenger || echo incumbent)
entries:
  - contention: C2
    stance: rebut
    rationale: "turn $n: still contested"
EOF
;;
esac
echo '{"session_id":"stub-sess"}'
