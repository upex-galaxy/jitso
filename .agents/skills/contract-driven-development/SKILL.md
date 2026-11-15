---
name: contract-driven-development
description: "How every behavior of the jitso CLI and its hooks is added, changed or fixed in this repo: write or edit the contract case first, run it against the frozen TypeScript oracle, implement it in the Go port, run the port, keep the xfail/XPASS status honest, and record the call in TS-ODDITIES.md or docs/decisions.md. Use whenever the request changes what jitso does or prints (a new command or flag, an exit code, the launch card, run.sh, the registry log, close/reap/analyze, the guard hooks, the dispatcher), fixes a defect such as S1 route_scope, S2 agents_md or S3 subagents, ports an area to Go, adds or edits a case under contract/cases, or asks why the runner printed FAIL, XPASS or LOAD. Use it even when the user only says 'fix this bug' or 'make jitso do X'. Do NOT use for: Go style, layout or build questions with no behavior change (go-development), or committing (git-flow-master)."
---

# Contract-driven development

The contract under `contract/` is the definition of JITSO's behavior. The TypeScript implementation is the frozen oracle; the Go port must pass every case. A behavior that is not a case does not exist yet, so every change starts there.

Read first, as needed: `contract/README.md` (run it, read the result), `contract/SPEC.md` (the case format; it wins over everything else), `contract/TS-ODDITIES.md` (oracle behaviors that look accidental), `contract/PLAN.md` (why the contract is built this way).

## The loop

1. **Find the behavior.** Search the cases before writing one: `grep -rl '<flag or text>' contract/cases`. A case's `id` equals its folder path under `contract/cases/`.
2. **Plan it** (AGENTS.md rule 2): which cases change or are added, which status each gets, which Go code moves. Owner's OK before editing.
3. **Write or edit the case.** Format: `contract/SPEC.md`. Cite only files the case's own `project` provides; prefer a `for_each` table to copied cases; put multi-line text in a file next to `case.json` and reference it with `{"from": "..."}`.
4. **Set the status** with the table below.
5. **Run it against the oracle** with `--filter '<regex on the id>'`. Expect `PASS` for pinned behavior and `XFAIL` for intended behavior. Anything else: `references/triage.md`.
6. **Implement in Go** (load `go-development`).
7. **Run the port** with `--impl bin --bin <built jitso> --filter ...`, then the whole suite on both implementations.
8. **Record the call** (below), in the same change as the case.

Commands (build the runner once; use the binary, not `go run`, because `go run` turns exit 2 into 1):

```sh
cd contract/runner && go build -o <scratch dir>/jitso-contract . && cd ..
<scratch dir>/jitso-contract --impl ts --ts-src <boilerplate checkout with cli/jitso/> --filter '^launch/' --jobs 4
<scratch dir>/jitso-contract --impl bin --bin <built jitso> --filter '^launch/' --jobs 4
```

Cases are written in the current vocabulary (`--build`, `jitso/builds/`, `build list`); the oracle still says preset, and the `ts` adapter of the runner maps the new spelling to the old one in one direction (`contract/SPEC.md` section 9). A command the oracle does not have, such as `build validate`, `install` or `setup`, is an unknown command there, so its case is `xfail` on `ts` and must pin text or a file effect that answer cannot produce, or it would `XPASS`.

Useful flags: `-v` (argv, stdout, stderr, sandbox of failing cases), `--keep` (keep the sandboxes), `--skip-tag posix` (on a machine without `/bin/sh`). Exit codes: `0` no `FAIL` and no `XPASS`, `1` at least one, `2` a load or usage error.

## Status

| The behavior | `status` | Also required |
|---|---|---|
| the oracle has it and it is intended | `{"ts": "pass", "go": "pass"}` (the default) | `source`: the oracle test it was harvested from |
| intended, the oracle lacks it (a defect or a new feature) | `{"ts": "xfail", "go": "pass"}` | `defect` id, `intent` paragraph, `source: intended: <note>` |
| the oracle has it but it is an accident the owner decided to change | edit the case to the new behavior with `{"ts": "xfail", "go": "pass"}` | the `TS-ODDITIES.md` row marked decided |

Never set `status.go` to `xfail`: the port must pass everything. While the port is incomplete, scope runs with `--filter` instead of hiding cases.

## Where the call is recorded

- A `TS-ODDITIES.md` row the case settles: change its proposal to the decision and the date, in the same change, and end the cell with `DONE (<defect id>)`; a row nobody has taken up ends with `OPEN (fix, not done)` or `OPEN (owner)` (the legend is at the top of that file). A `fix` row is a technical call inside an approved plan; an `ask` row is the owner's (product behavior): recommend, never decide it yourself.
- A product decision (new command, new field, changed exit code): a row in `docs/decisions.md`.
- A new defect id: a row in the defects table of `docs/decisions.md` and in `contract/README.md`.
- A format change (new key, new matcher, new step kind): `contract/SPEC.md` and the runner in the same change (`contract/runner/README.md` shows how to add a matcher or an adapter).

## Rules

- The oracle is frozen and lives in another repo: never edit it to make a case pass.
- When the oracle and a case disagree, the oracle wins unless the behavior is a listed defect or a decided `TS-ODDITIES.md` row: fix the case, or bring the disagreement to the owner.
- An `XPASS` on `ts` means the case no longer tests what it claims or the oracle moved: find out which before promoting anything.
- Cases assert observable effects only (exit code, output, files, registry events, `orca` calls, `claude` argv and env), never internals, so both implementations run them.
- A case pinned to a known oracle accident (for example TS-ODDITIES L2, the launch folder left behind after a failed launch) changes in the same commit as the decision to fix it.
