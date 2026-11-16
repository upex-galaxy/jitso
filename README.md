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
| CLI: `launch`, `close`, `reap`, `analyze`, `tree`, `build` (`list`, `validate`, `new`, `draft`, `save`, `discard`), `install skill`, `setup hook`, `setup instructions`, `doctor`, `version`, the dispatcher and the two hooks (`hook guard`, `hook dispatch-guard`) | in Go; passes the contract suite on fakes; never run against a real Orca or a real Claude Code session. `tree` prints the launch tree with its rollups. The command list is `jitso --help` |
| Build schema, validators, hard-rules core, templates | in this repo: [`docs/build-schema.md`](docs/build-schema.md) is the schema, `jitso build validate` is the validator, and the hard-rules core, the reply style and the build templates ship inside the binary. JITSO still ships no builds |
| Adoption (`install skill`, `setup hook`, `setup instructions`, `doctor`) | the first three show a diff, back up what they change, ask before they write and have `--remove` as their undo. `doctor` (defect N-20, the last piece of the same work) reports what is missing on the machine and in the project and changes nothing; `jitso --help` shows whether the binary you built has it. The launch guard denies `setup`, `install` and `build save` to agent sessions |
| Distribution (GitHub Releases, install script, Homebrew tap) | the packaging script, the install script with its offline test, the release workflow and the Homebrew formula template exist as local files; the workflow has never run against GitHub and nothing is published. The procedure is [`docs/release.md`](docs/release.md) |
| Platforms | builds for macOS, Linux and Windows; `launch` and the dispatcher need `/bin/sh`, so they do not run on Windows, and Windows is not tested |

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
