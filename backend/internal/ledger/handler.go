package ledger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
	"github.com/ciftpay/ciftpay/internal/platform/storage"
)

// STKPusher is the slice of the Daraja client the request-to-pay path needs
// (mpesa.Client in production, a fake in tests).
type STKPusher interface {
	Configured() bool
	STKPush(ctx context.Context, shortcode, msisdn string, amountCents int64, accountRef, desc, webhookBaseURL string) (checkoutID, merchantID string, err error)
}

// Retrier re-queues a NEEDS_REVIEW invoice (fiscal.Submitter).
type Retrier interface {
	Retry(ctx context.Context, orgID, invoiceID uuid.UUID, actor string) error
}

// Handler serves the tenant-scoped ledger routes. Mount behind auth + RequireOrg.
type Handler struct {
	S       *Service
	Keys    *crypto.Keyring
	STK     STKPusher
	Retrier Retrier
	// Files holds the uploaded Safaricom authorization letters (ADR-0008).
	Files storage.Store
	// PublicBaseURL is the web app buyers open receipt links on.
	PublicBaseURL string
	// PendingLongAfter marks invoices still not acked after this as attention items.
	PendingLongAfter time.Duration
}

// Mount registers the routes.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/shortcodes", h.listShortcodes)
	r.Post("/shortcodes", h.createShortcode)
	r.Get("/shortcodes/{id}", h.getShortcode)
	r.Patch("/shortcodes/{id}", h.updateShortcode)
	r.Post("/shortcodes/{id}/authorization", h.submitAuthorization)

	r.Get("/payments", h.listPayments)
	r.Post("/payments/{id}/convert", h.convertPayment)

	r.Get("/items", h.listItems)
	r.Post("/items", h.createItem)
	r.Patch("/items/{id}", h.updateItem)

	r.Get("/sales", h.listSales)
	r.Post("/sales", h.createSale)
	r.Get("/sales/{id}", h.getSale)

	r.Get("/invoices", h.listInvoices)
	r.Get("/invoices/{id}", h.getInvoice)
	r.Post("/invoices/{id}/retry", h.retryInvoice)

	r.Get("/attention", h.attention)
}

// ------------------------------------------------------------ helpers

func org(r *http.Request) (uuid.UUID, uuid.UUID) {
	p, _ := httpx.PrincipalFrom(r.Context())
	return p.OrgID, p.UserID
}

func idParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Unknown id")
		return uuid.Nil, false
	}
	return id, true
}

func page(r *http.Request) (limit, offset int32) {
	limit = 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 200 {
		limit = int32(n)
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && n >= 0 {
		offset = int32(n)
	}
	return limit, offset
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func fail(w http.ResponseWriter, err error, msg string) {
	switch {
	case db.NotFound(err), errors.Is(err, pgx.ErrNoRows):
		httpx.Fail(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, ErrShortcodeClaimed):
		httpx.Fail(w, http.StatusConflict, "shortcode_claimed", "This number is already verified by another business. If it is yours, contact support.")
	case errors.Is(err, ErrAlreadyVerified):
		httpx.Fail(w, http.StatusConflict, "conflict", "This shortcode is already verified.")
	case errors.Is(err, fiscal.ErrIllegalTransition), errors.Is(err, ErrDuplicate):
		httpx.Fail(w, http.StatusConflict, "conflict", err.Error())
	case errors.As(err, new(*validationError)):
		httpx.Fail(w, http.StatusUnprocessableEntity, "validation", err.Error())
	default:
		httpx.Fail(w, http.StatusInternalServerError, "internal", msg)
	}
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func invalid(format string, a ...any) error { return &validationError{msg: fmt.Sprintf(format, a...)} }

// --------------------------------------------------------- shortcodes

func (h *Handler) listShortcodes(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	out := []ShortcodeView{}
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListShortcodes(ctx, orgID)
		for _, s := range rows {
			out = append(out, toShortcode(s))
		}
		return err
	})
	if err != nil {
		fail(w, err, "Could not list shortcodes")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

type shortcodeInput struct {
	Kind          string     `json:"kind"`
	Shortcode     string     `json:"shortcode"`
	Label         string     `json:"label"`
	DefaultItemID *uuid.UUID `json:"default_item_id"`
	AutoInvoice   *bool      `json:"auto_invoice"`
}

func (h *Handler) createShortcode(w http.ResponseWriter, r *http.Request) {
	orgID, userID := org(r)
	var in shortcodeInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {kind, shortcode, label?, default_item_id?, auto_invoice?}")
		return
	}
	in.Shortcode = strings.TrimSpace(in.Shortcode)
	switch {
	case in.Kind == "pochi":
		// Daraja C2B RegisterURL does not cover Pochi la Biashara, so no
		// confirmation could ever reach CiftPay for one (plan.md §4.1).
		fail(w, invalid("Pochi la Biashara is not supported: Safaricom does not deliver C2B callbacks for it. Register a Till or Paybill."), "")
		return
	case in.Kind != "till" && in.Kind != "paybill":
		fail(w, invalid("kind must be till or paybill"), "")
		return
	case len(in.Shortcode) < 5 || len(in.Shortcode) > 12 || strings.Trim(in.Shortcode, "0123456789") != "":
		fail(w, invalid("shortcode must be 5–12 digits"), "")
		return
	}
	auto := true
	if in.AutoInvoice != nil {
		auto = *in.AutoInvoice
	}
	if err := h.claimedElsewhere(r.Context(), in.Shortcode, orgID); err != nil {
		fail(w, err, "Could not check the shortcode")
		return
	}
	var out gen.MpesaShortcode
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		if in.DefaultItemID != nil {
			if _, err := tx.GetItem(ctx, *in.DefaultItemID); err != nil {
				return invalid("default_item_id does not exist")
			}
		}
		var err error
		out, err = tx.CreateShortcode(ctx, gen.CreateShortcodeParams{OrgID: orgID, Kind: in.Kind, Shortcode: in.Shortcode, Label: in.Label, DefaultItemID: in.DefaultItemID, AutoInvoice: auto})
		if isUniqueViolation(err) {
			return ErrShortcodeClaimed
		}
		if err != nil {
			return err
		}
		actor := userID.String()
		return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "user", ActorID: &actor, Action: "shortcode.created", Entity: "mpesa_shortcode", EntityID: out.ID.String()})
	})
	if err != nil {
		fail(w, err, "Could not add the shortcode")
		return
	}
	httpx.JSON(w, http.StatusCreated, toShortcode(out))
}

func (h *Handler) updateShortcode(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in shortcodeInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {label?, default_item_id?, auto_invoice?}")
		return
	}
	var out gen.MpesaShortcode
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		if in.DefaultItemID != nil {
			if _, err := tx.GetItem(ctx, *in.DefaultItemID); err != nil {
				return invalid("default_item_id does not exist")
			}
		}
		var err error
		out, err = tx.UpdateShortcode(ctx, gen.UpdateShortcodeParams{ID: id, Label: optString(in.Label), DefaultItemID: in.DefaultItemID, AutoInvoice: in.AutoInvoice})
		return err
	})
	if err != nil {
		fail(w, err, "Could not update the shortcode")
		return
	}
	httpx.JSON(w, http.StatusOK, toShortcode(out))
}

func (h *Handler) getShortcode(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var out ShortcodeView
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		sc, err := tx.GetShortcode(ctx, id)
		if err != nil {
			return err
		}
		out = toShortcode(sc)
		return nil
	})
	if err != nil {
		fail(w, err, "Could not load the shortcode")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// claimedElsewhere returns ErrShortcodeClaimed when another organisation
// already holds a verified row for the number. It is the one cross-tenant
// read the merchant API makes, hence db.WithIngest.
func (h *Handler) claimedElsewhere(ctx context.Context, shortcode string, orgID uuid.UUID) error {
	var n int64
	err := h.S.DB.WithIngest(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		n, err = tx.CountVerifiedShortcodeElsewhere(ctx, gen.CountVerifiedShortcodeElsewhereParams{Shortcode: shortcode, OrgID: orgID})
		return err
	})
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrShortcodeClaimed
	}
	return nil
}

// submitAuthorization stores the signed Safaricom authorization letter
// (multipart field "letter", image or PDF). It is the merchant's only step in
// the Administrative Gate; an operator later marks the shortcode verified.
func (h *Handler) submitAuthorization(w http.ResponseWriter, r *http.Request) {
	orgID, userID := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if h.Files == nil {
		httpx.Fail(w, http.StatusNotImplemented, "not_implemented", "Uploads are not configured")
		return
	}
	// Slack above LetterMaxBytes for the multipart framing; the service
	// enforces the exact file limit.
	r.Body = http.MaxBytesReader(w, r.Body, LetterMaxBytes+64<<10)
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be multipart/form-data with a `letter` file")
		return
	}
	var sc gen.MpesaShortcode
	for {
		part, err := mr.NextPart()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fail(w, invalid("letter file is required"), "")
				return
			}
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				fail(w, invalid("letter must be at most %d MB", LetterMaxBytes>>20), "")
				return
			}
			httpx.Fail(w, http.StatusBadRequest, "bad_request", "Malformed multipart body")
			return
		}
		if part.FormName() != "letter" {
			_ = part.Close()
			continue
		}
		sc, err = h.S.SubmitAuthorization(r.Context(), h.Files, orgID, userID, id, part)
		_ = part.Close()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				err = invalid("letter must be at most %d MB", LetterMaxBytes>>20)
			}
			fail(w, err, "Could not store the letter")
			return
		}
		break
	}
	httpx.JSON(w, http.StatusOK, toShortcode(sc))
}

// ----------------------------------------------------------- payments

func (h *Handler) listPayments(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	limit, offset := page(r)
	out := []PaymentView{}
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListPayments(ctx, gen.ListPaymentsParams{OrgID: orgID, Limit: limit, Offset: offset, Status: optString(r.URL.Query().Get("status"))})
		if err != nil {
			return err
		}
		for _, p := range rows {
			out = append(out, toPayment(h.Keys, p, h.invoiceIDForSale(ctx, tx, p.SaleID)))
		}
		return nil
	})
	if err != nil {
		fail(w, err, "Could not list payments")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "limit": limit, "offset": offset})
}

func (h *Handler) invoiceIDForSale(ctx context.Context, tx db.Tx, saleID *uuid.UUID) *uuid.UUID {
	if saleID == nil {
		return nil
	}
	var id uuid.UUID
	if err := tx.Tx.QueryRow(ctx, "SELECT id FROM invoices WHERE sale_id = $1 AND kind = 'INVOICE' ORDER BY created_at DESC LIMIT 1", *saleID).Scan(&id); err != nil {
		return nil
	}
	return &id
}

// convertPayment turns an unmatched payment into a cash sale + invoice, using
// the given item (or the shortcode default).
func (h *Handler) convertPayment(w http.ResponseWriter, r *http.Request) {
	orgID, userID := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		ItemID *uuid.UUID `json:"item_id"`
		SaleID *uuid.UUID `json:"sale_id"`
	}
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {item_id?} or {sale_id?}")
			return
		}
	}
	var inv gen.Invoice
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		p, err := tx.GetPayment(ctx, id)
		if err != nil {
			return err
		}
		if p.Status != StatusUnmatched && p.Status != StatusPartial {
			return fmt.Errorf("%w: payment is already %s", ErrDuplicate, p.Status)
		}
		var saleID uuid.UUID
		status := StatusCashSale
		if in.SaleID != nil {
			status = StatusMatched
			sale, err := tx.GetSale(ctx, *in.SaleID)
			if err != nil || sale.Status != "open" {
				return invalid("sale_id must be an open sale")
			}
			paidAt := p.PaidAt
			if err := tx.MarkSalePaid(ctx, gen.MarkSalePaidParams{ID: sale.ID, PaidAt: &paidAt}); err != nil {
				return err
			}
			saleID = sale.ID
		} else {
			sc, err := tx.GetShortcode(ctx, p.ShortcodeID)
			if err != nil {
				return err
			}
			row := gen.ResolveShortcodeRow{ID: sc.ID, OrgID: sc.OrgID, Kind: sc.Kind, Shortcode: sc.Shortcode, DefaultItemID: in.ItemID, AutoInvoice: true, Status: sc.Status}
			if row.DefaultItemID == nil {
				row.DefaultItemID = sc.DefaultItemID
			}
			if row.DefaultItemID == nil {
				return invalid("item_id is required: this shortcode has no default item")
			}
			c2b := C2BInput{TransID: p.TransID, AmountCents: p.AmountCents, PayerName: p.PayerName, BillRef: p.BillRef, PaidAt: p.PaidAt}
			sale, err := h.S.createCashSale(ctx, tx, row, c2b, p.MsisdnHash, p.MsisdnEnc)
			if err != nil {
				return err
			}
			saleID = sale.ID
		}
		if err := tx.AttachPaymentToSale(ctx, gen.AttachPaymentToSaleParams{ID: p.ID, SaleID: &saleID, Status: status, MatchRule: db.Ptr(RuleManual)}); err != nil {
			return err
		}
		inv, err = h.S.CreateInvoiceForSale(ctx, tx, orgID, saleID, &p.ID, p.PaidAt)
		if err != nil {
			return err
		}
		actor := userID.String()
		return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "user", ActorID: &actor, Action: "payment.converted", Entity: "payment", EntityID: p.ID.String()})
	})
	if err != nil {
		fail(w, err, "Could not convert the payment")
		return
	}
	httpx.JSON(w, http.StatusCreated, toInvoice(h.Keys, h.PublicBaseURL, inv))
}

// -------------------------------------------------------------- items

func (h *Handler) listItems(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	out := []ItemView{}
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListItems(ctx, orgID)
		for _, i := range rows {
			out = append(out, toItem(i))
		}
		return err
	})
	if err != nil {
		fail(w, err, "Could not list items")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

type itemInput struct {
	Name           *string `json:"name"`
	EtimsClassCode *string `json:"etims_class_code"`
	TaxCategory    *string `json:"tax_category"`
	Unit           *string `json:"unit"`
	PriceCents     *int64  `json:"price_cents"`
	IsActive       *bool   `json:"is_active"`
}

func (in itemInput) validate(creating bool) error {
	if creating && (in.Name == nil || in.EtimsClassCode == nil || in.TaxCategory == nil) {
		return invalid("name, etims_class_code and tax_category are required")
	}
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || len(*in.Name) > 120) {
		return invalid("name must be 1–120 characters")
	}
	if in.TaxCategory != nil {
		if _, err := fiscal.ParseTaxCategory(*in.TaxCategory); err != nil {
			return invalid("tax_category must be one of A, B, C, D, E")
		}
	}
	if in.PriceCents != nil && *in.PriceCents < 0 {
		return invalid("price_cents must be ≥ 0")
	}
	return nil
}

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	var in itemInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {name, etims_class_code, tax_category, unit?, price_cents?}")
		return
	}
	if err := in.validate(true); err != nil {
		fail(w, err, "")
		return
	}
	unit, price := "PCS", int64(0)
	if in.Unit != nil && *in.Unit != "" {
		unit = *in.Unit
	}
	if in.PriceCents != nil {
		price = *in.PriceCents
	}
	var out gen.Item
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = tx.CreateItem(ctx, gen.CreateItemParams{OrgID: orgID, Name: strings.TrimSpace(*in.Name), EtimsClassCode: *in.EtimsClassCode, TaxCategory: *in.TaxCategory, Unit: unit, PriceCents: price})
		return err
	})
	if err != nil {
		fail(w, err, "Could not create the item")
		return
	}
	httpx.JSON(w, http.StatusCreated, toItem(out))
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in itemInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be a partial item")
		return
	}
	if err := in.validate(false); err != nil {
		fail(w, err, "")
		return
	}
	var out gen.Item
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = tx.UpdateItem(ctx, gen.UpdateItemParams{ID: id, Name: in.Name, EtimsClassCode: in.EtimsClassCode, TaxCategory: in.TaxCategory, Unit: in.Unit, PriceCents: in.PriceCents, IsActive: in.IsActive})
		return err
	})
	if err != nil {
		fail(w, err, "Could not update the item")
		return
	}
	httpx.JSON(w, http.StatusOK, toItem(out))
}

// -------------------------------------------------------------- sales

func (h *Handler) listSales(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	limit, offset := page(r)
	out := []SaleView{}
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListSales(ctx, gen.ListSalesParams{OrgID: orgID, Limit: limit, Offset: offset, Status: optString(r.URL.Query().Get("status"))})
		for _, s := range rows {
			out = append(out, toSale(s, nil))
		}
		return err
	})
	if err != nil {
		fail(w, err, "Could not list sales")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "limit": limit, "offset": offset})
}

func (h *Handler) getSale(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var out SaleView
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		s, err := tx.GetSale(ctx, id)
		if err != nil {
			return err
		}
		lines, err := tx.ListSaleItems(ctx, id)
		if err != nil {
			return err
		}
		out = toSale(s, lines)
		return nil
	})
	if err != nil {
		fail(w, err, "Could not load the sale")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

type saleLineInput struct {
	ItemID         *uuid.UUID `json:"item_id"`
	Description    string     `json:"description"`
	EtimsClassCode string     `json:"etims_class_code"`
	Qty            string     `json:"qty"`
	UnitPriceCents int64      `json:"unit_price_cents"`
	TaxCategory    string     `json:"tax_category"`
}

type saleInput struct {
	Lines          []saleLineInput `json:"lines"`
	CustomerMSISDN string          `json:"customer_msisdn"`
	CustomerName   string          `json:"customer_name"`
	ClientRef      *string         `json:"client_ref"`
	// Paid records a cash sale already settled outside M-Pesa (invoice issued now).
	Paid bool `json:"paid"`
}

// createSale records an open sale (to be matched by BillRef = ref) or, with
// paid=true, a settled sale that is invoiced immediately.
func (h *Handler) createSale(w http.ResponseWriter, r *http.Request) {
	orgID, userID := org(r)
	var in saleInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {lines[], customer_msisdn?, customer_name?, client_ref?, paid?}")
		return
	}
	if len(in.Lines) == 0 || len(in.Lines) > 50 {
		fail(w, invalid("lines must contain 1–50 entries"), "")
		return
	}
	var out SaleView
	var inv *gen.Invoice
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		var customerID *uuid.UUID
		if in.CustomerMSISDN != "" {
			norm, err := crypto.NormaliseMSISDN(in.CustomerMSISDN)
			if err != nil {
				return invalid("customer_msisdn is not a valid Kenyan number")
			}
			enc, err := h.Keys.EncryptString(norm)
			if err != nil {
				return err
			}
			c, err := tx.UpsertCustomerByMSISDN(ctx, gen.UpsertCustomerByMSISDNParams{OrgID: orgID, Name: in.CustomerName, MsisdnEnc: enc, MsisdnHash: h.Keys.Hash(norm)})
			if err != nil {
				return err
			}
			customerID = &c.ID
		}
		type line struct {
			saleLineInput
			item     *gen.Item
			cat      fiscal.TaxCategory
			qty      int64
			total    int64
			tax      int64
			position int32
		}
		lines := make([]line, 0, len(in.Lines))
		var subtotal, tax int64
		for i, l := range in.Lines {
			ln := line{saleLineInput: l, position: int32(i)}
			if l.ItemID != nil {
				it, err := tx.GetItem(ctx, *l.ItemID)
				if err != nil {
					return invalid("lines[%d].item_id does not exist", i)
				}
				ln.item = &it
				if l.Description == "" {
					ln.Description = it.Name
				}
				if l.TaxCategory == "" {
					ln.TaxCategory = it.TaxCategory
				}
				if l.UnitPriceCents == 0 {
					ln.UnitPriceCents = it.PriceCents
				}
				if l.EtimsClassCode == "" {
					ln.EtimsClassCode = it.EtimsClassCode
				}
			}
			if ln.Description == "" {
				return invalid("lines[%d].description is required", i)
			}
			if ln.EtimsClassCode == "" {
				return invalid("lines[%d].etims_class_code is required when item_id is not given", i)
			}
			cat, err := fiscal.ParseTaxCategory(ln.TaxCategory)
			if err != nil {
				return invalid("lines[%d].tax_category must be one of A, B, C, D, E", i)
			}
			ln.cat = cat
			if ln.Qty == "" {
				ln.Qty = "1"
			}
			q, err := strconv.ParseInt(ln.Qty, 10, 64)
			if err != nil || q <= 0 {
				return invalid("lines[%d].qty must be a positive whole number", i)
			}
			if ln.UnitPriceCents <= 0 {
				return invalid("lines[%d].unit_price_cents must be > 0", i)
			}
			ln.qty, ln.total = q, q*ln.UnitPriceCents
			ln.tax = fiscal.LineTax(ln.total, cat.RateBP())
			subtotal += ln.total - ln.tax
			tax += ln.tax
			lines = append(lines, ln)
		}
		ref, err := tx.NextSaleRef(ctx, orgID)
		if err != nil {
			return err
		}
		status, kind := "open", "open"
		var paidAt *time.Time
		if in.Paid {
			status, kind = "paid", "cash"
			paidAt = db.Ptr(h.S.Now())
		}
		sale, err := tx.CreateSale(ctx, gen.CreateSaleParams{OrgID: orgID, Ref: ref, Kind: kind, Status: status, CustomerID: customerID,
			SubtotalCents: subtotal, TaxCents: tax, TotalCents: subtotal + tax, ClientRef: in.ClientRef, CreatedBy: &userID, PaidAt: paidAt})
		if err != nil {
			return err
		}
		items := make([]gen.SaleItem, 0, len(lines))
		for _, ln := range lines {
			unit := "PCS"
			if ln.item != nil {
				unit = ln.item.Unit
			}
			it, err := tx.CreateSaleItem(ctx, gen.CreateSaleItemParams{OrgID: orgID, SaleID: sale.ID, ItemID: ln.ItemID, Description: ln.Description, EtimsClassCode: ln.EtimsClassCode, Unit: unit,
				Qty: strconv.FormatInt(ln.qty, 10), UnitPriceCents: ln.UnitPriceCents, TaxCategory: string(ln.cat), TaxRateBp: int32(ln.cat.RateBP()),
				LineTotalCents: ln.total, LineTaxCents: ln.tax, Position: ln.position})
			if err != nil {
				return err
			}
			items = append(items, it)
		}
		if in.Paid {
			created, err := h.S.CreateInvoiceForSale(ctx, tx, orgID, sale.ID, nil, *paidAt)
			if err != nil {
				return err
			}
			inv = &created
		}
		out = toSale(sale, items)
		actor := userID.String()
		return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "user", ActorID: &actor, Action: "sale.created", Entity: "sale", EntityID: sale.ID.String()})
	})
	if err != nil {
		fail(w, err, "Could not create the sale")
		return
	}
	resp := map[string]any{"sale": out}
	if inv != nil {
		resp["invoice"] = toInvoice(h.Keys, h.PublicBaseURL, *inv)
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

// ----------------------------------------------------------- invoices

func (h *Handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	limit, offset := page(r)
	out := []InvoiceView{}
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListInvoices(ctx, gen.ListInvoicesParams{OrgID: orgID, Limit: limit, Offset: offset, State: optString(r.URL.Query().Get("state"))})
		for _, i := range rows {
			out = append(out, toInvoice(h.Keys, h.PublicBaseURL, i))
		}
		return err
	})
	if err != nil {
		fail(w, err, "Could not list invoices")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out, "limit": limit, "offset": offset})
}

func (h *Handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var out InvoiceDetailView
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		inv, err := tx.GetInvoice(ctx, id)
		if err != nil {
			return err
		}
		lines, err := tx.ListSaleItems(ctx, inv.SaleID)
		if err != nil {
			return err
		}
		subs, err := tx.ListFiscalSubmissions(ctx, id)
		if err != nil {
			return err
		}
		o, err := tx.GetOrg(ctx, orgID)
		if err != nil {
			return err
		}
		pin, _ := h.Keys.DecryptString(o.KraPinEnc)
		out = InvoiceDetailView{InvoiceView: toInvoice(h.Keys, h.PublicBaseURL, inv), Lines: toLines(lines),
			Seller: SellerView{Name: o.Name, KRAPin: pin}, KRAQRPayload: inv.KraQrPayload, Submissions: toSubmissions(subs)}
		return nil
	})
	if err != nil {
		fail(w, err, "Could not load the invoice")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) retryInvoice(w http.ResponseWriter, r *http.Request) {
	orgID, userID := org(r)
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if h.Retrier == nil {
		httpx.Fail(w, http.StatusNotImplemented, "not_implemented", "Retry is not wired")
		return
	}
	if err := h.Retrier.Retry(r.Context(), orgID, id, userID.String()); err != nil {
		fail(w, err, "Could not re-queue the invoice")
		return
	}
	var inv gen.Invoice
	if err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		inv, err = tx.GetInvoice(ctx, id)
		return err
	}); err != nil {
		fail(w, err, "Could not load the invoice")
		return
	}
	httpx.JSON(w, http.StatusAccepted, toInvoice(h.Keys, h.PublicBaseURL, inv))
}

// ---------------------------------------------------------- attention

func (h *Handler) attention(w http.ResponseWriter, r *http.Request) {
	orgID, _ := org(r)
	out := AttentionView{FailedInvoices: []InvoiceView{}, UnmatchedPayments: []PaymentView{}, UnverifiedShortcodes: []ShortcodeView{}, PendingLong: []InvoiceView{}}
	pendingAfter := h.PendingLongAfter
	if pendingAfter == 0 {
		pendingAfter = 10 * time.Minute
	}
	err := h.S.DB.WithOrg(r.Context(), orgID, func(ctx context.Context, tx db.Tx) error {
		for _, st := range []string{string(fiscal.StateNeedsReview), string(fiscal.StateFailedTerminal)} {
			rows, err := tx.ListInvoices(ctx, gen.ListInvoicesParams{OrgID: orgID, Limit: 100, State: &st})
			if err != nil {
				return err
			}
			for _, i := range rows {
				out.FailedInvoices = append(out.FailedInvoices, toInvoice(h.Keys, h.PublicBaseURL, i))
			}
		}
		for _, st := range []string{StatusUnmatched, StatusPartial} {
			rows, err := tx.ListPayments(ctx, gen.ListPaymentsParams{OrgID: orgID, Limit: 100, Status: &st})
			if err != nil {
				return err
			}
			for _, p := range rows {
				out.UnmatchedPayments = append(out.UnmatchedPayments, toPayment(h.Keys, p, nil))
			}
		}
		scs, err := tx.ListShortcodes(ctx, orgID)
		if err != nil {
			return err
		}
		for _, s := range scs {
			if s.Status != ShortcodeVerified {
				out.UnverifiedShortcodes = append(out.UnverifiedShortcodes, toShortcode(s))
			}
		}
		cutoff := h.S.Now().Add(-pendingAfter)
		for _, st := range []string{string(fiscal.StateQueued), string(fiscal.StateSubmitted), string(fiscal.StateFailedRetryable)} {
			rows, err := tx.ListInvoices(ctx, gen.ListInvoicesParams{OrgID: orgID, Limit: 100, State: &st})
			if err != nil {
				return err
			}
			for _, i := range rows {
				if i.CreatedAt.Before(cutoff) {
					out.PendingLong = append(out.PendingLong, toInvoice(h.Keys, h.PublicBaseURL, i))
				}
			}
		}
		return nil
	})
	if err != nil {
		fail(w, err, "Could not build the attention list")
		return
	}
	out.ActionableCount = len(out.FailedInvoices) + len(out.UnmatchedPayments) + len(out.UnverifiedShortcodes)
	httpx.JSON(w, http.StatusOK, out)
}
