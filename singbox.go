package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const singBoxVersion = "1.13.12"

// Checksums mirror the Dockerfile pins (linux only). Darwin/windows are
// resolved via the GitHub release page at install time; for auto-provision
// we download without a pinned hash and rely on HTTPS + version tag.
var singBoxChecksums = map[string]string{
	"linux/amd64": "1540533adb3df24f5ad5f14b5c7ca3dbc2401b10a1c1eb278fcadcada47ec6c4",
	"linux/arm64": "1ffa3b48ad6fa98f9fd810482e39bdd5b6157782ef11ce37d67bdcfd9338547a",
}

// resolveSingBox returns an executable sing-box path, downloading a pinned
// release into the user data dir when no usable binary exists.
func resolveSingBox(configured string) string {
	candidates := []string{}
	if configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, defaultSingBoxPath(), "/usr/local/bin/sing-box", "/usr/bin/sing-box")
	for _, p := range candidates {
		if isExecutable(p) {
			return p
		}
	}
	if os.Getenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD") != "" {
		if configured != "" {
			return configured
		}
		return defaultSingBoxPath()
	}
	target := defaultSingBoxPath()
	if err := ensureSingBox(target); err != nil {
		// Return the configured path anyway; the tester will surface a
		// clear "failed to start" error instead of crashing here.
		if configured != "" {
			return configured
		}
		return target
	}
	return target
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func ensureSingBox(target string) error {
	pair := runtime.GOOS + "/" + runtime.GOARCH
	ext := "tar.gz"
	platform := "linux"
	switch runtime.GOOS {
	case "darwin":
		platform = "darwin"
	case "windows":
		platform = "windows"
		ext = "zip"
	case "linux":
		platform = "linux"
	default:
		return fmt.Errorf("automatic sing-box download is not supported on %s", pair)
	}
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("automatic sing-box download is not supported on %s", pair)
	}
	archive := fmt.Sprintf("sing-box-%s-%s-%s.%s", singBoxVersion, platform, arch, ext)
	url := fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/v%s/%s", singBoxVersion, archive)

	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download sing-box: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download sing-box: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "sing-box-download-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hasher), resp.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	if sum, ok := singBoxChecksums[pair]; ok {
		if got := hex.EncodeToString(hasher.Sum(nil)); !strings.EqualFold(got, sum) {
			return fmt.Errorf("sing-box checksum mismatch")
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	binName := "sing-box"
	if runtime.GOOS == "windows" {
		binName = "sing-box.exe"
	}
	var extracted string
	if ext == "zip" {
		extracted, err = extractSingBoxZip(tmpName, binName)
	} else {
		extracted, err = extractSingBoxTarGz(tmpName, binName)
	}
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(extracted))
	if err := os.Rename(extracted, target); err != nil {
		// Cross-device fallback.
		in, err2 := os.Open(extracted)
		if err2 != nil {
			return err
		}
		defer in.Close()
		out, err2 := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err2 != nil {
			return err2
		}
		defer out.Close()
		if _, err2 := io.Copy(out, in); err2 != nil {
			return err2
		}
	}
	return os.Chmod(target, 0755)
}

func extractSingBoxTarGz(archive, binName string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	dir, err := os.MkdirTemp("", "sing-box-*")
	if err != nil {
		return "", err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		base := filepath.Base(hdr.Name)
		if base != binName && base != "sing-box.exe" {
			continue
		}
		out := filepath.Join(dir, binName)
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(w, tr); err != nil {
			w.Close()
			return "", err
		}
		w.Close()
		return out, nil
	}
	return "", fmt.Errorf("sing-box binary not found in archive")
}

func extractSingBoxZip(archive, binName string) (string, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer r.Close()
	dir, err := os.MkdirTemp("", "sing-box-*")
	if err != nil {
		return "", err
	}
	for _, f := range r.File {
		if filepath.Base(f.Name) != binName && filepath.Base(f.Name) != "sing-box.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out := filepath.Join(dir, binName)
		w, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(w, rc)
		w.Close()
		rc.Close()
		if err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("sing-box binary not found in archive")
}
