# JITSO contract: plan

Status: design record, written 2026-11-14 before the build. The state of the suite is described by `README.md` and `SPEC.md`. The v0.1.0 cut removed the TypeScript oracle, the `ts` adapter and the `xfail` status model from the suite (`docs/decisions.md`, "The v0.1.0 cut"); this record keeps the original design where it still holds and says so where it was cut.

## Goal

A language-independent behavioral contract for the `jitso` CLI and its two hooks. The Go
binary must pass every case. (When this plan was written the frozen TypeScript binary was
the oracle; its code is kept at the tag `jitso-oracle-1` of the original QA boilerplate repo.)

## What the contract is

A black-box suite. A case is a sandbox (project tree, HOME, scripted fake `orca`, fake
`claude`), a list of steps (argv or hook input), and expectations on observable effects only:

| Observable | Example |
|---|---|
| exit code | `2` for `JITSO REJECTED:` |
| stdout / stderr | the launch card, the reject reason |
| files written | `.jitso/launches/<id>/run.sh`, `brief.md`, `mcp.json`, `card.txt` |
| registry events | the `launch` and `end` lines of `.jitso/launches.jsonl` |
| `orca` calls | the ordered argv the fake recorded (`terminal create`, `terminal wait`, `worker-start`...) |
| what `run.sh` does | executed against a fake `claude`: exact argv, selected env, no canary file |
| hook decision | `permissionDecision` for a Bash command or an `orca-bridge` tool call |

Nothing inspects internals, so the same cases run against any binary that speaks the contract.

## Layout

```
contract/
  PLAN.md        this file
  README.md      how to run it
  SPEC.md        the case format and the matchers (the only normative file for the format)
  fixtures/      base project(s): jitso.yaml, presets, .mcp.json, instruction stubs
  cases/<area>/  one folder per case: case.json (+ files the case needs)
  runner/        Go module, standard library only
```

## Decisions (DECIDED, with the option beaten)

| Decision | Beaten | Why |
|---|---|---|
| Runner in Go, stdlib only | a Bun/TS runner; a shell script | same toolchain as the product, no runtime needed to test a Go binary, no YAML dependency, runs `go vet`/`go test` on itself |
| Cases are JSON, fixtures are plain files | YAML cases | stdlib parses JSON; multi-line briefs live as real files, not escaped strings |
| Fake `orca` and fake `claude` are the runner binary itself, copied into the sandbox under those names (multicall) | `sh` stub scripts | no dependency on a shell for the fakes, JSON handling in Go, same code on every OS |
| The project root is the git primary checkout of the cwd | the script location (what the TypeScript code did) | a compiled binary has no script folder |
| The binary under test is chosen by `--impl bin --bin <path>` | a hardcoded target | the suite runs any binary built anywhere |
| Hooks are invoked through the adapter: the Go binary answers `jitso hook guard` and `jitso hook dispatch-guard` (stdin JSON, stdout JSON) | keeping `.mjs` in the Go repo | owner wants hooks without Node or Bun; the subcommand names can be renamed in one file |

## Status model

Every case must `PASS`. There is no declared status: a case that fails is a regression, and
the runner prints `PASS`, `FAIL`, `SKIP` or `LOAD` (`SPEC.md` section 8). The earlier model
(`xfail` per implementation, strict `XPASS`) existed only to track the TypeScript oracle and
was cut with it.

## Coverage, in harvest order

Source counts are test cases found by `grep` in the original TypeScript repo, not yet de-duplicated.

| Area | Source | Cases (approx) |
|---|---|---|
| CLI surface: help, no args, unknown command, `tree`, `presets list` | `index.test.ts` | 8 |
| Launch validation: required flags, every field x hostile value, mode, brief language, fleet needs run+task, unknown preset, missing required field | `launch.test.ts`, `profile.test.ts`, `escape.test.ts` | 40 |
| Brief checks and brief path guard: sections, length, language, ghost paths, symlink, traversal, outside folders | `brief.test.ts`, `escape.test.ts` | 22 |
| Card and dry run: golden text per preset, launch command resolution, subagents text | `card.test.ts`, `launch.test.ts` | 10 |
| Real launch with fake `orca`: files, call order, trust dialog, fleet adoption, orca failure closes the terminal, untrusted folder refuses | `launch.test.ts`, `trust.test.ts`, `orca.test.ts` | 25 |
| Shell-escape gate: 18 hostile briefs as content and as filename, run through `run.sh` | `escape.test.ts`, `shell.test.ts` | 36 |
| Registry log, `close`, `reap`, `analyze` | `log.test.ts`, `close.test.ts`, `reap.test.ts`, `analyze.test.ts` | 25 |
| Dispatcher: `jitso "<request>"` opens the fixed folder, `--tools=Bash,Write`, request last | `dispatch.test.ts`, `dispatcher-prompt.test.ts` | 10 |
| Hooks: `jitso-guard` (deny table, allow table, linear-time prefix, segment splitting), `jitso-dispatch-guard` | `.agents/hooks/jitso-*.test.ts` | 30 |

Not in the contract: Orca's own behavior, the real Claude Code TUI, pricing numbers in `prices.json`.

## The three defects (intended behavior)

All three were measured on the TypeScript code on 2026-11-14 (launch `L20261115T005309-e8f8`); the Go binary implements the intended behavior.

| Id | Intended behavior the case asserts | The TypeScript code did |
|---|---|---|
| S1 `route_scope` | The card carries `ROUTE-SCOPE: <ids>` (with the colon) and, in `direct` mode, the first prompt passed to `claude` carries the same line. Both must match the router's own regex `(?:^\|[.!?])[ \t>*-]*ROUTE-SCOPE:([^\n]*)`, and the ids must parse back to the preset's list | the card writes `ROUTE-SCOPE` with no colon; the first prompt is only `Read <brief> and follow it exactly.` |
| S2 `agents_md` | A preset with `agents_md: exclude` launches `claude` with `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` in its environment; `full` does not | the field is only logged |
| S3 `subagents` | A preset with `subagents: [plan, code, verifier]` puts `--agents <json>` in the `claude` argv; the JSON object has exactly those keys, each with a non-empty `description` and `prompt` | `subagents: null` is passed, so `--agents` never appears |

Two things I am NOT claiming:

- S2: the variable is in the official docs (`code.claude.com/docs/en/env-vars`: "prevent loading any CLAUDE.md memory files into context, including user, project, and auto memory files"). That it also stops the `@AGENTS.md` import and the native `AGENTS.md` read was inferred when this plan was written and measured afterwards (see `docs/decisions.md`, S2).
- S3: when the contract was first written, where each helper's `description` and `prompt` come from was left open and only the shape was fixed. The owner has since decided it (a subagent is a build with `tier: subagent`, `docs/decisions.md`) and the S3 cases pin the values.

## Validation

1. The suite is the acceptance test of the Go binary: every case `PASS`es.
2. A failing case is triaged by hand: either the case misreads the intended behavior (fix the case) or the binary is wrong (fix the binary and record the call in `docs/decisions.md`).

## Build order

1. `SPEC.md` (the case format), then the runner skeleton with the fakes.
2. Harvest by area, in parallel (Sonnet subagents, 7-component briefs, one area each).
3. Run against the binary, triage, fix.
4. `README.md`, diary line, Engram.

Spend: no cap (lifted by the owner); design and runner by Opus subagents, case harvest by Sonnet subagents.

## Open, not blocking

- Subcommand names for the hooks in Go (`jitso hook guard`) are a proposal.
- Windows: cases that execute `run.sh` need `/bin/sh`; they are tagged `posix` so Windows is not blocked later.
