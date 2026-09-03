package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileRegistryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry.json")
	enabled := true
	r, err := loadFileRegistry(path, []string{"https://a.example/sub"}, []SiteConfig{{URL: "https://www.google.com", Filename: "google.txt", Enabled: &enabled}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Subscriptions) != 1 || len(r.Sites) != 1 {
		t.Fatalf("seed=%#v", r)
	}
	r.addSubscriptions([]string{"https://b.example/sub", "https://a.example/sub"})
	if len(r.Subscriptions) != 2 {
		t.Fatalf("dedup=%v", r.Subscriptions)
	}
	r.removeSubscriptions([]string{"https://a.example/sub"})
	if len(r.Subscriptions) != 1 || r.Subscriptions[0] != "https://b.example/sub" {
		t.Fatalf("remove=%v", r.Subscriptions)
	}
	// Reload persists.
	r2, err := loadFileRegistry(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Subscriptions) != 1 || len(r2.Sites) != 1 {
		t.Fatalf("reload=%#v", r2)
	}
	r2.removeSite("https://www.google.com")
	if len(r2.Sites) != 0 {
		t.Fatalf("site remove=%#v", r2.Sites)
	}
}

func TestServiceUsesFileRegistryWithoutRedis(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REDIS_URL", "")
	t.Setenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD", "1")
	t.Setenv("V2RAYS_REGISTRY_PATH", filepath.Join(dir, "registry.json"))
	t.Setenv("YAML_CONFIG_PATH", filepath.Join(dir, "missing.yaml"))
	cfg := Config{
		StatePath: filepath.Join(dir, "state.json"), GeoIPPath: filepath.Join(dir, "missing.mmdb"),
		SubscriptionURLs: []string{"https://a.example/sub"}, MaxFailCount: 1, MaxDelayMS: 1000,
		BatchSize: 1, MaxConcurrentBatches: 1, MaxConcurrentTests: 1, TestAttempts: 1,
		BasePort: 20000, SingBoxPath: filepath.Join(dir, "missing-sing-box"),
		ConfigPath: filepath.Join(dir, "missing.yaml"),
	}
	// Point helpers at the temp registry.
	t.Setenv("V2RAYS_DATA_DIR", dir)
	svc, err := NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if len(svc.Subscriptions()) != 1 {
		t.Fatalf("subs=%v", svc.Subscriptions())
	}
	// Corrupt PATH resolution must not occur: missing sing-box falls back.
	if svc.config.SingBoxPath == "" {
		t.Fatal("sing-box path empty")
	}
}

func TestExportServersFiltersAndLimits(t *testing.T) {
	servers := []ProxyServer{
		{RawURI: "vless://a", CountryCode: "US"},
		{RawURI: "vless://b", CountryCode: "DE"},
		{RawURI: "vless://c", CountryCode: "US"},
	}
	out := filepath.Join(t.TempDir(), "out.txt")
	if code := exportServers(servers, "raw", "US", 1, out); code != 0 {
		t.Fatalf("code=%d", code)
	}
	data, _ := os.ReadFile(out)
	if string(data) != "vless://a\n" {
		t.Fatalf("content=%q", data)
	}
}

func TestCLIDispatchUnknown(t *testing.T) {
	if code := RunCLI([]string{"v2rays", "nope"}); code != 2 {
		t.Fatalf("code=%d", code)
	}
	if code := RunCLI([]string{"v2rays", "version"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
}

func TestLoadConfigReadsXDGEnvFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("V2RAYS_CONFIG_DIR", dir)
	t.Setenv("V2RAYS_DATA_DIR", dir)
	// Sentinel: must be picked up from the XDG file.
	t.Setenv("UNSET_SENTINEL_CHECK", "")
	os.Unsetenv("V2RAYS_DOTENV_TEST_KEY")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("V2RAYS_DOTENV_TEST_KEY=xdg-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loadDotEnvFiles()
	if got := os.Getenv("V2RAYS_DOTENV_TEST_KEY"); got != "xdg-value" {
		t.Fatalf("dotenv not loaded: %q", got)
	}
	// Existing process env (even empty) is never overwritten.
	t.Setenv("V2RAYS_DOTENV_TEST_KEY2", "")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("V2RAYS_DOTENV_TEST_KEY2=file-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loadDotEnvFiles()
	if got := os.Getenv("V2RAYS_DOTENV_TEST_KEY2"); got != "" {
		t.Fatalf("present env overwritten: %q", got)
	}
}

func TestUpsertXDGEnvRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("V2RAYS_CONFIG_DIR", dir)
	if err := upsertXDGEnv("MANAGEMENT_TOKEN", "tok123", 0600); err != nil {
		t.Fatal(err)
	}
	if err := upsertXDGEnv("SUB_URLS", "https://a.example/sub", 0600); err != nil {
		t.Fatal(err)
	}
	if got := readEnvToken(filepath.Join(dir, ".env")); got != "tok123" {
		t.Fatalf("token=%q", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, ".env"))
	if !strings.Contains(string(data), "SUB_URLS=https://a.example/sub") {
		t.Fatalf("content=%q", data)
	}
}
