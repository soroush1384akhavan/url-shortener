package link

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/soroush1384akhavan/url-shortener/internal/domain"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
	"github.com/soroush1384akhavan/url-shortener/internal/store"
)

func TestShortenValidURL(t *testing.T) {
	vldt := URLValidator{}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	s := NewShortenerService(vldt, st, gn)

	validUrl := "https://translate.google.com/?sl=en&tl=fa&op=translate"

	lnk, err := s.Shorten(validUrl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	norm, err := NormalizeURL(validUrl)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	if lnk.LongURL != norm {
		t.Errorf("expected URL %q, got %q", norm, lnk.LongURL)
	}

}

func TestShortenSameURLReturnsSameLink(t *testing.T) {
	vldt := URLValidator{}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	s := NewShortenerService(vldt, st, gn)

	firstUrl := "https://translate.google.com/?sl=en&tl=fa&op=translate"

	firstLink, err := s.Shorten(firstUrl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	secondUrl := "https://translate.google.com/?sl=en&tl=fa&op=translate"

	secondLink, err := s.Shorten(secondUrl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if *firstLink != *secondLink {
		t.Errorf(
			"expected same link, got first=%+v second=%+v",
			firstLink,
			secondLink,
		)
	}
}

func TestURLValidator_InvalidURLs(t *testing.T) {
	tooLong := "https://example.com/" + strings.Repeat("a", maxURLLength)

	tests := map[string]struct {
		input   string
		wantMsg string
	}{
		// empty
		"empty string":       {input: "", wantMsg: "url is empty"},
		"only spaces":        {input: "     ", wantMsg: "url is empty"},
		"only tabs/newlines": {input: "\t\n  \r\n", wantMsg: "url is empty"},

		// too long
		"exceeds max length": {input: tooLong, wantMsg: "url is too long"},

		// parse error // built in parse error massages
		"invalid percent escape":  {input: "http://example.com/%zz", wantMsg: "invalid URL escape"},
		"control character":       {input: "http://exa\x00mple.com", wantMsg: "invalid control character"},
		"missing protocol scheme": {input: "://example.com", wantMsg: "missing protocol scheme"},
		"space in host":           {input: "http://exa mple.com", wantMsg: "invalid character"},

		// scheme not allowed
		"no scheme":           {input: "example.com", wantMsg: "scheme must be http or https"},
		"no scheme with path": {input: "example.com/path", wantMsg: "scheme must be http or https"},
		"ftp scheme":          {input: "ftp://example.com", wantMsg: "scheme must be http or https"},
		"javascript scheme":   {input: "javascript:alert(1)", wantMsg: "scheme must be http or https"},
		"data scheme":         {input: "data:text/html,<h1>hi</h1>", wantMsg: "scheme must be http or https"},
		"file scheme":         {input: "file:///etc/passwd", wantMsg: "scheme must be http or https"},
		"mailto scheme":       {input: "mailto:a@b.com", wantMsg: "scheme must be http or https"},
		"httpx scheme":        {input: "httpx://example.com", wantMsg: "scheme must be http or https"},

		// no host
		"http without host":      {input: "http://", wantMsg: "url has no host"},
		"https without host":     {input: "https://", wantMsg: "url has no host"},
		"host is only a path":    {input: "http:///path/only", wantMsg: "url has no host"},
		"host is only port":      {input: "http://:8080", wantMsg: "url has no host"},
		"scheme only no slashes": {input: "http:example", wantMsg: "url has no host"},
		"userinfo but no host":   {input: "http://user:pass@", wantMsg: "url has no host"},
	}

	v := URLValidator{}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := v.Validate(tc.input)
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tc.input)
			}
			if !errors.Is(err, ErrInvalidURL) {
				t.Errorf("error %v does not wrap ErrInvalidURL", err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestGetByCodeFound(t *testing.T) {
	vldt := URLValidator{}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	s := NewShortenerService(vldt, st, gn)

	rawUrl := "https://translate.google.com/?sl=en&tl=fa&op=translate"

	existLink, err := s.Shorten(rawUrl)
	if err != nil {
		t.Fatalf("shorten failed: %v", err)
	}

	gotLnk, err := s.GetByCode(existLink.Code)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if gotLnk.LongURL != rawUrl {
		t.Errorf("expected URL %q, got %q", rawUrl, gotLnk.LongURL)
	}
}

func TestGetByCodeNotFound(t *testing.T) {
	vldt := URLValidator{}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	s := NewShortenerService(vldt, st, gn)

	notExistCode := "ertyuip"

	lnk, err := s.GetByCode(notExistCode)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if lnk != nil {
		t.Errorf("expected nil link, got %+v", lnk)
	}

}

func TestNormalizeURL(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"lowercase scheme and host": {"HTTPS://EXAMPLE.COM/a", "https://example.com/a"},
		"drop default http port":    {"http://example.com:80/a", "http://example.com/a"},
		"drop default https port":   {"https://example.com:443/a", "https://example.com/a"},
		"keep non-default port":     {"http://example.com:8080/a", "http://example.com:8080/a"},
		"ipv6 default port":         {"http://[::1]:80/a", "http://[::1]/a"},
		"no path gets root":         {"http://example.com", "http://example.com/"},
		"dot segments":              {"http://example.com/a/./b/../c", "http://example.com/a/c"},
		"drop fragment":             {"http://example.com/a#frag", "http://example.com/a"},
		"drop empty query":          {"http://example.com/a?", "http://example.com/a"},
		"trim spaces":               {"  http://example.com/a  ", "http://example.com/a"},
		"trailing slash":            {"http://example.com/./a/", "http://example.com/a/"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := NormalizeURL(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	t.Run("parse error", func(t *testing.T) {
		_, err := NormalizeURL("http://example.com/%zz")
		if !errors.Is(err, ErrInvalidURL) {
			t.Fatalf("err = %v, want ErrInvalidURL", err)
		}
	})
}

func TestShortenNormalizeError(t *testing.T) {
	const badURL = "http://example.com/%zz"

	st := store.NewMemoryStore()
	fv := &fakeValidator{err: nil}
	gn := shortcode.Base62Generator{}

	s := NewShortenerService(fv, st, gn)

	got, err := s.Shorten(badURL)

	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if !fv.called {
		t.Error("validator was not called")
	}
	if _, ok := st.FindByURL(badURL); ok {
		t.Error("link must not be saved when normalization fails")
	}
}

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

func TestShortenFakeValidator(t *testing.T) {

	fv := &fakeValidator{err: ErrInvalidURL}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	s := NewShortenerService(fv, st, gn)

	_, err := s.Shorten("anything")

	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("err = %v, want ErrInvalidURL", err)
	}
	if !fv.called {
		t.Error("validator was not called")
	}
	if _, ok := st.FindByURL("anything"); ok {
		t.Error("link must not be saved when validation fails")
	}
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

func TestShortenFakeGenerator(t *testing.T) {
	const rawURL = "https://translate.google.com/?sl=en&tl=fa&op=translate"

	t.Run("generator error is returned", func(t *testing.T) {
		genErr := errors.New("rand failed")
		st := store.NewMemoryStore()
		gn := &fakeGenerator{err: genErr}

		s := NewShortenerService(URLValidator{}, st, gn)

		got, err := s.Shorten(rawURL)

		if !errors.Is(err, genErr) {
			t.Fatalf("err = %v, want %v", err, genErr)
		}
		if got != nil {
			t.Errorf("got = %+v, want nil", got)
		}
	})

	t.Run("retries after collision and succeeds", func(t *testing.T) {
		st := store.NewMemoryStore()

		st.Save(domain.NewShortLink("abc", "https://ex.com/"))

		gn := &fakeGenerator{codes: []string{"abc", "xyz"}}
		s := NewShortenerService(URLValidator{}, st, gn)

		got, err := s.Shorten(rawURL)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Code != "xyz" {
			t.Errorf("Code = %q, want xyz", got.Code)
		}
	})

	t.Run("exhausted attempts", func(t *testing.T) {
		st := store.NewMemoryStore()
		st.Save(domain.NewShortLink("abc", "https://ex.com/"))

		gn := &fakeGenerator{codes: []string{"abc"}}
		s := NewShortenerService(URLValidator{}, st, gn)

		got, err := s.Shorten(rawURL)

		if !errors.Is(err, ErrCodeGenerationExhausted) {
			t.Fatalf("err = %v, want ErrCodeGenerationExhausted", err)
		}
		if got != nil {
			t.Errorf("got = %+v, want nil", got)
		}
		if gn.calls != maxAttempts {
			t.Errorf("generator calls = %d, want %d", gn.calls, maxAttempts)
		}
	})
}

func TestConcurrentShortenSameURL(t *testing.T) {
	const workers = 50
	const rawURL = "https://example.com/page"

	st := store.NewMemoryStore()
	s := NewShortenerService(URLValidator{}, st, shortcode.Base62Generator{})

	codes := make([]string, workers)
	errs := make([]error, workers)

	var ready, wg sync.WaitGroup
	start := make(chan int)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		ready.Add(1) //for starting all of them at the same time
		go func(i int) {
			defer wg.Done()

			ready.Done()
			<-start
			lnk, err := s.Shorten(rawURL)
			errs[i] = err
			if lnk != nil {
				codes[i] = lnk.Code
			}
		}(i)
	}

	ready.Wait()
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: unexpected error: %v", i, err)
		}
	}

	first := codes[0]
	if first == "" {
		t.Fatal("first code is empty")
	}
	for i, c := range codes {
		if c != first {
			t.Errorf("worker %d got code %q, want %q", i, c, first)
		}
	}

	if _, ok := st.FindByCode(first); !ok {
		t.Error("link not found in store")
	}
}

func TestConcurrentShortenDifferentURLs(t *testing.T) {
	const workers = 100

	st := store.NewMemoryStore()
	s := NewShortenerService(URLValidator{}, st, shortcode.Base62Generator{})

	urls := make([]string, workers)
	codes := make([]string, workers)
	errs := make([]error, workers)

	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/page/%d", i)
	}

	var ready, wg sync.WaitGroup
	start := make(chan int)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		ready.Add(1)
		go func(i int) {
			defer wg.Done()

			ready.Done()
			<-start
			lnk, err := s.Shorten(urls[i])
			errs[i] = err
			if lnk != nil {
				codes[i] = lnk.Code
			}
		}(i)
	}

	ready.Wait()
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: unexpected error: %v", i, err)
		}
	}

	seen := make(map[string]int, workers)
	for i, c := range codes {
		if c == "" {
			t.Fatalf("worker %d: empty code", i)
		}
		if j, dup := seen[c]; dup {
			t.Errorf("code %q given to both worker %d and %d", c, j, i)
		}
		seen[c] = i
	}

	for i, c := range codes {
		lnk, ok := st.FindByCode(c)
		if !ok {
			t.Errorf("worker %d: code %q not found in store", i, c)
			continue
		}

		normalized, err := NormalizeURL(urls[i])
		if err != nil {
			t.Fatalf("normalize failed for worker %d: %v", i, err)
		}
		if lnk.LongURL != normalized {
			t.Errorf("worker %d: code %q -> %q, want %q", i, c, lnk.LongURL, urls[i])
		}
	}
}
