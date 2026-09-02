// Package log builds the process-wide slog logger and offers PII-safe helpers.
package log

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON logger for staging/production and a text logger for local
// development. The level is Debug locally and Info otherwise.
func New(appEnv string) *slog.Logger {
	level := slog.LevelInfo
	if appEnv == "local" || appEnv == "test" {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if appEnv == "local" {
		h = slog.NewTextHandler(os.Stdout, opts)
	} else {
		h = slog.NewJSONHandler(os.Stdout, opts)
	}
	l := slog.New(h)
	slog.SetDefault(l)
	return l
}

// MaskMSISDN keeps the country code and the last three digits: 2547•••••149.
func MaskMSISDN(msisdn string) string {
	msisdn = strings.TrimSpace(msisdn)
	if len(msisdn) < 7 {
		return strings.Repeat("•", len(msisdn))
	}
	return msisdn[:4] + strings.Repeat("•", len(msisdn)-7) + msisdn[len(msisdn)-3:]
}

// MaskPIN keeps the first and last character of a KRA PIN: A•••••••••B.
func MaskPIN(pin string) string {
	pin = strings.TrimSpace(pin)
	if len(pin) < 3 {
		return strings.Repeat("•", len(pin))
	}
	return pin[:1] + strings.Repeat("•", len(pin)-2) + pin[len(pin)-1:]
}

// Redact wraps a personal value so it can be logged safely.
func Redact(key, msisdn string) slog.Attr {
	return slog.String(key, MaskMSISDN(msisdn))
}
