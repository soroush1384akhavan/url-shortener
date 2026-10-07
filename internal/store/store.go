package store

import (
	// "errors"
	"sync"

	"github.com/soroush1384akhavan/url-shortener/internal/link"
)


type MemoryStore struct {
	codeLink map[string]*link.ShortLink
	urlLink  map[string]*link.ShortLink
	mu       sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		codeLink: make(map[string]*link.ShortLink),
		urlLink:  make(map[string]*link.ShortLink),
	}
}

func (s *MemoryStore) FindByURL(normalizedURL string) (*link.ShortLink, bool) {
	s.mu.RLock()
	lnk, ok := s.urlLink[normalizedURL]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	return lnk, ok
}

func (s *MemoryStore) FindByCode(code string) (*link.ShortLink, bool) {
	s.mu.RLock()
	lnk, ok := s.codeLink[code]
	s.mu.RUnlock()

	if !ok {
		return nil, false
	}

	return lnk, ok
}

func (s *MemoryStore) SaveIfNotExist(shortLink *link.ShortLink) (*link.ShortLink, error) { // between check and save we dont have any lock so its have to be atomic
	s.mu.Lock()
	defer s.mu.Unlock()

	if lnk, ok := s.urlLink[shortLink.LongURL]; ok {
		return lnk, nil
	}

	if _, ok := s.codeLink[shortLink.Code]; ok {
		return nil, link.ErrCodeCollision

	}

	s.codeLink[shortLink.Code] = shortLink
	s.urlLink[shortLink.LongURL] = shortLink
	return shortLink, nil
}

// im not sure im going to use this or not (Probably not)
func (s *MemoryStore) Save(shortLink *link.ShortLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.codeLink[shortLink.Code] = shortLink
	s.urlLink[shortLink.LongURL] = shortLink

	return nil
}
