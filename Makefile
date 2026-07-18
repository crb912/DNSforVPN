# dnsforvpn — one Makefile for every platform.
#
#   make linux                    build/linux/    (binary + config + install/uninstall scripts)
#   make windows                  build/windows/  (dnsforvpn.exe + config + install/uninstall.bat)
#   make macos                    build/macos/
#   make openwrt                  build/openwrt/  (default ARCH=arm64; make openwrt ARCH=mipsle)
#   make run                      detect host OS, build it, run build/<os>/ binary in foreground
#   make clean                    remove build/
#
# Cross-compilation is plain Go (CGO disabled), e.g.:
#   make linux ARCH=arm64
#   make macos ARCH=arm64
#
# Staged config.toml / rules are copied only when absent (cp -n semantics),
# so edits made via the Web UI survive rebuilds. `make clean` resets them.

BINARY    := dnsforvpn
BUILD_DIR := build
GOFLAGS   := -trimpath
LDFLAGS   := -s -w

export CGO_ENABLED := 0

# --- host detection (for `make run` and default ARCH) ---
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
  HOST_OS := linux
else ifeq ($(UNAME_S),Darwin)
  HOST_OS := macos
else ifneq (,$(findstring MINGW,$(UNAME_S)))
  HOST_OS := windows
else ifneq (,$(findstring MSYS,$(UNAME_S)))
  HOST_OS := windows
endif

HOST_ARCH := $(shell go env GOHOSTARCH)

ifeq ($(HOST_OS),windows)
  RUN_BIN := $(BINARY).exe
else
  RUN_BIN := $(BINARY)
endif

.PHONY: all frontend linux windows macos openwrt run clean
all: linux

# --- frontend (embed requires dist/) ---
frontend:
ifeq ($(wildcard frontend/node_modules),)
	cd frontend && npm ci
endif
	cd frontend && npm run build

# stage config + rules seed into $(1) from $(2), never overwriting existing files
define stage_config
	@mkdir -p $(1)/rules $(1)/data
	@test -f $(1)/config.toml || cp $(2) $(1)/config.toml
	@test -f $(1)/rules/gfwlist.txt || cp configs/rules/gfwlist.txt $(1)/rules/gfwlist.txt
endef

# --- platform targets ---

linux: ARCH ?= $(HOST_ARCH)
linux: frontend
	@mkdir -p $(BUILD_DIR)/linux
	GOOS=linux GOARCH=$(ARCH) go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/linux/$(BINARY) ./cmd/dnsforvpn
	$(call stage_config,$(BUILD_DIR)/linux,configs/config.toml)
	cp deploy/linux/install.sh deploy/linux/uninstall.sh $(BUILD_DIR)/linux/
	cp frontend/public/icons/icon-512.png $(BUILD_DIR)/linux/icon.png
	@echo ">> $(BUILD_DIR)/linux ready (GOARCH=$(ARCH))"

windows: ARCH ?= $(HOST_ARCH)
windows: frontend
	@mkdir -p $(BUILD_DIR)/windows
	GOOS=windows GOARCH=$(ARCH) go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/windows/$(BINARY).exe ./cmd/dnsforvpn
	$(call stage_config,$(BUILD_DIR)/windows,configs/config.toml)
	cp deploy/windows/install.bat deploy/windows/uninstall.bat $(BUILD_DIR)/windows/
	@echo ">> $(BUILD_DIR)/windows ready (GOARCH=$(ARCH))"

macos: ARCH ?= $(HOST_ARCH)
macos: frontend
	@mkdir -p $(BUILD_DIR)/macos
	GOOS=darwin GOARCH=$(ARCH) go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/macos/$(BINARY) ./cmd/dnsforvpn
	$(call stage_config,$(BUILD_DIR)/macos,configs/config.toml)
	cp deploy/macos/install.sh deploy/macos/uninstall.sh $(BUILD_DIR)/macos/
	@echo ">> $(BUILD_DIR)/macos ready (GOARCH=$(ARCH))"

openwrt: ARCH ?= arm64
openwrt: frontend
	@mkdir -p $(BUILD_DIR)/openwrt
	if [ "$(ARCH)" = mipsle ] || [ "$(ARCH)" = mips ]; then SF=GOMIPS=softfloat; fi; \
	env GOOS=linux GOARCH=$(ARCH) $$SF go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/openwrt/$(BINARY) ./cmd/dnsforvpn
	$(call stage_config,$(BUILD_DIR)/openwrt,deploy/openwrt/config.toml)
	cp deploy/openwrt/dnsforvpn.init deploy/openwrt/install.sh deploy/openwrt/uninstall.sh $(BUILD_DIR)/openwrt/
	@echo ">> $(BUILD_DIR)/openwrt ready (GOARCH=$(ARCH))"

# --- run on the current host ---

run:
ifndef HOST_OS
	$(error unsupported host OS: $(UNAME_S))
endif
run: $(HOST_OS)
	./$(BUILD_DIR)/$(HOST_OS)/$(RUN_BIN) --config $(BUILD_DIR)/$(HOST_OS)/config.toml

clean:
	rm -rf $(BUILD_DIR)
