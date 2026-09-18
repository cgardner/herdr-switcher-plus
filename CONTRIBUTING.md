# Contributing

## Build and test

```bash
make build   # bin/herdr-switcher-plus
make ci      # gofmt, vet, generated docs, tests, coverage floor
make link    # build, then link this working copy into the running Herdr session
make run     # print the agent list without opening a pane
```

`make help` lists every target.

## Documentation is generated

The token, sort-mode and status tables in `README.md`,
`docs/configuration.md`, `example-config.toml` and `AGENTS.md` are written by
`make docs` from the registries in the code:

| registry | generates |
|---|---|
| `internal/ui.tokens` | the token reference |
| `internal/agents.Modes` | the sort-mode table |
| `internal/agents.StatusKeys` | the status-key table |

Only the regions between `BEGIN GENERATED` and `END GENERATED` markers are
rewritten, so the prose around them stays hand-written.

Adding a token means one registry entry carrying its description. Run
`make docs`, and the reference updates everywhere. `make ci` fails when the
files are out of date, so the documentation cannot drift from the code without
CI noticing.

## Coverage

`make cover` holds a floor of 99% of statements. Three statements are knowingly
uncovered, each a one-line boundary to the outside world with no logic in it:
`exec.Command`, `tea.NewProgram`, and the `os.Exit` shim in `main`.

Everything else sits behind a package variable a test replaces. If a fourth
statement becomes untestable, the seam is in the wrong place rather than the
floor being too high.

Coverage is measured with `-coverpkg` across the internal packages and the root,
so a call from one package into another counts. `tools/` is excluded: it is a
build-time generator, and `make docs-check` runs it end to end in CI.

## Testing the terminal UI

Bubble Tea needs a real PTY and asks the terminal two questions at startup. A
harness must answer both or it captures an empty screen. `AGENTS.md` has the
details, along with the two ways an assertion on rendered output goes wrong.

## Releasing

GitHub blocks Actions from opening pull requests by default, and the workflow's
own `pull-requests: write` permission does not override it. Without the repo
setting, release-please builds the whole release branch and then fails at the
final step with "GitHub Actions is not permitted to create or approve pull
requests". Enable it once, under Settings, Actions, General, Workflow
permissions.

A conventional commit on `main` opens a release pull request through
release-please, which bumps `version` in `herdr-plugin.toml` and writes the
changelog. Merging it tags the release, and the build job in the same workflow
cross-compiles all four platforms, writes `SHA256SUMS` and attaches them.

The build runs inside the release-please workflow rather than in one listening
for the tag. GitHub does not start a workflow from an event created with
`GITHUB_TOKEN`, so a tag that release-please pushes never fires an
`on: push: tags` trigger. That is how v0.1.0 came to be tagged and released
carrying no binaries, which sends every install to a source build. A tag pushed
by hand does fire, and `release.yml` covers that case; both call the same
reusable `build-release.yml`.

Three places carry the version and must agree: the git tag, `herdr-plugin.toml`,
and the download URL `scripts/install.sh` builds from it. The release workflow
fails a tag that disagrees with the manifest.

The `PLATFORMS` list in the `Makefile` and the `uname` cases in
`scripts/install.sh` name the same four targets. Changing one without the other
publishes assets nobody fetches, or fetches assets nobody published.

## Commit messages

Conventional commits, because release-please reads them to decide the version
and write the changelog. `feat:` and `fix:` appear in the changelog, `refactor:`
and `build:` do not.
