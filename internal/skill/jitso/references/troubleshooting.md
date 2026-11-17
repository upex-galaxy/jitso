# Troubleshooting

Two prefixes tell you whose problem it is. `JITSO REJECTED: <reason>` (exit 2) means your input was refused: fix it and run again. `jitso: <reason>` (exit 3) means the environment around the command is wrong: a missing file, no git checkout, Orca. Read the whole message first, since most of them name the file or the field.

## Rejections (exit 2)

| Message | Cause and fix |
|---|---|
| `--build is required`, `--brief is required` | a required flag is missing |
| `--mode must be direct or fleet` | `--mode` is required and has no default. A build's `mode_default` is not used |
| `fleet mode requires --run and --task` | fleet needs both. Direct mode needs neither |
| `unknown argument "<x>"` | a flag `launch` does not have, a stray word, or an old spelling such as `--preset`. `--help` after the first word ends here too (as the first word it prints the usage) |
| `--<name> needs a value` | the flag is the last word, or the next word starts with `--` (a value may not) |
| `jitso: warning: --mcp-exclude "<name>" is not an MCP server of build <id>; nothing excluded` | a warning, exit 0: the build does not list that name, so nothing was left out. A `(did you mean "<server>"?)` hint means the name differs only by case. Fix the name if you meant to drop a server |
| `--field must look like name=value` | write `--field ticket_key=PROJ-12` |
| `missing ticket_key` (any field the build `requires`) | pass `--field ticket_key=<value>` |
| `field ticket_key has an invalid value` | the value does not match the field's pattern. Read the build's description for the shape |
| `unknown field "foo" for build "<id>"` | the build does not require that field. Drop the flag |
| `unknown build "<id>"` | no build has that id for this project. Check `jitso build list`, `jitso build list --drafts` and `jitso build validate` |
| `build "<id>" has tier subagent: it is not launched alone, a worker lists it in its subagents` | launch the worker that lists it |
| `build "<worker>" lists subagent "<s>", which is <status>, not approved` | a subagent must be approved. Ask the owner, or pick another build |
| `build "<id>" from <layer> <path> is invalid here: <problems>` | a draft does not fit this project's allowlists. `jitso build validate <id>` |
| `brief failed: sections (...)`, `length (...)`, `language (...)`, `paths (...)` | see `briefs.md`; the text names what is wrong |
| `brief file does not exist`, `brief path is not a regular file`, `brief path is empty or contains a NUL byte` | the `--brief` path is wrong |
| `brief file is outside the allowed folders` | move the brief to `.jitso/briefs/` of the project |
| `unknown parent launch <id>` | `--parent` names an id that is not in `.jitso/launches.jsonl` |
| `unknown launch <id>` (close) | the id is not in this project's log. Check the primary checkout's `.jitso/launches.jsonl` |
| `usage: jitso close <launch_id>`, `usage: jitso reap [--run <run_id>]...` | the arguments are wrong. These commands have no `--help` |
| `untrusted folder <path>: a launch runs only in a worktree of this repo; the terminal was closed` | Orca opened the terminal in a folder that is not a worktree of this repo, or reported none (`(unknown)`). The folder is checked on every launch, with or without the trust dialog. Check `--worktree` |
| `untrusted folder <path>: --worktree asked for <path>; the terminal was closed` | `--worktree` names a path and Orca opened the terminal somewhere else. Name the worktree you want, or drop the flag |
| `--worktree <selector> must name an absolute path`, `--worktree <selector> names <path>, which is not a worktree of this repo` | a `path:` or `id:<id>::<path>` selector was rejected before anything was written or Orca was called. Use an absolute path of a worktree of this repo |
| `draft <...> is invalid: <problems>`, `build "<id>" already exists (<layer> <path>): a draft needs an id no build has` | fix the draft, or pick an id no saved build has |
| `saving a build needs the owner: read the file above, then run it again with --yes, or in a terminal` | not yours to run. Show the owner the draft and ask |

## Environment errors (exit 3)

| Message | Cause and fix |
|---|---|
| `jitso: <folder> is not inside a git checkout` | `cd` into the project |
| `jitso: jitso/jitso.yaml is missing: this project has no JITSO allowlists (create it with jitso build new <id>)` | the project has not adopted JITSO yet. Tell the owner |
| `build <file> is invalid: <problems>` | one file of the project's own `jitso/builds/` is broken, and it fails every catalog command. Run `jitso build validate` for every problem at once. Do not edit `jitso/builds/` yourself: report it |
| `.mcp.json` missing or not valid JSON | a real launch reads it only when the launch still names an MCP server after `--mcp-exclude` |
| `jitso: the trust dialog of terminal <handle> did not clear in 10 s; the launch is registered` | a warning with exit 0. Look at the terminal and tell the human |
| `terminal <handle> is still open after close`, or an Orca error from `jitso close` | the terminal was not closed. Nothing was written: the launch is still running and has no `footer.txt`. Run `jitso close` again, or tell the human |
| an Orca error, `terminal <handle> did not reach ... ; it was closed` | Orca did not answer or the harness never became ready. The launch folder was removed. Try once more, then report the message |

## What the hook denies

The guard hook runs only in a project that has `jitso/jitso.yaml`, on Bash and on the orca MCP tools. It exists in a project only after the owner ran `jitso setup hook`; `jitso doctor` says when a slot is missing. It denies, with a reason that ends in `(denied: <verb>)`:

- Launching an agent outside jitso: `orchestration worker-start`, `orchestration dispatch`, `agent launch`, `worktree create --agent`, `terminal create` with a harness as its `--command`, and through MCP the orca-bridge `start_worker`, `create_worktree` with an agent and `create_terminal` with a harness. The text is the one in `SKILL.md`. It always prints the fleet shape of the launch line.
- A harness started as a program of its own: a Bash segment whose program is `claude`, `codex` or `opencode`, by name or path, behind `env`, `sudo`, `nohup`, `xargs` and similar wrappers, or inside `sh -c '...'` or `eval`. Even `claude --version` is denied. The reason says a harness starts only through the launcher and ends in `(denied: claude)`, `(denied: codex)` or `(denied: opencode)`. Checking that a harness is installed is allowed (`command -v claude`, `which claude`). Text that only holds the word is denied when the shell line splits it into a command (a commit message with `(claude)` in it, a heredoc line that starts with it): put the text in a file and pass the file.
- `jitso build save`, in any spelling, including inside `sh -c '...'`: `JITSO: only the owner saves a draft as a build; ask the owner to run: jitso build save <id> (denied: jitso build save)`.
- `jitso setup` and `jitso install`, unless the command carries `--dry-run`: `JITSO: only the owner changes the JITSO setup; ask the owner to run it in a terminal (denied: jitso setup)`, or `(denied: jitso install)` for the other.

The match for the last two is on text, so a commit message that holds those words is denied too. After any denial: stop, do not retry, do not rephrase to slip past it. For a launch, go back to `jitso launch ... --dry-run`. For the other two, tell the owner what you wanted and why. The guard fails open on input it cannot read, so a missing denial is not permission: the JITSO rules at the end of your brief still bind you.

A bare `claude`, `codex` or `opencode` typed in a shell is not matched by the guard. It is still forbidden by JITSO's rules, because it makes a session with no card and no registry line.

## `jitso doctor`

A read-only check that prints one line per check, `ok`, `warn` or `fail`, then a summary line, and exits 0 when nothing fails and 3 when a check fails. It takes no argument. Run it first when something is off. What each check means:

| Check | A `warn` or `fail` means | What to do |
|---|---|---|
| `binary`, `path` | the running jitso is not the one on PATH, or is not on PATH | hooks and the launch card use the bare word `jitso`: tell the owner |
| `git`, `shell` | no git, or no `/bin/sh` (launch does not run on Windows) | environment, not yours to fix |
| `orca`, `claude` | the Orca CLI or `claude` is not found | a launch cannot start. Tell the human |
| `project` | not inside a git checkout (the later checks are skipped) | `cd` into the project |
| `catalog` | `jitso/jitso.yaml` missing, a repo build invalid, or draft files skipped | `jitso build validate` lists every problem |
| `mcp` | a launchable build lists servers and `.mcp.json` is missing or invalid | tell the owner |
| `state`, `drafts`, `registry`, `backups` | `.jitso/` is not git-ignored, drafts are old or not ignored, or the log has invalid lines | ignored lines are skipped. Tell the owner about the rest |
| `hooks` | the guard is not registered, only on some events, the file is untracked, hooks are disabled, or the registered command does not resolve | the guard is off, so nothing denies a bad launch. Ask the owner to run `jitso setup hook` |
| `skill-project`, `skill-user` | the jitso skill is missing, out of date, or edited by hand | ask the owner to run `jitso install skill --scope project` (or `user`) |
| `instructions`, `instructions-user` | the JITSO block is missing or an older version in the instruction files | ask the owner to run `jitso setup instructions` |
