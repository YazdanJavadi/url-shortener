// Package config loads application configuration via Viper.
//
// Config is searched in (first match wins):
//  1. explicit --config <path> flag
//  2. ./config.yaml in the working directory
//  3. $HOME/.urlshort.yaml
//
// Environment variables override file values (URLSHORT_DB_HOST, etc.).
// If no file is found the defaults baked into Defaults() are used so the
// binary still runs without a config file present.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config is the in-memory representation of configuration.
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	DB      DBConfig      `mapstructure:"db"`
	URL     URLConfig     `mapstructure:"url"`
	Cache   CacheConfig   `mapstructure:"cache"`
	Tracing TracingConfig `mapstructure:"tracing"`
	Log     LogConfig     `mapstructure:"log"`
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	Port    int    `mapstructure:"port"`
	BaseURL string `mapstructure:"base_url"`
}

// DBConfig describes the database connection.
type DBConfig struct {
	Driver          string `mapstructure:"driver"`
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	User            string `mapstructure:"user"`
	Password        string `mapstructure:"password"`
	Name            string `mapstructure:"name"`
	SSLMode         string `mapstructure:"ssl_mode"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	ConnMaxLifetime int    `mapstructure:"conn_max_lifetime_seconds"`
}

// URLConfig controls short-code generation.
type URLConfig struct {
	CodeLength int `mapstructure:"code_length"`
}

// CacheConfig controls the Redis cache.
type CacheConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Address    string `mapstructure:"address"`
	TTLSeconds int    `mapstructure:"ttl_seconds"`
}

// TracingConfig controls OpenTelemetry tracing export.
type TracingConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ServiceName  string `mapstructure:"service_name"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
	Insecure     bool   `mapstructure:"insecure"`
}

// LogConfig controls logging behavior.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// Defaults returns a Config with sensible default values. Used as the baseline
// when no config file is present and no env vars are set.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Port:    8080,
			BaseURL: "http://localhost:8080",
		},
		DB: DBConfig{
			Driver:          "postgres",
			Host:            "localhost",
			Port:            5432,
			User:            "urlshort",
			Password:        "urlshort",
			Name:            "urlshort",
			SSLMode:         "disable",
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 300,
		},
		URL: URLConfig{
			CodeLength: 6,
		},
		Cache: CacheConfig{
			Enabled:    true,
			Address:    "localhost:6379",
			TTLSeconds: 3600,
		},
		Tracing: TracingConfig{
			Enabled:      true,
			ServiceName:  "urlshort",
			OTLPEndpoint: "localhost:4318",
			Insecure:     true,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// DSN builds a Postgres connection string from the DB config.
func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// Load reads configuration from the given path (may be empty) and merges it
// with env vars and defaults. Returns an error only if a file was explicitly
// requested but could not be read.
func Load(path string) (Config, error) {
	v := viper.New()

	// Env var overrides: URLSHORT_<SECTION>_<KEY>, e.g. URLSHORT_DB_HOST.
	v.SetEnvPrefix("urlshort")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Defaults so the binary runs even with no file.
	d := Defaults()
	v.SetDefault("server.port", d.Server.Port)
	v.SetDefault("server.base_url", d.Server.BaseURL)
	v.SetDefault("db.driver", d.DB.Driver)
	v.SetDefault("db.host", d.DB.Host)
	v.SetDefault("db.port", d.DB.Port)
	v.SetDefault("db.user", d.DB.User)
	v.SetDefault("db.password", d.DB.Password)
	v.SetDefault("db.name", d.DB.Name)
	v.SetDefault("db.ssl_mode", d.DB.SSLMode)
	v.SetDefault("db.max_open_conns", d.DB.MaxOpenConns)
	v.SetDefault("db.max_idle_conns", d.DB.MaxIdleConns)
	v.SetDefault("db.conn_max_lifetime_seconds", d.DB.ConnMaxLifetime)
	v.SetDefault("url.code_length", d.URL.CodeLength)
	v.SetDefault("cache.enabled", d.Cache.Enabled)
	v.SetDefault("cache.address", d.Cache.Address)
	v.SetDefault("cache.ttl_seconds", d.Cache.TTLSeconds)
	v.SetDefault("tracing.enabled", d.Tracing.Enabled)
	v.SetDefault("tracing.service_name", d.Tracing.ServiceName)
	v.SetDefault("tracing.otlp_endpoint", d.Tracing.OTLPEndpoint)
	v.SetDefault("tracing.insecure", d.Tracing.Insecure)
	v.SetDefault("log.level", d.Log.Level)
	v.SetDefault("log.format", d.Log.Format)

	switch {
	case path != "":
		v.SetConfigFile(path)
	case fileExists("config.yaml"):
		v.SetConfigFile("config.yaml")
	case fileExists(homeConfigPath()):
		v.SetConfigFile(homeConfigPath())
	}

	// If a config file was set, read it; an error here is fatal only when the
	// user explicitly named a file via --config.
	if v.ConfigFileUsed() != "" {
		if err := v.ReadInConfig(); err != nil {
			if path != "" {
				return Config{}, fmt.Errorf("read config %s: %w", path, err)
			}
			// Non-explicit file read error: fall back to defaults silently.
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}
