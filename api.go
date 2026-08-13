package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type API struct {
	service *Service
	mux     *http.ServeMux
}
type serverResponse struct {
	Count   int           `json:"count"`
	Servers []ProxyServer `json:"servers"`
	Message string        `json:"message,omitempty"`
}
type customTestRequest struct {
	SubscriptionURLs []string `json:"subscription_urls"`
	CustomContent    string   `json:"custom_content"`
	TestURL          string   `json:"test_url"`
	MaxDelayMS       int      `json:"max_delay_ms"`
	Limit            int      `json:"limit"`
}

func NewAPI(service *Service) *API {
	a := &API{service: service, mux: http.NewServeMux()}
	a.mux.HandleFunc("/health", a.health)
	a.mux.HandleFunc("/openapi.json", a.openAPI)
	a.mux.HandleFunc("/swagger", a.swagger)
	a.mux.HandleFunc("/swagger/", a.swagger)
	a.mux.HandleFunc("/docs", a.swagger)
	a.mux.HandleFunc("/servers/live", a.live)
	a.mux.HandleFunc("/cache", a.cacheJSON)
	a.mux.HandleFunc("/cache/raw", a.cacheRaw)
	a.mux.HandleFunc("/cache/base64", a.cacheBase64)
	a.mux.HandleFunc("/cache/all/base64", a.cacheAllBase64)
	a.mux.HandleFunc("/subscription/site-specific", a.siteSpecific)
	a.mux.HandleFunc("/subscription/test", a.testContent)
	a.mux.HandleFunc("/subscription/test-custom", a.testCustom)
	return a
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	a.mux.ServeHTTP(w, r)
}
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) openAPI(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, openAPISpec)
}

func (a *API) swagger(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, swaggerHTML)
}
func (a *API) live(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	started := a.service.TriggerUpdate(a.service.runContext)
	servers := a.service.Cached(false)
	response := serverResponse{Count: len(servers), Servers: servers}
	if len(servers) == 0 {
		response.Message = "Update task enqueued. Please wait for the first cycle to complete."
	} else if !started {
		response.Message = "An update is already in progress."
	}
	writeJSON(w, http.StatusOK, response)
}
func (a *API) cacheJSON(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	servers := a.service.Cached(false)
	if !requireCache(w, servers) {
		return
	}
	writeJSON(w, http.StatusOK, serverResponse{Count: len(servers), Servers: servers})
}
func (a *API) cacheRaw(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	servers := a.service.Cached(false)
	if !requireCache(w, servers) {
		return
	}
	writeSubscription(w, servers, false)
}
func (a *API) cacheBase64(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	servers := filterCountries(a.service.Cached(false), r.URL.Query().Get("country"))
	if !requireCache(w, a.service.Cached(false)) {
		return
	}
	writeSubscription(w, servers, true)
}
func (a *API) cacheAllBase64(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	all := a.service.Cached(true)
	if !requireCache(w, all) {
		return
	}
	writeSubscription(w, filterCountries(all, r.URL.Query().Get("country")), true)
}
func (a *API) siteSpecific(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	target := r.URL.Query().Get("url")
	if !validTargetURL(target) {
		writeError(w, http.StatusBadRequest, "A valid http(s) url is required.")
		return
	}
	servers, err := a.service.SiteSpecific(r.Context(), target)
	if errors.Is(err, ErrBusy) {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	servers = filterCountries(servers, r.URL.Query().Get("country"))
	if len(servers) == 0 {
		writeError(w, http.StatusNotFound, "No servers could access "+target+".")
		return
	}
	writeSubscription(w, servers, true)
}
func (a *API) testContent(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		return
	}
	servers, err := a.service.TestContent(r.Context(), string(body), "", 0, 500)
	writeTestResponse(w, servers, err)
}
func (a *API) testCustom(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req customTestRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if req.Limit < 0 || req.Limit > 500 {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 500")
		return
	}
	if req.TestURL != "" && !validTargetURL(req.TestURL) {
		writeError(w, http.StatusBadRequest, "test_url must be a valid http(s) URL")
		return
	}
	servers, err := a.service.TestCustom(r.Context(), req.SubscriptionURLs, req.CustomContent, req.TestURL, req.MaxDelayMS, req.Limit)
	writeTestResponse(w, servers, err)
}
func writeTestResponse(w http.ResponseWriter, servers []ProxyServer, err error) {
	if errors.Is(err, ErrBusy) {
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if servers == nil {
		servers = []ProxyServer{}
	}
	writeJSON(w, http.StatusOK, serverResponse{Count: len(servers), Servers: servers})
}
func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method != allowed {
		w.Header().Set("Allow", allowed)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}
func requireCache(w http.ResponseWriter, servers []ProxyServer) bool {
	if len(servers) == 0 {
		writeError(w, http.StatusServiceUnavailable, "Cache not initialized. Please wait for the first update cycle.")
		return false
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]string{"detail": detail})
}
func writeSubscription(w http.ResponseWriter, servers []ProxyServer, encoded bool) {
	lines := make([]string, 0, len(servers))
	for _, server := range servers {
		lines = append(lines, server.RawURI)
	}
	content := strings.Join(lines, "\n")
	if encoded {
		content = base64.StdEncoding.EncodeToString([]byte(content))
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, content)
}
func filterCountries(servers []ProxyServer, raw string) []ProxyServer {
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
func validTargetURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
