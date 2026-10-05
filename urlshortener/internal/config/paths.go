package config

import (
	"os"
	"path/filepath"
)

// fileExists reports whether a path exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)

	return err == nil && !info.IsDir()
}

// homeConfigPath returns the path to a user-level config file under $HOME.
func homeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".urlshort.yaml")
}
