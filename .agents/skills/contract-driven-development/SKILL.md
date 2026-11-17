---
name: contract-driven-development
description: "How every behavior of the jitso CLI and its hooks is added, changed or fixed in this repo: write or edit the contract case first, run it against the built Go binary and see it FAIL, implement it, see it PASS, and record the call in docs/decisions.md. Use whenever the request changes what jitso does or prints (a new command or flag, an exit code, the launch card, run.sh, the registry log, close/reap, the guard hook), touches a defect id such as S1 route_scope, S2 agents_md or S3 subagents, adds or edits a case under contract/cases, or asks why the runner printed FAIL or LOAD. Use it even when the user only says 'fix this bug' or 'make jitso do X'. Do NOT use for: Go style, layout or build questions with no behavior change (go-development), or committing (git-flow-master)."
---

# Contract-driven development

The contract under `contract/` is the definition of JITSO's behavior. The Go binary must pass every case. A behavior that is not a case does not exist yet, so every change starts there. There is no second implementation to compare with: the binary and the cases are the only truth.

Read first, as needed: `contract/README.md` (run it, read the result), `contract/SPEC.md` (the case format; it wins over everything else), `contract/PLAN.md` (why the contract is built this way).

## The loop

1. **Find the behavior.** Search the cases before writing one: `grep -rl '<flag or text>' contract/cases`. A case's `id` equals its folder path under `contract/cases/`.
2. **Plan it** (AGENTS.md rule 2): which cases change or are added, which Go code moves. Owner's OK before editing.
3. **Write or edit the case.** Format: `contract/SPEC.md`. Cite only files the case's own `project` provides; prefer a `for_each` table to copied cases; put multi-line text in a file next to `case.json` and reference it with `{"from": "..."}`. Never weaken an existing assertion to make a case pass.
4. **Run it against the built binary and see it FAIL**, for the reason the case pins (`-v` shows it). A new case that passes at once pins nothing new: tighten it.
5. **Implement in Go** (load `go-development`).
6. **Run the case again, then the whole suite**, and see `PASS` everywhere.
7. **Record the call** (below), in the same change as the case.

Commands (build the runner once; use the binary, not `go run`, because `go run` turns exit 2 into 1):

```sh
cd contract/runner && go build -o <scratch dir>/jitso-contract . && cd ../..
go build -o <scratch dir>/jitso ./cmd/jitso
<scratch dir>/jitso-contract --impl bin --bin <scratch dir>/jitso --filter '^launch/' --jobs 4
```

Cases are written in the current vocabulary (`--build`, `jitso/builds/`, `build list`).

Useful flags: `-v` (argv, stdout, stderr, sandbox of failing cases), `--keep` (keep the sandboxes), `--skip-tag posix` (on a machine without `/bin/sh`). Exit codes: `0` no `FAIL`, `1` at least one, `2` a load or usage error.

## Optional metadata

A case may carry `source` (where the behavior came from, or `intended: <note>`), a `defect` id and an `intent` paragraph saying why it exists. None is required and none changes how the case runs. There is no `status` key: a case that still has one is a load error.

## Where the call is recorded

- A product decision (new command, new field, changed exit code): a row in `docs/decisions.md` with the options it beat. An `ask` decision is the owner's: recommend, never decide it yourself; a technical call inside an approved plan is yours, reported as DECIDED.
- A defect id kept as history: a row in the defect table of `docs/decisions.md` ("Defect ids (history)").
- A format change (new key, new matcher, new step kind): `contract/SPEC.md` and the runner in the same change (`contract/runner/README.md` shows how to add a matcher).

## Rules

- A behavior change is a case first. Never edit a case's expectation just to turn a `FAIL` green: decide first whether the case or the Go code is wrong.
- Cases assert observable effects only (exit code, output, files, registry events, `orca` calls, `claude` argv and env), never internals.
- A case that pins behavior the owner decided to change changes in the same commit as the decision.
