package fiscal

import (
	"errors"
	"fmt"
	"time"
)

// State is the position of an invoice in its lifecycle.
type State string

// Invoice states. See docs/architecture.md §4.
const (
	StateDraft           State = "DRAFT"
	StateQueued          State = "QUEUED"
	StateSubmitted       State = "SUBMITTED"
	StateAcked           State = "ACKED"
	StateFailedRetryable State = "FAILED_RETRYABLE"
	StateFailedTerminal  State = "FAILED_TERMINAL"
	StateNeedsReview     State = "NEEDS_REVIEW"
)

// ErrIllegalTransition is returned by Transition for moves the machine forbids.
var ErrIllegalTransition = errors.New("fiscal: illegal state transition")

var transitions = map[State][]State{
	StateDraft:           {StateQueued},
	StateQueued:          {StateSubmitted},
	StateSubmitted:       {StateAcked, StateFailedRetryable, StateFailedTerminal},
	StateFailedRetryable: {StateQueued, StateFailedTerminal},
	StateFailedTerminal:  {StateNeedsReview},
	StateNeedsReview:     {StateQueued},
	StateAcked:           {},
}

// Valid reports whether s is a known state.
func (s State) Valid() bool {
	_, ok := transitions[s]
	return ok
}

// Terminal reports whether no automatic transition leaves s.
func (s State) Terminal() bool { return s == StateAcked || s == StateNeedsReview }

// CanTransition reports whether from → to is allowed.
func CanTransition(from, to State) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Transition validates from → to and returns to, or ErrIllegalTransition.
func Transition(from, to State) (State, error) {
	if !from.Valid() || !to.Valid() {
		return from, fmt.Errorf("%w: %s → %s (unknown state)", ErrIllegalTransition, from, to)
	}
	if !CanTransition(from, to) {
		return from, fmt.Errorf("%w: %s → %s", ErrIllegalTransition, from, to)
	}
	return to, nil
}

// MaxAttempts is the number of submission attempts before NEEDS_REVIEW.
const MaxAttempts = 8

// BaseBackoff is the delay after the first failed attempt.
const BaseBackoff = 15 * time.Second

// Backoff returns the delay before attempt n+1 after attempt n (1-based)
// failed: 15s, 30s, 1m, 2m, 4m, 8m, 16m, 32m.
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > MaxAttempts {
		attempt = MaxAttempts
	}
	return BaseBackoff << (attempt - 1)
}

// Outcome describes what the worker should do after a submission attempt.
type Outcome struct {
	Next    State
	RetryIn time.Duration // > 0 only when Next == StateQueued
}

// Resolve decides the next state after attempt number `attempt` finished with
// err (nil on success). It encodes the retry policy in one place.
func Resolve(attempt int, err error) Outcome {
	if err == nil {
		return Outcome{Next: StateAcked}
	}
	switch Classify(err) {
	case ClassTerminal:
		return Outcome{Next: StateFailedTerminal}
	default:
		if attempt >= MaxAttempts {
			return Outcome{Next: StateFailedTerminal}
		}
		return Outcome{Next: StateQueued, RetryIn: Backoff(attempt)}
	}
}
