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
	siteTester *Tester
	geoIP      *GeoIP
	store      *StateStore
	redis      *RedisStore
	registry   *fileRegistry
	mu         sync.RWMutex
	jobs       sync.WaitGroup
	working    []ProxyServer
	candidates map[string]ProxyServer
	siteCache  map[string]siteCacheEntry
	jobToken   chan struct{}
	siteToken  chan struct{}
	runContext context.Context
}

func NewService(config Config) (*Service, error) {
	state, err := NewStateStore(config.StatePath).Load()
	if err != nil {
		return nil, err
	}
	// Auto-provision sing-box for standalone installs (no-op when a
	// system binary already exists).
	config.SingBoxPath = resolveSingBox(config.SingBoxPath)
	redisStore, err := NewRedisStore(context.Background(), config.RedisURL, config.RedisPrefix)
	if err != nil {
		return nil, err
	}
	var registry *fileRegistry
	if redisStore != nil {
		subscriptions, sites, err := redisStore.SeedAndLoad(context.Background(), config.SubscriptionURLs, config.Sites)
		if err != nil {
			_ = redisStore.Close()
			return nil, err
		}
		config.SubscriptionURLs = subscriptions
		config.Sites = sites
	} else {
		registry, err = loadFileRegistry(registryPath(), config.SubscriptionURLs, config.Sites)
		if err != nil {
			return nil, err
		}
		config.SubscriptionURLs = append([]string(nil), registry.Subscriptions...)
		config.Sites = append([]SiteConfig(nil), registry.Sites...)
	}
	siteTesterConfig := config
	siteTesterConfig.BasePort += config.BatchSize * config.MaxConcurrentBatches
	s := &Service{config: config, scraper: NewScraper(config.FetchTimeout), tester: NewTester(config), siteTester: NewTester(siteTesterConfig), geoIP: OpenGeoIP(config.GeoIPPath), store: NewStateStore(config.StatePath), redis: redisStore, registry: registry, candidates: make(map[string]ProxyServer), siteCache: make(map[string]siteCacheEntry), jobToken: make(chan struct{}, 1), siteToken: make(chan struct{}, 1), runContext: context.Background()}
	s.working = append([]ProxyServer(nil), state.Working...)
	for _, server := range state.Candidates {
		s.candidates[server.ConnectionFingerprint()] = server
	}
	return s, nil
}

func (s *Service) Close() error {
	s.jobs.Wait()
	return errors.Join(s.geoIP.Close(), s.redis.Close())
}

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
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		err := s.update(ctx)
		s.end()
		if err != nil {
			slog.Error("update failed", "error", err)
			return
		}
		s.warmConfiguredSites(ctx)
		if s.config.GitPushEnabled {
			s.publishIntegrations(s.runContext, s.Cached(true))
		}
	}()
	return true
}

func (s *Service) update(ctx context.Context) error {
	s.mu.RLock()
	sources := append([]string(nil), s.config.SubscriptionURLs...)
	previous := make([]ProxyServer, 0, len(s.candidates))
	for _, server := range s.candidates {
		previous = append(previous, server)
	}
	s.mu.RUnlock()
	slog.Info("starting scrape and test cycle", "sources", len(sources))
	fetch := s.scraper.FetchAllSummary(ctx, sources)
	slog.Info("subscription fetch complete", "successful_sources", fetch.Successful, "failed_sources", fetch.Failed, "fresh_candidates", len(fetch.Servers))
	if fetch.Failed > 0 || fetch.Attempted != len(sources) {
		return fmt.Errorf("subscription refresh incomplete: %d succeeded, %d failed, %d configured; preserved previous cache", fetch.Successful, fetch.Failed, len(sources))
	}
	fresh := fetch.Servers
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
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	slog.Info("update complete", "working", len(working), "candidates", len(retained), "sites", len(s.Sites()))
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
	return s.testAdHoc(ctx, servers, target, maxDelay, limit)
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
	if s.config.MaxCandidates > 0 && len(servers) > s.config.MaxCandidates {
		servers = servers[:s.config.MaxCandidates]
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
	return s.siteSpecific(ctx, target, true)
}

func (s *Service) siteSpecific(ctx context.Context, target string, acquireJob bool) ([]ProxyServer, error) {
	now := time.Now()
	s.mu.Lock()
	if cached, ok := s.siteCache[target]; ok && now.Before(cached.expires) {
		result := append([]ProxyServer(nil), cached.servers...)
		s.mu.Unlock()
		return result, nil
	}
	base := append([]ProxyServer(nil), s.working...)
	s.mu.Unlock()
	if cached, ok, err := s.redis.GetSiteCache(ctx, target); err == nil && ok {
		s.mu.Lock()
		s.siteCache[target] = siteCacheEntry{expires: now.Add(s.config.SiteCacheTTL), servers: append([]ProxyServer(nil), cached...)}
		s.mu.Unlock()
		return cached, nil
	} else if err != nil {
		slog.Warn("Redis site cache read failed", "url", target, "error", err)
	}
	if len(base) == 0 {
		return nil, fmt.Errorf("cache is empty")
	}
	if acquireJob && !s.tryBeginSite() {
		return nil, ErrBusy
	}
	if acquireJob {
		defer s.endSite()
	}
	results := s.siteTester.Test(ctx, base, target, true)
	successful := make([]ProxyServer, 0)
	for _, result := range results {
		if !result.Failed {
			successful = append(successful, result.Server)
		}
	}
	s.mu.Lock()
	s.siteCache[target] = siteCacheEntry{expires: now.Add(s.config.SiteCacheTTL), servers: successful}
	s.mu.Unlock()
	if err := s.redis.SetSiteCache(ctx, target, successful, s.config.SiteCacheTTL); err != nil {
		slog.Warn("Redis site cache write failed", "url", target, "error", err)
	}
	return successful, nil
}

func (s *Service) warmConfiguredSites(ctx context.Context) {
	for _, site := range s.Sites() {
		if _, err := s.siteSpecific(ctx, site.URL, true); err != nil && ctx.Err() == nil {
			slog.Warn("preloaded site check failed", "url", site.URL, "error", err)
		}
	}
}

func (s *Service) Subscriptions() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.config.SubscriptionURLs...)
}

func (s *Service) Sites() []SiteConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]SiteConfig(nil), s.config.Sites...)
}

func (s *Service) AddSubscriptions(ctx context.Context, urls []string) error {
	if s.redis != nil {
		if err := s.redis.AddSubscriptions(ctx, urls); err != nil {
			return err
		}
	} else if s.registry != nil {
		s.registry.addSubscriptions(urls)
	}
	s.mu.Lock()
	s.config.SubscriptionURLs = mergeStrings(s.config.SubscriptionURLs, urls)
	s.mu.Unlock()
	return nil
}

func (s *Service) RemoveSubscriptions(ctx context.Context, urls []string) error {
	if s.redis != nil {
		if err := s.redis.RemoveSubscriptions(ctx, urls); err != nil {
			return err
		}
	} else if s.registry != nil {
		s.registry.removeSubscriptions(urls)
	}
	removed := make(map[string]bool, len(urls))
	for _, value := range urls {
		removed[value] = true
	}
	s.mu.Lock()
	kept := s.config.SubscriptionURLs[:0]
	for _, value := range s.config.SubscriptionURLs {
		if !removed[value] {
			kept = append(kept, value)
		}
	}
	s.config.SubscriptionURLs = kept
	s.mu.Unlock()
	return nil
}

func (s *Service) PutSite(ctx context.Context, site SiteConfig) error {
	if s.redis != nil {
		if err := s.redis.PutSite(ctx, site); err != nil {
			return err
		}
	} else if s.registry != nil {
		s.registry.putSite(site)
	}
	s.mu.Lock()
	replaced := false
	for i := range s.config.Sites {
		if s.config.Sites[i].URL == site.URL {
			s.config.Sites[i] = site
			replaced = true
			break
		}
	}
	if !replaced {
		s.config.Sites = append(s.config.Sites, site)
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) RemoveSite(ctx context.Context, target string) error {
	if s.redis != nil {
		if err := s.redis.RemoveSite(ctx, target); err != nil {
			return err
		}
	} else if s.registry != nil {
		s.registry.removeSite(target)
	}
	s.mu.Lock()
	kept := s.config.Sites[:0]
	for _, site := range s.config.Sites {
		if site.URL != target {
			kept = append(kept, site)
		}
	}
	s.config.Sites = kept
	delete(s.siteCache, target)
	s.mu.Unlock()
	return nil
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
func (s *Service) tryBeginSite() bool {
	select {
	case s.siteToken <- struct{}{}:
		return true
	default:
		return false
	}
}
func (s *Service) endSite() { <-s.siteToken }
func (s *Service) saveLocked() error {
	candidates := make([]ProxyServer, 0, len(s.candidates))
	for _, server := range s.candidates {
		candidates = append(candidates, server)
	}
	return s.store.Save(PersistentState{Working: s.working, Candidates: candidates, UpdatedAt: time.Now()})
}
func mergeServers(groups ...[]ProxyServer) []ProxyServer {
	positions := make(map[string]int)
	result := make([]ProxyServer, 0)
	for _, group := range groups {
		for _, server := range group {
			fingerprint := server.ConnectionFingerprint()
			if index, ok := positions[fingerprint]; ok {
				result[index] = server
				continue
			}
			positions[fingerprint] = len(result)
			result = append(result, server)
		}
	}
	return result
}

func mergeStrings(groups ...[]string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, group := range groups {
		for _, value := range group {
			if !seen[value] {
				seen[value] = true
				result = append(result, value)
			}
		}
	}
	sort.Strings(result)
	return result
}
