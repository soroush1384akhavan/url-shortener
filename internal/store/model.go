package store

import (
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/soroush1384akhavan/url-shortener/internal/domain"
)

type ShortLinkModel struct {
	Code          string    `gorm:"type:varchar(8);primaryKey;not null"`
	NormalizedURL string    `gorm:"type:text;not null;uniqueIndex:idx_url_identity,priority:2"`
	URLHash       string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_url_identity,priority:1"`
	CreatedAt     time.Time `gorm:"not null"`
	UsedCount     int64     `gorm:"not null;default:0"`
}

func (ShortLinkModel) TableName() string {
	return "short_links"
}

func toModel(sl *domain.ShortLink) ShortLinkModel {
	return ShortLinkModel{
		Code:          sl.Code,
		NormalizedURL: sl.LongURL,
		URLHash:       hashURL(sl.LongURL),
		CreatedAt:     sl.CreatedAt,
		UsedCount:     int64(sl.UsedCount),
	}
}

func (m ShortLinkModel) toDomain() *domain.ShortLink {
	return &domain.ShortLink{
		Code:      m.Code,
		LongURL:   m.NormalizedURL,
		CreatedAt: m.CreatedAt,
		UsedCount: uint64(m.UsedCount),
	}
}

func hashURL(u string) string {
	sum := sha256.Sum256([]byte(u))
	return base64.RawURLEncoding.EncodeToString(sum[:])[:10]
}
