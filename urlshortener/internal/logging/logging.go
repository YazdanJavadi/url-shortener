// Package logging builds the application logger from configuration.
package logging

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/yazdanjavadi/urlshort/internal/config"
)

// New returns a Logrus logger configured from the config (level + format).
func New(cfg config.LogConfig) *logrus.Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)
	switch strings.ToLower(cfg.Format) {
	case "text":
		log.SetFormatter(&logrus.TextFormatter{})
	default:
		log.SetFormatter(&logrus.JSONFormatter{})
	}

	level, err := logrus.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = logrus.InfoLevel
	}

	log.SetLevel(level)

	return log
}
