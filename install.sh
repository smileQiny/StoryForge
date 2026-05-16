#!/usr/bin/env sh
set -eu

REPO="${STORYFORGE_REPO:-smileQiny/StoryForge}"
VERSION="${STORYFORGE_VERSION:-latest}"
INSTALL_DIR="${STORYFORGE_INSTALL_DIR:-/usr/local/bin}"
CONNECT_TIMEOUT="${STORYFORGE_CONNECT_TIMEOUT:-15}"
DOWNLOAD_TIMEOUT="${STORYFORGE_DOWNLOAD_TIMEOUT:-300}"
RETRY_COUNT="${STORYFORGE_RETRY:-5}"

log() {
  printf '[storyforge-install] %s\n' "$*" >&2
}

die() {
  printf '[storyforge-install] ERROR: %s\n' "$*" >&2
  exit 1
}

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
  RELEASE_API_URL="https://api.github.com/repos/${REPO}/releases/latest"
else
  RELEASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
  RELEASE_API_URL="https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
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
  label="$3"

  log "download ${label}"
  log "source: ${url}"
  if command -v curl >/dev/null 2>&1; then
    log "tool: curl --http1.1, retries=${RETRY_COUNT}, connect-timeout=${CONNECT_TIMEOUT}s, max-time=${DOWNLOAD_TIMEOUT}s, resume=on"
    if ! curl_fetch "$url" "$out" "$label" ""; then
      return 1
    fi
  elif command -v wget >/dev/null 2>&1; then
    log "tool: wget, timeout=${DOWNLOAD_TIMEOUT}s, retries=${RETRY_COUNT}"
    if ! wget -q --timeout="$CONNECT_TIMEOUT" --tries="$RETRY_COUNT" "$url" -O "$out"; then
      return 1
    fi
  else
    die "curl or wget is required"
  fi

  if [ ! -s "$out" ]; then
    die "downloaded ${label} is empty"
  fi
  log "saved ${label}: ${out} ($(wc -c < "$out" | tr -d ' ') bytes)"
}

curl_fetch() {
  url="$1"
  out="$2"
  label="$3"
  accept_header="$4"
  max_attempts=$((RETRY_COUNT + 1))
  attempt=1

  while [ "$attempt" -le "$max_attempts" ]; do
    resume_bytes=0
    if [ -f "$out" ]; then
      resume_bytes="$(wc -c < "$out" | tr -d ' ')"
    fi
    log "curl attempt ${attempt}/${max_attempts} for ${label}; resume_from=${resume_bytes} bytes"

    if [ -n "$accept_header" ]; then
      curl --http1.1 \
        --connect-timeout "$CONNECT_TIMEOUT" \
        --max-time "$DOWNLOAD_TIMEOUT" \
        -fL \
        --progress-bar \
        -C - \
        -H "$accept_header" \
        "$url" \
        -o "$out" && return 0
    else
      curl --http1.1 \
        --connect-timeout "$CONNECT_TIMEOUT" \
        --max-time "$DOWNLOAD_TIMEOUT" \
        -fL \
        --progress-bar \
        -C - \
        "$url" \
        -o "$out" && return 0
    fi

    attempt=$((attempt + 1))
    if [ "$attempt" -le "$max_attempts" ]; then
      log "download attempt failed for ${label}; retrying in 2s"
      sleep 2
    fi
  done

  return 1
}

asset_api_url() {
  asset="$1"
  json="$TMP_DIR/release.json"

  log "resolve GitHub API asset URL for ${asset}"
  if command -v curl >/dev/null 2>&1; then
    curl --http1.1 --connect-timeout "$CONNECT_TIMEOUT" --max-time "$DOWNLOAD_TIMEOUT" -fsSL "$RELEASE_API_URL" -o "$json" || return 1
  elif command -v wget >/dev/null 2>&1; then
    wget -q --timeout="$CONNECT_TIMEOUT" --tries="$RETRY_COUNT" "$RELEASE_API_URL" -O "$json" || return 1
  else
    die "curl or wget is required"
  fi

  awk -v asset="$asset" '
    /"url":/ {
      url=$0
      sub(/^.*"url": "/, "", url)
      sub(/",?$/, "", url)
    }
    /"name":/ && index($0, "\"" asset "\"") {
      print url
      found=1
      exit
    }
    END {
      if (!found) exit 1
    }
  ' "$json"
}

download_github_asset() {
  asset="$1"
  out="$2"
  api_url="$(asset_api_url "$asset")" || return 1
  log "fallback source: ${api_url}"

  if command -v curl >/dev/null 2>&1; then
    curl_fetch "$api_url" "$out" "$asset" "Accept: application/octet-stream"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --timeout="$CONNECT_TIMEOUT" --tries="$RETRY_COUNT" --header="Accept: application/octet-stream" "$api_url" -O "$out"
  else
    die "curl or wget is required"
  fi
}

download_release_asset() {
  asset="$1"
  out="$2"

  if download "${RELEASE_URL}/${asset}" "$out" "$asset"; then
    return 0
  fi

  log "direct GitHub Release download failed for ${asset}; trying GitHub API asset endpoint"
  rm -f "$out"
  if ! download_github_asset "$asset" "$out"; then
    die "failed to download ${asset}. Check network access to GitHub Releases, or retry with STORYFORGE_DOWNLOAD_TIMEOUT=600."
  fi
  if [ ! -s "$out" ]; then
    die "downloaded ${asset} is empty"
  fi
  log "saved ${asset}: ${out} ($(wc -c < "$out" | tr -d ' ') bytes)"
}

log "StoryForge installer"
log "repo=${REPO}"
log "version=${VERSION}"
log "platform=${OS}/${ARCH}"
log "asset=${ASSET}"
log "install_dir=${INSTALL_DIR}"
log "release_url=${RELEASE_URL}"
log "work_dir=${TMP_DIR}"

download_release_asset "$ASSET" "${TMP_DIR}/${ASSET}"
download_release_asset "checksums.txt" "${TMP_DIR}/checksums.txt"

(
  cd "$TMP_DIR"
  log "verify checksum for ${ASSET}"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  ${ASSET}$" checksums.txt | sha256sum -c - || exit 1
  elif command -v shasum >/dev/null 2>&1; then
    grep "  ${ASSET}$" checksums.txt | shasum -a 256 -c - || exit 1
  else
    die "sha256sum or shasum is required for checksum verification"
  fi
)

log "extract ${ASSET}"
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
  die "archive does not contain storyforge binary"
fi

log "install binary"
mkdir -p "$INSTALL_DIR"
cp "$BIN" "$INSTALL_DIR/"
chmod +x "${INSTALL_DIR}/$(basename "$BIN")"

log "installed: ${INSTALL_DIR}/$(basename "$BIN")"
log "try: ${INSTALL_DIR}/$(basename "$BIN") tui"
