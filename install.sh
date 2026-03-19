#!/bin/bash
set -e
REPO_DIR="$PWD"
LOG_FILE="$REPO_DIR/claw-setup-install.log"
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
  log "⚠  Not a git repo — cloning fresh copy from $GITHUB_REPO..."
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
      log "⏭  Skipping startup autorun — run install.sh again anytime to set it up"
      ;;
  esac
fi

# ── Verify pre-built binary ───────────────────────────────────────────────────
if [ ! -f "$REPO_DIR/claw-setup" ]; then
  log "❌ claw-setup binary not found in $REPO_DIR"
  log "   Make sure you cloned the full repo including the binary."
  exit 1
fi
chmod +x "$REPO_DIR/claw-setup"
log "✓ claw-setup binary ready"

# ── Start ─────────────────────────────────────────────────────────────────────
LOCAL_IP=$(hostname -I 2>/dev/null | awk '{print $1}' || ipconfig getifaddr en0)
log ""
log "================================"
log "✅ Ready — open in your browser:"
log "   👉 http://$LOCAL_IP:3000"
log "================================"
log ""
exec "$REPO_DIR/claw-setup"
