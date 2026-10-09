package shortcode

import (
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errors.New("random reader failed")
}

func TestGenerateCodeRandomError(t *testing.T) {
	originalReader := rand.Reader
	rand.Reader = failingReader{}

	defer func() {
		rand.Reader = originalReader
	}()

	gen := Base62Generator{}

	code, err := gen.GenerateCode()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if code != "" {
		t.Errorf("expected empty code, got %s", code)
	}

	if err.Error() != "generate short code: random reader failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateCodeLength(t *testing.T) {
	gen := Base62Generator{}

	code, err := gen.GenerateCode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(code) != length {
		t.Errorf("expected code length %d, got %d", length, len(code))
	}
}

func TestGenerateCodeUsesBase62Charset(t *testing.T) {
	gen := Base62Generator{}

	code, err := gen.GenerateCode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, char := range code {
		if !strings.ContainsRune(base62, char) {
			t.Errorf("invalid character %q in code %s", char, code)
		}
	}
}

func TestGenerateCodeNotEmpty(t *testing.T) {
	gen := Base62Generator{}

	for i := 0; i < 100; i++ {
		code, err := gen.GenerateCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if code == "" {
			t.Errorf("generated empty code at iteration %d", i)
		}
	}
}

func TestGenerateCodeProducesDifferentValues(t *testing.T) {
	gen := Base62Generator{}

	codes := make(map[string]bool)

	for i := 0; i < 100; i++ {
		code, err := gen.GenerateCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		codes[code] = true
	}

	if len(codes) <= 1 {
		t.Errorf("expected different codes, got %d unique codes", len(codes))
	}
}
