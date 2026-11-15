# Architecture of the Go CLI

The design of the `jitso` Go binary: how it is split into packages, where the seams are, how the decided contract rows and the known defects land, and the order in which it is ported against the contract. No code exists yet. What the CLI must do is defined by `contract/SPEC.md` and the cases under `contract/cases/`; product decisions are in `docs/decisions.md`; per-behavior calls are in `contract/TS-ODDITIES.md`. When this file and the contract disagree, the contract wins.

## 1. Principles

| Principle | What it means here |
|---|---|
| The contract is the acceptance test | every milestone ends with a runner result against the built binary (`--impl bin`), never with "it compiles" |
| Port the oracle, change only what is decided | behavior changes are limited to the DECIDED rows L2, L3 and L8, the defects S1, S2 and S3, N-1 (project root), the tier model (N-5, N-6), the dispatcher project handoff (N-7) and the launcher word of the dispatcher prompt (N-8). Every other `TS-ODDITIES.md` row, including the `fix` proposals, keeps the oracle behavior until its own contract change |
| Functional core, imperative shell | validation, resolution, card, brief checks, argv and `run.sh` text, hook decisions and transcript math are pure functions; I/O sits in a thin layer around them |
| Standard library first | one dependency (YAML, section 8), behind a single file |
| One binary | the CLI, both hooks and the dispatcher entry are subcommands of the same executable; no Node or Bun at run time |

## 2. Domain map

What the oracle does, split by responsibility. "Pure" means no file, process, clock or random access; the Go package names are the target layout of section 3. Oracle paths (`cli/jitso/...`, `.agents/hooks/...`) are relative to the boilerplate checkout at the tag `contract/README.md` names, and the short file names in the oracle column are under `cli/jitso/`; case ids are relative to `contract/cases/`; catalog paths (`jitso/...`) are relative to a consuming project, and in this repo exist only under `contract/fixtures/default/`.

| Responsibility | Oracle source | Pure logic | I/O | Go package |
|---|---|---|---|---|
| Command dispatch, usage, exit codes | `index.ts` | command table, usage text, "a first word with whitespace is a request" | stdout, stderr | `internal/cli` |
| Project root | `root.ts`, `cli/lib/worktree.ts` | choosing the primary from `--git-dir` vs `--git-common-dir` | `git rev-parse` | `internal/project` |
| Catalog loading and validation | `profile.ts` (`loadConfig`, `validatePreset`, `routeIds`) | every validation rule, route ids from front matter, tiers and the subagent names of a worker (new, section 7) | reads `jitso/`, `.agents/instructions/` | `internal/catalog` |
| Request resolution | `profile.ts` (`resolveRequest`) | allowlists, required fields, field patterns, the refusal of a subagent-tier build (new) | none | `internal/profile` |
| Launch arguments and field allowlist | `launch.ts` (`parseLaunchArgs`), `shell.ts` (`checkLaunchField`) | all | none | `internal/launch` (arguments), `internal/shell` (the field allowlists) |
| Brief checks | `brief.ts` (`checkBrief`) | sections, length, language, cited paths | one existence probe per cited path | `internal/brief` |
| Brief path guard | `brief.ts` (`resolveBriefPath`) | the "inside" test | `EvalSymlinks`, `Stat` | `internal/brief` |
| Card | `card.ts` | all | none | `internal/card` |
| Launch command on the card | `launch.ts` (`resolveLaunchCommand`) | the rule | reads `package.json` | `internal/card` |
| Shell and `run.sh` text | `shell.ts` | quoting, script text, terminal title | none | `internal/shell` |
| Harness adapter | `adapters/claude.ts`, `adapters/types.ts` | argv, env, MCP file, transcript path, trust dialog detection | session id (random) | `internal/harness`, `internal/harness/claude` |
| Orca client | `orca.ts` | response parsing, `worktreeId` path | runs `orca ... --json` | `internal/orca` |
| Trust dialog | `trust.ts` | the dialog state machine, exact real-path match | `git worktree list`, screen reads, key sends, clock | `internal/trust` |
| Registry | `log.ts` | event validation, folding events into launches | append under a `mkdir` lock, read the file | `internal/registry` |
| Launch execution | `launch.ts` (`launchForReal`) | the plan: files, argv, event | files, Orca, registry | `internal/launch` |
| Close | `close.ts` | none worth isolating | Orca, registry, footer file | `internal/closeout` |
| Reap | `reap.ts` | worker matching (handle or `[jitso:<id>]` mark), evidence de-duplication | Orca, registry | `internal/reap` |
| Analyze | `analyze.ts` | transcript scan, prices, verdict, budget percentage | streamed transcript read, `prices.json`, registry | `internal/analyze` |
| Dispatcher | `dispatch.ts`, `jitso/dispatcher/prompt.md` | prompt rendering, catalog summary, the `claude` argv | spawns `claude` with the terminal inherited | `internal/dispatch` |
| Guard hook | `.agents/hooks/jitso-guard.mjs` | Bash segment splitting, launch-verb detection, orca-bridge tool rules | `jitso/jitso.yaml` probe; SessionEnd appends to the registry | `internal/hook` |
| Dispatch guard hook | `.agents/hooks/jitso-dispatch-guard.mjs` | the one allowed launcher line, the brief write rule | `lstat`, real paths, own executable path | `internal/hook` |
| Hard-rules core | `jitso/hard-rules/core.md`, `style.md` | joining the two files | reads them from the project | `internal/launch` (no package of its own) |

The hard-rules core stays a pair of project files during the port because the contract pins their content as read from the fixture. Shipping the core inside the binary (D5) is a later contract change, done with `embed` as the default when a project has no `jitso/hard-rules/`.

## 3. Module and layout

Module path `github.com/upex-galaxy/jitso`. It is the intended path only: nothing is published and the repo has no remote (D2). The module lives at the repo root, separate from `contract/runner`. The `go` line matches the runner's and is raised only by a plan item that names the feature it needs.

```
cmd/jitso/main.go          wiring only: real ports, one exit-code mapping, nothing else
internal/
  cli/                     command table, usage, error kinds and exit codes
    cmd_<name>.go          one file per command (launch, close, reap, analyze, presets, tree, hook, dispatch)
  project/                 primary checkout, dispatcher folder, own executable identity
  catalog/                 load and validate jitso.yaml, presets (all three tiers), route ids
    decode.go              the only file that names the YAML decoder (section 8)
  profile/                 request plus catalog to profile or rejection (pure)
  brief/                   brief checks (pure) and the brief path guard
  card/                    card text and the ROUTE-SCOPE line (pure)
  shell/                   shell quoting, run.sh text, launcher field patterns (pure)
  harness/                 Adapter interface and the launch spec types
    claude/                the Claude Code adapter
  orca/                    client over an injectable runner
  trust/                   trusted-folder check and dialog state machine
  registry/                events, validation, fold, file store with mkdir lock
  jsonx/                   one JSON encoder setup used by every writer (section 5)
  launch/                  argument parsing, plan (pure), execute (I/O)
  closeout/                jitso close
  reap/                    jitso reap
  analyze/                 transcript scan and jitso analyze
  dispatch/                the interactive dispatcher
  hook/                    guard, dispatch-guard, SessionEnd
```

`closeout` avoids a package named after the `close` builtin. Each command registers itself from its own `cmd_<name>.go`, so parallel work never edits a shared file (section 9).

## 4. Seams

Interfaces exist only where I/O meets logic and a test needs a stand-in. Following the Go habit, a consumer declares the small interface it uses; the concrete type lives in the provider package.

| Seam | Shape | Real implementation | In unit tests |
|---|---|---|---|
| Harness | `harness.Adapter`: `ID()`, `Spec(SpecInput) (Spec, error)` returning argv, env and the MCP file, `NewSessionID()`, `TranscriptPath(cwd, session, home)`, `TrustDialog()` | `harness/claude` | the real adapter (it is pure) |
| Orca | `orca.Runner func(args []string) Result`; `orca.Client` is a concrete struct over it. `launch`, `closeout` and `reap` each declare the few `Client` methods they call | `exec` of `$ORCA_CLI_COMMAND` (else `orca`) with `--json`, stdin closed | a scripted `Runner` |
| Registry | `registry.Store`: `Append(Event) error`, `Events() ([]Event, error)` | `registry.FileStore` (directory, lock timeout, stale age) | `FileStore` on `t.TempDir()` |
| Catalog reads | `io/fs.FS` | `os.DirFS(root)` | `testing/fstest.MapFS` |
| Clock and wait | `Now func() time.Time`, `Sleep func(time.Duration)` | `time.Now`, `time.Sleep` | a fake clock that advances on `Sleep` |
| Randomness | `io.Reader` | `crypto/rand.Reader` | a fixed reader |
| Process spawn (dispatcher) | `Spawn func(argv []string, dir string, env []string) (int, error)` | `os/exec` with inherited stdio | a recorder |

Deliberately not abstracted:

- File writes. The launch folder, footer and registry write with `os` directly; unit tests point them at `t.TempDir()`.
- `git`. `project` and `trust` call it through `os/exec`; their units are tested on real temporary repositories, the way the contract does.
- Pure packages (`card`, `shell`, `brief` checks, `profile`, `hook` decisions). They take values and return values.
- No dependency-injection container, no plugin loader, no logger, no generic `Command` interface. A command is a struct with a name, a summary and a `Run` function. A second harness is a second package under `harness/`, registered in one map.
- No `flag` package for `launch`, `close`, `reap` or `analyze`: the contract pins the oracle's own messages (`JITSO REJECTED: unknown argument "<x>"`, `--<name> needs a value`), which `flag` would replace with its own.

## 5. Patterns, with the reason for each

| Pattern | Where | Why |
|---|---|---|
| Ports and adapters at the I/O seams | section 4 | the fakes of the contract and the stand-ins of unit tests plug in at the same points |
| Command registry | `internal/cli` | one table drives dispatch and the usage text, and each command lives in its own file |
| Data-driven validation | `catalog`, `profile`, `shell` | rules are a table of (field, check, message) evaluated in a fixed order, so the order the contract pins (for example B4) is visible in one place |
| Plan, then execute | `launch` | the plan (paths, file contents, argv, env, card, event) is a pure value; execution writes it and is the only place with cleanup (L2) |
| Event-sourced registry | `registry` | append-only lines are the state; a launch is the fold of its events by id; a later event wins over an earlier one; nothing is rewritten |
| Typed errors mapped once | `cli`, `cmd/jitso` | packages return `Rejected` (exit 2, `JITSO REJECTED: ...`), `Usage` (exit 2) or an environment error (exit 3, `jitso: ...`); `main` is the only caller of `os.Exit` |
| Table-driven tests | every package | the go-development skill's rule; contract cases stay the test of observable behavior |
| Consumer-side interfaces | `launch`, `closeout`, `reap` | each declares only the Orca methods it calls, so a fake implements three methods, not nine |

JSON fidelity, in `internal/jsonx`: one encoder with `SetEscapeHTML(false)` (the guard's reason contains `<id>` and `<file>`), optional fields omitted rather than written as `null` when the oracle omits them (`budget_pct`), `mcp.json` with a two-space indent and a trailing newline, registry lines compact with one trailing newline, timestamps as UTC with milliseconds (`2006-01-02T15:04:05.000Z`), and floats in the shortest round-trip form.

## 6. Cross-cutting rules

### Exit codes

`0` done, `2` usage or rejected by validation, `3` environment or Orca error. One exception the contract pins: the dispatcher returns the exit code of the `claude` it ran (`dispatch/claude-exit-status-is-propagated`), and `3` when `claude` cannot start.

### Project root (N-1)

The project is the git primary checkout of the working directory: `git rev-parse --show-toplevel`, `--git-dir` and `--git-common-dir` (absolute paths), primary = parent of the common dir when the checkout is linked. Outside a checkout every command that needs a project exits 3 with `jitso: <cwd> is not inside a git checkout`. `.jitso/` always lives in the primary.

The dispatcher breaks the plain rule: the dispatched `claude` runs from `~/.jitso/dispatch-cwd`, which is not in any checkout, so its one `jitso launch` would exit 3. The oracle avoided this by resolving its root from its own script folder. Design: before spawning the dispatcher, `jitso` resolves the root and passes it in the child's environment as `JITSO_PROJECT`. Every command reads `JITSO_PROJECT` first when it is set, requires it to be inside a git checkout (exit 3 otherwise) and uses that checkout's primary; without it, the cwd rule applies. The model cannot forge the variable: the dispatch guard already denies an environment prefix and any flag outside its list. DECIDED. The cases are `project/dispatcher-handoff/launch-from-outside-git` (a dry run from outside git succeeds with `JITSO_PROJECT` set; a `JITSO_PROJECT` that is not a git checkout exits 3 with one stderr line naming the folder) and `dispatch/hands-the-project-to-claude` (the dispatcher starts `claude` with the variable set), all defect N-7. The design rests on one predicted, unmeasured step: that Claude Code passes its own environment to the processes its Bash tool starts. No contract case can prove it, because the fake `claude` runs no tool. A `--project <path>` launch flag is the fallback if the step does not hold; it is not part of the contract and no case mentions it.

Hooks resolve the project from `CLAUDE_PROJECT_DIR`, else the git primary of the cwd (H7), never from the location of a file.

### Hooks are subcommands

`jitso hook guard` and `jitso hook dispatch-guard` read the hook JSON on stdin and print the hook JSON on stdout, the names `contract/runner/internal/impl/bin.go` already uses. The guard fails open (exit 0, empty stdout) on input it cannot read; the dispatch guard fails closed (H1). Go's RE2 is linear by construction, which settles the guard's linear-time cases.

Launcher identity. Under `--impl bin`, `{{JITSO}}` is whatever path was passed to `--bin`; its basename need not be `jitso`. Both hooks recognize the launcher by comparing the real path of the command's first word with the real path of `os.Executable()` (symlinks resolved on both sides; `/tmp` and `/private/tmp` differ on darwin). The guard also accepts a bare `jitso` and `bun run jitso`, each optionally after `NAME=value` prefixes; `hooks/guard/allow-bash` and `allow-env-prefix` pin those forms (the boilerplate consumes the CLI as `bun run jitso` until D9 completes). DECIDED: the guard keeps all three forms; dropping `bun run jitso` after D9 is a later contract change. The dispatch guard accepts only its own real path followed by `launch`.

The dispatcher registers the dispatch guard in `--settings` as the command `'<own real path>' hook dispatch-guard`, and renders `{{LAUNCHER}}` in the prompt as the same real path. The prompt template writes the launch command as `{{LAUNCHER}} launch ...` with no `bun` in front, because the Go launcher is an executable; the case `dispatch/prompt-launch-line-is-the-guarded-launcher` (defect N-8) pins that the rendered line starts with the launcher and is the line the dispatch guard allows. The oracle's guard allows only `bun <index.ts> launch`, which is why that case is `xfail` on `ts`. The boilerplate's own prompt keeps `bun` until it consumes the Go CLI (D9).

## 7. How the decided rows and the defects land

### Decided rows

| Row | Design | Where |
|---|---|---|
| L2 (N-2) | the execute step creates `.jitso/launches/<id>/` with `os.Mkdir` (so the folder is new) and removes exactly that folder when anything fails before the registry line is appended: a file write, terminal create, idle timeout, a refused trust dialog, `worker-start`, a stuck lock. A terminal it opened is closed first, as today. After the line is written, nothing is removed | `internal/launch` (execute) |
| L3 (N-3) | trust outcome `timeout` keeps exit 0 and the registry line with `trust_dialog: timeout`, and prints one stderr line, for example `jitso: the trust dialog of terminal <handle> did not clear in 10 s; the launch is registered` | `internal/launch`, outcome from `internal/trust` |
| L8 (N-4) | `direct` and `fleet` are the only mode values: `launch --mode` rejects anything else (exit 2, `--mode must be direct or fleet`) and the registry validator treats a line with another mode as invalid, so readers skip it. No alias table exists anywhere | `internal/launch`, `internal/registry` |

### S1 `route_scope`

One function, `card.ScopeLine(ids)`, returns `ROUTE-SCOPE: <ids joined by ,>` (or `ROUTE-SCOPE: none`). The card prints it as a line of its own, so it starts a line as the router's regex requires; the `| ROUTE-SCOPE ...` tail of the subagents line goes away. In direct mode the first prompt becomes `Read <brief> and follow it exactly.` followed by a newline and the same scope line. In fleet mode the brief file already starts with the card, so the scope reaches the session through the brief. Lives in `internal/card`; `internal/launch` builds the first prompt from the same function.

Consequence, DECIDED: the card grows from five lines to six, in this order: header, model line, subagents line, `ROUTE-SCOPE: <ids>`, launch line, budget line. The card goldens (`card/preset/*`, `launch/direct/files-written`) pin that card and are `xfail` on `ts` under S1. A five-line card that satisfies both the old goldens and the router regex exists only through an artificial `. ROUTE-SCOPE:` inside another line, which is rejected here.

### S2 `agents_md`

The adapter's `Spec` returns an environment list next to the argv. For `agents_md: exclude` the Claude adapter adds `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`. `shell.RunScript(env, argv)` writes one `export NAME='value'` line per entry between the shebang and the `exec` line; names must match `^[A-Z_][A-Z0-9_]*$` and values are quoted like every other word. The `env` program is not used, so the `claude` argv stays as it is. The `run.sh` pins (`^#!/bin/sh\n`, `\nexec `) still hold. Lives in `internal/harness/claude` and `internal/shell`.

The variable's documented effect is on CLAUDE.md memory files; a headless canary test measured that it also stops Claude Code's native read of `AGENTS.md` and the `@AGENTS.md` import (`docs/decisions.md`, S2). The case pins only that the variable is in the environment.

### Tiers and subagents (S3, N-5, N-6)

DECIDED. A subagent is a build with `tier: subagent`. Conductor, Worker and Subagent are the three tiers of one build: the field `tier` takes `conductor`, `worker` or `subagent` and is `worker` when absent. The contract does not tell conductor from worker (both are launchable); `role` keeps printing on the card as today. This replaces the earlier proposal of a `jitso/subagents/<name>.md` file format.

- A worker build lists its subagents by name in `subagents: [plan, code, verifier]`. Each name resolves, with the same lookup as `--preset`, to a build with `tier: subagent`. A name that is missing, or that names a build of another tier, is an invalid catalog entry: exit 3, one stderr line naming the file and the subagent (N-6). Because the catalog is validated as a whole, every command fails the same way.
- A subagent build is an ordinary build: the usual allowlists apply to its `model`, `effort` and `tools`, and it adds `prompt`, the system prompt of the subagent, which must not be empty. `description`, `prompt`, `model`, `effort` and `tools` are what the build supplies.
- JITSO passes the resolved builds to `claude` as `--agents <json>`: an object with exactly the listed names as keys. The cases pin `description`, `prompt`, `model` and `tools` of each entry, byte for byte as the build writes them (S3). `effort` is part of what the build supplies, and nothing in the cases forbids emitting it.
- Only those validated fields reach the JSON. Claude Code's `--agents` definition also accepts `mcpServers`, `hooks` and `permissionMode`; a pass-through of the whole build would let one add MCP servers that `--strict-mcp-config` does not strip, or run hooks.
- A subagent-tier build is not launchable alone: `launch --preset plan` is rejected by validation (exit 2, `JITSO REJECTED:` naming the build and the tier), `presets list` does not show it and the dispatcher prompt does not offer it (N-5).
- The adapter emits `--agents` right after the `--session-id` value and before `--append-system-prompt-file`, so the fleet argv still ends with the rules file path (pinned by `argv/sonnet-automator-fleet`).

The default fixture has `sonnet-automator` with `subagents: [plan, code, verifier]` and the three subagent-tier builds `jitso/presets/plan.yaml`, `code.yaml` and `verifier.yaml`. They satisfy the oracle's preset validation too, since the oracle ignores `tier` and `prompt`, and that is why the oracle lists them. Lives in `internal/catalog` (tiers and name resolution), `internal/profile` (the refusal of a subagent-tier build) and `internal/harness/claude` (the JSON).

## 8. The YAML decision

The catalog is YAML (`jitso.yaml`, one file per preset) and the standard library has no YAML. DECIDED: option 1, `go.yaml.in/yaml/v3`.

What the real catalog files use, read from the boilerplate's `jitso/` folder and from every YAML body the contract cases write:

| Feature | Used |
|---|---|
| full-line `#` comments | yes |
| one top-level block mapping | yes |
| block sequences of plain scalars and of one-line flow mappings | yes |
| flow sequences (`[a, b]`, `[]`) and one-line flow mappings (`{ tokens: 250000, usd_list_cap: 0.50 }`) | yes |
| plain scalars with spaces, commas, periods and parentheses | yes |
| single-quoted scalars (`'{ticket_key}'`) | yes |
| integers and one decimal (`0.50`) | yes |
| nested block mappings, anchors or aliases, block scalars (`\|`, `>`), double quotes, inline comments, tags, several documents, tabs | no |

| Option | Binary size | Cost | Risk |
|---|---|---|---|
| 1. `go.yaml.in/yaml/v3` (the maintained fork of the archived `gopkg.in/yaml.v3`) | measured: about 0.2 MB more than a stdlib-only probe of about 3 MB, about 7 percent on darwin/arm64 and linux/amd64; `go.yaml.in/yaml/v4` adds about 0.5 MB | one dependency with no further dependencies, the owner's OK, one review | supply chain of one well-known module |
| 2. a YAML-subset parser in this repo | near zero (predicted: it uses packages already linked) | a few hundred lines plus tests (predicted), and a documented subset | builds that projects write by hand (D5, D6) or that a dispatcher drafts (D7) in ordinary YAML fail on the first feature outside the subset; the subset must reject, never guess |
| 3. change the format to JSON or TOML | JSON zero; TOML also needs a dependency | migrate the catalog, the fixture and the cases | JSON has no comments, which builds rely on; it changes a format users author |

Decision: option 1, `go.yaml.in/yaml/v3`. Tradeoff: about 0.2 MB and one reviewed dependency buy full YAML for the format every adopting project writes by hand; option 2 keeps the binary stdlib-only but makes this repo own a parser and a subset rule its users will hit.

`internal/catalog/decode.go` is the only file that names the decoder: it walks the YAML nodes into an ordered generic map (a plain Go map randomizes the order of the oracle's "success placeholder" messages, which follow document order), and validation runs on those values. Two leaf packages not in section 3 exist: `internal/fail` (error kinds and the exit-code mapping, so `launch` need not import `cli`) and `internal/jsconv` (JavaScript `String()` coercion shared by `catalog`, `orca` and `harness/claude`). Decoding into typed structs is avoided because the oracle coerces values with `String(...)` and ignores unknown keys, so `model: 123` must produce `model "123" is not in the allowlist`, not a type error. Swapping the decoder later is a one-file change.

How the size was measured: stripped probes (`CGO_ENABLED=0`, `-trimpath -ldflags='-s -w'`) built with Go 1.27.4, each linking `encoding/json`, `regexp`, `os/exec`, `crypto/sha256` and `crypto/rand`, one with and one without the YAML module, on darwin/arm64, linux/amd64 and windows/amd64. The numbers are history, not a live value: measure again before a release (`.agents/skills/go-development/references/builds.md`).

## 9. Port plan

Every milestone ends with the runner against the built binary: `<runner> --impl bin --bin <built jitso> --filter '<regex>' --jobs 4`, all `PASS`. The oracle run (`--impl ts`) must keep the expected state that `contract/README.md` prints, updated by milestone 0.

| # | Milestone | Packages |
|---|---|---|
| 0 | Contract edits, no Go: the changes listed below, each run against the oracle first (`ts` `xfail` where the behavior changes) | `contract/` only |
| 1 | Skeleton: command table, usage, error kinds, project root | `cmd/jitso`, `cli`, `project` |
| 2 | Foundation: catalog load and validation (tiers and subagent names included), `presets list`; registry, Orca client, JSON encoder and the harness interface with unit tests only | `catalog`, `registry`, `orca`, `jsonx`, `harness`, `shell` (quoting only) |
| 3 | Launch up to the dry run: arguments, field allowlist, profile, brief checks and path guard, card, launch line | `launch` (parse and plan), `profile`, `brief`, `card`, `shell` |
| 4 | Real launch: files, `run.sh`, Claude adapter, trust dialog, fleet adoption, registry append, L2 and L3, S1 to S3 | `launch` (execute), `harness/claude`, `trust` |
| 5 | Lifecycle: close, reap, analyze | `closeout`, `reap`, `analyze` |
| 6 | Dispatcher | `dispatch` |
| 7 | Hooks | `hook` |
| 8 | Full suite, then cross-compile for the three targets | all |
| 9 | Optional, after green: the `fix` rows of `TS-ODDITIES.md`, one at a time, each a contract change first | as the row needs |

The `--filter` value that proves each milestone (milestone 0 runs the whole suite with `--impl ts` and expects 0 FAIL, 0 XPASS and exactly the planned `XFAIL` set; milestone 8 runs with no filter on both implementations):

```
1  ^(smoke/help|cli/(help-flags|no-arguments|unknown-command|unknown-command-empty-word|tree-not-implemented|presets-without-list))
2  ^(cli/presets-list|project/(linked-worktree|outside-git)/presets-list)
3  ^(validation/|brief/|card/|cli/reject-wiring/|smoke/launch-dry-run|project/subfolder-launch-dry-run|project/outside-git/launch|project/dispatcher-handoff/|defects/s1[ab])
4  ^(launch|argv|escape|registry|defects|project)/
5  ^(close|reap|analyze)/
6  ^dispatch/
7  ^(hooks/|smoke/guard)
```

In milestone 3, `card/dry-run/adds-nothing-after-a-real-launch` matches the filter but needs a real launch, so it passes only with milestone 4. In milestone 6, `dispatch/prompt-launch-line-is-the-guarded-launcher` passes only with milestone 7, because its second step runs the dispatch guard.

L2, L3 and L8 are already in the contract as defects N-2 to N-4 (`docs/decisions.md`, `contract/README.md`). Milestone 0 holds the remaining contract edits the port depends on, each `xfail` on `ts` with its defect id:

- S1: the card goldens under `card/preset/` and the `line_count` pin in `launch/direct/` follow the six-line card.
- S3, N-5, N-6 (tiers): the default fixture has the three subagent-tier builds; the `defects/s3-*` cases pin `--agents` from them; `validation/tier/` pins the refusal of a subagent-tier build; the `presets list` cases and the dispatcher catalog case pin the four launchable builds only; `validation/subagents/` pins the catalog error for a bad subagent name.
- N-7 (dispatcher handoff): `project/dispatcher-handoff/` and `dispatch/hands-the-project-to-claude`, section 6.
- N-8 (launcher word): the fixture prompt template says `{{LAUNCHER}} launch` and `dispatch/prompt-launch-line-is-the-guarded-launcher` pins the line, section 6.
- N-2: the registry lock case also pins that no launch folder is left behind.

### What can run in parallel

Milestones 1 and 2 are sequential and done by one session: they create every shared package (`cli`, `project`, `catalog`, `registry`, `orca`, `jsonx`, `harness`) and start `shell` with only its quoting function. After milestone 2 the shared packages are frozen; a change to one goes through the coordinating session, never through a track. Three tracks can then run as separate Sonnet subagents, each in its own packages and its own `cmd_<name>.go`:

| Track | Milestones | Owns | Reads only |
|---|---|---|---|
| A, launch | 3 then 4 | `launch`, `profile`, `brief`, `card`, `shell`, `trust`, `harness/claude`, `cmd_launch.go` | `catalog`, `registry`, `orca`, `project` |
| B, lifecycle | 5 | `closeout`, `reap`, `analyze`, `cmd_close.go`, `cmd_reap.go`, `cmd_analyze.go` | `registry`, `orca`, `catalog` |
| C, dispatcher and hooks | 6 and 7 | `dispatch`, `hook`, `cmd_dispatch.go`, `cmd_hook.go` | `catalog`, `registry`, `project`, `shell` (quoting only) |

Track C needs `shell.Quote` for the `--settings` command, so `shell` starts in milestone 2 with only that function; from then on track A owns `shell`, and track C uses `Quote` alone and gets no new symbol from it during the tracks. A few lifecycle cases (for example `reap/unlogged-worker-after-a-real-fleet-launch`) start with a real launch and pass only once track A lands; track B reports them as pending, not as failures of its own. The package boundaries make this safe because no two tracks own the same file, and the only shared registration point is a file per command.

## 10. Decisions on the open questions

| # | Question | Answer |
|---|---|---|
| 1 | YAML: dependency, own subset parser, or another format (section 8) | DECIDED: `go.yaml.in/yaml/v3`, behind the one file `internal/catalog/decode.go` |
| 2 | S3: where each helper's `description` and `prompt` come from | DECIDED: a subagent is a build with `tier: subagent`; the worker lists it by name, and the build supplies `description`, `prompt`, `model`, `effort` and `tools` (section 7) |
| 3 | S1 makes the card six lines, which changes card goldens the oracle passes today | DECIDED: accept; the goldens changed in milestone 0 under defect S1, with `ROUTE-SCOPE: <ids>` on its own line after the subagents line |
| 4 | The dispatcher runs outside git, so N-1 alone breaks every dispatched launch | DECIDED: the `JITSO_PROJECT` handoff, validated as a git checkout, with its cases (N-7). The `--project` flag is not part of the contract yet; it stays the fallback if Claude Code does not pass its environment to its Bash tool |
| 5 | Does the variable stop Claude Code's native `AGENTS.md` read and the `@AGENTS.md` import? | MEASURED, no decision needed: it stops both (headless canary test, Claude Code 2.1.325). Re-measure on upgrades; user-level and nested instruction files were not covered |
| 6 | Which launcher forms the guard accepts once the boilerplate stops using `bun run jitso` (H2, D9) | DECIDED: the guard keeps its three forms (own path, bare `jitso`, `bun run jitso`); dropping `bun run jitso` after D9 is a contract change |
| 7 | When the `preset` to `build` rename (D6) happens | DECIDED: after milestone 8, as its own contract change, so the port and the rename are never in the same diff |
| 8 | Windows: `run.sh` needs `/bin/sh`, so `launch` is POSIX only | DECIDED: release 1 builds for Windows with `launch` and the dispatcher unsupported there; read-only commands and hooks are expected to work, still "not tested" |
