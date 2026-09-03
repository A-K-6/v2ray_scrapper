package proxy

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

func (s ProxyServer) SingBoxOutbound(tag string) (map[string]any, error) {
	base := map[string]any{"type": s.Protocol, "tag": tag, "server": s.Address, "server_port": s.Port}
	transport, err := s.singBoxTransport()
	if err != nil {
		return nil, err
	}
	if transport != nil {
		base["transport"] = transport
	}
	switch s.Protocol {
	case "vless":
		if !validUUID(s.ID) {
			return nil, fmt.Errorf("invalid VLESS UUID")
		}
		if s.Flow != "" && s.Flow != "xtls-rprx-vision" {
			return nil, fmt.Errorf("unsupported VLESS flow %q", s.Flow)
		}
		if s.Security == "reality" && !validRealityParameters(s.PublicKey, s.ShortID) {
			return nil, fmt.Errorf("invalid Reality public key or short ID")
		}
		base["uuid"] = s.ID
		if s.Flow != "" {
			base["flow"] = s.Flow
		}
		if tls := s.singBoxTLS(defaultString(s.Security, "none")); tls != nil {
			base["tls"] = tls
		}
	case "vmess":
		if !validUUID(s.VMessID) {
			return nil, fmt.Errorf("invalid VMess UUID")
		}
		base["uuid"] = s.VMessID
		base["security"] = defaultString(s.Security, "auto")
		base["alter_id"] = s.AlterID
		if tls := s.singBoxTLS(defaultString(s.Encryption, "none")); tls != nil {
			base["tls"] = tls
		}
	case "trojan":
		base["password"] = s.Password
		base["tls"] = s.singBoxTLS("tls")
	case "shadowsocks":
		if !supportedShadowsocksCipher(s.Method) {
			return nil, fmt.Errorf("shadowsocks cipher %q is not supported by the bundled sing-box release", s.Method)
		}
		base["method"] = strings.ToLower(strings.TrimSpace(s.Method))
		base["password"] = s.Password
	case "hysteria2":
		base["password"] = s.Password
		base["tls"] = map[string]any{"enabled": true, "server_name": firstNonEmpty(s.SNI, s.Address), "insecure": s.Insecure}
		if s.Obfs != "" && s.Obfs != "none" {
			if s.Obfs != "salamander" {
				return nil, fmt.Errorf("hysteria2 obfuscation %q is not supported by sing-box 1.13", s.Obfs)
			}
			base["obfs"] = map[string]any{"type": s.Obfs, "password": s.ObfsPassword}
		}
	default:
		return nil, fmt.Errorf("unsupported protocol %q", s.Protocol)
	}
	return base, nil
}

func (s ProxyServer) singBoxTransport() (map[string]any, error) {
	switch strings.ToLower(defaultString(s.Network, "tcp")) {
	case "tcp":
		return nil, nil
	case "ws", "websocket":
		transport := map[string]any{"type": "ws", "path": defaultString(s.Path, "/")}
		if s.Host != "" {
			transport["headers"] = map[string]string{"Host": s.Host}
		}
		return transport, nil
	case "grpc":
		return map[string]any{"type": "grpc", "service_name": strings.TrimPrefix(s.Path, "/")}, nil
	case "http", "h2":
		transport := map[string]any{"type": "http", "path": defaultString(s.Path, "/")}
		if s.Host != "" {
			transport["host"] = []string{s.Host}
		}
		return transport, nil
	case "httpupgrade":
		transport := map[string]any{"type": "httpupgrade", "path": defaultString(s.Path, "/")}
		if s.Host != "" {
			transport["host"] = s.Host
		}
		return transport, nil
	case "xhttp", "splithttp":
		return nil, fmt.Errorf("network %q is not supported by sing-box", s.Network)
	default:
		return nil, fmt.Errorf("network %q is not supported by sing-box", s.Network)
	}
}

func (s ProxyServer) singBoxTLS(security string) map[string]any {
	if security != "tls" && security != "reality" {
		return nil
	}
	tls := map[string]any{"enabled": true, "server_name": firstNonEmpty(s.SNI, s.Host, s.Address)}
	if s.Fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": s.Fingerprint}
	}
	if security == "reality" {
		if s.Fingerprint == "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": "chrome"}
		}
		tls["reality"] = map[string]any{"enabled": true, "public_key": s.PublicKey, "short_id": s.ShortID}
	}
	return tls
}

func supportedShadowsocksCipher(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
		"chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
		"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
		"aes-128-cfb", "aes-192-cfb", "aes-256-cfb", "rc4-md5", "chacha20-ietf", "xchacha20", "none",
		"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305":
		return true
	default:
		return false
	}
}

func validUUID(value string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(value), "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.DecodeString(compact)
	return err == nil
}

func validRealityParameters(publicKey, shortID string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(decoded) != 32 {
		return false
	}
	if shortID == "" {
		return true
	}
	if len(shortID) > 16 || len(shortID)%2 != 0 {
		return false
	}
	_, err = hex.DecodeString(shortID)
	return err == nil
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
			if _, err = server.SingBoxOutbound("validate"); err != nil {
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

// FilterCountries returns servers whose CountryCode is in the
// comma-separated allowlist (case-insensitive). Empty filter keeps all.
func FilterCountries(servers []ProxyServer, raw string) []ProxyServer {
	if strings.TrimSpace(raw) == "" {
		return servers
	}
	allowed := make(map[string]bool)
	for _, code := range strings.Split(raw, ",") {
		allowed[strings.ToUpper(strings.TrimSpace(code))] = true
	}
	filtered := make([]ProxyServer, 0, len(servers))
	for _, server := range servers {
		if allowed[strings.ToUpper(server.CountryCode)] {
			filtered = append(filtered, server)
		}
	}
	return filtered
}

// ValidTargetURL reports whether raw is an absolute http(s) URL.
func ValidTargetURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
