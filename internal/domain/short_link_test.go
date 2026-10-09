package domain

import (
	"testing"
	"time"
)

func TestNewShortLink(t *testing.T) {
	before := time.Now().UTC()

	sl := NewShortLink("abc123", "https://example.com")

	after := time.Now().UTC()

	if sl == nil {
		t.Fatal("expected ShortLink, got nil")
	}

	if sl.Code != "abc123" {
		t.Errorf("expected code abc123, got %s", sl.Code)
	}

	if sl.LongURL != "https://example.com" {
		t.Errorf("expected URL https://example.com, got %s", sl.LongURL)
	}

	if sl.UsedCount != 0 {
		t.Errorf("expected UsedCount 0, got %d", sl.UsedCount)
	}

	if sl.CreatedAt.Before(before) || sl.CreatedAt.After(after) {
		t.Errorf(
			"expected CreatedAt between %v and %v, got %v",
			before,
			after,
			sl.CreatedAt,
		)
	}
}