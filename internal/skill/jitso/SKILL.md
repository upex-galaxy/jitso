---
name: jitso
description: Use when launching, delegating to or supervising another agent session (a worker, conductor or subagent) in a project that has jitso/jitso.yaml, when writing a brief for one, composing a build for it, reading a JITSO launch card, or running jitso launch, arsenal, close, reap, fleet, link, graph, build list, build validate, build draft or build save. Also use for a JITSO REJECTED message, a jitso brief warning, a hook denial that names worker-start, start_worker, terminal create or a harness (claude, codex, opencode, pi), and whenever the words worker, conductor, subagent, build, card, brief, route scope or fleet come up. Load it before spawning any agent session, even if the user never says JITSO, because in these projects only jitso launch may start one.
metadata:
  kind: workflow
---

# JITSO

JITSO launches every agent session from a **build** (model, effort, MCP servers, skills, rules). A build is saved in `jitso/builds/` or composed just in time as a draft. The launch line is composed in code, not typed by a model. Each launch is one line in `.jitso/launches.jsonl`, and every launch appends JITSO's short rules to the end of the session's `brief.md`.

## Start a worker

1. Find a build: `jitso build list` (saved, launchable, with what each `requires`). When none fits, compose one (below).
2. Write a brief to `.jitso/briefs/<name>.md` (the launcher reads briefs only from that folder). Rules and a filled example are in `references/briefs.md`.
3. Preview, always: `jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode direct --dry-run`. It validates, prints the card and any brief warning, and does not touch Orca.
4. Run it again without `--dry-run`. A real launch opens a paid session, so tell the human which build and brief before you do it, unless they already asked for exactly that.

`--mode` is required (a build's `mode_default` is not used). Fleet adds `--run <run>`: the Orca Run must exist before the launch (create it once with `orca orchestration run-create --objective <text> --json`). JITSO creates the Orca Task itself from `brief.md`, so there is no `--task`. `--worktree <sel>` picks the Orca worktree. `--name <name>` sets the session name peers address (default: the build id). A build that `requires` a field needs `--field name=value`. Full flag list in `references/launching.md`.

## Compose a build just in time

1. `jitso arsenal` prints what a build may name here, one line per item, as a build writes it. `(open)` means nothing restricts that kind.
2. Pipe the build to `jitso build draft -` (or name a `.yaml` file). `id`, `model` and `effort` are enough:

   ```
   printf 'id: scout\nmodel: <model>\neffort: low\n' | jitso build draft -
   ```

3. Launch it with `--build scout`, as above. Its card ends `(draft)`.
4. Optional: `jitso build save scout` keeps it as `jitso/builds/scout.yaml`. A saved build lasts and is the project's to commit; a draft stays local.

Fields, tiers and `user:` names are in `references/builds.md`.

## Launch a worker in another repository

Your repository (home) keeps the builds, the briefs and the registry; the target gives the worker its skills, MCP servers, route ids and instructions. The target needs no `jitso/jitso.yaml`.

1. `jitso arsenal --repo <absolute path>` lists what a build for it may name.
2. A saved or drafted build that fits both launches as it is. One that names what only the target has: `jitso build draft <file|-> --repo <path>`. It stays a draft.
3. `jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode <mode> --repo <path> --dry-run`, then without `--dry-run`. The terminal opens at the target's primary checkout; add `--worktree path:<P>` for one of its worktrees.
4. Exit 3 with `is not registered in Orca`: run the `orca repo add --path <path>` line the message prints, which trusts that folder for Orca, so tell the human, then launch again. `jitso launch` never registers a repository.

The worker's `jitso` calls work on your repository (`JITSO_PROJECT`), so `fleet`, `close`, `link` and `--parent` see one fleet. A child it launches with no `--repo` opens in your repository, and its brief goes in your `.jitso/briefs/` (the card's `home` path).

## What the guard blocks

Only starting an agent outside `jitso launch`: Orca worker start, an Orca terminal or worktree that runs a harness, the orca-bridge `start_worker` tool, and `claude`, `codex`, `opencode` or `pi` run to open a session. A launch outside `jitso launch` has no card and no registry line, so the rule binds a session without the guard too. A denial looks like this:

```
JITSO owns worker launch in this project. Use: jitso launch --build <id> --brief <file> --mode fleet --run <run> [--worktree <sel>]. ... (denied: orchestration worker-start)
```

Do not retry the denied call or look for a way around it. Use `jitso launch`. The text shows the fleet shape: use `--mode direct` when the human talks to the worker, `--mode fleet` when you are a conductor.

## Tiers

Conductor, worker and subagent are tiers of one build (`tier:` in its file, worker when absent). A conductor coordinates and starts workers. A worker does one job from a brief. A subagent is a helper inside a worker: it is never launched alone (`jitso launch` rejects it) and reaches the worker as `--agents` because the worker's build lists it in `subagents`.

## Modes

- `direct`: the worker may ask its human. The first prompt tells it to read the brief and carries the route scope.
- `fleet`: the worker never asks the human. It sends questions and its final report to the launcher named on the card. The card sits at the top of its brief.

## Read the card

Six lines: header (launch id, role/type when the build names them, mode, build, session name), model line (model, effort, MCP servers, skills to load first), subagents (a suggestion, not a limit), `ROUTE-SCOPE:`, the `launch:` command, the budget (only when the build has one) and who it reports to. Line by line in `references/card.md`.

## Briefs

Aim for headings `Goal`, `Scope`, `Done when` and `Report format`, English, a length inside the limits a warning prints, and backticked file paths that exist (a file the worker will create goes on an `Output:` or `Create:` line). A miss is a `jitso: warning: brief ...` line on stderr and the launch goes on: fix it, then dry-run again. A job already written elsewhere needs no rewrite: make the brief file the single line `Brief: <path to the card>`.

## After a launch

- `jitso close <launch id>` saves the screen footer, writes the end event and closes that terminal. It also prints the session's token counts when the transcript can be read.
- `jitso reap` ends launches whose terminal is gone and lists workers no launch explains.
- When a session ends, the SessionEnd hook writes its end event too. Details in `references/lifecycle.md`.

## Find the other sessions

`jitso fleet` is read only: it lists every session Orca lists, this project first (its launches count wherever they run), one row each with launch id, `NAME`, role/type, harness, state, Orca handle, parent, `REPO` and title. Address a peer by its `NAME`, which is its Claude Code or Pi session name; the tab title is `<name> [jitso:<id>]` and is not the name. `--all` adds closed launches, `--json` prints an object for a program. A row marked `unlaunched` is a terminal no launch of this project explains; `gone` is a launch whose terminal Orca no longer lists. Without Orca it prints the registry rows, with state `unknown`, and exits 3.

## Record who you talked to

An Orca message leaves a trace; a Claude Code `SendMessage` does not. After each `SendMessage`, run `jitso link <your launch id> <peer>`, where the peer is its launch id, its Orca handle, its session name or its exact tab title in quotes. It records who and when, never what. A reference that names no session or several is rejected: take the id or the handle from `jitso fleet`. The human sees the result with `jitso graph`, a page on `127.0.0.1` that runs until Ctrl-C; `jitso graph --json` prints one snapshot for a program.

## The rules at the end of brief.md

They say: start other sessions only with the card's `launch:` line; when the build has a budget it is your ceiling, so near it stop and report; load the card's skills before any other work; a direct session may ask the human, a fleet worker never does and sends its questions and final report to the session that launched it; never hand-edit `.jitso/launches.jsonl` or `.jitso/launches/`. Three more lines say how to reach another session: find it with `jitso fleet`, use `SendMessage` then `jitso link` between two Claude Code sessions, or `orca orchestration send --to <handle>` otherwise, and also when a `SendMessage` is held, declined or not delivered, put your own handle and name in every message, and answer with `orca orchestration reply`.

## Ask before you change the setup

`jitso init`, `jitso setup` and `jitso install` change how the project and the user account are wired (the guard, the `AGENTS.md` block, the skill). Run them when the human asked.

## Exit codes

`0` done. `2` rejected, printed as `JITSO REJECTED: <reason>`: fix the input. `3` environment, printed as `jitso: <reason>`: something around the command is wrong (no git checkout, a missing file, Orca). `jitso doctor` reports that state in one pass and changes nothing.

## References

| File | Read it when |
|---|---|
| `references/launching.md` | building a launch line, using fleet, `--parent`, `--mcp-exclude`, what a real launch writes and carries |
| `references/briefs.md` | writing a brief, or a launch printed a brief warning or rejection |
| `references/card.md` | reading or explaining a card |
| `references/builds.md` | composing, drafting, validating or saving a build, the allowlists, `user:` names |
| `references/lifecycle.md` | closing, reaping, `jitso fleet`, `jitso link`, `jitso graph`, the registry |
| `references/troubleshooting.md` | any `JITSO REJECTED`, exit 3, hook denial or doctor line |
