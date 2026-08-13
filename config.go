package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type SiteConfig struct {
	URL      string `json:"url" yaml:"url"`
	Filename string `json:"filename" yaml:"filename"`
	Enabled  *bool  `json:"enabled,omitempty" yaml:"enabled"`
}

type fileConfig struct {
	Git struct {
		Branch       string `yaml:"branch"`
		PushInterval int    `yaml:"push_interval"`
	} `yaml:"git"`
	Sites []SiteConfig `yaml:"sites"`
}

type Config struct {
	ListenAddr           string
	SubscriptionURLs     []string
	SingBoxPath          string
	LatencyTestURL       string
	FetchTimeout         time.Duration
	TestTimeout          time.Duration
	TestAttempts         int
	SingBoxStartTimeout  time.Duration
	CacheInterval        time.Duration
	SiteCacheTTL         time.Duration
	BatchSize            int
	BasePort             int
	MaxDelayMS           int
	MaxConcurrentBatches int
	MaxConcurrentTests   int
	MaxFailCount         int
	MaxCandidates        int
	StatePath            string
	GeoIPPath            string
	ConfigPath           string
	RedisURL             string
	RedisPrefix          string
	ManagementToken      string
	GitPushEnabled       bool
	GitMainPushEnabled   bool
	GitSitePushEnabled   bool
	GitRepoURL           string
	GitToken             string
	GitUser              string
	GitEmail             string
	GitBranch            string
	GitFilename          string
	GitRepoDir           string
	Sites                []SiteConfig
}

func LoadConfig() (Config, error) {
	c := Config{
		ListenAddr: env("LISTEN_ADDR", "0.0.0.0:8084"),
		SubscriptionURLs: envList("SUB_URLS", env("SUB_URL", `[
            "https://raw.githubusercontent.com/Epodonios/v2ray-configs/main/Sub1.txt",
            "https://raw.githubusercontent.com/barry-far/V2ray-config/main/Sub1.txt",
            "https://raw.githubusercontent.com/ebrasha/free-v2ray-public-list/refs/heads/main/V2Ray-Config-By-EbraSha.txt"
        ]`)),
		SingBoxPath:          env("SING_BOX_PATH", "/usr/local/bin/sing-box"),
		LatencyTestURL:       env("LATENCY_TEST_URL", "https://www.google.com/generate_204"),
		FetchTimeout:         envDurationSeconds("FETCH_TIMEOUT", 20),
		TestTimeout:          envDurationSeconds("TEST_TIMEOUT", 6),
		TestAttempts:         envInt("TEST_ATTEMPTS", 2),
		SingBoxStartTimeout:  envDurationSeconds("SING_BOX_START_TIMEOUT", 5),
		CacheInterval:        envDurationSeconds("CACHE_INTERVAL_SECONDS", 600),
		SiteCacheTTL:         envDurationSeconds("SITE_CACHE_TTL_SECONDS", 86400),
		BatchSize:            envInt("BATCH_SIZE", 100),
		BasePort:             envInt("BASE_PORT", 20000),
		MaxDelayMS:           envInt("MAX_DELAY_MS", 10000),
		MaxConcurrentBatches: envInt("MAX_CONCURRENT_BATCHES", 10),
		MaxConcurrentTests:   envInt("MAX_CONCURRENT_TESTS", 100),
		MaxFailCount:         envInt("MAX_FAIL_COUNT", 3),
		MaxCandidates:        envInt("MAX_CANDIDATES", 10000),
		StatePath:            env("STATE_FILE_PATH", "data/state.json"),
		GeoIPPath:            env("GEOIP_DB_PATH", "src/Country.mmdb"),
		ConfigPath:           env("YAML_CONFIG_PATH", "config.yaml"),
		RedisURL:             strings.TrimSpace(os.Getenv("REDIS_URL")),
		RedisPrefix:          env("REDIS_PREFIX", "v2ray-scrapper"),
		ManagementToken:      strings.TrimSpace(os.Getenv("MANAGEMENT_TOKEN")),
		GitPushEnabled:       envBool("GITHUB_PUSH_ENABLED", false),
		GitMainPushEnabled:   envBool("GITHUB_MAIN_PUSH_ENABLED", true),
		GitSitePushEnabled:   envBool("GITHUB_SITE_PUSH_ENABLED", true),
		GitRepoURL:           os.Getenv("GITHUB_REPO_URL"),
		GitToken:             os.Getenv("GITHUB_TOKEN"),
		GitUser:              env("GITHUB_USER", "V2Ray Updater"),
		GitEmail:             env("GITHUB_EMAIL", "bot@example.com"),
		GitBranch:            env("GITHUB_BRANCH", "main"),
		GitFilename:          env("GITHUB_FILENAME", "subscription.txt"),
		GitRepoDir:           env("GITHUB_REPO_DIR", "data/subscription_repo"),
	}
	if envBool("LOW_INTERNET_CONS", false) {
		c.MaxCandidates = min(c.MaxCandidates, envInt("LOW_INTERNET_LIMIT", 50))
	}
	if c.BatchSize < 1 || c.MaxConcurrentBatches < 1 || c.MaxConcurrentTests < 1 || c.TestAttempts < 1 || c.MaxFailCount < 1 || c.MaxCandidates < 1 {
		return Config{}, fmt.Errorf("batch and concurrency values must be positive")
	}
	if c.BasePort < 1024 || c.BasePort+2*c.BatchSize*c.MaxConcurrentBatches > 65535 {
		return Config{}, fmt.Errorf("BASE_PORT and batch range must fit within 1024-65535")
	}
	if c.CacheInterval <= 0 || c.TestTimeout <= 0 || c.SingBoxStartTimeout <= 0 || c.FetchTimeout <= 0 {
		return Config{}, fmt.Errorf("timeout and interval values must be positive")
	}
	if err := c.loadYAML(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c *Config) loadYAML() error {
	data, err := os.ReadFile(c.ConfigPath)
	if os.IsNotExist(err) {
		c.Sites = sitesFromURLs(envList("PRECHECK_SITES", ""))
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", c.ConfigPath, err)
	}
	var cfg fileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse %s: %w", c.ConfigPath, err)
	}
	if cfg.Git.Branch != "" {
		c.GitBranch = cfg.Git.Branch
	}
	if cfg.Git.PushInterval > 0 {
		c.CacheInterval = time.Duration(cfg.Git.PushInterval) * time.Second
	}
	for _, site := range cfg.Sites {
		if site.URL != "" && site.Filename != "" && (site.Enabled == nil || *site.Enabled) {
			c.Sites = append(c.Sites, site)
		}
	}
	if len(c.Sites) == 0 {
		c.Sites = sitesFromURLs(envList("PRECHECK_SITES", ""))
	}
	return nil
}

func sitesFromURLs(urls []string) []SiteConfig {
	result := make([]SiteConfig, 0, len(urls))
	for _, raw := range urls {
		host := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
		host = strings.TrimPrefix(strings.Split(host, "/")[0], "www.")
		if host != "" {
			enabled := true
			result = append(result, SiteConfig{URL: raw, Filename: strings.ReplaceAll(host, ".", "_") + ".txt", Enabled: &enabled})
		}
	}
	return result
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return b
}

func envDurationSeconds(key string, fallback int) time.Duration {
	return time.Duration(envInt(key, fallback)) * time.Second
}

func envList(key, fallback string) []string {
	raw := strings.TrimSpace(env(key, fallback))
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var values []string
		if json.Unmarshal([]byte(raw), &values) == nil {
			return cleanStrings(values)
		}
	}
	return cleanStrings(strings.Split(raw, ","))
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
