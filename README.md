# JITSO

JITSO is a strategy for orchestrating AI agents based on **builds**. A build is the profile
(model, effort, MCP servers, skills, rules) a session is launched with. The `jitso`
CLI launches each worker from a build, composes the launch line in code instead of trusting a
model to type it, and logs every launch to a local event log.

It is made to fit under any orchestration system: the one thing it blocks is starting an agent
outside `jitso launch`. A build can be composed just in time (`jitso arsenal` shows what it may
name, `jitso build draft -` keeps it, `jitso launch` runs it) and saved only if it is worth
keeping.

JITSO ships no builds of its own. JITSO validates every build against the repo that launches it,
and against the allowlists of that repo when it lists any.

## Status

Released: the versions are the git tags and GitHub Releases of this repository, and the procedure is [`docs/release.md`](docs/release.md).

Install the latest release on macOS or Linux (it installs to `~/.local/bin`, adds that folder to your shell's `PATH` when it is missing, checks the SHA-256 and needs no `sudo`):

```sh
curl -fsSL https://github.com/upex-galaxy/jitso/releases/latest/download/install.sh | sh
```

Read the script first if you prefer: it is `scripts/install.sh`. Then set everything up with one command, from the root of your project:

```sh
jitso init
```

It installs the `jitso` skill for every harness, writes the JITSO block into `AGENTS.md` (replacing the block of an older release), creates `jitso/jitso.yaml` when it is missing, and registers the guard for each harness it finds on `PATH` (Claude Code, Codex, OpenCode, Pi). It shows every change and asks once; `--yes` skips the question, `--dry-run` only shows, `--remove` undoes it. Asking an AI agent to install JITSO? Point it at [`INSTALL.md`](INSTALL.md).

| Part | State |
|---|---|
| CLI: `launch`, `close`, `reap`, `fleet`, `link`, `graph`, `build` (`list`, `validate`, `new`, `draft`, `save`, `discard`), `arsenal`, `init`, `install skill`, `setup hook`, `setup instructions`, `doctor`, `version` and the hook (`hook guard`) | in Go; the contract suite runs it against fakes of Orca and Claude Code. The attended real Orca run is required before `v1.0.0`. The command list is `jitso --help` |
| Release order | released first, then tested end to end from an install of that release; the attended real Orca run gates `v1.0.0`. Every outward step waits for the owner ([`docs/decisions.md`](docs/decisions.md), section "Release order and the conductor skill") |
| Conductor skill | the user-level skill `agent-orchestration` (repo `agentic-user-skills`) is the conductor doctrine. It runs on Orca by default and uses a `jitso` mode only when the `jitso` binary is on PATH and the project has `jitso/jitso.yaml`. JITSO becomes its default path only after it is published and has passed an attended real run |
| Build schema, validators, rules text, templates | in this repo: [`docs/build-schema.md`](docs/build-schema.md) is the schema, `jitso build validate` is the validator, JITSO's short rules text is appended to the end of every launched session's `brief.md` (`internal/card`), and the build templates ship inside the binary. JITSO still ships no builds |
| Adoption (`init`, `install skill`, `setup hook`, `setup instructions`, `doctor`) | `init` runs the other three (and the `jitso/jitso.yaml` skeleton) with one preview and one question. They show a diff, back up what they change, ask before they write and have `--remove` as their undo. Instruction blocks go to `AGENTS.md` files only, never `CLAUDE.md`. `doctor` reports what is missing on the machine and in the project and changes nothing |
| The guard hook (`jitso hook guard`) | in Go, with contract cases on fakes. It denies only launching an agent outside `jitso launch`: the Orca launch verbs, the orca MCP tools that start one, and a harness (`claude`, `codex`, `opencode`, `pi`) run to open a session. Invocations that start no session, such as `claude --version`, and every other `jitso` command pass. `jitso setup hook` registers it for Claude Code (`.claude/settings.json`), Codex (`.codex/hooks.json`), OpenCode (`.opencode/plugins/jitso-guard.js`) and Pi (`.pi/extensions/jitso-guard.js`), or under the home folder with `--scope user`; `jitso doctor` checks each |
| Which sessions the guard covers | a session of Claude Code, Codex, OpenCode or Pi that has the guard registered (project or user files), and every worker `jitso launch` starts, which always carries its own copy. Codex runs project hooks only in a trusted project after the person trusts them (the first session shows "Hooks need review"; `codex exec` skips untrusted hooks silently). A terminal outside an agent session and any session that turns hooks off are not covered, and the guard reads text, so a program it does not know (`python`, `node`, a script file) can still start an agent. The rules at the end of `brief.md` bind those sessions, a hook does not |
| Distribution (GitHub Releases, install script) | the packaging script, the install script with its offline test and the release workflow are files of this repo. The procedure is [`docs/release.md`](docs/release.md) |
| Upgrading from the TypeScript CLI | the launch log is schema v2 (`v: 2`, the build id under the key `build`) and a line of an older log (`v: 1`, key `preset`) is not read. Close or reap every running launch with the old CLI before the first run of this one: the old lines stay in `.jitso/launches.jsonl`, `jitso doctor` counts them, and a worker that a still-running old launch started is reported as unlogged. Moving the old file aside (`mv .jitso/launches.jsonl .jitso/launches.v1.jsonl`) is safe |
| Harnesses a worker can run in | `harness:` of a build is `claude`, `codex`, `opencode` or `pi`. Each adapter was proven by a real launch through Orca; what each one honors (MCP servers, skills, subagents, tools) is the table in [`docs/build-schema.md`](docs/build-schema.md), and a build that asks for more is rejected with a text that names the harness |
| Platforms | macOS, Linux and Windows. CI (`.github/workflows/ci.yml`) checks every push on all three, with Windows required: unit tests, the contract suite (the cases tagged `posix` are skipped there) and the PowerShell installer. `jitso launch` needs `/bin/sh` in an Orca terminal and does not run on Windows: it exits 3 with one line (`--dry-run` works). Everything else does. Install on Windows with `install.ps1` from the release ([`docs/release.md`](docs/release.md) section 7), and `jitso doctor` warns there about `/bin/sh` instead of failing |
| Gates not closed by the contract suite | the attended real Orca run (idle detection, the real trust dialog text and the real `terminal create` answer, which fakes cannot prove), the real Claude Code interactive path and a real Orca on Windows. The release steps are owner-gated in [`docs/release.md`](docs/release.md) |

## Adopt JITSO in a project

`jitso init` in the project is the whole setup (see Status). What it leaves in the checkout: `jitso/jitso.yaml` (it switches the guard on, and its optional allowlists restrict builds; `init` writes none, so everything that exists is open), the JITSO block in `AGENTS.md`, `.jitso/` in `.gitignore`, and the guard files of each harness. Saved builds go in `jitso/builds/`, drafts in `.jitso/drafts/`. `launch`, `build`, `arsenal` and `doctor` also work in a project without the file.

The brief of a launch is checked for sections, length, language and cited paths, and a miss is a warning. `jitso fleet` lists every session Orca knows with its name and repository, this project first. `jitso init --project-only` also keeps the skill in the checkout and writes nothing under the home folder.

- The jitso skill and the guard are optional because they travel. A launch carries the guard in its `--settings` unless the project already registers it, and the jitso skill for a conductor build or a build that names `jitso` unless the checkout holds a copy. They live in a per-launch folder, `.jitso/launches/<id>/pkg/`, passed with `--add-dir`.
- `jitso setup hook --scope user` guards the owner's own session, which JITSO does not launch. It registers the same guard in `~/.claude/settings.json` and prints nothing in a folder without `jitso/jitso.yaml`.
- A build names a user skill or user MCP server, one that lives outside the project, as `user:<name>`; a plain name never resolves there. `~/.jitso/jitso.yaml` is the owner's optional allowlist for them: an absent or empty list is open. A launch copies the skill into its package and the server into its `mcp.json`, with variable references only, never a literal secret.
- `jitso arsenal` lists what a build may name in the project, and `jitso arsenal --all` adds what exists on the machine but a restricting allowlist leaves out.
- Refused for now: claude.ai connectors (`cloud:`), servers that use OAuth, and any literal secret value of a user server.
- `jitso install skill --scope project`, `jitso setup hook` and `jitso setup instructions` stay available for a project that wants the files in its own checkout. [`docs/build-schema.md`](docs/build-schema.md) has the user allowlist and its texts.

## Names and messages between sessions

- Each launch has a session name: `jitso launch --name <name>`, else the build id, with a suffix when a running launch already has it. Claude Code and Pi sessions start with that name, the card shows it, `jitso fleet` lists it, `jitso link` takes it, and the Orca tab is titled `<name> [jitso:<id>]`.
- A launched Claude Code session reads no user settings. The launch copies one of them, `crossSessionInbound` (`accept`, `hold` or `refuse`), from the owner's `~/.claude/settings.json` (or the folder `CLAUDE_CONFIG_DIR` names) into the session's `--settings`, so the owner's choice about messages from other sessions holds there too. Nothing else of that file travels.

## Launch into another repository

`jitso launch --repo <absolute path>` (or `--worktree path:<folder>` of another repository) sends a worker into that repository, and yours keeps the fleet.

- Your repository keeps the builds, the briefs, `.jitso/` and the model, effort and field lists. The target gives the skills, MCP servers, route ids and instructions, and the skill, MCP and tool lists of its own `jitso/jitso.yaml` when it has one; it needs none.
- `jitso arsenal --repo <path>` shows what a build for it may name, and `jitso build draft --repo <path>` keeps one that names it.
- The target must be registered in Orca. JITSO refuses an unregistered one before it writes anything and prints the `orca repo add --path` line; it never registers a repository itself.
- The worker runs with `JITSO_PROJECT` set to your repository, so its `jitso` calls, its guard and its end event all reach your launch log.

## See who talks to whom

`jitso graph` serves a live picture of the sessions on `127.0.0.1`, prints its URL and runs until Ctrl-C. Open the URL in a browser.

- Each session is a star: the launches of the project and every other terminal Orca lists, with the states of `jitso fleet`.
- A dashed line goes from a parent launch to a child. A synapse joins two sessions that talked, drawn as one arrow when it went one way and as a double line when it went both ways. Sessions that never talked have none.
- Synapses come from the messages of Orca's orchestration inbox (direction, count and time, never the content) and from `jitso link <from> <to>`, which a session runs after a Claude Code `SendMessage`, because that leaves no trace to read.
- The page switches between this project's fleet and every Orca session, searches, shows the details of a star and can switch Orca to its terminal.
- Each star names the folder it lives in. Outside a project (no git checkout, or no launch registry yet) the page opens on every Orca session and says why.
- `jitso graph --json` prints one snapshot and exits. Without Orca both show the registry alone and say so, and an `orca` call that does not answer is given up with a notice.

The server answers only to `127.0.0.1` and `localhost`, and a switch needs a token that changes on every run. Any other user or process of your machine that can connect to the port can read the page, so do not run it on a shared machine.

## Build and test

Needs Go. From the repo root:

```sh
go build -o /tmp/jitso ./cmd/jitso
go test ./...
go vet ./...
```

The behavior is defined by a language-independent contract in [`contract/`](contract/README.md),
run against any `jitso` binary:

```sh
cd contract/runner && go build -o /tmp/jitso-contract . && cd ..
/tmp/jitso-contract --impl bin --bin /tmp/jitso --jobs 4
```

## Layout

| Path | What it is |
|---|---|
| `cmd/jitso`, `internal/` | the CLI; `internal/skill` holds the usage skill the binary ships and `internal/graph/web` the page of `jitso graph` |
| `contract/` | the contract: case format, cases, fixtures, runner |
| `docs/architecture.md` | the design and the port milestones |
| `docs/build-schema.md` | the schema of a build, where builds live, drafts and validators |
| `docs/decisions.md` | what was decided and the options it beat |
| `docs/release.md` | the release procedure, with every outward step marked owner-gated |
| `scripts/` | packaging, the install script and its offline test, the guard plugin test |
| `INSTALL.md` | the install steps, written so an AI agent can follow them |
| `.github/` | the CI and release workflows |
| `AGENTS.md`, `.agents/skills/` | instructions and skills for agents working on this repo |

## License

MIT, see [`LICENSE`](LICENSE).
