package httpapi

import (
	httpSwagger "github.com/swaggo/http-swagger"
	"net/http"
)

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/shorten", h.Shorten)
	mux.HandleFunc("GET /{code}", h.Redirect)
	mux.HandleFunc("GET /api/v1/links/{code}", h.GetMetadata)

	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	return mux
}
