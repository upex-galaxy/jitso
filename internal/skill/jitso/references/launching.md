# Launching a worker

Read this when you are about to build a `jitso launch` line. The card line `launch:` prints the command to use in this project: `jitso launch`, or `bun run jitso launch` when its `package.json` has a `jitso` script.

## The line

```
jitso launch --build <id> --brief <file> --mode direct|fleet [--run <run>] [--worktree <sel>] [--repo <path>] [--field name=value]... [--parent <launch id>] [--mcp-exclude <server>]... [--brief-language en|es|any] [--dry-run]
```

| Flag | Meaning |
|---|---|
| `--build` | the build id: a saved build (`jitso build list`) or a draft (`jitso build list --drafts`) |
| `--brief` | the brief file, a regular file under `.jitso/briefs/` of the project (a relative path is read from your working folder). Its only line may be `Brief: <path>` to launch a card that lives elsewhere in the project (`briefs.md`) |
| `--mode` | required: `direct` or `fleet`. The build's `mode_default` is ignored by `launch` |
| `--run` | fleet only, required there: the Orca Run the worker is adopted into. It must already exist (`orca orchestration run-create --objective <text> --json`) |
| `--task` | not accepted in fleet mode: `JITSO REJECTED: --task is not accepted in fleet mode: JITSO creates the Task from the brief`. In direct mode it is ignored |
| `--worktree` | which Orca worktree the terminal opens in (an id or a selector); without it, Orca picks. `path:<folder>` of another repository launches into it ("Another repository") |
| `--repo` | launch into another repository: an absolute path (spaces allowed, at most 1024 bytes) inside one of its checkouts. Without `--worktree` the terminal opens at its primary checkout |
| `--field` | a value for a field the build `requires`, as `name=value`; repeat for more. The values reach the worker in the `## Fields` block of its `brief.md` and the `fields` object of the launch line in the log |
| `--parent` | the launch id of the launcher, so the registry records the tree (an unknown id is rejected, also on a dry run) |
| `--mcp-exclude` | drop one of the build's MCP servers for this launch, by the name the build wrote (`user:<name>` for a user server); repeat for more |
| `--brief-language` | `en` (default), `es` or `any`. Only change it when the project's briefs are not in English |
| `--dry-run` | validate and print the card, write nothing; Orca is not called, except to ask whether the target of another repository is registered |

A flag whose next word starts with `--` has no value: `launch --build --dry-run` is `JITSO REJECTED: --build needs a value`. Old spellings are unknown: `--preset` is `JITSO REJECTED: unknown argument "--preset"`.

`jitso launch --help` (or `-h`) as the first word after `launch` prints the usage on stdout and exits 0, in any folder. Anywhere else `--help` is an unknown argument. `jitso build --help` and `jitso install skill --help` print usage too; `close` and `reap` have no help screen.

An `--mcp-exclude` name that is not exactly one of the build's MCP servers excludes nothing and prints one warning on stderr, for example `jitso: warning: --mcp-exclude "DBHUB" is not an MCP server of build sonnet-tester (did you mean "dbhub"?); nothing excluded`. The launch goes on.

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

The dry run checks the build, the required fields, the brief file and `--parent`, and prints the brief warnings. It does not check the terminal, the folder Orca opens it in, a `--worktree` path of this repository, the trust dialog or `.mcp.json`, so a clean dry run is not a promise that the real launch succeeds.

## Another repository

The target is the repository of `--repo`, or the one a `--worktree path:<folder>` belongs to when it is not yours. `--repo` with a `--worktree` of any other repository is rejected. `--repo` naming your own repository is an ordinary launch.

| From your repository (home) | From the target |
|---|---|
| builds, drafts, briefs, `.jitso/`, `--parent` | skills, `.mcp.json` servers, route ids, instructions |
| the `models`, `efforts` and `fields` of `jitso/jitso.yaml` | the `skills`, `mcp` and `tools` lists of its own `jitso/jitso.yaml`, all open without one |

- The build is checked against that mix, so only the build you launch must be valid there. `jitso build draft <file|-> --repo <path>` drafts one that names the target's items; here it shows as invalid, which is expected.
- Orca must have the target registered. Otherwise exit 3 before anything is written, on a dry run too, with the `orca repo add --path <target>` line. Running it is your explicit sign of trust; `jitso launch` never runs it. A dry run with no Orca to ask warns and goes on.
- The card gains the line `target <path> | home <path>`, and its `launch:` command follows the target's `package.json`.
- The launch line records `repo` (the target's folder name), `repo_path` and `home`.
- `run.sh` exports `JITSO_PROJECT=<home>`, and the guard travels with it even when the target registers one. In that session every `jitso` command that reads builds or the registry works on home, and the guard blocks launches although the target has no `jitso/jitso.yaml`. `init`, `setup`, `install` and `doctor` still act on the folder they run in.
- A launch from that session with no `--repo` goes to home: JITSO opens its terminal at home's primary checkout. Pass `--repo <target>` to stay in the target.

## Direct and fleet

Direct (a human works with the worker):

```
jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode direct
```

Fleet (a conductor starts workers that report back to it, never to the human):

```
jitso launch --build <id> --brief .jitso/briefs/<name>.md --mode fleet --run <run> [--worktree <sel>] --parent <your launch id>
```

Fleet without `--run` is `JITSO REJECTED: fleet mode requires --run`. In fleet mode the launcher also creates the Orca Task and adopts the terminal into the run with `orchestration worker-start --spec-file <launch folder>/brief.md`, so the worker starts by reading its brief and the JITSO rules. That call belongs to `jitso launch`, never type it yourself (the guard denies it), and never create a Task for the worker first. The launch line records the Task id that Orca returns; when the reply names none, the launch is still registered with a null task and a warning.

A conductor starts every worker with the `launch:` command of its own card plus the fleet flags. Nothing else may start a worker.

## What a real launch does

In this order (read from the launch code, then seen with a fake Orca on a scratch project):

1. Resolves the build and reads the brief. Any rejection stops here with exit 2; a failed brief check is only a warning. A `--worktree` that names a path (`path:<path>` or `id:<id>::<path>`) must be an absolute path of a worktree of the target repository (yours, unless it is another one), or the launch is rejected here, before anything is written. A selector with no path (a worktree name) is not checked here.
2. Creates `.jitso/launches/<id>/` and writes into it `brief.md` (the card, the skills to load first, your brief, a `## Fields` block when you gave `--field` values, then JITSO's rules), `mcp.json`, `card.txt` and `run.sh`.
3. Opens an Orca terminal titled `[jitso:<id>] <build>` running `run.sh`. The harness session gets the same text as its name where the harness has a launch flag for one (`--name` for Claude Code and Pi; Codex and OpenCode have none for their interactive session). Right away, before it waits for anything, it checks the folder Orca reports for that terminal, on every launch, with or without the trust dialog: it must be a worktree of the target repository, and the path `--worktree` names when it names one. A wrong or unknown folder closes the terminal and rejects the launch (exit 2). Then it waits for the harness prompt and answers the trust dialog.
4. In fleet mode, creates the Task from `brief.md` and adopts the terminal into the run.
5. Appends the launch line to `.jitso/launches.jsonl` and prints the card.

If anything fails before step 5, the launcher closes the terminal it opened and removes the launch folder, so nothing half-made stays. After step 5 nothing is removed.

`run.sh` starts the harness of the build (`harness:` is `claude`, `codex`, `opencode` or `pi`). For Claude Code: the build's model and effort, bypassed permissions, project settings only (`--setting-sources project`), the session name (`--name`, the terminal title), a strict MCP file that holds only the servers the build names, and the build's subagents as `--agents`; user settings, user hooks and any user skill or server the build does not name do not reach the session. The other harnesses honor less, and a build that asks for what its harness cannot give is rejected by `jitso build validate` and `launch` with a text that names the harness: codex keeps the owner's own Codex MCP servers (a note line after the card says so) and loads no package skills, so install the jitso skill with `jitso init`; opencode runs with its own config folder; pi gets no MCP server at all. Only a Claude launch records tokens at `jitso close`.

## The package folder

When a launch carries more than its own files, it also writes `.jitso/launches/<id>/pkg/` and passes it with `--add-dir <launch folder>/pkg`, right after `--mcp-config`. It holds:

| What | When it travels |
|---|---|
| each `user:` skill of the build, copied under `pkg/.claude/skills/<name>/` | the build names it and the owner's `~/.jitso/jitso.yaml` lists it |
| the jitso usage skill | the build is `tier: conductor` or names `jitso`, and the checkout holds no `.claude/skills/jitso/SKILL.md` |
| the guard hook | not in the folder: it is added to the `--settings` JSON, unless the project's `.claude/settings.json` already registers it on all three slots. Into another repository it always travels |

A launch that carries nothing writes no `pkg/` and passes no `--add-dir`. The launch event records what travelled under `package` (`references/lifecycle.md`). User MCP servers go into `mcp.json` by reference only: a `${VAR}` stays a reference, and a variable that is unset in jitso's environment is one stderr warning that names it.

## Preconditions

- You are inside a git checkout of the project. A linked worktree works: the project is its primary checkout. `jitso/jitso.yaml` is not required: it switches the guard on and may restrict builds.
- A real launch reads `.mcp.json` in the project only when it still names an MCP server after `--mcp-exclude`. A build with no servers needs no `.mcp.json`.
