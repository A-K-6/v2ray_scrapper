package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aeen/v2ray-scrapper/internal/config"
	"github.com/aeen/v2ray-scrapper/internal/proxy"
	"github.com/aeen/v2ray-scrapper/internal/tester"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	t.Setenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD", "1")
	t.Setenv("REDIS_URL", "")
	service, err := NewService(config.Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb", MaxFailCount: 3, MaxDelayMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func TestProcessResultsEnrichesSuccessAndRetainsTemporaryFailure(t *testing.T) {
	service := newTestService(t)
	success := proxy.ProxyServer{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "ok", RawURI: "vless://ok@1.2.3.4:443"}
	failure := proxy.ProxyServer{Protocol: "vless", Address: "1.2.3.5", Port: 443, ID: "fail", RawURI: "vless://fail@1.2.3.5:443"}
	working, retained := service.processResults([]tester.TestResult{{Server: success, Delay: 42}, {Server: failure, Delay: -1, Failed: true}}, 1000)
	if len(working) != 1 || working[0].Delay != 42 || working[0].CountryCode != "UN" || working[0].RawURI == success.RawURI {
		t.Fatalf("working=%#v", working)
	}
	if len(retained) != 2 {
		t.Fatalf("retained=%d", len(retained))
	}
}

func TestServiceBusyGate(t *testing.T) {
	service := newTestService(t)
	if !service.tryBegin() {
		t.Fatal("first job should start")
	}
	if service.tryBegin() {
		t.Fatal("second job should be rejected")
	}
	if !service.Processing() {
		t.Fatal("service should report processing")
	}
	service.end()
	if service.Processing() {
		t.Fatal("service should be idle")
	}
	servers, err := service.TestContent(context.Background(), "", "", 0, 0)
	if err != nil || len(servers) != 0 {
		t.Fatalf("empty test: %v %#v", err, servers)
	}
}

func TestServiceUsesFileRegistryWithoutRedis(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REDIS_URL", "")
	t.Setenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD", "1")
	t.Setenv("V2RAYS_REGISTRY_PATH", filepath.Join(dir, "registry.json"))
	t.Setenv("YAML_CONFIG_PATH", filepath.Join(dir, "missing.yaml"))
	cfg := config.Config{
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
	// Missing sing-box must fall back without network access.
	if svc.SingBoxPath() == "" {
		t.Fatal("sing-box path empty")
	}
}
