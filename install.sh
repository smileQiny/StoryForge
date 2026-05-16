#!/usr/bin/env sh
set -eu

REPO="${STORYFORGE_REPO:-smileQiny/StoryForge}"
VERSION="${STORYFORGE_VERSION:-latest}"
INSTALL_DIR="${STORYFORGE_INSTALL_DIR:-/usr/local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  darwin) OS="Darwin" ;;
  linux) OS="Linux" ;;
  msys*|mingw*|cygwin*) OS="Windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  RELEASE_URL="https://github.com/${REPO}/releases/latest/download"
else
  RELEASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
fi

EXT="tar.gz"
if [ "$OS" = "Windows" ]; then
  EXT="zip"
fi

ASSET="storyforge_${OS}_${ARCH}.${EXT}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$out"
  else
    echo "curl or wget is required" >&2
    exit 1
  fi
}

echo "Downloading StoryForge ${VERSION} from GitHub Releases"
download "${RELEASE_URL}/${ASSET}" "${TMP_DIR}/${ASSET}"
download "${RELEASE_URL}/checksums.txt" "${TMP_DIR}/checksums.txt"

(
  cd "$TMP_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  ${ASSET}$" checksums.txt | sha256sum -c -
  elif command -v shasum >/dev/null 2>&1; then
    grep "  ${ASSET}$" checksums.txt | shasum -a 256 -c -
  else
    echo "sha256sum or shasum is required for checksum verification" >&2
    exit 1
  fi
)

if [ "$EXT" = "zip" ]; then
  unzip -q "${TMP_DIR}/${ASSET}" -d "$TMP_DIR"
else
  tar -xzf "${TMP_DIR}/${ASSET}" -C "$TMP_DIR"
fi

BIN="${TMP_DIR}/storyforge"
if [ "$OS" = "Windows" ]; then
  BIN="${TMP_DIR}/storyforge.exe"
fi

if [ ! -f "$BIN" ]; then
  echo "Archive does not contain storyforge binary" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
cp "$BIN" "$INSTALL_DIR/"
chmod +x "${INSTALL_DIR}/$(basename "$BIN")"

echo "StoryForge installed to ${INSTALL_DIR}/$(basename "$BIN")"
