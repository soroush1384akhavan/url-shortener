package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/link"
)

type Handler struct {
	service link.Shortener
	baseURL string
	usage   link.UsageRecorder
}

func NewHandler(service link.Shortener, baseURL string, usage link.UsageRecorder) *Handler {
	return &Handler{
		service: service,
		baseURL: strings.TrimRight(baseURL, "/"),
		usage:   usage,
	}
}

type shortenRequest struct {
	URL string `json:"url"`
}

type shortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

// Shorten godoc
// @Summary Shorten a URL
// @Tags links
// @Accept json
// @Produce json
// @Param request body shortenRequest true "URL to shorten"
// @Success 201 {object} shortenResponse
// @Failure 400 {string} string
// @Router /api/shorten [post]
func (h *Handler) Shorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	sl, err := h.service.Shorten(ctx, req.URL)
	if err != nil {
		if errors.Is(err, link.ErrInvalidURL) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("shorten failed: %v", err) // clinet shouldn't know about actual error
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	response := shortenResponse{
		Code:     sl.Code,
		ShortURL: h.baseURL + "/" + sl.Code,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to encode shorten response: %v", err)
	}
}

// Redirect godoc
// @Summary Redirect to the original URL
// @Description Redirects a short code to its original long URL
// @Tags links
// @Param code path string true "Short code"
// @Success 302
// @Failure 404 {string} string "Link not found"
// @Router /{code} [get]
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	code := r.PathValue("code")

	if len(code) < 6 || len(code) > 8 {
		http.Error(w, "link not found", http.StatusNotFound)
		return
	}

	sl, err := h.service.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			http.Error(w, "link not found", http.StatusNotFound)
			return
		}

		log.Printf("get by code failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// this ->
	// h.usageCounter.Record(code)
	// if err := h.service.IncrementUsedCount(ctx, code, 1); err != nil { // i know its to heavy but m going to fix this
	// 	log.Printf("increment used count failed: %v", err) // best effort its not strict
	// }

	if h.usage != nil {
		h.usage.Record(code)
	}

	http.Redirect(w, r, sl.LongURL, http.StatusFound)
}

type GetMetadataResponse struct {
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// GetMetadata godoc
// @Summary Get link metadata
// @Description Returns metadata for a shortened link by code
// @Tags links
// @Produce json
// @Param code path string true "Short code"
// @Success 200 {object} GetMetadataResponse
// @Failure 404 {string} string "Link not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/links/{code} [get]
func (h *Handler) GetMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	code := r.PathValue("code")

	if len(code) < 6 || len(code) > 8 {
		http.Error(w, "link not found", http.StatusNotFound)
		return
	}

	sl, err := h.service.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			http.Error(w, "link not found", http.StatusNotFound)
			return
		}

		log.Printf("get by code failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	response := GetMetadataResponse{
		URL:       sl.LongURL,
		CreatedAt: sl.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to encode GetMetaData response: %v", err)
	}

}
