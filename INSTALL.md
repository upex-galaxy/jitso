# Install JITSO

This page is written for an AI agent that was asked to "install JITSO" or "set up JITSO in this project", and for the person who asked. Follow the steps in order. Each step says how to check it worked. Stop and report at the first step that fails.

JITSO needs macOS or Linux for `jitso launch` (it runs `/bin/sh`). On Windows the commands that only read or write files work, launching does not.

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

Check: `jitso version` prints a version.

## 3. Set it up

Run this from the root of the project (a git checkout). Run it from any other folder to set up only your user account.

```sh
jitso init --yes
```

`jitso init` does every setup step in one run:

- installs the `jitso` skill for every harness, in `~/.agents/skills/jitso` (Claude Code reads it through `~/.claude/skills/jitso`);
- inside a project, creates `jitso/jitso.yaml` with empty allowlists when it is missing, adds `.jitso/` to `.gitignore`, writes the JITSO block into `AGENTS.md`, and registers the guard for every harness it finds on `PATH`: Claude Code (`.claude/settings.json`), Codex (`.codex/hooks.json`), OpenCode (`.opencode/plugins/jitso-guard.js`) and Pi (`.pi/extensions/jitso-guard.js`).

It prints every change before it writes, keeps a backup of every file it changes, and `jitso init --remove` undoes it. Options:

| Option | Effect |
|---|---|
| `--project-only` | put the skill in the project instead, and write nothing under the home folder |
| `--user-only` | set up the user account only, even inside a project |
| `--harness claude,codex,opencode,pi` | choose the harnesses instead of the ones found on `PATH` |
| `--dry-run` | show the changes and write nothing |
| `--remove` | undo the skill, the block and the guard (`jitso/jitso.yaml` and the `.gitignore` line stay) |

Without `--yes` on a terminal, `jitso init` asks a few questions and then asks before writing.

If `jitso init` is denied with `JITSO: only the owner changes the JITSO setup`, a JITSO guard is already active in this session. Do not try another way. Give the person the exact command to run in their own terminal, and continue when they say it is done.

Check: `jitso doctor` ends with `0 fail`. Two lines may still fail on a new machine: `fail orca` until Orca is installed (step 4), and `fail claude` when no harness at all is installed (with Codex, OpenCode or Pi present it is only a warning). A `warn hooks` that says a file is not tracked by git goes away when the person commits the new files.

## 4. Harness notes

- **Codex** runs project hooks only after the person trusts them. The first `codex` session in the project shows "Hooks need review": choose "Trust all and continue". `codex exec` skips untrusted hooks without a word.
- **Orca** opens the terminal of each worker. `jitso doctor` says when it is missing. Install it from https://github.com/stablyai/orca and open it once.

## 5. First build

A build is the approved profile a worker is launched with. Create one as a draft, fill it, check it, then the person saves it:

```sh
jitso build new <id>
# edit jitso/jitso.yaml (the allowlists) and .jitso/drafts/<id>.yaml
jitso build validate <id>
jitso build save <id>     # the person runs this; agents are denied
```

Then `jitso launch --build <id> --brief <file> --mode direct` starts a worker. The `jitso` skill explains briefs, cards and launches.

## What to report back

Say which steps ran, paste the last line of `jitso doctor`, and list every `warn` or `fail` line it printed.
