#!/bin/sh
set -e

# homelabctl install script
# Usage: curl -fsSL https://raw.githubusercontent.com/jdillenberger/homelabctl/main/install.sh | sh

REPO_URL="https://github.com/jdillenberger/homelabctl/releases/latest/download"
INSTALL_DIR="/usr/local/bin"
BINARY="homelabctl"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
    echo "Error: homelabctl only supports Linux. Detected: $OS"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    armv7l|armhf)
        ARCH="armv7"
        ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        echo "Supported: amd64, arm64, armv7"
        exit 1
        ;;
esac

TARBALL="${BINARY}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="${REPO_URL}/${TARBALL}"

echo "Downloading homelabctl for ${OS}/${ARCH}..."
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if command -v curl > /dev/null 2>&1; then
    curl -fsSL "$DOWNLOAD_URL" -o "${TMPDIR}/${TARBALL}"
elif command -v wget > /dev/null 2>&1; then
    wget -q "$DOWNLOAD_URL" -O "${TMPDIR}/${TARBALL}"
else
    echo "Error: curl or wget is required to download homelabctl"
    exit 1
fi

echo "Extracting..."
tar -xzf "${TMPDIR}/${TARBALL}" -C "$TMPDIR"

echo "Installing to ${INSTALL_DIR}/${BINARY}..."
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
    sudo mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi
chmod +x "${INSTALL_DIR}/${BINARY}"

VERSION=$("${INSTALL_DIR}/${BINARY}" --version 2>/dev/null || echo "unknown")
echo ""
echo "homelabctl installed successfully!"
echo "  Version:  ${VERSION}"
echo "  Location: ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Get started:"
echo "  homelabctl doctor    # check dependencies"
echo "  homelabctl apps list # list available apps"
