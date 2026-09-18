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

## Sort modes

`s` cycles forward and `S` cycles back. The mode is remembered for the next
opening. Every mode breaks ties by recency, so each group still reads
newest-first.

<!-- BEGIN GENERATED: sort-modes -->
| mode | orders by | answers |
|---|---|---|
| `recent` | last message | where was I? |
| `attention` | needs attention | where am I blocking work? |
| `space` | space order | match the sidebar's own order |
| `name` | space name | a list that does not move as agents work |
| `oldest` | oldest first | what can I close? |
<!-- END GENERATED: sort-modes -->

`attention` puts `blocked` first because it waits on an answer, and `working`
near the bottom because it needs nobody.

## Filtering by status

Single keys narrow to one agent state, matching Herdr's own workspace picker,
whose footer reads `filter a/b/w/i/d`:

<!-- BEGIN GENERATED: status-keys -->
| key | shows | meaning |
|---|---|---|
| `a` | all | every agent, including any Herdr could not classify |
| `b` | blocked | waiting on an answer from you |
| `w` | working | busy, and needing nobody |
| `i` | idle | ready for the next instruction |
| `d` | done | finished work nobody has looked at yet |
<!-- END GENERATED: status-keys -->

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

## Configuring the layout

The layout is yours to choose, in Herdr's own vocabulary. Its agents sidebar
takes `rows = [["state_icon", "workspace"], ["agent"]]`, where an entry is a
token name or a `{ token, fg, bold, dim, rules }` table. This uses the same
language, so anything you know from configuring the sidebar applies here.

```toml
[ui]
rows = [
  ["age", "label", "state_icon", "state_text"],
  ["role", "message"],
]
```

That is the built-in layout written out: the default is expressed in the same
language a user would write, not a special case the config cannot reproduce.

**[docs/configuration.md](docs/configuration.md)** is the full reference: every
token, the style fields, the rules, and the errors a bad file earns.
`example-config.toml` is a commented copy with worked examples.

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

## Documentation

| | |
|---|---|
| [docs/configuration.md](docs/configuration.md) | every token, style and rule |
| [docs/design.md](docs/design.md) | why the switcher works the way it does |
| [docs/troubleshooting.md](docs/troubleshooting.md) | when something does not work |
| [CONTRIBUTING.md](CONTRIBUTING.md) | how to build, test and release |
| [AGENTS.md](AGENTS.md) | what a coding agent needs to know first |

## Develop

```bash
make ci      # gofmt, vet, generated docs, tests, and the coverage floor
make docs    # regenerate the reference tables from the registries
make dist    # cross-compile every released platform
```

The token, sort-mode and status tables in this file and in
`docs/configuration.md` are generated from the registries in the code. Edit the
registry, run `make docs`. CI fails when they drift.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the rest.

## License

GPL-2.0. See [LICENSE](LICENSE).
