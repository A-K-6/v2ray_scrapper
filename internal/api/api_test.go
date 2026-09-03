package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aeen/v2ray-scrapper/internal/config"
	"github.com/aeen/v2ray-scrapper/internal/proxy"
	svc "github.com/aeen/v2ray-scrapper/internal/service"
	"github.com/aeen/v2ray-scrapper/internal/store"
	"github.com/alicebob/miniredis/v2"
)

// isolatedEnv keeps file-registry and sing-box resolution off the real user
// directories during tests.
func isolatedEnv(t *testing.T) {
	t.Helper()
	t.Setenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD", "1")
	t.Setenv("REDIS_URL", "")
	t.Setenv("V2RAYS_REGISTRY_PATH", filepath.Join(t.TempDir(), "registry.json"))
}

func seedWorking(t *testing.T, statePath string) {
	t.Helper()
	seed := []proxy.ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "uuid", CountryCode: "US", RawURI: "vless://uuid@1.2.3.4:443"}}
	if err := store.NewStateStore(statePath).Save(store.PersistentState{Working: seed, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestCachedEndpoints(t *testing.T) {
	isolatedEnv(t)
	statePath := filepath.Join(t.TempDir(), "state.json")
	seedWorking(t, statePath)
	cfg := config.Config{StatePath: statePath, GeoIPPath: "missing.mmdb", SiteCacheTTL: 24}
	service, err := svc.NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
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
	if string(decoded) != "vless://uuid@1.2.3.4:443" {
		t.Fatalf("body=%q", decoded)
	}
}

func TestManagementRoutesPersistSubscriptionsAndSitesInRedis(t *testing.T) {
	t.Setenv("V2RAYS_SKIP_SINGBOX_DOWNLOAD", "1")
	redisServer := miniredis.RunT(t)
	statePath := filepath.Join(t.TempDir(), "state.json")
	cfg := config.Config{
		StatePath: statePath, GeoIPPath: "missing.mmdb", RedisURL: "redis://" + redisServer.Addr(),
		RedisPrefix: "test", ManagementToken: "secret", SubscriptionURLs: []string{"https://seed.example/sub"},
		Sites: []config.SiteConfig{{URL: "https://seed.example", Filename: "seed.txt"}},
	}
	service, err := svc.NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPI(service)

	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response
	}
	if response := call(http.MethodPost, "/subscriptions", `{"urls":["https://added.example/sub"]}`); response.Code != http.StatusOK {
		t.Fatalf("add subscription: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPost, "/sites", `{"url":"https://video.example"}`); response.Code != http.StatusCreated {
		t.Fatalf("add site: status=%d body=%s", response.Code, response.Body.String())
	}
	_ = service.Close()

	restarted, err := svc.NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if got := restarted.Subscriptions(); len(got) != 2 || got[0] != "https://added.example/sub" {
		t.Fatalf("subscriptions=%v", got)
	}
	if got := restarted.Sites(); len(got) != 2 || got[1].URL != "https://video.example" {
		t.Fatalf("sites=%#v", got)
	}
}

func TestCacheUnavailable(t *testing.T) {
	isolatedEnv(t)
	cfg := config.Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb", SiteCacheTTL: 24}
	service, err := svc.NewService(cfg)
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
	isolatedEnv(t)
	cfg := config.Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb"}
	service, err := svc.NewService(cfg)
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
	isolatedEnv(t)
	statePath := filepath.Join(t.TempDir(), "state.json")
	seedWorking(t, statePath)
	cfg := config.Config{StatePath: statePath, GeoIPPath: "missing.mmdb", SiteCacheTTL: time.Hour}
	service, err := svc.NewService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
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
		{"subscriptions require management configuration", http.MethodGet, "/subscriptions", "", "", http.StatusServiceUnavailable},
		{"sites require management configuration", http.MethodGet, "/sites", "", "", http.StatusServiceUnavailable},
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
	isolatedEnv(t)
	service, err := svc.NewService(config.Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb"})
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
