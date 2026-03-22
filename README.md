# 🦞 claw-setup-wizard

A browser-based setup wizard for [PicoClaw](https://github.com/sipeed/picoclaw) — runs on your Raspberry Pi or any Linux/macOS machine.

No JSON editing. No terminal juggling. Just open a browser and follow the steps.

---

## What it does

Walks you through the full setup in 6 steps:

1. **System Check** — detects your installation, shows disk/RAM/config status, installs PicoClaw if missing, checks for updates
2. **LLM Provider** — pick OpenRouter, Anthropic, Gemini or Groq, paste your key, validates it live, fetches available models
3. **Telegram** — step-by-step bot creation, token validation, real ping test end-to-end
4. **Your Twin's Soul** — 8 questions that generate your `SOUL.md` personality file, saved directly to the Pi
5. **Launch** — installs a systemd service (Linux) or launchd agent (macOS) so your agent starts on boot automatically
6. **Connected Tools** — connect Gmail and Google Calendar via OAuth2, install and activate the Weather tool

If you already have things configured, the wizard reads your existing config and shows what's already set.

---

## Quick start

### One-line install (recommended)

```bash
curl -fsSL https://claw-tools.dev/wizard/install.sh | bash
```

Then open **`http://YOUR_PI_IP:3000`** in any browser on your network.

This will:
1. Clone the repo to `~/.picoclaw/wizard/`
2. Install Go automatically if not present (detects arm64, armv6l, amd64)
3. Build the `claw-setup` binary from source
4. Optionally register autorun on boot
5. Start the wizard

### From source

```bash
git clone https://github.com/arpit0515/claw-setup-wizard.git
cd claw-setup-wizard
bash install.sh
```

Running `install.sh` from inside the repo directory will pull the latest changes, rebuild if needed, and start the wizard. Re-run it anytime to update.

---

## Manual install

Requires [Go 1.21+](https://go.dev/dl/):

```bash
git clone https://github.com/arpit0515/claw-setup-wizard.git
cd claw-setup-wizard
go build -o claw-setup .
./claw-setup
```

> The binary is fully self-contained — the entire UI (HTML, CSS, JS) is embedded inside it via Go's `embed`. No separate files needed to run it.

---

## Project structure

```
claw-setup-wizard/
├── main.go                 # HTTP server, route registration, static file serving
├── handlers.go             # Core API handlers (LLM, Telegram, Soul, service install)
├── weather_handlers.go     # Weather tool handlers (/api/weather/*)
├── oauth.go                # Google OAuth2 flow for Gmail + GCal
├── system.go               # System check, config read/write, PicoConfig struct
├── workspace.go            # Workspace file generation (SOUL.md, AGENTS.md, TOOLS.md, etc.)
├── soul.go                 # SOUL.md generation logic
├── validate.go             # LLM key + Telegram token validation
├── uninstall.go            # Full uninstall logic
├── picoclaw_update.go      # Version check + auto-update for PicoClaw
├── templates/
│   └── index.html          # Wizard UI (clean HTML, no inline JS/CSS)
└── static/
    ├── css/
    │   └── wizard.css      # All styles
    └── js/
        ├── core.js         # Shared state, navigation, alerts, QR modal, network bar
        ├── steps.js        # Steps 0–4 logic + uninstall modal
        ├── weather.js      # Weather tool UI logic (Step 6)
        ├── tools.js        # OAuth/Gmail/GCal UI logic (Step 6)
        ├── settings.js     # Settings page logic
        └── init.js         # Bootstrap (runs on page load)
```

---

## Requirements

- Raspberry Pi or any Linux/macOS machine
- PicoClaw installed (the wizard can install it for you if missing)
- Internet connection
- Go 1.21+ (only needed to build — the binary runs standalone)

---

## Connected Tools (Step 6)

### Gmail & Google Calendar

Connects to your Google account via OAuth2. Tokens are stored locally at `~/.picoclaw/tokens/` — nothing is sent to the cloud. Multi-account supported.

Once connected, your agent can:
- Summarise unread emails
- Fetch today's calendar events
- Deliver morning briefings via Telegram

Powered by [ClawTools](https://github.com/arpit0515/claw-tools.dev) — a separate repo of MCP tool connectors.

### Weather Tool

No API key required — powered by [Open-Meteo](https://open-meteo.com/) (free, open-source).

The wizard:
1. Geocodes your city name to lat/lon via Open-Meteo's geocoding API
2. Builds and installs `weather-mcp` — a local HTTP service on port 3104
3. Creates shell scripts at `~/.picoclaw/workspace/bin/` for the agent to call
4. Writes `~/.picoclaw/workspace/skills/claw-weather/SKILL.md` so PicoClaw knows to use your local service
5. Installs `claw-weather.service` (systemd) so it starts on boot
6. Restarts PicoClaw so it picks up the new skill immediately

After setup, ask your agent "what's the weather today?" — it will use your stored location and return a structured morning/afternoon/evening forecast with rain chance.

---

## Supported LLM providers

| Provider | Free tier | Notes |
|---|---|---|
| [OpenRouter](https://openrouter.ai/keys) | ✅ | Recommended — one key, hundreds of models including free ones |
| [Groq](https://console.groq.com) | ✅ | Very fast inference |
| [Gemini](https://aistudio.google.com/api-keys) | ✅ | Google models |
| [Anthropic](https://console.anthropic.com) | ❌ | Claude models direct |

### Free models on OpenRouter

OpenRouter gives access to hundreds of free models with no credits required. The wizard fetches the live model list and lets you filter to free-only — no hardcoded list, always up to date.

---

## Config files

Everything lives under `~/.picoclaw/`:

| Path | What it is |
|---|---|
| `config.json` | LLM provider, Telegram token, model config |
| `config/weather.json` | Stored weather location (lat, lon, label) |
| `tokens/<email>.enc` | OAuth tokens (AES-256 encrypted) |
| `workspace/SOUL.md` | Agent personality |
| `workspace/AGENTS.md` | Agent behaviour rules (auto-generated) |
| `workspace/TOOLS.md` | Tool reference (auto-generated) |
| `workspace/IDENTITY.md` | Who the agent is |
| `workspace/USER.md` | Owner preferences |
| `workspace/HEARTBEAT.md` | Scheduled task definitions |
| `workspace/MEMORY.md` | Accumulated agent memory |
| `workspace/bin/` | Tool binaries and shell scripts |
| `workspace/skills/` | PicoClaw skill definitions |

---

## How skills work

PicoClaw discovers capabilities via `SKILL.md` files in `~/.picoclaw/workspace/skills/`. Each skill is a markdown file with a YAML frontmatter block and plain-English instructions that tell the agent what the skill does and how to invoke it.

The wizard writes skills automatically when you install tools. You can also edit them directly to customise agent behaviour.

Example — `skills/claw-weather/SKILL.md`:
```markdown
---
name: claw-weather
description: "Get current weather and full day forecast..."
user-invocable: true
---

## Get full day forecast
exec: /home/pi/.picoclaw/workspace/bin/get_weather_forecast.sh
```

---

## Network bar

The wizard shows a network bar at the top with your Pi's local IP and a QR code. Scan it with your phone to open the wizard on mobile — useful when setting up a headless Pi.

---

## OS support

| OS | Service manager | Status |
|---|---|---|
| Raspberry Pi OS / Ubuntu | systemd | ✅ Full support |
| macOS | launchd | ✅ Full support |
| Other Linux | systemd | ✅ Should work |
| Windows | — | ❌ Not supported |

---

## Why this exists

Setting up PicoClaw requires editing raw JSON, creating Telegram bots manually, understanding provider APIs, and configuring systemd — all before you can say a single word to your agent.

This wizard removes all of that friction. It's developer infrastructure — self-hosted, privacy-first, and designed for the kind of person who wants to run their own AI agent without handing data to a hosted SaaS.

---

## Roadmap

- [ ] OpenClaw full support (compatible config, same SOUL.md structure)
- [ ] Morning briefing microservice setup (Gmail + GCal + Weather → Telegram)
- [ ] More ClawTools: Outlook/Exchange, SMS via Twilio
- [ ] WhatsApp channel setup
- [ ] Voice configuration (Whisper + ElevenLabs)
- [ ] Model health check and auto-suggest

---

## Related projects

- [PicoClaw](https://github.com/sipeed/picoclaw) — the AI agent runtime this wizard configures
- [ClawTools](https://github.com/arpit0515/claw-tools.dev) — MCP tool connectors (Gmail, GCal, Weather)

---

## License

MIT
