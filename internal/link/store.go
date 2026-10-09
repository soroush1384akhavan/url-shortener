package link

import (
	"context"

	"github.com/soroush1384akhavan/url-shortener/internal/domain"
)

// for fixig import cycle problem
type Store interface {
	FindByURL(ctx context.Context, normalizedURL string) (*domain.ShortLink, error)
	FindByCode(ctx context.Context, code string) (*domain.ShortLink, error)
	SaveIfNotExist(ctx context.Context, sl *domain.ShortLink) (*domain.ShortLink, error)
	IncrementUsedCount(ctx context.Context, code string) error
}
