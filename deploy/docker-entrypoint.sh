#!/bin/sh
set -e

# Fix data directory permissions when running as root.
# Docker named volumes / host bind-mounts may be owned by root,
# preventing the non-root sub2api user from writing files.
if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    # Use || true to avoid failure on read-only mounted files (e.g. config.yaml:ro)
    chown -R sub2api:sub2api /app/data 2>/dev/null || true
    # Re-invoke this script as sub2api so the flag-detection below
    # also runs under the correct user.
    exec su-exec sub2api "$0" "$@"
fi

# Compatibility: if the first arg looks like a flag (e.g. --help),
# prepend the default binary so it behaves the same as the old
# ENTRYPOINT ["/app/sub2api"] style.
if [ "${1#-}" != "$1" ]; then
    set -- /app/sub2api "$@"
fi

# Custom images keep the executable beside the persistent application data.
# A new image seeds its own binary once; restarting the same image preserves
# a binary installed by the built-in updater.
if [ "${SUB2API_PERSISTENT_UPDATES:-0}" = "1" ] && [ "${1:-}" = "/app/sub2api" ]; then
    runtime_dir=/app/data/runtime
    mkdir -p "$runtime_dir"
    image_hash=$(sha256sum /app/sub2api | cut -d ' ' -f 1)
    previous_hash=$(cat "$runtime_dir/image.sha256" 2>/dev/null || true)
    if [ "$image_hash" != "$previous_hash" ] || [ ! -x "$runtime_dir/sub2api" ]; then
        cp /app/sub2api "$runtime_dir/sub2api.seed"
        chmod 755 "$runtime_dir/sub2api.seed"
        mv -f "$runtime_dir/sub2api.seed" "$runtime_dir/sub2api"
        rm -f "$runtime_dir/sub2api.backup"
        printf '%s\n' "$image_hash" > "$runtime_dir/image.sha256.tmp"
        mv -f "$runtime_dir/image.sha256.tmp" "$runtime_dir/image.sha256"
    fi
    shift
    set -- "$runtime_dir/sub2api" "$@"
fi

exec "$@"
