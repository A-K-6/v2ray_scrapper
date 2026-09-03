package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// appName is the user-facing binary/command name.
const appName = "v2rays"

// configDir returns ~/.config/v2rays (or OS equivalent).
func configDir() string {
	if v := os.Getenv("V2RAYS_CONFIG_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("AppData"); v != "" {
			return filepath.Join(v, "v2rays")
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "v2rays")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "v2rays")
	}
	return filepath.Join(home, ".config", "v2rays")
}

// dataDir returns the writable data directory for state, registry, sing-box, geoip.
func dataDir() string {
	if v := os.Getenv("V2RAYS_DATA_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LocalAppData"); v != "" {
			return filepath.Join(v, "v2rays")
		}
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "v2rays")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "data"
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "v2rays")
	}
	return filepath.Join(home, ".local", "share", "v2rays")
}

func defaultStatePath() string {
	// Docker images set STATE_FILE_PATH explicitly; only apply XDG default
	// when the caller did not configure anything.
	if v := os.Getenv("STATE_FILE_PATH"); v != "" {
		return v
	}
	// Repo-local dev: if ./data exists or ./config.yaml exists, stay local.
	if _, err := os.Stat("data"); err == nil {
		return filepath.Join("data", "state.json")
	}
	return filepath.Join(dataDir(), "state.json")
}

func defaultConfigPath() string {
	if v := os.Getenv("YAML_CONFIG_PATH"); v != "" {
		return v
	}
	for _, p := range []string{"config.yaml", filepath.Join(configDir(), "config.yaml")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(configDir(), "config.yaml")
}

func defaultGeoIPPath() string {
	if v := os.Getenv("GEOIP_DB_PATH"); v != "" {
		return v
	}
	for _, p := range []string{"src/Country.mmdb", "Country.mmdb", filepath.Join(dataDir(), "Country.mmdb")} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dataDir(), "Country.mmdb")
}

func defaultSingBoxPath() string {
	if v := os.Getenv("SING_BOX_PATH"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(dataDir(), "bin", "sing-box.exe")
	}
	// Honor system installs first.
	for _, p := range []string{"/usr/local/bin/sing-box", "/usr/bin/sing-box"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(dataDir(), "bin", "sing-box")
}

func registryPath() string {
	if v := os.Getenv("V2RAYS_REGISTRY_PATH"); v != "" {
		return v
	}
	// Keep repo-local behaviour for docker-compose checkouts.
	if _, err := os.Stat("data"); err == nil {
		return filepath.Join("data", "registry.json")
	}
	return filepath.Join(dataDir(), "registry.json")
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
