# herdr-switcher-plus

An agent switcher for [Herdr](https://herdr.dev). It lists every coding agent,
offers five orderings, filters as you type, and jumps to the pane you pick.

The default ordering is the time of each session's **last real message**, which
Herdr's own sidebar cannot produce on its own.

```
 agents · last message
 12 items
▌ now  herdr-switcher-plus                  ● working
▌      ‹ [Bash]
  24m  platform ⑂ auth-service      ○ idle
       ‹ Saved into the existing GitHub auth memory rather than a second file…
   6h  web-client  @redesign-nav ○ idle
       › Authentication successful. Connected to the a vendor Docs MCP.
   1d  platform ⑂ lint-rules            ○ idle
       ‹ Saved that to project memory — it is a standing constraint, not a de…
   2d  dotfiles                           ○ idle
       › /install-app
   2d  infra-terraform / nix                     ○ idle
       ‹ The dashboard is republished in place: https://claude.ai/code/arti…

 ↑/k up • ↓/j down • / filter • enter jump • s/S sort • r refresh • q quit
```

## Repository and worktree context

Several Herdr spaces are often linked worktrees of one repository, and the
space name alone does not reveal that. The label says so, but only when the
space name does not already imply it:

| label | meaning |
|---|---|
| `dotfiles` | the space name tells you everything |
| `infra-terraform / nix` | an ordinary checkout in a differently named space |
| `platform ⑂ auth-service` | a linked worktree of another repository |
| `web-client  @redesign-nav` | a space whose name hides its branch |

The rule is to print only what is surprising. The repository appears when it
differs from the space name, the branch when it differs from the space name and
is not `main` or `master`. On a live 12-agent session that left five of twelve
rows silent, and the label column sizes itself to what the rows need.

The alternative layouts were worse in practice. A separate repository column
made half the rows repeat themselves (`web-client  web-client`) while
spending width the message preview needed. A third line per agent carried more
but dropped the visible list by a third.

Herdr reports the repository name and whether a space is a linked worktree, but
never the branch. `herdr worktree list` knows it and scopes to the calling
pane's repository, so covering every space would mean one call per repository.
Reading `.git/HEAD` in each checkout answers them all at once, in about 2 ms
across seventeen spaces.

## Configuring the layout

The layout is configurable in Herdr's own vocabulary. Its agents sidebar takes
`rows = [["state_icon", "workspace"], ["agent"]]`, where an entry is a token
name or a `{ token, fg, bold, dim, rules }` table. This uses the same language,
so anything you know from configuring the sidebar applies here.

Settings live in `config.toml` inside the directory that
`herdr plugin config-dir cgardner.herdr-switcher-plus` prints.
`example-config.toml` in this repository documents every token and ships
worked examples.

```toml
[ui]
rows = [
  ["age", "label", "state_icon", "state_text"],
  ["role", "message"],
]
```

That is the built-in layout written out: the default is expressed in the same
language a user would write, not a special case the config cannot reproduce.

Tokens: `age`, `label`, `space`, `repo`, `branch`, `state_icon`, `state_text`,
`agent`, `pane`, `cwd`, `terminal_title`, `role`, `message`. Columns size
themselves to their content, and a row after the first hangs under the first
row's second column so a message sits below the name it belongs to.

Rules are ordered and the first match wins, using `equals`, `contains`,
`starts_with`, `gt` or `lt`, with optional `ignore_case`:

```toml
rows = [
  [ "age", "label", "state_icon",
    { token = "state_text", rules = [
        { equals = "blocked", fg = "#ff6188", bold = true },
        { equals = "working", fg = "#f9e2af" },
    ]} ],
  [ "role", "message" ],
]
```

`gt` and `lt` compare a token's numeric value. Only `age` has one, in minutes,
which is what lets a rule say "older than a day".

A missing config file is normal and means the built-in layout. A malformed one
stops the switcher with the file, the row and the problem named, because
falling back silently would leave you staring at an unchanged pane.

## Age colours

The age grades itself from live to abandoned, which is the distinction most
worth seeing in a list sorted by recency:

| age | colour |
|---|---|
| under a day | green |
| under a week | yellow |
| older, or unknown | muted grey |

An unresolved age counts as the oldest, because not knowing when a session last
spoke is not evidence that it spoke recently. The bands are ordinary rules and
`example-config.toml` shows how to change them.

## Sort modes

`s` cycles forward and `S` cycles back. The mode is remembered for the next
opening. Every mode breaks ties by recency, so each group still reads
newest-first.

| mode | order | use |
|---|---|---|
| `recent` | newest message first | where was I? |
| `attention` | blocked, done, idle, working, unknown | where am I blocking work? |
| `space` | sidebar space order | match the sidebar's layout |
| `name` | space name, A to Z | a list that does not move |
| `oldest` | oldest message first | find sessions worth closing |

`attention` puts `blocked` first because it waits on an answer, and `working`
near the bottom because it needs nobody.

## Filtering by status

Single keys narrow to one agent state, matching Herdr's own workspace picker,
whose footer reads `filter a/b/w/i/d`:

| key | shows |
|---|---|
| `a` | all agents |
| `b` | blocked |
| `w` | working |
| `i` | idle |
| `d` | done |

The active filter appears in the title, so an empty list always explains
itself: `agents · last message · blocked`.

Herdr's picker offers no key for the `unknown` state, and this matches that.
An unknown agent is reachable under `a` only. `unknown` means Herdr could not
classify the pane, so it belongs to no state's list.

The status filter does **not** persist between openings, unlike the sort mode.
A sticky "blocked only" would silently hide agents the next time you open the
switcher.

`--status` sets it from the command line, taking either the picker key or the
full name:

```bash
herdr-switcher-plus --list --status blocked
herdr-switcher-plus --list --status w
```

## Filtering by text

`/` filters by case-insensitive substring across space name, repository,
branch, status, agent kind, pane title, working directory, pane ID and the last
message text. The repository and branch stay searchable on rows whose label
leaves them out as implied, so `/platform` finds every worktree of it. Every
space-separated term must match, so `/blocked nix` narrows to both.

Filtering never reorders. The bubbles default ranks by fuzzy score, which would
silently override the sort mode; `/working` also matched nine of twelve agents
under fuzzy rules when only one was working.

## Why this is a pane and not a sidebar setting

Herdr's own agents sidebar accepts two sort modes only:

```toml
agent_panel_sort = "spaces"    # grouped by space
agent_panel_sort = "priority"  # attention queue
```

No plugin entrypoint changes that order. The manifest offers `build`, `startup`,
`actions`, `events`, `panes` and `link_handlers`, and none of them reach the
sidebar's ordering. So this plugin renders its own popup instead of trying to
alter the built-in panel.

## Why the file modification time is not the answer

The obvious implementation sorts on the transcript's mtime. That is wrong, and
badly so.

Claude Code keeps appending bookkeeping records to a transcript long after the
conversation stops: `artifact-autoreact-ledger`, `bridge-session`,
`file-history-snapshot`, `mode`, `atis-latch` and others. None of them is a
message and none carries a timestamp.

Measured against a live 12-agent session:

| session | age by mtime | age by last message |
|---|---|---|
| `nix` | 31m | **52h** |
| `github` | 1m | **25h** |
| `billing` | 37m | **64h** |

Sorting on mtime ranked `github` 2nd when it belonged in 9th place. So this
plugin scans each transcript backwards for the newest entry of type `user` or
`assistant` that carries a timestamp, and sorts on that.

The cost is negligible. Only live agents need a read, the scan starts from the
tail, and the whole run takes about **20 ms**.

## Install

```bash
herdr plugin install cgardner/herdr-switcher-plus
```

That fetches the prebuilt binary for your platform from the matching GitHub
release. The Go toolchain is needed only when no release asset matches, in
which case the install compiles from source instead.

For local development:

```bash
git clone https://github.com/cgardner/herdr-switcher-plus
cd herdr-switcher-plus
make link
```

`plugin link` skips the `[[build]]` step, so `make link` builds the binary
first. `make help` lists the rest.

## Open it

```bash
# opens in whichever mode you last used
herdr plugin action invoke cgardner.herdr-switcher-plus.open

# opens with blocked agents first
herdr plugin action invoke cgardner.herdr-switcher-plus.open-attention
```

Outside Herdr the binary also runs standalone:

```bash
herdr-switcher-plus --list --sort attention
```

### Bind a key

Use `type = "plugin_action"` and put the fully qualified action id in
`command`:

```toml
[[keys.command]]
key = "prefix+a"
type = "plugin_action"
command = "cgardner.herdr-switcher-plus.open"
description = "agent switcher"

[[keys.command]]
key = "prefix+shift+a"
type = "plugin_action"
command = "cgardner.herdr-switcher-plus.open-attention"
description = "agents needing attention"
```

An `action = "..."` field does not work. Herdr 0.9.1 rejects it as an unknown
key, exactly as it rejects a misspelled one, and then disables the binding for
having no command.

Validate any change with `herdr config check`, and load it with
`herdr server reload-config` or the `reload_config` key.

If `~/.config/herdr/config.toml` is a symlink into the Nix store, it is
read-only. The binding belongs in the Home Manager module that generates it,
not in the linked file.

## Keys

| key | action |
|---|---|
| `↑` `↓` `k` `j` | move |
| `/` | filter on space, repo, branch, status, kind, title, path and message text |
| `a` `b` `w` `i` `d` | show all, blocked, working, idle or done |
| `s` `S` | cycle the sort mode forward or back |
| `enter` | focus that pane |
| `r` | refresh, holding the cursor in place |
| `q` `esc` | close |

A sort change or a status change returns the cursor to the top, because the
point of either is to see a different set. A refresh holds the cursor instead,
so background activity never moves the row you are reading.

## How it reads the data

1. `herdr api snapshot` returns every agent with its pane, space, status and
   `agent_session.value`, the Claude Code session UUID.
2. Every `~/.claude/projects/*/*.jsonl` filename is indexed by UUID. This step
   reads no file contents.
3. Each live agent's transcript is scanned from the tail for the newest `user`
   or `assistant` entry, which gives the timestamp and a preview line.
4. `herdr workspace focus`, `herdr tab focus` and `herdr agent focus` run in
   sequence on Enter, so the jump lands even when the target sits in a space
   that is off screen.

Changing sort mode never re-reads a transcript. `Collect` gathers the rows once
and `Apply` reorders the slice already in hand.

The chosen mode is stored in `HERDR_PLUGIN_STATE_DIR`, which is where the Herdr
docs say plugin state belongs. Herdr offers no plugin storage API, so every
read and write there fails silently rather than blocking the switcher.

## Limits

- The message time covers **Claude Code** sessions only. Another agent kind has
  no transcript here, so its row shows `—` and falls to the bottom, ordered by
  Herdr's `state_change_seq` counter.
- The list is a snapshot. `r` refreshes it. There is no live auto-sort yet.
- Filter matches are not highlighted. The match indexes address the filter
  value, but the list delegate paints them onto the rendered title, where an
  index landing inside a color sequence splits it and leaks escape text.
- Herdr allows one popup at a time. An open popup makes the action fail with
  `ui_busy`.
- Tested on macOS against Herdr 0.9.1. Linux is declared but untested.
- The `a/b/w/i/d` meanings are read off the built-in picker's footer text. No
  Herdr documentation describes that overlay, so the mapping is inferred from
  the key letters and the known agent states.

## Selected row

The selected agent gets a full-width background bar and a left marker, the way
Herdr's own overlays mark a selection. The delegate composes every segment
itself: a foreground style emits its own reset, so an unstyled segment would
punch a hole in the bar part way across the line.

The color comes from the first of these that holds a valid hex value:

1. the `HERDR_SWITCHER_PLUS_SELECTION_BG` environment variable
2. `selection_bg` under `[theme.custom]` in the Herdr config
3. `#313244`, which is catppuccin's Surface0

Herdr resolves a named theme inside its binary and exposes no API for the
resulting palette, so a different named theme cannot be read. Someone not on
catppuccin sets the color explicitly through one of the first two.

## Develop

```bash
make ci      # gofmt, go vet, tests, and the coverage floor
make dist    # cross-compile every released platform
```

`AGENTS.md` records the findings that are not visible in the code: the Herdr
API behavior that no documentation states, why the file modification time is
unusable for ordering, and how to drive the terminal UI in a test.

## Releasing

A conventional commit on `main` opens a release pull request through
release-please, which bumps `version` in `herdr-plugin.toml` and the changelog.
Merging it tags the release, and the release workflow cross-compiles all four
platforms, writes `SHA256SUMS` and attaches them.

The tag, the manifest version and the download URL in `scripts/install.sh` must
agree. The release workflow fails a tag that disagrees with the manifest.

Coverage is 99.8% of statements, with four of the six packages at 100%. The
three uncovered statements are the one-line boundaries to the outside world,
each holding no logic:

| statement | why no unit test reaches it |
|---|---|
| `exec.Command(...).Output()` | spawns a process |
| `tea.NewProgram(...)` | needs a real terminal |
| `os.Exit(cli.Run(...))` | the `main` shim |

Everything around those three sits behind a package variable that a test
replaces, so command sequencing, snapshot parsing, the transcript join, key
handling and row rendering are all exercised without a Herdr server or a TTY.
