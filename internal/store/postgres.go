package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/soroush1384akhavan/url-shortener/internal/apperr"
	"github.com/soroush1384akhavan/url-shortener/internal/domain"
)

type PostgresStore struct {
	db *gorm.DB
}

// AI help me with this I didnt know anything about gorm so i have t learn ablout it first :)
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&ShortLinkModel{}); err != nil {
		return nil, err
	}

	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) FindByURL(ctx context.Context, normalizedURL string) (*domain.ShortLink, error) {
	var m ShortLinkModel
	err := s.db.WithContext(ctx).
		Where("url_hash = ? AND normalized_url = ?", hashURL(normalizedURL), normalizedURL).
		Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (s *PostgresStore) FindByCode(ctx context.Context, code string) (*domain.ShortLink, error) {
	var m ShortLinkModel
	err := s.db.WithContext(ctx).
		Where("code = ?", code).
		Take(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m.toDomain(), nil
}

func (s *PostgresStore) SaveIfNotExist(ctx context.Context, sl *domain.ShortLink) (*domain.ShortLink, error) {
	m := toModel(sl)

	res := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "url_hash"}, {Name: "normalized_url"}},
			DoNothing: true,
		}).
		Create(&m)

	if res.Error != nil {
		var pgErr *pgconn.PgError
		if errors.As(res.Error, &pgErr) && pgErr.Code == "23505" { // I search this by ai
			return nil, apperr.ErrCodeCollision
		}
		return nil, res.Error
	}

	if res.RowsAffected == 0 {
		return s.FindByURL(ctx, sl.LongURL)
	}

	return m.toDomain(), nil
}

func (s *PostgresStore) IncrementUsedCount(ctx context.Context, code string) error {
	res := s.db.WithContext(ctx).
		Model(&ShortLinkModel{}).
		Where("code = ?", code).
		Update("used_count", gorm.Expr("used_count + 1")) // for being atomic in db

	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return apperr.ErrNotFound
	}

	return nil
}
