---
name: go-development
description: "Go conventions for this repo: the contract runner today and the jitso CLI port next. Covers standard library first, where code goes, tests, the check order (go test, go vet, gofmt), cross-compiling for macOS, Linux and Windows, the binary size budget, and looking things up in the official Go docs and go doc instead of from memory. Use whenever you write, review or refactor Go here, start the Go module of the CLI, add a package or a dependency, touch go.mod or the toolchain version, hit an OS-specific behavior (process groups, paths, /bin/sh), or are asked about binary size or builds per OS. Do NOT use for: deciding what the CLI should do (contract-driven-development owns behavior), or git and releases (git-flow-master; releases are owner-gated)."
---

# Go development

Two Go codebases, at different stages:

| Module | State | Where |
|---|---|---|
| `jitso-contract-runner` | exists, stdlib only | `contract/runner/` (`main.go` plus `internal/<pkg>`; package map in `contract/runner/README.md`) |
| the `jitso` CLI | not started | no module yet: its layout is decided in a plan the owner approves before the first file (AGENTS.md rule 2) |

They are separate modules. A `go test ./...` at one module's root never runs the other's tests, so run the checks in each module you touched.

## Before you write

- **Docs first.** Look up an API with `go doc <pkg>` or `go doc <pkg>.<Symbol>` on this machine, then the official docs (`pkg.go.dev/std`, `go.dev/ref/spec`, `go.dev/doc/devel/release` for what changed in a version). Do not rely on memory for an API signature or a toolchain behavior. Use the context7 MCP when it is available.
- **Standard library first.** A third-party module needs the owner's OK: it costs binary size, review and a supply-chain surface. Say what it saves over the stdlib, and measure the size difference (`references/builds.md`).
- **Language version.** The `go` line of the module's `go.mod` is the language version; do not use a feature newer than it. Raising it is a plan item, not a drive-by edit.

## Writing

- `gofmt` formatting, always. Idiomatic names, short packages under `internal/`, no exported API without a caller.
- Errors carry context (`fmt.Errorf("...: %w", err)`); the CLI maps them to the contract's exit codes, never `os.Exit` deep in a package.
- Nothing a model wrote reaches a shell as text: argv is built as a slice, and any generated script quotes every word (the oracle's rule, pinned by the shell-escape cases).
- OS differences go in files with build constraints, the way the runner does it (`internal/engine/proc_unix.go` with `//go:build unix`, `proc_other.go` with `//go:build !unix`). Use `path/filepath` for file paths. Only code behind cases tagged `posix` may assume `/bin/sh`.

## Testing

- Table-driven tests with `t.Run`, temp files under `t.TempDir()`, no network, and never a real `orca` or `claude`: behavior is tested by the contract with its fakes; Go tests cover the units.
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
- Version stamping, release archives and install scripts wait for the release plan (decision D4 in `docs/decisions.md`); publishing anything is the owner's call.
