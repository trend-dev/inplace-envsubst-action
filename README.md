# Inplace Environment Substitution for Files

This GitHub Action replaces `${VARIABLE}` placeholders in files using the workflow step's environment. It runs prebuilt, static Go binaries on Linux x64 and ARM64 runners. Docker, Go, and a build step are not required in the calling workflow.

```yaml
- name: Substitute environment variables
  uses: trend-dev/inplace-envsubst-action@v2
  env:
    COOL: "cool value"
  with:
    working-directory: testdir/replaced
    fail_on_missing_variables: "true"
    search_input: |
      {
        "patterns": ["\\.yaml$"],
        "files": ["replace.me"],
        "depth": 2
      }
```

## Running without Docker

The binaries are committed in `bin/` alongside `action.yml`. GitHub downloads them automatically with the action when resolving `uses: trend-dev/inplace-envsubst-action@v2` (or another tag or commit SHA). No separate release download or GitHub API token is required.

The composite action selects the binary for the runner's architecture, verifies the bundled SHA-256 checksums, and executes it directly. The binary, launcher, and inputs all come from the same action revision. All existing inputs and the deprecated parallel-processing alias remain supported.

Supported runners: Linux `x86_64` (GitHub `X64`) and `aarch64`/`arm64` (GitHub `ARM64`), including `ubuntu-latest` and `ubuntu-24.04-arm`. Bash and either `sha256sum` or `shasum` must be available. Other operating systems and architectures fail with a clear error.

## Inputs

| Name | Default | Description |
|------|---------|-------------|
| `working-directory` | `.` | Search root relative to `GITHUB_WORKSPACE`. Absolute paths inside the workspace are also accepted. The legacy value `/` means the workspace root. |
| `fail_on_missing_variables` | `false` | Fail if a selected placeholder has neither an environment variable nor a default. Missing variables otherwise remain unchanged. |
| `replace_in_parallel` | `false` | Process files with a worker pool bounded by the available CPU parallelism. |
| `replace_in_paralel` | | Deprecated spelling, used only when `replace_in_parallel` is not set. |
| `whitelist` | | JSON array of variable names to substitute, e.g. `["COOL"]`. All other variables remain unchanged. |
| `blacklist` | | JSON array of variable names to leave unchanged. Cannot be combined with a nonempty whitelist. |
| `search_input` | `{}` | JSON object containing `patterns`, `files`, and `depth`. |

## File selection

`patterns` contains regular expressions matched against file basenames. `files` contains exact basenames. A file matching either list is selected; when both lists are empty, all regular files are selected.

`depth` defaults to 15 when omitted or nonpositive. Depth 1 selects only files directly in the working directory; depth 2 also includes files in its immediate subdirectories.

Symbolic links, special files, and `.git` entries are skipped. File operations are confined to the selected working directory using Go's `os.Root`. A working directory escaping the workspace, including through a symbolic link, is rejected. Invalid inputs and filesystem errors fail the step.

## Substitution

- `${NAME}` uses the environment variable's value.
- `${NAME:-default}` uses the default only if the variable is **unset**. A variable set to an empty string remains empty, preserving the previous action's behavior.
- Whitelists and blacklists match the variable name, including placeholders with defaults.
- Replacement values are inserted literally and are not expanded again. Nested placeholders are unsupported.
- An unclosed `${...` placeholder fails processing of that file.
- Line endings, trailing newlines, non-placeholder bytes, and file permission bits are preserved. Lines longer than 64 KiB are supported.

Changed files are written to temporary files in the same directory and atomically renamed into place. This breaks hard links rather than modifying another linked file. A failed substitution leaves that file unchanged; other selected files may already have been processed. Replacing a file requires write permission on its parent directory. File ownership, extended attributes, and inode identity are not preserved.

Input values, template contents, and environment values are not included in validation errors. File paths can appear in filesystem errors.

## Development

The project uses Go 1.27.3 and has no third-party runtime dependencies. After changing Go source files or the Go version, rebuild both bundled Linux binaries and commit `bin/` with the source changes:

```sh
bash scripts/build.sh
bash scripts/build.sh --check
bash scripts/test-runner.sh
go test -race -cover ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

The build script requires the exact Go version from `go.mod`, disables CGO and VCS metadata, and removes local build paths. It can cross-compile both binaries from Linux or macOS. `--check` rebuilds into a temporary directory and fails if the committed binaries or checksums differ. Checksums detect corruption; the selected repository revision supplies the trust boundary.

CI checks binary reproducibility, validates the launcher and workflows, and runs the composite action against the repository fixtures, including native x64 and ARM64 execution. CI builds verify this repository; workflows using the action only download and run the committed binaries. Dependabot checks GitHub Actions updates monthly and assigns update pull requests to `sspakush`.
