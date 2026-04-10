APP_NAME   = komfy
CLI_NAME   = komfy-cli
MODULE     = github.com/mamorett/goKomfy
BUILD_DIR  = build

.PHONY: all clean linux-amd64 linux-arm64 macos-arm64 macos-app test

all: linux-amd64 linux-arm64 macos-arm64 macos-app

test:
	go test ./...

# Create macOS App Bundle
macos-app:
	bash bundle_macos.sh

# Linux amd64 (native on a Linux/amd64 host, or cross-compiled)
linux-amd64:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
	  go build -o $(BUILD_DIR)/$(APP_NAME)-linux-amd64 ./cmd/komfy
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
	  go build -o $(BUILD_DIR)/$(CLI_NAME)-linux-amd64 ./cmd/komfy-cli

# Linux arm64
linux-arm64:
	mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
	  CC=aarch64-linux-gnu-gcc \
	  go build -o $(BUILD_DIR)/$(APP_NAME)-linux-arm64 ./cmd/komfy
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
	  CC=aarch64-linux-gnu-gcc \
	  go build -o $(BUILD_DIR)/$(CLI_NAME)-linux-arm64 ./cmd/komfy-cli

# macOS arm64 (must be run on macOS arm64 host — CGO cross-compilation from Linux to macOS is complex)
macos-arm64:
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
	  go build -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 ./cmd/komfy
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
	  go build -o $(BUILD_DIR)/$(CLI_NAME)-darwin-arm64 ./cmd/komfy-cli

clean:
	rm -rf $(BUILD_DIR)
