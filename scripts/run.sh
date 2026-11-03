#!/usr/bin/env bash
set -euo pipefail

action_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"

case "$(uname -s):$(uname -m)" in
  Linux:x86_64) binary=envsubst-linux-amd64 ;;
  Linux:aarch64|Linux:arm64) binary=envsubst-linux-arm64 ;;
  *)
    echo 'envsubst: supported runners are Linux x64 and ARM64' >&2
    exit 1
    ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
  checksum=(sha256sum)
else
  checksum=(shasum -a 256)
fi

if ! (cd -- "$action_dir/bin" && "${checksum[@]}" --check --status SHA256SUMS); then
  echo 'envsubst: bundled binary checksum verification failed' >&2
  exit 1
fi

exec "$action_dir/bin/$binary"
