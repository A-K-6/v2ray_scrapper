package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Scraper struct {
	client *http.Client
}

type FetchSummary struct {
	Servers    []ProxyServer
	Attempted  int
	Successful int
	Failed     int
}

func NewScraper(timeout time.Duration) *Scraper {
	return &Scraper{client: &http.Client{Timeout: timeout}}
}

func (s *Scraper) FetchAll(ctx context.Context, urls []string) []ProxyServer {
	return s.FetchAllSummary(ctx, urls).Servers
}

func (s *Scraper) FetchAllSummary(ctx context.Context, urls []string) FetchSummary {
	if len(urls) == 0 {
		return FetchSummary{}
	}
	type fetchJob struct {
		index  int
		source string
	}
	type fetchResult struct {
		index   int
		source  string
		servers []ProxyServer
		err     error
	}
	var wg sync.WaitGroup
	jobs := make(chan fetchJob)
	results := make(chan fetchResult, len(urls))
	for range min(20, len(urls)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				var servers []ProxyServer
				var err error
				for attempt := 1; attempt <= 2; attempt++ {
					servers, err = s.Fetch(ctx, job.source)
					if err == nil && len(servers) > 0 {
						break
					}
					if err == nil {
						err = fmt.Errorf("source contained no sing-box-compatible configurations")
					}
					if attempt < 2 {
						select {
						case <-ctx.Done():
							err = ctx.Err()
						case <-time.After(500 * time.Millisecond):
						}
					}
				}
				if err != nil {
					err = fmt.Errorf("%s", strings.ReplaceAll(err.Error(), job.source, "subscription source"))
				}
				results <- fetchResult{index: job.index, source: sourceLabel(job.source), servers: servers, err: err}
			}
		}()
	}
sendJobs:
	for index, source := range urls {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		select {
		case jobs <- fetchJob{index: index, source: source}:
		case <-ctx.Done():
			break sendJobs
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	bySource := make([][]ProxyServer, len(urls))
	summary := FetchSummary{}
	for result := range results {
		summary.Attempted++
		if result.err != nil {
			summary.Failed++
			slog.Warn("subscription source failed", "source", result.source, "error", result.err)
			continue
		}
		summary.Successful++
		slog.Info("subscription source loaded", "source", result.source, "candidates", len(result.servers))
		bySource[result.index] = result.servers
	}
	seen := make(map[string]bool)
	merged := make([]ProxyServer, 0)
	for offset := 0; ; offset++ {
		added := false
		for _, servers := range bySource {
			if offset >= len(servers) {
				continue
			}
			added = true
			server := servers[offset]
			fingerprint := server.ConnectionFingerprint()
			if !seen[fingerprint] {
				seen[fingerprint] = true
				merged = append(merged, server)
			}
		}
		if !added {
			break
		}
	}
	summary.Servers = merged
	return summary
}

func sourceLabel(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "invalid-url"
}

func (s *Scraper) Fetch(ctx context.Context, source string) ([]ProxyServer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", source, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		return nil, fmt.Errorf("%s returned HTML", source)
	}
	return ParseSubscription(string(body)), nil
}
