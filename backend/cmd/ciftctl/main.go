// Command ciftctl is the developer CLI:
//
//	ciftctl migrate                 apply goose + River migrations
//	ciftctl seed                    demo org, shortcode 600123, default item, user, open KES 1 sale
//	ciftctl replay-webhook <file>   POST a recorded Daraja payload to the local api
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/boot"
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
	fmt.Fprintln(os.Stderr, "usage: ciftctl migrate | seed | replay-webhook <file.json>")
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
			if err := tx.MarkShortcodeVerified(ctx, gen.MarkShortcodeVerifiedParams{ID: sc.ID, VerificationCheckoutID: db.Ptr("seed")}); err != nil {
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
	path := "/webhooks/mpesa/c2b/confirmation/" + token
	if _, ok := probe["Body"]; ok {
		path = "/webhooks/mpesa/stk/" + token
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

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
