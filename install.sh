#!/usr/bin/env bash
set -euo pipefail

# ---- config ----------------------------------------------------------------
REPO="pixelib/goomba"
BINARY="goomba"
INSTALL_DIR="/usr/local/bin"
# ----------------------------------------------------------------------------

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  os="linux"  ;;
  Darwin) os="darwin" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

ASSET="${BINARY}-${os}-${arch}"
API_URL="https://api.github.com/repos/${REPO}/releases/latest"

echo "Fetching latest release from ${REPO}..."
DOWNLOAD_URL="$(
  curl -fsSL "$API_URL" \
    | grep -o "\"browser_download_url\": *\"[^\"]*${ASSET}[^\"]*\"" \
    | head -1 \
    | sed 's/.*"\(https[^"]*\)".*/\1/'
)"

if [ -z "$DOWNLOAD_URL" ]; then
  echo "Could not find asset '${ASSET}' in the latest release."
  exit 1
fi

TMP="$(mktemp)"
echo "Downloading ${ASSET}..."
curl -fsSL -o "$TMP" "$DOWNLOAD_URL"
chmod +x "$TMP"

DEST="${INSTALL_DIR}/${BINARY}"
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "$DEST"
else
  echo "Installing to ${INSTALL_DIR} (may prompt for sudo)..."
  sudo mv "$TMP" "$DEST"
fi

echo "Installed: $(which $BINARY)"
"$BINARY" --version 2>/dev/null || true
