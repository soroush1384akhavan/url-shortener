package link

import (

	// "github.com/soroush1384akhavan/url-shortener/internal/store"

	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
)

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

func (s *ShortenerService) Shorten(rawURL string) (*ShortLink, error) {
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
	lnk, ok := s.Store.FindByURL(normalizedURL)

	if ok {
		return lnk, nil
	}

	// generate code and collision handling
	// its not complete
	var code string

	for {
		generatedCode, err := s.Generator.GenerateCode()
		if err != nil {
			return nil, err
		}

		_, exists := s.Store.FindByCode(generatedCode)
		if !exists {
			code = generatedCode
			break
		}
	}

	return s.Store.SaveIfNotExist(NewShortLink(code, normalizedURL))
}
