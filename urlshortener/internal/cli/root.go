// Package cli wires the Cobra command tree: root, serve, migrate.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/yazdanjavadi/urlshort/internal/config"
)

// NewRootCmd builds the root command. The config path comes from --config.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "urlshort",
		Short: "URL shortener service",
		Long:  "urlshort is a small URL-shortener service backed by PostgreSQL.",
		// No Run on root: requires a subcommand.
		SilenceUsage: true,
	}
	root.PersistentFlags().String("config", "", "path to config file (default: ./config.yaml or $HOME/.urlshort.yaml)")
	_ = viper.BindPFlag("config", root.PersistentFlags().Lookup("config"))

	root.AddCommand(newServeCmd())
	root.AddCommand(newMigrateCmd())

	return root
}

// loadConfig loads config from the --config flag value (if any). It does NOT
// hard-fail when no file is present — defaults keep the binary runnable. It
// only fails when a file was explicitly requested but is unreadable.
func loadConfig(cmd *cobra.Command) (config.Config, error) {
	path, _ := cmd.Flags().GetString("config")

	return config.Load(path)
}

// Execute runs the root command and exits with the right status code.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
