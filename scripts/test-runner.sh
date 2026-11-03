#!/usr/bin/env bash
set -euo pipefail

action_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
test_dir="$(mktemp -d)"
trap 'rm -rf -- "$test_dir"' EXIT
mkdir -p "$test_dir/action with spaces/scripts" "$test_dir/action with spaces/bin" "$test_dir/mock"
cp "$action_dir/scripts/run.sh" "$test_dir/action with spaces/scripts/"

# Mock only platform detection and the binaries. Exercise real checksum checks.
cat > "$test_dir/mock/uname" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  -s) echo "${TEST_SYSTEM:-Linux}" ;;
  -m) echo "${TEST_MACHINE:-x86_64}" ;;
esac
EOF
chmod +x "$test_dir/mock/uname"
export PATH="$test_dir/mock:$PATH"

for arch in amd64 arm64; do
  printf '#!/usr/bin/env bash\nprintf "%%s\\n" "%s" "$INPUT_SEARCH_INPUT"\n' "$arch" \
    > "$test_dir/action with spaces/bin/envsubst-linux-$arch"
  chmod +x "$test_dir/action with spaces/bin/envsubst-linux-$arch"
done
(
  cd -- "$test_dir/action with spaces/bin"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum envsubst-linux-amd64 envsubst-linux-arm64
  else
    shasum -a 256 envsubst-linux-amd64 envsubst-linux-arm64
  fi
) > "$test_dir/action with spaces/bin/SHA256SUMS"

# Shell syntax in inputs must be passed literally to the binary.
export INPUT_SEARCH_INPUT='{"files":["$(exit 99); spaces and quotes"]}'
launcher="$test_dir/action with spaces/scripts/run.sh"
for machine in x86_64 aarch64 arm64; do
  case "$machine" in
    x86_64) expected=amd64 ;;
    *) expected=arm64 ;;
  esac
  actual="$(TEST_MACHINE="$machine" bash "$launcher")"
  [[ "$actual" == "$expected"$'\n'"$INPUT_SEARCH_INPUT" ]]
done

if TEST_SYSTEM=Darwin bash "$launcher" > "$test_dir/output" 2>&1; then
  echo 'Unsupported platform was accepted' >&2
  exit 1
fi
grep -q 'supported runners' "$test_dir/output"

echo 'corrupted' >> "$test_dir/action with spaces/bin/envsubst-linux-amd64"
if bash "$launcher" > "$test_dir/output" 2>&1; then
  echo 'Corrupted binary was executed' >&2
  exit 1
fi
grep -q 'checksum verification failed' "$test_dir/output"

rm "$test_dir/action with spaces/bin/envsubst-linux-amd64"
if bash "$launcher" > "$test_dir/output" 2>&1; then
  echo 'Missing binary was accepted' >&2
  exit 1
fi
echo 'Action launcher tests passed'
