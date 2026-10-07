package httpapi

import (
	"encoding/json"
	"net/http"

	// "example/internal/greeting"
)

type Handler struct {
	service greeting.Service
}

func NewHandler(service greeting.Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) Hello(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")

	message := h.service.Greet(name)

	response := map[string]string{
		"message": message,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}