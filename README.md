# JITSO

JITSO is a strategy for orchestrating AI agents based on **builds**. A build is an approved
profile (model, effort, MCP servers, skills, rules) that a session is launched with. The `jitso`
CLI launches each worker from a build, composes the launch line in code instead of trusting a
model to type it, and logs every launch to a local event log.

JITSO ships no builds of its own. Each project writes its builds; JITSO validates them against
the allowlists of the repo where they launch.

## Status

Pre-release. Nothing is published: no remote, no release, no tag.

| Part | State |
|---|---|
| CLI: `launch`, `close`, `reap`, `build` (`list`, `validate`, `new`, `draft`, `save`, `discard`), `install skill`, `setup hook`, `setup instructions`, `doctor`, `version` and the hook (`hook guard`) | in Go; passes the contract suite on fakes; never run against a real Orca or an interactive Claude Code session (a small headless `claude -p` run did confirm the launcher's argv, the hook deny and the environment variables). The command list is `jitso --help` |
| Build schema, validators, rules text, templates | in this repo: [`docs/build-schema.md`](docs/build-schema.md) is the schema, `jitso build validate` is the validator, JITSO's short rules text is appended to the end of every launched session's `brief.md` (`internal/card`), and the build templates ship inside the binary. JITSO still ships no builds |
| Adoption (`install skill`, `setup hook`, `setup instructions`, `doctor`) | the first three show a diff, back up what they change, ask before they write and have `--remove` as their undo. `doctor` (defect N-20, the last piece of the same work) reports what is missing on the machine and in the project and changes nothing; `jitso --help` shows whether the binary you built has it. The launch guard denies `setup`, `install` and `build save` to agent sessions |
| The guard hook (`jitso hook guard`) | in Go, with contract cases on fakes. It denies launching an agent outside `jitso launch` (the Orca launch verbs and the orca MCP tools), a harness started as a program of its own (`claude`, `codex`, `opencode`), `jitso build save`, `jitso setup` and `jitso install` (the owner's). `jitso setup hook` registers it as three groups in `.claude/settings.json`, and `jitso doctor` checks the same groups |
| Which sessions the guard covers | only a Claude Code session that loads the project's `.claude/settings.json` with the guard in it. A worker launched by `jitso launch` reads project settings only (`--setting-sources project`), so the guard has to be committed in the project, or in the worktree the worker runs in. Codex, OpenCode, Pi, a terminal outside Claude Code and any session that turns hooks off are not covered, and the guard reads text, so a program it does not know (`python`, `node`, a script file) can still start an agent. The rules at the end of `brief.md` bind those sessions, a hook does not |
| Distribution (GitHub Releases, install script, Homebrew tap) | the packaging script, the install script with its offline test, the release workflow and the Homebrew formula template exist as local files; the workflow has never run against GitHub and nothing is published. The procedure is [`docs/release.md`](docs/release.md) |
| Upgrading from the TypeScript CLI | the launch log is schema v2 (`v: 2`, the build id under the key `build`) and a line of an older log (`v: 1`, key `preset`) is not read. Close or reap every running launch with the old CLI before the first run of this one: the old lines stay in `.jitso/launches.jsonl`, `jitso doctor` counts them, and a worker that a still-running old launch started is reported as unlogged. Moving the old file aside (`mv .jitso/launches.jsonl .jitso/launches.v1.jsonl`) is safe |
| Platforms | builds for macOS, Linux and Windows; `launch` needs `/bin/sh`, so it does not run on Windows. The Windows zip is experimental and not tested: the install script refuses Windows (install by hand, see [`docs/release.md`](docs/release.md) section 7), and `jitso doctor` warns there about `/bin/sh` instead of failing |
| Not done | the first release and every outward step of [`docs/release.md`](docs/release.md) (no remote, no tag, nothing published, and the release workflow has never run); the attended real Orca run, so the idle detection, the real trust dialog text and the real `terminal create` answer have only been checked against fakes; the real Claude Code interactive path; a Linux run of the contract suite; Windows testing |

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
| `docs/build-schema.md` | the schema of a build, its layers, drafts and validators |
| `docs/decisions.md` | what was decided and the options it beat |
| `docs/release.md` | the release procedure, with every outward step marked owner-gated |
| `scripts/` | packaging, the install script, its offline test and the formula filler |
| `packaging/` | the Homebrew formula template |
| `.github/` | the release workflow (written, never run) |
| `AGENTS.md`, `.agents/skills/` | instructions and skills for agents working on this repo |

## License

MIT, see [`LICENSE`](LICENSE).
