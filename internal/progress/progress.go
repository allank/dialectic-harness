// Package progress defines a framework-agnostic vocabulary for reporting
// debate progress. It has no dependency on murli or any CLI framework —
// only cmd/dialectic (the sole murli import site) translates Event into
// murli.ProgressEvent.
package progress

// Event is a single progress update, emitted by internal/orchestrate and
// internal/compile as they run.
type Event struct {
	Stage     string // "turn" | "compile"
	Message   string
	Turn      int // 1-indexed; 0 if not applicable (compile stage)
	Round     int // current round_count at the time of the event
	MaxRounds int // st.MaxRounds; 0 if not applicable (compile stage)
}

// Func reports a single progress event. A nil Func is always safe: callers
// must nil-check before invoking it (there is no default no-op wrapper).
type Func func(Event)
