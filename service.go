package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

var ErrBusy = errors.New("a test is already in progress")

type siteCacheEntry struct {
	expires time.Time
	servers []ProxyServer
}

type Service struct {
	config     Config
	scraper    *Scraper
	tester     *Tester
	geoIP      *GeoIP
	store      *StateStore
	mu         sync.RWMutex
	working    []ProxyServer
	candidates map[string]ProxyServer
	siteCache  map[string]siteCacheEntry
	jobToken   chan struct{}
	runContext context.Context
}

func NewService(config Config) (*Service, error) {
	state, err := NewStateStore(config.StatePath).Load()
	if err != nil {
		return nil, err
	}
	s := &Service{config: config, scraper: NewScraper(config.FetchTimeout), tester: NewTester(config), geoIP: OpenGeoIP(config.GeoIPPath), store: NewStateStore(config.StatePath), candidates: make(map[string]ProxyServer), siteCache: make(map[string]siteCacheEntry), jobToken: make(chan struct{}, 1), runContext: context.Background()}
	s.working = append([]ProxyServer(nil), state.Working...)
	for _, server := range state.Candidates {
		s.candidates[server.ConnectionFingerprint()] = server
	}
	return s, nil
}

func (s *Service) Close() error { return s.geoIP.Close() }

func (s *Service) Start(ctx context.Context) {
	s.runContext = ctx
	s.TriggerUpdate(ctx)
	go func() {
		ticker := time.NewTicker(s.config.CacheInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.TriggerUpdate(ctx)
			}
		}
	}()
}

func (s *Service) TriggerUpdate(ctx context.Context) bool {
	if !s.tryBegin() {
		return false
	}
	go func() {
		err := s.update(ctx)
		s.end()
		if err != nil {
			slog.Error("update failed", "error", err)
			return
		}
		if s.config.GitPushEnabled {
			s.publishIntegrations(s.runContext, s.Cached(true))
		}
	}()
	return true
}

func (s *Service) update(ctx context.Context) error {
	slog.Info("starting scrape and test cycle", "sources", len(s.config.SubscriptionURLs))
	fresh := s.scraper.FetchAll(ctx, s.config.SubscriptionURLs)
	s.mu.RLock()
	previous := make([]ProxyServer, 0, len(s.candidates))
	for _, server := range s.candidates {
		previous = append(previous, server)
	}
	s.mu.RUnlock()
	candidates := mergeServers(fresh, previous)
	if len(candidates) > s.config.MaxCandidates {
		candidates = candidates[:s.config.MaxCandidates]
	}
	if len(candidates) == 0 {
		return fmt.Errorf("no valid proxy candidates")
	}
	results := s.tester.Test(ctx, candidates, s.config.LatencyTestURL, false)
	working, retained := s.processResults(results, s.config.MaxDelayMS)
	if len(retained) == 0 {
		return fmt.Errorf("test cycle produced no candidates")
	}
	s.mu.Lock()
	s.working = working
	s.candidates = retained
	s.siteCache = make(map[string]siteCacheEntry)
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	slog.Info("update complete", "working", len(working), "candidates", len(retained))
	return nil
}

func (s *Service) processResults(results []TestResult, maxDelay int) ([]ProxyServer, map[string]ProxyServer) {
	working := make([]ProxyServer, 0)
	retained := make(map[string]ProxyServer)
	for _, result := range results {
		server := result.Server
		if !result.Failed && result.Delay > 0 && result.Delay <= maxDelay {
			server.Delay = result.Delay
			server.FailCount = 0
			code, flag := s.geoIP.Country(server.Address)
			server.CountryCode = code
			server.Flag = flag
			server.Remark = fmt.Sprintf("%s %s %dms", flag, code, result.Delay)
			server.RawURI = server.ToURI()
			working = append(working, server)
			retained[server.ConnectionFingerprint()] = server
		} else {
			server.Delay = -1
			server.FailCount++
			if server.FailCount < s.config.MaxFailCount {
				retained[server.ConnectionFingerprint()] = server
			}
		}
	}
	sort.Slice(working, func(i, j int) bool { return working[i].Delay < working[j].Delay })
	return working, retained
}

func (s *Service) TestContent(ctx context.Context, content, target string, maxDelay, limit int) ([]ProxyServer, error) {
	servers := ParseSubscription(content)
	return s.testAdHoc(ctx, servers, target, maxDelay, len(servers))
}

func (s *Service) TestCustom(ctx context.Context, urls []string, content, target string, maxDelay, limit int) ([]ProxyServer, error) {
	servers := s.scraper.FetchAll(ctx, urls)
	servers = mergeServers(servers, ParseSubscription(content))
	return s.testAdHoc(ctx, servers, target, maxDelay, limit)
}

func (s *Service) testAdHoc(ctx context.Context, servers []ProxyServer, target string, maxDelay, limit int) ([]ProxyServer, error) {
	if len(servers) == 0 {
		return []ProxyServer{}, nil
	}
	if !s.tryBegin() {
		return nil, ErrBusy
	}
	defer s.end()
	if target == "" {
		target = s.config.LatencyTestURL
	}
	if maxDelay <= 0 {
		maxDelay = s.config.MaxDelayMS
	}
	if limit <= 0 {
		limit = 50
	}
	working, _ := s.processResults(s.tester.Test(ctx, servers, target, target != s.config.LatencyTestURL), maxDelay)
	if len(working) > limit {
		working = working[:limit]
	}
	return working, nil
}

func (s *Service) SiteSpecific(ctx context.Context, target string) ([]ProxyServer, error) {
	now := time.Now()
	s.mu.Lock()
	if cached, ok := s.siteCache[target]; ok && now.Before(cached.expires) {
		result := append([]ProxyServer(nil), cached.servers...)
		s.mu.Unlock()
		return result, nil
	}
	base := append([]ProxyServer(nil), s.working...)
	s.mu.Unlock()
	if len(base) == 0 {
		return nil, fmt.Errorf("cache is empty")
	}
	if !s.tryBegin() {
		return nil, ErrBusy
	}
	defer s.end()
	results := s.tester.Test(ctx, base, target, true)
	successful := make([]ProxyServer, 0)
	for _, result := range results {
		if !result.Failed {
			successful = append(successful, result.Server)
		}
	}
	s.mu.Lock()
	s.siteCache[target] = siteCacheEntry{expires: now.Add(s.config.SiteCacheTTL), servers: successful}
	s.mu.Unlock()
	return successful, nil
}

func (s *Service) Cached(all bool) []ProxyServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	servers := s.working
	if !all && len(servers) > 25 {
		servers = servers[:25]
	}
	return append([]ProxyServer(nil), servers...)
}
func (s *Service) Processing() bool { return len(s.jobToken) > 0 }
func (s *Service) tryBegin() bool {
	select {
	case s.jobToken <- struct{}{}:
		return true
	default:
		return false
	}
}
func (s *Service) end() { <-s.jobToken }
func (s *Service) saveLocked() error {
	candidates := make([]ProxyServer, 0, len(s.candidates))
	for _, server := range s.candidates {
		candidates = append(candidates, server)
	}
	return s.store.Save(PersistentState{Working: s.working, Candidates: candidates, UpdatedAt: time.Now()})
}
func mergeServers(groups ...[]ProxyServer) []ProxyServer {
	seen := make(map[string]ProxyServer)
	for _, group := range groups {
		for _, server := range group {
			seen[server.ConnectionFingerprint()] = server
		}
	}
	result := make([]ProxyServer, 0, len(seen))
	for _, server := range seen {
		result = append(result, server)
	}
	return result
}
