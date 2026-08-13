package main

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

const (
	testUUIDA = "bf000d23-0752-40b4-affe-68f7707a9661"
	testUUIDB = "bf000d23-0752-40b4-affe-68f7707a9662"
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
	uri := "vless://" + testUUIDA + "@1.2.3.4:443?encryption=none&type=tcp"
	encoded := base64.StdEncoding.EncodeToString([]byte(uri + "\n" + uri + "#another-name"))
	servers := ParseSubscription(encoded)
	if len(servers) != 1 {
		t.Fatalf("got %d servers", len(servers))
	}
}

func TestEveryProtocolBuildsASingBoxOutbound(t *testing.T) {
	vmessJSON, _ := json.Marshal(map[string]any{"v": "2", "add": "1.2.3.4", "port": "443", "id": testUUIDA, "net": "tcp"})
	for _, uri := range []string{
		"vless://" + testUUIDA + "@1.2.3.4:443?encryption=none&type=tcp",
		"vmess://" + base64.StdEncoding.EncodeToString(vmessJSON),
		"trojan://secret@1.2.3.4:443?type=tcp",
		"ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret")) + "@1.2.3.4:443",
		"hy2://secret@1.2.3.4:443?sni=example.com",
	} {
		server, err := ParseProxyURI(uri)
		if err != nil {
			t.Fatal(err)
		}
		outbound, err := server.SingBoxOutbound("test")
		if err != nil {
			t.Fatal(err)
		}
		if outbound["tag"] != "test" || outbound["type"] == "" {
			t.Fatalf("bad outbound: %#v", outbound)
		}
	}
}

func TestSingBoxOutboundSupportsHysteriaInsecureAndObfuscation(t *testing.T) {
	server, err := ParseProxyURI("hy2://secret@1.2.3.4:443?sni=example.com&insecure=1&obfs=salamander&obfs-password=cover")
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := server.SingBoxOutbound("test")
	if err != nil {
		t.Fatal(err)
	}
	tls := outbound["tls"].(map[string]any)
	if tls["insecure"] != true || outbound["obfs"].(map[string]any)["type"] != "salamander" {
		t.Fatalf("outbound=%#v", outbound)
	}
}

func TestSingBoxOutboundUsesNativeHysteriaTwoShape(t *testing.T) {
	server, err := ParseProxyURI("hy2://secret@1.2.3.4:443?sni=example.com")
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := server.SingBoxOutbound("test")
	if err != nil {
		t.Fatal(err)
	}
	if outbound["type"] != "hysteria2" || outbound["password"] != "secret" {
		t.Fatalf("outbound=%#v", outbound)
	}
}

func TestSingBoxOutboundSupportsLegacyShadowsocksCiphers(t *testing.T) {
	for _, method := range []string{"aes-256-cfb", "rc4-md5"} {
		server := ProxyServer{Protocol: "shadowsocks", Address: "1.2.3.4", Port: 443, Method: method, Password: "secret"}
		if _, err := server.SingBoxOutbound("test"); err != nil {
			t.Errorf("method %q: %v", method, err)
		}
	}
	for _, method := range []string{"aes-128-gcm", "chacha20-ietf-poly1305", "2022-blake3-aes-256-gcm"} {
		server := ProxyServer{Protocol: "shadowsocks", Address: "1.2.3.4", Port: 443, Method: method, Password: "secret"}
		if _, err := server.SingBoxOutbound("test"); err != nil {
			t.Errorf("method %q: %v", method, err)
		}
	}
}

func TestSubscriptionKeepsSingBoxCompatibleLegacyNodes(t *testing.T) {
	legacy := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-cfb:secret")) + "@1.2.3.4:443"
	valid := "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:secret")) + "@1.2.3.5:443"
	servers := ParseSubscription(legacy + "\n" + valid)
	if len(servers) != 2 {
		t.Fatalf("servers=%#v", servers)
	}
}

func TestSubscriptionDropsUnsupportedXHTTPNodes(t *testing.T) {
	servers := ParseSubscription("vless://" + testUUIDA + "@1.2.3.4:443?encryption=none&security=tls&type=xhttp")
	if len(servers) != 0 {
		t.Fatalf("servers=%#v", servers)
	}
}
