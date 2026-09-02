// Package mpesa integrates Safaricom Daraja: the outbound client (OAuth,
// C2B RegisterURL, STK push) and the inbound webhook handlers with TransID
// idempotency. It never moves money (ADR-0003).
package mpesa

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// Nairobi is the zone Daraja timestamps are expressed in.
var Nairobi = mustZone("Africa/Nairobi")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("EAT", 3*3600)
	}
	return loc
}

// C2BPayload is the body Daraja posts to the validation and confirmation URLs.
type C2BPayload struct {
	TransactionType   string `json:"TransactionType"`
	TransID           string `json:"TransID"`
	TransTime         string `json:"TransTime"`
	TransAmount       string `json:"TransAmount"`
	BusinessShortCode string `json:"BusinessShortCode"`
	BillRefNumber     string `json:"BillRefNumber"`
	InvoiceNumber     string `json:"InvoiceNumber"`
	OrgAccountBalance string `json:"OrgAccountBalance"`
	ThirdPartyTransID string `json:"ThirdPartyTransID"`
	MSISDN            string `json:"MSISDN"`
	FirstName         string `json:"FirstName"`
	MiddleName        string `json:"MiddleName"`
	LastName          string `json:"LastName"`
}

// IsReversal reports whether this confirmation reverses ThirdPartyTransID.
func (p C2BPayload) IsReversal() bool {
	return strings.EqualFold(strings.TrimSpace(p.TransactionType), "Reversal")
}

// PayerName joins the name parts Daraja sends.
func (p C2BPayload) PayerName() string {
	parts := []string{}
	for _, s := range []string{p.FirstName, p.MiddleName, p.LastName} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// Validate checks the fields the ledger depends on.
func (p C2BPayload) Validate() error {
	if len(strings.TrimSpace(p.TransID)) < 6 {
		return fmt.Errorf("mpesa: TransID missing")
	}
	if strings.TrimSpace(p.BusinessShortCode) == "" {
		return fmt.Errorf("mpesa: BusinessShortCode missing")
	}
	if _, err := ParseAmount(p.TransAmount); err != nil {
		return err
	}
	if _, err := ParseTransTime(p.TransTime); err != nil {
		return err
	}
	return nil
}

// ParseAmount converts "2400.00" (or "2400", "2,400.5") to cents exactly.
func ParseAmount(s string) (int64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" {
		return 0, fmt.Errorf("mpesa: TransAmount missing")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("mpesa: TransAmount %q is not a number", s)
	}
	cents := new(big.Rat).Mul(r, big.NewRat(100, 1))
	if !cents.IsInt() {
		return 0, fmt.Errorf("mpesa: TransAmount %q has sub-cent precision", s)
	}
	v := cents.Num()
	if !v.IsInt64() {
		return 0, fmt.Errorf("mpesa: TransAmount %q out of range", s)
	}
	return v.Int64(), nil
}

// ParseTransTime parses YYYYMMDDHHmmss in Nairobi time and returns UTC.
func ParseTransTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	t, err := time.ParseInLocation("20060102150405", s, Nairobi)
	if err != nil {
		return time.Time{}, fmt.Errorf("mpesa: TransTime %q: %w", s, err)
	}
	return t.UTC(), nil
}

// STKCallback is the body Daraja posts after an STK push resolves.
type STKCallback struct {
	Body struct {
		StkCallback struct {
			MerchantRequestID string `json:"MerchantRequestID"`
			CheckoutRequestID string `json:"CheckoutRequestID"`
			ResultCode        int    `json:"ResultCode"`
			ResultDesc        string `json:"ResultDesc"`
			CallbackMetadata  *struct {
				Item []struct {
					Name  string          `json:"Name"`
					Value json.RawMessage `json:"Value"`
				} `json:"Item"`
			} `json:"CallbackMetadata"`
		} `json:"stkCallback"`
	} `json:"Body"`
}

// STKResult is the flattened, typed view of an STK callback.
type STKResult struct {
	MerchantRequestID string
	CheckoutRequestID string
	ResultCode        int
	ResultDesc        string
	Success           bool
	AmountCents       int64
	ReceiptNo         string
	MSISDN            string
	PaidAt            time.Time
}

// Flatten extracts the metadata items into an STKResult.
func (c STKCallback) Flatten() (STKResult, error) {
	cb := c.Body.StkCallback
	if cb.CheckoutRequestID == "" {
		return STKResult{}, fmt.Errorf("mpesa: CheckoutRequestID missing")
	}
	r := STKResult{
		MerchantRequestID: cb.MerchantRequestID,
		CheckoutRequestID: cb.CheckoutRequestID,
		ResultCode:        cb.ResultCode,
		ResultDesc:        cb.ResultDesc,
		Success:           cb.ResultCode == 0,
	}
	if cb.CallbackMetadata == nil {
		return r, nil
	}
	for _, it := range cb.CallbackMetadata.Item {
		raw := strings.Trim(string(it.Value), `"`)
		switch it.Name {
		case "Amount":
			cents, err := ParseAmount(raw)
			if err != nil {
				return r, err
			}
			r.AmountCents = cents
		case "MpesaReceiptNumber":
			r.ReceiptNo = raw
		case "PhoneNumber":
			r.MSISDN = raw
		case "TransactionDate":
			if t, err := ParseTransTime(raw); err == nil {
				r.PaidAt = t
			}
		}
	}
	return r, nil
}

// DarajaAck is the body Daraja expects from every callback URL.
type DarajaAck struct {
	ResultCode int    `json:"ResultCode"`
	ResultDesc string `json:"ResultDesc"`
}

// Accepted is the standard positive acknowledgement.
var Accepted = DarajaAck{ResultCode: 0, ResultDesc: "Accepted"}

// Rejected is the validation-URL rejection (C2B validation only).
var Rejected = DarajaAck{ResultCode: 1, ResultDesc: "Rejected"}

// ExternalID builds the webhook_events.external_id for a C2B TransID.
func ExternalID(transID string) string { return "mpesa:" + strings.ToUpper(strings.TrimSpace(transID)) }

// ExternalIDForSTK builds the external id for an STK callback.
func ExternalIDForSTK(checkoutRequestID string) string { return "mpesa:stk:" + checkoutRequestID }

// FormatCents renders cents as "2400.00" for Daraja request bodies.
func FormatCents(cents int64) string {
	return strconv.FormatInt(cents/100, 10) + "." + fmt.Sprintf("%02d", cents%100)
}
