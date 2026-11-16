# Launching a worker

Read this when you are about to build a `jitso launch` line. The card line `launch:` prints the command to use in this project: `jitso launch`, or `bun run jitso launch` when its `package.json` has a `jitso` script.

## The line

```
jitso launch --build <id> --brief <file> --mode direct|fleet [--run <run> --task <task>] [--worktree <sel>] [--field name=value]... [--parent <launch id>] [--mcp-exclude <server>]... [--brief-language en|es|any] [--dry-run]
```

| Flag | Meaning |
|---|---|
| `--build` | the build id; it must be a launchable build of this project (`jitso build list`) |
| `--brief` | the brief file, a regular file under `.jitso/briefs/` of the project (a relative path is read from your working folder) |
| `--mode` | required: `direct` or `fleet`. The build's `mode_default` is ignored by `launch` |
| `--run`, `--task` | fleet only, both required there: the Orca run and task the worker is adopted into |
| `--worktree` | which Orca worktree the terminal opens in (an id or a selector); without it, the primary |
| `--field` | a value for a field the build `requires`, as `name=value`; repeat for more |
| `--parent` | the launch id of the launcher, so the registry records the tree (an unknown id is rejected, also on a dry run) |
| `--mcp-exclude` | drop one of the build's MCP servers for this launch; repeat for more |
| `--brief-language` | `en` (default), `es` or `any`. Only change it when the project's briefs are not in English |
| `--dry-run` | validate and print the card, write nothing, never call Orca |

A value that starts with `--` is read as the value of the flag before it, so write `--build <id>` with a real id. Old spellings are unknown: `--preset` is `JITSO REJECTED: unknown argument "--preset"`.

`launch --help` is not a help screen: it is the same rejection as any unknown argument. `jitso build --help` and `jitso install skill --help` do print usage.

## Dry run first

Example from a test fixture project (your builds, ids and numbers differ):

```
$ jitso launch --build sonnet-automator --brief .jitso/briefs/ok.md --mode direct --dry-run --field ticket_key=PROJ-12
JITSO L<yyyymmdd>T<hhmmss>-<hex> | operator/automator | mode direct (asks you) | build sonnet-automator v1
model claude-sonnet-5-5 effort medium | bypass | MCP dbhub,openapi | skill test-automation
subagents plan,code,verifier (suggested; others allowed)
ROUTE-SCOPE: code-quickref,ticket-work
launch: jitso launch
budget 400k tokens (100%), list cap US$ 6.00 | reports to: you | log .jitso/launches.jsonl
dry run: nothing was written and Orca was not called
```

The launch id in a dry run is only a preview. A real launch draws a new one.

The dry run checks the build, the required fields, the brief and `--parent`. It does not check the terminal, the folder Orca opens it in, a `--worktree` path, the trust dialog or `.mcp.json`, so a clean dry run is not a promise that the real launch succeeds.

## Direct and fleet

Direct (a human works with the worker):

```
jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode direct
```

Fleet (a conductor starts workers that report back to it, never to the human):

```
jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode fleet --run <run> --task <task> [--worktree <sel>] --parent <your launch id>
```

Fleet without `--run` and `--task` is `JITSO REJECTED: fleet mode requires --run and --task`. In fleet mode the launcher also adopts the terminal into the Orca run with `orchestration worker-start`; that call belongs to `jitso launch`, never type it yourself (the guard denies it).

A conductor starts every worker with the `launch:` command of its own card plus the fleet flags. Nothing else may start a worker.

## What a real launch does

In this order (read from the launch code, then seen with a fake Orca on a scratch project):

1. Resolves the build and checks the brief. Any rejection stops here with exit 2. A `--worktree` that names a path (`path:<path>` or `id:<id>::<path>`) must be an absolute path of a worktree of this repo, or the launch is rejected here, before anything is written or Orca is called. A selector with no path (a worktree name) is not checked here.
2. Creates `.jitso/launches/<id>/` and writes into it `brief.md` (the card, the skills to load first, then your brief), `core.md` (the hard rules), `mcp.json`, `card.txt` and `run.sh`.
3. Opens an Orca terminal titled `[jitso:<id>] <build>` running `run.sh`. Right away, before it waits for anything, it checks the folder Orca reports for that terminal, on every launch, with or without the trust dialog: it must be a worktree of this repo (or the dispatcher folder), and the path `--worktree` names when it names one. A wrong or unknown folder closes the terminal and rejects the launch (exit 2). Then it waits for the harness prompt and answers the trust dialog.
4. In fleet mode, adopts the terminal into the run.
5. Appends the launch line to `.jitso/launches.jsonl` and prints the card.

If anything fails before step 5, the launcher closes the terminal it opened and removes the launch folder, so nothing half-made stays. After step 5 nothing is removed.

`run.sh` starts the harness with the build's model and effort, bypassed permissions, project settings only (`--setting-sources project`), a strict MCP file that holds only the servers the build names, the build's subagents as `--agents`, and the hard rules appended to the system prompt.

## Preconditions

- You are inside a git checkout of the project, and `jitso/jitso.yaml` exists (the allowlists). A linked worktree works: the project is its primary checkout.
- A real launch also needs `.mcp.json` in the project, even for a build that names no MCP server (read from the code, not measured here).
- `JITSO_PROJECT` names the project when set. The dispatcher uses it. You should not need it.

## The dispatcher

`jitso "<a request with spaces>"` is a different entry: it starts an interactive Claude Code session (the dispatcher) that can only write a brief and run one `jitso launch`. It is the human's door, and it starts a real session at once. Do not use it from an agent session.
