package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

type TestResult struct {
	Server ProxyServer
	Delay  int
	Failed bool
}

type Tester struct{ config Config }

func NewTester(config Config) *Tester { return &Tester{config: config} }

func (t *Tester) Test(ctx context.Context, servers []ProxyServer, target string, siteCheck bool) []TestResult {
	if len(servers) == 0 {
		return nil
	}
	type batch struct {
		index, slot int
		servers     []ProxyServer
	}
	batchCount := (len(servers) + t.config.BatchSize - 1) / t.config.BatchSize
	jobs := make(chan batch)
	results := make(chan []TestResult, batchCount)
	workers := min(t.config.MaxConcurrentBatches, batchCount)
	var wg sync.WaitGroup
	for slot := 0; slot < workers; slot++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			for job := range jobs {
				basePort := t.config.BasePort + slot*t.config.BatchSize
				results <- t.testBatch(ctx, job.servers, target, siteCheck, basePort)
			}
		}(slot)
	}
	go func() {
		for i := 0; i < len(servers); i += t.config.BatchSize {
			end := min(i+t.config.BatchSize, len(servers))
			jobs <- batch{index: i / t.config.BatchSize, servers: servers[i:end]}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	all := make([]TestResult, 0, len(servers))
	for result := range results {
		all = append(all, result...)
	}
	return all
}

func (t *Tester) testBatch(ctx context.Context, servers []ProxyServer, target string, siteCheck bool, basePort int) []TestResult {
	inbounds := make([]any, 0, len(servers))
	outbounds := make([]any, 0, len(servers))
	rules := make([]any, 0, len(servers))
	for i, server := range servers {
		inTag, outTag := fmt.Sprintf("in-%d", i), fmt.Sprintf("out-%d", i)
		inbounds = append(inbounds, map[string]any{"tag": inTag, "port": basePort + i, "listen": "127.0.0.1", "protocol": "socks", "settings": map[string]any{"auth": "noauth", "udp": true}})
		outbound, err := server.XrayOutbound(outTag)
		if err != nil {
			outbound = map[string]any{"tag": outTag, "protocol": "blackhole"}
		}
		outbounds = append(outbounds, outbound)
		rules = append(rules, map[string]any{"type": "field", "inboundTag": []string{inTag}, "outboundTag": outTag})
	}
	configuration := map[string]any{"log": map[string]any{"loglevel": "error"}, "inbounds": inbounds, "outbounds": outbounds, "routing": map[string]any{"rules": rules}}
	data, _ := json.Marshal(configuration)
	file, err := os.CreateTemp("", "v2ray-scrapper-xray-*.json")
	if err != nil {
		return failedResults(servers)
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return failedResults(servers)
	}
	file.Close()
	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(processCtx, t.config.XrayPath, "-c", name)
	cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+t.config.XrayAssetsPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return failedResults(servers)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ready, startErr := waitForPort(processCtx, basePort, t.config.XrayStartTimeout, done)
	if !ready {
		cancel()
		if startErr == nil {
			startErr = <-done
		}
		if len(servers) > 1 && ctx.Err() == nil {
			slog.Warn("xray rejected a batch; isolating invalid configurations", "batch_size", len(servers), "error", startErr, "stderr", stderr.String())
			middle := len(servers) / 2
			left := t.testBatch(ctx, servers[:middle], target, siteCheck, basePort)
			right := t.testBatch(ctx, servers[middle:], target, siteCheck, basePort+middle)
			return append(left, right...)
		}
		if stderr.Len() > 0 {
			slog.Warn("xray failed to start", "error", startErr, "stderr", stderr.String())
		}
		return failedResults(servers)
	}
	result := make([]TestResult, len(servers))
	tasks := make(chan int)
	var wg sync.WaitGroup
	for range min(50, len(servers)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				delay := testSOCKSProxy(processCtx, basePort+i, target, t.config.TestTimeout, siteCheck)
				result[i] = TestResult{Server: servers[i], Delay: delay, Failed: delay < 0}
			}
		}()
	}
	for i := range servers {
		tasks <- i
	}
	close(tasks)
	wg.Wait()
	cancel()
	<-done
	return result
}

func waitForPort(ctx context.Context, port int, timeout time.Duration, exited <-chan error) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case err := <-exited:
			if err == nil {
				err = fmt.Errorf("xray exited before opening its SOCKS port")
			}
			return false, err
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
			connection, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
			if err == nil {
				connection.Close()
				return true, nil
			}
		}
	}
}

func testSOCKSProxy(ctx context.Context, port int, target string, timeout time.Duration, siteCheck bool) int {
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return -1
	}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.Dial(network, address)
	}}
	client := &http.Client{Transport: transport, Timeout: timeout}
	if !siteCheck {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return -1
	}
	start := time.Now()
	response, err := client.Do(request)
	if err != nil {
		return -1
	}
	response.Body.Close()
	if (siteCheck && response.StatusCode < 400) || (!siteCheck && response.StatusCode >= 200 && response.StatusCode < 300) {
		return int(time.Since(start).Milliseconds())
	}
	return -1
}

func failedResults(servers []ProxyServer) []TestResult {
	result := make([]TestResult, len(servers))
	for i, s := range servers {
		result[i] = TestResult{Server: s, Delay: -1, Failed: true}
	}
	return result
}
