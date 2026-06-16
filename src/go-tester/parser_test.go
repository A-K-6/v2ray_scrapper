package main

import (
	"encoding/json"
	"testing"
)

func TestParseVless(t *testing.T) {
	uri := "vless://4ff03ebf-8f83-4a17-bafe-a1aab0c8b919@1.2.3.4:443?encryption=none&security=reality&type=tcp&sni=example.com&fp=chrome&pbk=publickey123&sid=shortid123#MyVless"
	outbound, err := parseRawURI(uri)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if outbound.Protocol != "vless" {
		t.Errorf("expected protocol vless, got %s", outbound.Protocol)
	}

	outBytes, _ := json.MarshalIndent(outbound, "", "  ")
	t.Logf("Parsed Outbound: %s", string(outBytes))
}

func TestParseVmess(t *testing.T) {
	// Simple vmess config
	// {"v":"2","ps":"MyVmess","add":"1.2.3.4","port":"443","id":"uuid123","aid":0,"scy":"auto","net":"ws","type":"none","host":"example.com","path":"/path","tls":"tls","sni":"example.com"}
	uri := "vmess://eyJ2IjoiMiIsInBzIjoiTXlWbWVzcyIsImFkZCI6IjEuMi4zLjQiLCJwb3J0IjoiNDQzIiwiaWQiOiJ1dWlkMTIzIiwiYWlkIjowLCJzY3kiOiJhdXRvIiwibmV0Ijoid3MiLCJ0eXBlIjoibm9uZSIsImhvc3QiOiJleGFtcGxlLmNvbSIsInBhdGgiOiIvcGF0aCIsInRscyI6InRscyIsInNuaSI6ImV4YW1wbGUuY29tIn0="
	outbound, err := parseRawURI(uri)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if outbound.Protocol != "vmess" {
		t.Errorf("expected protocol vmess, got %s", outbound.Protocol)
	}

	outBytes, _ := json.MarshalIndent(outbound, "", "  ")
	t.Logf("Parsed Outbound: %s", string(outBytes))
}
