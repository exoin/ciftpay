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

// MaskPIN reveals the first three and last three characters of a KRA PIN,
// masking the middle five characters with bullet points: P05•••••67X.
func MaskPIN(pin string) string {
	pin = strings.TrimSpace(pin)
	if len(pin) < 7 {
		return strings.Repeat("•", len(pin))
	}
	middleLen := len(pin) - 6
	if middleLen < 5 {
		middleLen = 5
	}
	return pin[:3] + strings.Repeat("•", middleLen) + pin[len(pin)-3:]
}

// Redact wraps a personal value so it can be logged safely.
func Redact(key, msisdn string) slog.Attr {
	return slog.String(key, MaskMSISDN(msisdn))
}
