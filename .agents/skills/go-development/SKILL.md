---
name: go-development
description: "Go conventions for this repo: the contract runner and the jitso CLI. Covers standard library first, where code goes, tests, the check order (go test, go vet, gofmt), cross-compiling for macOS, Linux and Windows, the binary size budget, and looking things up in the official Go docs and go doc instead of from memory. Use whenever you write, review or refactor Go here, start the Go module of the CLI, add a package or a dependency, touch go.mod or the toolchain version, hit an OS-specific behavior (process groups, paths, /bin/sh), or are asked about binary size or builds per OS. Do NOT use for: deciding what the CLI should do (contract-driven-development owns behavior), or git and releases (git-flow-master; releases are owner-gated)."
---

# Go development

Two Go codebases, at different stages:

| Module | State | Where |
|---|---|---|
| `jitso-contract-runner` | exists, stdlib only | `contract/runner/` (`main.go` plus `internal/<pkg>`; package map in `contract/runner/README.md`) |
| the `jitso` CLI | exists, one dependency (YAML, behind `internal/catalog/decode.go`) | the repo root: `cmd/jitso/` is wiring only and everything else is under `internal/<package>/`; read the layout from the tree (`ls internal`, `go list ./...`) and the responsibilities from `docs/architecture.md` sections 3 to 5 |

They are separate modules. A `go test ./...` at one module's root never runs the other's tests, so run the checks in each module you touched.

## Before you write

- **Docs first.** Look up an API with `go doc <pkg>` or `go doc <pkg>.<Symbol>` on this machine, then the official docs (`pkg.go.dev/std`, `go.dev/ref/spec`, `go.dev/doc/devel/release` for what changed in a version). Do not rely on memory for an API signature or a toolchain behavior. Use the context7 MCP when it is available.
- **Standard library first.** A third-party module needs the owner's OK: it costs binary size, review and a supply-chain surface. Say what it saves over the stdlib, and measure the size difference (`references/builds.md`).
- **Language version.** The `go` line of the module's `go.mod` is the language version; do not use a feature newer than it. Raising it is a plan item, not a drive-by edit.

## Writing

- `gofmt` formatting, always. Idiomatic names, short packages under `internal/`, no exported API without a caller.
- Errors carry context (`fmt.Errorf("...: %w", err)`); the CLI maps them to the contract's exit codes, never `os.Exit` deep in a package.
- Nothing a model wrote reaches a shell as text: argv is built as a slice, and any generated script quotes every word (pinned by the shell-escape cases).
- OS differences go in files with build constraints, the way the runner does it (`internal/engine/proc_unix.go` with `//go:build unix`, `proc_other.go` with `//go:build !unix`). Use `path/filepath` for file paths. Only code behind cases tagged `posix` may assume `/bin/sh`.

## Testing

- Table-driven tests with `t.Run`, temp files under `t.TempDir()`, no network, and never a real `orca` or `claude`: behavior is tested by the contract with its fakes; Go tests cover the units, with the ports of `docs/architecture.md` section 4 stood in by small fakes (a scripted `orca.Runner`, a fake clock, an injected terminal and reader for the commands that ask a person). The terminal path of `jitso build save` and of the adoption commands is a unit test, because the contract never has a terminal.
- A Go test does not replace a contract case for observable behavior.

## Checks, in this order (AGENTS.md rule 8)

From the root of each module you touched:

```sh
go test ./...
go vet ./...
test -z "$(gofmt -l .)"    # gofmt -l exits 0 even when it lists files; this line fails instead
```

Then the contract cases the change touches (`contract-driven-development`). Cross-compile when the change touches OS-specific code: `references/builds.md`.

## Rules

- Never build into the repo tree with a name `.gitignore` does not cover; build into a scratch directory, as `contract/README.md` does.
- `go run` turns every non-zero exit into 1: build a binary wherever an exit code matters.
- Version stamping, the release archives and the install script exist: `scripts/package.sh` builds the archives and stamps `internal/version.Version` with `-ldflags -X`, and `docs/release.md` is the procedure. Use them; do not hand-roll a release build. Publishing anything (a tag, a remote, a GitHub Release) stays the owner's call (AGENTS.md rule 4).
