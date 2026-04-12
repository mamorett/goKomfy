#!/bin/bash
set -e

APP_NAME="goKomfy"
BINARY_NAME="komfy"
BUNDLE_DIR="build/${APP_NAME}.app"
CONTENTS_DIR="${BUNDLE_DIR}/Contents"
MACOS_DIR="${CONTENTS_DIR}/MacOS"
RESOURCES_DIR="${CONTENTS_DIR}/Resources"

echo "Building macOS App Bundle..."

# Create directory structure
mkdir -p "${MACOS_DIR}"
mkdir -p "${RESOURCES_DIR}"

# Build binary
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 \
  go build -o "${MACOS_DIR}/${BINARY_NAME}" ./cmd/komfy

# Handle icon (only if on macOS with iconutil/sips available)
LOGO_PATH="cmd/komfy/logo_macos.png"

if [ -f "$LOGO_PATH" ] && command -v iconutil >/dev/null 2>&1 && command -v sips >/dev/null 2>&1; then
    echo "Generating macOS ICNS icon from $LOGO_PATH..."
    mkdir -p logo.iconset
    sips -z 16 16     "$LOGO_PATH" --out logo.iconset/icon_16x16.png
    sips -z 32 32     "$LOGO_PATH" --out logo.iconset/icon_16x16@2x.png
    sips -z 32 32     "$LOGO_PATH" --out logo.iconset/icon_32x32.png
    sips -z 64 64     "$LOGO_PATH" --out logo.iconset/icon_32x32@2x.png
    sips -z 128 128   "$LOGO_PATH" --out logo.iconset/icon_128x128.png
    sips -z 256 256   "$LOGO_PATH" --out logo.iconset/icon_128x128@2x.png
    sips -z 256 256   "$LOGO_PATH" --out logo.iconset/icon_256x256.png
    sips -z 512 512   "$LOGO_PATH" --out logo.iconset/icon_256x256@2x.png
    sips -z 512 512   "$LOGO_PATH" --out logo.iconset/icon_512x512.png
    sips -z 1024 1024 "$LOGO_PATH" --out logo.iconset/icon_512x512@2x.png
    iconutil -c icns logo.iconset -o "${RESOURCES_DIR}/icon.icns"
    rm -rf logo.iconset
else
    echo "Warning: logo.iconset generation skipped (source missing or tools unavailable)."
fi

# Create Info.plist
cat > "${CONTENTS_DIR}/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>${BINARY_NAME}</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>com.github.mamorett.goKomfy</string>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>4.0</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.11</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

echo "App bundle created at ${BUNDLE_DIR}"
echo "To run: open ${BUNDLE_DIR}"
