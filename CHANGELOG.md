# Changelog

All notable changes to this project are documented here.

This file is written by [release-please](https://github.com/googleapis/release-please)
from the conventional commit messages on `main`. Entries below the first
release heading were added by hand before that automation existed.

## Unreleased

The first release is not cut yet. Everything below is the work so far.

### Features

- An agent switcher ordered by the time of each session's last real message,
  which Herdr's own sidebar cannot produce. Five sort modes, single-key status
  filters matching Herdr's picker, type-to-filter search, and Enter to jump.
- Repository and worktree context on each row, shown only when the space name
  does not already imply it.
- A configurable layout in Herdr's own `rows` and token vocabulary, including
  per-token styles and ordered rules.
- Age graded into three colour bands: green under a day, yellow under a week,
  muted after that.

### Notes

- Ordering comes from the newest `user` or `assistant` entry in a transcript,
  never the file modification time. See [docs/design.md](docs/design.md).
