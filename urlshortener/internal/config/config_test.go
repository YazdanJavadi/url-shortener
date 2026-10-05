package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ConfigSuite struct{ suite.Suite }

func TestConfigSuite(t *testing.T) { suite.Run(t, new(ConfigSuite)) }

// TestConfig_Defaults_SaneValues_Success verifies baked-in defaults.
func (s *ConfigSuite) TestConfig_Defaults_SaneValues_Success() {
	require := s.Require()
	expectedCacheAddress := "localhost:6379"

	d := Defaults()

	require.Equal(8080, d.Server.Port)
	require.Equal("postgres", d.DB.Driver)
	require.Equal(6, d.URL.CodeLength)
	require.True(d.Cache.Enabled)
	require.Equal(expectedCacheAddress, d.Cache.Address)
}

// TestConfig_DSN_Success verifies the Postgres DSN string builder.
func (s *ConfigSuite) TestConfig_DSN_Success() {
	require := s.Require()
	expectedDSN := "host=h port=5433 user=u password=p dbname=n sslmode=disable"

	d := DBConfig{Host: "h", Port: 5433, User: "u", Password: "p", Name: "n", SSLMode: "disable"}

	require.Equal(expectedDSN, d.DSN())
}

// TestConfig_Load_NoFile_UsesDefaults_Success verifies defaults apply with no config file.
func (s *ConfigSuite) TestConfig_Load_NoFile_UsesDefaults_Success() {
	require := s.Require()
	expectedPort := 8080

	chdir(s.T(), s.T().TempDir())
	cfg, err := Load("")

	require.NoError(err)
	require.Equal(expectedPort, cfg.Server.Port)
}

// TestConfig_Load_ReadsFile_Success verifies a config file is read and applied.
func (s *ConfigSuite) TestConfig_Load_ReadsFile_Success() {
	require := s.Require()
	expectedPort := 9999
	expectedHost := "dbhost"

	path := filepath.Join(s.T().TempDir(), "config.yaml")
	require.NoError(os.WriteFile(path, []byte("server:\n  port: 9999\ndb:\n  host: dbhost\n"), 0o644))
	cfg, err := Load(path)

	require.NoError(err)
	require.Equal(expectedPort, cfg.Server.Port)
	require.Equal(expectedHost, cfg.DB.Host)
}

// TestConfig_Load_ExplicitMissingFile_Failure verifies an explicit missing file errors.
func (s *ConfigSuite) TestConfig_Load_ExplicitMissingFile_Failure() {
	require := s.Require()

	_, err := Load("/nonexistent/path.yaml")

	require.Error(err)
}

// TestConfig_Load_EnvOverride_Success verifies env vars override config values.
func (s *ConfigSuite) TestConfig_Load_EnvOverride_Success() {
	require := s.Require()
	expectedHost := "envhost"
	expectedPort := 7000

	chdir(s.T(), s.T().TempDir())
	s.T().Setenv("URLSHORT_DB_HOST", "envhost")
	s.T().Setenv("URLSHORT_DB_PORT", "7000")
	cfg, err := Load("")

	require.NoError(err)
	require.Equal(expectedHost, cfg.DB.Host)
	require.Equal(expectedPort, cfg.DB.Port)
}

// TestConfig_FileExists_Success verifies the fileExists helper.
func (s *ConfigSuite) TestConfig_FileExists_Success() {
	require := s.Require()

	require.False(fileExists("/nonexistent"))
	require.True(fileExists("/etc/hosts"))
}

// TestConfig_HomeConfigPath_Success verifies the home config path is non-empty.
func (s *ConfigSuite) TestConfig_HomeConfigPath_Success() {
	require := s.Require()

	p := homeConfigPath()

	require.NotEmpty(p)
}

// chdir changes the working directory for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chdir(orig) })
}
