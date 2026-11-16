# Architecture of the Go CLI

The design of the `jitso` Go binary: how it is split into packages, where the seams are, how the decided contract rows and the known defects land, and the order in which it was ported against the contract and extended after the port. The code exists: the module is at the repo root and section 3 says how to read its layout. What the CLI must do is defined by `contract/SPEC.md` and the cases under `contract/cases/`; product decisions are in `docs/decisions.md`; per-behavior calls are in `contract/TS-ODDITIES.md`; the schema of a build is `docs/build-schema.md`. When this file and the contract disagree, the contract wins.

## 1. Principles

| Principle | What it means here |
|---|---|
| The contract is the acceptance test | every milestone ends with a runner result against the built binary (`--impl bin`), never with "it compiles" |
| Port the oracle, change only what is decided | behavior changes from the oracle are the DECIDED rows of `contract/TS-ODDITIES.md` and the defects of `contract/README.md`, each a contract case first (`xfail` on `ts`, with a defect id), then Go. Every other `TS-ODDITIES.md` row, including the `fix` proposals nobody has taken up, keeps the oracle behavior until its own contract change |
| Functional core, imperative shell | validation, resolution, card, brief checks, argv and `run.sh` text, hook decisions and transcript math are pure functions; I/O sits in a thin layer around them |
| Standard library first | one dependency (YAML, section 8), behind a single file |
| One binary | the CLI, both hooks and the dispatcher entry are subcommands of the same executable; no Node or Bun at run time |

## 2. Domain map

What the CLI does, split by responsibility. "Pure" means no file, process, clock or random access; the Go package names are the layout of section 3. Rows with no oracle source are behavior the oracle never had (defects N-10 and later in `contract/README.md`). Oracle paths (`cli/jitso/...`, `.agents/hooks/...`) are relative to the boilerplate checkout at the tag `contract/README.md` names, and the short file names in the oracle column are under `cli/jitso/`; case ids are relative to `contract/cases/`; catalog paths (`jitso/...`) are relative to a consuming project, and in this repo exist only under `contract/fixtures/default/`.

| Responsibility | Oracle source | Pure logic | I/O | Go package |
|---|---|---|---|---|
| Command dispatch, usage, exit codes | `index.ts` | command table, usage text, "a first word with whitespace is a request" | stdout, stderr | `internal/cli` |
| Project root | `root.ts`, `cli/lib/worktree.ts` | choosing the primary from `--git-dir` vs `--git-common-dir` | `git rev-parse` | `internal/project` |
| Catalog loading and validation | `profile.ts` (`loadConfig`, `validatePreset`, `routeIds`) | every validation rule, route ids from front matter, tiers and the subagent names of a worker (section 7) | reads `jitso/`, `.agents/instructions/` | `internal/catalog` |
| Build layers, user config | none (D6, N-12) | the order repo, trusted parents, user, draft; shadowing by id; trust matched on real paths | reads the layer folders and `~/.jitso/config.yaml` | `internal/catalog` (`layers.go`, `userconfig.go`) |
| Build validation report | none (N-13) | a tolerant scan that reports every problem of every file, and the launch-only rules | none | `internal/catalog` (`scan.go`), `internal/cli` |
| Request resolution | `profile.ts` (`resolveRequest`) | allowlists, required fields, field patterns, the refusal of a subagent-tier build (new) | none | `internal/profile` |
| Launch arguments and field allowlist | `launch.ts` (`parseLaunchArgs`), `shell.ts` (`checkLaunchField`) | all | none | `internal/launch` (arguments), `internal/shell` (the field allowlists) |
| Brief checks | `brief.ts` (`checkBrief`) | sections, length, language, cited paths | one existence probe per cited path | `internal/brief` |
| Brief path guard | `brief.ts` (`resolveBriefPath`) | the "inside" test | `EvalSymlinks`, `Stat` | `internal/brief` |
| Card | `card.ts` | all | none | `internal/card` |
| Launch command on the card | `launch.ts` (`resolveLaunchCommand`) | the rule | reads `package.json` | `internal/card` |
| Shell and `run.sh` text | `shell.ts` | quoting, script text, terminal title | none | `internal/shell` |
| Harness adapter | `adapters/claude.ts`, `adapters/types.ts` | argv, env, MCP file, transcript path, trust dialog detection | session id (random) | `internal/harness`, `internal/harness/claude` |
| Orca client | `orca.ts` | response parsing, `worktreeId` path | runs `orca ... --json` | `internal/orca` |
| Trust dialog and folder check | `trust.ts` | the dialog state machine, exact real-path match, and the folder check on every launch (N-22): `SelectorPath`, `Precheck` (before any Orca call) and `Place` (right after `terminal create`) | `git worktree list`, screen reads, key sends, clock | `internal/trust` |
| Registry | `log.ts` | event validation, folding events into launches | append under a `mkdir` lock, read the file | `internal/registry` |
| Launch execution | `launch.ts` (`launchForReal`) | the plan: files, argv, event | files, Orca, registry | `internal/launch` |
| Close | `close.ts` | none worth isolating | Orca, registry, footer file | `internal/closeout` |
| Reap | `reap.ts` | worker matching (handle or `[jitso:<id>]` mark), evidence de-duplication | Orca, registry | `internal/reap` |
| Analyze | `analyze.ts` | transcript scan, prices, verdict, budget percentage | streamed transcript read, `prices.json`, registry | `internal/analyze` |
| Dispatcher | `dispatch.ts`, `jitso/dispatcher/prompt.md` | prompt rendering, catalog summary, the `claude` argv | spawns `claude` with the terminal inherited | `internal/dispatch` |
| Guard hook | `.agents/hooks/jitso-guard.mjs` | Bash segment splitting, launch-verb detection, orca-bridge tool rules | `jitso/jitso.yaml` probe; SessionEnd appends to the registry | `internal/hook` |
| Dispatch guard hook | `.agents/hooks/jitso-dispatch-guard.mjs` | the one allowed launcher line, the brief write rule | `lstat`, real paths, own executable path | `internal/hook` |
| Hard-rules core | `jitso/hard-rules/core.md`, `style.md` | joining the two files | reads them from the project; the embedded default when the folder is missing | `internal/rules` (the shipped text), `internal/launch` (the join) |
| Templates | none (N-14) | one embedded template per tier and the `jitso/jitso.yaml` skeleton | none | `internal/templates` |
| Drafts | none (N-15) | the draft checks, the status rewrite (`catalog.StatusSpan`), the save token | the drafts folder, atomic writes, the save | `internal/drafts` |
| Adoption core | none (N-16 to N-19) | a change plan as a value, the unified diff with its masks, the confirmation rule | backups, atomic writes, links | `internal/adopt` |
| Settings file edit | none (N-17) | an order-preserving JSON tree, hook registration and removal | none | `internal/settingsjson` |
| Instruction block | none (N-18) | marker scanning, insert, replace, remove | reads which instruction files exist | `internal/instructions` |
| Usage skill | none (N-16) | the embedded tree, the manifest, the state of an installed copy, the install plan | reads the install folders | `internal/skill` |
| Doctor | none (N-20) | one check per environment fact, each answering ok, warn or fail | PATH lookups, version probes, `git check-ignore`, the files it inspects; it changes nothing | `internal/doctor` |
| Version | none (N-21) | the one-line text | build info | `internal/version` |
| Terminal test | none | none | the termios ioctl on standard input | `internal/tty` |

The hard-rules core is the text in `internal/rules` (`go:embed`), and it is the default only when a project has no `jitso/hard-rules/` folder: a project that has the folder owns both files and nothing is prepended (`docs/decisions.md`, defect N-14).

## 3. Module and layout

Module path `github.com/upex-galaxy/jitso`. It is the intended path only: nothing is published and the repo has no remote (D2). The module lives at the repo root, separate from `contract/runner`. The `go` line matches the runner's and is raised only by a plan item that names the feature it needs.

The layout is read from the tree, not from this page: `ls internal cmd/jitso` and `go list ./...` are current, and the table below groups the packages by responsibility so a reader knows where to look. A package that owns text the binary ships (`rules`, `templates`, `skill`, `instructions`) keeps it next to its code.

```
cmd/jitso/main.go          wiring only: real ports, one exit-code mapping, nothing else
internal/<package>/        everything else
```

| Responsibility | Packages |
|---|---|
| Command layer | `cli`: command table, usage, error kinds and exit codes, one `cmd_<name>.go` per command (and per subcommand of the groups `build`, `install` and `setup`), the shared flag parser (`flags.go`) and the sub-command group helper (`group.go`) |
| Project and catalog | `project` (primary checkout, state and dispatcher folders, own executable identity), `catalog` (load, validate and scan builds in layers, the user config; `decode.go` is the only file that names the YAML decoder, section 8), `profile` (request plus catalog to profile or rejection, pure), `drafts` (the drafts folder and the save of a draft as a build) |
| Launch | `launch` (argument parsing, plan, execute), `brief` (brief checks and the path guard), `card` (card text and the `ROUTE-SCOPE` line), `shell` (quoting, `run.sh` text, launcher field patterns), `harness` (the `Adapter` interface and launch spec types) with `harness/claude` (the Claude Code adapter), `trust` (trusted-folder check and dialog state machine), `rules` (the shipped hard-rules core and style) |
| Lifecycle | `orca` (client over an injectable runner), `registry` (events, validation, fold, file store with a `mkdir` lock), `closeout` (`jitso close`), `reap`, `analyze` |
| Dispatcher and hooks | `dispatch` (the interactive dispatcher), `hook` (guard, dispatch guard, SessionEnd) |
| Authoring | `templates` (the embedded build templates and the `jitso/jitso.yaml` skeleton) |
| Adoption | `adopt` (the shared core: plan, diff, confirmation, backups, apply), `settingsjson` (the order-preserving settings edit), `instructions` (the delimited block), `skill` (the embedded usage skill, its manifest and install plan), `tty` (is a file a terminal; `tty_darwin.go`, `tty_linux.go`, `tty_other.go`) |
| Diagnosis and identity | `doctor` (the environment checks of `jitso doctor`), `version` (the stamped version line) |
| Leaf helpers | `jsonx` (one JSON encoder setup used by every writer, section 5), `fail` (error kinds and the exit-code mapping), `jsconv` (JavaScript `String()` coercion shared by `catalog`, `orca` and `harness/claude`) |

`closeout` avoids a package named after the `close` builtin. Each command registers itself from its own `cmd_<name>.go`, so parallel work never edits a shared file (section 9). The adoption packages (`adopt`, `settingsjson`, `instructions`, `skill`) never import `cli`: they return `fail` errors and the command files turn them into output.

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
| Terminal test | `System.StdinIsTerminal func() bool` | `tty.IsTerminal(os.Stdin)`, the termios ioctl | a function that answers true or false |
| Adoption run | `adopt.Ports{Stdin, Stdout, Terminal, Now, Rand}` and `adopt.Options` (command, scope, root, backups folder, `--dry-run`, `--yes`) | the command file fills it from `System` | a buffer for stdout, a reader for the answer, a fixed clock and reader |

Deliberately not abstracted:

- File writes. The launch folder, footer, registry and drafts write with `os` directly, and so does `adopt.Apply` after it has made the backups; unit tests point them at `t.TempDir()`.
- `git`. `project` and `trust` call it through `os/exec`; their units are tested on real temporary repositories, the way the contract does.
- Pure packages (`card`, `shell`, `brief` checks, `profile`, `hook` decisions). They take values and return values.
- No dependency-injection container, no plugin loader, no logger, no generic `Command` interface. A command is a struct with a name, a summary and a `Run` function. A second harness is a second package under `harness/`, registered in one map.
- No `flag` package for any command: the contract pins the oracle's own messages (`JITSO REJECTED: unknown argument "<x>"`, `--<name> needs a value`), which `flag` would replace with its own. The adoption commands share one hand parser (`internal/cli/flags.go`) with the launch messages.

## 5. Patterns, with the reason for each

| Pattern | Where | Why |
|---|---|---|
| Ports and adapters at the I/O seams | section 4 | the fakes of the contract and the stand-ins of unit tests plug in at the same points |
| Command registry | `internal/cli` | one table drives dispatch and the usage text, and each command lives in its own file |
| Data-driven validation | `catalog`, `profile`, `shell` | rules are a table of (field, check, message) evaluated in a fixed order, so the order the contract pins (for example B4) is visible in one place |
| Plan, then execute | `launch` | the plan (paths, file contents, argv, env, card, event) is a pure value; execution writes it and is the only place with cleanup (L2) |
| Plan, preview, confirm, apply | `adopt`, the adoption commands | a change is a pure value (paths, old and new bytes, kind), so the dry run, the diff, the backup and the write all read the same plan, and `--remove` is the plan that undoes the add |
| Event-sourced registry | `registry` | append-only lines are the state; a launch is the fold of its events by id; a later event wins over an earlier one; nothing is rewritten |
| Typed errors mapped once | `cli`, `cmd/jitso` | packages return `Rejected` (exit 2, `JITSO REJECTED: ...`), `Usage` (exit 2) or an environment error (exit 3, `jitso: ...`); `main` is the only caller of `os.Exit` |
| Table-driven tests | every package | the go-development skill's rule; contract cases stay the test of observable behavior |
| Consumer-side interfaces | `launch`, `closeout`, `reap` | each declares only the Orca methods it calls, so a fake implements three methods, not nine |

JSON fidelity, in `internal/jsonx`: one encoder with `SetEscapeHTML(false)` (the guard's reason contains `<id>` and `<file>`), optional fields omitted rather than written as `null` when the oracle omits them (`budget_pct`), `mcp.json` with a two-space indent and a trailing newline, registry lines compact with one trailing newline, timestamps as UTC with milliseconds (`2006-01-02T15:04:05.000Z`), and floats in the shortest round-trip form.

## 6. Cross-cutting rules

### Exit codes

`0` done, `2` usage or rejected by validation, `3` environment or Orca error. One exception the contract pins: the dispatcher returns the exit code of the `claude` it ran (`dispatch/claude-exit-status-is-propagated`), and `3` when `claude` cannot start. `jitso doctor` answers `0` when no check fails and `3` when one does (a failed check is an environment problem), and `2` for an argument.

### Project root (N-1)

The project is the git primary checkout of the working directory: `git rev-parse --show-toplevel`, `--git-dir` and `--git-common-dir` (absolute paths), primary = parent of the common dir when the checkout is linked. Outside a checkout every command that needs a project exits 3 with `jitso: <cwd> is not inside a git checkout`. `.jitso/` always lives in the primary, and so do the layers of builds: they are computed from the real path of the primary, so a linked worktree gets the layers of its primary, and drafts are `<primary>/.jitso/drafts/` (section 7). The adoption commands split the two on purpose: `setup` and `install` at project scope write into the checkout of the working folder (a linked worktree has its own `.claude/settings.json`, which Claude Code reads from the session's folder), and keep their backups in the primary's `.jitso/backups/`.

The dispatcher breaks the plain rule: the dispatched `claude` runs from `~/.jitso/dispatch-cwd`, which is not in any checkout, so its one `jitso launch` would exit 3. The oracle avoided this by resolving its root from its own script folder. Design: before spawning the dispatcher, `jitso` resolves the root and passes it in the child's environment as `JITSO_PROJECT`. Every command reads `JITSO_PROJECT` first when it is set, requires it to be inside a git checkout (exit 3 otherwise) and uses that checkout's primary; without it, the cwd rule applies. The model cannot forge the variable: the dispatch guard already denies an environment prefix and any flag outside its list. DECIDED. The cases are `project/dispatcher-handoff/launch-from-outside-git` (a dry run from outside git succeeds with `JITSO_PROJECT` set; a `JITSO_PROJECT` that is not a git checkout exits 3 with one stderr line naming the folder) and `dispatch/hands-the-project-to-claude` (the dispatcher starts `claude` with the variable set), all defect N-7. The design rests on one step the contract cannot prove, because the fake `claude` runs no tool: that Claude Code passes its own environment to the processes its Bash tool starts. It was measured on Claude Code 2.1.325 in a headless run: a command run by the Bash tool saw `JITSO_PROJECT`, and the control run without the variable did not. The interactive TUI path was not measured. A `--project <path>` launch flag is the fallback if the step stops holding; it is not part of the contract and no case mentions it.

Hooks resolve the project from `CLAUDE_PROJECT_DIR`, else the git primary of the cwd (H7), never from the location of a file.

### Is a terminal

A command that asks a person a question asks only when standard input is a terminal, and "is a terminal" is a real isatty: `internal/tty` asks the kernel with the termios ioctl (`TIOCGETA` on macOS, `TCGETS` on Linux) and answers false on every other OS, for a pipe and for a regular file. A character-device test is wrong because `/dev/null` is one, and the contract runner gives it to every step that has no `stdin`. The commands that ask are `build save` and the adoption commands (`install skill`, `setup hook`, `setup instructions`); with no terminal they refuse (exit 2) unless the owner passed the explicit `--yes` (adoption) or `--confirm <token>` (`build save`). The contract never has a terminal, so those two flags are the success paths it can run, and the terminal path is a unit test with an injected terminal and reader.

### Hooks are subcommands

`jitso hook guard` and `jitso hook dispatch-guard` read the hook JSON on stdin and print the hook JSON on stdout, the names `contract/runner/internal/impl/bin.go` already uses. The guard fails open (exit 0, empty stdout) on input it cannot read; the dispatch guard fails closed (H1).

What the guard denies to a session that has it installed: the launch verbs (`worker-start` and the like), the orca-bridge tools that start workers or open terminals with a harness, a Bash segment whose program is `claude`, `codex` or `opencode` (N-24), and the owner-only verbs `jitso build save`, `jitso setup` and `jitso install`. The write boundary (N-25) is `hook.Boundary`, pure over three injected file-system calls (`hook.FS`): `ToolDenial` judges the four file tools and `BashWrites` the segments of a Bash line in order, following `cd` and `pushd`. `Area` is the component rule (`jitso` followed by `builds`, `hard-rules` or a last `jitso.yaml`, in any letter case, so it covers every checkout with no git call); `PathArea` expands `~` and the three variables, folds `..`, follows symbolic links (a dangling link and a missing tail included, at most 40 hops) and falls back to a text check for a word it cannot resolve; the anchor rule (`jitso/` and what holds it) applies only to `rm`, `rmdir` and the sources of `mv`, `git rm` and `git mv`. The writers come from a table keyed by the program `hook.Program` finds, so a wrapper or `sh -c` is followed the same way as for a harness. `RunGuard` builds the boundary (the primary checkout is looked up only when a removal operand needs it). `setup hook` and `doctor` know a fourth slot, `PreToolUse` on `Write|Edit|MultiEdit|NotebookEdit` (`settingsjson.MatcherWrite`), which an older three-group file lacks. The program of a segment is found by `hook.Program` (pure, in `internal/hook/program.go`: it skips `NAME=value` words, shell keywords and a table of wrappers, and hands the words after a shell `-c`, `eval` or `env -S` back to itself, up to 8 levels), and `hook.HarnessProgram` asks it whether the program is one of the three harnesses, after the verb checks of each segment. The launcher's own `run.sh` needs no exemption, because Orca starts it and a session's Bash call that runs it has the program `sh`. The splitter is the one the verbs use, so text after `(`, a backtick or a newline is read as a command; the deny text says to put such text in a file. An owner-only verb is matched on text in every launcher form (bare word, `bun run jitso`, a path ending in `jitso`, this binary's own path, after `NAME=value` words or a separator, inside `sh -c '...'`), and `setup` and `install` are allowed with a standalone `--dry-run`, because a dry run only reads. `build draft`, `build discard`, `build list`, `build validate`, `build new`, `doctor` and `version` stay allowed. The dispatch guard still allows one Write (a brief) and one Bash line (the launch), so the dispatcher cannot draft or save. The rows and their limits are in `docs/decisions.md` (defects N-15 and N-19). `jitso setup hook` registers the guard only, never the dispatch guard, which only the dispatcher registers through its own `--settings`. Go's RE2 is linear by construction, which settles the guard's linear-time cases.

Launcher identity. Under `--impl bin`, `{{JITSO}}` is whatever path was passed to `--bin`; its basename need not be `jitso`. Both hooks recognize the launcher by comparing the real path of the command's first word with the real path of `os.Executable()` (symlinks resolved on both sides; `/tmp` and `/private/tmp` differ on darwin). The guard also accepts a bare `jitso` and `bun run jitso`, each optionally after `NAME=value` prefixes; `hooks/guard/allow-bash` and `allow-env-prefix` pin those forms (the boilerplate consumes the CLI as `bun run jitso` until D9 completes). DECIDED: the guard keeps all three forms; dropping `bun run jitso` after D9 is a later contract change. The dispatch guard accepts only its own real path followed by `launch`.

The dispatcher registers the dispatch guard in `--settings` as the command `'<own real path>' hook dispatch-guard`, and renders `{{LAUNCHER}}` in the prompt as the same real path. The prompt template writes the launch command as `{{LAUNCHER}} launch ...` with no `bun` in front, because the Go launcher is an executable; the case `dispatch/prompt-launch-line-is-the-guarded-launcher` (defect N-8) pins that the rendered line starts with the launcher and is the line the dispatch guard allows. The oracle's guard allows only `bun <index.ts> launch`, which is why that case is `xfail` on `ts`. The boilerplate's own prompt keeps `bun` until it consumes the Go CLI (D9).

The dispatch guard reads the launch line as a grammar (N-29): after the launcher and `launch`, `--build`, `--brief`, `--mode direct` and `--worktree path:<primary>` each exactly once in any order, then `--field`, `--mcp-exclude` and `--dry-run` as often as needed. `commandProblem` counts the four required flags while it reads the flags (a repeat is denied as it is met), then names the first missing one in the order `--build`, `--brief`, `--mode`, `--worktree`. `writeProblem` also denies any entry that already exists at the brief's name, through an injected `Exists` (`Lstat`), so the dispatcher cannot overwrite a brief; the prompt template says to use a new slug for every brief. A project's own `jitso/dispatcher/prompt.md` is its own file: one written before N-23 has no `--worktree` and its launch line is denied until the flag is added after `--mode direct`. No template shipped in the binary holds the dispatcher prompt (the embedded templates are the build and `jitso.yaml` skeletons), so there is nothing else to keep in step.

The launch line also ends with `--worktree {{WORKTREE}}` (N-23): `dispatch.BuildPlan` renders it as `path:<primary>` (`PlanInput.Primary`, the root the dispatcher already holds) and exits 3 when the launch allowlist for `--worktree` rejects that value, the way it refuses a launcher path the guard could not allow. The dispatch guard learns the primary from `JITSO_PROJECT`, the variable the dispatcher already sets for the handoff: `RunDispatchGuard` resolves it to the real path of its primary checkout and passes it to the pure `DispatchGuard` (`Primary`, `PrimaryProblem`), which allows `--worktree` only as exactly `path:<primary>`. The hook inherits the variable the way it inherits `CLAUDE_PROJECT_DIR`; that is predicted for an extra variable, and the contract pins it through the `env` of the hook step. A launch is therefore never expected in `~/.jitso/dispatch-cwd`, and `trust.Policy` no longer allows it.

## 7. How the decided rows and the defects land

### Decided rows

| Row | Design | Where |
|---|---|---|
| L2 (N-2) | the execute step creates `.jitso/launches/<id>/` with `os.Mkdir` (so the folder is new) and removes exactly that folder when anything fails before the registry line is appended: a file write, terminal create, idle timeout, a refused trust dialog, `worker-start`, a stuck lock. A terminal it opened is closed first, as today. After the line is written, nothing is removed | `internal/launch` (execute) |
| L3 (N-3) | trust outcome `timeout` keeps exit 0 and the registry line with `trust_dialog: timeout`, and prints one stderr line, for example `jitso: the trust dialog of terminal <handle> did not clear in 10 s; the launch is registered` | `internal/launch`, outcome from `internal/trust` |
| L4 (N-22) | `execute` builds one `trust.Policy` and checks the folder on every launch: `Precheck` of the `--worktree` selector before the launch folder is made, `Place` of the folder Orca reports right after `terminal create` and before the idle wait. A failure closes the terminal and rejects with exit 2, so the deferred removal of L2 leaves no folder and no registry line. The trust dialog is answered after that, for a folder that already passed. The dispatcher folder is not allowed (N-23) | `internal/trust` (pure), `internal/launch` (execute, `answerTrust`) |
| L8 (N-4) | `direct` and `fleet` are the only mode values: `launch --mode` rejects anything else (exit 2, `--mode must be direct or fleet`) and the registry validator treats a line with another mode as invalid, so readers skip it. No alias table exists anywhere | `internal/launch`, `internal/registry` |

### S1 `route_scope`

One function, `card.ScopeLine(ids)`, returns `ROUTE-SCOPE: <ids joined by ,>` (or `ROUTE-SCOPE: none`). The card prints it as a line of its own, so it starts a line as the router's regex requires; the `| ROUTE-SCOPE ...` tail of the subagents line goes away. In direct mode the first prompt becomes `Read <brief> and follow it exactly.` followed by a newline and the same scope line. In fleet mode the brief file already starts with the card, so the scope reaches the session through the brief. Lives in `internal/card`; `internal/launch` builds the first prompt from the same function.

Consequence, DECIDED: the card grows from five lines to six, in this order: header, model line, subagents line, `ROUTE-SCOPE: <ids>`, launch line, budget line. The card goldens (`card/build/*`, `launch/direct/files-written`) pin that card and are `xfail` on `ts` under S1. A five-line card that satisfies both the old goldens and the router regex exists only through an artificial `. ROUTE-SCOPE:` inside another line, which is rejected here.

### S2 `agents_md`

The adapter's `Spec` returns an environment list next to the argv. For `agents_md: exclude` the Claude adapter adds `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`. `shell.RunScript(env, argv)` writes one `export NAME='value'` line per entry between the shebang and the `exec` line; names must match `^[A-Z_][A-Z0-9_]*$` and values are quoted like every other word. The `env` program is not used, so the `claude` argv stays as it is. The `run.sh` pins (`^#!/bin/sh\n`, `\nexec `) still hold. Lives in `internal/harness/claude` and `internal/shell`.

The variable's documented effect is on CLAUDE.md memory files; a headless canary test measured that it also stops Claude Code's native read of `AGENTS.md` and the `@AGENTS.md` import (`docs/decisions.md`, S2). The case pins only that the variable is in the environment.

### Tiers and subagents (S3, N-5, N-6, N-11)

DECIDED. A subagent is a build with `tier: subagent`. Conductor, Worker and Subagent are the three tiers of one build: the field `tier` takes `conductor`, `worker` or `subagent` and is `worker` when absent. The contract does not tell conductor from worker (both are launchable); `role` keeps printing on the card as today. This replaces the earlier proposal of a `jitso/subagents/<name>.md` file format. The field matrix of each tier is `docs/build-schema.md`; the validators in `internal/catalog` are its source of truth.

- A worker build lists its subagents by name in `subagents: [plan, code, verifier]`. Each name resolves, with the same layered lookup as `--build`, to a build with `tier: subagent`. A name that is missing, or that names a build of another tier, is an invalid catalog entry (N-6). The whole-catalog rule holds for the repo layer: an invalid file of the repo is exit 3 for every command, with one stderr line naming the file and the subagent. A file of a parent, user or draft layer that is invalid for the launching repo is skipped instead (a warning in `build list`, a rejection at `launch`, silence elsewhere), so one build that is valid in a repo and not in another never breaks the other (N-12).
- A subagent build is an ordinary build: the usual allowlists apply to its `model`, `effort` and `tools`, and it adds `prompt`, the system prompt of the subagent, which must not be empty. `description`, `prompt`, `model`, `effort` and `tools` are what the build supplies. The launch-only fields (`route_scope`, `success`, `budget`, `mode_default`, `agents_md`, `harness`) are optional and unused in this tier, and `skills_first`, `mcp`, `requires` and `subagents` must be empty in it, because a subagent runs inside its worker's session and could not honor them (N-11).
- A worker may launch only with approved subagents: a listed subagent that is `candidate` or `retired` is rejected at launch (exit 2), where the text is used, not at catalog load (N-11).
- JITSO passes the resolved builds to `claude` as `--agents <json>`: an object with exactly the listed names as keys. The cases pin `description`, `prompt`, `model` and `tools` of each entry, byte for byte as the build writes them (S3). `effort` is part of what the build supplies, and nothing in the cases forbids emitting it.
- Only those validated fields reach the JSON. Claude Code's `--agents` definition also accepts `mcpServers`, `hooks` and `permissionMode`; a pass-through of the whole build would let one add MCP servers that `--strict-mcp-config` does not strip, or run hooks.
- A subagent-tier build is not launchable alone: `launch --build plan` is rejected by validation (exit 2, `JITSO REJECTED:` naming the build and the tier), `build list` does not show it and the dispatcher prompt does not offer it (N-5).
- The adapter emits `--agents` right after the `--session-id` value and before `--append-system-prompt-file`, so the fleet argv still ends with the rules file path (pinned by `argv/sonnet-automator-fleet`).

The default fixture has `sonnet-automator` with `subagents: [plan, code, verifier]`, a `sonnet-conductor` that declares `tier: conductor`, and the three subagent-tier builds `jitso/builds/plan.yaml`, `code.yaml` and `verifier.yaml`. They satisfy the oracle's validation too, since the oracle ignores `tier` and `prompt`, and that is why the oracle lists them. Lives in `internal/catalog` (tiers and name resolution), `internal/profile` (the refusal of a subagent-tier build) and `internal/harness/claude` (the JSON).

### Build layers, drafts and adoption (N-10 to N-19)

Rows in `docs/decisions.md`; the shape of each, for a reader of the code:

- Vocabulary (N-10). The word is `build` everywhere a user sees it: the flag `--build`, the folder `jitso/builds/`, the command group `jitso build <subcommand>`, every message and the card header. There is no alias: `--preset`, `presets` and `jitso/presets/` are unknown. The one survivor is the registry's `v: 1` keys `preset` and `preset_version`, a storage schema, not vocabulary. The oracle keeps the old spelling, and the ts adapter of the runner maps the new one to it in one direction (`contract/SPEC.md` section 9).
- Layers (N-12). The lookup order is the repo's `jitso/builds/`, the trusted parent folders (named only by `~/.jitso/config.yaml`), the user's `~/.jitso/builds/`, then the drafts. The nearest file of an id is the build, every layer is validated against the launching repo, and the card and the registry line name the layer (`build_layer`, `build_file`, `build_sha256`). Code: `internal/catalog`.
- Validation report (N-13). `jitso build validate` scans every layer tolerantly and reports every problem; an invalid repo file is a finding there (exit 2), not the exit 3 of the other commands. Code: `internal/catalog` (`scan.go`).
- Core and templates (N-14). `internal/rules` and `internal/templates` are embedded; `jitso build new` writes a draft from a template and, in a fresh project, the allowlists skeleton.
- Drafts (N-15). `.jitso/drafts/<id>.yaml` in the primary, a last layer that never shadows a saved build; `build draft`, `build discard` and `build save` fill, empty and promote it. `build save` is the one way a build becomes approved and needs the owner: a terminal answer, or `--confirm <token>` bound to the bytes he read. Code: `internal/drafts`.
- Adoption (N-16 to N-19). `install skill`, `setup hook` and `setup instructions` share the flow of `internal/adopt`: preview, `--dry-run`, `--yes` or a terminal answer, a backup of every file changed under `.jitso/backups/`, and `--remove` as the exact undo. `setup hook` registers only `jitso hook guard`, in JITSO's own groups of `.claude/settings.json`; `setup instructions` keeps one marked block in the instruction files; `install skill` writes the shipped usage skill as real files in `.agents/skills/jitso/` plus the relative link `.claude/skills/jitso`, with a manifest that tells a hand edit from an old version. The agent boundary of all of them is the launch guard (section 6).
- Doctor (N-20). `jitso doctor` takes no argument and changes nothing: it prints one line per check, `ok`, `warn` or `fail`, with a stable check id (for example `binary`, `path`, `catalog`, `hooks`, `skill-project`, `instructions`), and ends with a summary line. Exit 0 when nothing fails, 3 when a check fails. It is defect N-20 in `contract/README.md`, with its cases under `doctor/`; the oracle has no such command.

## 8. The YAML decision

The catalog is YAML (`jitso.yaml` and one file per build) and the standard library has no YAML. DECIDED: option 1, `go.yaml.in/yaml/v3`.

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

`internal/catalog/decode.go` is the only file that names the decoder: it walks the YAML nodes into an ordered generic map (a plain Go map randomizes the order of the oracle's "success placeholder" messages, which follow document order), and validation runs on those values. Two leaf packages, listed in section 3, serve it: `internal/fail` (error kinds and the exit-code mapping, so `launch` need not import `cli`) and `internal/jsconv` (JavaScript `String()` coercion shared by `catalog`, `orca` and `harness/claude`). Decoding into typed structs is avoided because the oracle coerces values with `String(...)` and ignores unknown keys, so `model: 123` must produce `model "123" is not in the allowlist`, not a type error. Swapping the decoder later is a one-file change.

How the size was measured: stripped probes (`CGO_ENABLED=0`, `-trimpath -ldflags='-s -w'`) built with Go 1.27.4, each linking `encoding/json`, `regexp`, `os/exec`, `crypto/sha256` and `crypto/rand`, one with and one without the YAML module, on darwin/arm64, linux/amd64 and windows/amd64. The numbers are history, not a live value: measure again before a release (`.agents/skills/go-development/references/builds.md`).

## 9. Port plan

Every milestone ended with the runner against the built binary: `<runner> --impl bin --bin <built jitso> --filter '<regex>' --jobs 4`, all `PASS`. The oracle run (`--impl ts`) keeps the expected state that `contract/README.md` prints. The port is done: the milestones below are the order it was built in, each with the artifact that proves it. They are history that still runs, so a reader can re-prove any step with its filter.

| # | Milestone | Packages | Status |
|---|---|---|---|
| 0 | Contract edits, no Go: the changes listed below, each run against the oracle first (`ts` `xfail` where the behavior changes) | `contract/` only | done: the cases carry the defect ids of `contract/README.md` |
| 1 | Skeleton: command table, usage, error kinds, project root | `cmd/jitso`, `cli`, `project` | done: filter 1 |
| 2 | Foundation: catalog load and validation (tiers and subagent names included), `build list`; registry, Orca client, JSON encoder and the harness interface with unit tests only | `catalog`, `registry`, `orca`, `jsonx`, `harness`, `shell` (quoting only) | done: filter 2 |
| 3 | Launch up to the dry run: arguments, field allowlist, profile, brief checks and path guard, card, launch line | `launch` (parse and plan), `profile`, `brief`, `card`, `shell` | done: filter 3 |
| 4 | Real launch: files, `run.sh`, Claude adapter, trust dialog, fleet adoption, registry append, L2 and L3, S1 to S3 | `launch` (execute), `harness/claude`, `trust` | done: filter 4 |
| 5 | Lifecycle: close, reap, analyze | `closeout`, `reap`, `analyze` | done: filter 5 |
| 6 | Dispatcher | `dispatch` | done: filter 6 |
| 7 | Hooks | `hook` | done: filter 7 |
| 8 | Full suite, then cross-compile for the three targets | all | done: the whole suite passes on `--impl bin` and the oracle run keeps its expected state; the cross-compile is `scripts/package.sh` |
| 9 | Optional, after green: the `fix` rows of `TS-ODDITIES.md`, one at a time, each a contract change first | as the row needs | open by design: the rows taken up are marked in `contract/TS-ODDITIES.md`; the rest wait for their own change |

The `--filter` value that proves each milestone (milestone 0 runs the whole suite with `--impl ts` and expects 0 FAIL, 0 XPASS and exactly the planned `XFAIL` set; milestone 8 runs with no filter on both implementations). The ids are the current ones, after the rename of defect N-10:

```
1  ^(smoke/help|cli/(help-flags|no-arguments|unknown-command|unknown-command-empty-word|tree-not-implemented|presets-is-unknown))
2  ^(build/list|project/(linked-worktree|outside-git)/build-list)
3  ^(validation/|brief/|card/|cli/reject-wiring/|smoke/launch-dry-run|project/subfolder-launch-dry-run|project/outside-git/launch|project/dispatcher-handoff/|defects/s1[ab])
4  ^(launch|argv|escape|registry|defects|project)/
5  ^(close|reap|analyze)/
6  ^dispatch/
7  ^(hooks/|smoke/guard)
```

In milestone 3, `card/dry-run/adds-nothing-after-a-real-launch` matches the filter but needs a real launch, so it passes only with milestone 4. In milestone 6, `dispatch/prompt-launch-line-is-the-guarded-launcher` passes only with milestone 7, because its second step runs the dispatch guard.

L2, L3 and L8 were already in the contract as defects N-2 to N-4 (`docs/decisions.md`, `contract/README.md`). Milestone 0 held the remaining contract edits the port depended on, each `xfail` on `ts` with its defect id:

- S1: the card goldens under `card/build/` and the `line_count` pin in `launch/direct/` follow the six-line card.
- S3, N-5, N-6 (tiers): the default fixture has the three subagent-tier builds; the `defects/s3-*` cases pin `--agents` from them; `validation/tier/` pins the refusal of a subagent-tier build; the `build/list/` cases and the dispatcher catalog case pin the launchable builds only; `validation/subagents/` pins the catalog error for a bad subagent name.
- N-7 (dispatcher handoff): `project/dispatcher-handoff/` and `dispatch/hands-the-project-to-claude`, section 6.
- N-8 (launcher word): the fixture prompt template says `{{LAUNCHER}} launch` and `dispatch/prompt-launch-line-is-the-guarded-launcher` pins the line, section 6.
- N-2: the registry lock case also pins that no launch folder is left behind.

### After the port

Each block after milestone 8 is its own contract change, cases first, run against the oracle and then against the built binary, never mixed with another block in one diff. The defect id names the row of `contract/README.md` and `docs/decisions.md`.

| Block | Defect | Packages | Filter that proves it |
|---|---|---|---|
| Rename `preset` to `build` (D6) | N-10 | `cli`, `catalog`, `launch`, `hook`, the runner's ts adapter | `^(cli/presets-is-unknown\|validation/old-preset-flag-is-unknown\|build/list/old-presets-folder-is-ignored\|build/usage)`, then the whole suite |
| Tiers: the field matrix and approved subagents | N-11 | `catalog`, `launch` | `^validation/(tier\|subagents)/` |
| Layers and the user config | N-12 | `catalog`, `launch`, `card` | `^layers/` |
| `build validate` | N-13 | `catalog`, `cli` | `^build/validate/` |
| Shipped core and templates, `build new` | N-14 | `rules`, `templates`, `launch`, `cli` | `^(rules/\|build/new/)` |
| Drafts: `draft`, `discard`, `save`, the save guard | N-15 | `drafts`, `catalog`, `launch`, `tty`, `hook`, `cli` | `^(build/(draft\|discard\|save)/\|build/list/drafts\|launch/draft/\|hooks/guard/deny-build-save)` |
| Adoption: `install skill`, `setup hook`, `setup instructions`, the adoption guard | N-16 to N-19 | `adopt`, `settingsjson`, `instructions`, `skill`, `hook`, `cli` | `^(install/\|setup/\|hooks/guard/(deny-adoption-commands\|allows-read-only-jitso)\|hooks/dispatch-guard/bash-denies-adoption-commands\|cli/usage/)` |
| `doctor` | N-20 | `doctor`, `cli` | `^(doctor/\|cli/usage/)` |
| `version` | N-21 | `version`, `cli` | `^cli/(version/\|usage/)` |
| Release files: packaging, install script, workflow, formula template | none (no case) | `scripts/`, `.github/`, `packaging/` | `scripts/test-install.sh` (offline), and the steps of `docs/release.md` section 3 |

The pipes inside the table are escaped for Markdown; the filter is the text without the backslashes.

### What could run in parallel

Kept as the record of how the port was split; the port is finished. Milestones 1 and 2 are sequential and done by one session: they create every shared package (`cli`, `project`, `catalog`, `registry`, `orca`, `jsonx`, `harness`) and start `shell` with only its quoting function. After milestone 2 the shared packages are frozen; a change to one goes through the coordinating session, never through a track. Three tracks can then run as separate Sonnet subagents, each in its own packages and its own `cmd_<name>.go`:

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
| 4 | The dispatcher runs outside git, so N-1 alone breaks every dispatched launch | DECIDED: the `JITSO_PROJECT` handoff, validated as a git checkout, with its cases (N-7). MEASURED, no further decision: Claude Code 2.1.325 passes its environment to the processes its Bash tool starts, in a headless run (the interactive TUI path was not measured). The `--project` flag is not part of the contract; it stays the fallback if that stops holding |
| 5 | Does the variable stop Claude Code's native `AGENTS.md` read and the `@AGENTS.md` import? | MEASURED, no decision needed: it stops both (headless canary test, Claude Code 2.1.325). Re-measure on upgrades; user-level and nested instruction files were not covered |
| 6 | Which launcher forms the guard accepts once the boilerplate stops using `bun run jitso` (H2, D9) | DECIDED: the guard keeps its three forms (own path, bare `jitso`, `bun run jitso`); dropping `bun run jitso` after D9 is a contract change |
| 7 | When the `preset` to `build` rename (D6) happens | DONE: after milestone 8, as its own contract change (defect N-10), so the port and the rename were never in the same diff |
| 8 | Windows: `run.sh` needs `/bin/sh`, so `launch` is POSIX only | DECIDED: release 1 builds for Windows with `launch` and the dispatcher unsupported there; read-only commands and hooks are expected to work, still "not tested" |
