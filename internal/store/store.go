package store

import (
	"github.com/soroush1384akhavan/url-shortener/internal/link"
)

type MemoryStore struct {
	codeurl map[string]string
	urlCode map[string]string
}

func (s *MemoryStore) FindByURL(normalizedURL string) (*link.ShortLink, bool) {
	code, ok := s.urlCode[normalizedURL]

	var lnk *link.ShortLink // if its not ok is nill ? yep
	if ok {
		lnk = link.NewShortLink(code, normalizedURL) // its not correct but just for now (because of data its make a new link! and thats bad!)
	}

	return lnk, ok
}

func (s *MemoryStore) FindByCode(code string) (*link.ShortLink, bool) {
	URL, ok := s.codeurl[code]

	var lnk *link.ShortLink
	if ok {
		lnk = link.NewShortLink(code, URL) // its not correct but just for now (because of data its make a new link! and thats bad!)
	}

	return lnk, ok
}
