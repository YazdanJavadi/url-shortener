package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/yazdanjavadi/urlshort/internal/clock"
	"github.com/yazdanjavadi/urlshort/internal/models"
)

// gormRepo is the production Repository implementation backed by GORM.
type gormRepo struct {
	db    *gorm.DB
	clock clock.Clock
}

// New returns a Repository backed by the given GORM DB and clock. The clock is
// used to stamp CreatedAt/UpdatedAt so inserts can be tested deterministically.
func New(db *gorm.DB, clk clock.Clock) Repository {
	if clk == nil {
		clk = clock.New()
	}

	return &gormRepo{db: db, clock: clk}
}

// Save inserts a new URL record. A unique-constraint violation on Code is
// translated to ErrDuplicateCode so callers can retry with a fresh code.
// CreatedAt/UpdatedAt are set from the injected clock (overriding GORM's
// auto-stamp) to make the operation testable with a fixed time.
func (r *gormRepo) Save(ctx context.Context, code, longURL string) error {
	now := r.clock.Now()
	rec := &models.URL{
		Code:      code,
		LongURL:   longURL,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		if isDuplicateKeyError(err) {
			return ErrDuplicateCode
		}

		return err
	}

	return nil
}

// FindByCode loads the URL with the given code.
func (r *gormRepo) FindByCode(ctx context.Context, code string) (*models.URL, error) {
	var rec models.URL
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &rec, nil
}

// FindByLongURL loads an existing mapping for the given long URL.
func (r *gormRepo) FindByLongURL(ctx context.Context, longURL string) (*models.URL, error) {
	var rec models.URL
	err := r.db.WithContext(ctx).Where("long_url = ?", longURL).First(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}

		return nil, err
	}

	return &rec, nil
}

// isDuplicateKeyError reports whether err is a Postgres unique-violation.
// GORM's postgres driver surfaces these with SQLSTATE 23505 (unique_violation).
func isDuplicateKeyError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
