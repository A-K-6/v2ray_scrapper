// Package xdg resolves user-specific config and data directories.
package xdg

import (
	"os"
	"path/filepath"
	"runtime"
)

// ConfigDir returns ~/.config/v2rays (or OS equivalent).
func ConfigDir() string {
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

// DataDir returns the writable data directory for state, registry, sing-box, geoip.
func DataDir() string {
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

// EnsureDir creates a directory and parents when missing.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}
