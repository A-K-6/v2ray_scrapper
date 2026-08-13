package main

import (
	"context"
	"testing"
	"time"
)

func TestTesterReturnsFailureWhenXrayCannotStart(t *testing.T) {
	tester := NewTester(Config{XrayPath: "/definitely/missing/xray", TestTimeout: time.Millisecond, XrayStartTimeout: time.Millisecond, BatchSize: 10, MaxConcurrentBatches: 1, BasePort: 20000})
	servers := []ProxyServer{{Protocol: "vless", Address: "1.2.3.4", Port: 443, ID: "uuid"}}
	results := tester.Test(context.Background(), servers, "https://example.com", false)
	if len(results) != 1 || !results[0].Failed || results[0].Delay != -1 {
		t.Fatalf("results=%#v", results)
	}
}

func TestFailedResultsPreservesServers(t *testing.T) {
	servers := []ProxyServer{{Address: "one"}, {Address: "two"}}
	results := failedResults(servers)
	if len(results) != 2 || results[1].Server.Address != "two" || !results[0].Failed {
		t.Fatalf("results=%#v", results)
	}
}

func TestWaitForPortReturnsWhenXrayExits(t *testing.T) {
	exited := make(chan error, 1)
	exited <- nil
	ready, err := waitForPort(context.Background(), 1, time.Second, exited)
	if ready || err == nil {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestCombinedProcessOutputIncludesStdoutAndStderr(t *testing.T) {
	got := combinedProcessOutput(" config rejected \n", " details \n")
	if got != "config rejected\ndetails" {
		t.Fatalf("output=%q", got)
	}
}
