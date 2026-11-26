# Architecture of the Go CLI

The design of the `jitso` Go binary: how it is split into packages, where the seams are, how the decided contract rows and the known defects land, and the order in which it was ported against the contract and extended after the port. The code exists: the module is at the repo root and section 3 says how to read its layout. What the CLI must do is defined by `contract/SPEC.md` and the cases under `contract/cases/`; product decisions are in `docs/decisions.md`; the schema of a build is `docs/build-schema.md`. When this file and the contract disagree, the contract wins.

## 1. Principles

| Principle | What it means here |
|---|---|
| The contract is the acceptance test | every milestone ends with a runner result against the built binary (`--impl bin`), never with "it compiles" |
| Change only what is decided | a behavior change is a decided row of `docs/decisions.md` (with a defect id when it has one), each a contract case first, then Go |
| Functional core, imperative shell | validation, resolution, card, brief checks, argv and `run.sh` text, hook decisions and the transcript path are pure functions; I/O sits in a thin layer around them |
| Standard library first | one dependency (YAML, section 8), behind a single file |
| One binary | the CLI and the hook are subcommands of the same executable; no Node or Bun at run time |

## 2. Domain map

What the CLI does, split by responsibility. "Pure" means no file, process, clock or random access; the Go package names are the layout of section 3. Case ids are relative to `contract/cases/`; catalog paths (`jitso/...`) are relative to a consuming project, and in this repo exist only under `contract/fixtures/default/`.

| Responsibility | Pure logic | I/O | Go package |
|---|---|---|---|
| Command table, usage, exit codes | command table, usage text | stdout, stderr | `internal/cli` |
| Project root | choosing the primary from `--git-dir` vs `--git-common-dir` | `git rev-parse` | `internal/project` |
| Catalog loading and validation | every validation rule, route ids from front matter, tiers and the subagent names of a worker (section 7) | reads `jitso/`, `.agents/instructions/` | `internal/catalog` |
| Build layers | the order repo, draft; shadowing by id | reads the two layer folders | `internal/catalog` (`layers.go`) |
| Build validation report | a tolerant scan that reports every problem of every file, and the launch-only rules | none | `internal/catalog` (`scan.go`), `internal/cli` |
| Request resolution | allowlists, required fields, the field patterns of the `fields` key of `jitso/jitso.yaml`, the refusal of a subagent-tier build (new) | none | `internal/profile` |
| Launch arguments and field allowlist | all | none | `internal/launch` (arguments), `internal/shell` (the field allowlists) |
| Brief checks | sections, length, language, cited paths, the `Output:` hint after a `paths` failure, and for a pointer brief the target checks (length, language) | one existence probe per cited path | `internal/brief` |
| Brief path guard | the "inside" test, and for a pointer brief the target rules (primary checkout, real path ends in `.md`) | `EvalSymlinks`, `Stat` | `internal/brief` |
| Card | all | none | `internal/card` |
| Launch command on the card | the rule | reads `package.json` | `internal/card` |
| Shell and `run.sh` text | quoting, script text, terminal title | none | `internal/shell` |
| Harness adapter | argv, env, MCP file, extra launch files (a config folder, a guard plugin), transcript path, trust dialog detection, what the harness can honor (`Support`) | session id (random) | `internal/harness`, `internal/harness/claude`, `codex`, `opencode`, `pi` |
| Orca client | response parsing, `worktreeId` path, the refusal of a truncated terminal list | runs `orca ... --json` | `internal/orca` |
| Trust dialog and folder check | the dialog state machine, exact real-path match, and the folder check on every launch (N-22): `SelectorPath`, `Precheck` (before any Orca call) and `Place` (right after `terminal create`) | `git worktree list`, screen reads, key sends, clock | `internal/trust` |
| Registry | event validation (schema `v: 2` with `build` keys, N-31), folding events into launches, reading link events | append under a `mkdir` lock, read the file | `internal/registry` |
| Launch execution | the plan: files, argv, event, the Task id of a `worker-start` receipt | files, Orca, registry | `internal/launch` |
| User allowlist | the keys of `~/.jitso/jitso.yaml` and the scoped names `user:<name>` | reads the user file, `~/.claude/skills/`, `~/.claude.json` | `internal/catalog` (`userlayer.go`, `usermcp.go`) |
| Launch package | which skills, which user servers and which guard travel with a launch, and the tree hash | writes `.jitso/launches/<id>/pkg/`, copies the user skills, reads `~/.claude.json` | `internal/launch` (`pkg.go`, `usermcp.go`) |
| Arsenal | the inventory lines of what a build may name, and what exists but no allowlist names | reads the allowlists, skill folders and MCP files; never prints a value | `internal/arsenal` (`Render` pure, `Collect` reads), `internal/cli` (`cmd_arsenal.go`) |
| Close | the token sum of one transcript stream (`internal/usage`) | Orca, registry, footer file, transcript files | `internal/closeout` |
| Fleet | the join of registry launches with listed terminals into rows (state, unlaunched), the table and the JSON | Orca (`terminal list`), registry; writes nothing | `internal/fleet` |
| Link | which session a word names (launch id, handle, title), the rejections | Orca (`terminal list`, only when a word needs it), registry (one `link` line) | `internal/link` |
| Graph | the snapshot (nodes from the fleet rows, hierarchy edges, synapses from links and messages), the page | Orca (`terminal list`, `orchestration inbox`, `run-list`, `worker-list`, `terminal switch`), `git worktree list`, registry (both only inside a project), a server on `127.0.0.1`; writes nothing | `internal/graph` |
| Reap | worker matching (the handle under `terminal`, `agentTerminalHandle` or `resource.terminalHandle`, or the `[jitso:<id>]` mark), evidence text | Orca, registry | `internal/reap` |
| Guard hook | Bash line lexing for the data commands (the segment splitter for every other line), launch-verb detection, orca-bridge tool rules, the program of a segment (N-24) | `jitso/jitso.yaml` probe; SessionEnd appends to the registry | `internal/hook` |
| Guard files | the generated OpenCode plugin and Pi extension, and their marker line | none: the adoption commands and the launch write them | `internal/guardfiles` |
| Rules text | the short JITSO rules a launch appends to the end of `brief.md`, by mode and budget, and the join of the brief file | writes `brief.md` | `internal/card` (`Rules`), `internal/launch` (`BriefFile`) |
| Templates | one embedded template per tier and the `jitso/jitso.yaml` skeleton | none | `internal/templates` |
| Drafts | the draft checks, the status rewrite (`catalog.StatusSpan`) | the drafts folder, atomic writes, the save | `internal/drafts` |
| Adoption core | a change plan as a value, the unified diff with its masks, the confirmation rule | backups, atomic writes, links | `internal/adopt` |
| Settings file edit | an order-preserving JSON tree, hook registration and removal | none | `internal/settingsjson` |
| Instruction block | marker scanning, insert, replace, remove | reads which instruction files exist | `internal/instructions` |
| Usage skill | the embedded tree, the manifest, the state of an installed copy, the install plan | reads the install folders | `internal/skill` |
| One-command setup (`jitso init`) | the choice of parts by scope and harness; it composes the plans of the skill, the instruction block, the guard, the `jitso/jitso.yaml` skeleton and the `.gitignore` line | the questions on a terminal, one preview and one confirmation through `adopt.RunParts` | `internal/cli` (`cmd_init.go`) |
| Doctor | one check per environment fact, each answering ok, warn or fail | PATH lookups, version probes, `git check-ignore`, the files it inspects; it changes nothing | `internal/doctor` |
| Version | the one-line text | build info | `internal/version` |
| Terminal test | none | the termios ioctl on standard input | `internal/tty` |

The rules text is `card.Rules(mode, hasBudget)`, called with the build's budget presence, pure: a heading and three rules, the direct or the fleet wording, with or without the sentence that names the card's budget as the ceiling, then the lines of `card.Communication`. `launch.BriefFile` appends it to the end of `brief.md` after one blank line (`docs/decisions.md`, the v0.1.0 cut). The block of the launch's field values, `launch.FieldsBlock`, goes between the brief text and the rules, so the rules stay last; `launch.Event` copies the same values into the `fields` object of the launch event, and `catalog.HasControl` keeps a line break out of every value (`docs/decisions.md`, Pre-release fixes).

## 3. Module and layout

Module path `github.com/upex-galaxy/jitso`, the path of the public repository the owner decided on (`docs/decisions.md`); `go.mod` is the source of truth, and `git remote -v` and the tags say whether it is published yet. The module lives at the repo root, separate from `contract/runner`. The `go` line matches the runner's and is raised only by a plan item that names the feature it needs.

The layout is read from the tree, not from this page: `ls internal cmd/jitso` and `go list ./...` are current, and the table below groups the packages by responsibility so a reader knows where to look. A package that owns text the binary ships (`templates`, `skill`, `instructions`) keeps it next to its code.

```
cmd/jitso/main.go          wiring only: real ports, one exit-code mapping, nothing else
internal/<package>/        everything else
```

| Responsibility | Packages |
|---|---|
| Command layer | `cli`: command table, usage, error kinds and exit codes, one `cmd_<name>.go` per command (and per subcommand of the groups `build`, `install` and `setup`), the shared flag parser (`flags.go`) and the sub-command group helper (`group.go`) |
| Project and catalog | `project` (primary checkout, state and user folders, own executable identity), `catalog` (load, validate and scan builds in the repo and draft layers; `decode.go` is the only file that names the YAML decoder, section 8), `profile` (request plus catalog to profile or rejection, pure), `drafts` (the drafts folder and the save of a draft as a build) |
| Launch | `launch` (argument parsing, plan, execute), `brief` (brief checks and the path guard), `card` (card text and the `ROUTE-SCOPE` line), `shell` (quoting, `run.sh` text, launcher field patterns), `harness` (the `Adapter` interface and launch spec types) with `harness/claude`, `harness/codex`, `harness/opencode` and `harness/pi` (one adapter per harness), `trust` (trusted-folder check and dialog state machine) |
| Lifecycle | `orca` (client over an injectable runner), `registry` (events, validation, fold, file store with a `mkdir` lock), `closeout` (`jitso close`), `usage` (the token sum of a transcript), `reap`, `fleet` (`jitso fleet`, read only) |
| Communication | `link` (`jitso link` and the directory of session names), `graph` (`jitso graph`: snapshot, server and the embedded page under `web/`) |
| Hooks | `hook` (guard, SessionEnd), `guardfiles` (the generated OpenCode plugin and Pi extension that run the guard) |
| Discovery | `arsenal` (the inventory behind `jitso arsenal`: `Render` is pure, `Collect` reads the files) |
| Authoring | `templates` (the embedded build templates and the `jitso/jitso.yaml` skeleton) |
| Adoption | `adopt` (the shared core: plan, diff, confirmation, backups, apply), `settingsjson` (the order-preserving settings edit), `instructions` (the delimited block), `skill` (the embedded usage skill, its manifest and install plan), `tty` (is a file a terminal; `tty_darwin.go`, `tty_linux.go`, `tty_other.go`). `jitso init` has no package of its own: `cli` (`cmd_init.go`) composes these |
| Diagnosis and identity | `doctor` (the environment checks of `jitso doctor`), `version` (the stamped version line) |
| Leaf helpers | `jsonx` (one JSON encoder setup used by every writer, section 5), `fail` (error kinds and the exit-code mapping), `jsconv` (JavaScript `String()` coercion shared by `catalog`, `orca` and `harness/claude`) |

`closeout` avoids a package named after the `close` builtin. Each command registers itself from its own `cmd_<name>.go`, so parallel work never edits a shared file (section 9). The adoption packages (`adopt`, `settingsjson`, `instructions`, `skill`) never import `cli`: they return `fail` errors and the command files turn them into output.

## 4. Seams

Interfaces exist only where I/O meets logic and a test needs a stand-in. Following the Go habit, a consumer declares the small interface it uses; the concrete type lives in the provider package.

| Seam | Shape | Real implementation | In unit tests |
|---|---|---|---|
| Harness | `harness.Adapter`: `ID()`, `Spec(SpecInput) (Spec, error)` returning argv, env, the MCP file and extra files, `NewSessionID()`, `TranscriptPath(cwd, session, home)`, `TrustDialog()`, `Support()` | `harness/claude`, `harness/codex`, `harness/opencode`, `harness/pi` | the real adapters (they are pure) |
| Orca | `orca.Runner func(args []string) Result`; `orca.Client` is a concrete struct over it. `launch`, `closeout`, `reap`, `fleet`, `link` and `graph` each declare the few `Client` methods they call | `exec` of `$ORCA_CLI_COMMAND` (else `orca`) with `--json`, stdin closed; `graph` uses the runner with a time limit (`orca.ExecRunnerTimeout`) | a scripted `Runner` |
| Registry | `registry.Store`: `Append(Event) error`, `Events() ([]Event, error)` | `registry.FileStore` (directory, lock timeout, stale age) | `FileStore` on `t.TempDir()` |
| Catalog reads | `io/fs.FS` | `os.DirFS(root)` | `testing/fstest.MapFS` |
| Clock and wait | `Now func() time.Time`, `Sleep func(time.Duration)` | `time.Now`, `time.Sleep` | a fake clock that advances on `Sleep` |
| Randomness | `io.Reader` | `crypto/rand.Reader` | a fixed reader |
| Terminal test | `System.StdinIsTerminal func() bool` | `tty.IsTerminal(os.Stdin)`, the termios ioctl | a function that answers true or false |
| Adoption run | `adopt.Ports{Stdin, Stdout, Terminal, Now, Rand}` and `adopt.Options` (command, scope, root, backups folder, `--dry-run`, `--yes`) | the command file fills it from `System` | a buffer for stdout, a reader for the answer, a fixed clock and reader |

Deliberately not abstracted:

- File writes. The launch folder, footer, registry and drafts write with `os` directly, and so does `adopt.Apply` after it has made the backups; unit tests point them at `t.TempDir()`.
- `git`. `project` and `trust` call it through `os/exec`; their units are tested on real temporary repositories, the way the contract does.
- Pure packages (`card`, `shell`, `brief` checks, `profile`, `hook` decisions). They take values and return values.
- No dependency-injection container, no plugin loader, no logger, no generic `Command` interface. A command is a struct with a name, a summary and a `Run` function. A second harness is a second package under `harness/`, registered in one map.
- No `flag` package for any command: the contract pins the original messages (`JITSO REJECTED: unknown argument "<x>"`, `--<name> needs a value`), which `flag` would replace with its own. The adoption commands share one hand parser (`internal/cli/flags.go`) with the launch messages.

## 5. Patterns, with the reason for each

| Pattern | Where | Why |
|---|---|---|
| Ports and adapters at the I/O seams | section 4 | the fakes of the contract and the stand-ins of unit tests plug in at the same points |
| Command registry | `internal/cli` | one table drives the commands and the usage text, and each command lives in its own file |
| Data-driven validation | `catalog`, `profile`, `shell` | rules are a table of (field, check, message) evaluated in a fixed order, so the order the contract pins (for example B4) is visible in one place |
| Plan, then execute | `launch` | the plan (paths, file contents, argv, env, card, event) is a pure value; execution writes it and is the only place with cleanup (L2) |
| Plan, preview, confirm, apply | `adopt`, the adoption commands | a change is a pure value (paths, old and new bytes, kind), so the dry run, the diff, the backup and the write all read the same plan, and `--remove` is the plan that undoes the add |
| Event-sourced registry | `registry` | append-only lines are the state; a launch is the fold of its events by id; a later event wins over an earlier one; nothing is rewritten. A `link` event belongs to no launch: the fold leaves it out and `registry.Links` reads it |
| Typed errors mapped once | `cli`, `cmd/jitso` | packages return `Rejected` (exit 2, `JITSO REJECTED: ...`), `Usage` (exit 2) or an environment error (exit 3, `jitso: ...`); `main` is the only caller of `os.Exit` |
| Table-driven tests | every package | the go-development skill's rule; contract cases stay the test of observable behavior |
| Consumer-side interfaces | `launch`, `closeout`, `reap` | each declares only the Orca methods it calls, so a fake implements three methods, not nine |

JSON fidelity, in `internal/jsonx`: one encoder with `SetEscapeHTML(false)` (the guard's reason contains `<id>` and `<file>`), optional fields omitted rather than written as `null`, `mcp.json` with a two-space indent and a trailing newline, registry lines compact with one trailing newline, timestamps as UTC with milliseconds (`2006-01-02T15:04:05.000Z`), and floats in the shortest round-trip form.

## 6. Cross-cutting rules

### Exit codes

`0` done, `2` usage or rejected by validation, `3` environment or Orca error. `jitso doctor` answers `0` when no check fails and `3` when one does (a failed check is an environment problem), and `2` for an argument. `launch`, `close` and `reap` check that Orca is installed right after their arguments are parsed and answer `3` with one line before anything is read or written; `launch --dry-run` skips the check.

### Platforms

macOS and Linux run every command. A real `jitso launch` does not run on Windows: it writes `run.sh`, which needs `/bin/sh`, so it exits 3 with one line before it touches Orca or the project, and `--dry-run` works there (`cmd_launch.go`; `docs/decisions.md`, the Windows rows). Installation is `scripts/install.sh` on macOS and Linux and `scripts/install.ps1` on Windows, tested offline by `scripts/test-install.sh` and `scripts/test-install.ps1`. `.github/workflows/ci.yml` runs on all three systems and skips on Windows the contract cases tagged `posix` (`contract/SPEC.md`). Because `internal/tty` answers false outside macOS and Linux, a command that asks a question needs `--yes` on Windows.

### Project root (N-1)

The project is the git primary checkout of the working directory: `git rev-parse --show-toplevel`, `--git-dir` and `--git-common-dir` (absolute paths), primary = parent of the common dir when the checkout is linked. Outside a checkout every command that needs a project exits 3 with `jitso: <cwd> is not inside a git checkout`. `.jitso/` always lives in the primary, and so do the layers of builds: they are computed from the real path of the primary, so a linked worktree gets the layers of its primary, and drafts are `<primary>/.jitso/drafts/` (section 7). The adoption commands split the two on purpose: `setup` and `install` at project scope write into the checkout of the working folder (a linked worktree has its own `.claude/settings.json`, which Claude Code reads from the session's folder), and keep their backups in the primary's `.jitso/backups/`.

Hooks resolve the project from `CLAUDE_PROJECT_DIR`, else the git primary of the cwd (H7), never from the location of a file.

### Is a terminal

A command that asks a person a question asks only when standard input is a terminal, and "is a terminal" is a real isatty: `internal/tty` asks the kernel with the termios ioctl (`TIOCGETA` on macOS, `TCGETS` on Linux) and answers false on every other OS, for a pipe and for a regular file. A character-device test is wrong because `/dev/null` is one, and the contract runner gives it to every step that has no `stdin`. The commands that ask are `build save` and the adoption commands (`init`, `install skill`, `setup hook`, `setup instructions`); with no terminal they refuse (exit 2) unless the owner passed the explicit `--yes`, and `--yes` on a terminal skips the question. The contract never has a terminal, so `--yes` is the success path it can run, and the terminal path is a unit test with an injected terminal and reader.

### Hooks are subcommands

`jitso hook guard` reads the hook JSON on stdin and prints the hook JSON on stdout, the name `contract/runner/internal/impl/bin.go` already uses. The guard fails open (exit 0, empty stdout) on input it cannot read.

What the guard denies to a session that has it installed: the launch verbs (`worker-start` and the like), the orca-bridge tools that start workers or open terminals with a harness, a Bash segment whose program is `claude`, `codex`, `opencode` or `pi` (N-24), and the owner-only verbs `jitso build save`, `jitso init`, `jitso setup` and `jitso install`. The guard is two branches of one entry point, picked by `tool_name`: a Bash line is judged command by command, and a tool of an orca MCP server by name and arguments; every other tool passes. `jitso setup hook` registers three groups, each with the handler `jitso hook guard`: under `PreToolUse` the matchers `Bash` and `mcp__.*orca.*__.*` (`settingsjson.MatcherBash`, `MatcherMCP`, in that order), then the one without matcher under `SessionEnd`. `doctor` checks the same three slots and warns (`warn hooks`) about a file that lacks one. The program of a segment is found by `hook.Program` (pure, in `internal/hook/program.go`: it skips `NAME=value` words, shell keywords and a table of wrappers, and hands the words after a shell `-c`, `eval` or `env -S` back to itself, up to 8 levels), and `hook.HarnessProgram` asks it whether the program is one of the four harnesses, after the verb checks of each segment. The launcher's own `run.sh` needs no exemption, because Orca starts it and a session's Bash call that runs it has the program `sh`. A Bash line is read by a small shell lexer (`internal/hook/lex.go`): separators split commands only outside quotes, the commands of `$(...)`, backticks and process substitutions are commands of their own, and a heredoc body stays out of the commands, except for the substitutions of a body whose delimiter is not quoted. When every command of the line is a data command (`git commit` and `git tag` (the `-m` or `--message` value, a heredoc with `-F -`), `orca orchestration send`, `ask` and `reply` (`--body`, `--subject`, `--question`), `gh pr`, `issue` and `release` with `create`, `edit` or `comment` (`--body`, `-b`, `--title`, `-t`, `--notes`, `-n`), the arguments of `echo` and `printf`, and `cat` that reads a heredoc and writes to its output or a `>` or `>>` file; `hook.dataPositions`), a word in a data position that holds whitespace is masked for the verb matchers. Any other line, and a line that does not lex (an unbalanced quote, parenthesis or backtick, a heredoc without its delimiter, nesting past 64), is judged by the old splitter (`Segments`: cut at every separator, quotes removed, nothing masked). The deny reasons do not depend on the path. An owner-only verb is matched on text in every launcher form (bare word, `bun run jitso`, a path ending in `jitso`, this binary's own path, after `NAME=value` words or a separator, inside `sh -c '...'`), and `init`, `setup` and `install` are allowed with a standalone `--dry-run`, because a dry run only reads. `build draft`, `build discard`, `build list`, `build validate`, `build new`, `fleet`, `link`, `graph`, `doctor` and `version` stay allowed. The rows and their limits are in `docs/decisions.md` (defects N-15 and N-19). Go's RE2 is linear by construction and the lexer is one pass with bounded nesting, which settles the guard's linear-time cases.

Launcher identity. Under `--impl bin`, `{{JITSO}}` is whatever path was passed to `--bin`; its basename need not be `jitso`. The hook recognizes the launcher by comparing the real path of the command's first word with the real path of `os.Executable()` (symlinks resolved on both sides; `/tmp` and `/private/tmp` differ on darwin). The guard also accepts a bare `jitso` and `bun run jitso`, each optionally after `NAME=value` prefixes; `hooks/guard/allow-bash` and `allow-env-prefix` pin those forms (the boilerplate consumes the CLI as `bun run jitso` until D9 completes). DECIDED: the guard keeps all three forms; dropping `bun run jitso` after D9 is a later contract change.

Where the guard is registered. In the project's `.claude/settings.json` (`setup hook`), in `~/.claude/settings.json` for the owner's own session (`setup hook --scope user`), in the files of the other harnesses that `setup hook --harness` writes (below), and, for a launched session, in the JSON of the launch's `--settings` flag: the same three groups, each running the absolute path of the running binary, added only when the project does not already register all three (so a worker is never guarded twice). The launch package (`.jitso/launches/<id>/pkg/`, passed with `--add-dir`) carries user skills and, when the project holds no copy, the embedded jitso skill; it is built in `internal/launch` and the adapter in `internal/harness/claude` adds the flags.

The other harnesses. `setup hook --harness <list>` (claude, codex, opencode, pi, or all; default each one whose program is on PATH, claude when none is) puts every chosen harness in one `adopt.Plan`, so one preview, one confirmation and one backup. Codex reads `.codex/hooks.json` (user scope `$CODEX_HOME/hooks.json`) in the schema of Claude Code's `hooks` object and sends Claude-shaped input, so `internal/settingsjson` writes and removes the same three groups there. OpenCode 2 and Pi take no command hook: `internal/guardfiles` generates `.opencode/plugins/jitso-guard.js` (`export default { id, setup }`, `execute.before`) and `.pi/extensions/jitso-guard.js` (`tool_call`, `session_shutdown`), which run the guard with `spawnSync`, send it a Claude-shaped input, block on a deny and fail open otherwise. The OpenCode plugin runs the guard in `ctx.location.directory`, because `opencode run` works through a background service whose own folder is not the project. A generated file starts with a marker line (`guardfiles.MarkerVersion` reads it): a file with the marker is upgraded, one without it is refused unless `--force`, which backs it up. `doctor`'s `hooks` and `hooks-user` lines add one part per harness file found (ok, or warn, never fail). `cli.HookPlan` builds the plan for the command and for any other caller (`HookSetup` holds the plan, its scope root, its backups folder, the display function and a refusal). `scripts/test-guard-plugins.mjs` imports both generated files with a fake OpenCode context and a fake Pi object and checks deny, allow and fail-open.

Launched sessions of the other harnesses get the guard on every launch, because none reads `.claude/settings.json`: codex as inline hooks (`-c hooks.PreToolUse=...`, `-c hooks.SessionEnd=...` with `--dangerously-bypass-hook-trust`, which trusts every hook of that session; the folder has passed the trust policy, and a terminal in any other folder is closed), opencode as `plugins/jitso-guard.js` of the launch's `OPENCODE_CONFIG_DIR` (`.jitso/launches/<id>/opencode/`, which replaces the owner's `~/.config/opencode`; the TUI runs `--standalone` so it reads that variable), and pi as `-e .jitso/launches/<id>/pi/jitso-guard.js`. The package skills reach opencode through the `skills` key of that config and pi through `--skill`; codex loads none, so a codex build may name no `user:` skill and reads the jitso skill from its own `.agents/skills`. Only a claude launch records a transcript path, and `jitso close` counts tokens only for a claude launch.

## 7. How the decided rows and the defects land

### Decided rows

| Row | Design | Where |
|---|---|---|
| L1 (N-26) | `writeFolder` reads the repo `.mcp.json` only when the launch's MCP list, after `--mcp-exclude`, is not empty; otherwise the harness spec gets no repo table and `mcp.json` is `{"mcpServers": {}}`. A missing or broken file still fails a launch that names a server, with exit 3 | `internal/launch` (`folder.go`) |
| L2 (N-2) | the execute step creates `.jitso/launches/<id>/` with `os.Mkdir` (so the folder is new) and removes exactly that folder when anything fails before the registry line is appended: a file write, terminal create, idle timeout, a refused trust dialog, `worker-start`, a stuck lock. A terminal it opened is closed first, as today. After the line is written, nothing is removed | `internal/launch` (execute) |
| L3 (N-3) | trust outcome `timeout` keeps exit 0 and the registry line with `trust_dialog: timeout`, and prints one stderr line, for example `jitso: the trust dialog of terminal <handle> did not clear in 10 s; the launch is registered` | `internal/launch`, outcome from `internal/trust` |
| L4 (N-22) | `execute` builds one `trust.Policy` and checks the folder on every launch: `Precheck` of the `--worktree` selector before the launch folder is made, `Place` of the folder Orca reports right after `terminal create` and before the idle wait. A failure closes the terminal and rejects with exit 2, so the deferred removal of L2 leaves no folder and no registry line. The trust dialog is answered after that, for a folder that already passed. | `internal/trust` (pure), `internal/launch` (execute, `answerTrust`) |
| L6 (N-26) | after the build, the brief and the parent have passed and before the card is printed, `Run` calls the pure `UnknownExcludes(mcp, exclude)` and prints one stderr line per distinct `--mcp-exclude` value that is not exactly a name of the build's `mcp` list, with a `did you mean` hint for a case-only match. Dry run and real launch alike; nothing is excluded and the exit code is unchanged | `internal/launch` (`plan.go`, `launch.go`) |
| L7 (N-26) | one predicate, `card.DeclaresJitsoScript(packageJSON)`: `scripts.jitso` is a JSON string that is not blank. `card.LaunchCommand` and the guard's `hook.LauncherWord` both call it, so the card and the deny reason name the same launcher | `internal/card`, `internal/hook` |
| L8 (N-4) | `direct` and `fleet` are the only mode values: `launch --mode` rejects anything else (exit 2, `--mode must be direct or fleet`) and the registry validator treats a line with another mode as invalid, so readers skip it. No alias table exists anywhere | `internal/launch`, `internal/registry` |
| L10 (N-26) | `runLaunch` prints the `launchUsage` constant on stdout and returns when the first word after `launch` is `--help` or `-h`, before `launch.Parse`, so no project, catalog or Orca is needed. Any other position leaves the word to `Parse` as an unknown argument. `close` and `reap` have no help screen | `internal/cli` (`cmd_launch.go`) |
| L11 (N-26) | in `launch.Parse` a value flag whose next word starts with `--`, or that is last, is `--<flag> needs a value`; a value that starts with a single `-` is taken | `internal/launch` (`args.go`) |
| L12 | `orca.Client.WaitIdle` returns `orca.Wait{Idle, Blocked}` read from the JSON of `terminal wait` (`wait.satisfied`, `wait.blockedReason`), never from the exit status, because Orca exits 1 for a blocked wait with `ok: true`. `awaitReady` in `internal/launch` sends a wait blocked by `agent-trust-workspace` to `answerTrust`, then waits for idle once more; any other reason fails naming it | `internal/orca` (`client.go`), `internal/launch` (`launch.go`) |
| B1 (N-27) | the pure `countWords(text)` of `internal/brief`: the text is split on newlines, a line that starts with one to six `#` not followed by another `#` adds nothing, and every other whitespace-separated token that holds a letter is one word. `Check` uses it for `length` only; `language` keeps the letter runs of the whole lowered text | `internal/brief` (`check.go`) |
| R1 (N-28) | `closeout.Run` reads the footer into memory, closes the terminal, lists the terminals and only then calls `record`, which creates the launch folder, writes `footer.txt` and appends the `end` event. A failure before `record` writes nothing | `internal/closeout` |
| R3 (N-28) | `reap.endGone` skips a launch only when it has a string handle that is still listed; every other launch without an end event is ended as source `reap`, outcome `none` | `internal/reap` |
| R4 (N-28) | `reap.auditWorkers` keys a worker by `handle:<h>` (or by its evidence text when it has no handle) and returns the evidence text of each worker it lists; the phase is output only (`docs/decisions.md`, the v0.1.0 cut): `reap.Run` prints one `unlogged worker: <evidence>` line per worker before the summary, writes no event and remembers nothing between reaps | `internal/reap` |

### S1 `route_scope`

One function, `card.ScopeLine(ids)`, returns `ROUTE-SCOPE: <ids joined by ,>` (or `ROUTE-SCOPE: none`). The card prints it as a line of its own, so it starts a line as the router's regex requires; the `| ROUTE-SCOPE ...` tail of the subagents line goes away. In direct mode the first prompt becomes `Read <brief> and follow it exactly.` followed by a newline and the same scope line. In fleet mode the brief file already starts with the card, so the scope reaches the session through the brief. Lives in `internal/card`; `internal/launch` builds the first prompt from the same function.

Consequence, DECIDED: the card grows from five lines to six, in this order: header, model line, subagents line, `ROUTE-SCOPE: <ids>`, launch line, budget line (without its budget segment when the build has no budget). The card goldens (`card/build/*`, `launch/direct/files-written`) pin that card (defect S1). A five-line card that satisfies both the old goldens and the router regex exists only through an artificial `. ROUTE-SCOPE:` inside another line, which is rejected here.

### S2 `agents_md`

The adapter's `Spec` returns an environment list next to the argv. For `agents_md: exclude` the Claude adapter adds `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1`. `shell.RunScript(env, argv)` writes one `export NAME='value'` line per entry between the shebang and the `exec` line; names must match `^[A-Z_][A-Z0-9_]*$` and values are quoted like every other word. The `env` program is not used, so the `claude` argv stays as it is. The `run.sh` pins (`^#!/bin/sh\n`, `\nexec `) still hold. Lives in `internal/harness/claude` and `internal/shell`.

The variable's documented effect is on CLAUDE.md memory files; a headless canary test measured that it also stops Claude Code's native read of `AGENTS.md` and the `@AGENTS.md` import (`docs/decisions.md`, S2). The case pins only that the variable is in the environment.

### Tiers and subagents (S3, N-5, N-6, N-11)

DECIDED. A subagent is a build with `tier: subagent`. Conductor, Worker and Subagent are the three tiers of one build: the field `tier` takes `conductor`, `worker` or `subagent` and is `worker` when absent. The contract does not tell conductor from worker (both are launchable); `role` keeps printing on the card as today. This replaces the earlier proposal of a `jitso/subagents/<name>.md` file format. The field matrix of each tier is `docs/build-schema.md`; the validators in `internal/catalog` are its source of truth.

- A worker build lists its subagents by name in `subagents: [plan, code, verifier]`. Each name resolves, with the same lookup as `--build`, to a build with `tier: subagent`. A name that is missing, or that names a build of another tier, is an invalid catalog entry (N-6). The whole-catalog rule holds for the repo layer: an invalid file of the repo is exit 3 for every command, with one stderr line naming the file and the subagent. A draft file that is invalid for the launching repo is skipped instead (a warning in `build list`, a rejection at `launch`, silence elsewhere), so a draft in progress never breaks every command.
- A subagent build is an ordinary build: the usual allowlists apply to its `model`, `effort` and `tools`, and it adds `prompt`, the system prompt of the subagent, which must not be empty. `description`, `prompt`, `model`, `effort` and `tools` are what the build supplies. The launch-only fields (`route_scope`, `budget`, `mode_default`, `agents_md`, `harness`) are optional and unused in this tier, and `skills_first`, `mcp`, `requires` and `subagents` must be empty in it, because a subagent runs inside its worker's session and could not honor them (N-11).
- A worker may launch only with approved subagents: a listed subagent that is `candidate` or `retired` is rejected at launch (exit 2), where the text is used, not at catalog load (N-11).
- JITSO passes the resolved builds to `claude` as `--agents <json>`: an object with exactly the listed names as keys. The cases pin `description`, `prompt`, `model` and `tools` of each entry, byte for byte as the build writes them (S3). `effort` is part of what the build supplies, and nothing in the cases forbids emitting it.
- Only those validated fields reach the JSON. Claude Code's `--agents` definition also accepts `mcpServers`, `hooks` and `permissionMode`; a pass-through of the whole build would let one add MCP servers that `--strict-mcp-config` does not strip, or run hooks.
- A subagent-tier build is not launchable alone: `launch --build plan` is rejected by validation (exit 2, `JITSO REJECTED:` naming the build and the tier) and `build list` does not show it (N-5).
- The adapter emits `--agents` right after the `--session-id` value, so a fleet argv of a build with subagents ends with the `--agents` JSON (pinned by `argv/sonnet-automator-fleet`). There is no system-prompt flag: the rules travel in `brief.md`.

The default fixture has `sonnet-automator` with `subagents: [plan, code, verifier]`, a `sonnet-conductor` that declares `tier: conductor`, and the three subagent-tier builds `jitso/builds/plan.yaml`, `code.yaml` and `verifier.yaml`. Lives in `internal/catalog` (tiers and name resolution), `internal/profile` (the refusal of a subagent-tier build) and `internal/harness/claude` (the JSON).

### Build layers, drafts and adoption (N-10 to N-19)

Rows in `docs/decisions.md`; the shape of each, for a reader of the code:

- Vocabulary (N-10). The word is `build` everywhere a user sees it: the flag `--build`, the folder `jitso/builds/`, the command group `jitso build <subcommand>`, every message and the card header. There is no alias: `--preset`, `presets` and `jitso/presets/` are unknown. The registry log was the one survivor (`v: 1` with the keys `preset` and `preset_version`, a storage schema) until N-31 moved it to `v: 2` with `build` and `build_version`; a v1 line is an invalid line.
- Layers (N-12, narrowed by the v0.1.0 cut). The lookup order is the repo's `jitso/builds/`, then the drafts; builds are read only from the project. The one file read from outside is the user allowlist `~/.jitso/jitso.yaml`, and only to resolve `user:<name>` items (`docs/build-schema.md`). The nearest file of an id is the build, every file is validated against the launching repo, and the registry line names the layer (`build_layer`, `build_file`, `build_sha256`) and the card names a draft. Code: `internal/catalog`.
- Validation report (N-13). `jitso build validate` scans every file tolerantly and reports every problem; an invalid repo file is a finding there (exit 2), not the exit 3 of the other commands. Code: `internal/catalog` (`scan.go`).
- Templates (N-14). `internal/templates` is embedded; `jitso build new` writes a draft from a template and, where `jitso/jitso.yaml` is missing, its skeleton.
- Drafts (N-15). `.jitso/drafts/<id>.yaml` in the primary, a last layer that never shadows a saved build; `build draft`, `build discard` and `build save` fill, empty and promote it. `build save` is the one way a build becomes approved and needs the owner: a terminal answer, or `--yes` (the rule of the adoption commands, `adopt.Confirm`). Code: `internal/drafts`.
- Adoption (N-16 to N-19). `install skill`, `setup hook` and `setup instructions` share the flow of `internal/adopt`: preview, `--dry-run`, `--yes` or a terminal answer, a backup of every file changed under `.jitso/backups/`, and `--remove` as the exact undo. `setup hook` registers only `jitso hook guard`, in JITSO's own three groups of `.claude/settings.json` and `.codex/hooks.json` and in the generated OpenCode and Pi files (section 6); `setup instructions` keeps one marked block in the `AGENTS.md` files and never writes a `CLAUDE.md` (it only takes out a block an older version left there); `install skill` writes the shipped usage skill as real files in `.agents/skills/jitso/` plus the relative link `.claude/skills/jitso`, with a manifest that tells a hand edit from an old version. `jitso init` runs them in one plan with one preview and one question, and also writes the `jitso/jitso.yaml` skeleton and the `.gitignore` line of a project that lacks them. The agent boundary of all of them is the launch guard (section 6).
- Doctor (N-20). `jitso doctor` takes no argument and changes nothing: it prints one line per check, `ok`, `warn` or `fail`, with a stable check id (for example `binary`, `path`, `catalog`, `hooks`, `hooks-user`, `skill-project`, `instructions`), and ends with a summary line. Exit 0 when nothing fails, 3 when a check fails. It is defect N-20 in `docs/decisions.md`, with its cases under `doctor/`.

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
| 2. a YAML-subset parser in this repo | near zero (predicted: it uses packages already linked) | a few hundred lines plus tests (predicted), and a documented subset | builds that projects write by hand (D5, D6) or that a session drafts (D7) in ordinary YAML fail on the first feature outside the subset; the subset must reject, never guess |
| 3. change the format to JSON or TOML | JSON zero; TOML also needs a dependency | migrate the catalog, the fixture and the cases | JSON has no comments, which builds rely on; it changes a format users author |

Decision: option 1, `go.yaml.in/yaml/v3`. Tradeoff: about 0.2 MB and one reviewed dependency buy full YAML for the format every adopting project writes by hand; option 2 keeps the binary stdlib-only but makes this repo own a parser and a subset rule its users will hit.

`internal/catalog/decode.go` is the only file that names the decoder: it walks the YAML nodes into an ordered generic map (a plain Go map randomizes key order, and validation messages follow document order), and validation runs on those values. Two leaf packages, listed in section 3, serve it: `internal/fail` (error kinds and the exit-code mapping, so `launch` need not import `cli`) and `internal/jsconv` (JavaScript `String()` coercion shared by `catalog`, `orca` and `harness/claude`). Decoding into typed structs is avoided because values are coerced with `String(...)` and unknown keys are ignored, so `model: 123` must produce `model "123" is not in the allowlist`, not a type error. Swapping the decoder later is a one-file change.

How the size was measured: stripped probes (`CGO_ENABLED=0`, `-trimpath -ldflags='-s -w'`) built with Go 1.27.4, each linking `encoding/json`, `regexp`, `os/exec`, `crypto/sha256` and `crypto/rand`, one with and one without the YAML module, on darwin/arm64, linux/amd64 and windows/amd64. The numbers are history, not a live value: measure again before a release (`.agents/skills/go-development/references/builds.md`).

## 9. Port plan

Every milestone ended with the runner against the built binary: `<runner> --impl bin --bin <built jitso> --filter '<regex>' --jobs 4`, all `PASS`. The port is done: the milestones below are the order it was built in, each with the artifact that proves it. They are history that still runs, so a reader can re-prove any step with its filter.

| # | Milestone | Packages | Status |
|---|---|---|---|
| 0 | Contract edits, no Go: the changes listed below, each written as a case first | `contract/` only | done: the cases carry the defect ids of `docs/decisions.md` |
| 1 | Skeleton: command table, usage, error kinds, project root | `cmd/jitso`, `cli`, `project` | done: filter 1 |
| 2 | Foundation: catalog load and validation (tiers and subagent names included), `build list`; registry, Orca client, JSON encoder and the harness interface with unit tests only | `catalog`, `registry`, `orca`, `jsonx`, `harness`, `shell` (quoting only) | done: filter 2 |
| 3 | Launch up to the dry run: arguments, field allowlist, profile, brief checks and path guard, card, launch line | `launch` (parse and plan), `profile`, `brief`, `card`, `shell` | done: filter 3 |
| 4 | Real launch: files, `run.sh`, Claude adapter, trust dialog, fleet adoption, registry append, L2 and L3, S1 to S3 | `launch` (execute), `harness/claude`, `trust` | done: filter 4 |
| 5 | Lifecycle: close, reap | `closeout`, `reap` | done: filter 5 |
| 6 | (empty: its piece was cut from v0.1.0, see `docs/decisions.md`) | none | none |
| 7 | Hooks | `hook` | done: filter 7 |
| 8 | Full suite, then cross-compile for the three targets | all | done: the whole suite passes on `--impl bin`; the cross-compile is `scripts/package.sh` |
| 9 | Optional, after green: the owner-decided behavior fixes, one at a time, each a contract change first | as the fix needs | done: the blocks are in "After the port" below |

The `--filter` value that proves each milestone (milestone 8 runs with no filter). The ids are the current ones, after the rename of defect N-10:

```
1  ^(smoke/help|cli/(help-flags|no-arguments|unknown-command|unknown-command-empty-word|presets-is-unknown))
2  ^(build/list|project/(linked-worktree|outside-git)/build-list)
3  ^(validation/|brief/|card/|cli/reject-wiring/|smoke/launch-dry-run|project/subfolder-launch-dry-run|project/outside-git/launch|defects/s1[ab])
4  ^(launch|argv|escape|registry|defects|project)/
5  ^(close|reap)/
6  (no cases)
7  ^(hooks/|smoke/guard)
```

In milestone 3, `card/dry-run/adds-nothing-after-a-real-launch` matches the filter but needs a real launch, so it passes only with milestone 4.

L2, L3 and L8 were already in the contract as defects N-2 to N-4 (`docs/decisions.md`). Milestone 0 held the remaining contract edits the port depended on, each with its defect id:

- S1: the card goldens under `card/build/` and the `line_count` pin in `launch/direct/` follow the six-line card.
- S3, N-5, N-6 (tiers): the default fixture has the three subagent-tier builds; the `defects/s3-*` cases pin `--agents` from them; `validation/tier/` pins the refusal of a subagent-tier build; the `build/list/` cases pin the launchable builds only; `validation/subagents/` pins the catalog error for a bad subagent name.
- N-2: the registry lock case also pins that no launch folder is left behind.

### After the port

Each block after milestone 8 is its own contract change, cases first, run against the built binary, never mixed with another block in one diff. The defect id names the row of `docs/decisions.md`.

| Block | Defect | Packages | Filter that proves it |
|---|---|---|---|
| Rename `preset` to `build` (D6) | N-10 | `cli`, `catalog`, `launch`, `hook` | `^(cli/presets-is-unknown\|validation/old-preset-flag-is-unknown\|build/list/old-presets-folder-is-ignored\|build/usage)`, then the whole suite |
| Tiers: the field matrix and approved subagents | N-11 | `catalog`, `launch` | `^validation/(tier\|subagents)/` |
| Build layers | N-12 | `catalog`, `launch`, `card` | `^layers/` |
| `build validate` | N-13 | `catalog`, `cli` | `^build/validate/` |
| Shipped templates, `build new` | N-14 | `templates`, `launch`, `cli` | `^build/new/` |
| Drafts: `draft`, `discard`, `save`, the save guard | N-15 | `drafts`, `catalog`, `launch`, `tty`, `hook`, `cli` | `^(build/(draft\|discard\|save)/\|build/list/drafts\|launch/draft/\|hooks/guard/deny-build-save)` |
| Adoption: `install skill`, `setup hook`, `setup instructions`, the adoption guard | N-16 to N-19 | `adopt`, `settingsjson`, `instructions`, `skill`, `hook`, `cli` | `^(install/\|setup/\|hooks/guard/(deny-adoption-commands\|allows-read-only-jitso)\|cli/usage/)` |
| `doctor` | N-20 | `doctor`, `cli` | `^(doctor/\|cli/usage/)` |
| `version` | N-21 | `version`, `cli` | `^cli/(version/\|usage/)` |
| Folder check on every launch | N-22 | `trust`, `launch` | `^launch/(trust/\|direct/worktree-is-forwarded\|fleet/worktree-is-forwarded)` |
| A harness started directly is denied | N-24 | `hook` | `^hooks/guard/(deny-direct-harness\|text-in-quotes-or-a-heredoc-is-not-a-command\|live-text-is-still-a-command\|allows-commands-that-only-name-a-harness)` |
| Launch arguments: `.mcp.json` only when needed, exclusion warning, blank script, `launch --help`, `--` values | N-26 | `launch`, `card`, `hook`, `cli` | `^(launch/mcp-json/\|card/(mcp-exclude\|launch-line)/\|validation/\|hooks/guard/blank-jitso-script)` |
| Brief word count | N-27 | `brief` | `^brief/` |
| Close and reap rows: close order, reap without a handle, unlogged workers | N-28 | `closeout`, `reap`, `cli` | `^(close\|reap)/` |
| Registry v2: `v: 2` with `build` keys, v1 lines skipped, `doctor` counts them | N-31 | `registry`, `hook`, `doctor` | `^(registry\|close\|reap\|launch\|layers\|project\|hooks/guard/session-end\|doctor)/`, then the whole suite |
| Pre-release fixes: reap reads the `terminal` key | none (owner decision) | `reap` | `^reap/` |
| Pre-release fixes: the `Output:` hint in the ghost path rejection | none | `brief` | `^brief/` |
| Pre-release fixes: pointer briefs | none | `brief`, `launch` | `^brief/pointer/` |
| Pre-release fixes: every terminal listed, a truncated list refused | none | `orca` | `^(reap\|close)/` |
| Pre-release fixes: the field values reach `brief.md` and the launch event | none | `profile`, `launch`, `registry` | `^(launch/\|validation/field)`, then the whole suite |
| Pre-release fixes: fleet creates the Task from `brief.md` | none | `launch`, `hook`, `cli` | `^(launch/fleet/\|validation/fleet/\|hooks/guard/deny-reason)`, then the whole suite |
| User allowlist, launch package and `arsenal` | none (owner decision) | `catalog`, `launch`, `arsenal`, `settingsjson`, `doctor` | `^(arsenal/\|launch/(package\|user-mcp)/)` |
| One-command setup: `init` | none (owner decision) | `cli`, `adopt`, `hook` | `^(init/\|hooks/guard/deny-adoption-commands)` |
| The guard and `launch` for Codex, OpenCode and Pi | none (owner decision) | `harness/*`, `guardfiles`, `launch`, `hook`, `cli` | `^(argv/(codex\|opencode\|pi)-\|launch/harness/\|setup/hook/harness\|hooks/guard/(deny-direct-harness\|deny-terminal-create-harness\|mcp-create-terminal-harness))` |
| Windows | none (owner decision) | `settingsjson`, `skill`, `doctor`, `harness/claude`, `cli` | `--skip-tag posix` on Windows, as `.github/workflows/ci.yml` runs it; `scripts/test-install.ps1` |
| Release 1 of the communication plan: the session name, `fleet`, the communication lines | none (owner decision) | `harness`, `launch`, `fleet`, `card`, `cli` | `^(fleet/\|launch/name/\|launch/(direct\|fleet)/.*brief-md-ends-with\|hooks/guard/allows-fleet\|cli/usage/)` |
| Release 2 of the communication plan: `link`, synapses and `graph` | none (owner decision) | `link`, `graph`, `registry`, `orca`, `card`, `cli` | `^(link/\|graph/\|hooks/guard/allows-link-and-graph\|launch/fleet/brief-md-ends-with-the-communication\|cli/usage/)` |
| Release files: packaging, install scripts, workflows | none (no case) | `scripts/`, `.github/` | `scripts/test-install.sh` and `scripts/test-install.ps1` (offline), and the steps of `docs/release.md` section 3 |

The pipes inside the table are escaped for Markdown; the filter is the text without the backslashes.

### What could run in parallel

Kept as the record of how the port was split; the port is finished. Milestones 1 and 2 are sequential and done by one session: they create every shared package (`cli`, `project`, `catalog`, `registry`, `orca`, `jsonx`, `harness`) and start `shell` with only its quoting function. After milestone 2 the shared packages are frozen; a change to one goes through the coordinating session, never through a track. Three tracks can then run as separate Sonnet subagents, each in its own packages and its own `cmd_<name>.go`:

| Track | Milestones | Owns | Reads only |
|---|---|---|---|
| A, launch | 3 then 4 | `launch`, `profile`, `brief`, `card`, `shell`, `trust`, `harness/claude`, `cmd_launch.go` | `catalog`, `registry`, `orca`, `project` |
| B, lifecycle | 5 | `closeout`, `reap`, `cmd_close.go`, `cmd_reap.go` | `registry`, `orca`, `catalog` |
| C, hooks | 7 | `hook`, `cmd_hook.go` | `catalog`, `registry`, `project`, `shell` (quoting only) |

Track C needs `shell.Quote` for the `--settings` command, so `shell` starts in milestone 2 with only that function; from then on track A owns `shell`, and track C uses `Quote` alone and gets no new symbol from it during the tracks. A few lifecycle cases (for example `reap/unlogged-worker-after-a-real-fleet-launch`) start with a real launch and pass only once track A lands; track B reports them as pending, not as failures of its own. The package boundaries make this safe because no two tracks own the same file, and the only shared registration point is a file per command.

## 10. Decisions on the open questions

| # | Question | Answer |
|---|---|---|
| 1 | YAML: dependency, own subset parser, or another format (section 8) | DECIDED: `go.yaml.in/yaml/v3`, behind the one file `internal/catalog/decode.go` |
| 2 | S3: where each helper's `description` and `prompt` come from | DECIDED: a subagent is a build with `tier: subagent`; the worker lists it by name, and the build supplies `description`, `prompt`, `model`, `effort` and `tools` (section 7) |
| 3 | S1 makes the card six lines, which changes the card goldens | DECIDED: accept; the goldens changed in milestone 0 under defect S1, with `ROUTE-SCOPE: <ids>` on its own line after the subagents line |
| 4 | (withdrawn: the question belonged to a piece cut from v0.1.0, see `docs/decisions.md`) | none |
| 5 | Does the variable stop Claude Code's native `AGENTS.md` read and the `@AGENTS.md` import? | MEASURED, no decision needed: it stops both (headless canary test, Claude Code 2.1.325). Re-measure on upgrades; user-level and nested instruction files were not covered |
| 6 | Which launcher forms the guard accepts once the boilerplate stops using `bun run jitso` (H2, D9) | DECIDED: the guard keeps its three forms (own path, bare `jitso`, `bun run jitso`); dropping `bun run jitso` after D9 is a contract change |
| 7 | When the `preset` to `build` rename (D6) happens | DONE: after milestone 8, as its own contract change (defect N-10), so the port and the rename were never in the same diff |
| 8 | Windows: `run.sh` needs `/bin/sh`, so `launch` is POSIX only | DECIDED: release 1 builds for Windows with `launch` unsupported there; read-only commands and hooks are expected to work, still "not tested". Later: CI tests Windows and a real launch refuses there (section 6, Platforms) |
