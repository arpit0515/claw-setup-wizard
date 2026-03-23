BINARY_NAME     := claw-setup
BUILD_DIR       := build
CMD_DIR         := .
VERSION         ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT      := $(shell git rev-parse --short=8 HEAD 2>/dev/null || echo "dev")
BUILD_TIME      := $(shell date +%FT%T%z)

# ── Secret injection ───────────────────────────────────────────────────────────
# Pass via environment or command line, e.g.:
#   CLAW_API_SECRET=abc123 make build-all
#   make build-all CLAW_API_SECRET=abc123
# Never hardcode the secret here — keep it out of git.
CLAW_API_SECRET ?=

ifeq ($(CLAW_API_SECRET),)
$(warning WARNING: CLAW_API_SECRET is not set. Binary will have no API secret.)
endif

LDFLAGS := -ldflags "-X main.clawAPISecret=$(CLAW_API_SECRET) \
                      -X main.Version=$(VERSION) \
                      -X main.GitCommit=$(GIT_COMMIT) \
                      -X main.BuildTime=$(BUILD_TIME) \
                      -s -w"

GO      ?= CGO_ENABLED=0 go
GOFLAGS ?= -v

# ── Targets ────────────────────────────────────────────────────────────────────

.PHONY: all build build-all build-pi build-pi-zero \
        build-linux-arm64 build-linux-arm build-linux-amd64 build-darwin-arm64 \
        install clean deps run help

## Default: build for current machine
all: build

## Build for the current OS/arch
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)"

## Build for all supported platforms
build-all: build-linux-arm64 build-linux-arm build-linux-amd64 build-darwin-arm64
	@echo "✓ All builds complete:"
	@ls -lh $(BUILD_DIR)/

## Build for Pi 4, Pi 5, Pi Zero 2W (64-bit OS)
build-linux-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

## Build for Pi Zero, Pi Zero W, Pi Zero 2W (32-bit OS)
build-linux-arm:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=6 $(GO) build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm"

## Build for x86 Linux (servers, VMs, desktops)
build-linux-amd64:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

## Build for macOS Apple Silicon (M1/M2/M3)
build-darwin-arm64:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64"

## Shortcut: build both Pi binaries (arm64 + arm)
build-pi: build-linux-arm64 build-linux-arm
	@echo "✓ Pi builds ready (arm64 + arm)"

## Alias for Pi Zero specifically (arm only)
build-pi-zero: build-linux-arm

## Install binary to /usr/local/bin (current arch build)
install: build
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "✓ Installed to /usr/local/bin/$(BINARY_NAME)"

## Download dependencies
deps:
	$(GO) mod download
	$(GO) mod tidy
	@echo "✓ Dependencies ready"

## Run locally (dev mode) — secret still required
run:
	$(GO) run $(LDFLAGS) $(CMD_DIR)

## Remove all build artifacts
clean:
	@rm -rf $(BUILD_DIR)
	@echo "✓ Cleaned"

## Show this help
help:
	@echo ""
	@echo "  claw-setup-wizard — build targets"
	@echo ""
	@echo "  Required env var:"
	@echo "    CLAW_API_SECRET=<secret>   injected into binary via -ldflags"
	@echo ""
	@echo "  Usage:"
	@echo "    CLAW_API_SECRET=abc123 make build-all"
	@echo ""
	@echo "  Targets:"
	@echo "    make build               Build for current machine"
	@echo "    make build-all           Build for all platforms"
	@echo "    make build-pi            Pi 4/5 (arm64) + Pi Zero (arm)"
	@echo "    make build-pi-zero       Pi Zero / Pi Zero W (arm)"
	@echo "    make build-linux-arm64   Pi 4, Pi 5, Pi Zero 2W 64-bit"
	@echo "    make build-linux-arm     Pi Zero, Pi Zero W, Pi Zero 2W 32-bit"
	@echo "    make build-linux-amd64   x86 Linux servers / VMs"
	@echo "    make build-darwin-arm64  macOS Apple Silicon"
	@echo "    make install             Install to /usr/local/bin"
	@echo "    make deps                Download Go dependencies"
	@echo "    make run                 Run locally"
	@echo "    make clean               Remove build artifacts"
	@echo ""