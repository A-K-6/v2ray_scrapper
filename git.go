package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (s *Service) publishIntegrations(ctx context.Context, working []ProxyServer) {
	updates := make(map[string]string)
	if s.config.GitMainPushEnabled {
		updates[s.config.GitFilename] = subscriptionText(working)
	}
	if s.config.GitSitePushEnabled {
		for _, site := range s.config.Sites {
			servers, err := s.SiteSpecific(ctx, site.URL)
			if err == nil {
				updates[site.Filename] = subscriptionText(servers)
			}
		}
	}
	if len(updates) == 0 {
		return
	}
	if err := pushFiles(ctx, s.config, updates); err != nil {
		slog.Error("git publish failed", "error", err)
	}
}

func pushFiles(parent context.Context, c Config, updates map[string]string) error {
	if c.GitRepoURL == "" {
		return fmt.Errorf("GITHUB_REPO_URL is required")
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	if _, err := os.Stat(filepath.Join(c.GitRepoDir, ".git")); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(c.GitRepoDir), 0755); err != nil {
			return err
		}
		if err = runGit(ctx, c, "", "clone", "--depth", "1", "--branch", c.GitBranch, c.GitRepoURL, c.GitRepoDir); err != nil {
			return err
		}
	}
	if err := runGit(ctx, c, c.GitRepoDir, "pull", "--rebase", "origin", c.GitBranch); err != nil {
		return err
	}
	for name, content := range updates {
		clean, err := safeOutputName(name)
		if err != nil {
			return err
		}
		path := filepath.Join(c.GitRepoDir, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
		if err := runGit(ctx, c, c.GitRepoDir, "add", "--", clean); err != nil {
			return err
		}
	}
	_ = runGit(ctx, c, c.GitRepoDir, "config", "user.name", c.GitUser)
	_ = runGit(ctx, c, c.GitRepoDir, "config", "user.email", c.GitEmail)
	if err := runGit(ctx, c, c.GitRepoDir, "diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	if err := runGit(ctx, c, c.GitRepoDir, "commit", "-m", "Auto-update: "+time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return runGit(ctx, c, c.GitRepoDir, "push", "origin", c.GitBranch)
}

func safeOutputName(name string) (string, error) {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe output filename %q", name)
	}
	return clean, nil
}

func runGit(ctx context.Context, c Config, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = os.Environ()
	if c.GitToken != "" {
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + c.GitToken))
		cmd.Env = append(cmd.Env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Basic "+auth)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}
func subscriptionText(servers []ProxyServer) string {
	lines := make([]string, 0, len(servers))
	for _, server := range servers {
		lines = append(lines, server.RawURI)
	}
	return strings.Join(lines, "\n")
}
