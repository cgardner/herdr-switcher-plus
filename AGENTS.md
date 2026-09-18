# AGENTS.md

A Herdr plugin: an agent switcher that orders every coding agent by the time of
its last real message. Go, Bubble Tea, no runtime dependencies.

Run `make help` for the targets and `herdr plugin --help` for the CLI. This file
carries only what those cannot tell you.

`README.md` is the front door, `docs/configuration.md` the full config
reference, `docs/design.md` the reasoning behind the decisions, and
`docs/troubleshooting.md` the failure modes.

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

## Herdr reports no branch

The snapshot carries a workspace's `repo_name`, `is_linked_worktree` and
`checkout_path`, and no branch. `herdr worktree list` knows the branch but
scopes to the calling pane's repository, so covering every space costs one call
per repository. `internal/gitref` reads `.git/HEAD` instead and answered 14 of
17 spaces in about 2 ms.

A linked worktree's `.git` is a file holding `gitdir: <path>`, not a directory.
Several spaces here are linked worktrees of one repository, so that is the
common path rather than an edge case.

`Row.Label` decides what reaches the screen, and its rule is to print only what
is surprising: the repository when it differs from the space name, the branch
when it differs from the space name and is not the default. Adding a field that
is usually implied costs width on every row and tells the reader nothing.

## The layout config speaks Herdr's language

`rows = [[...]]` with `{ token, fg, bold, dim, rules }` tables is Herdr's own
sidebar vocabulary, adopted wholesale rather than invented. The limits are
Herdr's too: 16 rows, 16 tokens per row, 16 rules per token. Keep it that way.
A second dialect for the same job is the thing this avoids.

`ui.DefaultSpec` is written in that vocabulary, so the built-in look is not a
special case the config cannot reproduce. A test asserts it validates against
the token registry.

Adding a token means one entry in `ui.tokens`, and nothing else. The registry
drives rendering, validation, the error message a bad name earns, and the
reference tables `make docs` writes. A token may carry a number, which is what
`gt` and `lt` compare against; only `age` does, in minutes.

<!-- BEGIN GENERATED: token-count -->
There are 13 tokens, listed in `docs/configuration.md`.
<!-- END GENERATED: token-count -->

Bold and Dim are `*bool` throughout. Herdr states that an omitted style field
keeps the contextual default, and a plain bool cannot tell "unset" from "off".

`example-config.toml` is executable documentation: a test extracts every
commented example and loads it, so the file cannot drift from the registry.

## Reference documentation is generated

The token, sort-mode and status tables are written by `make docs` from the
registries, into the regions between `BEGIN GENERATED` and `END GENERATED`
markers. Prose outside those markers is hand-written and stays that way.

Each registry carries its own descriptions, so a new entry documents itself:
`ui.tokens`, `agents.modeDescriptions`, `agents.statusDescriptions`. Never
write one of these tables by hand. `make ci` fails when a file is out of date,
and a missing marker is an error rather than a silent skip.

Coverage uses `-coverpkg` over `./internal/...` and the root. Without it a
helper called only from a sibling package's tests reads as dead. `tools/` is
left out deliberately: `make docs-check` runs the generator end to end in CI,
which proves more than a unit test of its flag parsing.

## Asserting on rendered output

Two mistakes cost time here, both in tests rather than code:

- Column positions are runes, not bytes. The selection bar is one column and
  three bytes, so `strings.Index` reports a selected row further right than it
  renders.
- An SGR attribute rides alongside colours (`\x1b[1;97;48;2;48;50;68m`), so a
  search for `\x1b[1m` misses it. An extended colour spells itself
  `48;2;R;G;B`, where the 2 is the truecolor marker, so its arguments must be
  skipped or every backgrounded cell looks faint. `hasSGR` in the ui tests does
  this correctly.

## Seams

Anything reaching outside the process sits behind a package variable that a
test replaces: `herdr.run`, `agents.loadSnapshot`, `agents.indexTranscripts`,
`agents.readTranscript`, `agents.resolveBranch`, `transcript.openTail`,
`ui.collectRows`, `cli.loadConfig`,
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
from that version. release-please syncs the manifest, and the build workflow
fails a tag that disagrees.

The release build runs inside the release-please workflow, gated on its
`release_created` output. GitHub starts no workflow from an event created with
`GITHUB_TOKEN`, so a tag release-please pushes never fires `on: push: tags`.
Moving the build back out would tag a release with no binaries attached, and
nothing would fail: installs would quietly compile from source instead.

The `PLATFORMS` list in the `Makefile` and the `uname` cases in
`scripts/install.sh` name the same four targets. Changing one without the other
publishes assets nobody fetches, or fetches assets nobody published.
