package gateway

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/exoin/ciftpay/internal/platform/httpx"
)

// Handler serves GET /kra/pin/{pin} and GET /kra/obligations/{pin}.
type Handler struct {
	Client *Client
}

// Mount registers routes on chi.Router.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/kra/pin/{pin}", h.getTaxpayerByPIN)
	r.Get("/kra/obligations/{pin}", h.getObligationsByPIN)
}

func (h *Handler) getTaxpayerByPIN(w http.ResponseWriter, r *http.Request) {
	pin := chi.URLParam(r, "pin")
	taxpayer, err := h.Client.CheckPIN(r.Context(), pin)
	switch {
	case errors.Is(err, ErrPINInvalid):
		httpx.Fail(w, http.StatusBadRequest, "invalid_pin", "KRA PIN must match format A/P + 9 digits + letter")
	case errors.Is(err, ErrPINNotFound):
		httpx.Fail(w, http.StatusNotFound, "pin_not_found", "PIN not found in KRA registry")
	case err != nil:
		httpx.Fail(w, http.StatusBadGateway, "gateway_error", err.Error())
	default:
		httpx.JSON(w, http.StatusOK, taxpayer)
	}
}

func (h *Handler) getObligationsByPIN(w http.ResponseWriter, r *http.Request) {
	pin := chi.URLParam(r, "pin")
	obligations, err := h.Client.FetchObligations(r.Context(), pin)
	switch {
	case errors.Is(err, ErrPINInvalid):
		httpx.Fail(w, http.StatusBadRequest, "invalid_pin", "KRA PIN must match format A/P + 9 digits + letter")
	case errors.Is(err, ErrPINNotFound):
		httpx.Fail(w, http.StatusNotFound, "pin_not_found", "PIN not found in KRA registry")
	case err != nil:
		httpx.Fail(w, http.StatusBadGateway, "gateway_error", err.Error())
	default:
		httpx.JSON(w, http.StatusOK, map[string]any{
			"pin":         pin,
			"obligations": obligations,
		})
	}
}
