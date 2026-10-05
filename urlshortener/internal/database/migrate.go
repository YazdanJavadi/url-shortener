package database

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/yazdanjavadi/urlshort/internal/models"
)

// Migrate creates the database tables for the known models if they don't exist.
// It is idempotent: running it repeatedly is safe.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.URL{}); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	return nil
}
