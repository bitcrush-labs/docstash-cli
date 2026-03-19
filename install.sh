#!/bin/sh
set -e

REPO="bitcrush-labs/docstash-cli"
INSTALL_DIR="${HOME}/.local/bin"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
    linux)  OS="linux" ;;
    darwin) OS="darwin" ;;
    *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version
VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version" >&2
    exit 1
fi

FILENAME="docstash_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

echo "Downloading docstash ${VERSION} for ${OS}/${ARCH}..."

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

curl -fsSL "$URL" -o "${TMPDIR}/${FILENAME}"
tar -xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

mkdir -p "$INSTALL_DIR"
mv "${TMPDIR}/docstash" "${INSTALL_DIR}/docstash"

echo "docstash ${VERSION} installed to ${INSTALL_DIR}/docstash"

# Check if INSTALL_DIR is in PATH
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        echo ""
        echo "Add ~/.local/bin to your PATH:"
        echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc"
        ;;
esac

echo ""
echo "Get started:"
echo "  docstash login"
