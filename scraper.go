package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Scraper struct {
	client *http.Client
}

func NewScraper(timeout time.Duration) *Scraper {
	return &Scraper{client: &http.Client{Timeout: timeout}}
}

func (s *Scraper) FetchAll(ctx context.Context, urls []string) []ProxyServer {
	type fetchResult struct {
		index   int
		servers []ProxyServer
	}
	var wg sync.WaitGroup
	results := make(chan fetchResult, len(urls))
	for index, source := range urls {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			servers, err := s.Fetch(ctx, source)
			if err == nil {
				results <- fetchResult{index: index, servers: servers}
			}
		}(index)
	}
	wg.Wait()
	close(results)
	bySource := make([][]ProxyServer, len(urls))
	for result := range results {
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
	return merged
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
