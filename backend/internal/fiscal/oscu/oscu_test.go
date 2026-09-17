package oscu_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/fiscal/oscu"
	"github.com/exoin/ciftpay/internal/fiscal/providertest"
)

// fakeKRA is an httptest server that speaks just enough of the OSCU gateway
// shape (OAuth token, selectInitOsdcInfo, insertTrnsSalesReq,
// selectItemClsCodeList) for the shared contract suite, without any real
// network access.
type fakeKRA struct {
	mu       sync.Mutex
	devices  map[string]string // "tin|bhfId|dvcSrlNo" -> cmcKey
	receipts map[string]bool   // dedupe by invcNo, mirrors a real device's own idempotency
}

func newFakeKRA() *fakeKRA {
	return &fakeKRA{devices: map[string]string{}, receipts: map[string]bool{}}
}

func (f *fakeKRA) server(t *testing.T, key, secret string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/token/generate", func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != key || p != secret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-" + shortHash(u+p), "token_type": "Bearer", "expires_in": 3600})
	})

	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer fake-") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/etims-api/selectInitOsdcInfo", auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ TIN, BhfID, DvcSrlNo string }
		_ = json.NewDecoder(r.Body).Decode(&struct {
			TIN      *string `json:"tin"`
			BhfID    *string `json:"bhfId"`
			DvcSrlNo *string `json:"dvcSrlNo"`
		}{&in.TIN, &in.BhfID, &in.DvcSrlNo})
		if in.TIN == "" || in.DvcSrlNo == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"resultCd": "902", "resultMsg": "missing fields"})
			return
		}
		cmcKey := "CMC-" + shortHash(in.TIN+in.BhfID+in.DvcSrlNo)[:16]
		f.mu.Lock()
		f.devices[in.TIN+"|"+in.BhfID+"|"+in.DvcSrlNo] = cmcKey
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resultCd": "000", "resultMsg": "Successful",
			"data": map[string]any{"info": map[string]any{
				"tin": in.TIN, "taxprNm": "Test Trader", "bhfId": in.BhfID, "bhfNm": "Head Office",
				"dvcId": "DVC1", "sdcId": "SDC1", "mrcNo": "MRC1", "cmcKey": cmcKey,
			}},
		})
	}))

	mux.HandleFunc("/etims-api/insertTrnsSalesReq", auth(func(w http.ResponseWriter, r *http.Request) {
		in, _ := decodeSale(r)
		if r.Header.Get("cmcKey") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"resultCd": "905", "resultMsg": "device not registered"})
			return
		}
		if in.OrgInvcNo == "" && r.URL.Query().Get("cn") == "1" {
			_ = json.NewEncoder(w).Encode(map[string]any{"resultCd": "904", "resultMsg": "original invoice required"})
			return
		}
		f.mu.Lock()
		dup := f.receipts[in.InvcNo]
		f.receipts[in.InvcNo] = true
		f.mu.Unlock()
		rcpt := "KRACU01" + shortHash(in.InvcNo)[:10]
		if dup {
			// Real KRA has no client-side idempotency; the adapter's own cache
			// (Provider.acks) is what makes SubmitInvoice idempotent, so this
			// branch never actually gets hit by the contract suite. Kept as
			// documentation of that gap.
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resultCd": "000", "resultMsg": "Successful",
			"data": map[string]any{
				"rcptNo": rcpt, "rcptSign": strings.ToUpper(shortHash(in.InvcNo)[10:26]),
				"qrCodeUrl": "https://etims-sbx.kra.go.ke/common/link/etims/receipt/indexEtimsReceiptData?Data=" + rcpt,
			},
		})
	}))

	mux.HandleFunc("/etims-api/selectItemClsCodeList", auth(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"resultCd": "000", "resultMsg": "Successful",
			"data": map[string]any{"itemClsList": []map[string]string{
				{"itemClsCd": "99000000", "itemClsNm": "General retail sale", "taxTyCd": "B"},
				{"itemClsCd": "50000000", "itemClsNm": "Food, beverage and tobacco products", "taxTyCd": "B"},
			}},
		})
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func decodeSale(r *http.Request) (struct {
	TIN, BhfID, InvcNo, OrgInvcNo string
	TotAmt                        int64
}, error) {
	var in struct {
		TIN, BhfID, InvcNo, OrgInvcNo string
		TotAmt                        int64
	}
	err := json.NewDecoder(r.Body).Decode(&struct {
		TIN       *string `json:"tin"`
		BhfID     *string `json:"bhfId"`
		InvcNo    *string `json:"invcNo"`
		OrgInvcNo *string `json:"orgInvcNo"`
		TotAmt    *int64  `json:"totAmt"`
	}{&in.TIN, &in.BhfID, &in.InvcNo, &in.OrgInvcNo, &in.TotAmt})
	return in, err
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// registeredProvider returns a fresh oscu.Provider whose one device (the
// contract suite's org "org-1" seller PIN "P051234567A", branch "00") is
// already initialised, since providertest.ValidInvoice always uses that PIN.
func registeredProvider(t *testing.T, srv *httptest.Server) fiscal.Provider {
	t.Helper()
	p, err := oscu.New(oscu.Config{BaseURL: srv.URL, ConsumerKey: "key", ConsumerSecret: "secret", DeviceSerial: "SN0001", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.RegisterDevice(context.Background(), fiscal.OrgFiscalProfile{OrgID: "org-1", Name: "Test", KRAPIN: "P051234567A", VATRegistered: true, BranchID: "00", DeviceSerial: "SN0001"}); err != nil {
		t.Fatal(err)
	}
	return p
}

// deviceProfileProvider wraps a Provider so every Invoice/CreditNote it is
// given carries the DeviceProfile a real Submitter would have read back from
// orgs.fiscal_profile after RegisterDevice. providertest itself has no
// notion of that plumbing (it is internal to internal/fiscal/submitter.go),
// so the adapter test fills it in here.
type deviceProfileProvider struct {
	fiscal.Provider
	profile json.RawMessage
}

func (p deviceProfileProvider) SubmitInvoice(ctx context.Context, inv fiscal.Invoice) (fiscal.Ack, error) {
	inv.DeviceProfile = p.profile
	return p.Provider.SubmitInvoice(ctx, inv)
}

func (p deviceProfileProvider) SubmitCreditNote(ctx context.Context, cn fiscal.CreditNote) (fiscal.Ack, error) {
	cn.DeviceProfile = p.profile
	return p.Provider.SubmitCreditNote(ctx, cn)
}

func TestContract(t *testing.T) {
	f := newFakeKRA()
	srv := f.server(t, "key", "secret")
	providertest.Run(t, func(t *testing.T) fiscal.Provider {
		p, err := oscu.New(oscu.Config{BaseURL: srv.URL, ConsumerKey: "key", ConsumerSecret: "secret", DeviceSerial: "SN0001", Timeout: 5 * time.Second})
		if err != nil {
			t.Fatal(err)
		}
		ref, err := p.RegisterDevice(context.Background(), fiscal.OrgFiscalProfile{OrgID: "org-1", Name: "Test", KRAPIN: "P051234567A", VATRegistered: true, BranchID: "00", DeviceSerial: "SN0001"})
		if err != nil {
			t.Fatal(err)
		}
		return deviceProfileProvider{Provider: p, profile: ref.Raw}
	})
}

func TestRegisterDevice_RequiresSerial(t *testing.T) {
	f := newFakeKRA()
	srv := f.server(t, "key", "secret")
	p, err := oscu.New(oscu.Config{BaseURL: srv.URL, ConsumerKey: "key", ConsumerSecret: "secret", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.RegisterDevice(context.Background(), fiscal.OrgFiscalProfile{OrgID: "org-x", KRAPIN: "P051234567A"})
	var ve *fiscal.ValidationError
	if !errors.As(err, &ve) || ve.Code != "device_serial_required" {
		t.Fatalf("got %v", err)
	}
}

func TestSubmitInvoice_WithoutRegistration(t *testing.T) {
	f := newFakeKRA()
	srv := f.server(t, "key", "secret")
	p, err := oscu.New(oscu.Config{BaseURL: srv.URL, ConsumerKey: "key", ConsumerSecret: "secret", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.SubmitInvoice(context.Background(), providertest.ValidInvoice("no-device"))
	var ce *fiscal.ConfigError
	if !errors.As(err, &ce) || ce.Code != "device_not_registered" {
		t.Fatalf("got %v", err)
	}
}

func TestClassification(t *testing.T) {
	ctx := context.Background()

	t.Run("bad consumer credentials is a config error", func(t *testing.T) {
		f := newFakeKRA()
		srv := f.server(t, "right", "secret")
		p, _ := oscu.New(oscu.Config{BaseURL: srv.URL, ConsumerKey: "wrong", ConsumerSecret: "secret", Timeout: 5 * time.Second})
		err := p.Health(ctx)
		var ce *fiscal.ConfigError
		if !errors.As(err, &ce) || ce.Code != "invalid_api_key" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("5xx is retryable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
		defer srv.Close()
		p, _ := oscu.New(oscu.Config{BaseURL: srv.URL, ConsumerKey: "k", ConsumerSecret: "s", Timeout: 5 * time.Second})
		err := p.Health(ctx)
		if fiscal.Classify(err) != fiscal.ClassRetryable {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("connection refused is retryable", func(t *testing.T) {
		p, _ := oscu.New(oscu.Config{BaseURL: "http://127.0.0.1:1", ConsumerKey: "k", ConsumerSecret: "s", Timeout: time.Second})
		err := p.Health(ctx)
		if fiscal.Classify(err) != fiscal.ClassRetryable {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("missing config is terminal", func(t *testing.T) {
		if _, err := oscu.New(oscu.Config{}); fiscal.Classify(err) != fiscal.ClassTerminal {
			t.Fatalf("got %v", err)
		}
	})
}

// TestDNSResolverOverride only asserts that a bogus resolver address makes
// the client fail closed (network error) rather than silently falling back
// to the system resolver, i.e. that Config.DNSResolver is actually wired
// into the transport and not ignored. It does not require real network
// access itself: 127.0.0.1:1 refuses the UDP "connection" immediately.
func TestDNSResolverOverride(t *testing.T) {
	p, err := oscu.NewClient(oscu.Config{BaseURL: "https://sbx.kra.go.ke", ConsumerKey: "k", ConsumerSecret: "s", DNSResolver: "127.0.0.1:1", Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.Token(ctx); err == nil {
		t.Fatal("expected the bogus resolver to make the lookup fail")
	}
}
