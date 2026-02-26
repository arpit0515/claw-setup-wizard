#!/bin/bash
set -e
REPO_DIR="$PWD"
LOG_FILE="$REPO_DIR/claw-setup-install.log"
GO_VERSION="1.23.4"
GITHUB_REPO="https://github.com/arpit0515/claw-setup-wizard"

log() {
  echo "$1" | tee -a "$LOG_FILE"
}
log ""
log "🦞 claw-setup-wizard installer"
log "================================"
log "Started: $(date)"
log "Directory: $REPO_DIR"

# ── (1) Pull latest from GitHub ───────────────────────────────────────────────
log ""
log "🔄 Checking for updates from source repo..."
if git -C "$REPO_DIR" rev-parse --is-inside-work-tree &>/dev/null; then
  BEFORE=$(git -C "$REPO_DIR" rev-parse HEAD)
  git -C "$REPO_DIR" fetch origin >> "$LOG_FILE" 2>&1
  git -C "$REPO_DIR" reset --hard origin/$(git -C "$REPO_DIR" rev-parse --abbrev-ref HEAD) >> "$LOG_FILE" 2>&1
  AFTER=$(git -C "$REPO_DIR" rev-parse HEAD)
  if [ "$BEFORE" != "$AFTER" ]; then
    log "✓ Updated to latest commit: ${AFTER:0:7} (was ${BEFORE:0:7})"
    log "  ↻  Restarting with updated script..."
    exec bash "$0" "$@"
  else
    log "✓ Already up to date (${AFTER:0:7})"
  fi
else
  log "⚠  Not a git repo - cloning fresh copy from $GITHUB_REPO..."
  TMP_CLONE=$(mktemp -d)
  git clone "$GITHUB_REPO" "$TMP_CLONE" >> "$LOG_FILE" 2>&1
  cp -r "$TMP_CLONE/." "$REPO_DIR/"
  rm -rf "$TMP_CLONE"
  log "✓ Cloned latest source"
fi

# ── (2) Autorun on boot in terminal (via ~/.bashrc profile trap) ──────────────
AUTORUN_MARKER="# claw-setup-autorun"
AUTORUN_CMD="bash $REPO_DIR/install.sh"
AUTORUN_BLOCK="$AUTORUN_MARKER
if [ \"\$(tty)\" = \"/dev/tty1\" ]; then
  $AUTORUN_CMD
fi"

if grep -q "$AUTORUN_MARKER" ~/.bashrc 2>/dev/null; then
  log ""
  log "✓ Startup autorun already registered, skipping"
else
  log ""
  printf "🔁 Would you like claw-setup to launch automatically on boot? [y/N]: "
  read -r AUTORUN_ANSWER </dev/tty
  case "$AUTORUN_ANSWER" in
    [yY][eE][sS]|[yY])
      echo "" >> ~/.bashrc
      echo "$AUTORUN_BLOCK" >> ~/.bashrc
      log "✓ Autorun registered in ~/.bashrc"

      # Auto-login on tty1 is only relevant on Linux with systemd
      if [[ "$(uname -s)" == "Linux" ]] && command -v systemctl &>/dev/null; then
        CURRENT_USER=$(whoami)
        AUTOLOGIN_CONF="/etc/systemd/system/getty@tty1.service.d/autologin.conf"
        if [ ! -f "$AUTOLOGIN_CONF" ]; then
          log "   Configuring auto-login for $CURRENT_USER on tty1 (requires sudo)..."
          sudo mkdir -p "$(dirname $AUTOLOGIN_CONF)"
          sudo bash -c "cat > $AUTOLOGIN_CONF" <<EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin $CURRENT_USER --noclear %I \$TERM
EOF
          sudo systemctl daemon-reload
          sudo systemctl restart getty@tty1
          log "✓ Auto-login configured for $CURRENT_USER"
        fi
      else
        log "   ℹ  Skipping tty1 auto-login (not a Linux/systemd system)"
      fi

      log "✓ Will launch automatically on next boot"
      ;;
    *)
      log "⏭  Skipping startup autorun - run install.sh again anytime to set it up"
      ;;
  esac
fi

# ── Detect architecture ───────────────────────────────────────────────────────
ARCH=$(uname -m)
case $ARCH in
  aarch64) GO_ARCH="arm64" ;;
  armv7l)  GO_ARCH="armv6l" ;;
  x86_64)  GO_ARCH="amd64" ;;
  arm64)   GO_ARCH="arm64" ;;  # macOS Apple Silicon
  *)
    log "❌ Unsupported architecture: $ARCH"
    exit 1
    ;;
esac
log ""
log "✓ Architecture: $ARCH ($GO_ARCH)"

# ── Check / Install Go ────────────────────────────────────────────────────────
# Prepend /usr/local/go/bin so we find a previously installed Go even when
# ~/.zshrc / ~/.bash_profile haven't been sourced in this shell session
export PATH=/usr/local/go/bin:$PATH

if command -v go &>/dev/null; then
  GO_INSTALLED=$(go version | awk '{print $3}' | sed 's/go//')
  log "✓ Go already installed: $GO_INSTALLED"
else
  log ""
  log "⬇  Go not found - installing Go $GO_VERSION..."

  OS=$(uname -s)
  if [[ "$OS" == "Darwin" ]]; then
    GO_TARBALL="go${GO_VERSION}.darwin-${GO_ARCH}.tar.gz"
  else
    GO_TARBALL="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  fi
  GO_URL="https://go.dev/dl/${GO_TARBALL}"
  TMP_DIR=$(mktemp -d)
  log "   Downloading $GO_URL"
  # Use curl on macOS (wget may not be present), wget on Linux
  if [[ "$OS" == "Darwin" ]]; then
    curl -fsSL -o "$TMP_DIR/$GO_TARBALL" "$GO_URL" >> "$LOG_FILE" 2>&1
  else
    wget -q -O "$TMP_DIR/$GO_TARBALL" "$GO_URL" >> "$LOG_FILE" 2>&1
  fi
  log "   Extracting to /usr/local/go..."
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$TMP_DIR/$GO_TARBALL" >> "$LOG_FILE" 2>&1
  rm -rf "$TMP_DIR"
  export PATH=$PATH:/usr/local/go/bin
  if [[ "$OS" == "Darwin" ]]; then
    # macOS defaults to zsh; update both shells just in case
    grep -q '/usr/local/go/bin' ~/.zshrc 2>/dev/null    || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
    grep -q '/usr/local/go/bin' ~/.bash_profile 2>/dev/null || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bash_profile
  else
    grep -q '/usr/local/go/bin' ~/.bashrc   || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    grep -q '/usr/local/go/bin' ~/.profile  || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
  fi
  log "✓ Go $GO_VERSION installed"
fi

# ── Build ─────────────────────────────────────────────────────────────────────
log ""
log "🔨 Building claw-setup..."
[ ! -f go.mod ] && /usr/local/go/bin/go mod init claw-setup >> "$LOG_FILE" 2>&1
/usr/local/go/bin/go build -o claw-setup . >> "$LOG_FILE" 2>&1
log "✓ Build complete"

# ── Start ─────────────────────────────────────────────────────────────────────
if [[ "$(uname -s)" == "Darwin" ]]; then
  LOCAL_IP=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || echo "localhost")
else
  LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
fi
[ -z "$LOCAL_IP" ] && LOCAL_IP="localhost"
log ""
log "================================"
log "✅ Ready - open in your browser:"
log "   👉 http://$LOCAL_IP:3000"
log "================================"
log ""
exec "$REPO_DIR/claw-setup"