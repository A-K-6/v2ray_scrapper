package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func scraperWithResponse(status int, body string) *Scraper {
	return &Scraper{client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
}

func TestScraperFetchesAndParsesSubscription(t *testing.T) {
	servers, err := scraperWithResponse(http.StatusOK, "vless://"+testUUIDA+"@1.2.3.4:443?encryption=none&type=tcp").Fetch(context.Background(), "https://source.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Protocol != "vless" {
		t.Fatalf("servers=%#v", servers)
	}
}

func TestScraperRejectsHTML(t *testing.T) {
	if _, err := scraperWithResponse(http.StatusOK, "<html>not a subscription</html>").Fetch(context.Background(), "https://source.example"); err == nil {
		t.Fatal("expected HTML error")
	}
}

func TestFetchAllBalancesSources(t *testing.T) {
	scraper := &Scraper{client: &http.Client{Timeout: time.Second, Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := "vless://" + testUUIDA + "@1.1.1.1:443\nvless://" + testUUIDB + "@1.1.1.2:443"
		if request.URL.Host == "two.example" {
			body = "vless://" + testUUIDA + "@2.2.2.1:443\nvless://" + testUUIDB + "@2.2.2.2:443"
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	servers := scraper.FetchAll(context.Background(), []string{"https://one.example/sub", "https://two.example/sub"})
	if len(servers) != 4 || servers[0].Address != "1.1.1.1" || servers[1].Address != "2.2.2.1" || servers[2].Address != "1.1.1.2" || servers[3].Address != "2.2.2.2" {
		t.Fatalf("unbalanced result: %#v", servers)
	}
}
