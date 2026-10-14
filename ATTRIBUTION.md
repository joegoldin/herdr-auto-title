# Attribution

This is a fork of [kryptamine/herdr-auto-title](https://github.com/kryptamine/herdr-auto-title),
based on **v0.13.2** (commit `8b25045`), Copyright (c) 2026 Alexander
Satretdinov, licensed under the MIT License. The notice is kept unchanged in
[`LICENSE`](LICENSE). Everything not listed below is upstream's.

The Go module path stays `github.com/kryptamine/herdr-auto-title`, so upstream
releases merge without rewriting imports.

## Changes

- **Title order.** `resolver.Format` joins a title's parts as activity, agent,
  context, branch, where upstream reads context, branch, agent, activity. A
  tab now leads with what it is about (`PR 6583 performance review › main~1`),
  and a title cut to `HERDR_AUTO_TITLE_MAX_LENGTH` loses the branch before the
  activity. The workspace `topic` token, built by the same function, follows
  the same order.
- **jj repositories.** A directory inside a jj workspace (colocated or not) is
  labelled the way the jj-starship prompt names it: the nearest bookmark on or
  behind the working copy with its distance (`main~1`), or the change id when
  no bookmark is near. This replaces the detached commit hash git reports for
  a colocated repository. Unlike a git branch, a jj label naming the trunk is
  shown, since the distance is what it says. A long bookmark is shortened like
  a branch, keeping its `~N` whole.
- **New package `internal/jj`.** Runs `jj-starship` (`--cwd <root>
  --no-color --no-symbol --no-jj-prefix --no-jj-status`), the first command
  this plugin runs: jj keeps its state in a store upstream's file-reading
  approach does not cover. A label is cached per workspace until the
  repository's `op_heads` changes, so a poll costs one `stat` per repository
  while nothing changes. A run is bounded by a one-second timeout, and a
  missing `jj-starship` is logged once and leaves jj repositories unlabelled.
- **`git.Checkout.JJ`.** The field carrying that label; `internal/reads` fills
  it in place of `Branch` and `Commit`.
