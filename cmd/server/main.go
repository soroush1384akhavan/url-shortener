package main
// new structure for testing

/// $env:DATABASE_URL="host=127.0.0.1 user=urlshortener password=urlshortener dbname=urlshortener port=5434 sslmode=disable"
import (
	"context"
	"errors"
	"flag"
	"fmt"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Getenv)
	stop()

	if err != nil {
		log.Fatal(err)
	}
}

// newStore بر اساس نوع storage، پیاده‌سازی مناسب link.Store را می‌سازد.
func newStore(storageType, dsn string) (link.Store, error) {
	switch storageType {
	case "memory":
		return store.NewMemoryStore(), nil

	case "postgres":
		if dsn == "" {
			return nil, errors.New("DATABASE_URL is required when storage=postgres")
		}
		pg, err := store.NewPostgresStore(dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize postgres store: %w", err)
		}
		return pg, nil

	default:
		return nil, fmt.Errorf("unknown storage type: %s", storageType)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string) error {
	fs := flag.NewFlagSet("url-shortener", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "server listen address")
	base := fs.String("base", "http://localhost:8080", "base URL for short links")
	storageType := fs.String("storage", "memory", "storage backend: memory or postgres")

	if err := fs.Parse(args); err != nil {
		return err
	}

	st, err := newStore(*storageType, getenv("DATABASE_URL"))
	if err != nil {
		return err
	}

	service := link.NewShortenerService(link.URLValidator{}, st, shortcode.Base62Generator{})
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

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("server listening on %s", *addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server failed: %w", err)
	case <-ctx.Done():
		log.Println("shutting down...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	log.Println("server stopped")
	return nil
}