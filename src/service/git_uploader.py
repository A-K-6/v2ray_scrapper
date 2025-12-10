import os
import subprocess
import sys

class GitUploader:
    def __init__(self, repo_url: str, token: str, user_name: str, user_email: str, repo_dir: str, branch: str = "main"):
        # Embed the token into the URL for authentication
        if token and "@" not in repo_url:
             self.repo_url = repo_url.replace("https://", f"https://{token}@")
        else:
             self.repo_url = repo_url
             
        self.user_name = user_name
        self.user_email = user_email
        self.branch = branch
        self.repo_dir = repo_dir

    def _run_command(self, command: list, cwd: str = None):
        try:
            result = subprocess.run(
                command, 
                cwd=cwd, 
                check=True, 
                stdout=subprocess.PIPE, 
                stderr=subprocess.PIPE, 
                text=True
            )
            return result.stdout.strip()
        except subprocess.CalledProcessError as e:
            # Don't print the error if it's just a "nothing to commit" status
            if "nothing to commit" not in e.stderr:
                print(f"Git command failed: {' '.join(command)}\nError: {e.stderr}", file=sys.stderr)
            raise

    def setup_repo(self):
        """Clones the repo if it doesn't exist, or pulls if it does."""
        if not os.path.exists(self.repo_dir):
            print(f"Cloning repository to {self.repo_dir}...")
            # Ensure parent dir exists
            parent_dir = os.path.dirname(self.repo_dir)
            if parent_dir and not os.path.exists(parent_dir):
                os.makedirs(parent_dir, exist_ok=True)
                
            self._run_command(["git", "clone", "-b", self.branch, "--single-branch", self.repo_url, self.repo_dir])
            
            # Configure user identity
            self._run_command(["git", "config", "user.name", self.user_name], cwd=self.repo_dir)
            self._run_command(["git", "config", "user.email", self.user_email], cwd=self.repo_dir)
        else:
            # FIX: Pull latest changes (e.g., README updates) before doing anything else
            # We use --rebase to apply our local bot commits on top of remote changes
            try:
                # Ensure it is a git repo
                if not os.path.exists(os.path.join(self.repo_dir, ".git")):
                    print(f"Warning: {self.repo_dir} exists but is not a git repository. Cleaning up...", file=sys.stderr)
                    import shutil
                    shutil.rmtree(self.repo_dir)
                    self.setup_repo()
                    return

                self._run_command(["git", "pull", "--rebase", "origin", self.branch], cwd=self.repo_dir)
            except Exception as e:
                print(f"Warning: Git pull failed, attempting to reset. Error: {e}", file=sys.stderr)
                # Fallback: If rebase fails, hard reset to match remote (be careful, this discards local unpushed changes)
                try:
                    self._run_command(["git", "fetch", "origin", self.branch], cwd=self.repo_dir)
                    self._run_command(["git", "reset", "--hard", f"origin/{self.branch}"], cwd=self.repo_dir)
                except Exception as reset_err:
                     print(f"Critical: Git reset failed too. {reset_err}", file=sys.stderr)

    def update_file_and_push(self, filename: str, content: str):
        try:
            self.setup_repo()
            
            file_path = os.path.join(self.repo_dir, filename)
            
            # Write content to file
            with open(file_path, "w") as f:
                f.write(content)
            
            # Check status
            status = self._run_command(["git", "status", "--porcelain"], cwd=self.repo_dir)
            if not status:
                print(f"No changes to push for {filename}.")
                return

            print(f"Committing and pushing {filename}...")
            self._run_command(["git", "add", filename], cwd=self.repo_dir)
            self._run_command(["git", "commit", "-m", f"Auto-update {filename}"], cwd=self.repo_dir)
            self._run_command(["git", "push", "origin", self.branch], cwd=self.repo_dir)
            print(f"Push successful for {filename}!")
            
        except Exception as e:
            print(f"Failed to push to GitHub: {e}", file=sys.stderr)
