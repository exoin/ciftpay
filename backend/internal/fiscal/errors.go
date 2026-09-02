package fiscal

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// Class is the retry classification of a submission error.
type Class string

// Error classes. Unknown errors are treated as retryable so a transient vendor
// quirk never strands an invoice; they are logged at ERROR for triage.
const (
	ClassOK        Class = "ok"
	ClassRetryable Class = "retryable"
	ClassTerminal  Class = "terminal"
)

// ValidationError is a terminal rejection: the document itself is wrong and
// resubmitting it unchanged would fail again.
type ValidationError struct {
	Code    string // stable machine code, e.g. invalid_item_code, buyer_pin_invalid
	Message string
	Field   string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("fiscal: validation %s on %s: %s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("fiscal: validation %s: %s", e.Code, e.Message)
}

// ConfigError is terminal and points at credentials or device registration.
type ConfigError struct {
	Code    string // invalid_api_key, device_not_registered
	Message string
}

func (e *ConfigError) Error() string { return "fiscal: config " + e.Code + ": " + e.Message }

// TransientError is an explicit retryable failure (5xx, 429, maintenance).
type TransientError struct {
	Code    string // upstream_5xx, rate_limited, maintenance
	Message string
}

func (e *TransientError) Error() string { return "fiscal: transient " + e.Code + ": " + e.Message }

// Classify maps an error to a Class.
func Classify(err error) Class {
	if err == nil {
		return ClassOK
	}
	var ve *ValidationError
	var ce *ConfigError
	var te *TransientError
	switch {
	case errors.As(err, &ve), errors.As(err, &ce):
		return ClassTerminal
	case errors.As(err, &te):
		return ClassRetryable
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ClassRetryable
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ClassRetryable
	}
	return ClassRetryable
}

// ErrorCode extracts the stable machine code from a classified error, or
// "unknown" for errors the adapters did not wrap.
func ErrorCode(err error) string {
	var ve *ValidationError
	var ce *ConfigError
	var te *TransientError
	switch {
	case errors.As(err, &ve):
		return ve.Code
	case errors.As(err, &ce):
		return ce.Code
	case errors.As(err, &te):
		return te.Code
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case err == nil:
		return ""
	}
	return "unknown"
}
