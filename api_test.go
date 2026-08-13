package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCachedEndpoints(t *testing.T) {
	config := Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb", SiteCacheTTL: 24}
	service, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.working = []ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "uuid", CountryCode: "US", RawURI: "vless://uuid@1.2.3.4:443"}}
	api := NewAPI(service)
	request := httptest.NewRequest(http.MethodGet, "/cache/base64?country=US", nil)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(response.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != service.working[0].RawURI {
		t.Fatalf("body=%q", decoded)
	}
}

func TestCacheUnavailable(t *testing.T) {
	config := Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb", SiteCacheTTL: 24}
	service, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	response := httptest.NewRecorder()
	NewAPI(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/cache", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestHealth(t *testing.T) {
	config := Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb"}
	service, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	response := httptest.NewRecorder()
	NewAPI(service).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEveryAPIRouteHasAnIsolatedBehaviorCheck(t *testing.T) {
	config := Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb", SiteCacheTTL: time.Hour}
	service, err := NewService(config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.working = []ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "uuid", CountryCode: "US", RawURI: "vless://uuid@1.2.3.4:443"}}
	api := NewAPI(service)

	tests := []struct {
		name, method, target, body, contentType string
		wantStatus                              int
	}{
		{"health", http.MethodGet, "/health", "", "", http.StatusOK},
		{"live", http.MethodGet, "/servers/live", "", "", http.StatusOK},
		{"cache json", http.MethodGet, "/cache", "", "", http.StatusOK},
		{"cache raw", http.MethodGet, "/cache/raw", "", "", http.StatusOK},
		{"cache base64", http.MethodGet, "/cache/base64", "", "", http.StatusOK},
		{"cache all", http.MethodGet, "/cache/all/base64", "", "", http.StatusOK},
		{"site validation", http.MethodGet, "/subscription/site-specific?url=not-a-url", "", "", http.StatusBadRequest},
		{"test empty", http.MethodPost, "/subscription/test", "", "text/plain", http.StatusOK},
		{"test custom empty", http.MethodPost, "/subscription/test-custom", `{}`, "application/json", http.StatusOK},
		{"openapi", http.MethodGet, "/openapi.json", "", "", http.StatusOK},
		{"swagger", http.MethodGet, "/swagger", "", "", http.StatusOK},
		{"docs alias", http.MethodGet, "/docs", "", "", http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCustomRequestRejectsUnknownFieldsAndInvalidLimits(t *testing.T) {
	service, err := NewService(Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	api := NewAPI(service)
	for _, body := range []string{`{"unknown":true}`, `{"limit":501}`, `{"test_url":"file:///etc/passwd"}`} {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/subscription/test-custom", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Errorf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
		var problem map[string]string
		if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil || problem["detail"] == "" {
			t.Errorf("body=%s is not an error response", response.Body.String())
		}
	}
}
