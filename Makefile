# ─────────────────────────────────────────────────────────────────────────────
# goKomfy — Build Configuration
# ─────────────────────────────────────────────────────────────────────────────

APP_NAME    := goKomfy
APP_ID      := gokomfy
BINARY_NAME := gokomfy
CLI_NAME    := gokomfy-cli
MODULE      := github.com/mamorett/goKomfy

# Paths
BUILD_DIR   := build
CMD_KOMFY   := cmd/komfy
CMD_CLI     := cmd/komfy-cli
ICON_NAME   := logo.png
ICON_SRC    := $(CMD_KOMFY)/$(ICON_NAME)

# Tools
FYNE        := $(HOME)/go/bin/fyne

# Install paths (Linux)
INSTALL_BIN  := $(HOME)/.local/bin
INSTALL_APPS := $(HOME)/.local/share/applications
INSTALL_ICON := $(HOME)/.local/share/icons/hicolor

# ─────────────────────────────────────────────────────────────────────────────
# Detect host OS
# ─────────────────────────────────────────────────────────────────────────────

OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')

# ─────────────────────────────────────────────────────────────────────────────
# Phony targets
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: all build bundle-icon install-linux clean test

# ─────────────────────────────────────────────────────────────────────────────
# Default target
# ─────────────────────────────────────────────────────────────────────────────

all: build

# ─────────────────────────────────────────────────────────────────────────────
# Tests
# ─────────────────────────────────────────────────────────────────────────────

test:
	go test ./...

# ─────────────────────────────────────────────────────────────────────────────
# Bundle icon as an embedded Go resource
# ─────────────────────────────────────────────────────────────────────────────

bundle-icon: $(ICON_SRC)
	cd $(CMD_KOMFY) && $(FYNE) bundle -o bundled.go $(ICON_NAME)

# ─────────────────────────────────────────────────────────────────────────────
# Build (auto-detects host OS)
# ─────────────────────────────────────────────────────────────────────────────

build: bundle-icon
	mkdir -p $(BUILD_DIR)
ifeq ($(OS),darwin)
	@echo "→ Building macOS app bundle…"
	cd $(CMD_KOMFY) && CGO_ENABLED=1 $(FYNE) package \
		-os darwin -name $(APP_NAME) -icon logo_macos.png -id $(APP_ID)
	mv $(CMD_KOMFY)/$(APP_NAME).app $(BUILD_DIR)/
else ifeq ($(OS),linux)
	@echo "→ Building Linux binaries…"
	GOOS=linux CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_KOMFY)
	GOOS=linux CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(CLI_NAME)    ./$(CMD_CLI)
else
	$(error Unsupported OS: $(OS))
endif

# ─────────────────────────────────────────────────────────────────────────────
# Install (Linux only)
# ─────────────────────────────────────────────────────────────────────────────

install-linux:
ifneq ($(OS),linux)
	$(error install-linux is only supported on Linux)
endif
	@echo "→ Removing stale desktop entries…"
	rm -f $(INSTALL_APPS)/$(APP_ID).desktop
	rm -f $(INSTALL_APPS)/io.github.mamorett.$(APP_ID).desktop

	@echo "→ Installing binaries…"
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_BIN)/$(BINARY_NAME)
	install -Dm755 $(BUILD_DIR)/$(CLI_NAME)    $(INSTALL_BIN)/$(CLI_NAME)

	@echo "→ Installing icons…"
	install -Dm644 $(ICON_SRC) $(INSTALL_ICON)/256x256/apps/$(APP_ID).png
	install -Dm644 $(ICON_SRC) $(INSTALL_ICON)/128x128/apps/$(APP_ID).png
	install -Dm644 $(ICON_SRC) $(INSTALL_ICON)/48x48/apps/$(APP_ID).png

	@echo "→ Installing desktop entry…"
	mkdir -p $(INSTALL_APPS)
	cp assets/linux/$(APP_ID).desktop $(INSTALL_APPS)/$(APP_ID).desktop
	sed -i "s|^Exec=.*|Exec=$(INSTALL_BIN)/$(BINARY_NAME)|" \
		$(INSTALL_APPS)/$(APP_ID).desktop

	@echo "→ Refreshing desktop and icon caches…"
	update-desktop-database $(INSTALL_APPS)/ 2>/dev/null || true
	gtk-update-icon-cache -f -t $(INSTALL_ICON)/ 2>/dev/null || true
	kbuildsycoca6 2>/dev/null || kbuildsycoca5 2>/dev/null || true

	@echo "✓ Installation complete."

# ─────────────────────────────────────────────────────────────────────────────
# Clean
# ─────────────────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BUILD_DIR)
	rm -f  $(CMD_KOMFY)/bundled.go
	rm -rf $(CMD_KOMFY)/$(APP_NAME).app
	rm -f  $(APP_NAME).tar.xz
