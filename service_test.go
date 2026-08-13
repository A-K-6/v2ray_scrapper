package main

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	service, err := NewService(Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb", MaxFailCount: 3, MaxDelayMS: 1000})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func TestProcessResultsEnrichesSuccessAndRetainsTemporaryFailure(t *testing.T) {
	service := newTestService(t)
	success := ProxyServer{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "ok", RawURI: "vless://ok@1.2.3.4:443"}
	failure := ProxyServer{Protocol: "vless", Address: "1.2.3.5", Port: 443, ID: "fail", RawURI: "vless://fail@1.2.3.5:443"}
	working, retained := service.processResults([]TestResult{{Server: success, Delay: 42}, {Server: failure, Delay: -1, Failed: true}}, 1000)
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
