#!/usr/bin/env sh
set -eu

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
DIST_DIR="${DIST_DIR:-dist}"
APP="storyforge"
TARGETS="${TARGETS:-darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64}"

rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

for target in $TARGETS; do
  GOOS="${target%/*}"
  GOARCH="${target#*/}"

  case "$GOOS" in
    darwin) OS_NAME="Darwin" ;;
    linux) OS_NAME="Linux" ;;
    windows) OS_NAME="Windows" ;;
    *) echo "Unsupported GOOS: $GOOS" >&2; exit 1 ;;
  esac

  case "$GOARCH" in
    amd64) ARCH_NAME="x86_64" ;;
    arm64) ARCH_NAME="arm64" ;;
    *) echo "Unsupported GOARCH: $GOARCH" >&2; exit 1 ;;
  esac

  work_dir="${DIST_DIR}/${APP}_${OS_NAME}_${ARCH_NAME}"
  mkdir -p "$work_dir"

  bin_name="$APP"
  if [ "$GOOS" = "windows" ]; then
    bin_name="${APP}.exe"
  fi

  echo "Building ${APP} ${VERSION} for ${GOOS}/${GOARCH}"
  CGO_ENABLED="${CGO_ENABLED:-0}" GOOS="$GOOS" GOARCH="$GOARCH" go build \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "${work_dir}/${bin_name}" \
    ./cmd/storyforge

  cp README.md LICENSE.md COMMERCIAL-LICENSE.md DISTRIBUTION.md "$work_dir/"

  if [ "$GOOS" = "windows" ]; then
    archive="${DIST_DIR}/${APP}_${OS_NAME}_${ARCH_NAME}.zip"
    (cd "$work_dir" && zip -qr "../$(basename "$archive")" .)
  else
    archive="${DIST_DIR}/${APP}_${OS_NAME}_${ARCH_NAME}.tar.gz"
    tar -czf "$archive" -C "$work_dir" .
  fi
done

(
  cd "$DIST_DIR"
  rm -f checksums.txt
  archives=""
  for archive in *.tar.gz *.zip; do
    if [ -f "$archive" ]; then
      archives="${archives} ${archive}"
    fi
  done
  if [ -z "$archives" ]; then
    echo "No release archives were created" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    # shellcheck disable=SC2086
    sha256sum $archives > checksums.txt
  else
    # shellcheck disable=SC2086
    shasum -a 256 $archives > checksums.txt
  fi
)

echo "Release artifacts written to ${DIST_DIR}"
