#!/bin/bash

set -e

REPO="liyu1981/moshpf"
BINARY_NAME="mpf"
INSTALL_DIR="${HOME}/.local/bin"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "${OS}" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "Error: Unsupported OS '${OS}'"
    exit 1
    ;;
esac

# Detect Architecture
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture '${ARCH}'"
    exit 1
    ;;
esac

# Check if OS/Arch combination is supported
SUPPORTED=false
if [ "${OS}" = "linux" ] && ([ "${ARCH}" = "amd64" ] || [ "${ARCH}" = "arm64" ]); then
  SUPPORTED=true
elif [ "${OS}" = "darwin" ] && [ "${ARCH}" = "arm64" ]; then
  SUPPORTED=true
fi

if [ "${SUPPORTED}" = false ]; then
  echo "Error: Unsupported platform '${OS}-${ARCH}'"
  echo "Supported platforms are: linux-amd64, linux-arm64, darwin-arm64"
  exit 1
fi

# Get the latest release tag from GitHub
echo "Fetching latest release version..."
LATEST_TAG=$(curl -s https://api.github.com/repos/${REPO}/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "${LATEST_TAG}" ]; then
  echo "Error: Could not determine latest release version."
  exit 1
fi

VERSION="${LATEST_TAG}"
TARBALL="mpf-${VERSION}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

echo "Downloading ${BINARY_NAME} ${VERSION} for ${OS}-${ARCH}..."
TMP_DIR=$(mktemp -d)
curl -L "${URL}" -o "${TMP_DIR}/${TARBALL}"

echo "Installing to ${INSTALL_DIR}..."
mkdir -p "${INSTALL_DIR}"
tar -xzf "${TMP_DIR}/${TARBALL}" -C "${TMP_DIR}"
mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

# Clean up
rm -rf "${TMP_DIR}"

echo "Successfully installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"

# Check if INSTALL_DIR is in PATH
if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
  echo ""
  echo "Warning: ${INSTALL_DIR} is not in your PATH."
  echo "You can add it by adding the following line to your shell profile (e.g., ~/.bashrc or ~/.zshrc):"
  echo "  export PATH="\$HOME/.local/bin:\$PATH""
fi
