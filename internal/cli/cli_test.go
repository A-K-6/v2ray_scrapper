package cli

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeen/v2ray-scrapper/internal/proxy"
)

func TestExportServersFiltersAndLimits(t *testing.T) {
	servers := []proxy.ProxyServer{
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

func TestServeFailsFastOnOccupiedPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("V2RAYS_CONFIG_DIR", dir)
	t.Setenv("V2RAYS_DATA_DIR", dir)
	t.Setenv("V2RAYS_REGISTRY_PATH", filepath.Join(dir, "registry.json"))
	t.Setenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD", "1")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener available")
	}
	defer ln.Close()
	if code := runServe([]string{"--addr", ln.Addr().String()}); code != 1 {
		t.Fatalf("code=%d, want 1", code)
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
