package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type OutboundConfig struct {
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings"`
	StreamSettings map[string]interface{} `json:"streamSettings,omitempty"`
}

// ConnectionInfo extracts only fields that define a unique connection.
// This is used for fingerprinting to avoid testing the same server multiple times.
type ConnectionInfo struct {
	Protocol string
	Address  string
	Port     int
	Auth     string // ID, Password, etc.
	Network  string // tcp, ws, grpc, etc.
	Security string // tls, reality, none
	SNI      string
	Path     string
	Host     string
}

func (c *OutboundConfig) GetConnectionInfo() ConnectionInfo {
	info := ConnectionInfo{
		Protocol: c.Protocol,
	}

	// Extract from Settings
	if c.Protocol == "vmess" || c.Protocol == "vless" {
		if vnext, ok := c.Settings["vnext"].([]map[string]interface{}); ok && len(vnext) > 0 {
			info.Address, _ = vnext[0]["address"].(string)
			info.Port, _ = vnext[0]["port"].(int)
			if users, ok := vnext[0]["users"].([]map[string]interface{}); ok && len(users) > 0 {
				info.Auth, _ = users[0]["id"].(string)
			}
		}
	} else if c.Protocol == "trojan" || c.Protocol == "shadowsocks" || c.Protocol == "hysteria2" {
		if servers, ok := c.Settings["servers"].([]map[string]interface{}); ok && len(servers) > 0 {
			info.Address, _ = servers[0]["address"].(string)
			info.Port, _ = servers[0]["port"].(int)
			if c.Protocol == "shadowsocks" {
				method, _ := servers[0]["method"].(string)
				pass, _ := servers[0]["password"].(string)
				info.Auth = method + ":" + pass
			} else {
				info.Auth, _ = servers[0]["password"].(string)
			}
		}
	}

	// Extract from StreamSettings
	if c.StreamSettings != nil {
		info.Network, _ = c.StreamSettings["network"].(string)
		info.Security, _ = c.StreamSettings["security"].(string)

		if ws, ok := c.StreamSettings["wsSettings"].(map[string]interface{}); ok {
			info.Path, _ = ws["path"].(string)
			info.Host, _ = ws["host"].(string)
		}

		if tls, ok := c.StreamSettings["tlsSettings"].(map[string]interface{}); ok {
			info.SNI, _ = tls["serverName"].(string)
		} else if reality, ok := c.StreamSettings["realitySettings"].(map[string]interface{}); ok {
			info.SNI, _ = reality["serverName"].(string)
		}
	}

	return info
}

func (c *OutboundConfig) Fingerprint() string {
	info := c.GetConnectionInfo()
	data, _ := json.Marshal(info)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func parseRawURI(uri string) (*OutboundConfig, error) {
	if strings.HasPrefix(uri, "vmess://") {
		return parseVmess(uri)
	} else if strings.HasPrefix(uri, "vless://") {
		return parseVless(uri)
	} else if strings.HasPrefix(uri, "trojan://") {
		return parseTrojan(uri)
	} else if strings.HasPrefix(uri, "ss://") {
		return parseShadowsocks(uri)
	} else if strings.HasPrefix(uri, "hy2://") || strings.HasPrefix(uri, "hysteria2://") {
		return parseHysteria2(uri)
	}
	return nil, fmt.Errorf("unsupported protocol: %s", uri)
}

func parseVmess(uri string) (*OutboundConfig, error) {
	b64Data := strings.TrimPrefix(uri, "vmess://")
	if len(b64Data)%4 != 0 {
		b64Data += strings.Repeat("=", 4-len(b64Data)%4)
	}
	decoded, err := base64.StdEncoding.DecodeString(b64Data)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil, err
	}

	address, _ := data["add"].(string)
	portStr, _ := data["port"].(string)
	if portStr == "" {
		if p, ok := data["port"].(float64); ok {
			portStr = strconv.Itoa(int(p))
		}
	}
	port, _ := strconv.Atoi(portStr)
	uuid, _ := data["id"].(string)
	aid, _ := data["aid"].(float64)
	netType, _ := data["net"].(string)
	tls, _ := data["tls"].(string)
	path, _ := data["path"].(string)
	host, _ := data["host"].(string)
	sni, _ := data["sni"].(string)

	if netType == "http" || netType == "h2" {
		return nil, fmt.Errorf("unsupported network type: %s", netType)
	}
	if netType == "" {
		netType = "tcp"
	}

	vnext := []map[string]interface{}{
		{
			"address": address,
			"port":    port,
			"users": []map[string]interface{}{
				{
					"id":       uuid,
					"alterId":  int(aid),
					"security": "auto",
				},
			},
		},
	}

	streamSettings := map[string]interface{}{
		"network":  netType,
		"security": tls,
	}

	if tls == "" || tls == "none" {
		streamSettings["security"] = "none"
	}

	if netType == "ws" {
		wsSettings := map[string]interface{}{
			"path": path,
		}
		if host != "" {
			wsSettings["host"] = host
		}
		streamSettings["wsSettings"] = wsSettings
	}

	if streamSettings["security"] == "tls" {
		tlsSettings := map[string]interface{}{}
		if sni != "" {
			tlsSettings["serverName"] = sni
		} else if host != "" {
			tlsSettings["serverName"] = host
		} else {
			tlsSettings["serverName"] = address
		}
		streamSettings["tlsSettings"] = tlsSettings
	}

	return &OutboundConfig{
		Protocol: "vmess",
		Settings: map[string]interface{}{
			"vnext": vnext,
		},
		StreamSettings: streamSettings,
	}, nil
}

func parseVless(uri string) (*OutboundConfig, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	uuid := u.User.Username()
	address := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	q := u.Query()

	netType := q.Get("type")
	if netType == "http" || netType == "h2" {
		return nil, fmt.Errorf("unsupported network type: %s", netType)
	}
	if netType == "" {
		netType = "tcp"
	}
	security := q.Get("security")
	if security == "" {
		security = "none"
	}
	flow := q.Get("flow")
	sni := q.Get("sni")
	fp := q.Get("fp")
	pbk := q.Get("pbk")
	sid := q.Get("sid")
	path := q.Get("path")
	host := q.Get("host")

	vnext := []map[string]interface{}{
		{
			"address": address,
			"port":    port,
			"users": []map[string]interface{}{
				{
					"id":         uuid,
					"encryption": "none",
					"flow":       flow,
				},
			},
		},
	}

	streamSettings := map[string]interface{}{
		"network":  netType,
		"security": security,
	}

	if netType == "ws" {
		wsSettings := map[string]interface{}{
			"path": path,
		}
		if host != "" {
			wsSettings["host"] = host
		}
		streamSettings["wsSettings"] = wsSettings
	}

	if security == "tls" || security == "reality" {
		tlsSettings := map[string]interface{}{
			"fingerprint": fp,
		}
		if sni != "" {
			tlsSettings["serverName"] = sni
		} else if host != "" {
			tlsSettings["serverName"] = host
		} else {
			tlsSettings["serverName"] = address
		}

		if security == "reality" {
			tlsSettings["publicKey"] = pbk
			tlsSettings["shortId"] = sid
			streamSettings["realitySettings"] = tlsSettings
		} else {
			streamSettings["tlsSettings"] = tlsSettings
		}
	}

	return &OutboundConfig{
		Protocol: "vless",
		Settings: map[string]interface{}{
			"vnext": vnext,
		},
		StreamSettings: streamSettings,
	}, nil
}

func parseTrojan(uri string) (*OutboundConfig, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	password := u.User.Username()
	address := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	q := u.Query()

	sni := q.Get("sni")
	netType := q.Get("type")
	if netType == "http" || netType == "h2" {
		return nil, fmt.Errorf("unsupported network type: %s", netType)
	}
	if netType == "" {
		netType = "tcp"
	}
	path := q.Get("path")
	host := q.Get("host")

	servers := []map[string]interface{}{
		{
			"address":  address,
			"port":     port,
			"password": password,
		},
	}

	streamSettings := map[string]interface{}{
		"network":  netType,
		"security": "tls",
		"tlsSettings": map[string]interface{}{
			"serverName": sni,
		},
	}
	if sni == "" {
		if host != "" {
			streamSettings["tlsSettings"].(map[string]interface{})["serverName"] = host
		} else {
			streamSettings["tlsSettings"].(map[string]interface{})["serverName"] = address
		}
	}

	if netType == "ws" {
		wsSettings := map[string]interface{}{
			"path": path,
		}
		if host != "" {
			wsSettings["host"] = host
		}
		streamSettings["wsSettings"] = wsSettings
	}

	return &OutboundConfig{
		Protocol: "trojan",
		Settings: map[string]interface{}{
			"servers": servers,
		},
		StreamSettings: streamSettings,
	}, nil
}

func parseShadowsocks(uri string) (*OutboundConfig, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	userInfo := u.User.String()
	decoded, err := base64.RawURLEncoding.DecodeString(userInfo)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(userInfo)
		if err != nil {
			return nil, err
		}
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid shadowsocks userinfo")
	}

	method := parts[0]
	password := parts[1]
	address := u.Hostname()
	port, _ := strconv.Atoi(u.Port())

	servers := []map[string]interface{}{
		{
			"address":  address,
			"port":     port,
			"method":   method,
			"password": password,
		},
	}

	return &OutboundConfig{
		Protocol: "shadowsocks",
		Settings: map[string]interface{}{
			"servers": servers,
		},
	}, nil
}

func parseHysteria2(uri string) (*OutboundConfig, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	password := u.User.Username()
	address := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	q := u.Query()

	sni := q.Get("sni")
	insecure := q.Get("insecure") == "1"
	obfs := q.Get("obfs")
	obfsPassword := q.Get("obfs-password")

	serverInfo := map[string]interface{}{
		"address":  address,
		"port":     port,
		"password": password,
	}

	if obfs != "" && obfs != "none" {
		serverInfo["obfs"] = map[string]interface{}{
			"type":     obfs,
			"password": obfsPassword,
		}
	}

	streamSettings := map[string]interface{}{
		"security": "tls",
		"tlsSettings": map[string]interface{}{
			"serverName":    sni,
			"allowInsecure": insecure,
		},
	}
	if sni == "" {
		streamSettings["tlsSettings"].(map[string]interface{})["serverName"] = address
	}

	return &OutboundConfig{
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"servers": []map[string]interface{}{serverInfo},
		},
		StreamSettings: streamSettings,
	}, nil
}
