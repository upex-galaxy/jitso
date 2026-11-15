# JITSO contract: plan

Status: design record, written 2026-11-14 before the build. The state of the suite is described by `README.md` and `SPEC.md`.

## Goal

A language-independent behavioral contract for the `jitso` CLI and its two hooks. The
current TypeScript binary is the frozen oracle; the Go binary must pass every case.

## What the contract is

A black-box suite. A case is a sandbox (project tree, HOME, scripted fake `orca`, fake
`claude`), a list of steps (argv or hook input), and expectations on observable effects only:

| Observable | Example |
|---|---|
| exit code | `2` for `JITSO REJECTED:` |
| stdout / stderr | the launch card, the reject reason |
| files written | `.jitso/launches/<id>/run.sh`, `brief.md`, `core.md`, `mcp.json`, `card.txt` |
| registry events | the `launch` and `end` lines of `.jitso/launches.jsonl` |
| `orca` calls | the ordered argv the fake recorded (`terminal create`, `terminal wait`, `worker-start`...) |
| what `run.sh` does | executed against a fake `claude`: exact argv, selected env, no canary file |
| hook decision | `permissionDecision` for a Bash command or an `orca-bridge` tool call |

Nothing inspects internals, so the same cases run against any implementation.

## Layout

```
contract/
  PLAN.md        this file
  README.md      how to run it
  SPEC.md        the case format and the matchers (the only normative file for the format)
  fixtures/      base project(s): jitso.yaml, presets, hard-rules, .mcp.json, instruction stubs
  cases/<area>/  one folder per case: case.json (+ files the case needs)
  runner/        Go module, standard library only
```

## Decisions (DECIDED, with the option beaten)

| Decision | Beaten | Why |
|---|---|---|
| Runner in Go, stdlib only | a Bun/TS runner; a shell script | same toolchain as the product, no runtime needed to test a Go binary, no YAML dependency, runs `go vet`/`go test` on itself |
| Cases are JSON, fixtures are plain files | YAML cases | stdlib parses JSON; multi-line briefs live as real files, not escaped strings |
| Fake `orca` and fake `claude` are the runner binary itself, copied into the sandbox under those names (multicall) | `sh` stub scripts | no dependency on a shell for the fakes, JSON handling in Go, same code on every OS |
| The project root is the git primary checkout of the cwd | the script location (what TS does today) | a compiled binary has no script folder; TS is adapted by copying `cli/jitso` into the sandbox project, so both behave the same under the same cases |
| Implementation under test is chosen by `--impl ts --ts-src <boilerplate>` or `--impl bin --bin <path>` | one hardcoded target | the same suite serves both binaries |
| Hooks are invoked through an adapter map: TS runs `node .agents/hooks/jitso-*.mjs`; the Go binary is expected to answer `jitso hook guard` and `jitso hook dispatch-guard` (stdin JSON, stdout JSON) | keeping `.mjs` in the Go repo | owner wants hooks without Node or Bun; the subcommand names are a proposal the Go design may rename in one file |

## Status model: expected failures are strict

Each case declares a status per implementation: `pass` or `xfail`.

- `pass` and it fails: the run is red (a regression).
- `xfail` and it passes: the run is red too (`UNEXPECTED PASS`), so the oracle cannot drift silently and a fixed defect forces the case to be promoted.
- Go must run with every case `pass`, including the three defects.

## Coverage, in harvest order

Source counts are test cases found by `grep` in the TS repo, not yet de-duplicated.

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

## The three defects (intended behavior, `xfail` on `ts`)

All three were measured on 2026-11-14 (launch `L20261115T005309-e8f8`).

| Id | Intended behavior the case asserts | Today (TS) |
|---|---|---|
| S1 `route_scope` | The card carries `ROUTE-SCOPE: <ids>` (with the colon) and, in `direct` mode, the first prompt passed to `claude` carries the same line. Both must match the router's own regex `(?:^\|[.!?])[ \t>*-]*ROUTE-SCOPE:([^\n]*)`, and the ids must parse back to the preset's list | the card writes `ROUTE-SCOPE` with no colon; the first prompt is only `Read <brief> and follow it exactly.` |
| S2 `agents_md` | A preset with `agents_md: exclude` launches `claude` with `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` in its environment; `full` does not | the field is only logged |
| S3 `subagents` | A preset with `subagents: [plan, code, verifier]` puts `--agents <json>` in the `claude` argv; the JSON object has exactly those keys, each with a non-empty `description` and `prompt` | `subagents: null` is passed, so `--agents` never appears |

Two things I am NOT claiming:

- S2: the variable is in the official docs (`code.claude.com/docs/en/env-vars`: "prevent loading any CLAUDE.md memory files into context, including user, project, and auto memory files"). That it also stops the `@AGENTS.md` import and the native `AGENTS.md` read was inferred when this plan was written and measured afterwards (see `docs/decisions.md`, S2).
- S3: when the contract was first written, where each helper's `description` and `prompt` come from was left open and only the shape was fixed. The owner has since decided it (a subagent is a build with `tier: subagent`, `docs/decisions.md`) and the S3 cases pin the values.

## Validation against the truth

1. Run the suite against the TS binary: every `pass` case passes, the three defects fail as declared.
2. Each failing `pass` case is triaged by hand: either the case misreads the TS behavior (fix the case) or the TS behavior is itself wrong (move it to the defect list and tell the owner). TS wins on disagreement unless the audit already marks the behavior as a defect.
3. The same suite is the acceptance test of the Go port.

## Build order

1. `SPEC.md` (the case format), then the runner skeleton with the fakes.
2. Harvest by area, in parallel (Sonnet subagents, 7-component briefs, one area each).
3. Run against TS, triage, fix.
4. `README.md`, diary line, Engram.

Spend: no cap (lifted by the owner); design and runner by Opus subagents, case harvest by Sonnet subagents.

## Open, not blocking

- Subcommand names for the hooks in Go (`jitso hook guard`) are a proposal.
- Windows: cases that execute `run.sh` need `/bin/sh`; they are tagged `posix` so Windows is not blocked later.
