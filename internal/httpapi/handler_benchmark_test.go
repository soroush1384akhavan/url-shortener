package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/soroush1384akhavan/url-shortener/internal/domain"
)

func BenchmarkRedirect(b *testing.B) {
	fs := &fakeService{
		link: domain.NewShortLink("abc123", "https://example.com/"),
	}
	h := NewHandler(fs, "http://localhost:8080")

	b.ReportAllocs()
	

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
		req.SetPathValue("code", "abc123")
		rec := httptest.NewRecorder()

		h.Redirect(rec, req)

		if rec.Code != http.StatusFound {
			b.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
		}
	}
}
