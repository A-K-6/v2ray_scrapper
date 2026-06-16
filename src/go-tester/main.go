package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"time"
)

type TestRequest struct {
	Command            string         `json:"command"`
	SubURLs            []string       `json:"sub_urls"`
	RawURIs            []string       `json:"raw_uris"`
	XrayPath           string         `json:"xray_path"`
	XrayAssetsPath     string         `json:"xray_assets_path"`
	TestURL            string         `json:"test_url"`
	TimeoutSec         int            `json:"timeout"`
	BasePort           int            `json:"base_port"`
	BatchSize          int            `json:"batch_size"`
	MaxParallelBatches int            `json:"max_parallel_batches"`
	IsSiteCheck        bool           `json:"is_site_check"`
	StatePath          string         `json:"state_path"`
	Git                GitPushRequest `json:"git"`
}

type TestResult struct {
	URI    string `json:"uri,omitempty"`
	Port   int    `json:"port"`
	Delay  int    `json:"delay"`
	Failed bool   `json:"failed"`
}

type ServerState struct {
	URI       string    `json:"uri"`
	LastCheck time.Time `json:"last_check"`
	FailCount int       `json:"fail_count"`
	LastDelay int       `json:"last_delay"`
}

type GlobalState struct {
	Servers map[string]ServerState `json:"servers"`
}

func main() {
	var req TestRequest
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&req); err != nil {
		log.Fatalf("failed to decode request: %v", err)
	}

	if req.BatchSize <= 0 {
		req.BatchSize = 100
	}
	if req.MaxParallelBatches <= 0 {
		req.MaxParallelBatches = 4
	}

	log.Printf("[Main] Received command: %s (BatchSize: %d, Parallel: %d)", req.Command, req.BatchSize, req.MaxParallelBatches)

	switch req.Command {
	case "scrape-and-test":
		handleScrapeAndTest(req)
	case "git-push":
		err := handleGitPush(req.Git)
		if err != nil {
			log.Fatalf("git push failed: %v", err)
		}
		fmt.Println("{\"status\": \"success\"}")
	default:
		results := runBatchedTests(req)
		outBytes, _ := json.Marshal(results)
		fmt.Println(string(outBytes))
	}
}

func loadState(path string) GlobalState {
	state := GlobalState{Servers: make(map[string]ServerState)}
	if path == "" {
		return state
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	json.Unmarshal(data, &state)
	return state
}

func saveState(path string, state GlobalState) {
	if path == "" {
		return
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	os.WriteFile(path, data, 0644)
}

func handleScrapeAndTest(req TestRequest) {
	state := loadState(req.StatePath)
	log.Printf("[Main] Loaded state from %s, found %d servers", req.StatePath, len(state.Servers))

	scrapeResults := scrapeAll(req.SubURLs, time.Duration(req.TimeoutSec)*time.Second)

	uniqueServers := make(map[string]string)
	totalScraped := 0

	// 1. Process existing servers from state
	for fp, s := range state.Servers {
		if s.FailCount < 5 {
			uniqueServers[fp] = s.URI
		}
	}

	// 2. Process newly scraped servers
	for _, res := range scrapeResults {
		totalScraped += len(res.URIs)
		for _, uri := range res.URIs {
			config, err := parseRawURI(uri)
			if err == nil {
				fp := config.Fingerprint()
				if _, exists := uniqueServers[fp]; !exists {
					uniqueServers[fp] = uri
				}
			}
		}
	}

	var rawURIs []string
	for _, uri := range uniqueServers {
		rawURIs = append(rawURIs, uri)
	}

	log.Printf("[Main] Connection fingerprinting complete: %d unique servers from ~%d URIs", len(rawURIs), totalScraped)

	if len(rawURIs) == 0 {
		fmt.Println("[]")
		return
	}

	req.RawURIs = rawURIs
	allResults := runBatchedTests(req)

	for _, res := range allResults {
		config, err := parseRawURI(res.URI)
		if err != nil {
			continue
		}
		fp := config.Fingerprint()

		s, ok := state.Servers[fp]
		if !ok {
			s = ServerState{URI: res.URI}
		}
		s.LastCheck = time.Now()
		if res.Failed {
			s.FailCount++
			s.LastDelay = -1
		} else {
			s.FailCount = 0
			s.LastDelay = res.Delay
		}
		state.Servers[fp] = s
	}
	saveState(req.StatePath, state)

	outBytes, _ := json.Marshal(allResults)
	fmt.Println(string(outBytes))
}

func runBatchedTests(req TestRequest) []TestResult {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allResults []TestResult

	sem := make(chan struct{}, req.MaxParallelBatches)

	for i := 0; i < len(req.RawURIs); i += req.BatchSize {
		end := i + req.BatchSize
		if end > len(req.RawURIs) {
			end = len(req.RawURIs)
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(startIdx, endIdx int) {
			defer wg.Done()
			defer func() { <-sem }()

			log.Printf("[Main] Starting batch %d-%d", startIdx, endIdx)
			batchReq := req
			batchReq.RawURIs = req.RawURIs[startIdx:endIdx]
			batchReq.BasePort = req.BasePort + startIdx

			batchResults := handleTestOnlyInternal(batchReq)

			mu.Lock()
			allResults = append(allResults, batchResults...)
			
			// Incremental State Update: If we're in scrape-and-test mode, save state now
			if req.Command == "scrape-and-test" && req.StatePath != "" {
				state := loadState(req.StatePath)
				for _, res := range batchResults {
					config, err := parseRawURI(res.URI)
					if err != nil { continue }
					fp := config.Fingerprint()

					s, ok := state.Servers[fp]
					if !ok { s = ServerState{URI: res.URI} }
					s.LastCheck = time.Now()
					if res.Failed {
						s.FailCount++
						s.LastDelay = -1
					} else {
						s.FailCount = 0
						s.LastDelay = res.Delay
					}
					state.Servers[fp] = s
				}
				saveState(req.StatePath, state)
				log.Printf("[Main] Incremental state saved after batch %d-%d (Total: %d)", startIdx, endIdx, len(state.Servers))
			}
			
			mu.Unlock()
			log.Printf("[Main] Finished batch %d-%d", startIdx, endIdx)
		}(i, end)
	}

	wg.Wait()
	return allResults
}

func handleTestOnlyInternal(req TestRequest) []TestResult {
	if len(req.RawURIs) == 0 {
		return []TestResult{}
	}

	inbounds := []map[string]interface{}{}
	outbounds := []map[string]interface{}{}
	rules := []map[string]interface{}{}
	ports := []int{}

	for i, uri := range req.RawURIs {
		port := req.BasePort + i
		ports = append(ports, port)
		inboundTag := fmt.Sprintf("in-%d", i)
		outboundTag := fmt.Sprintf("out-%d", i)

		inbounds = append(inbounds, map[string]interface{}{
			"tag": inboundTag, "port": port, "listen": "127.0.0.1", "protocol": "socks",
			"settings": map[string]interface{}{"auth": "noauth", "udp": true, "ip": "127.0.0.1"},
		})

		outbound, err := parseRawURI(uri)
		if err != nil {
			outbounds = append(outbounds, map[string]interface{}{"tag": outboundTag, "protocol": "blackhole"})
		} else {
			outboundMap, _ := json.Marshal(outbound)
			var outboundFinal map[string]interface{}
			json.Unmarshal(outboundMap, &outboundFinal)
			outboundFinal["tag"] = outboundTag
			outbounds = append(outbounds, outboundFinal)
		}

		rules = append(rules, map[string]interface{}{"type": "field", "inboundTag": []string{inboundTag}, "outboundTag": outboundTag})
	}

	xrayConfig := map[string]interface{}{
		"log":      map[string]interface{}{"loglevel": "error"},
		"inbounds": inbounds, "outbounds": outbounds, "routing": map[string]interface{}{"rules": rules},
	}

	tmpFile, _ := os.CreateTemp("", "xray-config-*.json")
	defer os.Remove(tmpFile.Name())
	configBytes, _ := json.Marshal(xrayConfig)
	tmpFile.Write(configBytes)
	tmpFile.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, req.XrayPath, "-c", tmpFile.Name())
	if req.XrayAssetsPath != "" {
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+req.XrayAssetsPath)
	}

	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		log.Printf("[Batch %d] Failed to start xray: %v", req.BasePort, err)
		return []TestResult{}
	}

	// Buffer stderr to report it on failure
	stderrChan := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := stderrPipe.Read(buf)
		if n > 0 {
			stderrChan <- string(buf[:n])
		} else {
			stderrChan <- ""
		}
		io.Copy(io.Discard, stderrPipe)
	}()

	if !waitForPort(cmd, ports[0], 20*time.Second) {
		select {
		case errOutput := <-stderrChan:
			log.Printf("[Batch %d] Xray failed to bind port in time. Stderr: %s", req.BasePort, errOutput)
		default:
			log.Printf("[Batch %d] Xray failed to bind port in time (no stderr)", req.BasePort)
		}
		return []TestResult{}
	}

	results := make([]TestResult, len(ports))
	var wg sync.WaitGroup
	taskChan := make(chan int, len(ports))
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		MaxIdleConns: 100, DisableKeepAlives: true,
	}

	workerCount := 50 // Fixed worker count for testing within a batch
	if workerCount > len(ports) {
		workerCount = len(ports)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range taskChan {
				port := ports[idx]
				delay := testProxy(port, req.TestURL, time.Duration(req.TimeoutSec)*time.Second, req.IsSiteCheck, transport)
				results[idx] = TestResult{
					URI: req.RawURIs[idx], Port: port, Delay: delay, Failed: delay < 0,
				}
			}
		}()
	}

	for i := range ports {
		taskChan <- i
	}
	close(taskChan)
	wg.Wait()

	return results
}

func waitForPort(cmd *exec.Cmd, port int, timeout time.Duration) bool {
	start := time.Now()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Since(start) < timeout {
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return false
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func testProxy(port int, targetURL string, timeout time.Duration, isSiteCheck bool, baseTransport *http.Transport) int {
	proxyURL, _ := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", port))
	tr := baseTransport.Clone()
	tr.Proxy = http.ProxyURL(proxyURL)
	client := &http.Client{Transport: tr, Timeout: timeout}
	if !isSiteCheck {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	}
	start := time.Now()
	resp, err := client.Get(targetURL)
	if err != nil { return -1 }
	defer resp.Body.Close()
	if isSiteCheck {
		if resp.StatusCode < 400 { return int(time.Since(start).Milliseconds()) }
	} else {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 { return int(time.Since(start).Milliseconds()) }
	}
	return -1
}
