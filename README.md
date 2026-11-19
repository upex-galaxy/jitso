# JITSO

JITSO is a strategy for orchestrating AI agents based on **builds**. A build is an approved
profile (model, effort, MCP servers, skills, rules) that a session is launched with. The `jitso`
CLI launches each worker from a build, composes the launch line in code instead of trusting a
model to type it, and logs every launch to a local event log.

JITSO ships no builds of its own. Each project writes its builds; JITSO validates them against
the allowlists of the repo where they launch.

## Status

Released: the versions are the git tags and GitHub Releases of this repository, and the procedure is [`docs/release.md`](docs/release.md).

Install the latest release on macOS or Linux (it installs to `~/.local/bin`, checks the SHA-256 and needs no `sudo`):

```sh
curl -fsSL https://github.com/upex-galaxy/jitso/releases/latest/download/install.sh | sh
```

Read the script first if you prefer: it is `scripts/install.sh`. Then run `jitso doctor` in your project.

| Part | State |
|---|---|
| CLI: `launch`, `close`, `reap`, `build` (`list`, `validate`, `new`, `draft`, `save`, `discard`), `arsenal`, `install skill`, `setup hook`, `setup instructions`, `doctor`, `version` and the hook (`hook guard`) | in Go; the contract suite runs it against fakes of Orca and Claude Code. The attended real Orca run is required before `v1.0.0`. The command list is `jitso --help` |
| Release order | released first, then tested end to end from an install of that release; the attended real Orca run gates `v1.0.0`. Every outward step waits for the owner ([`docs/decisions.md`](docs/decisions.md), section "Release order and the conductor skill") |
| Conductor skill | the user-level skill `agent-orchestration` (repo `agentic-user-skills`) is the conductor doctrine. It runs on Orca by default and uses a `jitso` mode only when the `jitso` binary is on PATH and the project has `jitso/jitso.yaml`. JITSO becomes its default path only after it is published and has passed an attended real run |
| Build schema, validators, rules text, templates | in this repo: [`docs/build-schema.md`](docs/build-schema.md) is the schema, `jitso build validate` is the validator, JITSO's short rules text is appended to the end of every launched session's `brief.md` (`internal/card`), and the build templates ship inside the binary. JITSO still ships no builds |
| Adoption (`install skill`, `setup hook`, `setup instructions`, `doctor`) | the first three show a diff, back up what they change, ask before they write and have `--remove` as their undo. `doctor` reports what is missing on the machine and in the project and changes nothing. The launch guard denies `setup`, `install` and `build save` to agent sessions. All three project installs are optional, see "Adopt JITSO in a project" |
| The guard hook (`jitso hook guard`) | in Go, with contract cases on fakes. It denies launching an agent outside `jitso launch` (the Orca launch verbs and the orca MCP tools), a harness started as a program of its own (`claude`, `codex`, `opencode`), `jitso build save`, `jitso setup` and `jitso install` (the owner's). `jitso setup hook` registers it as three groups in `.claude/settings.json` (or in `~/.claude/settings.json` with `--scope user`), and `jitso doctor` checks the same groups |
| Which sessions the guard covers | only a Claude Code session that gets the guard from the project's `.claude/settings.json`, from a launch's `--settings`, or from `~/.claude/settings.json`. A worker launched by `jitso launch` reads project settings only (`--setting-sources project`), so the launch carries the guard in its own `--settings` unless the project registers it. The owner's own session is guarded by `jitso setup hook --scope user`. Codex, OpenCode, Pi, a terminal outside Claude Code and any session that turns hooks off are not covered, and the guard reads text, so a program it does not know (`python`, `node`, a script file) can still start an agent. The rules at the end of `brief.md` bind those sessions, a hook does not |
| Distribution (GitHub Releases, install script) | the packaging script, the install script with its offline test and the release workflow are files of this repo. The procedure is [`docs/release.md`](docs/release.md) |
| Upgrading from the TypeScript CLI | the launch log is schema v2 (`v: 2`, the build id under the key `build`) and a line of an older log (`v: 1`, key `preset`) is not read. Close or reap every running launch with the old CLI before the first run of this one: the old lines stay in `.jitso/launches.jsonl`, `jitso doctor` counts them, and a worker that a still-running old launch started is reported as unlogged. Moving the old file aside (`mv .jitso/launches.jsonl .jitso/launches.v1.jsonl`) is safe |
| Platforms | builds for macOS, Linux and Windows; `launch` needs `/bin/sh`, so it does not run on Windows. The Windows zip is experimental and not tested: the install script refuses Windows (install by hand, see [`docs/release.md`](docs/release.md) section 7), and `jitso doctor` warns there about `/bin/sh` instead of failing |
| Gates not closed by the contract suite | the attended real Orca run (idle detection, the real trust dialog text and the real `terminal create` answer, which fakes cannot prove), the real Claude Code interactive path and Windows testing. The release steps are owner-gated in [`docs/release.md`](docs/release.md) |

## Adopt JITSO in a project

A project needs two things in the checkout: `jitso/jitso.yaml` (the allowlists) and `jitso/builds/` (the builds). Nothing else is written there.

- The jitso skill and the guard are optional because they travel. A launch carries the guard in its `--settings` unless the project already registers it, and the jitso skill for a conductor build or a build that names `jitso` unless the checkout holds a copy. They live in a per-launch folder, `.jitso/launches/<id>/pkg/`, passed with `--add-dir`.
- `jitso setup hook --scope user` guards the owner's own session, which JITSO does not launch. It registers the same guard in `~/.claude/settings.json` and prints nothing in a folder without `jitso/jitso.yaml`.
- `~/.jitso/jitso.yaml` is the owner's allowlist for items that live outside the project: user skills and user MCP servers. A build names one as `user:<name>`; a plain name never resolves there. A launch copies the skill into its package and the server into its `mcp.json`, with variable references only, never a literal secret.
- `jitso arsenal` lists what a build may name in the project, and `jitso arsenal --all` adds what exists on the machine but no allowlist names yet.
- Refused for now: claude.ai connectors (`cloud:`), servers that use OAuth, and any literal secret value of a user server.
- `jitso install skill --scope project`, `jitso setup hook` and `jitso setup instructions` stay available for a project that wants the files in its own checkout. [`docs/build-schema.md`](docs/build-schema.md) has the user allowlist and its texts.

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
| `cmd/jitso`, `internal/` | the CLI; `internal/skill` holds the usage skill the binary ships |
| `contract/` | the contract: case format, cases, fixtures, runner |
| `docs/architecture.md` | the design and the port milestones |
| `docs/build-schema.md` | the schema of a build, where builds live, drafts and validators |
| `docs/decisions.md` | what was decided and the options it beat |
| `docs/release.md` | the release procedure, with every outward step marked owner-gated |
| `scripts/` | packaging, the install script and its offline test |
| `.github/` | the release workflow |
| `AGENTS.md`, `.agents/skills/` | instructions and skills for agents working on this repo |

## License

MIT, see [`LICENSE`](LICENSE).
