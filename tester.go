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
	"regexp"
	"strconv"
	"strings"
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
		start   int
		servers []ProxyServer
	}
	batchSize := max(1, t.config.BatchSize)
	batchCount := (len(servers) + batchSize - 1) / batchSize
	jobs := make(chan batch)
	type completedBatch struct {
		start   int
		results []TestResult
	}
	results := make(chan completedBatch, batchCount)
	workers := min(max(1, t.config.MaxConcurrentBatches), batchCount)
	var wg sync.WaitGroup
	for slot := 0; slot < workers; slot++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			for job := range jobs {
				basePort := t.config.BasePort + slot*batchSize
				results <- completedBatch{start: job.start, results: t.testBatch(ctx, job.servers, target, siteCheck, basePort)}
			}
		}(slot)
	}
	go func() {
		for i := 0; i < len(servers); i += batchSize {
			end := min(i+batchSize, len(servers))
			select {
			case jobs <- batch{start: i, servers: servers[i:end]}:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				close(results)
				return
			}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	all := failedResults(servers)
	for completed := range results {
		copy(all[completed.start:], completed.results)
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
			return t.testBuildableBatch(ctx, servers, target, siteCheck, basePort, i, err)
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
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
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
		output := combinedProcessOutput(stdout.String(), stderr.String())
		if len(servers) > 1 && ctx.Err() == nil {
			if invalid := invalidOutboundIndex(output); invalid >= 0 && invalid < len(servers) {
				return t.testBuildableBatch(ctx, servers, target, siteCheck, basePort, invalid, fmt.Errorf("xray rejected outbound %d", invalid))
			}
			slog.Warn("xray rejected a batch; isolating invalid configurations", "batch_size", len(servers), "error", startErr, "output", output)
			middle := len(servers) / 2
			left := t.testBatch(ctx, servers[:middle], target, siteCheck, basePort)
			right := t.testBatch(ctx, servers[middle:], target, siteCheck, basePort+middle)
			return append(left, right...)
		}
		if output != "" {
			slog.Warn("xray failed to start", "error", startErr, "output", output)
		}
		return failedResults(servers)
	}
	result := make([]TestResult, len(servers))
	tasks := make(chan int)
	var wg sync.WaitGroup
	for range min(max(1, t.config.MaxConcurrentTests), len(servers)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range tasks {
				delay := testSOCKSProxy(processCtx, basePort+i, target, t.config.TestTimeout, siteCheck, max(1, t.config.TestAttempts))
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

func (t *Tester) testBuildableBatch(ctx context.Context, servers []ProxyServer, target string, siteCheck bool, basePort, invalid int, cause error) []TestResult {
	slog.Debug("skipping Xray-incompatible configuration", "protocol", servers[invalid].Protocol, "address", servers[invalid].Address, "error", cause)
	result := failedResults(servers)
	valid := make([]ProxyServer, 0, len(servers)-1)
	positions := make([]int, 0, len(servers)-1)
	for i, server := range servers {
		if i != invalid {
			valid = append(valid, server)
			positions = append(positions, i)
		}
	}
	if len(valid) == 0 {
		return result
	}
	for i, tested := range t.testBatch(ctx, valid, target, siteCheck, basePort) {
		result[positions[i]] = tested
	}
	return result
}

var outboundIndexPattern = regexp.MustCompile(`(?:tag|outbound) out-(\d+)`)

func invalidOutboundIndex(output string) int {
	match := outboundIndexPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return -1
	}
	index, err := strconv.Atoi(match[1])
	if err != nil {
		return -1
	}
	return index
}

func combinedProcessOutput(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	if value := strings.TrimSpace(stdout); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(stderr); value != "" {
		parts = append(parts, value)
	}
	return strings.Join(parts, "\n")
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

func testSOCKSProxy(ctx context.Context, port int, target string, timeout time.Duration, siteCheck bool, attempts int) int {
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", port), nil, &net.Dialer{Timeout: timeout})
	if err != nil {
		return -1
	}
	transport := &http.Transport{DisableKeepAlives: true, ForceAttemptHTTP2: true, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			return contextDialer.DialContext(ctx, network, address)
		}
		return dialer.Dial(network, address)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: timeout}
	if !siteCheck {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	totalDelay := int64(0)
	for range max(1, attempts) {
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
		if !successfulProbe(response.StatusCode, target, siteCheck) {
			return -1
		}
		totalDelay += time.Since(start).Milliseconds()
	}
	return max(1, int(totalDelay/int64(max(1, attempts))))
}

func successfulProbe(status int, target string, siteCheck bool) bool {
	if !siteCheck && strings.Contains(target, "generate_204") {
		return status == http.StatusNoContent
	}
	if siteCheck {
		return status >= 200 && status < 400
	}
	return status >= 200 && status < 300
}

func failedResults(servers []ProxyServer) []TestResult {
	result := make([]TestResult, len(servers))
	for i, s := range servers {
		result[i] = TestResult{Server: s, Delay: -1, Failed: true}
	}
	return result
}
