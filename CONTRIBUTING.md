# Contributing

## Build and test

```bash
make build   # bin/herdr-switcher-plus
make ci      # gofmt, vet, generated docs, tests, coverage floor, e2e
make e2e     # run the built binary on a pseudo terminal, against a fake Herdr
make link    # build, then link this working copy into the running Herdr session
make run     # print the agent list without opening a pane
```

`make help` lists every target.

## Screenshots

`make demo` opens the switcher on fabricated sessions. Take documentation
screenshots from that, never from a real list. `docs/images/switcher.png` and
the `docs/images/tree*.png` images were taken that way.

A real list names your employer, your colleagues, your private repositories and
whatever the last message happened to say, and a README is public forever. The
fixture in `scripts/demo_fixture.py` is invented, and is built to show what the
switcher is for: a repository with several linked worktrees, a space whose name
hides its branch, an ordinary checkout in a differently named space, a space
that is no checkout at all, and ages spanning all three colour bands.

It runs the real rendering path. Nothing about the picture is faked except the
sessions, so it stays honest while carrying nothing real.

Test fixtures use the same invented names, for the same reason.

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

## End-to-end tests

`make e2e` builds the binary and runs it as Herdr does: a process with the
Herdr environment set, a Herdr CLI on `HERDR_BIN_PATH` and a Herdr socket on
`HERDR_SOCKET_PATH`. The unit tests replace every seam. These replace none, so
they prove that the seams join up. The tests are in `e2e/`, behind the `e2e`
build tag, so `go test ./...` does not run them.

- `e2e/testdata/herdr` is the fake CLI. It records every call and answers the
  snapshot and the session list. `FAKE_HERDR_FAIL=<args>` makes a call fail.
- `e2e/socket_test.go` is the fake socket. It records each request, and it
  fails a method on demand.
- `e2e/term_test.go` runs the binary on a pseudo terminal. It answers the two
  startup questions, and it keeps a virtual screen so that a test reads what is
  visible now, not what was ever printed.
- The sessions come from `scripts/demo_fixture.py`, so they are invented.

The suite needs python3 and no Herdr session. It starts with an empty
environment, so a run from inside a Herdr pane cannot reach that pane's
session. `E2E_BIN=<path> make e2e` tests a given binary, such as a release
asset, in place of a fresh build.

Add a test here when a change crosses a seam: a new flag, a new key, a new
call to Herdr. A change inside one package belongs in that package's tests.

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
