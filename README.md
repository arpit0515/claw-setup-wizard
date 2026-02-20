# 🦞 claw-setup-wizard

A browser-based setup wizard for [PicoClaw](https://github.com/sipeed/picoclaw) and [OpenClaw](https://github.com/openclaw/openclaw) - runs on your Raspberry Pi or any Linux machine.

No JSON editing. No terminal juggling. Just open a browser and follow the steps.

---

## What it does

Walks you through the full setup in 5 steps:

1. **System Check** - detects your installation, shows disk/RAM/config status
2. **LLM Provider** - pick OpenRouter, Anthropic, Gemini or Groq, paste your key, validates it live
3. **Telegram** - step-by-step bot creation, token validation, real ping test
4. **Your Twin's Soul** - 8 questions that generate your `SOUL.md` personality file
5. **Launch** - installs a systemd service so your agent starts on boot

If you already have things configured, the wizard reads your existing config and shows what's set.

---

## Quick start

```bash
# Download the binary for your platform
wget https://github.com/arpit0515/claw-setup-wizard/releases/latest/download/claw-setup-linux-arm64

# Make it executable
chmod +x claw-setup-linux-arm64

# Run it
./claw-setup-linux-arm64
```

Then open **`http://YOUR_PI_IP:3000`** in any browser on your network.

---

## Build from source

Requires [Go 1.21+](https://go.dev/dl/)

```bash
git clone https://github.com/arpit0515/claw-setup-wizard.git
cd claw-setup-wizard
go build .
./claw-setup
```

---

## Requirements

- Raspberry Pi or any Linux machine
- PicoClaw or OpenClaw installed
- Internet connection (for API key validation)

---

## Supported providers

| Provider | Free tier | Notes |
|---|---|---|
| [OpenRouter](https://openrouter.ai/keys) | ✅ | Recommended - one key, all models |
| [Groq](https://console.groq.com) | ✅ | Very fast |
| [Gemini](https://aistudio.google.com/api-keys) | ✅ | Google models |
| [Anthropic](https://console.anthropic.com) | ❌ | Claude models direct |

---

## Why this exists

Setting up PicoClaw or OpenClaw requires editing raw JSON, creating Telegram bots manually, understanding provider APIs, and configuring systemd - all before you can say a single word to your agent.

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
