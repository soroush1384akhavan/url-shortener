package store

import (
	// "errors"
	"context"
	"sync"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/domain"
)

type MemoryStore struct {
	codeLink map[string]*domain.ShortLink
	urlLink  map[string]*domain.ShortLink
	mu       sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		codeLink: make(map[string]*domain.ShortLink),
		urlLink:  make(map[string]*domain.ShortLink),
	}
}
func (s *MemoryStore) FindByURL(_ context.Context, normalizedURL string) (*domain.ShortLink, error) {
	s.mu.RLock()
	lnk, ok := s.urlLink[normalizedURL]
	s.mu.RUnlock()

	if !ok {
		return nil, apperr.ErrNotFound
	}
	return lnk, nil
}

func (s *MemoryStore) FindByCode(_ context.Context, code string) (*domain.ShortLink, error) {
	s.mu.RLock()
	lnk, ok := s.codeLink[code]
	s.mu.RUnlock()

	if !ok {
		return nil, apperr.ErrNotFound
	}

	return lnk, nil
}

func (s *MemoryStore) SaveIfNotExist(_ context.Context, shortLink *domain.ShortLink) (*domain.ShortLink, error) { // between check and save we dont have any lock so its have to be atomic
	s.mu.Lock()
	defer s.mu.Unlock()

	if lnk, ok := s.urlLink[shortLink.LongURL]; ok {
		return lnk, nil
	}

	if _, ok := s.codeLink[shortLink.Code]; ok {
		return nil, apperr.ErrCodeCollision

	}

	s.codeLink[shortLink.Code] = shortLink
	s.urlLink[shortLink.LongURL] = shortLink
	return shortLink, nil
}

func (s *MemoryStore) IncrementUsedCount(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	lnk, ok := s.codeLink[code]
	if !ok {
		return apperr.ErrNotFound
	}

	lnk.UsedCount++

	return nil
}
