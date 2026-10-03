package link

import (


	// "github.com/soroush1384akhavan/url-shortener/internal/store"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"

)

type ShortenerService struct {
	Validator Validator
	Store Store
}

func NewShortenerService(validator Validator, st Store) (*ShortenerService, error) {
	return &ShortenerService{
		Validator: validator,
		Store: st,
	}, nil
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
	code, ok := s.Store.FindByURL(normalizedURL)

	if ok{
		link := NewShortLink(code, normalizedURL)

		return link, nil
	}

	// generate code
	code, genErr := shortcode.CodeGenerator(normalizedURL)
	if genErr != nil{
		return nil, genErr
	}

	// 5. check collision

	// 6. create ShortLink

	// 7. save

	// 8. return
}
