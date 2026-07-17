# List available recipes (default when running bare `just`).
default:
    @just --list

# Refresh the embedded skill bundle from the canonical docs/site/skill.md (drift-guarded by a test).
sync-skill:
    ./scripts/sync-skill.sh

# Build the wt binary into ./bin/wt, stamped with `git describe` as the version.
build:
    ./scripts/build.sh

# Build and install to ~/.local/bin/wt (ensure that dir is on your $PATH).
local-install:
    ./scripts/install.sh

# Run the Go test suite under src/.
test:
    cd src && go test ./...

# Cut a release: compute next semver tag from `git describe`, push the tag.
# CI (.github/workflows/release.yml) takes over from there to cross-compile,
# create the GitHub Release, and update the Homebrew tap. Working tree must be clean.
# Bump tag from `git describe` and push — CI builds the release. (patch|minor|major, default: patch)
release bump="patch":
    ./scripts/release.sh {{bump}}
