package link

import (
	// ""
	"time"
)

type ShortLink struct {
	Code      string
	LongURL   string
	CreatedAt time.Time
}

func NewShortLink(code, longURL string) *ShortLink {
	return &ShortLink{
		Code:      code,
		LongURL:   longURL,
		CreatedAt: time.Now().UTC(),
	}
}
