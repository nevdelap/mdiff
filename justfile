set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

# Keep uv's mutable formatter cache outside the repository.
UV_CACHE_DIR := "/tmp/mdiff-uv-cache"
UV_TOOL_DIR := "/tmp/mdiff-uv-tools"
GOLANGCI_LINT_VERSION := "v2.14.0"
APP_PACKAGES := "./cmd/mdiff ./internal/compare ./internal/prototype ./internal/viewport"

# Show available recipes when Just is invoked without a recipe.
@_:
    @just --list

help:
    @just --list

# Download declared Go module dependencies.
install:
    go mod download

# Install the pinned Go linter into GOBIN or GOPATH/bin.
install-tools:
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{ GOLANGCI_LINT_VERSION }}

# Format Go, Markdown, and this Justfile.
format:
    go fmt ./...
    UV_CACHE_DIR={{ UV_CACHE_DIR }} UV_TOOL_DIR={{ UV_TOOL_DIR }} uv tool run --from mdformat==0.7.22 --with mdformat-tables==1.0.0 mdformat README.md docs design_docs
    docker run --rm -u "$(id -u):$(id -g)" -v "$(pwd):/workdir" -w /workdir ghcr.io/google/yamlfmt:latest .github/workflows/ci.yml .github/dependabot.yml
    just --fmt --unstable

# Verify source and documentation formatting without rewriting files.
format-check:
    #!/usr/bin/env bash
    set -euo pipefail
    mapfile -t go_files < <(find . -path './.git' -prune -o -path './.jj' -prune -o -path './vendor' -prune -o -type f -name '*.go' -print)
    if (( ${#go_files[@]} > 0 )); then
        unformatted="$(gofmt -l "${go_files[@]}")"
        if [[ -n "$unformatted" ]]; then
            echo "Go files need gofmt:" >&2
            echo "$unformatted" >&2
            exit 1
        fi
    fi
    UV_CACHE_DIR={{ UV_CACHE_DIR }} UV_TOOL_DIR={{ UV_TOOL_DIR }} uv tool run --from mdformat==0.7.22 --with mdformat-tables==1.0.0 mdformat --check README.md docs design_docs
    docker run --rm -v "$(pwd):/workdir" -w /workdir ghcr.io/google/yamlfmt:latest -lint .github/workflows/ci.yml .github/dependabot.yml
    just --fmt --check --unstable

# Lint Go source and Markdown.
lint:
    just format-check
    go vet -composites=false {{ APP_PACKAGES }}
    golangci-lint run --timeout=5m {{ APP_PACKAGES }}
    just lint-markdown
    just lint-yaml

# Lint workflow YAML.
lint-yaml:
    docker run --rm -v "$(pwd):/workdir" -w /workdir ghcr.io/ffurrer2/yamllint:latest .github/workflows/ci.yml .github/dependabot.yml

# Lint Markdown using the same containerized tool as the stay project.
lint-markdown:
    docker run --rm -u "$(id -u):$(id -g)" -v "$(pwd):/workdir" -w /workdir ghcr.io/igorshubovych/markdownlint-cli:latest README.md docs design_docs

# Run the complete Go test suite.
test:
    go test ./...

# Run the standalone terminal interaction tests.
test-pty:
    go test ./internal/prototype -run '^TestPTY' -count=1

# Run Go tests with the race detector.
test-race:
    go test -race ./...

# Run deterministic performance fixtures; output stays outside the source tree.
bench:
    #!/usr/bin/env bash
    set -euo pipefail
    bench_out="${MDIFF_BENCH_OUT:-/tmp/mdiff-bench.txt}"
    go test -run '^$' -bench='^(BenchmarkAlign|BenchmarkExpand|BenchmarkPaste|BenchmarkCached)' -benchtime=1x -benchmem -count=5 -v ./internal/compare ./internal/viewport ./internal/prototype | tee "$bench_out"
    go test -run '^$' -bench='^BenchmarkRSSMemoryGate$' -benchtime=1x -benchmem -count=5 -v ./internal/compare | tee -a "$bench_out"
    echo "benchmark report: $bench_out"

# Build the mdiff command.
build:
    go build -buildvcs=false ./cmd/mdiff

# Build an optimized local command binary.
build-release:
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o bin/mdiff ./cmd/mdiff

# Run the terminal editor prototype.
run *args:
    go run -buildvcs=false ./cmd/mdiff -- {{ args }}

# Run the fast local quality and test gate.
check-fast:
    just lint
    just test

# Run formatting, lint, tests, vet, and build.
check:
    just lint
    just test
    just build

# Run vet independently.
vet:
    go vet -composites=false {{ APP_PACKAGES }}

# Run the fast gate quietly.
qcheck-fast: (_q "check-fast")

# Run the whole-repository quality and test gate quietly.
qcheck: (_q "check")

# Run formatting and lint checks quietly.
qlint: (_q "lint")

_q target:
    #!/usr/bin/env bash
    set -uo pipefail
    echo -n "{{ target }}: "
    if just {{ target }} > check.log 2>&1; then
        echo "ok"
    else
        status=$?
        echo "FAILED: {{ target }} — tail of check.log (full log there):" >&2
        tail -n 40 check.log >&2
        exit "$status"
    fi
