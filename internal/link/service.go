package link

import (
	"context"
	"errors"

	// "github.com/soroush1384akhavan/url-shortener/internal/store"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/domain"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
)

const maxAttempts = 20

type Shortener interface {
	Shorten(context.Context, string) (*domain.ShortLink, error)
	GetByCode(context.Context, string) (*domain.ShortLink, error)
	IncrementUsedCount(context.Context, string, uint64) error
}

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

func (s *ShortenerService) Shorten(ctx context.Context, rawURL string) (*domain.ShortLink, error) {
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
	lnk, err := s.Store.FindByURL(ctx, normalizedURL)
	switch {
	case err == nil:
		return lnk, nil
	case errors.Is(err, apperr.ErrNotFound):
		// its new do nothing
	default:
		return nil, err
	}

	// generate code and collision handling
	for i := 0; i < maxAttempts; i++ {
		code, err := s.Generator.GenerateCode()
		if err != nil {
			return nil, err
		}

		lnk, err := s.Store.SaveIfNotExist(ctx, domain.NewShortLink(code, normalizedURL))
		if errors.Is(err, apperr.ErrCodeCollision) {
			continue
		}

		return lnk, err
	}

	return nil, ErrCodeGenerationExhausted
}

func (s *ShortenerService) GetByCode(ctx context.Context, code string) (*domain.ShortLink, error) {

	ShortLink, err := s.Store.FindByCode(ctx, code)

	switch {
	case err == nil:
		return ShortLink, nil
	case errors.Is(err, apperr.ErrNotFound):
		return nil, apperr.ErrNotFound

	default:
		return nil, err
	}

}

func (s *ShortenerService) IncrementUsedCount(ctx context.Context, code string, amount uint64) error {
	return s.Store.IncrementUsedCount(ctx, code, amount)

}
