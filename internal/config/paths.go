package config

import (
	"os"
	"path/filepath"

	"github.com/aeen/v2ray-scrapper/internal/xdg"
)

func defaultStatePath() string {
	// Docker images set STATE_FILE_PATH explicitly; only apply XDG default
	// when the caller did not configure anything.
	if v := os.Getenv("STATE_FILE_PATH"); v != "" {
		return v
	}
	// Repo-local dev: if ./data exists, stay local.
	if _, err := os.Stat("data"); err == nil {
		return filepath.Join("data", "state.json")
	}
	return filepath.Join(xdg.DataDir(), "state.json")
}

// DefaultConfigPath resolves the YAML config location for display and setup.
func DefaultConfigPath() string {
	if v := os.Getenv("YAML_CONFIG_PATH"); v != "" {
		return v
	}
	for _, p := range []string{"config.yaml", filepath.Join(xdg.ConfigDir(), "config.yaml")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(xdg.ConfigDir(), "config.yaml")
}

func defaultGeoIPPath() string {
	if v := os.Getenv("GEOIP_DB_PATH"); v != "" {
		return v
	}
	for _, p := range []string{"assets/Country.mmdb", "Country.mmdb", "src/Country.mmdb", filepath.Join(xdg.DataDir(), "Country.mmdb")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(xdg.DataDir(), "Country.mmdb")
}
