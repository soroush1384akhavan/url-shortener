package link

import (
	"errors"
	"net/url"
	"strings"
)

var allowedSchemes = map[string]bool{
	"http":  true,
	"https": true,
}

const maxURLLength = 200

type Validator interface {
	Validate(string) error
}

type URLValidator struct{}

func (v URLValidator) Validate(longURL string) error {
	rawURL := strings.TrimSpace(longURL)

	if rawURL == "" {
		return errors.New("url is empty")
	}
	if len(rawURL) > maxURLLength {
		return errors.New("url is too long")
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	if !allowedSchemes[u.Scheme] {
		return errors.New("scheme must be http or https")
	}
	if u.Hostname() == "" {
		return errors.New("url has no host")
	}
	if u.User != nil {
		return errors.New("userinfo is not allowed")
	}
	return nil
}
