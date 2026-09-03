package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aeen/v2ray-scrapper/internal/config"
	svc "github.com/aeen/v2ray-scrapper/internal/service"
)

func TestOpenAPIDocumentsEveryPublicOperation(t *testing.T) {
	var spec struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &spec); err != nil {
		t.Fatalf("OpenAPI is invalid JSON: %v", err)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI version=%q", spec.OpenAPI)
	}
	for _, path := range []string{"/health", "/servers/live", "/cache", "/cache/raw", "/cache/base64", "/cache/all/base64", "/subscription/site-specific", "/subscription/test", "/subscription/test-custom", "/subscriptions", "/sites"} {
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("missing documented path %s", path)
		}
	}
}

func TestSwaggerEndpoints(t *testing.T) {
	isolatedEnv(t)
	service, err := svc.NewService(config.Config{StatePath: filepath.Join(t.TempDir(), "state.json"), GeoIPPath: "missing.mmdb"})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	api := NewAPI(service)

	for _, test := range []struct {
		path, contentType, bodyFragment string
	}{
		{"/openapi.json", "application/json", `"openapi": "3.1.0"`},
		{"/swagger", "text/html", "SwaggerUIBundle"},
		{"/docs", "text/html", "SwaggerUIBundle"},
	} {
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("%s status=%d", test.path, response.Code)
		}
		if !strings.HasPrefix(response.Header().Get("Content-Type"), test.contentType) {
			t.Errorf("%s content-type=%q", test.path, response.Header().Get("Content-Type"))
		}
		if !strings.Contains(response.Body.String(), test.bodyFragment) {
			t.Errorf("%s missing %q", test.path, test.bodyFragment)
		}
	}
}
