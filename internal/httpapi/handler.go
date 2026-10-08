package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/soroush1384akhavan/url-shortener/internal/link"
)

type Handler struct {
	service *link.ShortenerService
	baseURL string
}

func NewHandler(service *link.ShortenerService, baseURL string) *Handler {
	return &Handler{
		service: service,
		baseURL: strings.TrimRight(baseURL, "/"),
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

	sl, err := h.service.Shorten(req.URL)
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

	code := r.PathValue("code")

	if len(code) < 6 || len(code) > 8 {
		http.Error(w, "link not found", http.StatusNotFound)
		return
	}

	sl, err := h.service.GetByCode(code)
	if err != nil {
		if errors.Is(err, link.ErrNotFound) {
			http.Error(w, "link not found", http.StatusNotFound)
			return
		}

		log.Printf("get by code failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, sl.LongURL, http.StatusFound)

}
