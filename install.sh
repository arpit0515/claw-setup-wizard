#!/bin/bash

set -e

REPO_DIR="$PWD"
LOG_FILE="$REPO_DIR/claw-setup-install.log"
GO_VERSION="1.23.4"

log() {
  echo "$1" | tee -a "$LOG_FILE"
}

log ""
log "🦞 claw-setup-wizard installer"
log "================================"
log "Started: $(date)"
log "Directory: $REPO_DIR"

# ── Detect architecture ───────────────────────────────────────────────────────

ARCH=$(uname -m)
case $ARCH in
  aarch64) GO_ARCH="arm64" ;;
  armv7l)  GO_ARCH="armv6l" ;;
  x86_64)  GO_ARCH="amd64" ;;
  *)
    log "❌ Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

log "✓ Architecture: $ARCH ($GO_ARCH)"

# ── Check / Install Go ────────────────────────────────────────────────────────

if command -v go &>/dev/null; then
  GO_INSTALLED=$(go version | awk '{print $3}' | sed 's/go//')
  log "✓ Go already installed: $GO_INSTALLED"
else
  log ""
  log "⬇  Go not found — installing Go $GO_VERSION in background..."

  GO_TARBALL="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  GO_URL="https://go.dev/dl/${GO_TARBALL}"
  TMP_DIR=$(mktemp -d)

  log "   Downloading $GO_URL"
  wget -q -O "$TMP_DIR/$GO_TARBALL" "$GO_URL" >> "$LOG_FILE" 2>&1

  log "   Extracting..."
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf "$TMP_DIR/$GO_TARBALL" >> "$LOG_FILE" 2>&1
  rm -rf "$TMP_DIR"

  export PATH=$PATH:/usr/local/go/bin

  grep -q '/usr/local/go/bin' ~/.bashrc || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
  grep -q '/usr/local/go/bin' ~/.profile || echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile

  log "✓ Go $GO_VERSION installed"
fi

# ── Build ─────────────────────────────────────────────────────────────────────

log ""
log "🔨 Building claw-setup..."

[ ! -f go.mod ] && /usr/local/go/bin/go mod init claw-setup >> "$LOG_FILE" 2>&1

/usr/local/go/bin/go build -o claw-setup . >> "$LOG_FILE" 2>&1
log "✓ Build complete"

# ── Start ─────────────────────────────────────────────────────────────────────

LOCAL_IP=$(hostname -I | awk '{print $1}')

log ""
log "================================"
log "✅ Ready — open in your browser:"
log "   👉 http://$LOCAL_IP:3000"
log "================================"
log ""

exec "$REPO_DIR/claw-setup"
