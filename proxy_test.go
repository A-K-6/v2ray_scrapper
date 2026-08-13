package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestParseSupportedProtocols(t *testing.T) {
	vmessJSON, _ := json.Marshal(map[string]any{"v": "2", "ps": "node", "add": "1.2.3.4", "port": "443", "id": "uuid", "aid": 0, "scy": "auto", "net": "ws", "host": "example.com", "path": "/ws", "tls": "tls", "sni": "example.com"})
	tests := []struct{ name, uri, protocol string }{
		{"vless", "vless://uuid@1.2.3.4:443?encryption=none&security=reality&type=tcp&sni=example.com&fp=chrome&pbk=key&sid=id#node", "vless"},
		{"vmess", "vmess://" + base64.StdEncoding.EncodeToString(vmessJSON), "vmess"},
		{"trojan", "trojan://secret@1.2.3.4:443?security=tls&type=ws&path=%2Fws&sni=example.com#node", "trojan"},
		{"shadowsocks", "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret")) + "@1.2.3.4:443#node", "shadowsocks"},
		{"shadowsocks-legacy", "ss://" + base64.RawStdEncoding.EncodeToString([]byte("aes-256-gcm:secret@1.2.3.4:443")) + "#node", "shadowsocks"},
		{"hysteria2", "hy2://secret@1.2.3.4:443?sni=example.com&insecure=1#node", "hysteria2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, err := ParseProxyURI(test.uri)
			if err != nil {
				t.Fatal(err)
			}
			if server.Protocol != test.protocol {
				t.Fatalf("protocol=%q", server.Protocol)
			}
			if server.Address != "1.2.3.4" || server.Port != 443 {
				t.Fatalf("bad endpoint: %#v", server)
			}
			if _, err := ParseProxyURI(server.ToURI()); err != nil {
				t.Fatalf("round trip: %v", err)
			}
		})
	}
}

func TestParseBase64SubscriptionDeduplicates(t *testing.T) {
	uri := "vless://uuid@1.2.3.4:443?encryption=none&type=tcp"
	encoded := base64.StdEncoding.EncodeToString([]byte(uri + "\n" + uri + "#another-name"))
	servers := ParseSubscription(encoded)
	if len(servers) != 1 {
		t.Fatalf("got %d servers", len(servers))
	}
}

func TestEveryProtocolBuildsAnXrayOutbound(t *testing.T) {
	vmessJSON, _ := json.Marshal(map[string]any{"v": "2", "add": "1.2.3.4", "port": "443", "id": "uuid", "net": "tcp"})
	for _, uri := range []string{
		"vless://uuid@1.2.3.4:443?encryption=none&type=tcp",
		"vmess://" + base64.StdEncoding.EncodeToString(vmessJSON),
		"trojan://secret@1.2.3.4:443?type=tcp",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret")) + "@1.2.3.4:443",
		"hy2://secret@1.2.3.4:443?sni=example.com",
	} {
		server, err := ParseProxyURI(uri)
		if err != nil {
			t.Fatal(err)
		}
		outbound, err := server.XrayOutbound("test")
		if err != nil {
			t.Fatal(err)
		}
		if outbound["tag"] != "test" || outbound["protocol"] == "" {
			t.Fatalf("bad outbound: %#v", outbound)
		}
	}
}

func TestXrayOutboundRejectsRemovedHysteriaInsecureMode(t *testing.T) {
	server, err := ParseProxyURI("hy2://secret@1.2.3.4:443?sni=example.com&insecure=1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.XrayOutbound("test"); err == nil {
		t.Fatal("expected insecure Hysteria 2 node to be rejected")
	}
}

func TestXrayOutboundUsesNativeHysteriaVersionTwoShape(t *testing.T) {
	server, err := ParseProxyURI("hy2://secret@1.2.3.4:443?sni=example.com")
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := server.XrayOutbound("test")
	if err != nil {
		t.Fatal(err)
	}
	if outbound["protocol"] != "hysteria" {
		t.Fatalf("protocol=%v", outbound["protocol"])
	}
	stream := outbound["streamSettings"].(map[string]any)
	if stream["network"] != "hysteria" {
		t.Fatalf("stream=%#v", stream)
	}
}
