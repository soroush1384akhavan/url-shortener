package link

import "errors"

var ErrInvalidURL = errors.New("invalid URL")

type Validator interface {
	Validate(longURL string) error
}

type URLValidator struct{}

func (v URLValidator) Validate(longURL string) error {
	if longURL == "" {
		return ErrInvalidURL
	}

	// TODO: validate scheme: http / https

	return nil
}