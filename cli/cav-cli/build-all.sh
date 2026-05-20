#!/bin/bash
# Cross-compile cav-cli for all platforms
# Output goes to ./dist/ — upload these to the gateway's /dl/ path
#
# Usage: ./build-all.sh
# Then copy dist/* to /var/www/html/dl/ on the server

set -e

VERSION="0.1.0"
MODULE="github.com/anthropic-cav/cav-cli"
LDFLAGS="-s -w -X main.version=$VERSION"
DIST="./dist"

rm -rf "$DIST"
mkdir -p "$DIST"

echo "Building cav-cli v$VERSION..."

# Linux amd64
echo "  → linux/amd64"
GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$DIST/cav-cli-linux-amd64" .

# Linux arm64
echo "  → linux/arm64"
GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$DIST/cav-cli-linux-arm64" .

# macOS amd64
echo "  → darwin/amd64"
GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$DIST/cav-cli-darwin-amd64" .

# macOS arm64 (Apple Silicon)
echo "  → darwin/arm64"
GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$DIST/cav-cli-darwin-arm64" .

# Windows amd64
echo "  → windows/amd64"
GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$DIST/cav-cli-windows-amd64.exe" .

echo ""
echo "✓ All builds complete:"
ls -lh "$DIST/"
echo ""
echo "Deploy: copy dist/* to the gateway's --dist-dir"
echo "  default: <gateway-cwd>/gateway-data/dist/"
echo "  override: cav-gateway --dist-dir /path/to/dist (or CAV_DIST_DIR=...)"
echo ""
echo "install.sh is embedded in the gateway binary; drop a file at"
echo "<dist-dir>/install.sh only if you need to override at runtime."
