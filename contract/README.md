# JITSO contract

A language-independent behavioral test suite for the `jitso` CLI and its two hooks. It runs
one `jitso` executable, the Go binary built from the repo root, against the cases; every
case must pass. The earlier TypeScript implementation is no longer part of the suite; its
code is kept at the tag `jitso-oracle-1` of the original QA boilerplate repo.

| File | What it is |
|---|---|
| `SPEC.md` | the case format and the matchers (normative) |
| `PLAN.md` | why it is built this way and the decisions of the first design; a dated record |
| `fixtures/default/` | the base project every case copies (`jitso/jitso.yaml`, the builds under `jitso/builds/`, instruction stubs) |
| `cases/<area>/<slug>/case.json` | one case per folder |
| `runner/` | the Go runner, standard library only |

## Run it

Build the runner once. Use the built binary, not `go run`: `go run` turns every non-zero
exit into 1, and the runner uses 1 for a failed case and 2 for a load error.

```sh
cd contract/runner
go build -o /tmp/jitso-contract .
cd ..
```

Build the binary under test from the repo root (`go build -o /path/to/jitso ./cmd/jitso`),
then:

```sh
/tmp/jitso-contract --impl bin --bin /path/to/jitso --jobs 4
```

The binary has to answer `jitso hook guard` (stdin JSON,
stdout JSON), the names `runner/internal/impl/bin.go` uses.

Useful flags: `--filter '<regex on id>'`, `--skip-tag posix` (cases that need `/bin/sh`),
`-v` (argv, stdout, stderr and the sandbox path of failing cases), `--keep` (keep sandboxes).
All flags: `runner/README.md`.

## Reading the result

| Line | Meaning |
|---|---|
| `PASS` | the case holds |
| `FAIL` | a regression: an expectation of the case broke |
| `SKIP` | excluded by `--skip-tag` |
| `LOAD` | a `case.json` could not be loaded (unknown key, bad regex, unknown placeholder) |

Exit codes: `0` no `FAIL` and no load error; `1` at least one `FAIL`; `2` a case could not
be loaded or the invocation is wrong. The expected result is every case `PASS`, `0 FAIL`,
`0 LOAD ERROR`. The summary line looks like:

```
bin: <n> cases, <n> PASS, <n> FAIL, <n> SKIP, <n> LOAD ERROR in <t>
```

The `defect` ids and `intent` texts in the cases are history of the port, kept in
`docs/decisions.md` ("Defect ids (history)"); nothing in the runner requires them.

S2 evidence: the variable is in the official Claude Code docs
(`code.claude.com/docs/en/env-vars`) for `CLAUDE.md` files, and a headless canary test on
Claude Code 2.1.325 showed it also stops the native read of a lone `AGENTS.md` and the
`@AGENTS.md` import of a `CLAUDE.md`. That test did not cover user-level instruction files,
nested or parent folders, or other versions; re-measure when Claude Code is upgraded.

## What the suite does not cover

Orca's own behavior, the real Claude Code TUI, and
white-box details of the Go code (function names, types). Cases that execute a
generated `run.sh` need `/bin/sh` and carry the tag `posix`. A case cannot talk to a server that stays up, so the contract pins `jitso graph --json`, and the server of `jitso graph` (the Host check, the token, the switch) and its page are Go tests of `internal/graph`. A case also cannot run as another OS: the host binary always reports the host's `GOOS`, so the Windows branch of `doctor` (N-20 `shell`) is a Go unit test.

## Adding a case

1. Create `cases/<area>/<slug>/case.json` (the `id` equals the folder path). Format: `SPEC.md`.
2. Cite only files the case's own `project` provides; the default fixture has
   `docs/guide.md`, `src/sync.ts` and `api/openapi.json`.
3. Run it against the built binary and see it `FAIL` for the behavior it pins, then implement
   the behavior and see it `PASS`. A case may carry an `intent` (and a `defect` id) saying why
   it exists; both are optional.
4. Prefer a `for_each` table to copy-pasted cases.
