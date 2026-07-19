package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Handler holds the HTTP transport for the orders example. It decodes and
// validates requests, delegates to the service, and renders uniform JSON
// responses. Domain errors are mapped to status codes in one place (writeError).
type Handler struct {
	service *OrderService
	logger  *slog.Logger
}

// NewHandler builds the HTTP handler over the given service.
func NewHandler(service *OrderService, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// Routes registers the example's endpoints onto the given router.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/products", h.createProduct)
	r.Get("/products/{id}", h.getProduct)
	r.Post("/orders", h.createOrder)
	r.Get("/orders/{id}", h.getOrder)
}

// --- request DTOs ---

type createProductRequest struct {
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	Stock      int64  `json:"stock"`
}

type createOrderRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
}

// --- handlers ---

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req createProductRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.Name == "" {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.PriceCents < 0 {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "price_cents must not be negative")
		return
	}
	if req.Stock < 0 {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "stock must not be negative")
		return
	}

	product, err := h.service.CreateProduct(r.Context(), req.Name, req.PriceCents, req.Stock)
	if err != nil {
		h.renderServiceError(w, err)
		return
	}

	writeJSON(w, h.logger, http.StatusCreated, product)
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "id must be a positive integer")
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		h.renderServiceError(w, err)
		return
	}

	writeJSON(w, h.logger, http.StatusOK, product)
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if req.ProductID <= 0 {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "product_id is required")
		return
	}
	if req.Quantity <= 0 {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "quantity must be greater than zero")
		return
	}

	order, err := h.service.CreateOrder(r.Context(), req.ProductID, req.Quantity)
	if err != nil {
		h.renderServiceError(w, err)
		return
	}

	writeJSON(w, h.logger, http.StatusCreated, order)
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_request", "id must be a positive integer")
		return
	}

	order, err := h.service.GetOrder(r.Context(), id)
	if err != nil {
		h.renderServiceError(w, err)
		return
	}

	writeJSON(w, h.logger, http.StatusOK, order)
}

// renderServiceError maps domain errors returned by the service to HTTP status
// codes and a uniform error envelope. Unmapped errors become a 500 with the
// detail logged rather than leaked to the client.
func (h *Handler) renderServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, h.logger, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, ErrInsufficientStock):
		writeError(w, h.logger, http.StatusConflict, "insufficient_stock", err.Error())
	default:
		h.logger.Error("unhandled service error", "error", err)
		writeError(w, h.logger, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

// --- helpers ---

// decodeJSON strictly decodes a JSON request body, rejecting unknown fields.
func decodeJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// pathID parses the {id} URL parameter as a positive int64.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// errorEnvelope is the uniform error response body: {"error":{"code","message"}}.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string) {
	writeJSON(w, logger, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("failed to encode response", "error", err)
	}
}
