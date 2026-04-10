APP_NAME    = goKomfy
APP_ID      = gokomfy
BINARY_NAME = gokomfy
CLI_NAME    = gokomfy-cli
MODULE      = github.com/mamorett/goKomfy
BUILD_DIR   = build
ICON_NAME   = logo.png
ICON_PATH   = cmd/komfy/$(ICON_NAME)
FYNE        = $(HOME)/go/bin/fyne

# Detect OS
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')

.PHONY: all clean build-host install-linux bundle-icon test

all: build-host

test:
	go test ./...

# Bundle logo.png as a Go resource
bundle-icon:
	cd cmd/komfy && $(FYNE) bundle -o bundled.go $(ICON_NAME)

# Main build target that detects host
build-host: bundle-icon
	mkdir -p $(BUILD_DIR)
ifeq ($(OS),darwin)
	@echo "Building macOS App Bundle..."
	cd cmd/komfy && CGO_ENABLED=1 $(FYNE) package -os darwin -name $(APP_NAME) -icon $(ICON_NAME) -id $(APP_ID)
	mv cmd/komfy/$(APP_NAME).app $(BUILD_DIR)/
else ifeq ($(OS),linux)
	@echo "Building Linux binaries..."
	GOOS=linux CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/komfy
	GOOS=linux CGO_ENABLED=1 go build -o $(BUILD_DIR)/$(CLI_NAME) ./cmd/komfy-cli
else
	@echo "Unsupported OS: $(OS)"
	exit 1
endif

# Linux installation for desktop environment integration
install-linux:
ifneq ($(OS),linux)
	@echo "Install-linux is only supported on Linux"
	exit 1
endif
	install -Dm755 $(BUILD_DIR)/$(BINARY_NAME) $(HOME)/.local/bin/$(BINARY_NAME)
	install -Dm644 $(ICON_PATH) $(HOME)/.local/share/icons/hicolor/256x256/apps/gokomfy.png
	install -Dm644 build/linux/gokomfy.desktop $(HOME)/.local/share/applications/gokomfy.desktop
	update-desktop-database $(HOME)/.local/share/applications/ || true
	gtk-update-icon-cache -f -t $(HOME)/.local/share/icons/hicolor/ || true

clean:
	rm -rf $(BUILD_DIR)
	rm -f cmd/komfy/bundled.go
	rm -f $(APP_NAME).tar.xz
	rm -rf cmd/komfy/$(APP_NAME).app
