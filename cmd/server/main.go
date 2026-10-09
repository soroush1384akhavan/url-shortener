package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/soroush1384akhavan/url-shortener/docs"
	"github.com/soroush1384akhavan/url-shortener/internal/httpapi"
	"github.com/soroush1384akhavan/url-shortener/internal/link"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
	"github.com/soroush1384akhavan/url-shortener/internal/store"
)

// @title URL Shortener API
// @version 1.0
// @description Simple URL shortener API
// @host localhost:8080
// @BasePath /
func main() {
	addr := flag.String("addr", ":8080", "server listen address")
	base := flag.String("base", "http://localhost:8080", "base URL for short links")

	flag.Parse()

	vldt := link.URLValidator{}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	service := link.NewShortenerService(vldt, st, gn)

	handler := httpapi.NewHandler(service, *base)

	router := httpapi.NewRouter(handler)

	server := &http.Server{
		Addr:              *addr,
		Handler:           router,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("server listening on %s", *addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Fatalf("server failed: %v", err)
	case <-ctx.Done():
		log.Println("shutting down...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		return
	}
	log.Println("server stopped")
}
