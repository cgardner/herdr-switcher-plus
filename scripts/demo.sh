#!/usr/bin/env bash
# Run the switcher against fabricated sessions.
#
# Screenshots for the documentation must not carry real session data: a real
# list names employers, colleagues, private repositories and whatever the last
# message happened to say. This builds a throwaway fixture and points the
# switcher at it, so the picture is genuine output of the real rendering path
# with nothing real in it.
#
# Everything lives in a temporary directory that is removed on exit.
set -euo pipefail
cd "$(dirname "$0")/.."

[ -x bin/herdr-switcher-plus ] || make build >/dev/null

FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

python3 scripts/demo_fixture.py "$FIXTURE"

# A stand-in for the Herdr CLI, answering the one call the switcher makes.
cat > "$FIXTURE/herdr" <<'FAKE'
#!/usr/bin/env bash
if [ "${1:-}" = "api" ] && [ "${2:-}" = "snapshot" ]; then
  cat "$(dirname "$0")/snapshot.json"
  exit 0
fi
exit 0
FAKE
chmod +x "$FIXTURE/herdr"

HOME="$FIXTURE/home" \
HERDR_BIN_PATH="$FIXTURE/herdr" \
HERDR_PLUGIN_CONFIG_DIR="$FIXTURE/config" \
HERDR_PLUGIN_STATE_DIR="$FIXTURE/state" \
  exec ./bin/herdr-switcher-plus "$@"
