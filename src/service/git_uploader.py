import os
import subprocess
import sys
from loguru import logger

class GitUploader:
    def __init__(self, repo_url: str, token: str, user_name: str, user_email: str, repo_dir: str, branch: str = "main", settings=None, proxy_url: str = None):
        # Embed the token into the URL for authentication
        if token and "@" not in repo_url:
             self.repo_url = repo_url.replace("https://", f"https://{token}@")
        else:
             self.repo_url = repo_url
             
        self.user_name = user_name
        self.user_email = user_email
        self.branch = branch
        self.repo_dir = repo_dir
        self.settings = settings
        self.proxy_url = proxy_url

    def _run_command(self, command: list, cwd: str = None):
        # Prepare environment with proxy if configured
        env = os.environ.copy()
        final_command = list(command)
        
        if self.settings:
            # 1. Add SSL verification bypass if configured
            if self.settings.GIT_SSL_NO_VERIFY:
                 if final_command[0] == "git":
                      final_command.insert(1, "-c")
                      final_command.insert(2, "http.sslVerify=false")
            
            # 2. Add Explicit Proxy Configuration to the Git command
            # This is more reliable than environment variables for Git
            git_proxy = self.proxy_url
            if not git_proxy and self.settings.GIT_HTTP_PROXY:
                git_proxy = self.settings.GIT_HTTP_PROXY
            
            if git_proxy and final_command[0] == "git":
                # Find the position to insert -c arguments
                # If we already inserted sslVerify, we insert after that
                insert_idx = 1
                if self.settings.GIT_SSL_NO_VERIFY:
                    insert_idx = 3
                
                # SOCKS5 proxies need the socks5h:// prefix for Git to handle DNS resolution through the proxy
                proxy_to_use = git_proxy
                if proxy_to_use.startswith("socks5://"):
                    proxy_to_use = proxy_to_use.replace("socks5://", "socks5h://")

                final_command.insert(insert_idx, "-c")
                final_command.insert(insert_idx + 1, f"http.proxy={proxy_to_use}")
                final_command.insert(insert_idx + 2, "-c")
                final_command.insert(insert_idx + 3, f"https.proxy={proxy_to_use}")

            # 3. Fallback: also set environment variables for sub-processes or other tools
            if git_proxy:
                env["HTTP_PROXY"] = git_proxy
                env["HTTPS_PROXY"] = git_proxy
                env["ALL_PROXY"] = git_proxy
                env["http_proxy"] = git_proxy
                env["https_proxy"] = git_proxy
                env["all_proxy"] = git_proxy
        
        try:
            result = subprocess.run(
                final_command, 
                cwd=cwd, 
                check=True, 
                stdout=subprocess.PIPE, 
                stderr=subprocess.PIPE, 
                text=True,
                env=env,
                timeout=300 # 5 minute timeout for git operations
            )
            return result.stdout.strip()
        except subprocess.TimeoutExpired:
            logger.error(f"Git command timed out after 300s: {' '.join(final_command)}")
            raise
        except subprocess.CalledProcessError as e:
            # Don't print the error if it's just a "nothing to commit" status
            if "nothing to commit" not in e.stderr:
                logger.error(f"Git command failed: {' '.join(final_command)}\nError: {e.stderr}")
            raise

    def setup_repo(self):
        """Clones the repo if it doesn't exist, or pulls if it does."""
        if not os.path.exists(self.repo_dir):
            logger.info(f"Cloning repository to {self.repo_dir}...")
            # Ensure parent dir exists
            parent_dir = os.path.dirname(self.repo_dir)
            if parent_dir and not os.path.exists(parent_dir):
                os.makedirs(parent_dir, exist_ok=True)
            
            # Retry loop for clone
            # We use --depth 1 (shallow clone) to avoid downloading history of large repos (10k+ commits)
            max_retries = 2
            for attempt in range(max_retries):
                try:
                    logger.info(f"Cloning repository (Attempt {attempt+1}/{max_retries})...")
                    self._run_command(["git", "clone", "--depth", "1", "-b", self.branch, self.repo_url, self.repo_dir])
                    break
                except Exception as e:
                    logger.warning(f"Clone failed (attempt {attempt+1}/{max_retries}): {e}")
                    # Clean up failed clone attempt
                    if os.path.exists(self.repo_dir):
                        import shutil
                        shutil.rmtree(self.repo_dir)
                    
                    if attempt < max_retries - 1:
                        import time
                        wait_time = 5 * (attempt + 1)
                        logger.info(f"Waiting {wait_time}s before retrying...")
                        time.sleep(wait_time)
                    else:
                        logger.error("All clone attempts failed. Please check your network connection.")
                        raise

            # Configure user identity
            self._run_command(["git", "config", "user.name", self.user_name], cwd=self.repo_dir)
            self._run_command(["git", "config", "user.email", self.user_email], cwd=self.repo_dir)
        else:
            # FIX: Pull latest changes (e.g., README updates) before doing anything else
            # We use --rebase to apply our local bot commits on top of remote changes
            try:
                # Ensure it is a git repo
                if not os.path.exists(os.path.join(self.repo_dir, ".git")):
                    logger.warning(f"Warning: {self.repo_dir} exists but is not a git repository. Cleaning up...")
                    import shutil
                    shutil.rmtree(self.repo_dir)
                    self.setup_repo()
                    return

                # Fetch all remotes to ensure we know about all branches
                self._run_command(["git", "fetch", "--all"], cwd=self.repo_dir)

                # Checkout the target branch
                # This handles switching from 'main' to 'tci_ir' etc.
                # If branch exists locally, it switches. If not, it creates it tracking origin.
                try:
                    self._run_command(["git", "checkout", self.branch], cwd=self.repo_dir)
                except Exception:
                    # If checkout fails, maybe it doesn't exist locally. Try creating it.
                    self._run_command(["git", "checkout", "-b", self.branch, f"origin/{self.branch}"], cwd=self.repo_dir)

                self._run_command(["git", "pull", "--rebase", "origin", self.branch], cwd=self.repo_dir)
            except Exception as e:
                logger.warning(f"Warning: Git pull failed, attempting to reset. Error: {e}")
                # Fallback: If rebase fails, hard reset to match remote (be careful, this discards local unpushed changes)
                try:
                    self._run_command(["git", "fetch", "origin", self.branch], cwd=self.repo_dir)
                    self._run_command(["git", "reset", "--hard", f"origin/{self.branch}"], cwd=self.repo_dir)
                except Exception as reset_err:
                     logger.critical(f"Critical: Git reset failed too. {reset_err}")
                     logger.critical(f"Deleting corrupted repository at {self.repo_dir} to start fresh.")
                     import shutil
                     if os.path.exists(self.repo_dir):
                         shutil.rmtree(self.repo_dir)
                     self.setup_repo()

    def update_file_and_push(self, filename: str, content: str):
        self.setup_repo()
        
        file_path = os.path.join(self.repo_dir, filename)
        
        # Write content to file
        with open(file_path, "w") as f:
            f.write(content)
        
        # Check status
        status = self._run_command(["git", "status", "--porcelain"], cwd=self.repo_dir)
        if not status:
            logger.info(f"No changes to push for {filename}.")
            return

        logger.info(f"Committing {filename}...")
        self._run_command(["git", "add", filename], cwd=self.repo_dir)
        try:
            self._run_command(["git", "commit", "-m", f"Auto-update {filename}"], cwd=self.repo_dir)
        except Exception:
            # If commit fails (e.g. empty commit race condition), just return
            return

        # Retry loop for push
        max_retries = 3
        for attempt in range(max_retries):
            try:
                self._run_command(["git", "push", "origin", self.branch], cwd=self.repo_dir)
                logger.info(f"Push successful for {filename}!")
                return
            except Exception as e:
                logger.warning(f"Push failed (attempt {attempt+1}/{max_retries}): {e}")
                if attempt < max_retries - 1:
                    logger.info("Retrying fetch and rebase...")
                    try:
                        self._run_command(["git", "pull", "--rebase", "origin", self.branch], cwd=self.repo_dir)
                    except Exception as rebase_err:
                        logger.error(f"Rebase failed during retry: {rebase_err}")
                        # If rebase fails here, we might be in a bad state, but let's try next iteration or fail
                        pass
        
        logger.error(f"Failed to push {filename} after {max_retries} attempts.")
