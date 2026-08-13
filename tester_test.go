package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedConfigurationPassesSingBoxCheck(t *testing.T) {
	binary := os.Getenv("SING_BOX_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set SING_BOX_INTEGRATION_BINARY to run the core integration check")
	}
	servers := []ProxyServer{
		{Protocol: "vless", Address: "vless.example", Port: 443, ID: "bf000d23-0752-40b4-affe-68f7707a9661", Security: "tls", Network: "ws", Host: "vless.example", Path: "/ws"},
		{Protocol: "vless", Address: "reality.example", Port: 443, ID: "bf000d23-0752-40b4-affe-68f7707a9661", Security: "reality", PublicKey: "jNXHt1yRo0vDuchQlIP6Z0ZvjT3KtzVI-T4E7RoLJS0", ShortID: "0123456789abcdef"},
		{Protocol: "vmess", Address: "vmess.example", Port: 443, VMessID: "bf000d23-0752-40b4-affe-68f7707a9661", Security: "auto", Encryption: "tls", Network: "grpc", Path: "/TunService"},
		{Protocol: "trojan", Address: "trojan.example", Port: 443, Password: "secret", Network: "httpupgrade", Host: "trojan.example", Path: "/upgrade"},
		{Protocol: "shadowsocks", Address: "ss.example", Port: 443, Method: "aes-256-cfb", Password: "secret"},
		{Protocol: "hysteria2", Address: "hy2.example", Port: 443, Password: "secret", SNI: "hy2.example", Insecure: true, Obfs: "salamander", ObfsPassword: "cover"},
	}
	configuration, _, err := buildSingBoxConfiguration(servers, 25000)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sing-box.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("sing-box check: %v\n%s", err, output)
	}
}

func TestTesterReturnsFailureWhenSingBoxCannotStart(t *testing.T) {
	tester := NewTester(Config{SingBoxPath: "/definitely/missing/sing-box", TestTimeout: time.Millisecond, SingBoxStartTimeout: time.Millisecond, BatchSize: 10, MaxConcurrentBatches: 1, BasePort: 20000})
	servers := []ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: testUUIDA}}
	results := tester.Test(context.Background(), servers, "https://example.com", false)
	if len(results) != 1 || !results[0].Failed || results[0].Delay != -1 {
		t.Fatalf("results=%#v", results)
	}
}

func TestFailedResultsPreservesServers(t *testing.T) {
	servers := []ProxyServer{{Address: "one"}, {Address: "two"}}
	results := failedResults(servers)
	if len(results) != 2 || results[1].Server.Address != "two" || !results[0].Failed {
		t.Fatalf("results=%#v", results)
	}
}

func TestWaitForPortReturnsWhenSingBoxExits(t *testing.T) {
	exited := make(chan error, 1)
	exited <- nil
	ready, err := waitForPort(context.Background(), 1, time.Second, exited)
	if ready || err == nil {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestCombinedProcessOutputIncludesStdoutAndStderr(t *testing.T) {
	got := combinedProcessOutput(" config rejected \n", " details \n")
	if got != "config rejected\ndetails" {
		t.Fatalf("output=%q", got)
	}
}

func TestInvalidOutboundIndexParsesCoreError(t *testing.T) {
	output := "failed to build outbound config with tag out-27 > unknown cipher"
	if got := invalidOutboundIndex(output); got != 27 {
		t.Fatalf("index=%d", got)
	}
}

func TestGenerate204ProbeRequiresExactStatus(t *testing.T) {
	if successfulProbe(200, "https://example.com/generate_204", false) {
		t.Fatal("a captive/intercepted 200 response must not pass a 204 probe")
	}
	if !successfulProbe(204, "https://example.com/generate_204", false) {
		t.Fatal("204 response should pass")
	}
}
