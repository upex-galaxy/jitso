# Claude

<!-- BEGIN JITSO block=1 added=1: managed by "jitso setup instructions"; edits inside this block are replaced -->
## JITSO

This machine launches agent sessions with JITSO. In a project that has `jitso/jitso.yaml`:

- Start a worker session only with `jitso launch` or the launch line a JITSO card prints. Never start an agent another way: no Orca worker start, no terminal or worktree that runs a harness, no raw `claude` process. A hook denies those.
- Launch from an approved build (`jitso build list` shows them). Write the brief first, then run the launch with `--dry-run` to see the card.
- Read the card `jitso launch` prints: its model, subagents, route scope, launch line and budget are the limits of that session.
- Do not edit `jitso/`, `.jitso/` or this block by hand, and do not run `jitso build save`, `jitso setup` or `jitso install`: they are the owner's.
- Load the `jitso` skill for briefs, cards, modes and the lifecycle (`jitso close`, `jitso reap`, `jitso analyze`). `jitso doctor` checks the setup.
<!-- END JITSO block=1 -->
