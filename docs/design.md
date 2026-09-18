# Design notes

Why the switcher works the way it does. The reasoning is here rather than in
the README so the front page stays short, and because these are the decisions
most likely to be revisited.

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
| A | 31m | **52h** |
| B | 1m | **25h** |
| C | 37m | **64h** |

Sorting on mtime ranked session B 2nd when it belonged in 9th place. So this
plugin scans each transcript backwards for the newest entry of type `user` or
`assistant` that carries a timestamp, and sorts on that.

The cost is negligible. Only live agents need a read, the scan starts from the
tail, and the whole run takes about **20 ms**.

## Repository and worktree context

Several Herdr spaces are often linked worktrees of one repository, and the
space name alone does not reveal that. The label says so, but only when the
space name does not already imply it:

| label | meaning |
|---|---|
| `dotfiles` | the space name tells you everything |
| `infra-terraform / infra` | an ordinary checkout in a differently named space |
| `platform ⑂ auth-service` | a linked worktree of another repository |
| `web-client  @redesign-nav` | a space whose name hides its branch |

The rule is to print only what is surprising. The repository appears when it
differs from the space name, the branch when it differs from the space name and
is not `main` or `master`. On a live 12-agent session that left five of twelve
rows silent, and the label column sizes itself to what the rows need.

The alternative layouts were worse in practice. A separate repository column
made half the rows repeat themselves (`api-gateway  api-gateway`) while
spending width the message preview needed. A third line per agent carried more
but dropped the visible list by a third.

Herdr reports the repository name and whether a space is a linked worktree, but
never the branch. `herdr worktree list` knows it and scopes to the calling
pane's repository, so covering every space would mean one call per repository.
Reading `.git/HEAD` in each checkout answers them all at once, in about 2 ms
across seventeen spaces.

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
