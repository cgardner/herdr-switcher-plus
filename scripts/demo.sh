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

# A stand-in for the Herdr CLI, answering the snapshot and the session list.
# The first session's socket is the one HERDR_SOCKET_PATH names below, so the
# panel marks it as the session the switcher runs in. Changes made from
# the tree go over a socket that does not exist here, so they fail visibly.
cat > "$FIXTURE/herdr" <<'FAKE'
#!/usr/bin/env bash
if [ "${1:-}" = "api" ] && [ "${2:-}" = "snapshot" ]; then
  cat "$(dirname "$0")/snapshot.json"
  exit 0
fi
if [ "${1:-}" = "session" ] && [ "${2:-}" = "list" ]; then
  d="$HOME/.config/herdr"
  printf '{"sessions":[%s,%s,%s]}\n' \
    "{\"name\":\"default\",\"default\":true,\"running\":true,\"session_dir\":\"$d\",\"socket_path\":\"$d/herdr.sock\"}" \
    "{\"name\":\"spike\",\"default\":false,\"running\":true,\"session_dir\":\"$d/sessions/spike\",\"socket_path\":\"$d/sessions/spike/herdr.sock\"}" \
    "{\"name\":\"release-0.2\",\"default\":false,\"running\":false,\"session_dir\":\"$d/sessions/release-0.2\",\"socket_path\":\"$d/sessions/release-0.2/herdr.sock\"}"
  exit 0
fi
exit 0
FAKE
chmod +x "$FIXTURE/herdr"

# HERDR_SOCKET_PATH must point into the fixture. Inherited from a real pane,
# it would send every change made in the demo to the real session.
HOME="$FIXTURE/home" \
HERDR_BIN_PATH="$FIXTURE/herdr" \
HERDR_SOCKET_PATH="$FIXTURE/home/.config/herdr/herdr.sock" \
HERDR_PLUGIN_CONFIG_DIR="$FIXTURE/config" \
HERDR_PLUGIN_STATE_DIR="$FIXTURE/state" \
  exec ./bin/herdr-switcher-plus "$@"
