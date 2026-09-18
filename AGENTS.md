# AGENTS.md

A Herdr plugin: an agent switcher that orders every coding agent by the time of
its last real message. Go, Bubble Tea, no runtime dependencies.

Run `make help` for the targets and `herdr plugin --help` for the CLI. This file
carries only what those cannot tell you.

## Do not sort on the file modification time

The single trap in this codebase. Claude Code appends bookkeeping records to a
transcript long after the conversation stops, so mtime runs hours ahead of the
last message. Measured on a live session, mtime reported 31m for a conversation
that was 52h old and put one session 2nd that belonged 9th.

Ordering comes from the newest `user` or `assistant` entry that carries a
timestamp. `internal/transcript/transcript.go` holds the full reasoning and the
list of bookkeeping types. Read it before touching recency.

## Herdr facts that no documentation states

Herdr's docs cover the plugin manifest and the CLI. Everything below was read
out of the binary or found by probing, and each one cost real time:

- **Bind a key with `type = "plugin_action"`** and the fully qualified action id
  in `command`. An `action = "..."` field is rejected as an unknown key, and the
  binding is then disabled for having no command.
- **`herdr config check` is the oracle.** It validates a config and names every
  unknown key, so probing a candidate form beats guessing. `HERDR_CONFIG_PATH`
  points it at a scratch file.
- **One popup exists at a time.** A second `plugin pane open` returns
  `ui_busy`. Close the open one with the `popup.close` socket method, which has
  no CLI.
- **The built-in picker filters with `a/b/w/i/d`** (all, blocked, working, idle,
  done). The switcher mirrors those keys. The meanings are inferred from the key
  letters, because nothing documents that overlay.
- **`agent.view.set` sorts the real sidebar.** It takes a sort list and a
  filter, and a sort field may be `{"token": "<name>"}`, matching a token that
  `pane.report-metadata` writes onto a pane. So the built-in agents panel *can*
  be ordered by last message, even though `agent_panel_sort` offers only
  `spaces` and `priority`. It has no CLI, and the snapshot does not report the
  resulting order, so only a human can confirm it visually.

## Seams

Anything reaching outside the process sits behind a package variable that a
test replaces: `herdr.run`, `agents.loadSnapshot`, `agents.indexTranscripts`,
`agents.readTranscript`, `transcript.openTail`, `ui.collectRows`,
`cli.collect`, `cli.loadMode`, `cli.saveMode`, `cli.focus`, `cli.newProgram`.

Keep new outside calls behind the same pattern. `make cover` holds a 99% floor,
and three statements are knowingly uncovered: `exec.Command`, `tea.NewProgram`
and the `os.Exit` shim in `main`. Each is one line with no logic. Adding a
fourth means the seam is in the wrong place.

## Testing the terminal UI

Bubble Tea needs a real PTY, and it blocks at startup asking the terminal two
questions. A harness has to answer both or it captures nothing:

1. `\x1b]11;?` asks the background color.
   Reply `\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\`.
2. `\x1b[6n` asks the cursor position. Reply `\x1b[1;1R`.

Set the window size with `TIOCSWINSZ` after forking, or the list renders empty.
Call `lipgloss.SetColorProfile(termenv.TrueColor)` in a unit test, or lipgloss
strips every sequence and color assertions pass against nothing.

## Rendering

The delegate composes each row segment by segment and attaches the selection
background to every one. A foreground style emits its own reset, so styling a
row up front punches a hole in the highlight part way across the line.

The filter returns empty `MatchedIndexes`. They address the filter value, but
the list paints them onto the rendered title, where an index landing inside a
color sequence splits it and leaks escape text into the pane.

The filter matches substrings rather than the bubbles fuzzy default, which
reorders results and so overrides the chosen sort mode.

## Releasing

Three places carry the version and all three must agree: the git tag, `version`
in `herdr-plugin.toml`, and the download URL that `scripts/install.sh` builds
from that version. release-please syncs the manifest, and the release workflow
fails a tag that disagrees.

The `PLATFORMS` list in the `Makefile` and the `uname` cases in
`scripts/install.sh` name the same four targets. Changing one without the other
publishes assets nobody fetches, or fetches assets nobody published.
