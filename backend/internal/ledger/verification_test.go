package ledger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ciftpay/ciftpay/internal/ledger"
	"github.com/ciftpay/ciftpay/internal/mpesa"
	"github.com/ciftpay/ciftpay/internal/mpesa/mpesatest"
	"github.com/ciftpay/ciftpay/internal/platform/config"
	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/dbtest"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
	"github.com/ciftpay/ciftpay/internal/platform/jobs"
)

const (
	ownerMSISDN = "254140994513"
	otherMSISDN = "254708374149"
	webhookTok  = "test-token"
)

// rig wires a real ledger service + handler to the fake Daraja and an in-
// process webhook endpoint, exactly the loop a merchant goes through.
type rig struct {
	t      *testing.T
	d      *db.DB
	keys   *crypto.Keyring
	svc    *ledger.Service
	h      *ledger.Handler
	daraja *mpesa.Client
	fake   *mpesatest.Server
	api    *httptest.Server // ledger routes + webhook routes
}

func newRig(t *testing.T, darajaConfigured bool) *rig {
	t.Helper()
	d := dbtest.Open(t)
	keys, err := crypto.New(bytes.Repeat([]byte{9}, 32), "pepper")
	if err != nil {
		t.Fatal(err)
	}
	jc, err := jobs.NewInsertOnly(d.Pool)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := ledger.New(d, jc, keys, log)

	fake, fakeSrv := mpesatest.Start()
	t.Cleanup(fakeSrv.Close)
	dcfg := config.Daraja{Env: "sandbox", BaseURL: fakeSrv.URL, WebhookToken: webhookTok}
	if darajaConfigured {
		dcfg.ConsumerKey, dcfg.ConsumerSecret = "key", "secret"
	}
	daraja := mpesa.NewClient(dcfg)

	r := &rig{t: t, d: d, keys: keys, svc: svc, daraja: daraja, fake: fake}
	r.h = &ledger.Handler{S: svc, Keys: keys, Daraja: daraja, PublicBaseURL: "http://web.test"}

	mux := chi.NewRouter()
	mux.Route("/webhooks", func(mr chi.Router) {
		(&mpesa.Webhooks{Token: webhookTok, Ingest: svc, Log: log}).Mount(mr)
	})
	// Tenant routes: the test sets the principal through headers instead of a
	// session cookie so the handler can be exercised in isolation.
	mux.Group(func(mr chi.Router) {
		mr.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				orgID, _ := uuid.Parse(req.Header.Get("X-Test-Org"))
				userID, _ := uuid.Parse(req.Header.Get("X-Test-User"))
				next.ServeHTTP(w, req.WithContext(httpx.WithPrincipal(req.Context(), httpx.Principal{OrgID: orgID, UserID: userID, Role: "owner"})))
			})
		})
		r.h.Mount(mr)
	})
	r.api = httptest.NewServer(mux)
	t.Cleanup(r.api.Close)
	r.h.WebhookBaseURL = r.api.URL
	return r
}

type tenant struct {
	orgID  uuid.UUID
	userID uuid.UUID
}

func (r *rig) newTenant(name, pin, msisdn string) tenant {
	r.t.Helper()
	var tn tenant
	err := r.d.Unscoped(context.Background(), func(ctx context.Context, tx db.Tx) error {
		pinEnc, _ := r.keys.EncryptString(pin)
		o, err := tx.CreateOrg(ctx, gen.CreateOrgParams{Name: name, KraPinEnc: pinEnc, KraPinHash: r.keys.Hash(pin), Locale: "en"})
		if err != nil {
			return err
		}
		enc, _ := r.keys.EncryptString(msisdn)
		u, err := tx.CreateUser(ctx, gen.CreateUserParams{MsisdnEnc: enc, MsisdnHash: r.keys.Hash(msisdn), Locale: "en"})
		if err != nil {
			return err
		}
		if _, err := tx.CreateMembership(ctx, gen.CreateMembershipParams{OrgID: o.ID, UserID: u.ID, Role: "owner", IsDefault: true}); err != nil {
			return err
		}
		tn = tenant{orgID: o.ID, userID: u.ID}
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return tn
}

func (r *rig) call(tn tenant, method, path string, body any) (int, map[string]any) {
	r.t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, _ := http.NewRequestWithContext(context.Background(), method, r.api.URL+path, buf)
	req.Header.Set("X-Test-Org", tn.orgID.String())
	req.Header.Set("X-Test-User", tn.userID.String())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (r *rig) createShortcode(tn tenant, number string) (uuid.UUID, int, map[string]any) {
	r.t.Helper()
	status, out := r.call(tn, http.MethodPost, "/shortcodes", map[string]any{"kind": "till", "shortcode": number, "auto_invoice": false})
	id, _ := uuid.Parse(fmt.Sprint(out["id"]))
	return id, status, out
}

func (r *rig) count(tn tenant, table string) int64 {
	r.t.Helper()
	var n int64
	err := r.d.WithOrg(context.Background(), tn.orgID, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	})
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

func errCode(out map[string]any) string {
	e, _ := out["error"].(map[string]any)
	return fmt.Sprint(e["code"])
}

func TestVerification_OwnTillPaymentVerifiesWithoutPayment(t *testing.T) {
	r := newRig(t, true)
	tn := r.newTenant("Mama Njeri", "A012345678Z", ownerMSISDN)
	scID, status, _ := r.createShortcode(tn, "600123")
	if status != http.StatusCreated {
		t.Fatalf("create shortcode = %d", status)
	}

	// 1. Open the challenge.
	status, out := r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", map[string]any{})
	if status != http.StatusAccepted {
		t.Fatalf("verify = %d %v", status, out)
	}
	if out["status"] != "pending" || out["msisdn_masked"] != "2541•••••513" {
		t.Fatalf("verify body %v", out)
	}
	pay, _ := out["pay"].(map[string]any)
	if pay["kind"] != "till" || pay["shortcode"] != "600123" || pay["amount_cents"] != float64(100) || pay["account_ref"] != "CIFTPAY" {
		t.Fatalf("pay instructions %v", pay)
	}
	if _, ok := out["expires_at"]; !ok {
		t.Fatal("expires_at missing")
	}
	// RegisterURL was attempted against the fake with the webhook base URL.
	reg, ok := r.fake.Registration("600123")
	if !ok || !strings.HasPrefix(reg.ConfirmationURL, r.api.URL+"/webhooks/mpesa/c2b/confirmation/") {
		t.Fatalf("registration = %+v, %v", reg, ok)
	}
	status, sc := r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil)
	if status != http.StatusOK || sc["verified"] != false || sc["c2b_urls_registered_at"] == nil {
		t.Fatalf("GET after verify = %d %v", status, sc)
	}
	if v, _ := sc["verification"].(map[string]any); v["status"] != "pending" {
		t.Fatalf("verification state %v", sc["verification"])
	}

	// 2. The merchant pays KES 1 from their own phone.
	if _, err := r.daraja.SimulateC2B(context.Background(), "600123", ownerMSISDN, 100, "CIFTPAY"); err != nil {
		t.Fatal(err)
	}

	// 3. Verified, no money recorded.
	status, sc = r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil)
	if status != http.StatusOK || sc["verified"] != true || sc["verified_at"] == nil {
		t.Fatalf("GET after payment = %d %v", status, sc)
	}
	if v, _ := sc["verification"].(map[string]any); v["status"] != "verified" {
		t.Fatalf("verification state %v", sc["verification"])
	}
	for _, table := range []string{"payments", "sales", "invoices"} {
		if n := r.count(tn, table); n != 0 {
			t.Errorf("%s rows = %d, want 0", table, n)
		}
	}
	var processed int64
	var transID *string
	_ = r.d.Unscoped(context.Background(), func(ctx context.Context, tx db.Tx) error {
		if err := tx.Tx.QueryRow(ctx, "SELECT count(*) FROM webhook_events WHERE processed_at IS NOT NULL AND error IS NULL").Scan(&processed); err != nil {
			return err
		}
		return nil
	})
	if processed != 1 {
		t.Fatalf("processed webhook events = %d", processed)
	}
	_ = r.d.WithOrg(context.Background(), tn.orgID, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx, "SELECT trans_id FROM shortcode_verifications WHERE shortcode_id = $1 AND status = 'verified'", scID).Scan(&transID)
	})
	if transID == nil || !strings.HasPrefix(*transID, "FAKE") {
		t.Fatalf("challenge trans_id = %v", transID)
	}
	var audits int64
	_ = r.d.WithOrg(context.Background(), tn.orgID, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE action = 'shortcode.verified'").Scan(&audits)
	})
	if audits != 1 {
		t.Fatalf("shortcode.verified audit rows = %d", audits)
	}

	// 4. Verify again: already verified.
	status, out = r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", map[string]any{})
	if status != http.StatusOK || out["status"] != "verified" {
		t.Fatalf("re-verify = %d %v", status, out)
	}

	// 5. A replay of the same TransID is a duplicate: still one event, no payment.
	sims := r.fake.Simulations()
	replay := fmt.Sprintf(`{"TransactionType":"Pay Bill","TransID":%q,"TransTime":"20260907120000","TransAmount":"1.00","BusinessShortCode":"600123","BillRefNumber":"","MSISDN":%q,"FirstName":"FAKE"}`, sims[0].TransID, ownerMSISDN)
	replayReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, r.api.URL+"/webhooks/mpesa/c2b/confirmation/"+webhookTok, strings.NewReader(replay))
	replayReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(replayReq)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("replay = %v %v", resp, err)
	}
	resp.Body.Close()
	if n := r.count(tn, "payments"); n != 0 {
		t.Fatalf("payments after replay = %d", n)
	}
}

func TestVerification_OtherPayerOrAmountIsANormalPayment(t *testing.T) {
	r := newRig(t, true)
	tn := r.newTenant("Duka B", "A022345678Z", ownerMSISDN)
	scID, _, _ := r.createShortcode(tn, "600124")
	if status, _ := r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", nil); status != http.StatusAccepted {
		t.Fatalf("verify = %d", status)
	}

	// A customer pays KES 1 from another phone, and the owner pays KES 2.
	if _, err := r.daraja.SimulateC2B(context.Background(), "600124", otherMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.daraja.SimulateC2B(context.Background(), "600124", ownerMSISDN, 200, ""); err != nil {
		t.Fatal(err)
	}
	if n := r.count(tn, "payments"); n != 2 {
		t.Fatalf("payments = %d, want 2 ordinary (unmatched) payments", n)
	}
	var unmatched int64
	_ = r.d.WithOrg(context.Background(), tn.orgID, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx, "SELECT count(*) FROM payments WHERE status = 'unmatched'").Scan(&unmatched)
	})
	if unmatched != 2 {
		t.Fatalf("unmatched = %d", unmatched)
	}
	_, sc := r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil)
	if sc["verified"] != false {
		t.Fatalf("shortcode must still be unverified: %v", sc)
	}
	if v, _ := sc["verification"].(map[string]any); v["status"] != "pending" {
		t.Fatalf("challenge should still be open: %v", sc["verification"])
	}

	// Now the real thing.
	if _, err := r.daraja.SimulateC2B(context.Background(), "600124", ownerMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	_, sc = r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil)
	if sc["verified"] != true {
		t.Fatalf("shortcode should be verified: %v", sc)
	}
	if n := r.count(tn, "payments"); n != 2 {
		t.Fatalf("payments = %d, the KES 1 must not be recorded", n)
	}
}

func TestVerification_ClaimedByAnotherOrg(t *testing.T) {
	r := newRig(t, true)
	a := r.newTenant("First", "A032345678Z", ownerMSISDN)
	b := r.newTenant("Second", "A042345678Z", otherMSISDN)

	// Both orgs register the same unverified number; B was first.
	bID, status, _ := r.createShortcode(b, "600125")
	if status != http.StatusCreated {
		t.Fatalf("B create = %d", status)
	}
	aID, status, _ := r.createShortcode(a, "600125")
	if status != http.StatusCreated {
		t.Fatalf("A create (duplicate unverified) = %d, want 201", status)
	}

	// A proves it: the C2B is routed to A's row even though B's is older.
	if status, _ := r.call(a, http.MethodPost, "/shortcodes/"+aID.String()+"/verify", nil); status != http.StatusAccepted {
		t.Fatalf("A verify = %d", status)
	}
	if _, err := r.daraja.SimulateC2B(context.Background(), "600125", ownerMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if _, sc := r.call(a, http.MethodGet, "/shortcodes/"+aID.String(), nil); sc["verified"] != true {
		t.Fatalf("A should be verified: %v", sc)
	}
	if n := r.count(b, "payments"); n != 0 {
		t.Fatalf("B must not receive A's KES 1, got %d payments", n)
	}

	// B can no longer verify or re-add the number.
	status, out := r.call(b, http.MethodPost, "/shortcodes/"+bID.String()+"/verify", nil)
	if status != http.StatusConflict || errCode(out) != "shortcode_claimed" {
		t.Fatalf("B verify = %d %v", status, out)
	}
	_, status, out = r.createShortcode(b, "600125")
	if status != http.StatusConflict || errCode(out) != "shortcode_claimed" {
		t.Fatalf("B create = %d %v", status, out)
	}

	// Lost race: B opens a challenge, then A gets 600126 verified first.
	cID, _, _ := r.createShortcode(b, "600126")
	if status, _ := r.call(b, http.MethodPost, "/shortcodes/"+cID.String()+"/verify", nil); status != http.StatusAccepted {
		t.Fatalf("B verify 600126 = %d", status)
	}
	var aRow uuid.UUID
	_ = r.d.WithOrg(context.Background(), a.orgID, func(ctx context.Context, tx db.Tx) error {
		row, err := tx.CreateShortcode(ctx, gen.CreateShortcodeParams{OrgID: a.orgID, Kind: "till", Shortcode: "600126"})
		if err != nil {
			return err
		}
		aRow = row.ID
		return tx.MarkShortcodeVerified(ctx, gen.MarkShortcodeVerifiedParams{ID: row.ID, VerificationCheckoutID: db.Ptr("test")})
	})
	if aRow == uuid.Nil {
		t.Fatal("could not set up the race")
	}
	// A verified row always wins the resolver, so B's KES 1 lands in A's
	// ledger as an ordinary payment and B's challenge never completes.
	if _, err := r.daraja.SimulateC2B(context.Background(), "600126", otherMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if n := r.count(a, "payments"); n != 1 {
		t.Fatalf("A payments = %d, want the KES 1 as an ordinary payment", n)
	}
	status, out = r.call(b, http.MethodPost, "/shortcodes/"+cID.String()+"/verify", nil)
	if status != http.StatusConflict || errCode(out) != "shortcode_claimed" {
		t.Fatalf("B re-verify after loss = %d %v", status, out)
	}
}

func TestVerification_ExpiredChallengeAndRestart(t *testing.T) {
	r := newRig(t, true)
	tn := r.newTenant("Slow Duka", "A052345678Z", ownerMSISDN)
	scID, _, _ := r.createShortcode(tn, "600127")
	if status, _ := r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", nil); status != http.StatusAccepted {
		t.Fatalf("verify = %d", status)
	}
	// Time passes.
	_ = r.d.WithOrg(context.Background(), tn.orgID, func(ctx context.Context, tx db.Tx) error {
		_, err := tx.Tx.Exec(ctx, "UPDATE shortcode_verifications SET expires_at = now() - interval '1 minute' WHERE shortcode_id = $1", scID)
		return err
	})
	_, sc := r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil)
	if v, _ := sc["verification"].(map[string]any); v["status"] != "expired" {
		t.Fatalf("expected expired, got %v", sc["verification"])
	}
	// A late KES 1 is just a payment.
	if _, err := r.daraja.SimulateC2B(context.Background(), "600127", ownerMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if n := r.count(tn, "payments"); n != 1 {
		t.Fatalf("late payment rows = %d", n)
	}
	_, sc = r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil)
	if sc["verified"] != false {
		t.Fatalf("expired challenge must not verify: %v", sc)
	}

	// Start again: a fresh challenge, the old one marked expired.
	status, out := r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", nil)
	if status != http.StatusAccepted {
		t.Fatalf("restart = %d %v", status, out)
	}
	var pending, expired int64
	_ = r.d.WithOrg(context.Background(), tn.orgID, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx, "SELECT count(*) FILTER (WHERE status='pending'), count(*) FILTER (WHERE status='expired') FROM shortcode_verifications WHERE shortcode_id = $1", scID).Scan(&pending, &expired)
	})
	if pending != 1 || expired != 1 {
		t.Fatalf("pending=%d expired=%d", pending, expired)
	}
	if _, err := r.daraja.SimulateC2B(context.Background(), "600127", ownerMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if _, sc := r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil); sc["verified"] != true {
		t.Fatalf("should verify on the fresh challenge: %v", sc)
	}
}

func TestVerification_MSISDNOverrideAndValidation(t *testing.T) {
	r := newRig(t, true)
	tn := r.newTenant("Override", "A062345678Z", ownerMSISDN)
	scID, _, _ := r.createShortcode(tn, "600128")

	status, out := r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", map[string]any{"msisdn": "12345"})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("bad msisdn = %d %v", status, out)
	}
	status, out = r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", map[string]any{"msisdn": "0708374149"})
	if status != http.StatusAccepted || out["msisdn_masked"] != "2547•••••149" {
		t.Fatalf("override = %d %v", status, out)
	}
	// The owner's own number no longer matches; the override does.
	if _, err := r.daraja.SimulateC2B(context.Background(), "600128", ownerMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if _, sc := r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil); sc["verified"] != false {
		t.Fatalf("owner number should not settle an overridden challenge: %v", sc)
	}
	if _, err := r.daraja.SimulateC2B(context.Background(), "600128", otherMSISDN, 100, ""); err != nil {
		t.Fatal(err)
	}
	if _, sc := r.call(tn, http.MethodGet, "/shortcodes/"+scID.String(), nil); sc["verified"] != true {
		t.Fatalf("override number should verify: %v", sc)
	}
	if status, _ := r.call(tn, http.MethodGet, "/shortcodes/"+uuid.NewString(), nil); status != http.StatusNotFound {
		t.Fatalf("unknown id = %d", status)
	}
}

func TestVerification_LocalModeVerifiesImmediately(t *testing.T) {
	r := newRig(t, false)
	tn := r.newTenant("Local", "A072345678Z", ownerMSISDN)
	scID, _, _ := r.createShortcode(tn, "600129")
	status, out := r.call(tn, http.MethodPost, "/shortcodes/"+scID.String()+"/verify", nil)
	if status != http.StatusOK || out["status"] != "verified" {
		t.Fatalf("local verify = %d %v", status, out)
	}
	sc, _ := out["shortcode"].(map[string]any)
	if sc["verified"] != true {
		t.Fatalf("shortcode in body %v", sc)
	}
	if _, ok := r.fake.Registration("600129"); ok {
		t.Fatal("no Daraja call expected without credentials")
	}
}
