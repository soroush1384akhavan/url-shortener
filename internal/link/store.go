package link

import "github.com/soroush1384akhavan/url-shortener/internal/domain"

// for fixig import cycle problem
type Store interface {
	FindByURL(string) (*domain.ShortLink, bool)
	FindByCode(string) (*domain.ShortLink, bool)
	SaveIfNotExist(*domain.ShortLink) (*domain.ShortLink, error)
	// Save(*domain.ShortLink) (error)
}
	