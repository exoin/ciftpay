package mpesa_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ciftpay/ciftpay/internal/mpesa"
	"github.com/ciftpay/ciftpay/internal/mpesa/mpesatest"
	"github.com/ciftpay/ciftpay/internal/platform/config"
)

func fakeClient(t *testing.T) (*mpesa.Client, *mpesatest.Server) {
	t.Helper()
	fake, srv := mpesatest.Start()
	t.Cleanup(srv.Close)
	c := mpesa.NewClient(config.Daraja{
		Env: "sandbox", BaseURL: srv.URL, ConsumerKey: "key", ConsumerSecret: "secret", Shortcode: "174379", WebhookToken: "tok",
	})
	return c, fake
}

func TestClient_OAuthAndRegisterURL(t *testing.T) {
	ctx := context.Background()
	c, fake := fakeClient(t)
	if !c.Configured() {
		t.Fatal("client with key+secret must report configured")
	}
	tok, err := c.Token(ctx)
	if err != nil || !strings.HasPrefix(tok, "fake-token-") {
		t.Fatalf("Token = %q, %v", tok, err)
	}
	again, _ := c.Token(ctx)
	if again != tok {
		t.Fatal("token should be cached until close to expiry")
	}

	if err := c.RegisterC2BURLs(ctx, "600123", "https://api.example.com"); err != nil {
		t.Fatal(err)
	}
	reg, ok := fake.Registration("600123")
	if !ok {
		t.Fatal("RegisterURL not recorded")
	}
	if reg.ConfirmationURL != "https://api.example.com/webhooks/daraja/c2b/confirmation/tok" || reg.ValidationURL != "https://api.example.com/webhooks/daraja/c2b/validation/tok" {
		t.Fatalf("registered %+v", reg)
	}
	if reg.ResponseType != "Completed" {
		t.Fatalf("ResponseType = %q", reg.ResponseType)
	}
}

func TestClient_SimulateDeliversConfirmation(t *testing.T) {
	ctx := context.Background()
	c, fake := fakeClient(t)

	// A stand-in for the api's webhook endpoint.
	var got mpesa.C2BPayload
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/webhooks/mpesa/c2b/confirmation/tok") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(mpesa.Accepted)
	}))
	defer webhook.Close()

	if err := c.RegisterC2BURLs(ctx, "600123", webhook.URL); err != nil {
		t.Fatal(err)
	}
	out, err := c.SimulateC2B(ctx, "600123", "254140994513", 100, "CIFTPAY")
	if err != nil {
		t.Fatal(err)
	}
	if out["ResponseCode"] != "0" {
		t.Fatalf("simulate response %+v", out)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("delivered payload invalid: %v (%+v)", err, got)
	}
	if got.BusinessShortCode != "600123" || got.MSISDN != "254140994513" || got.TransAmount != "1.00" || got.BillRefNumber != "CIFTPAY" {
		t.Fatalf("delivered %+v", got)
	}
	amount, _ := mpesa.ParseAmount(got.TransAmount)
	if amount != 100 {
		t.Fatalf("amount cents = %d", amount)
	}
	sims := fake.Simulations()
	if len(sims) != 1 || sims[0].TransID != got.TransID || sims[0].DeliveryError != "" {
		t.Fatalf("simulations = %+v", sims)
	}
}

func TestClient_SimulateWithoutRegistrationFails(t *testing.T) {
	c, _ := fakeClient(t)
	if _, err := c.SimulateC2B(context.Background(), "999999", "254140994513", 100, ""); err == nil {
		t.Fatal("simulate must fail when no ConfirmationURL is registered")
	}
}

func TestClient_SimulateRefusedInProduction(t *testing.T) {
	c := mpesa.NewClient(config.Daraja{Env: "production", BaseURL: "http://127.0.0.1:1", ConsumerKey: "k", ConsumerSecret: "s"})
	if _, err := c.SimulateC2B(context.Background(), "600123", "254140994513", 100, ""); err == nil {
		t.Fatal("simulate must be refused in production")
	}
}
