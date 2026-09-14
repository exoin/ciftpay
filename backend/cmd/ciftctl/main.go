// Command ciftctl is the developer CLI:
//
//	ciftctl migrate                 apply goose + River migrations
//	ciftctl seed                    demo org, shortcode 600123, default item, user, open KES 1 sale
//	ciftctl replay-webhook <file>   POST a recorded Daraja payload to the local api
//	ciftctl register-urls <shortcode>          Daraja RegisterURL for a shortcode at WEBHOOK_BASE_URL
//	ciftctl simulate-c2b --shortcode --msisdn  Daraja sandbox C2B simulate (or the fake)
//	ciftctl daraja-fake [--addr :18090]        run the in-process fake Daraja for local end-to-end runs
//	ciftctl verify-shortcode <id|number> [--reject REASON] [--note TEXT]
//	                                           the Administrative Gate (ADR-0008): mark a shortcode
//	                                           verified once Safaricom mapped it, or reject the letter
//	ciftctl kra-init --pin P000000000X [--branch 00] [--serial S]
//	                                           direct KRA OSCU (ADR-0009): fetch a gateway token and run
//	                                           selectInitOsdcInfo; prints the device info, masks the cmcKey
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/admin"
	"github.com/ciftpay/ciftpay/internal/boot"
	"github.com/ciftpay/ciftpay/internal/fiscal/oscu"
	"github.com/ciftpay/ciftpay/internal/ledger"
	"github.com/ciftpay/ciftpay/internal/mpesa"
	"github.com/ciftpay/ciftpay/internal/mpesa/mpesatest"
	"github.com/ciftpay/ciftpay/internal/platform/config"
	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error
	switch args[0] {
	case "migrate":
		err = withDeps(ctx, migrate)
	case "seed":
		err = withDeps(ctx, seed)
	case "replay-webhook":
		if len(args) < 2 {
			usage()
			return 2
		}
		err = replay(ctx, args[1])
	case "register-urls":
		if len(args) < 2 {
			usage()
			return 2
		}
		err = registerURLs(ctx, args[1])
	case "simulate-c2b":
		err = simulateC2B(ctx, args[1:])
	case "daraja-fake":
		cancel()
		err = darajaFake(args[1:])
	case "verify-shortcode":
		if len(args) < 2 {
			usage()
			return 2
		}
		err = withDeps(ctx, func(ctx context.Context, d *boot.Deps) error { return verifyShortcode(ctx, d, args[1:]) })
	case "kra-init":
		err = kraInit(ctx, args[1:])
	default:
		usage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ciftctl:", err)
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ciftctl migrate | seed | replay-webhook <file.json> | register-urls <shortcode> | simulate-c2b --shortcode N --msisdn N [--amount KES] [--ref REF] | daraja-fake [--addr :18090] | verify-shortcode <id|number> [--reject REASON] [--note TEXT] | kra-init --pin PIN [--branch 00] [--serial S]")
}

// kraInit is the first live check of the direct OSCU path (ADR-0009): a
// gateway token (GET + Basic, through the DNS override) and one
// selectInitOsdcInfo. Needs no database. The cmcKey is a credential and is
// only shown masked; storing it on the org is the RegisterDevice step.
func kraInit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("kra-init", flag.ContinueOnError)
	pin := fs.String("pin", "", "taxpayer KRA PIN (sandbox test PIN from the developer portal)")
	branch := fs.String("branch", "00", "branch office id")
	serial := fs.String("serial", "", "device serial (default KRA_OSCU_DEVICE_SERIAL)")
	tokenOnly := fs.Bool("token-only", false, "stop after the OAuth token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.KRA.Configured() {
		return errors.New("KRA_OSCU_CONSUMER_KEY and KRA_OSCU_CONSUMER_SECRET are not set")
	}
	c, err := oscu.NewClient(boot.OSCUConfig(cfg.KRA, cfg.Fiscal))
	if err != nil {
		return err
	}
	start := time.Now()
	tok, err := c.Token(ctx)
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	fmt.Printf("token ok (%d chars, %s, resolver=%s) in %s\n", len(tok), cfg.KRA.BaseURL, orSystem(cfg.KRA.DNSResolver), time.Since(start).Round(time.Millisecond))
	if *tokenOnly {
		return nil
	}
	if *pin == "" {
		return errors.New("--pin is required (or pass --token-only)")
	}
	info, _, err := c.Initialize(ctx, *pin, *branch, *serial)
	if err != nil {
		return fmt.Errorf("selectInitOsdcInfo: %w", err)
	}
	fmt.Printf("device initialised: tin=%s taxpayer=%q bhfId=%s bhfNm=%q dvcId=%s sdcId=%s mrcNo=%s cmcKey=%s\n",
		info.TIN, info.TaxpayerName, info.BranchID, info.BranchName, info.DeviceID, info.SDCID, info.MRCNo, mask(info.CmcKey))
	return nil
}

func orSystem(s string) string {
	if s == "" {
		return "system"
	}
	return s
}

func mask(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

// verifyShortcode is the operator's shell entry into the Administrative Gate.
// It takes the row id or the shortcode number (which must be unambiguous) and
// uses the same admin.Shortcodes path as PATCH /admin/shortcodes/{id}/verify.
func verifyShortcode(ctx context.Context, d *boot.Deps, args []string) error {
	fs := flag.NewFlagSet("verify-shortcode", flag.ContinueOnError)
	reject := fs.String("reject", "", "reject with this reason instead of verifying")
	note := fs.String("note", "", "free text for the audit row (e.g. Safaricom ticket)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	files, err := d.Files()
	if err != nil {
		return err
	}
	svc := &admin.Shortcodes{DB: d.DB, Keys: d.Keys, Files: files, Daraja: mpesa.NewClient(d.Cfg.Daraja), WebhookBaseURL: d.Cfg.WebhookBaseURL, Log: d.Log}

	id, err := uuid.Parse(args[0])
	if err != nil {
		rows, err := svc.FindByNumber(ctx, strings.TrimSpace(args[0]))
		if err != nil {
			return err
		}
		switch len(rows) {
		case 0:
			return fmt.Errorf("no shortcode %q", args[0])
		case 1:
			id = rows[0].ID
		default:
			fmt.Fprintf(os.Stderr, "shortcode %s is claimed by %d organisations; pass the row id:\n", args[0], len(rows))
			for _, r := range rows {
				fmt.Fprintf(os.Stderr, "  %s  org=%s  status=%s  letter=%v\n", r.ID, r.OrgID, r.Status, r.AuthorizationLetterPath != nil)
			}
			return errors.New("ambiguous shortcode")
		}
	}
	var out ledger.AdminShortcodeView
	if *reject != "" {
		out, err = svc.Reject(ctx, id, "ciftctl", *reject)
	} else {
		out, err = svc.Verify(ctx, id, "ciftctl", *note)
	}
	if err != nil {
		return err
	}
	fmt.Printf("%s %s (%s, org %s) -> %s", out.Kind, out.Shortcode, out.OrgName, out.OrgID, out.Status)
	if out.C2BURLsRegisteredAt != nil {
		fmt.Print(", C2B URLs registered")
	}
	fmt.Println()
	return nil
}

func withDeps(ctx context.Context, fn func(context.Context, *boot.Deps) error) error {
	d, err := boot.Load(ctx)
	if err != nil {
		return err
	}
	defer d.Close()
	return fn(ctx, d)
}

func migrate(ctx context.Context, d *boot.Deps) error {
	if err := d.DB.Migrate(ctx); err != nil {
		return err
	}
	v, err := d.DB.Version(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("migrated: schema version %d, river tables up to date\n", v)
	return nil
}

// Seed constants; the webhook fixtures in tools/webhooks target them.
const (
	seedOrgName    = "Mama Njeri Wholesalers"
	seedPIN        = "P051234567X"
	seedShortcode  = "600123"
	seedOwnerPhone = "254712345678"
	seedBuyerPhone = "254708374149"
	seedCheckoutID = "ws_CO_02092026121712345"
)

func seed(ctx context.Context, d *boot.Deps) error {
	pinEnc, err := d.Keys.EncryptString(seedPIN)
	if err != nil {
		return err
	}
	ownerEnc, err := d.Keys.EncryptString(seedOwnerPhone)
	if err != nil {
		return err
	}
	buyerEnc, err := d.Keys.EncryptString(seedBuyerPhone)
	if err != nil {
		return err
	}

	var org gen.Org
	err = d.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		org, err = tx.GetOrgByPINHash(ctx, d.Keys.Hash(seedPIN))
		if errors.Is(err, pgx.ErrNoRows) {
			org, err = tx.CreateOrg(ctx, gen.CreateOrgParams{Name: seedOrgName, KraPinEnc: pinEnc, KraPinHash: d.Keys.Hash(seedPIN), VatRegistered: true, Locale: "en"})
		}
		if err != nil {
			return err
		}
		user, err := tx.CreateUser(ctx, gen.CreateUserParams{MsisdnEnc: ownerEnc, MsisdnHash: d.Keys.Hash(seedOwnerPhone), Name: "Njeri Kamau", Locale: "en"})
		if err != nil {
			return err
		}
		_, err = tx.CreateMembership(ctx, gen.CreateMembershipParams{OrgID: org.ID, UserID: user.ID, Role: "owner", IsDefault: true})
		return err
	})
	if err != nil {
		return err
	}

	return d.DB.WithOrg(ctx, org.ID, func(ctx context.Context, tx db.Tx) error {
		items, err := tx.ListItems(ctx, org.ID)
		if err != nil {
			return err
		}
		var item gen.Item
		if len(items) > 0 {
			item = items[0]
		} else {
			item, err = tx.CreateItem(ctx, gen.CreateItemParams{OrgID: org.ID, Name: "General goods", EtimsClassCode: "5020230000", TaxCategory: "B", Unit: "PCS", PriceCents: 0})
			if err != nil {
				return err
			}
		}
		scs, err := tx.ListShortcodes(ctx, org.ID)
		if err != nil {
			return err
		}
		var sc gen.MpesaShortcode
		for _, s := range scs {
			if s.Shortcode == seedShortcode {
				sc = s
			}
		}
		if sc.ID == uuid.Nil {
			sc, err = tx.CreateShortcode(ctx, gen.CreateShortcodeParams{OrgID: org.ID, Kind: "till", Shortcode: seedShortcode, Label: "Shop till", DefaultItemID: &item.ID, AutoInvoice: true})
			if err != nil {
				return err
			}
			// The seed skips the Administrative Gate so make replay-webhook
			// has a verified shortcode to land on.
			if _, err := tx.VerifyShortcode(ctx, gen.VerifyShortcodeParams{ID: sc.ID, ReviewedBy: db.Ptr("seed")}); err != nil {
				return err
			}
		}
		if _, err := tx.GetSTKRequestByCheckoutID(ctx, seedCheckoutID); err == nil {
			fmt.Printf("seed: already present (org %s)\n", org.ID)
			return nil
		}
		// An open KES 1 sale awaiting the stk_callback.json fixture.
		cust, err := tx.UpsertCustomerByMSISDN(ctx, gen.UpsertCustomerByMSISDNParams{OrgID: org.ID, Name: "Mary Wanjiku Kamau", MsisdnEnc: buyerEnc, MsisdnHash: d.Keys.Hash(seedBuyerPhone)})
		if err != nil {
			return err
		}
		ref, err := tx.NextSaleRef(ctx, org.ID)
		if err != nil {
			return err
		}
		sale, err := tx.CreateSale(ctx, gen.CreateSaleParams{OrgID: org.ID, Ref: ref, Kind: "open", Status: "open", CustomerID: &cust.ID, SubtotalCents: 86, TaxCents: 14, TotalCents: 100})
		if err != nil {
			return err
		}
		if _, err := tx.CreateSaleItem(ctx, gen.CreateSaleItemParams{OrgID: org.ID, SaleID: sale.ID, ItemID: &item.ID, Description: item.Name, EtimsClassCode: item.EtimsClassCode,
			Unit: item.Unit, Qty: "1", UnitPriceCents: 100, TaxCategory: "B", TaxRateBp: 1600, LineTotalCents: 100, LineTaxCents: 14}); err != nil {
			return err
		}
		_, err = tx.CreateSTKRequest(ctx, gen.CreateSTKRequestParams{OrgID: org.ID, SaleID: &sale.ID, ShortcodeID: sc.ID, MsisdnHash: d.Keys.Hash(seedBuyerPhone),
			AmountCents: 100, CheckoutRequestID: seedCheckoutID, MerchantRequestID: db.Ptr("29115-34620561-1"), Purpose: "sale", ExpiresAt: time.Now().Add(365 * 24 * time.Hour)})
		if err != nil {
			return err
		}
		fmt.Printf("seed: org %s, shortcode %s, item %s, open sale %s, owner %s\n", org.ID, seedShortcode, item.ID, sale.Ref, seedOwnerPhone)
		return nil
	})
}

func replay(ctx context.Context, file string) error {
	body, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return fmt.Errorf("%s is not JSON: %w", file, err)
	}
	token := env("DARAJA_WEBHOOK_TOKEN", "dev-webhook-token")
	base := strings.TrimRight(env("API_BASE_URL", "http://localhost:8080"), "/")
	path := "/webhooks/daraja/c2b/confirmation/" + token
	if _, ok := probe["Body"]; ok {
		path = "/webhooks/daraja/stk/" + token
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Printf("POST %s -> %d %s\n", path, resp.StatusCode, strings.TrimSpace(string(out)))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("api returned %d", resp.StatusCode)
	}
	return nil
}

// darajaClient builds the outbound Daraja client from the environment. With
// DARAJA_BASE_URL pointing at `ciftctl daraja-fake` any key/secret will do.
func darajaClient() (*mpesa.Client, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, cfg, err
	}
	c := mpesa.NewClient(cfg.Daraja)
	if !c.Configured() {
		return nil, cfg, fmt.Errorf("DARAJA_CONSUMER_KEY and DARAJA_CONSUMER_SECRET are required (any value works against the fake)")
	}
	return c, cfg, nil
}

// registerURLs points Daraja's C2B validation/confirmation URLs for shortcode
// at WEBHOOK_BASE_URL. In the sandbox this is how the test shortcode is wired
// to a tunnel before `simulate-c2b`.
func registerURLs(ctx context.Context, shortcode string) error {
	c, cfg, err := darajaClient()
	if err != nil {
		return err
	}
	if err := c.RegisterC2BURLs(ctx, shortcode, cfg.WebhookBaseURL); err != nil {
		return err
	}
	fmt.Printf("registered C2B URLs for %s -> %s/webhooks/daraja/c2b/{validation,confirmation}/<token>\n", shortcode, cfg.WebhookBaseURL)
	return nil
}

// simulateC2B asks the sandbox (or the fake) to emit a C2B confirmation, the
// stand-in for the merchant paying KES 1 to their own Till.
func simulateC2B(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("simulate-c2b", flag.ContinueOnError)
	shortcode := fs.String("shortcode", "", "BusinessShortCode receiving the payment")
	msisdn := fs.String("msisdn", "", "payer phone, e.g. 0140994513")
	amount := fs.Float64("amount", 1, "amount in KES")
	ref := fs.String("ref", "", "BillRefNumber / account reference (Paybill)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *shortcode == "" || *msisdn == "" {
		return fmt.Errorf("--shortcode and --msisdn are required")
	}
	norm, err := crypto.NormaliseMSISDN(*msisdn)
	if err != nil {
		return err
	}
	c, cfg, err := darajaClient()
	if err != nil {
		return err
	}
	cents := int64(*amount*100 + 0.5)
	out, err := c.SimulateC2B(ctx, *shortcode, norm, cents, *ref)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(out)
	fmt.Printf("simulate %s KES %.2f from %s via %s -> %s\n", *shortcode, float64(cents)/100, norm, cfg.Daraja.BaseURL, b)
	return nil
}

// darajaFake serves mpesatest on --addr until interrupted.
func darajaFake(args []string) error {
	fs := flag.NewFlagSet("daraja-fake", flag.ContinueOnError)
	addr := fs.String("addr", ":18090", "listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	srv := &http.Server{Addr: *addr, Handler: mpesatest.New().Handler, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("fake Daraja listening on %s (oauth, c2b registerurl, c2b simulate); set DARAJA_BASE_URL=http://localhost%s\n", *addr, *addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
