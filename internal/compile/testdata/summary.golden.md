# Debate Summary: zaru-order-book

- Target artifact: drafts/zaru-order-book.md
- Outcome: round_limit
- Rounds: 3 of 3; turns: 6
- Roles: challenger=agy (clean room), incumbent=claude

## Decisions Made and Why

- **C1 — The matching engine must be written in Go.** (resolved turn 3): Concurrence: team skill set and murli reuse outweigh alternatives.

Withdrawn:
- **C3 — Missing glossary** (withdrawn turn 4): Incumbent cited CONTEXT.md; contention withdrawn.

## Unresolved Tensions

- **C2 — In-memory vs Redis state store** (severity: high)
  - challenger: Redis is required for fault tolerance.
  - incumbent: In-memory via channel concurrency is faster.

## Ignored Directives

- challenger ignored a directive on **C2** (issued turn 2): Provide a latency benchmark for Redis at 10k TPS.
