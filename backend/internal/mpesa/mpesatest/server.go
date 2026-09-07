// Package mpesatest is an in-process fake of the Daraja sandbox: OAuth,
// C2B RegisterURL and the C2B simulator. `simulate` posts a confirmation to
// whichever ConfirmationURL was registered for the shortcode, so a test (or
// `make daraja-fake` + ciftctl) can drive the whole verification loop without
// touching Safaricom. It is deliberately loose about auth: any Basic header
// gets a token, any Bearer token is accepted.
package mpesatest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Registration is what RegisterURL stored for a shortcode.
type Registration struct {
	ShortCode       string `json:"ShortCode"`
	ResponseType    string `json:"ResponseType"`
	ConfirmationURL string `json:"ConfirmationURL"`
	ValidationURL   string `json:"ValidationURL"`
}

// Simulation is one C2B the fake emitted.
type Simulation struct {
	TransID       string
	ShortCode     string
	MSISDN        string
	AmountCents   int64
	BillRef       string
	DeliveredTo   string
	DeliveryError string
}

// Server is the fake. Zero value is not usable; use New.
type Server struct {
	mu            sync.Mutex
	registrations map[string]Registration
	simulations   []Simulation
	seq           int
	// Now supplies TransTime; override in tests for determinism.
	Now func() time.Time
	// Deliver posts confirmations; defaults to http.DefaultClient.Do.
	Deliver func(*http.Request) (*http.Response, error)
	// Handler is the mux; mount it on any server.
	Handler http.Handler
}

// New builds the fake.
func New() *Server {
	s := &Server{registrations: map[string]Registration{}, Now: time.Now, Deliver: http.DefaultClient.Do}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v1/generate", s.oauth)
	mux.HandleFunc("/mpesa/c2b/v1/registerurl", s.registerURL)
	mux.HandleFunc("/mpesa/c2b/v1/simulate", s.simulate)
	s.Handler = mux
	return s
}

// Start runs the fake on an httptest server; the caller closes it.
func Start() (*Server, *httptest.Server) {
	s := New()
	return s, httptest.NewServer(s.Handler)
}

// Registrations returns a copy of everything RegisterURL received.
func (s *Server) Registrations() []Registration {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Registration, 0, len(s.registrations))
	for _, r := range s.registrations {
		out = append(out, r)
	}
	return out
}

// Registration returns what was registered for shortcode, if anything.
func (s *Server) Registration(shortcode string) (Registration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.registrations[shortcode]
	return r, ok
}

// Simulations returns a copy of every emitted confirmation.
func (s *Server) Simulations() []Simulation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Simulation(nil), s.simulations...)
}

func (s *Server) oauth(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"errorCode": "400.008.01", "errorMessage": "Invalid Authentication passed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"access_token": "fake-token-" + strconv.FormatInt(time.Now().UnixNano(), 36), "expires_in": "3599"})
}

func (s *Server) requireBearer(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"errorCode": "404.001.03", "errorMessage": "Invalid Access Token"})
		return false
	}
	return true
}

func (s *Server) registerURL(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	var in Registration
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ShortCode == "" || in.ConfirmationURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"errorCode": "400.002.02", "errorMessage": "Bad Request - Invalid ShortCode/ConfirmationURL"})
		return
	}
	s.mu.Lock()
	s.registrations[in.ShortCode] = in
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"OriginatorCoversationID": "fake-" + in.ShortCode, "ResponseCode": "0", "ResponseDescription": "Success"})
}

// simulate mirrors POST /mpesa/c2b/v1/simulate and, like the sandbox, calls
// the registered ConfirmationURL synchronously.
func (s *Server) simulate(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	var in struct {
		ShortCode     string `json:"ShortCode"`
		CommandID     string `json:"CommandID"`
		Amount        any    `json:"Amount"`
		Msisdn        string `json:"Msisdn"`
		BillRefNumber string `json:"BillRefNumber"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ShortCode == "" || in.Msisdn == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"errorCode": "400.002.02", "errorMessage": "Bad Request - Invalid ShortCode/Msisdn"})
		return
	}
	amount, err := parseAmountCents(in.Amount)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"errorCode": "400.002.02", "errorMessage": "Bad Request - Invalid Amount"})
		return
	}
	sim, err := s.Emit(r.Context(), in.ShortCode, in.Msisdn, amount, in.BillRefNumber)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"errorCode": "500.003.1001", "errorMessage": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"OriginatorCoversationID": sim.TransID, "ResponseCode": "0", "ResponseDescription": "Accept the service request successfully."})
}

// Emit builds a C2B confirmation and posts it to the ConfirmationURL
// registered for shortcode. Exposed so tests can bypass HTTP.
func (s *Server) Emit(ctx context.Context, shortcode, msisdn string, amountCents int64, billRef string) (Simulation, error) {
	s.mu.Lock()
	reg, ok := s.registrations[shortcode]
	s.seq++
	transID := fmt.Sprintf("FAKE%06d%s", s.seq, strings.ToUpper(strconv.FormatInt(time.Now().UnixNano()%1296, 36)))
	s.mu.Unlock()
	sim := Simulation{TransID: transID, ShortCode: shortcode, MSISDN: msisdn, AmountCents: amountCents, BillRef: billRef}
	if !ok {
		sim.DeliveryError = "no ConfirmationURL registered for " + shortcode
		s.record(sim)
		return sim, fmt.Errorf("mpesa fake: %s", sim.DeliveryError)
	}
	payload := map[string]string{
		"TransactionType":   "Pay Bill",
		"TransID":           transID,
		"TransTime":         s.Now().In(nairobi()).Format("20060102150405"),
		"TransAmount":       fmt.Sprintf("%d.%02d", amountCents/100, amountCents%100),
		"BusinessShortCode": shortcode,
		"BillRefNumber":     billRef,
		"InvoiceNumber":     "",
		"OrgAccountBalance": "0.00",
		"ThirdPartyTransID": "",
		"MSISDN":            msisdn,
		"FirstName":         "FAKE",
		"MiddleName":        "",
		"LastName":          "PAYER",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reg.ConfirmationURL, bytes.NewReader(body))
	if err != nil {
		sim.DeliveryError = err.Error()
		s.record(sim)
		return sim, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Deliver(req)
	if err != nil {
		sim.DeliveryError = err.Error()
		s.record(sim)
		return sim, fmt.Errorf("mpesa fake: deliver confirmation: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	sim.DeliveredTo = reg.ConfirmationURL
	if resp.StatusCode != http.StatusOK {
		sim.DeliveryError = "confirmation url returned " + resp.Status
	}
	s.record(sim)
	return sim, nil
}

func (s *Server) record(sim Simulation) {
	s.mu.Lock()
	s.simulations = append(s.simulations, sim)
	s.mu.Unlock()
}

func parseAmountCents(v any) (int64, error) {
	switch a := v.(type) {
	case float64:
		return int64(a*100 + 0.5), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(a), 64)
		if err != nil {
			return 0, err
		}
		return int64(f*100 + 0.5), nil
	case nil:
		return 0, fmt.Errorf("amount missing")
	}
	return 0, fmt.Errorf("amount has unexpected type %T", v)
}

func nairobi() *time.Location {
	loc, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		return time.FixedZone("EAT", 3*3600)
	}
	return loc
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
