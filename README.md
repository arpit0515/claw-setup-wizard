# 🦞 claw-setup-wizard

A browser-based setup wizard for [PicoClaw](https://github.com/sipeed/picoclaw) and [OpenClaw](https://github.com/openclaw/openclaw) — runs on your Raspberry Pi or any Linux machine.

No JSON editing. No terminal juggling. Just open a browser and follow the steps.

---

## What it does

Walks you through the full setup in 5 steps:

1. **System Check** — detects your installation, shows disk/RAM/config status
2. **LLM Provider** — pick OpenRouter, Anthropic, Gemini or Groq, paste your key, validates it live
3. **Telegram** — step-by-step bot creation, token validation, real ping test
4. **Your Twin's Soul** — 8 questions that generate your `SOUL.md` personality file
5. **Launch** — installs a systemd service so your agent starts on boot

If you already have things configured, the wizard reads your existing config and shows what's set.

---

## Quick start

The easiest way — `install.sh` handles everything including Go installation if needed:

```bash
git clone https://github.com/arpit0515/claw-setup-wizard.git
cd claw-setup-wizard
bash install.sh
```

Then open **`http://YOUR_PI_IP:3000`** in any browser on your network.

The install script will:
- Detect your device architecture (arm64, armv6l, amd64)
- Install Go automatically if not present
- Build the binary
- Start the wizard

---

## Manual install

If you prefer to build yourself — requires [Go 1.21+](https://go.dev/dl/):

```bash
git clone https://github.com/arpit0515/claw-setup-wizard.git
cd claw-setup-wizard
go build .
./claw-setup
```

> The binary is fully self-contained — the entire UI is embedded inside it. No separate files or folders needed to run it.

---

## Requirements

- Raspberry Pi or any Linux machine
- PicoClaw installed (the wizard can install it for you if missing)
- Internet connection

---

## Supported providers

| Provider | Free tier | Notes |
|---|---|---|
| [OpenRouter](https://openrouter.ai/keys) | ✅ | Recommended — one key, access to hundreds of models including free ones |
| [Groq](https://console.groq.com) | ✅ | Very fast inference |
| [Gemini](https://aistudio.google.com/api-keys) | ✅ | Google models |
| [Anthropic](https://console.anthropic.com) | ❌ | Claude models direct |

### Free models on OpenRouter

OpenRouter gives you access to hundreds of free models with no credits required. The wizard fetches the live model list from your account and lets you filter to free-only models — no hardcoded list, always up to date.

---

## Why this exists

Setting up PicoClaw or OpenClaw requires editing raw JSON, creating Telegram bots manually, understanding provider APIs, and configuring systemd — all before you can say a single word to your agent.

This wizard removes all of that friction.

---

## Roadmap

- [ ] OpenClaw full support
- [ ] WhatsApp channel setup
- [ ] Voice configuration (Whisper + ElevenLabs)
- [ ] Model health check and auto-suggest

---

## License

MIT
