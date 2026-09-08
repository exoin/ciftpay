package mpesa

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ciftpay/ciftpay/internal/ledger"
)

// fakeIngester records inputs and enforces TransID idempotency the way the
// webhook_events UNIQUE constraint does in the real ledger.Service.
type fakeIngester struct {
	mu   sync.Mutex
	seen map[string]bool
	c2b  []ledger.C2BInput
	stk  []ledger.STKInput
	err  error
}

func (f *fakeIngester) IngestC2B(_ context.Context, in ledger.C2BInput) (ledger.C2BResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return ledger.C2BResult{}, f.err
	}
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	if f.seen[in.TransID] {
		return ledger.C2BResult{Duplicate: true}, nil
	}
	f.seen[in.TransID] = true
	f.c2b = append(f.c2b, in)
	return ledger.C2BResult{Status: ledger.StatusCashSale, Rule: ledger.RuleAutoInvoice}, nil
}

func (f *fakeIngester) IngestSTK(_ context.Context, in ledger.STKInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stk = append(f.stk, in)
	return f.err
}

func newServer(t *testing.T, ing Ingester) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	h := &Webhooks{Token: "t0k3n", Ingest: ing, Log: slog.New(slog.NewTextHandler(os.Stderr, nil))}
	r.Route("/webhooks", h.Mount)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func post(t *testing.T, url string, body []byte) (int, DarajaAck) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var ack DarajaAck
	_ = json.NewDecoder(resp.Body).Decode(&ack)
	return resp.StatusCode, ack
}

func TestConfirmation_DuplicateTransIDIngestedOnce(t *testing.T) {
	ing := &fakeIngester{}
	srv := newServer(t, ing)
	body := fixture(t, "c2b_confirmation.json")
	url := srv.URL + "/webhooks/daraja/c2b/confirmation/t0k3n"

	for i := 0; i < 3; i++ {
		status, ack := post(t, url, body)
		if status != http.StatusOK || ack.ResultCode != 0 {
			t.Fatalf("replay %d: status=%d ack=%+v", i, status, ack)
		}
	}
	if len(ing.c2b) != 1 {
		t.Fatalf("want exactly one ingested payment, got %d", len(ing.c2b))
	}
	in := ing.c2b[0]
	if in.TransID != "SLJ7X2K91Q" || in.ShortCode != "600123" || in.AmountCents != 240000 || in.Kind != "c2b_confirmation" {
		t.Errorf("normalised input wrong: %+v", in)
	}
	if in.PayerName != "MARY WANJIKU KAMAU" {
		t.Errorf("payer name = %q", in.PayerName)
	}
	if in.PaidAt.IsZero() {
		t.Error("paid_at not parsed")
	}
}

func TestConfirmation_ReversalIsFlagged(t *testing.T) {
	ing := &fakeIngester{}
	srv := newServer(t, ing)
	status, _ := post(t, srv.URL+"/webhooks/daraja/c2b/confirmation/t0k3n", fixture(t, "reversal.json"))
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if len(ing.c2b) != 1 || ing.c2b[0].Kind != "reversal" || ing.c2b[0].OriginalID != "SLJ7X2K91Q" {
		t.Fatalf("reversal not normalised: %+v", ing.c2b)
	}
}

func TestConfirmation_BadTokenRejected(t *testing.T) {
	ing := &fakeIngester{}
	srv := newServer(t, ing)
	status, _ := post(t, srv.URL+"/webhooks/daraja/c2b/confirmation/wrong", fixture(t, "c2b_confirmation.json"))
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	if len(ing.c2b) != 0 {
		t.Fatal("payload must not reach the ledger with a bad token")
	}
}

func TestConfirmation_InvalidBodyIs400(t *testing.T) {
	srv := newServer(t, &fakeIngester{})
	status, ack := post(t, srv.URL+"/webhooks/daraja/c2b/confirmation/t0k3n", []byte(`{"TransID":"X"}`))
	if status != http.StatusBadRequest || ack.ResultCode != 1 {
		t.Fatalf("status=%d ack=%+v", status, ack)
	}
}

func TestConfirmation_IngestErrorStill200(t *testing.T) {
	// Daraja retries on non-200; the raw event is already stored, so the api
	// must acknowledge and let the reconcile job finish the work.
	ing := &fakeIngester{err: context.DeadlineExceeded}
	srv := newServer(t, ing)
	status, ack := post(t, srv.URL+"/webhooks/daraja/c2b/confirmation/t0k3n", fixture(t, "c2b_confirmation.json"))
	if status != http.StatusOK || ack.ResultCode != 0 {
		t.Fatalf("status=%d ack=%+v", status, ack)
	}
}

func TestSTKCallback_Flattened(t *testing.T) {
	ing := &fakeIngester{}
	srv := newServer(t, ing)
	status, _ := post(t, srv.URL+"/webhooks/daraja/stk/t0k3n", fixture(t, "stk_callback.json"))
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if len(ing.stk) != 1 {
		t.Fatalf("want one stk ingest, got %d", len(ing.stk))
	}
	in := ing.stk[0]
	if !in.Success || in.CheckoutRequestID != "ws_CO_02092026121712345" || in.ReceiptNo != "SLJ8A1B2C3" || in.AmountCents != 100 || in.MSISDN != "254708374149" {
		t.Errorf("flattened stk wrong: %+v", in)
	}
}

func TestValidation_AlwaysAccepts(t *testing.T) {
	srv := newServer(t, &fakeIngester{})
	status, ack := post(t, srv.URL+"/webhooks/daraja/c2b/validation/t0k3n", fixture(t, "c2b_confirmation.json"))
	if status != http.StatusOK || ack.ResultCode != 0 {
		t.Fatalf("validation must never bounce a payment: status=%d ack=%+v", status, ack)
	}
}
