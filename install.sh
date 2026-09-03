#!/bin/sh
# v2rays installer: curl -fsSL .../install.sh | bash
# Installs the latest (or pinned) v2rays binary to ~/.local/bin or /usr/local/bin.
set -eu

REPO="${V2RAYS_REPO:-A-K-6/v2ray_scrapper}"
# Source repo for the code; release artifacts live here. Override with V2RAYS_REPO.
# If your code repo differs, export V2RAYS_REPO=owner/repo before running.
VERSION="${V2RAYS_VERSION:-latest}"
INSTALL_DIR="${V2RAYS_INSTALL_DIR:-}"
BIN_NAME="v2rays"

log() { printf '%s\n' "$*" >&2; }

uname_os() {
  case "$(uname -s)" in
    Linux*) echo linux ;;
    Darwin*) echo darwin ;;
    *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

uname_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo amd64 ;;
    arm64|aarch64) echo arm64 ;;
    *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
  esac
}

OS=$(uname_os)
ARCH=$(uname_arch)

if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ]; then
    INSTALL_DIR=/usr/local/bin
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

resolve_version() {
  if [ "$VERSION" != "latest" ]; then
    printf '%s' "$VERSION"
    return
  fi
  # Try GitHub API; fall back to a static hint on failure.
  if command -v curl >/dev/null 2>&1; then
    tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | grep -m1 '"tag_name"' | cut -d'"' -f4 || true)
    if [ -n "${tag:-}" ]; then
      printf '%s' "$tag"
      return
    fi
  fi
  log "could not resolve latest release; set V2RAYS_VERSION=vX.Y.Z explicitly"
  exit 1
}

TAG=$(resolve_version)
ARCHIVE="v2rays-${TAG#v}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

log "downloading $URL"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$tmp/$ARCHIVE"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$tmp/$ARCHIVE" "$URL"
else
  log "need curl or wget"
  exit 1
fi

tar -xzf "$tmp/$ARCHIVE" -C "$tmp"
mkdir -p "$INSTALL_DIR"
mv "$tmp/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    log "add to PATH: export PATH=\"$INSTALL_DIR:\$PATH\""
    for rc in "$HOME/.bashrc" "$HOME/.zshrc"; do
      if [ -f "$rc" ] && ! grep -q "$INSTALL_DIR" "$rc" 2>/dev/null; then
        printf '\nexport PATH="%s:$PATH"\n' "$INSTALL_DIR" >>"$rc"
      fi
    done
    export PATH="$INSTALL_DIR:$PATH"
    ;;
esac

log "installed $INSTALL_DIR/$BIN_NAME ($TAG)"
"$INSTALL_DIR/$BIN_NAME" version || true
log "next: v2rays config init && v2rays doctor && v2rays tui"
