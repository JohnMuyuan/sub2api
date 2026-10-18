#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/app" "$test_root/bin"
# Exercise the actual entrypoint without root, Docker, or touching /app.
printf '#!/bin/sh\nprintf "1000\\n"\n' > "$test_root/bin/id"
chmod +x "$test_root/bin/id"
export PATH="$test_root/bin:$PATH"
export SUB2API_PERSISTENT_UPDATES=1
sed "s|/app|$test_root/app|g" "$repo_root/deploy/docker-entrypoint.sh" > "$test_root/entrypoint.sh"
image="$test_root/app/sub2api"
runtime="$test_root/app/data/runtime"
write_binary() {
    printf '#!/bin/sh\nprintf "%%s\\n" "%s"\n' "$2" > "$1"
    chmod +x "$1"
}
start() { sh "$test_root/entrypoint.sh" "$image"; }
write_binary "$image" image-v1
[ "$(start)" = image-v1 ]
[ -s "$runtime/image.sha256" ]
# Simulate the updater's atomic replacement, then same-image restart/recreate.
cp "$runtime/sub2api" "$runtime/sub2api.backup"
write_binary "$runtime/download" updated-v2
mv "$runtime/download" "$runtime/sub2api"
[ "$(start)" = updated-v2 ]
# Rollback remains executable after restart.
mv "$runtime/sub2api.backup" "$runtime/sub2api"
[ "$(start)" = image-v1 ]
# An explicitly pulled different image takes precedence over persistent updates.
write_binary "$image" image-v3
[ "$(start)" = image-v3 ]
[ ! -e "$runtime/sub2api.backup" ]
# Custom commands and ordinary images retain their existing startup behavior.
[ "$(sh "$test_root/entrypoint.sh" printf custom-command)" = custom-command ]
SUB2API_PERSISTENT_UPDATES=0
export SUB2API_PERSISTENT_UPDATES
write_binary "$runtime/sub2api" persistent-other
[ "$(start)" = image-v3 ]
printf 'persistent Docker update tests passed\n'
