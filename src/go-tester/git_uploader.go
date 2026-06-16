package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitPushRequest struct {
	RepoURL     string            `json:"repo_url"`
	RepoDir     string            `json:"repo_dir"`
	Branch      string            `json:"branch"`
	Token       string            `json:"token"`
	UserName    string            `json:"user_name"`
	UserEmail   string            `json:"user_email"`
	FileUpdates map[string]string `json:"file_updates"`
	ProxyURL    string            `json:"proxy_url"`
}

func handleGitPush(req GitPushRequest) error {
	// Embed token in URL if needed
	repoURL := req.RepoURL
	if req.Token != "" && !strings.Contains(repoURL, "@") {
		repoURL = strings.Replace(repoURL, "https://", "https://"+req.Token+"@", 1)
	}

	// 1. Setup Repo
	if err := setupRepo(req, repoURL); err != nil {
		return fmt.Errorf("setup repo failed: %v", err)
	}

	// 2. Update Files
	changed := false
	for filename, content := range req.FileUpdates {
		filePath := filepath.Join(req.RepoDir, filename)
		err := os.WriteFile(filePath, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %v", filename, err)
		}
		runGitCommand(req.RepoDir, req.ProxyURL, nil, "add", filename)
		changed = true
	}

	if !changed {
		log.Println("No changes to push.")
		return nil
	}

	// 3. Commit and Push
	msg := "Auto-update: " + time.Now().Format("2006-01-02 15:04:05")
	env := map[string]string{
		"GIT_AUTHOR_NAME":     req.UserName,
		"GIT_AUTHOR_EMAIL":    req.UserEmail,
		"GIT_COMMITTER_NAME":  req.UserName,
		"GIT_COMMITTER_EMAIL": req.UserEmail,
	}
	if err := runGitCommand(req.RepoDir, req.ProxyURL, env, "commit", "-m", msg); err != nil {
		// If nothing to commit, it's fine
		if !strings.Contains(strings.ToLower(err.Error()), "nothing to commit") {
			log.Printf("Commit failed (might be nothing to commit): %v", err)
		}
	}

	err := runGitCommand(req.RepoDir, req.ProxyURL, nil, "push", "origin", req.Branch)
	if err != nil {
		// Try rebase once
		log.Printf("Push failed, attempting rebase: %v", err)
		if err := runGitCommand(req.RepoDir, req.ProxyURL, nil, "pull", "--rebase", "origin", req.Branch); err == nil {
			return runGitCommand(req.RepoDir, req.ProxyURL, nil, "push", "origin", req.Branch)
		}
		return err
	}

	return nil
}

func setupRepo(req GitPushRequest, repoURL string) error {
	// Ensure directory exists
	if err := os.MkdirAll(req.RepoDir, 0755); err != nil {
		return fmt.Errorf("failed to create repo dir: %v", err)
	}

	// Check if it's a valid git repo
	isValid := false
	if _, err := os.Stat(filepath.Join(req.RepoDir, ".git")); err == nil {
		if err := runGitCommand(req.RepoDir, "", nil, "rev-parse", "--is-inside-work-tree"); err == nil {
			isValid = true
		}
	}

	if !isValid {
		log.Printf("Repository in %s is missing or invalid. Re-cloning...", req.RepoDir)
		if err := os.RemoveAll(req.RepoDir); err != nil {
			log.Printf("Warning: failed to remove broken repo dir: %v", err)
		}
		os.MkdirAll(req.RepoDir, 0755)
		err := runGitCommand(".", req.ProxyURL, nil, "clone", "--depth", "1", "-b", req.Branch, repoURL, req.RepoDir)
		if err != nil {
			// Fallback: clone without branch then checkout
			log.Printf("Cloning with branch failed, trying default clone: %v", err)
			err = runGitCommand(".", req.ProxyURL, nil, "clone", "--depth", "1", repoURL, req.RepoDir)
			if err != nil {
				return fmt.Errorf("clone failed: %v", err)
			}
			runGitCommand(req.RepoDir, req.ProxyURL, nil, "checkout", req.Branch)
		}
	}

	// Always set identity locally
	runGitCommand(req.RepoDir, req.ProxyURL, nil, "config", "user.name", req.UserName)
	runGitCommand(req.RepoDir, req.ProxyURL, nil, "config", "user.email", req.UserEmail)

	// Fetch and Reset to be safe
	if err := runGitCommand(req.RepoDir, req.ProxyURL, nil, "fetch", "origin", req.Branch); err == nil {
		// Only reset if origin/branch exists
		if err := runGitCommand(req.RepoDir, req.ProxyURL, nil, "rev-parse", "--verify", "origin/"+req.Branch); err == nil {
			if err := runGitCommand(req.RepoDir, req.ProxyURL, nil, "reset", "--hard", "origin/"+req.Branch); err != nil {
				log.Printf("Reset failed, proceeding with local state: %v", err)
			}
		}
	} else {
		log.Printf("Fetch failed: %v. Repository might be empty or branch missing.", err)
	}

	return nil
}

func runGitCommand(dir string, proxy string, extraEnv map[string]string, args ...string) error {
	finalArgs := []string{}
	if proxy != "" {
		finalArgs = append(finalArgs, "-c", "http.proxy="+proxy, "-c", "https.proxy="+proxy)
	}
	finalArgs = append(finalArgs, args...)

	cmd := exec.Command("git", finalArgs...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (Output: %s)", err, string(output))
	}
	return nil
}

func clearDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		os.RemoveAll(filepath.Join(dir, entry.Name()))
	}
}
