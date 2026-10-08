package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/soroush1384akhavan/url-shortener/internal/domain"
	"github.com/soroush1384akhavan/url-shortener/internal/link"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
	"github.com/soroush1384akhavan/url-shortener/internal/store"
)

func newTestHandler() *Handler {
	service := link.NewShortenerService(
		link.URLValidator{},
		store.NewMemoryStore(),
		shortcode.Base62Generator{},
	)
	return NewHandler(service, "http://localhost:8080")
}

func TestShortenHandler(t *testing.T) {
	h := newTestHandler()

	body := strings.NewReader(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["short_url"] == "" {
		t.Errorf("expected short_url in response, got %v", resp)
	}

	if resp["code"] == "" {
		t.Errorf("expected code in response, got %v", resp)
	}
}

func TestShortenHandlerIdempotency(t *testing.T) {
	h := newTestHandler()

	body := strings.NewReader(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	sBody := strings.NewReader(`{"url":"https://example.com"}`)
	sReq := httptest.NewRequest(http.MethodPost, "/shorten", sBody)
	sReq.Header.Set("Content-Type", "application/json")
	sRec := httptest.NewRecorder()

	h.Shorten(sRec, sReq)

	var respOne map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&respOne); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	var respSec map[string]string
	if err := json.NewDecoder(sRec.Body).Decode(&respSec); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if respOne["short_url"] != respSec["short_url"] {
		t.Errorf("expected equality between short_url_1 and 2 in response, got %v %v", respOne, respSec)
	}

}

func TestShortenHandlerInvalidMethod(t *testing.T) {
	service := link.NewShortenerService(
		link.URLValidator{},
		store.NewMemoryStore(),
		shortcode.Base62Generator{},
	)
	h := NewHandler(service, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/shorten", nil)
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestShortenHandlerInvalidBody(t *testing.T) {
	h := newTestHandler()

	tests := map[string]string{
		"not json":    `not json`,
		"broken json": `{"url":`,
		"wrong type":  `{"url": 123}`,
		"empty body":  ``,
		"empty":       `{}`,
		"missing":     `{"url":""}`,
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(body))
			rec := httptest.NewRecorder()

			h.Shorten(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestShortenHandlerLargeBody(t *testing.T) {
	h := newTestHandler()
	const maxBodyBytes = 1 << 20 // 5 Mb

	big := `{"url":"https://example.com/` + strings.Repeat("a", maxBodyBytes) + `"}`

	req := httptest.NewRequest(http.MethodPost, "/shorten", strings.NewReader(big))
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)

	}
}

func TestShortenHandlerInvalidUrl(t *testing.T) {
	h := newTestHandler()

	body := strings.NewReader(`{"url":"httpss://example.com"}`)

	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)

	}
}

type fakeService struct {
	link         *domain.ShortLink
	err          error
	shortenCalls int
	gotURL       string
	gotCode      string
}

func (f *fakeService) Shorten(rawURL string) (*domain.ShortLink, error) {
	f.shortenCalls++
	f.gotURL = rawURL
	return f.link, f.err
}

func (f *fakeService) GetByCode(code string) (*domain.ShortLink, error) {
	f.gotCode = code
	return f.link, f.err
}

func TestShortenHandlerInternalError(t *testing.T) {
	fs := &fakeService{err: errors.New("internal error")}

	h := NewHandler(fs, "http://localhost:8080")

	body := strings.NewReader(`{"url":"https://example.com"}`)

	req := httptest.NewRequest(http.MethodPost, "/shorten", body)
	rec := httptest.NewRecorder()

	h.Shorten(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)

	}
}

type failingWriter struct { // helped by ai
	header http.Header
	code   int
}

func (f *failingWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *failingWriter) WriteHeader(code int) { f.code = code }

func (f *failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestShortenHandlerEncodeError(t *testing.T) {
	fs := &fakeService{link: domain.NewShortLink("abc123", "https://example.com/")}
	h := NewHandler(fs, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodPost, "/shorten",
		strings.NewReader(`{"url":"https://example.com"}`))
	w := &failingWriter{}

	h.Shorten(w, req)

	if w.code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.code, http.StatusCreated)
	}
}

func TestRouterShorten(t *testing.T) {
	h := newTestHandler()
	router := NewRouter(h)

	body := strings.NewReader(`{"url":"https://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/shorten", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(resp) == 0 {
		t.Errorf("expected non-empty response, got %v", resp)
	}
}

func TestRouterUnknownPath(t *testing.T) {
	router := NewRouter(newTestHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/nope", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// Redirect
func TestRedirectHandler(t *testing.T) {
	fs := &fakeService{
		link: domain.NewShortLink("abc123", "https://example.com/"),
	}
	h := NewHandler(fs, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	req.SetPathValue("code", "abc123")
	rec := httptest.NewRecorder()

	h.Redirect(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com/" {
		t.Errorf("Location = %q", loc)
	}
}

func TestRedirectHandlerErrors(t *testing.T) {
	tests := map[string]struct {
		method     string
		code       string
		err        error
		wantStatus int
	}{
		"wrong method":   {http.MethodPost, "abc123", nil, http.StatusMethodNotAllowed},
		"code too short": {http.MethodGet, "abc", nil, http.StatusNotFound},
		"code too long":  {http.MethodGet, "abcdefghi", nil, http.StatusNotFound},
		"not found":      {http.MethodGet, "abc123", link.ErrNotFound, http.StatusNotFound},
		"internal error": {http.MethodGet, "abc123", errors.New("boom"), http.StatusInternalServerError},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := &fakeService{err: tc.err}
			h := NewHandler(fs, "http://localhost:8080")

			req := httptest.NewRequest(tc.method, "/"+tc.code, nil)
			req.SetPathValue("code", tc.code)
			rec := httptest.NewRecorder()

			h.Redirect(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestRouterShortenThenRedirect(t *testing.T) {
	router := NewRouter(newTestHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/shorten",
		strings.NewReader(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var resp struct {
		ShortURL string `json:"short_url"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	code := path.Base(resp.ShortURL)

	req2 := httptest.NewRequest(http.MethodGet, "/"+code, nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusFound)
	}
	if loc := rec2.Header().Get("Location"); loc != "https://example.com/" {
		t.Errorf("Location = %q", loc)
	}
}

// GetMetaData
func TestGetMetadataSuccess(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sl := domain.NewShortLink("abc123", "https://example.com/")
	sl.CreatedAt = createdAt

	fs := &fakeService{link: sl}
	h := NewHandler(fs, "http://localhost:8080")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/abc123", nil)
	req.SetPathValue("code", "abc123")
	rec := httptest.NewRecorder()

	h.GetMetadata(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if fs.gotCode != "abc123" {
		t.Errorf("service got code %q, want abc123", fs.gotCode)
	}

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	if raw["url"] != "https://example.com/" {
		t.Errorf("url = %v, want https://example.com/", raw["url"])
	}
	got, ok := raw["created_at"].(string)
	if !ok {
		t.Fatalf("created_at missing or not a string: %v", raw["created_at"])
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("created_at %q is not RFC3339: %v", got, err)
	}
	if !parsed.Equal(createdAt) {
		t.Errorf("created_at = %v, want %v", parsed, createdAt)
	}
}

func TestGetMetadataErrors(t *testing.T) {
	tests := map[string]struct {
		method     string
		code       string
		err        error
		wantStatus int
	}{
		"wrong method POST":   {http.MethodPost, "abc123", nil, http.StatusMethodNotAllowed},
		"wrong method DELETE": {http.MethodDelete, "abc123", nil, http.StatusMethodNotAllowed},
		"code too short":      {http.MethodGet, "abc", nil, http.StatusNotFound},
		"code empty":          {http.MethodGet, "", nil, http.StatusNotFound},
		"code too long":       {http.MethodGet, "abcdefghi", nil, http.StatusNotFound},
		"not found":           {http.MethodGet, "abc123", link.ErrNotFound, http.StatusNotFound},
		"wrapped not found":   {http.MethodGet, "abc123", fmt.Errorf("lookup: %w", link.ErrNotFound), http.StatusNotFound},
		"internal error":      {http.MethodGet, "abc123", errors.New("boom"), http.StatusInternalServerError},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := &fakeService{err: tc.err}
			h := NewHandler(fs, "http://localhost:8080")

			req := httptest.NewRequest(tc.method, "/api/v1/links/"+tc.code, nil)
			req.SetPathValue("code", tc.code)
			rec := httptest.NewRecorder()

			h.GetMetadata(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestRouterGetMetadata(t *testing.T) {
	sl := domain.NewShortLink("abc123", "https://example.com/")
	h := NewHandler(&fakeService{link: sl}, "http://localhost:8080")
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/abc123", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
