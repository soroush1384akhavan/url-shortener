package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/domain"
	"github.com/soroush1384akhavan/url-shortener/internal/link"
)

type fakeValidator struct {
	err    error
	called bool
	got    string
}

func (f *fakeValidator) Validate(longURL string) error {
	f.called = true
	f.got = longURL
	return f.err
}

type fakeGenerator struct {
	codes []string
	err   error
	calls int
}

func (f *fakeGenerator) GenerateCode() (string, error) {
	if f.err != nil {
		return "", f.err
	}
	i := f.calls
	if i >= len(f.codes) {
		i = len(f.codes) - 1
	}
	f.calls++
	fmt.Println(f.calls)
	return f.codes[i], nil
}

func getTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")

	if dsn == "" {
		t.Skip("postgres integration test requires DATABASE_URL")
	}

	st, err := NewPostgresStore(dsn)
	if err != nil {
		t.Skipf("postgres integration test skipped: database unavailable: %v", err)
	}

	return st
}

func TestShortenPostgresStore(t *testing.T) {
	st := getTestStore(t)
	code := fmt.Sprintf("t%05d", time.Now().UnixNano()%100000)
	gn := &fakeGenerator{codes: []string{code}}

	s := link.NewShortenerService(&fakeValidator{}, st, gn)

	rawURL := fmt.Sprintf(
		"https://example.com/page-%d", time.Now().UnixNano(),
	)

	got, err := s.Shorten(context.Background(), rawURL)

	if err != nil {
		t.Fatalf("Shorten() unexpected error: %v", err)
	}

	if got == nil {
		t.Fatal("Shorten() got nil, want non-nil link")
	}

	if got.Code != code {
		t.Errorf("Code = %q, want %q", got.Code, code)
	}

	if got.LongURL != rawURL {
		t.Errorf("LongURL = %q, want %q", got.LongURL, rawURL)
	}

	if got.UsedCount != 0 {
		t.Errorf("UsedCount = %d, want 0", got.UsedCount)
	}

	saved, err := st.FindByCode(context.Background(), code)
	if err != nil {
		t.Fatalf("FindByCode() unexpected error: %v", err)
	}

	if saved.LongURL != rawURL {
		t.Errorf("saved LongURL = %q, want %q", saved.LongURL, rawURL)
	}
}

func TestPostgresStoreRestartTest(t *testing.T) {
	st := getTestStore(t)
	code := fmt.Sprintf("t%05d", time.Now().UnixNano()%100000)
	gn := &fakeGenerator{codes: []string{code}}

	s := link.NewShortenerService(&fakeValidator{}, st, gn)

	rawURL := fmt.Sprintf(
		"https://example.com/page-%d", time.Now().UnixNano(),
	)

	got, err := s.Shorten(context.Background(), rawURL)

	if err != nil {
		t.Fatalf("Shorten() unexpected error: %v", err)
	}

	if got == nil {
		t.Fatal("Shorten() got nil, want non-nil link")
	}

	if got.Code != code {
		t.Errorf("Code = %q, want %q", got.Code, code)
	}

	st2 := getTestStore(t)

	savedAfterRestart, err := st2.FindByCode(context.Background(), code)
	if err != nil {
		t.Fatalf("FindByCode() after restart unexpected error: %v", err)
	}

	if savedAfterRestart == nil {
		t.Fatal("got nil after restart")
	}

	if savedAfterRestart.Code != code {
		t.Errorf("Code after restart = %q, want %q", savedAfterRestart.Code, code)
	}

	if savedAfterRestart.LongURL != rawURL {
		t.Errorf("LongURL after restart = %q, want %q", savedAfterRestart.LongURL, rawURL)
	}

	if savedAfterRestart.UsedCount != 0 {
		t.Errorf("UsedCount after restart = %d, want 0", savedAfterRestart.UsedCount)
	}
}

func testCode(n int64) string {
	s := fmt.Sprintf("%x", n)
	return s[len(s)-8:]
}

func TestPostgresStoreIdempotency(t *testing.T) {
	st := getTestStore(t)

	ctx := context.Background()

	rawURL := fmt.Sprintf(
		"https://example.com/idempotent-%d",
		time.Now().UnixNano(),
	)

	now := time.Now().UnixNano()
	code1 := testCode(now)
	code2 := testCode(now + 1)

	first := domain.NewShortLink(code1, rawURL)

	_, err := st.SaveIfNotExist(ctx, first)
	if err != nil {
		t.Fatalf("unexpected error saving first link: %v", err)
	}

	second := domain.NewShortLink(code2, rawURL)

	result, err := st.SaveIfNotExist(ctx, second)
	if err != nil {
		t.Fatalf("unexpected error saving second link: %v", err)
	}

	if result.Code != code1 {
		t.Errorf("expected code %s, got %s", code1, result.Code)
	}

	if result.LongURL != rawURL {
		t.Errorf("expected URL %s, got %s", rawURL, result.LongURL)
	}
}

func TestPostgresStoreCodeCollision(t *testing.T) {
	st := getTestStore(t)

	ctx := context.Background()

	hex := fmt.Sprintf("%x", time.Now().UnixNano())
	code := hex[len(hex)-8:]

	url1 := fmt.Sprintf(
		"https://example.com/collision-a-%d",
		time.Now().UnixNano(),
	)

	url2 := fmt.Sprintf(
		"https://example.com/collision-b-%d",
		time.Now().UnixNano(),
	)

	first := domain.NewShortLink(code, url1)

	_, err := st.SaveIfNotExist(ctx, first)
	if err != nil {
		t.Fatalf("unexpected error saving first link: %v", err)
	}

	second := domain.NewShortLink(code, url2)

	_, err = st.SaveIfNotExist(ctx, second)

	if !errors.Is(err, apperr.ErrCodeCollision) {
		t.Errorf("expected ErrCodeCollision, got %v", err)
	}
}

func TestPostgresStoreIncrementUsedCount(t *testing.T) {
	st := getTestStore(t)

	ctx := context.Background()

	hex := fmt.Sprintf("%x", time.Now().UnixNano())
	code := hex[len(hex)-8:]

	rawURL := fmt.Sprintf(
		"https://example.com/count-%d",
		time.Now().UnixNano(),
	)

	sl := domain.NewShortLink(code, rawURL)

	_, err := st.SaveIfNotExist(ctx, sl)
	if err != nil {
		t.Fatalf("unexpected error saving link: %v", err)
	}
	err = st.IncrementUsedCount(ctx, code)
	if err != nil {
		t.Fatalf("unexpected error incrementing used count: %v", err)
	}

	result, err := st.FindByCode(ctx, code)
	if err != nil {
		t.Fatalf("unexpected error finding link: %v", err)
	}

	if result.UsedCount != 1 {
		t.Errorf("expected UsedCount 1, got %d", result.UsedCount)
	}
}

func TestPostgresStoreSaveIfNotExistDBError(t *testing.T) {
	st := getTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sl := domain.NewShortLink("abc123", "https://example.com/error")

	result, err := st.SaveIfNotExist(ctx, sl)

	if err == nil {
		t.Fatal("expected database error, got nil")
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
}

func TestPostgresStoreIncrementUsedCountDBError(t *testing.T) {
	st := getTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := st.IncrementUsedCount(ctx, "abc123")

	if err == nil {
		t.Fatal("expected database error, got nil")
	}
}

func TestPostgresStoreIncrementUsedCountNotFound(t *testing.T) {
	st := getTestStore(t)

	ctx := context.Background()

	code := fmt.Sprintf("missing-%d", time.Now().UnixNano())

	err := st.IncrementUsedCount(ctx, code)

	if !errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestNewPostgresStoreConnectionError(t *testing.T) {
	dsn := "host=127.0.0.1 user=invalid password=invalid dbname=invalid port=1 sslmode=disable connect_timeout=1"

	st, err := NewPostgresStore(dsn)

	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	if st != nil {
		t.Errorf("expected nil store, got %+v", st)
	}
}

func TestPostgresStoreFindByURLDBError(t *testing.T) {
	st := getTestStore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := st.FindByURL(ctx, "https://example.com/")

	if err == nil {
		t.Fatal("expected database error, got nil")
	}

	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}

	if errors.Is(err, apperr.ErrNotFound) {
		t.Errorf("expected database error, got ErrNotFound")
	}
}
