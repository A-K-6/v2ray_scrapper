package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aeen/v2ray-scrapper/internal/api"
	"github.com/aeen/v2ray-scrapper/internal/config"
	"github.com/aeen/v2ray-scrapper/internal/proxy"
	"github.com/aeen/v2ray-scrapper/internal/service"
	"github.com/aeen/v2ray-scrapper/internal/singbox"
	"github.com/aeen/v2ray-scrapper/internal/store"
	"github.com/aeen/v2ray-scrapper/internal/xdg"
)

var Version = "dev"

// RunCLI dispatches v2rays subcommands. No-args preserves the legacy
// `go run .` behaviour (serve).
func RunCLI(args []string) int {
	if len(args) < 2 {
		return runServe([]string{})
	}
	switch args[1] {
	case "serve", "server", "up":
		return runServe(args[2:])
	case "refresh", "update":
		return runRefresh(args[2:])
	case "get", "export", "cache":
		return runGet(args[2:])
	case "test":
		return runTestCmd(args[2:])
	case "sources", "source", "subscriptions":
		return runSources(args[2:])
	case "sites", "site":
		return runSites(args[2:])
	case "token":
		return runToken(args[2:])
	case "config", "init", "setup":
		return runConfigCmd(args[2:])
	case "doctor", "status", "check":
		return runDoctor(args[2:])
	case "tui", "menu", "ui":
		return runTUI()
	case "version", "--version", "-v":
		fmt.Printf("v2rays %s\n", Version)
		return 0
	case "help", "--help", "-h":
		printHelp()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[1])
		printHelp()
		return 2
	}
}

func printHelp() {
	fmt.Printf(`v2rays %s - standalone proxy scraper & tester

Usage:
  v2rays <command> [flags]

Commands:
  serve                 Run the HTTP API server (default)
  refresh               Run one scrape+test cycle, then export results
  get                   Export cached results from state file (no network)
  test                  Test a subscription URL or raw content ad-hoc
  sources <list|add|rm> Manage subscription source URLs
  sites <list|add|rm>    Manage preloaded site checks
  token <show|set|gen>  Manage MANAGEMENT_TOKEN
  config <init|show|path> Setup and inspect configuration
  doctor                Check sing-box, geoip, config and state health
  tui                   Interactive text menu (easiest way to start)
  version               Print version

Examples:
  v2rays tui
  v2rays refresh --out subscription.txt --format base64
  v2rays get --all --country US,DE --format raw
  v2rays sources add https://example.com/sub.txt
  v2rays serve

Install:
  curl -fsSL https://raw.githubusercontent.com/A-K-6/v2ray_scrapper/main/install.sh | bash
  # Windows (PowerShell):
  # irm https://raw.githubusercontent.com/A-K-6/v2ray_scrapper/main/install.ps1 | iex
`, Version)
}

// ---------- flag helpers ----------

func flagValue(args []string, names []string, def string) string {
	for i := 0; i < len(args); i++ {
		for _, n := range names {
			if args[i] == n && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(args[i], n+"=") {
				return strings.TrimPrefix(args[i], n+"=")
			}
		}
	}
	return def
}

func flagBool(args []string, names []string) bool {
	for _, a := range args {
		for _, n := range names {
			if a == n {
				return true
			}
		}
	}
	return false
}

// ---------- serve ----------

func runServe(args []string) int {
	config, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		return 2
	}
	if v := flagValue(args, []string{"--addr", "--listen"}, ""); v != "" {
		config.ListenAddr = v
	}
	service, err := service.NewService(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		return 1
	}
	defer service.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	service.Start(ctx)
	server := &http.Server{Addr: config.ListenAddr, Handler: api.NewAPI(service), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Minute, IdleTimeout: 60 * time.Second}
	go func() {
		fmt.Printf("v2rays serving on http://%s  (swagger: /swagger)\n", config.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server failed: %v\n", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "graceful shutdown failed: %v\n", err)
	}
	return 0
}

// ---------- refresh ----------

func runRefresh(args []string) int {
	out := flagValue(args, []string{"--out", "-o"}, "")
	format := strings.ToLower(flagValue(args, []string{"--format", "-f"}, "base64"))
	all := flagBool(args, []string{"--all"})
	countries := flagValue(args, []string{"--country"}, "")
	limit := atoiOr(flagValue(args, []string{"--limit", "-n"}, "0"), 0)

	config, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		return 2
	}
	service, err := service.NewService(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		return 1
	}
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	fmt.Println("refreshing subscriptions (this may take a few minutes)...")
	// Run synchronously: trigger background update then wait for completion.
	if !service.TriggerUpdate(ctx) {
		fmt.Fprintln(os.Stderr, "another update is already in progress")
		return 1
	}
	for service.Processing() {
		time.Sleep(2 * time.Second)
	}
	servers := service.Cached(all)
	if len(servers) == 0 {
		fmt.Fprintln(os.Stderr, "refresh produced no working nodes (check SUB_URLS / network)")
		return 1
	}
	return exportServers(servers, format, countries, limit, out)
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

// ---------- get ----------

func runGet(args []string) int {
	out := flagValue(args, []string{"--out", "-o"}, "")
	format := strings.ToLower(flagValue(args, []string{"--format", "-f"}, "base64"))
	all := flagBool(args, []string{"--all"})
	countries := flagValue(args, []string{"--country"}, "")
	limit := atoiOr(flagValue(args, []string{"--limit", "-n"}, "0"), 0)

	config, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		return 2
	}
	state, err := store.NewStateStore(config.StatePath).Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read state: %v\n", err)
		return 1
	}
	servers := state.Working
	if all {
		// working already holds the full set; nothing extra to merge.
	}
	if len(servers) == 0 {
		fmt.Fprintf(os.Stderr, "cache is empty (%s). Run `v2rays refresh` first.\n", config.StatePath)
		return 1
	}
	return exportServers(servers, format, countries, limit, out)
}

func exportServers(servers []proxy.ProxyServer, format, countries string, limit int, out string) int {
	servers = proxy.FilterCountries(servers, countries)
	if limit > 0 && len(servers) > limit {
		servers = servers[:limit]
	}
	var content string
	switch format {
	case "raw", "text", "uri":
		lines := make([]string, 0, len(servers))
		for _, s := range servers {
			lines = append(lines, s.RawURI)
		}
		content = strings.Join(lines, "\n") + "\n"
	case "json":
		data, err := json.MarshalIndent(map[string]any{"count": len(servers), "servers": servers}, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			return 1
		}
		content = string(data) + "\n"
	default: // base64
		lines := make([]string, 0, len(servers))
		for _, s := range servers {
			lines = append(lines, s.RawURI)
		}
		content = base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n"))) + "\n"
	}
	if out == "" || out == "-" {
		fmt.Print(content)
	} else {
		if err := os.WriteFile(out, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
			return 1
		}
		fmt.Printf("wrote %d nodes (%s) to %s\n", len(servers), format, out)
	}
	return 0
}

// ---------- test ----------

func runTestCmd(args []string) int {
	sub := flagValue(args, []string{"--sub", "--url"}, "")
	file := flagValue(args, []string{"--file", "--content-file"}, "")
	content := flagValue(args, []string{"--content"}, "")
	target := flagValue(args, []string{"--target"}, "")
	maxDelay := atoiOr(flagValue(args, []string{"--max-delay"}, "0"), 0)
	limit := atoiOr(flagValue(args, []string{"--limit", "-n"}, "50"), 50)
	out := flagValue(args, []string{"--out", "-o"}, "")
	format := strings.ToLower(flagValue(args, []string{"--format", "-f"}, "raw"))
	// Positional shorthand: v2rays test <url-or-file>
	positional := []string{}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
		}
	}
	if sub == "" && len(positional) > 0 {
		cand := positional[0]
		if strings.HasPrefix(cand, "http://") || strings.HasPrefix(cand, "https://") {
			sub = cand
		} else if _, err := os.Stat(cand); err == nil {
			file = cand
		} else {
			content = cand
		}
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", file, err)
			return 1
		}
		content = string(data)
	}
	var urls []string
	if sub != "" {
		urls = []string{sub}
	}
	if len(urls) == 0 && strings.TrimSpace(content) == "" {
		fmt.Fprintln(os.Stderr, "usage: v2rays test --sub <url> | --file <path> [--target <url>] [--limit 50]")
		return 2
	}
	config, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
		return 2
	}
	service, err := service.NewService(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load state: %v\n", err)
		return 1
	}
	defer service.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	servers, err := service.TestCustom(ctx, urls, content, target, maxDelay, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "test failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "%d working nodes\n", len(servers))
	return exportServers(servers, format, "", limit, out)
}

// ---------- sources ----------

func openService() (*service.Service, error) {
	config, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	return service.NewService(config)
}

func runSources(args []string) int {
	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		svc, err := openService()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load: %v\n", err)
			return 1
		}
		defer svc.Close()
		for i, u := range svc.Subscriptions() {
			fmt.Printf("%d. %s\n", i+1, u)
		}
		return 0
	}
	svc, err := openService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return 1
	}
	defer svc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch args[0] {
	case "add":
		urls := config.CleanStrings(args[1:])
		if len(urls) == 0 {
			fmt.Fprintln(os.Stderr, "usage: v2rays sources add <url...>")
			return 2
		}
		for _, u := range urls {
			if !proxy.ValidTargetURL(u) {
				fmt.Fprintf(os.Stderr, "not an http(s) URL: %s\n", u)
				return 2
			}
		}
		if err := svc.AddSubscriptions(ctx, urls); err != nil {
			fmt.Fprintf(os.Stderr, "add: %v\n", err)
			return 1
		}
		fmt.Printf("added %d source(s), total %d\n", len(urls), len(svc.Subscriptions()))
		return 0
	case "rm", "remove", "del", "delete":
		urls := config.CleanStrings(args[1:])
		if len(urls) == 0 {
			fmt.Fprintln(os.Stderr, "usage: v2rays sources rm <url...>")
			return 2
		}
		if err := svc.RemoveSubscriptions(ctx, urls); err != nil {
			fmt.Fprintf(os.Stderr, "remove: %v\n", err)
			return 1
		}
		fmt.Printf("removed. %d source(s) remain.\n", len(svc.Subscriptions()))
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: v2rays sources [list|add|rm] ...")
		return 2
	}
}

// ---------- sites ----------

func runSites(args []string) int {
	svc, err := openService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return 1
	}
	defer svc.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if len(args) == 0 || args[0] == "list" || args[0] == "ls" {
		for i, s := range svc.Sites() {
			fmt.Printf("%d. %s -> %s\n", i+1, s.URL, s.Filename)
		}
		return 0
	}
	switch args[0] {
	case "add":
		url := flagValue(args[1:], []string{"--url"}, "")
		filename := flagValue(args[1:], []string{"--file", "--filename"}, "")
		if url == "" && len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			url = args[1]
		}
		if len(args) > 2 && filename == "" && !strings.HasPrefix(args[2], "-") {
			filename = args[2]
		}
		if !proxy.ValidTargetURL(url) {
			fmt.Fprintln(os.Stderr, "usage: v2rays sites add <url> [filename]")
			return 2
		}
		if filename == "" {
			filename = config.SitesFromURLs([]string{url})[0].Filename
		}
		enabled := true
		if err := svc.PutSite(ctx, config.SiteConfig{URL: url, Filename: filename, Enabled: &enabled}); err != nil {
			fmt.Fprintf(os.Stderr, "add site: %v\n", err)
			return 1
		}
		fmt.Printf("site %s -> %s stored\n", url, filename)
		return 0
	case "rm", "remove", "del", "delete":
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		if !proxy.ValidTargetURL(target) {
			fmt.Fprintln(os.Stderr, "usage: v2rays sites rm <url>")
			return 2
		}
		if err := svc.RemoveSite(ctx, target); err != nil {
			fmt.Fprintf(os.Stderr, "remove site: %v\n", err)
			return 1
		}
		fmt.Println("site removed")
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: v2rays sites [list|add|rm] ...")
		return 2
	}
}

// ---------- token ----------

func envFiles() []string {
	files := []string{}
	if _, err := os.Stat(".env"); err == nil {
		files = append(files, ".env")
	}
	p := filepath.Join(xdg.ConfigDir(), ".env")
	if p != ".env" {
		files = append(files, p)
	}
	return files
}

func runToken(args []string) int {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "show":
		tok := strings.TrimSpace(os.Getenv("MANAGEMENT_TOKEN"))
		if tok == "" {
			if t := readEnvToken(filepath.Join(xdg.ConfigDir(), ".env")); t != "" {
				tok = t
			} else if t := readEnvToken(".env"); t != "" {
				// Docker-workflow convenience fallback: the repo .env is
				// NOT auto-loaded for standalone runs (see loadDotEnvFiles).
				tok = t
				fmt.Fprintln(os.Stderr, "(from ./.env; standalone commands use ~/.config/v2rays/.env — run `v2rays token set <token>` to adopt it)")
			}
		}
		if tok == "" {
			fmt.Println("MANAGEMENT_TOKEN is not set. Run `v2rays token gen`.")
			return 1
		}
		fmt.Println(tok)
		return 0
	case "gen", "generate", "new":
		tok := randomToken()
		if err := writeEnvToken(tok); err != nil {
			fmt.Fprintf(os.Stderr, "generate: %v\n", err)
			return 1
		}
		fmt.Println("generated and saved MANAGEMENT_TOKEN:")
		fmt.Println(tok)
		return 0
	case "set":
		tok := ""
		if len(args) > 1 {
			tok = strings.TrimSpace(args[1])
		}
		if tok == "" {
			fmt.Fprintln(os.Stderr, "usage: v2rays token set <token>")
			return 2
		}
		if err := writeEnvToken(tok); err != nil {
			fmt.Fprintf(os.Stderr, "set: %v\n", err)
			return 1
		}
		fmt.Println("MANAGEMENT_TOKEN saved")
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: v2rays token [show|gen|set]")
		return 2
	}
}

func randomToken() string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "changeme-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	return hex.EncodeToString(buf)
}

func readEnvToken(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MANAGEMENT_TOKEN=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "MANAGEMENT_TOKEN="))
		}
	}
	return ""
}

func writeEnvToken(tok string) error {
	if err := upsertXDGEnv("MANAGEMENT_TOKEN", tok, 0600); err != nil {
		return err
	}
	os.Setenv("MANAGEMENT_TOKEN", tok)
	return nil
}

// upsertXDGEnv writes KEY=VALUE into ~/.config/v2rays/.env (the file
// LoadConfig auto-loads), preserving other entries. The repo-local .env is
// Docker's file and is never touched by standalone commands.
func upsertXDGEnv(key, value string, perm os.FileMode) error {
	path := filepath.Join(xdg.ConfigDir(), ".env")
	if err := xdg.EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	lines := []string{}
	if data, err := os.ReadFile(path); err == nil {
		found := false
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), key+"=") && !found {
				lines = append(lines, key+"="+value)
				found = true
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(l), key+"=") {
				continue
			}
			lines = append(lines, l)
		}
		if !found {
			lines = append(lines, key+"="+value)
		}
	} else {
		lines = []string{key + "=" + value}
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), perm); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "(saved to %s)\n", path)
	return nil
}

// ---------- config ----------

func runConfigCmd(args []string) int {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "init", "setup":
		return runConfigInit(args[1:])
	case "path":
		fmt.Printf("config: %s\n", config.DefaultConfigPath())
		fmt.Printf("state:  %s\n", mustLoadStatePath())
		fmt.Printf("data:   %s\n", xdg.DataDir())
		fmt.Printf("config-dir: %s\n", xdg.ConfigDir())
		return 0
	default: // show
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid configuration: %v\n", err)
			return 2
		}
		redacted := cfg
		if redacted.ManagementToken != "" {
			redacted.ManagementToken = "<set>"
		}
		if redacted.GitToken != "" {
			redacted.GitToken = "<set>"
		}
		data, _ := json.MarshalIndent(map[string]any{
			"listen":      redacted.ListenAddr,
			"sources":     len(redacted.SubscriptionURLs),
			"sites":       len(redacted.Sites),
			"sing_box":    redacted.SingBoxPath,
			"state":       redacted.StatePath,
			"geoip":       redacted.GeoIPPath,
			"config_file": redacted.ConfigPath,
			"redis":       redacted.RedisURL != "",
			"git_push":    redacted.GitPushEnabled,
			"git_repo":    redacted.GitRepoURL,
		}, "", "  ")
		fmt.Println(string(data))
		return 0
	}
}

func mustLoadStatePath() string {
	c, err := config.LoadConfig()
	if err != nil {
		return ""
	}
	return c.StatePath
}

func runConfigInit(args []string) int {
	force := flagBool(args, []string{"--force"})
	nonInteractive := flagBool(args, []string{"--non-interactive", "-y"})
	if os.Getenv("V2RAYS_NON_INTERACTIVE") == "1" {
		nonInteractive = true
	}
	if err := xdg.EnsureDir(xdg.ConfigDir()); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		return 1
	}
	if err := xdg.EnsureDir(xdg.DataDir()); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		return 1
	}
	if err := xdg.EnsureDir(filepath.Join(xdg.DataDir(), "bin")); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		return 1
	}
	cfgPath := config.DefaultConfigPath()
	if _, err := os.Stat(cfgPath); err == nil && !force {
		fmt.Printf("keeping existing %s (use --force to overwrite)\n", cfgPath)
	} else {
		sample := "config.yaml.sample"
		var data []byte
		if d, err := os.ReadFile(sample); err == nil {
			data = d
		} else {
			data = []byte("sites:\n  - url: https://www.google.com\n    filename: google_valid.txt\n    enabled: true\n")
		}
		if err := xdg.EnsureDir(filepath.Dir(cfgPath)); err == nil {
			if err := os.WriteFile(cfgPath, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "write config: %v\n", err)
				return 1
			}
			fmt.Printf("created %s\n", cfgPath)
		}
	}
	in := bufio.NewReader(os.Stdin)
	ask := func(label, def string) string {
		if nonInteractive {
			return def
		}
		fmt.Printf("%s [%s]: ", label, def)
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}
	port := ask("API listen port", "8084")
	sources := ask("Subscription URL (blank = keep defaults)", "")
	if strings.TrimSpace(sources) != "" {
		if err := upsertXDGEnv("SUB_URLS", strings.TrimSpace(sources), 0600); err != nil {
			fmt.Fprintf(os.Stderr, "save SUB_URLS: %v\n", err)
			return 1
		}
		os.Setenv("SUB_URLS", strings.TrimSpace(sources))
	}
	if p := strings.TrimSpace(port); p != "" {
		if _, err := strconv.Atoi(p); err != nil {
			fmt.Fprintf(os.Stderr, "invalid port %q, keeping default 8084\n", p)
		} else if p != "8084" {
			if err := upsertXDGEnv("LISTEN_ADDR", "0.0.0.0:"+p, 0600); err != nil {
				fmt.Fprintf(os.Stderr, "save LISTEN_ADDR: %v\n", err)
				return 1
			}
			os.Setenv("LISTEN_ADDR", "0.0.0.0:"+p)
		}
	}
	// Ensure registry/state exist.
	config, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}
	svc, err := service.NewService(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init service: %v\n", err)
		return 1
	}
	svc.Close()
	// Token (standalone reads process env + XDG file; repo .env is docker-only).
	hasToken := strings.TrimSpace(os.Getenv("MANAGEMENT_TOKEN")) != "" ||
		readEnvToken(filepath.Join(xdg.ConfigDir(), ".env")) != ""
	if !hasToken {
		if nonInteractive || ask("Generate MANAGEMENT_TOKEN now? (Y/n)", "Y") != "n" {
			_ = writeEnvToken(randomToken())
		}
	}
	fmt.Println("init complete. Next: `v2rays doctor`, `v2rays refresh`, `v2rays serve`")
	return 0
}

// ---------- doctor ----------

func runDoctor(args []string) int {
	_ = args
	ok := true
	fail := func(format string, a ...any) {
		ok = false
		fmt.Printf("  ✗ "+format+"\n", a...)
	}
	pass := func(format string, a ...any) {
		fmt.Printf("  ✓ "+format+"\n", a...)
	}
	fmt.Println("v2rays doctor:")
	config, err := config.LoadConfig()
	if err != nil {
		fail("config: %v", err)
		return 1
	}
	pass("config ok (sources=%d sites=%d)", len(config.SubscriptionURLs), len(config.Sites))
	sb := singbox.ResolveSingBox(config.SingBoxPath)
	if singbox.IsExecutable(sb) {
		pass("sing-box: %s", sb)
	} else {
		fail("sing-box missing at %s (run `v2rays config init` with network, or set SING_BOX_PATH)", sb)
	}
	if _, err := os.Stat(config.GeoIPPath); err == nil {
		pass("geoip: %s", config.GeoIPPath)
	} else {
		fmt.Printf("  ! geoip missing at %s (country will show UN; optional)\n", config.GeoIPPath)
	}
	if config.RedisURL != "" {
		pass("redis configured")
	} else {
		pass("standalone registry: %s", store.RegistryPath())
	}
	if _, err := os.Stat(config.StatePath); err == nil {
		pass("state: %s", config.StatePath)
	} else {
		fmt.Printf("  ! no state yet at %s (run `v2rays refresh`)\n", config.StatePath)
	}
	if strings.TrimSpace(config.ManagementToken) != "" {
		pass("management token set")
	} else {
		fmt.Println("  ! MANAGEMENT_TOKEN empty (management API disabled; run `v2rays token gen`)")
	}
	if !ok {
		return 1
	}
	state, _ := store.NewStateStore(config.StatePath).Load()
	fmt.Printf("cached working nodes: %d\n", len(state.Working))
	return 0
}

// ---------- TUI (stdlib menu) ----------

func runTUI() int {
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Println()
		fmt.Println("══ v2rays ══════════════════════")
		fmt.Println(" 1. Check health (doctor)")
		fmt.Println(" 2. First-time setup (config init)")
		fmt.Println(" 3. Refresh now (scrape + test)")
		fmt.Println(" 4. Show cached nodes (get)")
		fmt.Println(" 5. Manage sources")
		fmt.Println(" 6. Manage sites")
		fmt.Println(" 7. Token & Git setup")
		fmt.Println(" 8. Start server (serve)")
		fmt.Println(" 9. Quit")
		fmt.Print("choose [1-9]: ")
		line, _ := in.ReadString('\n')
		choice := strings.TrimSpace(line)
		switch choice {
		case "1":
			_ = runDoctor(nil)
		case "2":
			_ = runConfigCmd([]string{"init"})
		case "3":
			fmt.Print("output file (blank = print base64 to screen): ")
			out, _ := in.ReadString('\n')
			out = strings.TrimSpace(out)
			a := []string{}
			if out != "" {
				a = append(a, "--out", out)
			}
			_ = runRefresh(a)
		case "4":
			_ = runGet([]string{"--format", "raw", "--limit", "25"})
		case "5":
			tuiSources(in)
		case "6":
			tuiSites(in)
		case "7":
			tuiTokenGit(in)
		case "8":
			return runServe(nil)
		case "9", "q", "quit", "exit":
			return 0
		default:
			fmt.Println("pick 1-9")
		}
	}
}

func tuiSources(in *bufio.Reader) {
	svc, err := openService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return
	}
	defer svc.Close()
	subs := svc.Subscriptions()
	sort.Strings(subs)
	fmt.Println("\nsources:")
	for i, u := range subs {
		fmt.Printf(" %d. %s\n", i+1, u)
	}
	fmt.Print("[a]dd / [r]emove / [enter] back: ")
	line, _ := in.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "a", "add":
		fmt.Print("URL: ")
		u, _ := in.ReadString('\n')
		u = strings.TrimSpace(u)
		if !proxy.ValidTargetURL(u) {
			fmt.Println("not an http(s) URL")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := svc.AddSubscriptions(ctx, []string{u}); err != nil {
			fmt.Fprintf(os.Stderr, "add: %v\n", err)
			return
		}
		fmt.Println("added")
	case "r", "rm", "remove":
		fmt.Print("URL to remove: ")
		u, _ := in.ReadString('\n')
		u = strings.TrimSpace(u)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := svc.RemoveSubscriptions(ctx, []string{u}); err != nil {
			fmt.Fprintf(os.Stderr, "remove: %v\n", err)
			return
		}
		fmt.Println("removed")
	}
}

func tuiSites(in *bufio.Reader) {
	svc, err := openService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return
	}
	defer svc.Close()
	for i, s := range svc.Sites() {
		fmt.Printf(" %d. %s -> %s\n", i+1, s.URL, s.Filename)
	}
	fmt.Print("[a]dd / [r]emove / [enter] back: ")
	line, _ := in.ReadString('\n')
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "a", "add":
		fmt.Print("site URL: ")
		u, _ := in.ReadString('\n')
		u = strings.TrimSpace(u)
		if !proxy.ValidTargetURL(u) {
			fmt.Println("not an http(s) URL")
			return
		}
		enabled := true
		fn := config.SitesFromURLs([]string{u})[0].Filename
		if err := svc.PutSite(ctx, config.SiteConfig{URL: u, Filename: fn, Enabled: &enabled}); err != nil {
			fmt.Fprintf(os.Stderr, "add: %v\n", err)
			return
		}
		fmt.Printf("added -> %s\n", fn)
	case "r", "rm", "remove":
		fmt.Print("site URL to remove: ")
		u, _ := in.ReadString('\n')
		u = strings.TrimSpace(u)
		if err := svc.RemoveSite(ctx, u); err != nil {
			fmt.Fprintf(os.Stderr, "remove: %v\n", err)
			return
		}
		fmt.Println("removed")
	}
}

func tuiTokenGit(in *bufio.Reader) {
	fmt.Println("\n1. show token  2. generate token  3. show git settings  [enter] back")
	fmt.Print("choose: ")
	line, _ := in.ReadString('\n')
	switch strings.TrimSpace(line) {
	case "1":
		_ = runToken([]string{"show"})
	case "2":
		_ = runToken([]string{"gen"})
	case "3":
		c, err := config.LoadConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			return
		}
		fmt.Printf("push=%v repo=%s branch=%s file=%s\n", c.GitPushEnabled, c.GitRepoURL, c.GitBranch, c.GitFilename)
		fmt.Println("set via env: GITHUB_PUSH_ENABLED, GITHUB_REPO_URL, GITHUB_TOKEN, GITHUB_BRANCH")
	}
}
