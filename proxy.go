package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type ProxyServer struct {
	Protocol     string `json:"protocol"`
	Remark       string `json:"remark"`
	Address      string `json:"address"`
	Port         int    `json:"port"`
	Delay        int    `json:"delay"`
	CountryCode  string `json:"country_code"`
	Flag         string `json:"flag"`
	FailCount    int    `json:"fail_count"`
	RawURI       string `json:"raw_uri"`
	ID           string `json:"vless_id,omitempty"`
	VMessID      string `json:"vmess_id,omitempty"`
	Encryption   string `json:"encryption,omitempty"`
	Security     string `json:"security,omitempty"`
	Network      string `json:"type,omitempty"`
	Host         string `json:"host,omitempty"`
	Path         string `json:"path,omitempty"`
	SNI          string `json:"sni,omitempty"`
	Flow         string `json:"flow,omitempty"`
	Fingerprint  string `json:"fp,omitempty"`
	PublicKey    string `json:"pbk,omitempty"`
	ShortID      string `json:"sid,omitempty"`
	AlterID      int    `json:"aid,omitempty"`
	Method       string `json:"method,omitempty"`
	Password     string `json:"password,omitempty"`
	Insecure     bool   `json:"insecure,omitempty"`
	Obfs         string `json:"obfs,omitempty"`
	ObfsPassword string `json:"obfs_password,omitempty"`
}

func ParseProxyURI(raw string) (ProxyServer, error) {
	raw = strings.TrimSpace(raw)
	var server ProxyServer
	var err error
	switch {
	case strings.HasPrefix(raw, "vless://"):
		server, err = parseVLESS(raw)
	case strings.HasPrefix(raw, "vmess://"):
		server, err = parseVMess(raw)
	case strings.HasPrefix(raw, "trojan://"):
		server, err = parseTrojan(raw)
	case strings.HasPrefix(raw, "ss://"):
		server, err = parseShadowsocks(raw)
	case strings.HasPrefix(raw, "hy2://"), strings.HasPrefix(raw, "hysteria2://"):
		server, err = parseHysteria2(raw)
	default:
		err = fmt.Errorf("unsupported proxy protocol")
	}
	if err != nil {
		return ProxyServer{}, err
	}
	if server.Address == "" || server.Port < 1 || server.Port > 65535 {
		return ProxyServer{}, fmt.Errorf("invalid proxy address or port")
	}
	server.RawURI = raw
	if server.CountryCode == "" {
		server.CountryCode = "UN"
		server.Flag = "🇺🇳"
	}
	return server, nil
}

func parseVLESS(raw string) (ProxyServer, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return ProxyServer{}, fmt.Errorf("invalid vless URI")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return ProxyServer{}, err
	}
	q := u.Query()
	network := defaultString(q.Get("type"), "tcp")
	if network == "http" || network == "h2" {
		return ProxyServer{}, fmt.Errorf("unsupported network %q", network)
	}
	return ProxyServer{Protocol: "vless", ID: u.User.Username(), Address: u.Hostname(), Port: port,
		Encryption: defaultString(q.Get("encryption"), "none"), Security: defaultString(q.Get("security"), "none"),
		Network: network, Host: q.Get("host"), Path: q.Get("path"), SNI: q.Get("sni"), Flow: q.Get("flow"),
		Fingerprint: q.Get("fp"), PublicKey: q.Get("pbk"), ShortID: q.Get("sid"), Remark: fragment(u)}, nil
}

func parseVMess(raw string) (ProxyServer, error) {
	decoded, err := decodeAnyBase64(strings.TrimPrefix(raw, "vmess://"))
	if err != nil {
		return ProxyServer{}, err
	}
	var data map[string]any
	if err := json.Unmarshal(decoded, &data); err != nil {
		return ProxyServer{}, err
	}
	port, err := anyInt(data["port"])
	if err != nil {
		return ProxyServer{}, err
	}
	aid, _ := anyInt(data["aid"])
	network := defaultString(anyString(data["net"]), "tcp")
	if network == "http" || network == "h2" {
		return ProxyServer{}, fmt.Errorf("unsupported network %q", network)
	}
	return ProxyServer{Protocol: "vmess", VMessID: anyString(data["id"]), Address: anyString(data["add"]), Port: port,
		Security: defaultString(anyString(data["scy"]), "auto"), Network: network, Host: anyString(data["host"]),
		Path: anyString(data["path"]), SNI: anyString(data["sni"]), Encryption: defaultString(anyString(data["tls"]), "none"),
		AlterID: aid, Remark: anyString(data["ps"])}, nil
}

func parseTrojan(raw string) (ProxyServer, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return ProxyServer{}, fmt.Errorf("invalid trojan URI")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return ProxyServer{}, err
	}
	q := u.Query()
	network := defaultString(q.Get("type"), "tcp")
	if network == "http" || network == "h2" {
		return ProxyServer{}, fmt.Errorf("unsupported network %q", network)
	}
	return ProxyServer{Protocol: "trojan", Password: u.User.Username(), Address: u.Hostname(), Port: port,
		Security: defaultString(q.Get("security"), "tls"), Network: network, Host: q.Get("host"), Path: q.Get("path"),
		SNI: q.Get("sni"), Flow: q.Get("flow"), Remark: fragment(u)}, nil
}

func parseShadowsocks(raw string) (ProxyServer, error) {
	payload := strings.TrimPrefix(raw, "ss://")
	mainPart := strings.SplitN(payload, "#", 2)[0]
	mainPart = strings.SplitN(mainPart, "?", 2)[0]
	if !strings.Contains(mainPart, "@") {
		decoded, err := decodeAnyBase64(mainPart)
		if err == nil {
			credentialsAndHost := string(decoded)
			at := strings.LastIndex(credentialsAndHost, "@")
			if at > 0 {
				credentials := strings.SplitN(credentialsAndHost[:at], ":", 2)
				host, portText, splitErr := net.SplitHostPort(credentialsAndHost[at+1:])
				port, portErr := strconv.Atoi(portText)
				if len(credentials) == 2 && splitErr == nil && portErr == nil {
					remarkValue := ""
					if parts := strings.SplitN(payload, "#", 2); len(parts) == 2 {
						remarkValue, _ = url.PathUnescape(parts[1])
					}
					return ProxyServer{Protocol: "shadowsocks", Method: credentials[0], Password: credentials[1], Address: host, Port: port, Remark: remarkValue}, nil
				}
			}
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ProxyServer{}, err
	}
	var userInfo string
	if u.User != nil {
		userInfo = u.User.String()
	}
	decoded, decodeErr := decodeAnyBase64(userInfo)
	if decodeErr != nil {
		// SIP002 also permits method:password directly in userinfo.
		decoded = []byte(userInfo)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return ProxyServer{}, fmt.Errorf("invalid shadowsocks credentials")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return ProxyServer{}, err
	}
	return ProxyServer{Protocol: "shadowsocks", Method: parts[0], Password: parts[1], Address: u.Hostname(), Port: port, Remark: fragment(u)}, nil
}

func parseHysteria2(raw string) (ProxyServer, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return ProxyServer{}, fmt.Errorf("invalid hysteria2 URI")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return ProxyServer{}, err
	}
	q := u.Query()
	return ProxyServer{Protocol: "hysteria2", Password: u.User.Username(), Address: u.Hostname(), Port: port,
		SNI: q.Get("sni"), Insecure: q.Get("insecure") == "1" || strings.EqualFold(q.Get("insecure"), "true"),
		Obfs: q.Get("obfs"), ObfsPassword: q.Get("obfs-password"), Remark: fragment(u)}, nil
}

func (s ProxyServer) ConnectionFingerprint() string {
	stable := []any{s.Protocol, s.Address, s.Port, s.ID, s.VMessID, s.Method, s.Password, s.Security, s.Network, s.SNI, s.Path, s.Host, s.Flow, s.PublicKey, s.ShortID, s.Obfs, s.ObfsPassword}
	data, _ := json.Marshal(stable)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s ProxyServer) ToURI() string {
	switch s.Protocol {
	case "vless":
		q := url.Values{"encryption": {defaultString(s.Encryption, "none")}, "security": {defaultString(s.Security, "none")}, "type": {defaultString(s.Network, "tcp")}}
		addOptional(q, "host", s.Host)
		addOptional(q, "path", s.Path)
		addOptional(q, "sni", s.SNI)
		addOptional(q, "flow", s.Flow)
		addOptional(q, "fp", s.Fingerprint)
		addOptional(q, "pbk", s.PublicKey)
		addOptional(q, "sid", s.ShortID)
		return "vless://" + url.User(s.ID).String() + "@" + hostPort(s.Address, s.Port) + "?" + q.Encode() + remark(s.Remark)
	case "vmess":
		data := map[string]any{"v": "2", "ps": s.Remark, "add": s.Address, "port": strconv.Itoa(s.Port), "id": s.VMessID, "aid": s.AlterID, "scy": defaultString(s.Security, "auto"), "net": defaultString(s.Network, "tcp"), "type": "none", "host": s.Host, "path": s.Path, "tls": defaultString(s.Encryption, "none"), "sni": s.SNI}
		raw, _ := json.Marshal(data)
		return "vmess://" + base64.StdEncoding.EncodeToString(raw)
	case "trojan":
		q := url.Values{"security": {defaultString(s.Security, "tls")}, "type": {defaultString(s.Network, "tcp")}}
		addOptional(q, "host", s.Host)
		addOptional(q, "path", s.Path)
		addOptional(q, "sni", s.SNI)
		addOptional(q, "flow", s.Flow)
		return "trojan://" + url.User(s.Password).String() + "@" + hostPort(s.Address, s.Port) + "?" + q.Encode() + remark(s.Remark)
	case "shadowsocks":
		credentials := base64.RawURLEncoding.EncodeToString([]byte(s.Method + ":" + s.Password))
		return "ss://" + credentials + "@" + hostPort(s.Address, s.Port) + remark(s.Remark)
	case "hysteria2":
		q := url.Values{}
		addOptional(q, "sni", s.SNI)
		addOptional(q, "obfs", s.Obfs)
		addOptional(q, "obfs-password", s.ObfsPassword)
		if s.Insecure {
			q.Set("insecure", "1")
		}
		return "hy2://" + url.User(s.Password).String() + "@" + hostPort(s.Address, s.Port) + "?" + q.Encode() + remark(s.Remark)
	default:
		return s.RawURI
	}
}

func (s ProxyServer) XrayOutbound(tag string) (map[string]any, error) {
	stream := map[string]any{"network": defaultString(s.Network, "tcp"), "security": defaultString(s.Security, "none")}
	if s.Protocol == "vmess" {
		stream["security"] = defaultString(s.Encryption, "none")
	}
	if s.Protocol == "trojan" {
		stream["security"] = "tls"
	}
	if s.Protocol == "hysteria2" {
		stream = map[string]any{"security": "tls"}
	}
	switch stream["network"] {
	case "ws", "websocket":
		ws := map[string]any{"path": defaultString(s.Path, "/")}
		if s.Host != "" {
			ws["host"] = s.Host
		}
		stream["wsSettings"] = ws
	case "grpc":
		grpc := map[string]any{"serviceName": strings.TrimPrefix(s.Path, "/")}
		if s.Host != "" {
			grpc["authority"] = s.Host
		}
		stream["grpcSettings"] = grpc
	case "xhttp", "splithttp":
		xhttp := map[string]any{"path": defaultString(s.Path, "/")}
		if s.Host != "" {
			xhttp["host"] = s.Host
		}
		stream["xhttpSettings"] = xhttp
	case "httpupgrade":
		upgrade := map[string]any{"path": defaultString(s.Path, "/")}
		if s.Host != "" {
			upgrade["host"] = s.Host
		}
		stream["httpupgradeSettings"] = upgrade
	}
	security, _ := stream["security"].(string)
	if security == "tls" || security == "reality" {
		settings := map[string]any{"serverName": firstNonEmpty(s.SNI, s.Host, s.Address)}
		if s.Fingerprint != "" {
			settings["fingerprint"] = s.Fingerprint
		}
		if security == "reality" {
			settings["publicKey"] = s.PublicKey
			settings["shortId"] = s.ShortID
			stream["realitySettings"] = settings
		} else {
			stream["tlsSettings"] = settings
		}
	}
	switch s.Protocol {
	case "vless":
		settings := map[string]any{"vnext": []any{map[string]any{
			"address": s.Address, "port": s.Port,
			"users": []any{map[string]any{"id": s.ID, "encryption": "none", "flow": s.Flow}},
		}}}
		return map[string]any{"tag": tag, "protocol": "vless", "settings": settings, "streamSettings": stream}, nil
	case "vmess":
		settings := map[string]any{"vnext": []any{map[string]any{
			"address": s.Address, "port": s.Port,
			"users": []any{map[string]any{"id": s.VMessID, "alterId": s.AlterID, "security": defaultString(s.Security, "auto")}},
		}}}
		return map[string]any{"tag": tag, "protocol": "vmess", "settings": settings, "streamSettings": stream}, nil
	case "trojan":
		return map[string]any{"tag": tag, "protocol": "trojan", "settings": map[string]any{"servers": []any{map[string]any{"address": s.Address, "port": s.Port, "password": s.Password}}}, "streamSettings": stream}, nil
	case "shadowsocks":
		if !supportedShadowsocksCipher(s.Method) {
			return nil, fmt.Errorf("shadowsocks cipher %q is not supported by the bundled Xray release", s.Method)
		}
		return map[string]any{"tag": tag, "protocol": "shadowsocks", "settings": map[string]any{"servers": []any{map[string]any{"address": s.Address, "port": s.Port, "method": s.Method, "password": s.Password}}}}, nil
	case "hysteria2":
		if s.Insecure {
			return nil, fmt.Errorf("hysteria2 node requires disabled TLS verification, which current Xray releases no longer support")
		}
		if s.Obfs != "" && s.Obfs != "none" {
			return nil, fmt.Errorf("hysteria2 obfuscation %q is not supported by the Xray converter", s.Obfs)
		}
		stream = map[string]any{
			"network":          "hysteria",
			"security":         "tls",
			"tlsSettings":      map[string]any{"serverName": firstNonEmpty(s.SNI, s.Address), "alpn": []string{"h3"}},
			"hysteriaSettings": map[string]any{"version": 2, "auth": s.Password},
		}
		settings := map[string]any{"version": 2, "address": s.Address, "port": s.Port}
		return map[string]any{"tag": tag, "protocol": "hysteria", "settings": settings, "streamSettings": stream}, nil
	default:
		return nil, fmt.Errorf("unsupported protocol %q", s.Protocol)
	}
}

func supportedShadowsocksCipher(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "aes-128-gcm", "aead_aes_128_gcm",
		"aes-256-gcm", "aead_aes_256_gcm",
		"chacha20-poly1305", "aead_chacha20_poly1305", "chacha20-ietf-poly1305",
		"xchacha20-poly1305", "aead_xchacha20_poly1305", "xchacha20-ietf-poly1305",
		"none", "plain",
		"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return true
	default:
		return false
	}
}

func ParseSubscription(content string) []ProxyServer {
	if servers := parseSubscriptionLines(content); len(servers) > 0 {
		return servers
	}
	decoded, err := decodeAnyBase64(strings.TrimSpace(content))
	if err != nil {
		return nil
	}
	return parseSubscriptionLines(string(decoded))
}

func parseSubscriptionLines(content string) []ProxyServer {
	seen := make(map[string]bool)
	result := make([]ProxyServer, 0)
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		server, err := ParseProxyURI(strings.TrimSpace(line))
		if err == nil {
			if _, err = server.XrayOutbound("validate"); err != nil {
				continue
			}
			fingerprint := server.ConnectionFingerprint()
			if !seen[fingerprint] {
				seen[fingerprint] = true
				result = append(result, server)
			}
		}
	}
	return result
}

func decodeAnyBase64(raw string) ([]byte, error) {
	raw = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(raw); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid base64")
}

func fragment(u *url.URL) string { value, _ := url.PathUnescape(u.Fragment); return value }
func remark(value string) string {
	if value == "" {
		return ""
	}
	return "#" + url.PathEscape(value)
}
func hostPort(host string, port int) string { return net.JoinHostPort(host, strconv.Itoa(port)) }
func addOptional(values url.Values, key, value string) {
	if value != "" {
		values.Set(key, value)
	}
}
func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func anyString(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}
func anyInt(value any) (int, error) {
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("invalid integer")
	}
}
