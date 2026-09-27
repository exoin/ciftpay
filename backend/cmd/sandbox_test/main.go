package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/fiscal/oscu"
)

// loadEnv loads key-value pairs from .env files if present.
func loadEnv() {
	paths := []string{
		".env",
		"../.env",
		"../../.env",
	}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)
				if os.Getenv(k) == "" && v != "" {
					os.Setenv(k, v)
				}
			}
		}
		abs, _ := filepath.Abs(p)
		fmt.Printf("[sandbox_test] Checked env from %s\n", abs)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	loadEnv()

	consumerKey := os.Getenv("KRA_OSCU_CONSUMER_KEY")
	consumerSecret := os.Getenv("KRA_OSCU_CONSUMER_SECRET")
	baseURL := getEnv("KRA_OSCU_BASE_URL", "https://sbx.kra.go.ke")
	apiBaseURL := getEnv("KRA_OSCU_API_BASE_URL", "https://etims-api-sbx.kra.go.ke")
	deviceSerial := os.Getenv("KRA_OSCU_DEVICE_SERIAL")
	if deviceSerial == "" {
		deviceSerial = os.Getenv("KRA_OSCU_TEST_DEVICE_SERIAL")
	}
	testPIN := os.Getenv("KRA_OSCU_TEST_PIN")
	testBhfID := getEnv("KRA_OSCU_TEST_BHF_ID", "00")
	dnsResolver := getEnv("KRA_OSCU_DNS_RESOLVER", "8.8.8.8:53")

	fmt.Println("================================================================")
	fmt.Println("KRA OSCU Sandbox Idempotency Test")
	fmt.Println("================================================================")
	fmt.Printf("BaseURL:        %s\n", baseURL)
	fmt.Printf("APIBaseURL:     %s\n", apiBaseURL)
	fmt.Printf("ConsumerKey:    %s\n", mask(consumerKey))
	fmt.Printf("ConsumerSecret: %s\n", mask(consumerSecret))
	fmt.Printf("DeviceSerial:   %s\n", deviceSerial)
	fmt.Printf("TestPIN:        %s\n", testPIN)
	fmt.Printf("TestBhfID:      %s\n", testBhfID)
	fmt.Printf("DNSResolver:    %s\n", dnsResolver)
	fmt.Println("----------------------------------------------------------------")

	if consumerKey == "" || consumerSecret == "" {
		fmt.Fprintln(os.Stderr, "ERROR: KRA_OSCU_CONSUMER_KEY and KRA_OSCU_CONSUMER_SECRET must be set in .env.")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := oscu.Config{
		BaseURL:        baseURL,
		APIBaseURL:     apiBaseURL,
		ConsumerKey:    consumerKey,
		ConsumerSecret: consumerSecret,
		DeviceSerial:   deviceSerial,
		DNSResolver:    dnsResolver,
		Timeout:        30 * time.Second,
	}

	client, err := oscu.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create oscu.Client: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[Step 0] Testing OAuth Token generation with credentials...")
	tok, err := client.Token(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OAuth Token Generation FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("OAuth Token SUCCESS! Token length: %d chars (Token preview: %s...)\n", len(tok), tok[:min(10, len(tok))])

	if testPIN == "" || deviceSerial == "" {
		fmt.Println("\n----------------------------------------------------------------")
		fmt.Println("BLOCKED: To submit invoices to KRA OSCU Sandbox, KRA requires:")
		if testPIN == "" {
			fmt.Println(" - KRA_OSCU_TEST_PIN: The taxpayer PIN registered on sandbox (e.g., P051234567A)")
		}
		if deviceSerial == "" {
			fmt.Println(" - KRA_OSCU_DEVICE_SERIAL (or KRA_OSCU_TEST_DEVICE_SERIAL): The device serial number issued by KRA eTIMS")
		}
		fmt.Println("Please provide them in .env so device registration and invoice submission can run.")
		os.Exit(1)
	}

	fmt.Println("\n[Step 1] Initializing device (selectInitOsdcInfo)...")
	info, rawInit, err := client.Initialize(ctx, testPIN, testBhfID, deviceSerial)
	fmt.Printf("KRA Raw selectInitOsdcInfo Response: %s\n", string(rawInit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to register device: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Device registered successfully! DeviceID: %s, BranchID: %s, Taxpayer: %s\n", info.DeviceID, info.BranchID, info.TaxpayerName)

	rawProfile, _ := json.Marshal(map[string]string{
		"device_id":     info.DeviceID,
		"branch_id":     info.BranchID,
		"device_serial": deviceSerial,
		"cmc_key":       info.CmcKey,
		"sdc_id":        info.SDCID,
		"mrc_no":        info.MRCNo,
		"taxpayer_name": info.TaxpayerName,
	})

	prov, err := oscu.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create oscu.Provider: %v\n", err)
		os.Exit(1)
	}

	hardcodedUUID := uuid.MustParse("00000000-0000-0000-0000-123456789abc")

	doc := fiscal.Invoice{
		ID:            hardcodedUUID.String(),
		OrgID:         "test-org",
		SellerPIN:     testPIN,
		SellerName:    info.TaxpayerName,
		BranchID:      testBhfID,
		IssuedAt:      time.Now().UTC(),
		SubtotalCents: 1000,
		TaxCents:      160,
		TotalCents:    1160,
		PaymentMethod: "MOBILE_MONEY",
		DeviceProfile: rawProfile,
		Lines: []fiscal.Line{
			{
				ItemCode:       "5020230000",
				Description:    "Test Item 1",
				Qty:            "1",
				Unit:           "PCS",
				UnitPriceCents: 1160,
				TaxCategory:    fiscal.TaxStandard,
				LineTaxCents:   160,
				LineTotalCents: 1160,
			},
		},
	}

	fmt.Printf("\n[Step 2] Calling SubmitInvoice (First Call) with UUID: %s...\n", doc.ID)
	ack1, err := prov.SubmitInvoice(ctx, doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SubmitInvoice (First Call) FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("First call SUCCEEDED!\n")
	fmt.Printf("  rcptNo:   %s\n", ack1.KRAInvoiceNo)
	fmt.Printf("  rcptSign: %s\n", ack1.Signature)
	fmt.Printf("  raw:      %s\n", string(ack1.Raw))

	fmt.Println("\n[Step 3] Sleeping for 5 seconds...")
	time.Sleep(5 * time.Second)

	fmt.Printf("\n[Step 4] Calling SubmitInvoice (Second Call - same Provider instance) with exact same UUID: %s...\n", doc.ID)
	ack2, err2 := prov.SubmitInvoice(ctx, doc)
	if err2 != nil {
		fmt.Printf("Second call returned error: %v\n", err2)
	} else {
		fmt.Printf("Second call SUCCEEDED!\n")
		fmt.Printf("  rcptNo:   %s\n", ack2.KRAInvoiceNo)
		fmt.Printf("  rcptSign: %s\n", ack2.Signature)
		fmt.Printf("  raw:      %s\n", string(ack2.Raw))
	}

	// Also perform a live HTTP test bypassing client-side in-memory cache
	// to see how the actual KRA OSCU Sandbox endpoint behaves on duplicate submission
	fmt.Printf("\n[Step 5] Direct HTTP call to KRA OSCU Sandbox endpoint with exact same UUID to test remote API idempotency...\n")
	directRaw, directStatus, directErr := sendDirectWireSale(ctx, cfg, doc, rawProfile)
	if directErr != nil {
		fmt.Printf("Direct HTTP call returned error: %v\n", directErr)
	}
	fmt.Printf("Direct HTTP Status: %d\n", directStatus)
	fmt.Printf("Direct HTTP Raw Response: %s\n", string(directRaw))

	fmt.Println("\n================================================================")
	fmt.Println("Test Summary:")
	if err2 == nil && ack1.KRAInvoiceNo == ack2.KRAInvoiceNo {
		fmt.Printf("Provider Call Outcome: Matched receipt number %s\n", ack2.KRAInvoiceNo)
	}
	fmt.Println("================================================================")
}

func sendDirectWireSale(ctx context.Context, cfg oscu.Config, inv fiscal.Invoice, devProfileRaw []byte) ([]byte, int, error) {
	client, err := oscu.NewClient(cfg)
	if err != nil {
		return nil, 0, err
	}
	tok, err := client.Token(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("token error: %w", err)
	}

	var prof struct {
		BranchID string `json:"branch_id"`
		CmcKey   string `json:"cmc_key"`
	}
	_ = json.Unmarshal(devProfileRaw, &prof)

	branch := prof.BranchID
	if branch == "" {
		branch = inv.BranchID
	}
	if branch == "" {
		branch = "00"
	}

	type wireLine struct {
		ItemSeq  int    `json:"itemSeq"`
		ItemCd   string `json:"itemCd"`
		ItemNm   string `json:"itemNm"`
		Qty      string `json:"qty"`
		Prc      int64  `json:"prc"`
		SplyAmt  int64  `json:"splyAmt"`
		TaxTyCd  string `json:"taxTyCd"`
		TaxblAmt int64  `json:"taxblAmt"`
		TaxAmt   int64  `json:"taxAmt"`
		TotAmt   int64  `json:"totAmt"`
	}
	type wireSale struct {
		TIN         string     `json:"tin"`
		BhfID       string     `json:"bhfId"`
		InvcNo      string     `json:"invcNo"`
		OrgInvcNo   string     `json:"orgInvcNo,omitempty"`
		CustTin     string     `json:"custTin,omitempty"`
		CustNm      string     `json:"custNm,omitempty"`
		RcptTyCd    string     `json:"rcptTyCd"`
		PmtTyCd     string     `json:"pmtTyCd"`
		SalesDt     string     `json:"salesDt"`
		TotItemCnt  int        `json:"totItemCnt"`
		TotTaxblAmt int64      `json:"totTaxblAmt"`
		TotTaxAmt   int64      `json:"totTaxAmt"`
		TotAmt      int64      `json:"totAmt"`
		ItemList    []wireLine `json:"itemList"`
	}

	w := wireSale{
		TIN: inv.SellerPIN, BhfID: branch, InvcNo: inv.ID,
		RcptTyCd: "S", PmtTyCd: "04", SalesDt: inv.IssuedAt.UTC().Format("20060102"),
		TotItemCnt: len(inv.Lines), TotTaxblAmt: inv.SubtotalCents, TotTaxAmt: inv.TaxCents, TotAmt: inv.TotalCents,
		ItemList: []wireLine{
			{
				ItemSeq: 1, ItemCd: inv.Lines[0].ItemCode, ItemNm: inv.Lines[0].Description,
				Qty: inv.Lines[0].Qty, Prc: inv.Lines[0].UnitPriceCents, SplyAmt: inv.Lines[0].LineTotalCents - inv.Lines[0].LineTaxCents,
				TaxTyCd: string(inv.Lines[0].TaxCategory), TaxblAmt: inv.Lines[0].LineTotalCents - inv.Lines[0].LineTaxCents,
				TaxAmt: inv.Lines[0].LineTaxCents, TotAmt: inv.Lines[0].LineTotalCents,
			},
		},
	}

	body, err := json.Marshal(w)
	if err != nil {
		return nil, 0, err
	}

	apiBase := cfg.APIBaseURL
	if apiBase == "" {
		apiBase = "https://etims-api-sbx.kra.go.ke"
	}
	url := strings.TrimRight(apiBase, "/") + "/etims-api/insertTrnsSalesReq"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("tin", w.TIN)
	req.Header.Set("bhfId", w.BhfID)
	req.Header.Set("cmcKey", prof.CmcKey)

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mask(s string) string {
	if len(s) == 0 {
		return "(empty)"
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}
