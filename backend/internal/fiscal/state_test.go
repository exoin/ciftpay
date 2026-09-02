package fiscal

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"
)

func TestTransitionTable(t *testing.T) {
	allowed := []struct{ from, to State }{
		{StateDraft, StateQueued},
		{StateQueued, StateSubmitted},
		{StateSubmitted, StateAcked},
		{StateSubmitted, StateFailedRetryable},
		{StateSubmitted, StateFailedTerminal},
		{StateFailedRetryable, StateQueued},
		{StateFailedRetryable, StateFailedTerminal},
		{StateFailedTerminal, StateNeedsReview},
		{StateNeedsReview, StateQueued},
	}
	for _, tc := range allowed {
		if got, err := Transition(tc.from, tc.to); err != nil || got != tc.to {
			t.Errorf("%s → %s should be allowed, got %s err %v", tc.from, tc.to, got, err)
		}
	}
	forbidden := []struct{ from, to State }{
		{StateAcked, StateQueued},
		{StateAcked, StateSubmitted},
		{StateDraft, StateAcked},
		{StateQueued, StateAcked},
		{StateNeedsReview, StateAcked},
		{StateSubmitted, StateNeedsReview},
		{State("BOGUS"), StateQueued},
	}
	for _, tc := range forbidden {
		got, err := Transition(tc.from, tc.to)
		if !errors.Is(err, ErrIllegalTransition) {
			t.Errorf("%s → %s should be illegal, got err %v", tc.from, tc.to, err)
		}
		if got != tc.from {
			t.Errorf("%s → %s: state must not move on illegal transition, got %s", tc.from, tc.to, got)
		}
	}
}

func TestBackoff(t *testing.T) {
	want := []time.Duration{
		15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute,
		4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 32 * time.Minute,
	}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("attempt %d: got %s want %s", i+1, got, w)
		}
	}
	if Backoff(0) != 15*time.Second || Backoff(99) != 32*time.Minute {
		t.Error("backoff must clamp out-of-range attempts")
	}
}

func TestResolve(t *testing.T) {
	if o := Resolve(1, nil); o.Next != StateAcked {
		t.Errorf("success → %s", o.Next)
	}
	o := Resolve(1, &TransientError{Code: "upstream_5xx"})
	if o.Next != StateQueued || o.RetryIn != 15*time.Second {
		t.Errorf("retryable attempt 1 → %+v", o)
	}
	o = Resolve(3, errors.New("some unknown vendor error"))
	if o.Next != StateQueued || o.RetryIn != time.Minute {
		t.Errorf("unknown error is retryable: %+v", o)
	}
	o = Resolve(MaxAttempts, context.DeadlineExceeded)
	if o.Next != StateFailedTerminal {
		t.Errorf("exhausted attempts → %s", o.Next)
	}
	o = Resolve(1, &ValidationError{Code: "invalid_item_code"})
	if o.Next != StateFailedTerminal || o.RetryIn != 0 {
		t.Errorf("validation error is terminal: %+v", o)
	}
	o = Resolve(1, &ConfigError{Code: "invalid_api_key"})
	if o.Next != StateFailedTerminal {
		t.Errorf("config error is terminal: %+v", o)
	}
}

func TestClassifyAndErrorCode(t *testing.T) {
	cases := []struct {
		err   error
		class Class
		code  string
	}{
		{nil, ClassOK, ""},
		{&ValidationError{Code: "buyer_pin_invalid"}, ClassTerminal, "buyer_pin_invalid"},
		{&ConfigError{Code: "device_not_registered"}, ClassTerminal, "device_not_registered"},
		{&TransientError{Code: "rate_limited"}, ClassRetryable, "rate_limited"},
		{context.DeadlineExceeded, ClassRetryable, "timeout"},
		{errors.New("boom"), ClassRetryable, "unknown"},
	}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.class {
			t.Errorf("Classify(%v) = %s want %s", tc.err, got, tc.class)
		}
		if got := ErrorCode(tc.err); got != tc.code {
			t.Errorf("ErrorCode(%v) = %q want %q", tc.err, got, tc.code)
		}
	}
}

func TestLineTax(t *testing.T) {
	cases := []struct {
		total, rate, want int64
	}{
		{240000, 1600, 33103}, // KES 2 400 incl. 16 % → VAT 331.03
		{116000, 1600, 16000}, // exact
		{100, 1600, 14},       // 13.79 → 14
		{108, 800, 8},         // exact
		{0, 1600, 0},
		{240000, 0, 0},
		{-116000, 1600, -16000}, // credit note
	}
	for _, tc := range cases {
		if got := LineTax(tc.total, tc.rate); got != tc.want {
			t.Errorf("LineTax(%d, %d) = %d want %d", tc.total, tc.rate, got, tc.want)
		}
	}
	// half-to-even: 25 / 2 → 12, 35 / 2 → 18
	if roundHalfEvenInts(25, 2) != 12 || roundHalfEvenInts(35, 2) != 18 {
		t.Error("banker's rounding broken")
	}
}

func roundHalfEvenInts(num, den int64) int64 {
	return roundHalfEven(big.NewInt(num), big.NewInt(den))
}

func TestTotalsAndCategories(t *testing.T) {
	lines := []Line{
		{LineTotalCents: 240000, LineTaxCents: LineTax(240000, 1600)},
		{LineTotalCents: 5000, LineTaxCents: 0},
	}
	sub, tax, total := Totals(lines)
	if total != 245000 || tax != 33103 || sub != 211897 {
		t.Errorf("totals = %d %d %d", sub, tax, total)
	}
	for _, c := range []string{"A", "B", "C", "D", "E"} {
		if _, err := ParseTaxCategory(c); err != nil {
			t.Errorf("%s should parse", c)
		}
	}
	if _, err := ParseTaxCategory("Z"); err == nil {
		t.Error("Z should not parse")
	}
	if DefaultTaxCategory(true) != TaxStandard || DefaultTaxCategory(false) != TaxNonVAT {
		t.Error("default category")
	}
	if TaxStandard.RateBP() != 1600 || TaxReduced.RateBP() != 800 || TaxExempt.RateBP() != 0 {
		t.Error("rates")
	}
}

func TestReceiptCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		c, err := NewReceiptCode()
		if err != nil || len(c) != ReceiptCodeLen {
			t.Fatalf("code %q err %v", c, err)
		}
		if _, err := NormaliseReceiptCode(c); err != nil {
			t.Fatalf("generated code %q must normalise: %v", c, err)
		}
		seen[c] = true
	}
	if len(seen) < 495 {
		t.Fatalf("too many collisions in 500 codes: %d unique", len(seen))
	}
	got, err := NormaliseReceiptCode(" 7kq2mo ")
	if err != nil || got != "7KQ2M0" {
		t.Errorf("normalise: %q %v", got, err)
	}
	if _, err := NormaliseReceiptCode("7KQ2"); err == nil {
		t.Error("short code must fail")
	}
}
