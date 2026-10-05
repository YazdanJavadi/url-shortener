// Package repository defines the persistence interface for URL records and a
// GORM-backed implementation. The interface enables dependency injection so
// the HTTP layer and tests can depend on an abstraction rather than a concrete
// database.
package repository

import (
	"context"

	"github.com/yazdanjavadi/urlshort/internal/models"
)

// Repository is the persistence contract for URL mappings.
type Repository interface {
	// Save persists a new code->longURL mapping. It returns ErrDuplicateCode if
	// the code already exists (caller should retry with a fresh code).
	Save(ctx context.Context, code, longURL string) error
	// FindByCode returns the URL for the given code, or ErrNotFound.
	FindByCode(ctx context.Context, code string) (*models.URL, error)
	// FindByLongURL returns an existing mapping for the long URL, or
	// ErrNotFound. Used to dedup so the same long URL keeps one code.
	FindByLongURL(ctx context.Context, longURL string) (*models.URL, error)
}
