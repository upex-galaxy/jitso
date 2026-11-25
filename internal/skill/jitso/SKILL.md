---
name: jitso
description: Use when launching, delegating to or supervising another agent session (a worker, conductor or subagent) in a project that has jitso/jitso.yaml, when writing a brief for one, reading a JITSO launch card, or running jitso launch, close, reap, fleet, link, graph, build list, build validate or build draft. Also use for a JITSO REJECTED message, for a hook denial that names worker-start, start_worker, terminal create, build save, setup or install, and whenever the words worker, conductor, subagent, build, card, brief, route scope or fleet come up. Load it before spawning any agent session, even if the user never says JITSO, because in these projects only jitso launch may start one.
metadata:
  kind: workflow
---

# JITSO

JITSO launches every agent session from an approved **build** (model, effort, MCP servers, skills, rules). The launch line is composed in code, not typed by a model. Each launch is one line in `.jitso/launches.jsonl`, and every launch appends JITSO's short rules to the end of the session's `brief.md`.

## Start a worker: the only way

1. See what exists: `jitso build list` (launchable builds, with what each `requires`).
2. Write a brief to `.jitso/briefs/<name>.md` (the launcher reads briefs only from that folder). Rules and a filled example: `references/briefs.md`.
3. Preview, always: `jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode direct --dry-run`. It validates everything and prints the card without touching Orca.
4. Run it again without `--dry-run`. A real launch opens a paid session, so tell the human which build and brief before you do it, unless they already asked for exactly that.

`--mode` is required (a build's `mode_default` is not used). Fleet adds `--run <run>`: the Orca Run must exist before the launch (create it once with `orca orchestration run-create --objective <text> --json`). JITSO creates the Orca Task itself from `brief.md`, so there is no `--task`. `--worktree <sel>` picks the Orca worktree. A build that `requires` a field needs `--field name=value`. Full flag list: `references/launching.md`.

Never start a session any other way: not Orca worker start, not an Orca terminal or worktree that runs a harness, not the orca-bridge `start_worker` tool, not `claude`, `codex`, `opencode` or `pi` by hand. A launch outside `jitso launch` has no card and no registry line. The hook denies the Orca forms. A session without the guard is not stopped, so the rule binds you anyway.

When the hook denies something, the reply looks like this:

```
JITSO owns worker launch in this project. Use: jitso launch --build <id> --brief <file> --mode fleet --run <run> [--worktree <sel>]. ... (denied: orchestration worker-start)
```

Do not retry the denied call or look for a way around it. Go back to step 1 and use `jitso launch`. The text always shows the fleet shape. Use `--mode direct` when the human talks to the worker, `--mode fleet` when you are a conductor.

## Tiers

Conductor, worker and subagent are tiers of one build (`tier:` in its file, worker when absent). A conductor coordinates and starts workers. A worker does one job from a brief. A subagent is a helper inside a worker: it is never launched alone (`jitso launch` rejects it) and reaches the worker as `--agents` because the worker's build lists it in `subagents`.

## Modes

- `direct`: the worker may ask its human. The first prompt tells it to read the brief and carries the route scope.
- `fleet`: the worker never asks the human. It sends questions and its final report to the launcher named on the card. The card sits at the top of its brief.

## Read the card

Six lines: header (launch id, role/type, mode, build), model line (model, effort, MCP servers, skills to load first), subagents (a suggestion, not a limit), `ROUTE-SCOPE:`, the `launch:` command, the budget (only when the build has one) and who it reports to. Line by line: `references/card.md`.

## Brief rules in one breath

Headings `Goal`, `Scope`, `Done when`, `Report format`. English. Within the word limits the rejection prints. Every backticked file path (a slash and a file extension, no leading `@`, no trailing `/`) must exist, unless it sits on an `Output:` or `Create:` line. A rejection names the failing check, so fix that and run `--dry-run` again. A job already written in another format needs no rewrite: make the brief file the single line `Brief: <path to the card>` (`references/briefs.md`, "Pointing at a card you already have").

## After a launch

- `jitso close <launch id>` saves the screen footer, writes the end event and closes that terminal. It also prints the session's token counts when the transcript can be read.
- `jitso reap` ends launches whose terminal is gone and lists workers no launch explains.
- When a session ends, the SessionEnd hook writes its end event too. Details: `references/lifecycle.md`.

## Find the other sessions

`jitso fleet` is read only: it lists the live sessions of the project, one row each with launch id, role/type, harness, state, Orca handle, parent and title. The title is also the name of the session (`[jitso:<id>] <build>`), so a peer can be addressed by it. `--all` adds closed launches, `--json` prints an object for a program. A row marked `unlaunched` is a terminal no launch explains; `gone` is a launch whose terminal Orca no longer lists. Without Orca it still prints the registry rows, with state `unknown`, and exits 3. Details: `references/lifecycle.md`.

## Record who you talked to

An Orca message leaves a trace; a Claude Code `SendMessage` does not. After each `SendMessage`, run `jitso link <your launch id> <peer>`, where the peer is its launch id, its Orca handle or its exact session title in quotes. It appends one line to the log (who and when, never what) and prints what it recorded. A reference that names no session or several is rejected: take the id or the handle from `jitso fleet`. The human sees the result with `jitso graph`, a page on `127.0.0.1` that runs until Ctrl-C; `jitso graph --json` prints one snapshot for a program, from any folder. Details: `references/lifecycle.md`.

## Builds and drafts

`jitso build list`, `jitso build validate [<id>]` and `jitso build list --drafts` are read-only. When no build fits, propose one with `jitso build draft <file.yaml|->` and tell the human. A draft launches like any build, but only the owner turns it into an approved build. Where builds live and the draft flow: `references/builds.md`.

## Compose a build from the arsenal

When no build fits, ask what a build may name here: `jitso arsenal` prints one line per item (models, efforts, tools, skills, MCP servers, approved subagents, route ids, fields), each as a build writes it. `jitso arsenal --all` adds what exists in the project or on this machine but no allowlist names yet, each marked with the file the owner would edit. Compose the draft from the first list only. A user skill or user MCP server appears as `user:<name>` and needs the owner's `~/.jitso/jitso.yaml`. It prints names and key names, never a value. Steps and examples: `references/builds.md`.

## The rules at the end of brief.md

Every launch appends JITSO's rules to the end of `brief.md`, after your brief. They say: start other sessions only with the card's `launch:` line; when the build has a budget, the card's budget is your ceiling, so near it stop and report; load the card's skills before any other work; a direct session may ask the human, a fleet worker never does and sends its questions and its final report to the session that launched it; leave `jitso/`, `.jitso/` and `~/.jitso/` alone except your own draft in `.jitso/drafts/`, and never run `jitso build save`. Three more lines say how to reach another session: find it with `jitso fleet`, use `SendMessage` to its session name when both sessions are Claude Code and then run `jitso link`, or `orca orchestration send --to <handle>` otherwise, put your own handle and name in every message, and answer with `orca orchestration reply`. Read the end of your `brief.md` to see the exact text.

## Never run these

- `jitso build save`, `jitso setup` and `jitso install` change what is approved or how the project is wired. The guard denies them (`--dry-run` previews are allowed). If you need one, ask the owner to run it in a terminal.
- Hand edits of `jitso/builds/`, `jitso/jitso.yaml` or `.jitso/`.

## Exit codes

`0` done. `2` rejected, printed as `JITSO REJECTED: <reason>`: fix the input. `3` environment, printed as `jitso: <reason>`: something around the command is wrong (no git checkout, a missing file, Orca). `jitso doctor` reports that state in one pass and changes nothing. Messages and fixes: `references/troubleshooting.md`.

## References

| File | Read it when |
|---|---|
| `references/launching.md` | building a launch line, using fleet, `--parent`, `--mcp-exclude`, what a real launch writes and carries (the package folder) |
| `references/briefs.md` | writing a brief, or a brief was rejected |
| `references/card.md` | reading or explaining a card |
| `references/builds.md` | choosing, validating or drafting a build, composing one from `jitso arsenal`, `user:` names |
| `references/lifecycle.md` | closing, reaping, listing the sessions (`jitso fleet`), `jitso link`, `jitso graph`, the registry |
| `references/troubleshooting.md` | any `JITSO REJECTED`, exit 3, hook denial or doctor line |
