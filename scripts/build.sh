#!/usr/bin/env bash
# Cross-compiles linkmeta for every released target into dist/.
# VERSION is injected into main.version; defaults to "dev" for local builds.
set -euo pipefail

VERSION="${VERSION:-dev}"
BIN="linkmeta"
PKG="./cmd/linkmeta"
OUT="dist"

rm -rf "$OUT"
mkdir -p "$OUT"

# goos goarch goarm
TARGETS=(
  "linux amd64 "
  "linux arm64 "
  "linux arm 7"
  "linux arm 6"
  "linux 386 "
  "darwin amd64 "
  "darwin arm64 "
  "windows amd64 "
  "windows arm64 "
)

for target in "${TARGETS[@]}"; do
  read -r goos goarch goarm <<<"$target"
  name="${BIN}_${VERSION}_${goos}-${goarch}"
  ext=""
  if [ -n "${goarm:-}" ]; then
    name="${name}v${goarm}"
  fi
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi

  echo "building ${name}${ext}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOARM="${goarm:-}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o "${OUT}/${name}${ext}" "$PKG"
done

( cd "$OUT" && sha256sum "${BIN}"_* > checksums.txt )
echo "built $(ls -1 "$OUT" | wc -l) artifacts into ${OUT}/"
