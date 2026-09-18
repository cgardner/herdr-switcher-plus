# Troubleshooting

## The keybinding does nothing

Herdr binds a plugin action with `type = "plugin_action"` and the fully
qualified action id in `command`:

```toml
[[keys.command]]
key = "prefix+a"
type = "plugin_action"
command = "cgardner.herdr-switcher-plus.open"
description = "agent switcher"
```

An `action = "..."` field does not work. Herdr rejects it as an unknown key,
exactly as it rejects a misspelled one, and then disables the binding for having
no command. `herdr config check` reports this, and accepts `HERDR_CONFIG_PATH`
so you can validate a scratch file before touching your real one.

Load a changed config with `herdr server reload-config`.

If `~/.config/herdr/config.toml` is a symlink into the Nix store, it is
read-only. The binding belongs in the Home Manager module that generates it.

## The action fails with `ui_busy`

Herdr allows one popup at a time, so opening the switcher while another popup
is up returns:

```
{"error":{"code":"ui_busy","message":"a popup pane is already open"}}
```

Close the open one and try again.

## An agent shows `—` for its age, and sorts last

No Claude Code transcript matched that pane. The age comes from the session
UUID in the snapshot, matched against `~/.claude/projects/*/*.jsonl`.

Another agent kind has no transcript there at all, so it always reads `—` and
falls to the bottom, ordered by Herdr's own state-change counter. That is
expected rather than broken.

## The ages look wrong after resuming an old session

The age is the newest `user` or `assistant` entry carrying a timestamp, not the
file modification time. Claude Code keeps appending bookkeeping records long
after a conversation stops, so a transcript's mtime can run hours or days ahead
of its last real message. See [design.md](design.md).

## My config did nothing

It should never do nothing. A malformed config stops the switcher and names the
file, the row and the problem:

```
herdr-switcher-plus: …/config.toml: row 1: unknown token "nonsense"
```

If you see the built-in layout instead, the file is not where the plugin reads
it. Check the path:

```bash
herdr plugin config-dir cgardner.herdr-switcher-plus
```

The file is `config.toml` inside that directory.

## The colours look wrong

Colours given as an ANSI index follow your terminal theme. Colours given as hex
do not, so a config written against catppuccin will look wrong elsewhere.

The selection bar is the exception: Herdr resolves a named theme inside its own
binary and exposes no API for the palette, so the bar defaults to catppuccin's
`#313244`. Set `HERDR_SWITCHER_PLUS_SELECTION_BG`, or `selection_bg` under
`[theme.custom]` in your Herdr config.

## Installing needs a Go toolchain

It should not. `scripts/install.sh` fetches the prebuilt binary for your
platform from the matching GitHub release, and compiles only when no asset
matches. A source build means either your platform is not in the release matrix,
or the tag and the manifest version disagree so the download URL points nowhere.

## Nothing here helped

`herdr plugin log list --plugin cgardner.herdr-switcher-plus` shows what each
action ran, its exit code and its stderr.
