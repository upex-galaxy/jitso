# JITSO contract

A language-independent behavioral test suite for the `jitso` CLI and its two hooks. It runs
any implementation against the same cases. The frozen TypeScript binary in the boilerplate is
the oracle; the Go binary must pass every case.

| File | What it is |
|---|---|
| `SPEC.md` | the case format and the matchers (normative) |
| `PLAN.md` | why it is built this way, the decisions, the three known defects |
| `TS-ODDITIES.md` | behaviors the oracle has today that look accidental, with a proposal per row |
| `fixtures/default/` | the base project every case copies (catalog, builds, hard rules, instruction stubs) |
| `cases/<area>/<slug>/case.json` | one case per folder |
| `runner/` | the Go runner, standard library only |

## The oracle is a frozen commit

The TypeScript oracle is the `cli/jitso/` folder of the boilerplate repo at commit
`acd3d3de61e85e53e53b6028f4676161a8b34b49` (branch `feat/jitso`, local at the time of
writing), tagged `jitso-oracle-1` in that repo (a local annotated tag). The expected state
below is only valid for that commit. When the boilerplate drops its JITSO code, the tag keeps
it reachable for as long as the suite still runs against `--impl ts`: check it out in a
separate checkout and pass that folder as `--ts-src`.

## Run it

Build the runner once. Use the built binary, not `go run`: `go run` turns every non-zero
exit into 1, and the runner uses 1 for a failed case and 2 for a load error.

```sh
cd contract/runner
go build -o /tmp/jitso-contract .
cd ..
```

Against the TypeScript oracle (needs `bun` and `node` on PATH, and a checkout of the boilerplate):

```sh
/tmp/jitso-contract --impl ts --ts-src /path/to/agentic-qa-boilerplate --jobs 4
```

Against any other binary, for example the Go port:

```sh
/tmp/jitso-contract --impl bin --bin /path/to/jitso --jobs 4
```

The port is expected to answer `jitso hook guard` and `jitso hook dispatch-guard` (stdin JSON,
stdout JSON). Those two names are a proposal; changing them is a one-line edit in
`runner/internal/impl/bin.go`.

Useful flags: `--filter '<regex on id>'`, `--skip-tag posix` (cases that need `/bin/sh`),
`-v` (argv, stdout, stderr and the sandbox path of failing cases), `--keep` (keep sandboxes).
All flags: `runner/README.md`.

## Reading the result

| Line | Meaning |
|---|---|
| `PASS` | the case holds for this implementation |
| `FAIL` | a regression: the case is declared `pass` and broke |
| `XFAIL` | declared `xfail` (a known defect) and failed, as declared |
| `XPASS` | declared `xfail` but passed: promote the case to `pass` |
| `SKIP` | excluded by `--skip-tag` |

Exit codes: `0` no `FAIL` and no `XPASS`; `1` at least one of them; `2` a case could not be
loaded or the invocation is wrong.

Expected state against the TypeScript oracle: `0 FAIL`, `0 XPASS`, `0 LOAD ERROR`, and an
`XFAIL` set that is every case that declares `status.ts: xfail`, each with a `defect` id.
Everything else is `PASS`. The summary line looks like:

```
ts: <n> cases, <n> PASS, 0 FAIL, <n> XFAIL, 0 XPASS, <n> SKIP, 0 LOAD ERROR
```

The `XFAIL` cases, found with `grep -rl '"xfail"' cases`, are the known defects, one design rule and the owner's decisions on TS oddities:

| Defect | Where | Intended behavior |
|---|---|---|
| S1 `route_scope` | `defects/s1*`, `card/build/*`, `launch/direct/files-written` | the card has six lines, with `ROUTE-SCOPE: <ids>` (the colon the router needs) on a line of its own after the subagents line, and the first prompt carries the same line |
| S2 `agents_md` | `defects/s2*` | `agents_md: exclude` launches `claude` with `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` |
| S3 `subagents` | `defects/s3*` | a worker build's `subagents` reach `claude` as `--agents <json>` with exactly those keys, each entry carrying the `description`, `prompt`, `model` and `tools` of the subagent-tier build (`tier: subagent`) of that name |
| N-1 project root | `project/outside-git`, `project/nested-checkout-is-its-own-project` | the project is the git primary checkout of the cwd; outside one the command exits 3; from a repo nested in another one (made with the `git_init` step), the nested repo's catalog is read |
| N-2 failed launch | the failed-launch cases of `launch/orca-failure`, `launch/trust`, `launch/fleet`, `launch/mcp-json`, `registry/lock-busy-fails-the-launch` and `rules/one-rules-file-missing-exits-3` (TS-ODDITIES L2) | a launch that fails before its registry line leaves no `.jitso/launches/<id>/` folder behind; the exit code, the stderr text, the empty registry and the orca calls stay as they are |
| N-3 trust timeout | `launch/trust/dialog-that-never-clears-is-a-timeout` (TS-ODDITIES L3) | a trust dialog that never clears still exits 0 and registers the launch, and stderr carries a warning that names the trust dialog and says it did not clear (the case pins the regex, `intent` has the exact wording rule) |
| N-4 modes | `validation/mode/legacy-alias-*`, `registry/spanish-mode-alias-is-read` (TS-ODDITIES L8) | only `direct` and `fleet` exist: `launch --mode directo` and `--mode flota` exit 2 (`JITSO REJECTED: --mode must be direct or fleet`) and write nothing, and a registry line with either value is an invalid line, ignored like any other |
| N-5 tiers: launch and listing | `validation/tier/*`, `build/list/{hides-subagent-tier,hides-unapproved,order-and-count}`, `project/subfolder-cwd`, `project/linked-worktree/build-list-finds-the-primary-catalog`, `dispatch/system-prompt-catalog` | a subagent-tier build is not launchable alone (`launch --build plan` exits 2, `JITSO REJECTED:`, names the tier) and is not listed by `build list` or offered in the dispatcher catalog |
| N-6 tiers: subagent names | `validation/subagents/*` | a worker whose `subagents` names a missing build, or a build that is not subagent tier, is a catalog error: exit 3, stderr names the file and the subagent |
| N-7 dispatcher project handoff | `project/dispatcher-handoff/*`, `dispatch/hands-the-project-to-claude` | `JITSO_PROJECT` names the project: a launch from a cwd outside git succeeds with it, a `JITSO_PROJECT` that is not a git checkout exits 3 naming the folder, and the dispatcher starts `claude` with the variable set |
| N-8 dispatcher launcher word | `dispatch/prompt-launch-line-is-the-guarded-launcher` | the prompt's launch line starts with the launcher itself, no `bun` word, and is the line the dispatch guard allows |
| N-9 quoted words | `hooks/dispatch-guard/bash-allows-quoted-words` | the dispatch guard allows a single-quoted word only when it holds a space, for the launcher and the value of `--brief`; every other quote stays denied (`hooks/dispatch-guard/bash-denies-misused-quotes`) |
| N-10 build vocabulary | `cli/presets-is-unknown`, `validation/old-preset-flag-is-unknown`, `build/list/old-presets-folder-is-ignored`, `build/usage`, `validation/build/flag-without-value`, `hooks/guard/deny-reason-names-the-build-flag`, and the cases that pin a renamed message: `smoke/help`, `cli/help-flags`, `cli/no-arguments`, `project/subfolder-launch-dry-run`, `analyze/by-id-{clean-transcript-verdict-unknown,unknown-build-is-verdict-unknown,verdict-success-when-every-check-is-evaluable}`, `validation/build/{unknown,not-approved}`, `validation/field/{conductor-needs-none,unknown-for-build,valid-ticket-key}`, `validation/{flag-takes-next-word-as-value,required/{missing-build,no-flags},format/build-rejected,hostile/build}`, `hooks/dispatch-guard/bash-allows-the-launcher` | a profile is a build: the launch flag is `--build`, the catalog folder is `jitso/builds/`, the catalog commands are `jitso build <subcommand>`, and every message says build. `--preset`, `presets` and `jitso/presets/` are unknown, with no alias (docs/decisions.md D6). The registry log keeps its `preset` and `preset_version` keys. The oracle keeps the old spelling, and the ts adapter maps the new one to it in one direction (SPEC.md section 9), so only the cases that pin renamed output are `xfail` |
| N-11 tier fields | `validation/tier/subagent-needs-no-launch-fields`, `validation/tier/subagent-lists-must-be-empty`, `validation/subagents/listed-subagent-must-be-approved` | a subagent-tier build is valid without `route_scope`, `success`, `budget`, `mode_default`, `agents_md` and `harness`; a non-empty `skills_first`, `mcp` or `requires` in one is a catalog error (exit 3, `<field> must be empty in a subagent-tier build`); a worker whose listed subagent is `candidate` or `retired` is rejected at launch (exit 2) |
| N-12 build layers | `layers/*` (no other case changed: a `registry` check names only the fields it pins, so the new `build_*` fields break none) | builds come from layers, nearest first: the repo's `jitso/builds/`, the trusted parent folders of `~/.jitso/config.yaml` that strictly contain the repo (nearest first, compared by real path, `/` and HOME refused, a repo can never declare trust), then `~/.jitso/builds/`; the nearest file of an id is the build; every build is validated against the launching repo's allowlists and route ids, and one that is invalid there is skipped (a stderr warning in `build list`, `launch --build` exits 2 with `is invalid here`); a missing `jitso/jitso.yaml` is exit 3 and an invalid user config is exit 3; `build list` marks a parent or user build `(from <layer> <path>)`, the card header ends `(<layer>)`, and the registry launch line records `build_layer`, `build_file` and `build_sha256` |
| N-14 shipped core and templates | `rules/embedded-core-without-a-hard-rules-folder` | with no `jitso/hard-rules/` folder, `core.md` of the launch folder is the embedded hard-rules core (`# JITSO hard rules`) and reply style, and `core_sha256` is its hash; a project that has the folder keeps using its own two files byte for byte, and one missing file is exit 3 naming it (docs/decisions.md) |

For the Go port every case must `PASS`: set `status.go` to `pass` (the default) and keep
`status.ts` as `xfail` until the oracle is retired.

S2 evidence: the variable is in the official Claude Code docs
(`code.claude.com/docs/en/env-vars`) for `CLAUDE.md` files, and a headless canary test on
Claude Code 2.1.325 showed it also stops the native read of a lone `AGENTS.md` and the
`@AGENTS.md` import of a `CLAUDE.md`. That test did not cover user-level instruction files,
nested or parent folders, or other versions; re-measure when Claude Code is upgraded.

## What the suite does not cover

Orca's own behavior, the real Claude Code TUI, the price numbers in `prices.json`, and
white-box details of the TypeScript code (function names, types). Cases that execute a
generated `run.sh` need `/bin/sh` and carry the tag `posix`.

## Adding a case

1. Create `cases/<area>/<slug>/case.json` (the `id` equals the folder path). Format: `SPEC.md`.
2. Cite only files the case's own `project` provides; the default fixture has
   `docs/guide.md`, `src/sync.ts` and `api/openapi.json`.
3. Run it against the oracle, then against the port. Behavior the oracle lacks is written as
   the intended behavior with `status: {"ts": "xfail", "go": "pass"}`, a `defect` id and an `intent`.
4. Prefer a `for_each` table to copy-pasted cases.
