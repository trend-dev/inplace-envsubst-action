#!/usr/bin/env bash
set -euo pipefail

action_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd -- "$action_dir"

case "${1:-}" in
  '') check=false ;;
  --check) check=true ;;
  *) echo 'Usage: bash scripts/build.sh [--check]' >&2; exit 1 ;;
esac

expected="go$(awk '$1 == "go" { print $2 }' go.mod)"
if [[ "$(go env GOVERSION)" != "$expected" ]]; then
  echo "Build with $expected, as specified in go.mod" >&2
  exit 1
fi

build_dir="$(mktemp -d)"
trap 'rm -rf -- "$build_dir"' EXIT

for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 \
    go build -trimpath -buildvcs=false -ldflags='-s -w' \
    -o "$build_dir/envsubst-linux-$arch" .
done

(
  cd -- "$build_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum envsubst-linux-amd64 envsubst-linux-arm64
  else
    shasum -a 256 envsubst-linux-amd64 envsubst-linux-arm64
  fi
) > "$build_dir/SHA256SUMS"

if "$check"; then
  for file in envsubst-linux-amd64 envsubst-linux-arm64 SHA256SUMS; do
    if ! cmp -s "$build_dir/$file" "$action_dir/bin/$file"; then
      echo "bin/$file is out of date; run bash scripts/build.sh and commit bin/" >&2
      exit 1
    fi
  done
else
  mkdir -p bin
  cp "$build_dir/"* bin/
fi
