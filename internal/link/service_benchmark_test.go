package link

import (
	"context"
	"fmt"
	"testing"

	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
	"github.com/soroush1384akhavan/url-shortener/internal/store"
)

func BenchmarkShortenNewURL(b *testing.B) {
	s := NewShortenerService(URLValidator{}, store.NewMemoryStore(), shortcode.Base62Generator{})

	urls := make([]string, b.N)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/page/%d", i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := s.Shorten(context.Background(), urls[i]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShortenExistingURL(b *testing.B) {
	s := NewShortenerService(URLValidator{}, store.NewMemoryStore(), shortcode.Base62Generator{})
	const rawURL = "https://example.com/page"

	if _, err := s.Shorten(context.Background(), rawURL); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := s.Shorten(context.Background(), rawURL); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShortenParallel(b *testing.B) {
	s := NewShortenerService(URLValidator{}, store.NewMemoryStore(), shortcode.Base62Generator{})

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) { // i didnt know about this syntax so i got helped by ai
		i := 0
		for pb.Next() {
			i++
			u := fmt.Sprintf("https://example.com/p/%p/%d", pb, i)
			if _, err := s.Shorten(context.Background(), u); err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkGetByCode(b *testing.B) {
	s := NewShortenerService(URLValidator{}, store.NewMemoryStore(), shortcode.Base62Generator{})

	lnk, err := s.Shorten(context.Background(), "https://example.com/page")
	if err != nil {
		b.Fatal(err)
	}
	code := lnk.Code

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := s.GetByCode(context.Background(), code); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGetByCodeParallel(b *testing.B) {
	s := NewShortenerService(URLValidator{}, store.NewMemoryStore(), shortcode.Base62Generator{})

	lnk, err := s.Shorten(context.Background(), "https://example.com/page")
	if err != nil {
		b.Fatal(err)
	}
	code := lnk.Code

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := s.GetByCode(context.Background(), code); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
