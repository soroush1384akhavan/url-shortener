package link

import (
	"fmt"
	"net/url"
	"strings"
)

var allowedSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

const maxURLLength = 400

type Validator interface {
	Validate(string) error
}

type URLValidator struct{}

func (v URLValidator) Validate(longURL string) error {
	rawURL := strings.TrimSpace(longURL)

	if rawURL == "" {
		return fmt.Errorf("%w: url is empty", ErrInvalidURL)
	}
	if len(rawURL) > maxURLLength {
		return fmt.Errorf("%w: url is too long", ErrInvalidURL)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if !allowedSchemes[scheme] {
		return fmt.Errorf("%w: scheme must be http or https", ErrInvalidURL)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("%w: url has no host", ErrInvalidURL)
	}

	return nil
}
