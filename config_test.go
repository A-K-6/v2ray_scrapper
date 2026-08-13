package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEnvListSupportsCSVAndJSON(t *testing.T) {
	t.Setenv("TEST_LIST", "one, two")
	if got := envList("TEST_LIST", ""); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("CSV=%v", got)
	}
	t.Setenv("TEST_LIST", `["one","two"]`)
	if got := envList("TEST_LIST", ""); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("JSON=%v", got)
	}
}

func TestLoadYAMLDefaultsEnabledAndHonorsDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "sites:\n  - url: https://one.example\n    filename: one.txt\n  - url: https://two.example\n    filename: two.txt\n    enabled: false\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	config := Config{ConfigPath: path}
	if err := config.loadYAML(); err != nil {
		t.Fatal(err)
	}
	if len(config.Sites) != 1 || config.Sites[0].Filename != "one.txt" {
		t.Fatalf("sites=%#v", config.Sites)
	}
}

func TestProductionDefaultsMeetRefreshBudget(t *testing.T) {
	for _, key := range []string{"SUB_URLS", "SUB_URL", "CACHE_INTERVAL_SECONDS", "MAX_CANDIDATES", "YAML_CONFIG_PATH"} {
		t.Setenv(key, "")
	}
	t.Setenv("YAML_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.CacheInterval != 10*time.Minute {
		t.Fatalf("refresh interval=%s", config.CacheInterval)
	}
	if config.MaxCandidates != 60 {
		t.Fatalf("max candidates=%d", config.MaxCandidates)
	}
	if config.BatchSize != 20 || config.MaxConcurrentBatches != 3 || config.TestTimeout != 6*time.Second {
		t.Fatalf("cold-start profile: batch=%d concurrency=%d timeout=%s", config.BatchSize, config.MaxConcurrentBatches, config.TestTimeout)
	}
	if len(config.SubscriptionURLs) != 3 {
		t.Fatalf("sources=%v", config.SubscriptionURLs)
	}
}
