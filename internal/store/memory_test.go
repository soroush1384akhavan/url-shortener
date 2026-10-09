package store

import (
	"context"
	"errors"
	"testing"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/domain"
)

func TestMemoryStoreSaveAndFind(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	sl := domain.NewShortLink("abc123", "https://example.com/")

	_, err := st.SaveIfNotExist(ctx, sl)
	if err != nil {
		t.Fatalf("unexpected error saving link: %v", err)
	}

	byCode, err := st.FindByCode(ctx, sl.Code)
	if err != nil {
		t.Fatalf("unexpected error finding by code: %v", err)
	}

	if byCode.Code != sl.Code || byCode.LongURL != sl.LongURL {
		t.Errorf("FindByCode: expected %+v, got %+v", sl, byCode)
	}

	byURL, err := st.FindByURL(ctx, sl.LongURL)
	if err != nil {
		t.Fatalf("unexpected error finding by URL: %v", err)
	}

	if byURL.Code != sl.Code || byURL.LongURL != sl.LongURL {
		t.Errorf("FindByURL: expected %+v, got %+v", sl, byURL)
	}
}

func TestMemoryStoreIdempotency(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	first := domain.NewShortLink("abc123", "https://example.com/")
	second := domain.NewShortLink("xyz789", "https://example.com/")

	_, err := st.SaveIfNotExist(ctx, first)
	if err != nil {
		t.Fatalf("unexpected error saving first link: %v", err)
	}

	result, err := st.SaveIfNotExist(ctx, second)
	if err != nil {
		t.Fatalf("unexpected error saving second link: %v", err)
	}

	if result.Code != "abc123" {
		t.Errorf("expected code abc123, got %s", result.Code)
	}
}

func TestMemoryStoreCodeCollision(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	first := domain.NewShortLink("abc123", "https://example.com/a")
	second := domain.NewShortLink("abc123", "https://example.com/b")

	_, err := st.SaveIfNotExist(ctx, first)
	if err != nil {
		t.Fatalf("unexpected error saving first link: %v", err)
	}

	_, err = st.SaveIfNotExist(ctx, second)

	if !errors.Is(err, apperr.ErrCodeCollision) {
		t.Errorf("expected ErrCodeCollision, got %v", err)
	}
}

func TestMemoryStoreNotFound(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	_, err := st.FindByCode(ctx, "unknown")
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("expected ErrNotFound for FindByCode, got %v", err)
	}

	_, err = st.FindByURL(ctx, "https://unknown.com/")
	if !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("expected ErrNotFound for FindByURL, got %v", err)
	}
}

func TestMemoryStoreIncrementUsedCount(t *testing.T) {
	ctx := context.Background()
	st := NewMemoryStore()

	sl := domain.NewShortLink("abc123", "https://example.com/")
	_, err := st.SaveIfNotExist(ctx, sl)
	if err != nil {
		t.Fatalf("unexpected error saving link: %v", err)
	}

	err = st.IncrementUsedCount(ctx, sl.Code, 1)
	if err != nil {
		t.Fatalf("unexpected error incrementing used count: %v", err)
	}

	result, err := st.FindByCode(ctx, sl.Code)
	if err != nil {
		t.Fatalf("unexpected error finding link: %v", err)
	}

	if result.UsedCount != 1 {
		t.Errorf("expected UsedCount 1, got %d", result.UsedCount)
	}
}
