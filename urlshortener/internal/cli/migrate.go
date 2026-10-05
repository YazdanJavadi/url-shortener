package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yazdanjavadi/urlshort/internal/database"
	"github.com/yazdanjavadi/urlshort/internal/logging"
)

// newMigrateCmd builds the `migrate` subcommand. It connects to the database
// and creates the necessary tables if they don't already exist.
func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Create database tables if they don't exist",
		RunE:  runMigrate,
	}
}

// runMigrate connects to the DB and runs AutoMigrate. Idempotent.
func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	log := logging.New(cfg.Log)

	log.WithField("host", cfg.DB.Host).Info("connecting to database")
	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.WithError(err).Error("cannot connect to database")

		return fmt.Errorf("connect db: %w", err)
	}

	if err := database.Migrate(db); err != nil {
		log.WithError(err).Error("migration failed")

		return err
	}

	log.Info("migration complete")
	fmt.Println("migrate: schema up to date")

	return nil
}
