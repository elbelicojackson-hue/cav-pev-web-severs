#!/bin/sh
# CAV CLI Installer — one-line install from gateway
#
# Usage:
#   curl -fsSL https://modgert.online/install.sh | sh
#   wget -qO- https://modgert.online/install.sh | sh
#
# Or with custom install dir:
#   curl -fsSL https://modgert.online/install.sh | sh -s -- --dir /opt/bin

set -e

GATEWAY="https://modgert.online"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="cav-cli"

# Parse args
while [ $# -gt 0 ]; do
  case "$1" in
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    *) shift ;;
  esac
done

# Detect OS and arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7l) ARCH="arm" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux) OS="linux" ;;
  darwin) OS="darwin" ;;
  mingw*|msys*|cygwin*) OS="windows"; BINARY_NAME="cav-cli.exe" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

DOWNLOAD_URL="${GATEWAY}/dl/cav-cli-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
  DOWNLOAD_URL="${DOWNLOAD_URL}.exe"
fi

echo "╔══════════════════════════════════════════╗"
echo "║   CAV Citizen Protocol CLI Installer     ║"
echo "╚══════════════════════════════════════════╝"
echo ""
echo "  OS:       $OS"
echo "  Arch:     $ARCH"
echo "  Install:  $INSTALL_DIR/$BINARY_NAME"
echo "  Source:   $DOWNLOAD_URL"
echo ""

# Download
echo "→ Downloading cav-cli..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$DOWNLOAD_URL" -o "/tmp/$BINARY_NAME"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "/tmp/$BINARY_NAME" "$DOWNLOAD_URL"
else
  echo "Error: curl or wget required"
  exit 1
fi

# Install
echo "→ Installing to $INSTALL_DIR/$BINARY_NAME..."
mkdir -p "$INSTALL_DIR"
mv "/tmp/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Verify
echo "→ Verifying installation..."
if "$INSTALL_DIR/$BINARY_NAME" --help >/dev/null 2>&1; then
  echo ""
  echo "✓ cav-cli installed successfully!"
  echo ""
  echo "Quick start:"
  echo "  cav-cli init                    # Generate identity"
  echo "  cav-cli auth                    # Authenticate"
  echo "  cav-cli peers                   # See who's online"
  echo "  cav-cli publish finding.json    # Share a Praxon"
  echo ""
  echo "Gateway: $GATEWAY"
else
  echo "✗ Installation failed"
  exit 1
fi
