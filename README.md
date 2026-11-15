# JITSO

JITSO is a strategy for orchestrating AI agents based on **builds**. A build is an approved
profile (model, effort, MCP servers, skills, rules) that a session is launched with. The `jitso`
CLI launches each worker from a build, composes the launch line in code instead of trusting a
model to type it, logs every launch to a local event log, and reads the log back to show what a
session cost and whether it broke a rule.

JITSO ships no builds of its own. Each project writes its builds; JITSO validates them against
the allowlists of the repo where they launch.

## Status

Pre-release. Nothing is published: no remote, no release, no tag.

| Part | State |
|---|---|
| CLI (`launch`, `close`, `reap`, `analyze`, `presets list`, `version`, the dispatcher, the two hooks) | ported to Go; passes the contract suite on fakes; never run against a real Orca or a real Claude Code session |
| Build schema, validators, hard-rules core, templates | not in this repo yet |
| Adoption commands (`install skill`, `setup hook`, `setup instructions`, `doctor`) | not built |
| Distribution (GitHub Releases, install script, Homebrew tap) | the packaging script, the install script with its offline test, the release workflow and the Homebrew formula template exist as local files and have never run against GitHub; nothing is published. The procedure is [`docs/release.md`](docs/release.md) |
| Platforms | builds for macOS, Linux and Windows; `launch` and the dispatcher need `/bin/sh`, so they do not run on Windows |

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
| `cmd/jitso`, `internal/` | the CLI |
| `contract/` | the contract: case format, cases, fixtures, runner |
| `docs/architecture.md` | the design and the port milestones |
| `docs/decisions.md` | what was decided and the options it beat |
| `AGENTS.md`, `.agents/skills/` | instructions and skills for agents working on this repo |

## License

MIT, see [`LICENSE`](LICENSE).
