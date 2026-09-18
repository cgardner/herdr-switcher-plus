# Configuration

Settings live in `config.toml` inside the directory that
`herdr plugin config-dir cgardner.herdr-switcher-plus` prints. Every setting is
optional, and a missing file means the built-in layout.

`example-config.toml` in the repository root is a commented copy of everything
below, with worked examples.

## The layout language

The vocabulary is Herdr's own. Its agents sidebar takes
`rows = [["state_icon", "workspace"], ["agent"]]`, where an entry is a token
name or a `{ token, fg, bold, dim, rules }` table. This plugin uses the same
language, so anything you know from configuring `[ui.sidebar.agents]` applies.

```toml
[ui]
row_gap = 0
rows = [
  ["age", "label", "state_icon", "state_text"],
  ["role", "message"],
]
```

That is the built-in layout written out. The default is expressed in the same
language a user would write, rather than a special case the config cannot
reproduce.

Limits are Herdr's: at most 16 rows, 16 tokens per row, and 16 rules per token.

Columns size themselves to their content, so a list where nothing carries
repository context stays as narrow as one that never had the feature. A row
after the first hangs under the first row's second column, which puts a message
below the name it belongs to rather than under the age.

## Tokens

<!-- BEGIN GENERATED: tokens -->
| token | shows | number for `gt` and `lt` |
|---|---|---|
| `age` | time since the last message | minutes |
| `agent` | the agent kind, such as claude or codex | — |
| `branch` | the branch the checkout sits on | — |
| `cwd` | the working directory, with your home written as a tilde | — |
| `label` | the space, plus the repository and branch when not already implied | — |
| `message` | the last message in the session | — |
| `pane` | the Herdr pane ID, such as w5:p1E | — |
| `repo` | the repository name, empty when the space is not a checkout | — |
| `role` | a mark showing whether you or the agent spoke last | — |
| `space` | the Herdr space name on its own | — |
| `state_icon` | a filled dot while an agent wants attention, hollow at rest | — |
| `state_text` | idle, working, blocked, done or unknown | — |
| `terminal_title` | the pane title as the terminal reports it | — |
<!-- END GENERATED: tokens -->

## Styles

A token table accepts `fg`, `bold` and `dim`.

An omitted field keeps the contextual default, so restyling one token does not
mean restating the whole theme. An explicit `false` does turn a default off,
which is how the message row loses its dimming.

`fg` accepts an ANSI index as a string, such as `"2"`, or a hex value such as
`"#a6e3a1"`. An index follows whatever theme your terminal uses. A hex value
does not, so it will look wrong to someone on a different theme.

## Rules

Rules style a token only when its value matches. They are ordered and the first
match wins, so a specific case goes before a general one.

| condition | compares | notes |
|---|---|---|
| `equals` | the whole text | |
| `contains` | any substring | |
| `starts_with` | the beginning | |
| `gt` | a number | only tokens with a numeric value |
| `lt` | a number | only tokens with a numeric value |

`ignore_case = true` applies to the three text conditions, matching ASCII case
insensitively. A numeric condition never fires on a token that carries no
number.

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

## Age colours

The age grades itself from live to abandoned, because two bands put yesterday's
session and last month's in the same colour:

| age | colour |
|---|---|
| under a day | green |
| under a week | yellow |
| older, or unknown | muted grey |

An unresolved age counts as the oldest. Not knowing when a session last spoke is
not evidence that it spoke recently.

The bands are ordinary rules on the `age` token, whose number is minutes:

```toml
rows = [
  [ { token = "age", rules = [
      { lt = 1440,  fg = "2" },
      { lt = 10080, fg = "3" },
      { gt = 10080, fg = "8" },
    ]},
    "label", "state_icon", "state_text" ],
  [ "role", "message" ],
]
```

## The selection bar

The colour of the bar behind the selected agent comes from the first of these
that holds a valid hex value:

1. the `HERDR_SWITCHER_PLUS_SELECTION_BG` environment variable
2. `selection_bg` under `[theme.custom]` in your Herdr config
3. `#313244`, which is catppuccin's Surface0

Herdr resolves a named theme inside its own binary and exposes no API for the
resulting palette, so a different named theme cannot be read. Set the colour
explicitly through one of the first two.

## Errors

A missing config file is normal and never an error.

A malformed one stops the switcher, naming the file, the row and the problem.
Falling back to the built-in layout would leave you staring at an unchanged pane
with no idea why your file did nothing.

```
herdr-switcher-plus: …/config.toml: row 1: unknown token "nonsense"
```
