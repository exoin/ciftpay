package mock_test

import (
	"context"
	"testing"

	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/fiscal/mock"
	"github.com/ciftpay/ciftpay/internal/fiscal/providertest"
)

func TestContract(t *testing.T) {
	providertest.Run(t, func(*testing.T) fiscal.Provider { return mock.New(mock.FailNone) })
}

func TestFailureInjection(t *testing.T) {
	ctx := context.Background()

	t.Run("retryable then recovers", func(t *testing.T) {
		p := mock.New(mock.FailRetryable)
		p.FailTimes = 2
		inv := providertest.ValidInvoice("inv-flaky")
		for attempt := 1; attempt <= 2; attempt++ {
			_, err := p.SubmitInvoice(ctx, inv)
			if fiscal.Classify(err) != fiscal.ClassRetryable {
				t.Fatalf("attempt %d: want retryable, got %v", attempt, err)
			}
			if out := fiscal.Resolve(attempt, err); out.Next != fiscal.StateQueued {
				t.Fatalf("attempt %d: worker would move to %s", attempt, out.Next)
			}
		}
		ack, err := p.SubmitInvoice(ctx, inv)
		if err != nil || ack.KRAInvoiceNo == "" {
			t.Fatalf("third attempt should succeed: %v", err)
		}
		if p.Calls != 3 {
			t.Fatalf("calls = %d", p.Calls)
		}
	})

	t.Run("terminal goes to needs review", func(t *testing.T) {
		p := mock.New(mock.FailTerminal)
		_, err := p.SubmitInvoice(ctx, providertest.ValidInvoice("inv-doomed"))
		if fiscal.Classify(err) != fiscal.ClassTerminal {
			t.Fatalf("want terminal, got %v", err)
		}
		out := fiscal.Resolve(1, err)
		if out.Next != fiscal.StateFailedTerminal {
			t.Fatalf("worker would move to %s", out.Next)
		}
		if next, err := fiscal.Transition(out.Next, fiscal.StateNeedsReview); err != nil || next != fiscal.StateNeedsReview {
			t.Fatalf("FAILED_TERMINAL → NEEDS_REVIEW: %v", err)
		}
	})

	t.Run("always retryable exhausts attempts", func(t *testing.T) {
		p := mock.New(mock.FailRetryable)
		inv := providertest.ValidInvoice("inv-outage")
		var out fiscal.Outcome
		for attempt := 1; attempt <= fiscal.MaxAttempts; attempt++ {
			_, err := p.SubmitInvoice(ctx, inv)
			out = fiscal.Resolve(attempt, err)
			if attempt < fiscal.MaxAttempts && out.Next != fiscal.StateQueued {
				t.Fatalf("attempt %d: %s", attempt, out.Next)
			}
		}
		if out.Next != fiscal.StateFailedTerminal {
			t.Fatalf("after %d attempts want FAILED_TERMINAL, got %s", fiscal.MaxAttempts, out.Next)
		}
	})

	t.Run("mode can be switched off", func(t *testing.T) {
		p := mock.New(mock.FailRetryable)
		inv := providertest.ValidInvoice("inv-switch")
		if _, err := p.SubmitInvoice(ctx, inv); err == nil {
			t.Fatal("expected injected failure")
		}
		p.SetFailMode(mock.FailNone)
		if _, err := p.SubmitInvoice(ctx, inv); err != nil {
			t.Fatalf("after switching off: %v", err)
		}
	})
}

func TestParseFailMode(t *testing.T) {
	for _, ok := range []string{"", "none", "retryable", "terminal"} {
		if _, err := mock.ParseFailMode(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	if _, err := mock.ParseFailMode("sometimes"); err == nil {
		t.Error("bogus mode accepted")
	}
}

func TestLookupFilters(t *testing.T) {
	p := mock.New(mock.FailNone)
	codes, _ := p.LookupItemCodes(context.Background(), "vegetables")
	if len(codes) != 1 || codes[0].Code != "50401500" {
		t.Fatalf("got %+v", codes)
	}
	codes, _ = p.LookupItemCodes(context.Background(), "5040")
	if len(codes) != 1 {
		t.Fatalf("prefix search got %+v", codes)
	}
}
