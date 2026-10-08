package link

import (
	"errors"

	// "github.com/soroush1384akhavan/url-shortener/internal/store"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/domain"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
)

const maxAttempts = 20

type ShortenerService struct {
	Validator Validator
	Store     Store
	Generator shortcode.Generator
}

func NewShortenerService(vld Validator, st Store, gn shortcode.Generator) *ShortenerService {
	return &ShortenerService{
		Validator: vld,
		Store:     st,
		Generator: gn,
	}
}

func (s *ShortenerService) Shorten(rawURL string) (*domain.ShortLink, error) {
	// validate
	if err := s.Validator.Validate(rawURL); err != nil {
		return nil, err
	}

	// normalize
	normalizedURL, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	// check if URL already exists
	if lnk, ok := s.Store.FindByURL(normalizedURL); ok {
		return lnk, nil
	}

	// generate code and collision handling
	for i := 0; i < maxAttempts; i++ {
		code, err := s.Generator.GenerateCode()
		if err != nil {
			return nil, err
		}

		lnk, err := s.Store.SaveIfNotExist(domain.NewShortLink(code, normalizedURL))
		if errors.Is(err, apperr.ErrCodeCollision) {	
			continue
		}

		return lnk, err
	}

	return nil, ErrCodeGenerationExhausted
}

func (s *ShortenerService) GetByCode(code string) (*domain.ShortLink, error) {

	ShortLink, exists := s.Store.FindByCode(code)

	if exists {
		return ShortLink, nil
	}

	return nil, ErrNotFound

}
