package domain

import (
	"time"
)

type ShortLink struct {
	Code      string
	LongURL   string
	CreatedAt time.Time
	UsedCount uint64 //maybe it will burn database for a simple read ??!
}

func NewShortLink(code, longURL string) *ShortLink {
	return &ShortLink{
		Code:      code,
		LongURL:   longURL,
		CreatedAt: time.Now().UTC(),
	}
}
