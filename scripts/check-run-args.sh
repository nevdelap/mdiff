#!/usr/bin/env bash
set -euo pipefail

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/go" <<'EOF'
#!/usr/bin/env bash
printf '%s\0' "$@" > "$MDIFF_ARGS_CAPTURE"
EOF
chmod +x "$tmp/go"

check() {
    local name=$1
    shift
    MDIFF_ARGS_CAPTURE="$tmp/args" PATH="$tmp:$PATH" just --no-deps run "$@" >/dev/null
    printf '%s\0' run -buildvcs=false ./cmd/mdiff "$@" > "$tmp/expected"
    if ! cmp -s "$tmp/expected" "$tmp/args"; then
        printf '%s: just run changed or split an argument\n' "$name" >&2
        exit 1
    fi
}

check two-files left.txt right.txt
check cli-flag --config-dir "$tmp/config dir"
check spaced-path 'left with space.txt' right.txt
