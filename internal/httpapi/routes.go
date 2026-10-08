package httpapi

import (
	httpSwagger "github.com/swaggo/http-swagger"
	"net/http"
)

func NewRouter(h *Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/shorten", h.Shorten)
	mux.HandleFunc("GET /{code}", h.Redirect)


	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	return mux
}
