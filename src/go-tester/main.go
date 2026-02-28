package main

import (
	"bytes"
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
	XrayConfig     map[string]interface{} `json:"xray_config"`
	XrayPath       string                 `json:"xray_path"`
	XrayAssetsPath string                 `json:"xray_assets_path"`
	TestURL        string                 `json:"test_url"`
	TimeoutSec     int                    `json:"timeout"`
	Ports          []int                  `json:"ports"`
	IsSiteCheck    bool                   `json:"is_site_check"`
}

type TestResult struct {
	Port   int  `json:"port"`
	Delay  int  `json:"delay"`
	Failed bool `json:"failed"`
}

func main() {
	// 1. Parse Input
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("failed to read stdin: %v", err)
	}

	var req TestRequest
	if err := json.Unmarshal(input, &req); err != nil {
		log.Fatalf("failed to unmarshal request: %v", err)
	}

	if len(req.Ports) == 0 {
		fmt.Println("[]")
		return
	}

	// 2. Prepare Xray Config File
	tmpFile, err := os.CreateTemp("", "xray-config-*.json")
	if err != nil {
		log.Fatalf("failed to create temp config: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configBytes, _ := json.Marshal(req.XrayConfig)
	if _, err := tmpFile.Write(configBytes); err != nil {
		log.Fatalf("failed to write temp config: %v", err)
	}
	tmpFile.Close()

	// 3. Start Xray Subprocess
	xrayPath := req.XrayPath
	if xrayPath == "" {
		xrayPath = "xray" // fallback
	}
	
	cmd := exec.Command(xrayPath, "-c", tmpFile.Name())
	
	// Capture stderr for debugging
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// Set environment variables for Xray
	if req.XrayAssetsPath != "" {
		env := os.Environ()
		env = append(env, "XRAY_LOCATION_ASSET="+req.XrayAssetsPath)
		cmd.Env = env
	}
	
	if err := cmd.Start(); err != nil {
		log.Fatalf("failed to start xray: %v", err)
	}

	// Ensure process is killed on exit
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// 4. Wait for the first port to be ready
	firstPort := req.Ports[0]
	if !waitForPort(firstPort, 30*time.Second) {
		log.Fatalf("xray failed to bind port %d in time. Stderr: %s", firstPort, stderrBuf.String())
	}

	// 5. Test all ports concurrently
	results := make([]TestResult, len(req.Ports))
	var wg sync.WaitGroup

	for i, port := range req.Ports {
		wg.Add(1)
		go func(idx, p int) {
			defer wg.Done()
			delay := testProxy(p, req.TestURL, time.Duration(req.TimeoutSec)*time.Second, req.IsSiteCheck)
			results[idx] = TestResult{
				Port:   p,
				Delay:  delay,
				Failed: delay < 0,
			}
		}(i, port)
	}

	wg.Wait()

	// 6. Return JSON results
	outBytes, _ := json.Marshal(results)
	fmt.Println(string(outBytes))
}

func waitForPort(port int, timeout time.Duration) bool {
	start := time.Now()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Since(start) < timeout {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func testProxy(port int, targetURL string, timeout time.Duration, isSiteCheck bool) int {
	proxyURL, err := url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d", port))
	if err != nil {
		return -1
	}

	// Configure client
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			DisableKeepAlives: true,
		},
		Timeout: timeout,
	}

	// If it's a latency test (not a site check), do not follow redirects
	if !isSiteCheck {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	start := time.Now()
	resp, err := client.Head(targetURL)
	if err != nil {
		return -1
	}
	defer resp.Body.Close()

	if isSiteCheck {
		if resp.StatusCode < 400 {
			return int(time.Since(start).Milliseconds())
		}
	} else {
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return int(time.Since(start).Milliseconds())
		}
	}

	return -1
}
