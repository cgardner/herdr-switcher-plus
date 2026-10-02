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

## The tree view

Herdr gives each linked worktree a workspace of its own. A tree with the
workspace at the root therefore has exactly one checkout under each workspace,
which adds a level and says nothing. The tree groups on the repository
instead, so that the worktrees of one repository sit together.

The grouping key is `repo_key`, the git directory that every checkout of a
repository shares. `repo_root` is not safe as a key: Herdr reports it with a
trailing slash for some checkouts and without one for others.

A repository with only one workspace is not given a level. The workspace takes
its place at the root and uses the label rule above, so that a group of one
does not cost a line.

An open group shows no summary, because its panes show the same status and age
on the lines below. A closed group shows the most urgent status, the newest age
and the agent count, because those panes are then out of sight.

The tree draws its own lines rather than use the bubbles list. The list items
are flat, and the list filter reorders them. The tree still pages the way the list does,
with the list's own pager dots, so that the two views read alike.

`agent focus` rejects a shell pane with `agent_not_found`, and the CLI has no
command that focuses a pane by its ID. The `pane.focus` socket method does, so
a shell pane is focused over the socket. An agent pane still goes through
`agent focus`, the path the list uses.

## Searching the tree

The tree search matches the way the list filter does: case-insensitive
substrings, with every word required, and no fuzzy match. A fuzzy match
reorders the results and spreads a word across the line, and the reasons in
`internal/ui/filter.go` apply here too.

A pane's search text includes the names of every group above it. A search
for a repository, a workspace or a branch therefore finds every pane below it,
and the tree still shows only panes and the groups that lead to them. The
cursor goes to the first match after each key, so that `enter` jumps without a
move. Letters are text while the search line is open, so a management key
cannot fire while you type.

## Managing the session from the tree

The tree already knows the workspace, the tab, the pane and a directory for
every line, so it is the place to act on them. The list shows agents only, and
a workspace with no agent never appears in it, so it gets no management keys.

Every change goes over the socket rather than the CLI. The socket takes the IDs
that the snapshot gives, and `pane.rename` takes a null label, which is the
only way to clear a name. No change moves focus, because the switcher is open
while it runs. After each change the tree reads the snapshot again, and a node
that is gone leaves the cursor at the same height rather than at the top.

A close asks for `y` first, and any other key cancels. Herdr has no undo for a
closed pane, and an agent in it stops. A rename and a new workspace ask for a
name on the line above the pager. That line carries the result too, so a
refusal from Herdr is visible without a notification.

A move uses `pane.move`, which keeps the pane's terminal and so the process in
it. That is what makes a move safe for an agent in the middle of a
conversation, where a close and a new pane would lose it. The pane gets a new
ID in its new workspace, so the cursor cannot follow it, and stays at the same
height. The move panel walks the full tree, not the filtered one, so that a
status filter cannot hide a destination. It offers the pane's own workspace as
a new tab only when the pane shares its tab with another.

A named session is a separate server with its own socket, and the socket API
has no session methods. The session panel therefore runs `herdr session list`,
`stop` and `delete`. It finds the session that the switcher runs in by
comparing each socket path with `HERDR_SOCKET_PATH`, and refuses to stop that
one. Herdr itself refuses to delete a running session, and the panel says so
before it asks.

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
