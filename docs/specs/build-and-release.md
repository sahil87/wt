# Build and Release

How the `wt` binary is built locally and released to GitHub + Homebrew.

## Local build

`scripts/build.sh` (invoked as `just build`) compiles the binary into
`bin/wt`, stamping `main.version` from `git describe --tags --always`:

```bash
VERSION="$(git describe --tags --always 2>/dev/null || echo dev)"
mkdir -p bin
cd src
go build -ldflags "-X main.version=${VERSION}" -o ../bin/wt ./cmd/wt
```

`git describe --tags --always` returns the most recent tag if HEAD is exactly at
one (e.g. `v0.1.0`), otherwise the tag plus a commits-since suffix and short SHA
(e.g. `v0.1.0-3-g88cff5e`), or just the short SHA when no annotated tag exists in
history. The literal string `dev` is only used when `git describe` itself fails
(e.g. running outside a git repo).

## Local install

`scripts/install.sh` (invoked as `just local-install`) calls `build.sh` and
copies the resulting binary into `~/.local/bin/wt`:

```bash
./scripts/build.sh
DEST="${HOME}/.local/bin/wt"
mkdir -p "$(dirname "$DEST")"
cp -f ./bin/wt "$DEST"
```

For this to put `wt` on `$PATH`, `~/.local/bin` must already be in the user's
`PATH`.

## Release flow

Releases are **tag-driven**: the git tag is the single source of truth, no
`VERSION` file is mutated, no commit is created in this repo. Two steps:

1. **Local**: `just release [patch|minor|major]` calls `scripts/release.sh`,
   which:
   - Verifies the working tree is clean and a branch is checked out.
   - Computes the next semver tag from `git describe --tags --abbrev=0`
     (default `v0.0.0` if no tags exist).
   - Creates the tag locally and pushes it to `origin`.
2. **Remote**: the tag push triggers `.github/workflows/release.yml` (see
   below).

To preview without pushing, run `./scripts/release.sh --help`.

## CI workflow

`.github/workflows/release.yml` runs on `push` of any tag matching `v*`. The
workflow:

1. Checks out the repo with `fetch-depth: 0` and sets up Go from
   `src/go.mod`.
2. Extracts the version from the tag (`v0.1.0` → `0.1.0`).
3. **Cross-compiles** four platform binaries from `src/cmd/wt`:
   `darwin/arm64`, `darwin/amd64`, `linux/arm64`, `linux/amd64`. Each binary
   is packaged as `wt-<os>-<arch>.tar.gz` containing the bare `wt` binary.
4. Computes a base tag for release notes (for minor-version releases, uses
   the first tag of the previous minor series so notes span the whole minor
   range).
5. Creates a GitHub Release with auto-generated notes and all four tarballs
   attached.
6. **Updates the Homebrew tap** (`sahil87/homebrew-tap`):
   - Computes `sha256` of each tarball.
   - Clones the tap using the `HOMEBREW_TAP_TOKEN` secret.
   - Runs `sed` over `.github/formula-template.rb`, substituting
     `VERSION_PLACEHOLDER` and the four `SHA_*` markers.
   - Commits the rendered `Formula/wt.rb` to the tap repo with message
     `wt v<version>` and pushes.

After the workflow completes, end users can install with:

```bash
brew install sahil87/tap/wt
```

## Cross-compile matrix

| OS | Arch | Binary tarball |
|----|------|----------------|
| `darwin` | `arm64` | `wt-darwin-arm64.tar.gz` |
| `darwin` | `amd64` | `wt-darwin-amd64.tar.gz` |
| `linux` | `arm64` | `wt-linux-arm64.tar.gz` |
| `linux` | `amd64` | `wt-linux-amd64.tar.gz` |

`CGO_ENABLED=0` is set during cross-compile to keep binaries fully static.

## Pre-Release Setup

These prerequisites must be satisfied before the **first** tag push. Both are
one-time operator actions; the change that introduced this repo deliberately
does not perform them.

1. **`HOMEBREW_TAP_TOKEN` repo secret**. The repo `github.com/sahil87/wt`
   must have a repository secret named `HOMEBREW_TAP_TOKEN` containing a
   GitHub personal access token (or fine-grained token) with `contents:write`
   permission on `github.com/sahil87/homebrew-tap`. Without this, the
   Homebrew tap update step fails on the first run; the GitHub Release
   itself is still published correctly.
2. **`Formula/wt.rb` placeholder in the tap repo**. The tap repo
   (`github.com/sahil87/homebrew-tap`) must already contain a placeholder
   `Formula/wt.rb` so `git push` after `sed` succeeds (the workflow
   overwrites the file in place rather than creating it). The placeholder
   may be a minimal stub — its content is fully replaced on first release.
