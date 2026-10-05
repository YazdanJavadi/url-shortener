package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/yazdanjavadi/urlshort/internal/cache"
	"github.com/yazdanjavadi/urlshort/internal/clock"
	"github.com/yazdanjavadi/urlshort/internal/database"
	"github.com/yazdanjavadi/urlshort/internal/logging"
	"github.com/yazdanjavadi/urlshort/internal/repository"
	"github.com/yazdanjavadi/urlshort/internal/server"
	"github.com/yazdanjavadi/urlshort/internal/tracing"
	"github.com/yazdanjavadi/urlshort/internal/urlservice"
	"github.com/yazdanjavadi/urlshort/internal/worker"
)

// newServeCmd builds the `serve` subcommand. It accepts an optional --port
// switch; if omitted, the port from config (or default 8080) is used.
func newServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the URL shortener HTTP server",
		RunE:  runServe,
	}
	cmd.Flags().Int("port", 0, "port to listen on (overrides config)")

	return cmd
}

// runServe wires all dependencies (DI) and starts the HTTP server. If the
// database is unavailable it fails fast rather than serving broken requests.
func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}

	log := logging.New(cfg.Log)

	// --port overrides the config port.
	portFlag, _ := cmd.Flags().GetInt("port")
	if portFlag > 0 {
		cfg.Server.Port = portFlag
	}

	// OpenTelemetry tracing -> Jaeger (OTLP). Disabled in config => skip.
	if cfg.Tracing.Enabled {
		shutdown, err := tracing.Init(cmd.Context(), tracing.Config{
			ServiceName: cfg.Tracing.ServiceName,
			Endpoint:    cfg.Tracing.OTLPEndpoint,
			Insecure:    cfg.Tracing.Insecure,
		})
		if err != nil {
			log.WithError(err).Error("cannot init tracing")

			return fmt.Errorf("init tracing: %w", err)
		}

		defer func() {
			if err := shutdown(context.Background()); err != nil {
				log.WithError(err).Warn("tracer shutdown failed")
			}
		}()
		log.WithField("endpoint", cfg.Tracing.OTLPEndpoint).Info("tracing initialized")
	}

	// Connect to the database. This is an essential resource — fail fast.
	log.WithField("host", cfg.DB.Host).Info("connecting to database")
	db, err := database.Connect(cfg.DB)
	if err != nil {
		log.WithError(err).Error("cannot connect to database")

		return fmt.Errorf("connect db: %w", err)
	}

	log.Info("database connected")

	// Run migrations on startup so the schema is always current. Safe to skip
	// if the user prefers the explicit `migrate` command.
	if err := database.Migrate(db); err != nil {
		log.WithError(err).Error("migration failed")

		return fmt.Errorf("migrate: %w", err)
	}

	log.Info("database schema verified")

	// Build the dependency graph: repo -> cache -> worker pool -> service -> server.
	repo := repository.New(db, clock.New())

	// Cache (Redis). Disabled in config => Noop.
	var c cache.Cache = cache.Noop{}
	if cfg.Cache.Enabled {
		rdb, err := database.ConnectRedis(cfg.Cache.Address)
		if err != nil {
			log.WithError(err).Error("cannot configure redis")

			return fmt.Errorf("connect redis: %w", err)
		}

		if err := database.PingRedis(cmd.Context(), rdb); err != nil {
			log.WithError(err).Error("cannot reach redis")

			return fmt.Errorf("ping redis: %w", err)
		}

		c = cache.NewRedis(rdb, log.WithField("component", "cache"))
		log.Info("redis cache connected")
	}

	// Async insert worker pool (buffered job channel).
	pool := worker.New(repo.Save, 4, 256, log.WithField("component", "worker"))
	defer pool.Close()

	cacheTTL := time.Duration(cfg.Cache.TTLSeconds) * time.Second
	svc := urlservice.New(repo, c, log.WithField("component", "service"), cfg.URL.CodeLength, cacheTTL, pool)
	srv := server.New(svc, log, cfg.Server.BaseURL)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.WithFields(map[string]interface{}{
		"addr":     addr,
		"base_url": cfg.Server.BaseURL,
	}).Info("starting url shortener")

	if err := srv.Start(addr); err != nil {
		log.WithError(err).Error("server stopped")

		return err
	}

	return nil
}
