# Troubleshooting

Two prefixes tell you whose problem it is. `JITSO REJECTED: <reason>` (exit 2) means your input was refused: fix it and run again. `jitso: <reason>` (exit 3) means the environment around the command is wrong: a missing file, no git checkout, Orca. Read the whole message first, since most of them name the file or the field.

## Rejections (exit 2)

| Message | Cause and fix |
|---|---|
| `--build is required`, `--brief is required` | a required flag is missing |
| `--mode must be direct or fleet` | `--mode` is required and has no default. A build's `mode_default` is not used |
| `fleet mode requires --run` | fleet needs an existing Orca Run. Direct mode needs none |
| `--task is not accepted in fleet mode: JITSO creates the Task from the brief` | drop `--task`: the launcher creates the Task from the launch folder's `brief.md` |
| `unknown argument "<x>"` | a flag `launch` does not have, a stray word, or an old spelling such as `--preset`. `--help` after the first word ends here too (as the first word it prints the usage) |
| `--<name> needs a value` | the flag is the last word, or the next word starts with `--` (a value may not) |
| `jitso: warning: --mcp-exclude "<name>" is not an MCP server of build <id>; nothing excluded` | a warning, exit 0: the build does not list that name, so nothing was left out. A `(did you mean "<server>"?)` hint means the name differs only by case. Fix the name if you meant to drop a server |
| `--field must look like name=value` | write `--field ticket_key=PROJ-12` |
| `missing ticket_key` (any field the build `requires`) | pass `--field ticket_key=<value>` |
| `field ticket_key has an invalid value` | the value does not match the pattern the project gave that field in the `fields` key of `jitso/jitso.yaml`, or, for a field with no pattern, it is empty; a line break or any other control character is refused for every field. Read the build's description for the shape |
| `unknown field "foo" for build "<id>"` | the build does not require that field. Drop the flag |
| `unknown build "<id>"` | no build has that id for this project. Check `jitso build list`, `jitso build list --drafts` and `jitso build validate` |
| `build "<id>" has tier subagent: it is not launched alone, a worker lists it in its subagents` | launch the worker that lists it |
| `build "<worker>" lists subagent "<s>", which is <status>, not approved` | a saved worker needs approved subagents: `jitso build save <s>` when it is your draft, else pick another build |
| `build "<id>" from <layer> <path> is invalid here: <problems>` | a draft does not fit this project. `jitso build validate <id>` |
| `model "<m>" is not in the allowlist` (`effort`, `skill`, `mcp`, `tool`) | a list of `jitso/jitso.yaml` restricts that kind: use a value `jitso arsenal` lists, or ask the human to add it |
| `skill "<s>" has no folder under .agents/skills or .claude/skills`, `mcp "<m>" is not defined in .mcp.json` | the build names something the project does not have. Check `jitso arsenal` |
| `model is missing`, `model must be a non-empty value with no control character` (`effort` too) | give the build both |
| `jitso: warning: brief sections: ...`, `brief length: ...`, `brief language: ...`, `brief paths: ...` | warnings, exit 0: the launch goes on. Fix the brief before a real launch (`briefs.md`) |
| `brief target ...` (path empty, does not exist, not a regular file, outside the project, must be a .md file, is a protected path) | the brief file is a pointer (`Brief: <path>`) and its target was refused: `briefs.md`, "Pointing at a card you already have" |
| `brief file does not exist`, `brief path is not a regular file`, `brief path is empty or contains a NUL byte` | the `--brief` path is wrong |
| `brief file is outside the allowed folders` | move the brief to `.jitso/briefs/` of the project |
| `unknown parent launch <id>` | `--parent` names an id that is not in `.jitso/launches.jsonl` |
| `unknown launch <id>` (close) | the id is not in this project's log. Check the primary checkout's `.jitso/launches.jsonl` |
| `usage: jitso close <launch_id>`, `usage: jitso reap [--run <run_id>]...`, `usage: jitso fleet [--json] [--all]`, `usage: jitso link <from> <to>`, `usage: jitso graph [--json] [--port <n>]` | the arguments are wrong. `close` and `reap` have no `--help`; `fleet`, `link` and `graph` print their usage line for it |
| `unknown session "<ref>": give a launch id, a terminal handle or a session title (see jitso fleet)` | `link` found no session by that word. Take the id or the handle from `jitso fleet` |
| `"<ref>" names <n> sessions (...): use a terminal handle or a launch id` | the title is shared. Use one of the ids or handles the line lists |
| `"<from>" and "<to>" are the same session` | a synapse joins two sessions. Check which side is the peer |
| `cannot listen on 127.0.0.1 port <n>: ...` (exit 3, `graph`) | the port is taken. Drop `--port` or name another |
| `orca <verb> did not answer within 10 s` (exit 3, `graph --json`; a notice on the page) | Orca is busy or hung. Check the Orca app; `graph` printed what it could |
| `untrusted folder <path>: a launch runs only in a worktree of this repo; the terminal was closed` | Orca opened the terminal in a folder that is not a worktree of this repo, or reported none (`(unknown)`). The folder is checked on every launch, with or without the trust dialog. Check `--worktree`. Into another repository the text names the target instead of `this repo` |
| `untrusted folder <path>: --worktree asked for <path>; the terminal was closed` | `--worktree` names a path and Orca opened the terminal somewhere else. Name the worktree you want, or drop the flag |
| `--worktree <selector> must name an absolute path`, `--worktree <selector> names <path>, which is not a worktree of <this repo or the target>` | a `path:` or `id:<id>::<path>` selector was rejected before anything was written. Use an absolute path of a worktree of the target repository; with `--repo`, one of that repository |
| `invalid value for --repo`, `--repo <path> is not inside a git checkout` | give `--repo` an absolute path inside a git checkout, with no control character |
| `skill "user:<n>" is not in the user allowlist (~/.jitso/jitso.yaml)`, `mcp "user:<n>" is not in the user allowlist (~/.jitso/jitso.yaml)` | a list in the owner's file restricts user items. Pick one `jitso arsenal` lists, or ask the owner to add the name |
| `user skill "<n>" has no SKILL.md under ~/.claude/skills/<n>` | the folder is missing or holds no `SKILL.md`. Check the name with `jitso arsenal --all` |
| `user skill "<n>" has the name of a project skill`, `user mcp "<n>" has the name of a project mcp server` | a user item may not share a name with a project item. Name the project item without `user:`, or ask the owner to rename one |
| `user mcp "<n>" is not defined in ~/.claude.json` | the server is not under `mcpServers` of the owner's file. Tell the owner |
| `user mcp "<n>" has a literal value in env <KEY>: use ${VAR}, or list <n>/<KEY> in literal_env of ~/.jitso/jitso.yaml` (or `in headers <KEY>`) | a launch never copies a secret value into its folder. The owner moves the value to a variable, or lists `<n>/<KEY>` in `literal_env` if it is not a secret. The text names the key, never the value |
| `user mcp "<n>" has a literal value in args after <flag>: use ${VAR}, or list <n>/<flag> in literal_env of ~/.jitso/jitso.yaml` | an `args` element holds a secret after a flag named like key, token, secret, password, passwd or auth (or as `<flag>=value`). The owner passes it as `${VAR}`, or lists `<n>/<flag>` in `literal_env` if it is not a secret. The text names the flag, never the value |
| `user mcp "<n>" uses OAuth, which a launch cannot carry` | pick another server or give the session the work another way |
| `mcp "cloud:<n>": cloud connectors cannot be given to a launched session yet` | claude.ai connectors do not reach a launched session. Use them in your own session |
| `skill "<scope>:<n>": unknown scope (use user:)`, `mcp "<scope>:<n>": unknown scope (use user:)` | only `user:` is a scope |
| `user skill "<n>" is larger than a launch carries` | a user skill over 5 MiB or 500 files is not copied. Tell the owner |
| `jitso: warning: user mcp "<n>" refers to ${<VAR>}, which is not set here; the session gets the text unexpanded` | a warning, exit 0: set the variable in the environment that runs `jitso launch`, or tell the owner |
| `draft <...> is invalid: <problems>`, `build "<id>" already exists (<layer> <path>): a draft needs an id no build has` | fix the draft, or pick an id no saved build has |
| `draft "<id>" cannot be saved: write its status as the plain word draft, or in quotes` | save cannot rewrite that form of status. Write `status: draft`, or remove the line |
| `jitso/builds/<id>.yaml already exists: save never overwrites a build` | pick another id, or edit the saved file on purpose |

## Environment errors (exit 3)

| Message | Cause and fix |
|---|---|
| `jitso: <folder> is not inside a git checkout` | `cd` into the project |
| `jitso: <path> is not registered in Orca, so Orca opens no terminal there. Register it with: orca repo add --path <path>` | a launch into another repository. Run that line (it is your sign of trust for the folder; tell the human), then launch again |
| `jitso: warning: could not check that Orca has <path> registered (...); a launch checks it` | a dry run with no Orca to ask: exit 0. The real launch checks |
| `jitso: jitso/jitso.yaml is invalid: <problems>` | the project file is broken and fails every catalog command. Report it, or fix it if the human asks |
| `jitso: ~/.jitso/jitso.yaml is invalid: <problems>` | only a build that names a `user:` item needs the owner's file, and it is broken. Tell the owner; do not edit it |
| `build <file> is invalid: <problems>` | one file of `jitso/builds/` is broken, and it fails every catalog command. `jitso build validate` lists every problem. Fix a file you saved; report any other |
| `.mcp.json` missing or not valid JSON | a real launch reads it only when the launch still names an MCP server after `--mcp-exclude` |
| `jitso: the trust dialog of terminal <handle> did not clear in 10 s; the launch is registered` | a warning with exit 0. Look at the terminal and tell the human |
| `terminal <handle> is still open after close`, or an Orca error from `jitso close` | the terminal was not closed. Nothing was written: the launch is still running and has no `footer.txt`. Run `jitso close` again, or tell the human |
| an Orca error, `terminal <handle> did not reach ... ; it was closed` | Orca did not answer or the harness never became ready. The launch folder was removed. Try once more, then report the message |
| Claude Code asks "can't rule out a dangerous removal" for a heredoc command, even under bypassed permissions | in an unattended fleet the prompt blocks the worker for about two minutes and Orca reports "waiting on a prompt". Write the file with the Write tool, then run a plain command |

## What the hook denies

The guard blocks one thing: starting an agent outside `jitso launch`. It runs only in a project that has `jitso/jitso.yaml`, or in a session whose `JITSO_PROJECT` names a git checkout (one launched into another repository), on Bash and on the orca MCP tools, in every harness that has it: Claude Code (`.claude/settings.json`), Codex (`.codex/hooks.json`; Codex runs project hooks only after the person trusts them), OpenCode (`.opencode/plugins/jitso-guard.js`) and Pi (`.pi/extensions/jitso-guard.js`). `jitso init` or `jitso setup hook` registers it; a launched session always carries its own copy (Claude through `--settings` unless the project registers it). The `jitso-guard.js` files are generated: do not edit them. `jitso doctor` says when a slot is missing. It denies, with a reason that ends in `(denied: <verb>)`:

- Orca launches: `orchestration worker-start`, `orchestration dispatch`, `agent launch`, `worktree create --agent`, `terminal create` with a harness as its `--command`, and through MCP the orca-bridge `start_worker`, `create_worktree` with an agent and `create_terminal` with a harness. The text is the one in `SKILL.md`. It always prints the fleet shape of the launch line.
- A harness run to open a session: a command whose program is `claude`, `codex`, `opencode` or `pi`, by name or path, behind `env`, `sudo`, `nohup`, `xargs` and similar wrappers, or inside `sh -c '...'` or `eval`. The reason ends in `(denied: <program>)`. An invocation that starts no session passes, such as `claude --version`, `claude mcp list` or `codex login`, and so does `command -v claude`.

Each command of a line is judged on its own. Quoted text and heredoc bodies are data in a data command: `git commit` and `git tag` (the `-m` or `--message` value, a heredoc with `-F -`), `orca orchestration send`, `ask` and `reply` (`--body`, `--subject`, `--question`), `gh pr`, `issue` and `release` with `create`, `edit` or `comment`, `echo`, `printf`, and `cat` that reads a heredoc into its output or a file. Text that a shell on the line reads as commands is judged as commands, and so is a line the guard cannot parse: put such text in a file and pass the file.

Every other command passes, `jitso build save`, `init`, `setup` and `install` included. After a denial: stop, do not retry or rephrase to slip past it, and use `jitso launch ... --dry-run`. The guard fails open on input it cannot read, and it does not see an agent started by a program it does not read (`python`, `node`, a script file). Both ways are still against JITSO's rules, because they make a session with no card and no registry line.

## `jitso doctor`

A read-only check that prints one line per check, `ok`, `warn` or `fail`, then a summary line, and exits 0 when nothing fails and 3 when a check fails. It takes no argument. Run it first when something is off. What each check means:

| Check | A `warn` or `fail` means | What to do |
|---|---|---|
| `binary`, `path` | the running jitso is not the one on PATH, or is not on PATH | hooks and the launch card use the bare word `jitso`: tell the owner |
| `git`, `shell` | no git, or no `/bin/sh` (launch does not run on Windows) | environment, not yours to fix |
| `orca`, `claude` | the Orca CLI is not found, or `claude` is not (a `warn` when another harness is there: only `harness: claude` builds cannot launch) | a launch cannot start, or not in that harness. Tell the human |
| `project` | a `warn`: not inside a git checkout, so the project checks are skipped and the exit stays 0 | `cd` into the project |
| `catalog` | a `warn`: no `jitso/jitso.yaml` (everything is open and the guard is off), or draft files skipped. A `fail`: the project file or a saved build is invalid | `jitso build validate` lists every problem |
| `mcp` | a launchable build lists servers and `.mcp.json` is missing or invalid | tell the owner |
| `state`, `drafts`, `registry`, `backups` | `.jitso/` is not git-ignored, drafts are old or not ignored, or the log has invalid lines | ignored lines are skipped. Tell the owner about the rest |
| `hooks`, `hooks-user` | the guard is not registered for a harness, only on some events, a file is untracked, hooks are disabled, or the registered command does not resolve | that session is not guarded. Tell the human, who can ask for `jitso init` in the project, or `jitso setup hook --scope user` for every folder. A launched session carries its own guard |
| `skill-project`, `skill-user` | `ok ... not installed (optional ...)` is fine: a launch carries the jitso skill for a conductor build or a build that names `jitso`. A `warn` means a copy is out of date, edited by hand or not linked | `jitso install skill --scope <scope>` when the human asks. A user copy serves the human's own session, not a launched one |
| `instructions`, `instructions-user` | the JITSO block is missing, older, or left in a CLAUDE.md file by an older jitso | when the human asks, `jitso init` or `jitso setup instructions` for the project, `jitso setup instructions --scope user` for the user files. A CLAUDE.md that does not import AGENTS.md needs the line `@AGENTS.md` (`@../AGENTS.md` in `.claude/CLAUDE.md`) |
