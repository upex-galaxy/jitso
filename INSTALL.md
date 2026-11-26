# Install JITSO

This page is written for an AI agent that was asked to "install JITSO" or "set up JITSO in this project", and for the person who asked. Follow the steps in order. Each step says how to check it worked. Stop and report at the first step that fails.

JITSO needs macOS or Linux (or WSL) for `jitso launch` (it runs `/bin/sh`). On Windows every other command works, launching does not: `jitso launch` stops with one line there, and `--dry-run` still checks a launch.

## 1. Is it installed?

```sh
jitso version
```

If it prints `jitso v...`, go to step 3. If the command is not found, go to step 2.

## 2. Install the binary

```sh
curl -fsSL https://github.com/upex-galaxy/jitso/releases/latest/download/install.sh | sh
```

The script installs to `~/.local/bin/jitso`, checks the SHA-256 of the archive, and needs no `sudo`. When `~/.local/bin` is not on `PATH`, it adds one marked line to the profile of your shell (`~/.zshrc`, `~/.bashrc`, `~/.config/fish/conf.d/jitso.fish`, else `~/.profile`) and says which file it changed. Open a new terminal afterwards, or run the `export PATH=...` line it prints. Set `JITSO_NO_MODIFY_PATH=1` to keep your profiles untouched.

On Windows, use PowerShell instead: download `install.ps1` from the latest release, read it, and run it.

```powershell
irm https://github.com/upex-galaxy/jitso/releases/latest/download/install.ps1 -OutFile install.ps1
.\install.ps1
```

It installs `jitso.exe` into `%LOCALAPPDATA%\Programs\jitso`, checks the SHA-256 of the zip against `checksums.txt`, needs no administrator rights, and appends that folder to your user `PATH` once (set `JITSO_NO_MODIFY_PATH=1` to skip that). Open a new terminal afterwards.

Check: `jitso version` prints a version.

## 3. Set it up

Run this from the root of the project (a git checkout). Run it from any other folder to set up only your user account.

```sh
jitso init --yes
```

`jitso init` does every setup step in one run:

- installs the `jitso` skill for every harness, in `~/.agents/skills/jitso` (Claude Code reads it through `~/.claude/skills/jitso`);
- inside a project, creates `jitso/jitso.yaml` with no allowlist (everything open) when it is missing, adds `.jitso/` to `.gitignore`, writes the JITSO block into `AGENTS.md` (replacing the block of an older release), and registers the guard for every harness it finds on `PATH`: Claude Code (`.claude/settings.json`), Codex (`.codex/hooks.json`), OpenCode (`.opencode/plugins/jitso-guard.js`) and Pi (`.pi/extensions/jitso-guard.js`).

It prints every change before it writes, keeps a backup of every file it changes, and `jitso init --remove` undoes it. Options:

| Option | Effect |
|---|---|
| `--project-only` | put the skill in the project instead, and write nothing under the home folder |
| `--user-only` | set up the user account only, even inside a project |
| `--harness claude,codex,opencode,pi` | choose the harnesses instead of the ones found on `PATH` |
| `--dry-run` | show the changes and write nothing |
| `--remove` | undo the skill, the block and the guard (`jitso/jitso.yaml` and the `.gitignore` line stay) |

Without `--yes` on a terminal, `jitso init` asks a few questions and then asks before writing.

Check: `jitso doctor` ends with `0 fail`. Two lines may still fail on a new machine: `fail orca` until Orca is installed (step 4), and `fail claude` when no harness at all is installed (with Codex, OpenCode or Pi present it is only a warning). A `warn hooks` that says a file is not tracked by git goes away when the person commits the new files.

## 4. Harness notes

- **Codex** runs project hooks only after the person trusts them. The first `codex` session in the project shows "Hooks need review": choose "Trust all and continue". `codex exec` skips untrusted hooks without a word.
- **Orca** opens the terminal of each worker. `jitso doctor` says when it is missing. Install it from https://github.com/stablyai/orca and open it once.

## 5. First build

A build is the profile a worker is launched with. See what it may name, then compose one as a draft:

```sh
jitso arsenal
printf 'id: scout\nmodel: <model>\neffort: low\n' | jitso build draft -
jitso launch --build scout --brief .jitso/briefs/<name>.md --mode direct --dry-run
```

Run the last line without `--dry-run` to start the worker. A real launch opens a paid session, so ask the person first. `jitso build save scout` keeps the build as `jitso/builds/scout.yaml`, which is optional. `jitso build new <id>` writes a commented template instead. The `jitso` skill explains builds, briefs, cards and launches.

## What to report back

Say which steps ran, paste the last line of `jitso doctor`, and list every `warn` or `fail` line it printed.
