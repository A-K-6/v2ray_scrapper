package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ScrapeResult struct {
	URL  string   `json:"url"`
	URIs []string `json:"uris"`
	Err  string   `json:"err,omitempty"`
}

func scrapeAll(urls []string, timeout time.Duration) []ScrapeResult {
	var wg sync.WaitGroup
	results := make([]ScrapeResult, len(urls))

	client := &http.Client{
		Timeout: timeout,
	}

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, urlStr string) {
			defer wg.Done()
			log.Printf("[Scraper] Fetching %s", urlStr)
			uris, err := fetchSingleURL(client, urlStr)
			res := ScrapeResult{URL: urlStr}
			if err != nil {
				log.Printf("[Scraper] Error fetching %s: %v", urlStr, err)
				res.Err = err.Error()
			} else {
				log.Printf("[Scraper] Found %d URIs from %s", len(uris), urlStr)
				res.URIs = uris
			}
			results[idx] = res
		}(i, u)
	}

	wg.Wait()
	return results
}

func decodeBase64(input string) (string, error) {
	input = strings.TrimSpace(input)
	input = strings.ReplaceAll(input, "\n", "")
	input = strings.ReplaceAll(input, "\r", "")
	input = strings.ReplaceAll(input, " ", "")

	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}

	for _, enc := range encodings {
		decoded, err := enc.DecodeString(input)
		if err == nil {
			return string(decoded), nil
		}
	}

	// Try adding padding
	temp := input
	if len(temp)%4 != 0 {
		temp += strings.Repeat("=", 4-len(temp)%4)
		for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
			decoded, err := enc.DecodeString(temp)
			if err == nil {
				return string(decoded), nil
			}
		}
	}

	return "", fmt.Errorf("failed to decode base64")
}

func fetchSingleURL(client *http.Client, urlStr string) ([]string, error) {
	resp, err := client.Get(urlStr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rawText := string(body)
	if strings.HasPrefix(strings.TrimSpace(rawText), "<") {
		return nil, fmt.Errorf("content is HTML")
	}

	// Try base64 decoding first
	if decoded, err := decodeBase64(rawText); err == nil {
		log.Printf("[Scraper] Successfully decoded base64 from %s", urlStr)
		rawText = decoded
	}

	// Standardize line endings and split
	rawText = strings.ReplaceAll(rawText, "\r\n", "\n")
	lines := strings.Split(rawText, "\n")
	
	var uris []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "://") {
			uris = append(uris, line)
		}
	}

	return uris, nil
}
