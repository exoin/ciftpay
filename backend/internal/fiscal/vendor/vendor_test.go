package vendor_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/fiscal/mock"
	"github.com/exoin/ciftpay/internal/fiscal/providertest"
	"github.com/exoin/ciftpay/internal/fiscal/vendor"
)

// fakeIntegrator is an httptest server that behaves like a KRA-approved
// integrator by delegating to the mock adapter. It lets the vendor adapter
// pass the same contract suite without network access.
func fakeIntegrator(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	backend := mock.New(mock.FailNone)
	mux := http.NewServeMux()

	writeErr := func(w http.ResponseWriter, status int, code, field, msg string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "field": field, "message": msg}})
	}
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+apiKey {
				writeErr(w, http.StatusUnauthorized, "unauthorised", "", "bad key")
				return
			}
			next(w, r)
		}
	}
	mapErr := func(w http.ResponseWriter, err error) {
		var ve *fiscal.ValidationError
		if errors.As(err, &ve) {
			writeErr(w, http.StatusUnprocessableEntity, ve.Code, ve.Field, ve.Message)
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal", "", err.Error())
	}

	mux.HandleFunc("/v1/health", auth(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("/v1/devices", auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ClientRef string `json:"clientRef"`
			PIN       string `json:"pin"`
			Name      string `json:"name"`
			BranchID  string `json:"branchId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		ref, err := backend.RegisterDevice(r.Context(), fiscal.OrgFiscalProfile{OrgID: in.ClientRef, KRAPIN: in.PIN, Name: in.Name, BranchID: in.BranchID})
		if err != nil {
			mapErr(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"deviceId": ref.DeviceID, "branchId": ref.BranchID})
	}))
	mux.HandleFunc("/v1/invoices", auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ClientRef     string `json:"clientRef"`
			Kind          string `json:"kind"`
			OriginalKRANo string `json:"originalInvoiceNo"`
			SellerPIN     string `json:"sellerPin"`
			BuyerPIN      string `json:"buyerPin"`
			TotalCents    int64  `json:"totalCents"`
			Lines         []struct {
				ItemCode  string `json:"itemCode"`
				TaxType   string `json:"taxType"`
				LineTotal int64  `json:"lineTotalCents"`
				LineTax   int64  `json:"lineTaxCents"`
			} `json:"lines"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "bad_json", "", err.Error())
			return
		}
		lines := make([]fiscal.Line, 0, len(in.Lines))
		for _, l := range in.Lines {
			lines = append(lines, fiscal.Line{ItemCode: l.ItemCode, TaxCategory: fiscal.TaxCategory(l.TaxType), LineTotalCents: l.LineTotal, LineTaxCents: l.LineTax})
		}
		var ack fiscal.Ack
		var err error
		if in.Kind == "CREDIT_NOTE" {
			ack, err = backend.SubmitCreditNote(r.Context(), fiscal.CreditNote{ID: in.ClientRef, OriginalKRANo: in.OriginalKRANo, Lines: lines, TotalCents: in.TotalCents})
		} else {
			ack, err = backend.SubmitInvoice(r.Context(), fiscal.Invoice{ID: in.ClientRef, SellerPIN: in.SellerPIN, BuyerPIN: in.BuyerPIN, Lines: lines, TotalCents: in.TotalCents})
		}
		if err != nil {
			mapErr(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"invoiceNo": ack.KRAInvoiceNo, "signature": ack.Signature, "qrPayload": ack.QRPayload,
			"receivedAt": ack.ReceivedAt.Format(time.RFC3339),
		})
	}))
	mux.HandleFunc("/v1/taxpayers/", auth(func(w http.ResponseWriter, r *http.Request) {
		pin := strings.TrimPrefix(r.URL.Path, "/v1/taxpayers/")
		tp, err := backend.LookupPIN(r.Context(), pin)
		switch {
		case errors.Is(err, fiscal.ErrPINUnknown):
			writeErr(w, http.StatusNotFound, "not_found", "pin", "no such taxpayer")
		case err != nil:
			mapErr(w, err)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"pin": tp.PIN, "name": tp.Name, "vatRegistered": tp.VATRegistered})
		}
	}))
	mux.HandleFunc("/v1/item-codes", auth(func(w http.ResponseWriter, r *http.Request) {
		codes, _ := backend.LookupItemCodes(r.Context(), r.URL.Query().Get("q"))
		out := make([]map[string]string, 0, len(codes))
		for _, c := range codes {
			out = append(out, map[string]string{"code": c.Code, "description": c.Description, "taxType": string(c.TaxCategory)})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestContract(t *testing.T) {
	srv := fakeIntegrator(t, "test-key")
	providertest.Run(t, func(t *testing.T) fiscal.Provider {
		p, err := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "test-key", Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		return p
	})
}

func TestClassification(t *testing.T) {
	ctx := context.Background()

	t.Run("bad api key is a config error", func(t *testing.T) {
		srv := fakeIntegrator(t, "right")
		p, _ := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "wrong"})
		_, err := p.SubmitInvoice(ctx, providertest.ValidInvoice("x"))
		var ce *fiscal.ConfigError
		if !errors.As(err, &ce) || ce.Code != "invalid_api_key" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("5xx and 429 are retryable", func(t *testing.T) {
		var status int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		defer srv.Close()
		p, _ := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "k"})
		for _, status = range []int{500, 502, 503, 429} {
			_, err := p.SubmitInvoice(ctx, providertest.ValidInvoice("x"))
			if fiscal.Classify(err) != fiscal.ClassRetryable {
				t.Fatalf("status %d: got %v", status, err)
			}
		}
	})

	t.Run("connection refused is retryable", func(t *testing.T) {
		p, _ := vendor.New(vendor.Config{BaseURL: "http://127.0.0.1:1", APIKey: "k", Timeout: time.Second})
		err := p.Health(ctx)
		if fiscal.Classify(err) != fiscal.ClassRetryable || fiscal.ErrorCode(err) != "network" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("vendor validation body becomes ValidationError", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_item_code","field":"lines[0].itemCode","message":"unknown"}}`))
		}))
		defer srv.Close()
		p, _ := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "k"})
		_, err := p.SubmitInvoice(ctx, providertest.ValidInvoice("x"))
		var ve *fiscal.ValidationError
		if !errors.As(err, &ve) || ve.Code != "invalid_item_code" || !strings.Contains(ve.Field, "itemCode") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("missing config", func(t *testing.T) {
		if _, err := vendor.New(vendor.Config{}); fiscal.Classify(err) != fiscal.ClassTerminal {
			t.Fatalf("got %v", err)
		}
	})
}

func TestLookupPIN(t *testing.T) {
	ctx := context.Background()
	srv := fakeIntegrator(t, "k")
	p, err := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "k", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var lookup fiscal.PINLookup = p

	t.Run("known pin", func(t *testing.T) {
		tp, err := lookup.LookupPIN(ctx, "A012345678Z")
		if err != nil || tp.PIN != "A012345678Z" || tp.Name == "" || !tp.VATRegistered {
			t.Fatalf("got %+v, %v", tp, err)
		}
	})
	t.Run("unknown pin", func(t *testing.T) {
		if _, err := lookup.LookupPIN(ctx, mock.UnknownPIN); !errors.Is(err, fiscal.ErrPINUnknown) {
			t.Fatalf("got %v, want ErrPINUnknown", err)
		}
	})
	t.Run("bare 404 is unknown too", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }))
		defer srv.Close()
		p, _ := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "k"})
		if _, err := p.LookupPIN(ctx, "A012345678Z"); !errors.Is(err, fiscal.ErrPINUnknown) {
			t.Fatalf("got %v, want ErrPINUnknown", err)
		}
	})
	t.Run("integrator down is unavailable", func(t *testing.T) {
		p, _ := vendor.New(vendor.Config{BaseURL: "http://127.0.0.1:1", APIKey: "k", Timeout: time.Second})
		if _, err := p.LookupPIN(ctx, "A012345678Z"); !errors.Is(err, fiscal.ErrLookupUnavailable) {
			t.Fatalf("got %v, want ErrLookupUnavailable", err)
		}
	})
	t.Run("bad api key is unavailable, not unknown", func(t *testing.T) {
		p, _ := vendor.New(vendor.Config{BaseURL: srv.URL, APIKey: "wrong"})
		if _, err := p.LookupPIN(ctx, "A012345678Z"); !errors.Is(err, fiscal.ErrLookupUnavailable) {
			t.Fatalf("got %v, want ErrLookupUnavailable", err)
		}
	})
}
