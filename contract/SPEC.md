# JITSO contract: case format (SPEC)

Normative for `cases/**/case.json`, the fixtures and the runner. If the runner and this file
disagree, fix one of them in the same change. Draft 3, 2026-11-14 (format gaps G1 to G10 closed after the harvest); the `status` key and the oracle were removed with the v0.1.0 cut.

A case describes a sandbox, a list of steps and what must be observable afterwards. It never
looks inside an implementation: only argv, stdin, environment, exit code, stdout, stderr, files
and the calls the fake `orca` recorded.

## 1. Layout

```
contract/
  fixtures/<name>/        a base project tree (default: fixtures/default)
  cases/<area>/<slug>/
    case.json             the case
    <any other file>      referenced by case.json through {"from": "<relative path>"}
```

`id` in `case.json` is `<area>/<slug>` and must equal the folder path under `cases/`.

## 2. case.json

```json
{
  "id": "launch/dry-run/card-direct",
  "title": "one line, what behavior this pins",
  "source": ["cli/jitso/launch.test.ts: 'dry run prints the card'"],
  "tags": ["posix"],
  "defect": "S1",
  "intent": "optional prose: why the case exists",
  "project": { "base": "default", "files": { "docs/extra.md": "text" } },
  "home": { "files": {} },
  "orca": { "screens": ["..."] },
  "claude": { "exit": 0 },
  "for_each": { "var": "value", "values": ["a", "b"] },
  "steps": []
}
```

| Field | Rule |
|---|---|
| `id`, `title`, `steps` | required; `title` is not empty, `steps` has at least one step |
| `source` | where the behavior was harvested from (file and test name in the original TypeScript repo) or `intended: <note>` for behavior that does not exist yet |
| `tags` | lowercase words (`[a-z0-9-]`). `posix` = needs `/bin/sh` (cases that execute `run.sh`); the runner skips tagged cases with `--skip-tag` |
| `defect`, `intent` | optional history: a defect id of the port (`docs/decisions.md`, "Defect ids (history)") and prose saying why the case exists. Nothing requires them and no status depends on them |
| `project.base` | fixture folder name, default `default`. The runner copies it to a fresh temp dir, runs `git init -q` there, and that dir is the cwd of every step unless a step says otherwise |
| `project.files`, `home.files` | overlay applied after the base and after `git init`, see section 3 |
| `orca` | the fake `orca` scenario, section 5.1 |
| `claude` | the fake `claude` knobs, section 5.2 |
| `for_each` | expands the case once per row, in one of two forms. Single var: `{"var": "<name>", "values": ["<string>", ...]}`. Several vars: `{"vars": ["<name>", ...], "rows": [["<string>", ...], ...]}`, each row holding one string per var. Optional `ids: ["<name>", ...]`, one per row, unique, `[A-Za-z0-9][A-Za-z0-9._-]*`. Every string in the case may use each var and `{{index}}`; the expanded id is `<id>[<ids entry>]`, or `<id>[<index>]` without `ids`. A var is a placeholder name that is not a built-in (section 4) and not `index` |

Any key not listed in this SPEC, at any depth of `case.json`, is a load error (it catches typos). The metadata fields (`id`, `title`, `source`, `tags`, `defect`, `intent`) are never resolved; placeholders live in `project`, `home`, `orca` and `steps`.

## 3. File maps

A file map is `{ "<relative path>": <entry> }`. Paths are relative to the project dir (for `project.files`) or the sandbox HOME (for `home.files`); parent folders are created. An entry is:

| Entry | Meaning |
|---|---|
| `"text"` | file with exactly that content (mode `0644`) |
| `{ "from": "brief.md" }` | content of a file next to `case.json` (use this for multi-line text). That content is resolved like any other string of the case (section 4): write `{{{{` for a literal `{{` in it |
| `{ "content": "text", "mode": "0755" }` | content plus a mode; `mode` (octal string) may also go with `from` |
| `{ "symlink": "<target>" }` | a symlink; the target may use placeholders; no `mode` |
| `null` | delete the file or folder if the base has it |

An object entry holds exactly one of `from`, `content`, `symlink`. A path must be relative and stay inside its folder (no absolute path, no `..` that leaves it). The runner refuses a write whose folder resolves, through a symlink, outside the project dir or HOME: that is a runner error, never a write outside the sandbox.

## 4. Placeholders

Replaced in every string of the case at run time (argv, stdin, env values, file contents, matcher values, paths).

| Name | Value |
|---|---|
| `{{PROJECT}}` | real path of the project dir |
| `{{HOME}}` | the sandbox HOME (real path) |
| `{{BIN}}` | the sandbox folder holding the fakes |
| `{{JITSO}}` | the command line, as one string of words, that a hook or a shell would use to start the implementation under test in this sandbox. Supplied by the adapter (section 9). It exists so a case can write the launcher line a hook must judge, such as `{{JITSO}} launch --build ...`, without naming one implementation. As one element of a `run` argv it is a single word with spaces in it, not a command |
| `{{WORKTREE_<name>}}` | the real path of the linked worktree a `git_worktree` step of the case creates (section 6.5). It is known in every section, also before that step runs: the path is fixed, the folder appears when the step runs |
| `{{<NAME>}}` | any value a previous step captured, section 6 |
| `{{var}}`, `{{index}}` | from `for_each` |

`{{LAUNCH_ID}}` is the conventional capture of the launch id.

Resolve placeholders before anything else. A literal `{{` that must stay literal is written `{{{{`. A closing `}}` never needs an escape, so `{{{{X}}` is the literal text `{{X}}` (the first four braces give `{{`, the rest is plain text).

Grammar and timing:

- A placeholder is `{{NAME}}` with `NAME` matching `[A-Za-z_][A-Za-z0-9_]*`. A `{{` that is neither `{{{{` nor such a placeholder is a load error, so a typo never passes as text. Object keys are resolved too.
- A value is inserted verbatim and once: it is not resolved again and not regex-escaped (a captured value inside a `regex` matcher is regex text).
- A `for_each` value is verbatim except for the built-ins: every `{{PROJECT}}`, `{{HOME}}`, `{{BIN}}`, `{{JITSO}}` and `{{WORKTREE_<name>}}` inside it is replaced before the value is inserted. Nothing else in a value is read: no other placeholder, no `{{{{` escape, no error for a stray `{{` (hostile values stay byte for byte).
- The built-ins are `PROJECT`, `HOME`, `BIN`, `JITSO` and every `WORKTREE_<name>`; the prefix `WORKTREE_` is reserved. They are known in every section and no step can capture them or name a `for_each` var after them.
- A name that is neither a built-in, the `for_each` var, `index`, nor captured by some step of the case is a load error. A capture used before the step that takes it fails at run time.
- `project`, `home` and `orca` are resolved when the sandbox is built, with the built-ins and the `for_each` values only: a capture there is a load error.
- A step's own fields are resolved just before it runs; its `expect` is resolved after its captures, so a step may check what it just captured.

## 5. The sandbox

The runner builds, per case, a temp root with `home/`, `project/`, `bin/`. Every implementation is started with exactly this environment, plus the step's `env`:

| Variable | Value |
|---|---|
| `HOME` | `{{HOME}}` |
| `PATH` | `{{BIN}}` then a minimal system path (`/usr/bin:/bin`), then a sandbox `tools/` folder holding symlinks to exactly the programs the implementation adapter needs to start (none for a `bin` binary). The adapter never adds a program's real folder, which may hold a real `claude` or `orca` |
| `ORCA_CLI_COMMAND` | `{{BIN}}/orca` |
| `JITSO_CONTRACT_SCENARIO` | path of the scenario JSON written from the case `orca` field |
| `JITSO_CONTRACT_ORCA_LOG` | path of the file the fake `orca` appends its calls to |
| `JITSO_CONTRACT_CLAUDE_DUMP` | path of the file the fake `claude` writes |

No other variable of the runner leaks in, except the few an adapter declares to start its implementation (none for `bin`, section 9). In particular the real `orca`, the real `claude` and the real HOME are unreachable. `git init -q` runs with this same environment. A step's `cwd` must lie inside the sandbox root (runner error otherwise).

### 5.1 Fake `orca`

An executable named `orca` in `{{BIN}}`. An implementation calls it as `orca <words...> --json` (a trailing `--json` is stripped before recording). It writes one JSON object to stdout and exits `0` on `ok: true`, `1` on `ok: false`.

Scenario (`orca` field of the case, all keys optional):

| Key | Default | Meaning |
|---|---|---|
| `handle` | `term_fake` | the handle `terminal create` returns |
| `cwd` | `{{PROJECT}}` | the folder reported as `worktreeId: "fake-uuid::<cwd>"`; `null` omits `worktreeId` |
| `screens` | one idle Claude prompt screen: `"❯ \n medium · /effort\n bypass permissions on"` | successive `terminal read` answers; the last repeats |
| `wait_times_out` | `false` | `terminal wait` answers `ok: false` |
| `fail_on` | `[]` | verbs (first two argv words joined by a space) that answer `ok:false`, error `{code:"fake_failure",message:"<verb> failed"}` |
| `errors` | `{}` | `{ "<verb>": {"code": "...", "message": "..."} }`: overrides the error body of a failing verb (also makes that verb fail); each value is an object |
| `listed` | `[<handle>]`, whether or not `terminal create` ran | handles `terminal list` reports, minus every handle a previous `terminal close --terminal <h>` closed |
| `responses` | `{}` | `{ "<verb>": <result object> }`: replaces the `result` of a verb that does not fail |
| `close_keeps_listed` | `false` | `true`: `terminal close` still answers `{}`, but the closed handle stays in `terminal list` (a terminal Orca failed to close) |

Default answers: `terminal create` returns `{terminal:{handle,worktreeId}}`; `terminal wait` returns `{}` (with `wait_times_out`, the `fake_failure` error of `fail_on`); `terminal read` returns `{terminal:{tail:[<screen lines>]}}` (the screen split on `\n`); `terminal list` returns `{terminals:[{handle},...]}`; `terminal close` returns `{}` and removes the handle from `listed` (unless `close_keeps_listed`); every other verb returns `{state:"ready"}`. Precedence: `errors`, then `fail_on` and `wait_times_out`, then `responses`, then the default.

Call log: one JSON line per call, `{"argv":["terminal","create","--title","..."]}` (trailing `--json` removed), failing calls included. The fake keeps no other state: the screen index and the closed handles are recomputed from this log, which accumulates over the whole case.

### 5.2 Fake `claude`

An executable named `claude` in `{{BIN}}`. It writes `{"argv":[...],"cwd":"...","env":{...}}` (full argv without the program name, the cwd as a real path, the whole environment) as JSON to `JITSO_CONTRACT_CLAUDE_DUMP` and exits `0`. It starts nothing. Its output on stdout and stderr is empty. A later run overwrites the dump, and the runner deletes it before every `run` and `exec` step, so a `claude` check sees only what its own step started.

The case field `claude` sets two knobs, both optional, which exclude each other:

| Key | Default | Meaning |
|---|---|---|
| `exit` | `0` | the code (0 to 255) the fake exits with, after it wrote its dump |
| `missing` | `false` | `true`: the runner installs no fake `claude`, so no `claude` is on PATH at all |

The knobs travel in the scenario file (`JITSO_CONTRACT_SCENARIO`, key `claude`), so no extra variable appears in the environment the dump records.

## 6. Steps

`steps` run in order; a failed expectation stops the case (later steps are reported as not run). Every expectation of the failing step is still evaluated and reported.

A step holds exactly one of `run`, `hook`, `exec`, `write`, `git_worktree`, `git_init`, and only the keys of its kind:

| Kind | Keys |
|---|---|
| `run` | `run`, `stdin`, `env`, `cwd`, `capture`, `expect` |
| `hook` | `hook`, `input` (required), `env`, `capture`, `expect` |
| `exec` | `exec`, `launch` (required), `env`, `capture`, `expect` |
| `write` | `write`, `expect` |
| `git_worktree` | `git_worktree`, `expect` |
| `git_init` | `git_init`, `expect` |

`env` is an object of strings, applied over the sandbox environment. `capture` maps a placeholder name (not a built-in, not a `for_each` var) to `{"from": <source>, "regex": "re", "group": n}`; `group` defaults to `1` when the regex has a group, else `0`. The source, read after the step ran, is one of:

| `from` | Text the regex runs on |
|---|---|
| `stdout`, `stderr` | the step's output |
| `file` | the content of `path` (required with this source, only there): relative to the project dir, or an absolute placeholder path |
| `orca_log` | the raw fake `orca` call log (JSON lines), for example to take a failed launch's id from the `--title [jitso:<id>]` of its recorded `terminal create` |

A missing file or log is empty text. A capture whose regex does not match is a failure of its step; the step's other expectations are still evaluated.

### 6.1 `run`: the CLI

```json
{ "run": ["launch", "--build", "haiku-explorer"], "stdin": "", "env": {}, "cwd": ".",
  "capture": { "LAUNCH_ID": { "from": "stdout", "regex": "JITSO (L\\d{8}T\\d{6}-[0-9a-f]{4})", "group": 1 } },
  "expect": { } }
```

The runner executes the implementation with those arguments (an empty `run` array = no arguments). `cwd` is relative to the project dir (`"."`, default), or `"{{HOME}}"`, or an absolute placeholder path. `stdin` defaults to empty (closed). Timeout 60 s.

### 6.2 `hook`: a hook script

```json
{ "hook": "guard", "input": {"hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_input": {"command": "ls"}},
  "env": {}, "expect": { "exit": 0, "decision": "none" } }
```

`hook` is `guard` or `dispatch-guard`. `input` is a JSON value (serialised to stdin) or a string (sent raw). The hook runs with `CLAUDE_PROJECT_DIR={{PROJECT}}` and the cwd `{{PROJECT}}`. The implementation adapter maps the name to a command (section 9).

Hook output convention: the hook prints nothing and exits `0` to allow, or prints one JSON object `{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"..."}}`. The runner exposes it as `decision` (`allow`, `deny`, `ask` or `none` when stdout is empty or only whitespace) and `reason`. Stdout that is not one JSON object, or has no `permissionDecision`, matches no `decision` value.

### 6.3 `exec`: run a generated launch script

```json
{ "exec": "run_sh", "launch": "{{LAUNCH_ID}}", "env": {}, "expect": { "claude": { } } }
```

Runs `/bin/sh <project>/.jitso/launches/<launch>/run.sh` with the cwd `{{PROJECT}}` and the sandbox environment, so the fake `claude` records what the script would have started. `exec` is always `"run_sh"`; `launch` is one path segment (a launch id). Tag the case `posix`.

### 6.4 `write`: change the sandbox between steps

```json
{ "write": { "project": { "docs/late.md": "text" }, "home": {} } }
```

Takes the same file maps as section 3. Used to set up state a later step reads (for example a registry line before `close`).

### 6.5 `git_worktree`: a linked worktree of the project

```json
{ "git_worktree": { "name": "linked" }, "expect": { "files": [ { "path": "{{WORKTREE_linked}}/.git" } ] } }
```

Adds a real linked git worktree to the project, so a case pins what happens from inside one (state goes to the primary checkout's `.jitso/`, the trust of a linked worktree) instead of faking `.git` internals. `name` is a literal `[A-Za-z0-9_]+`, unique in the case; the worktree lives at `{{WORKTREE_<name>}}`, a real path inside the sandbox root, on a new branch `contract-<name>`.

When the project has no commit yet, the runner first makes one empty commit (no file is added, so the worktree holds only its `.git` file). Every git command of this step runs with the sandbox environment plus a runner-controlled author and committer (`jitso-contract <contract@example.invalid>`), `GIT_CONFIG_NOSYSTEM=1`, `GIT_CONFIG_GLOBAL=/dev/null`, `commit.gpgsign=false` and `core.hooksPath=/dev/null`: no signing, no hook, no owner config. A git failure is a runner error.

### 6.6 `git_init`: a nested git checkout

```json
{ "git_init": { "path": "nested" }, "expect": { "files": [ { "path": "nested/.git" } ] } }
```

Creates a second, independent git repository inside the sandbox, so a case pins what happens from inside a nested checkout (which checkout is the project, which catalogs are in scope) instead of faking `.git`. `path` is a folder relative to the project dir, or an absolute placeholder path (for example `{{HOME}}/repo`). The runner creates the folder and its parents and runs `git init -q` there, with the sandbox environment and the same controlled git settings as section 6.5: the runner-controlled author and committer, `GIT_CONFIG_NOSYSTEM=1`, `GIT_CONFIG_GLOBAL=/dev/null`, `commit.gpgsign=false` and `core.hooksPath=/dev/null`. The step makes no commit and adds no file: files for the nested checkout come from the case's `project.files` (they are written before the first step) or from an earlier `write` step.

These are runner errors, never expectation failures: a path that already holds a `.git` (the project dir itself included), a path that lies outside the project dir and HOME once its symlinks are resolved (a symlinked parent that leads out counts), and a git failure. A relative `path` that leaves the project dir by its text (`..`) is a load error, like any other bad shape of the step (a missing or empty `path`, an unknown key).

Only the expectations `files`, `registry`, `orca_calls` and `elapsed_ms_lt` apply; any other key is a load error (section 7).

## 7. Expectations

All keys optional; every given key must hold. `expect` can be placed on any step. A key used on a step kind its `On` column does not list is a load error.

| Key | On | Meaning |
|---|---|---|
| `exit` | run, hook, exec | exact exit code |
| `exit_not` | run, hook, exec | the exit code is not this one |
| `exit_in` | run, hook, exec | the exit code is one of this non-empty list |
| `stdout`, `stderr` | run, hook, exec | list of text matchers, section 7.1 |
| `files` | any | list of file checks, section 7.2 |
| `registry` | any | checks on `.jitso/launches.jsonl`, section 7.3 |
| `orca_calls` | any | checks on the fake `orca` call log, section 7.4 |
| `claude` | run, exec | checks on the fake `claude` dump, section 7.5 |
| `decision`, `reason` | hook | `decision` exact; `reason` a list of text matchers |
| `elapsed_ms_lt` | any | the step took less than that |

A process that hit the 60 s timeout satisfies no `exit`, `exit_not` or `exit_in`.

### 7.1 Text matchers

One of: `{"contains":"s"}`, `{"not_contains":"s"}`, `{"equals":"s"}`, `{"regex":"re"}`, `{"not_regex":"re"}`, `{"empty":true}`, `{"line_count":n}`, `{"route_scope":["id", "id"]}`.

`regex` is RE2 syntax (the common subset of Go regexp and JavaScript; no lookaround, no backreferences), matched with the multiline flag off (`^` and `$` match the whole text) unless the pattern starts with `(?m)`.

`route_scope` reads the text exactly like the router does: it collects every match of the regex `(?:^|[.!?])[ \t>*-]*ROUTE-SCOPE:([^\n]*)` (case-insensitive, multiline), splits the captures on `,`, trims each part and strips trailing `.` or `;`, and drops `none`; the resulting list of ids must equal the given list, in order. `{"route_scope": []}` means a `ROUTE-SCOPE:` line exists and names only `none` or nothing.

Details the router fixes (`personality-reinject.mjs`, `scopeOf`): the captures are joined with `,` before the split; a part is trimmed first and then loses every trailing `.`/`;` (it is not trimmed again); empty parts are dropped; `none` is dropped case-insensitively; ids are not de-duplicated. A text with no match at all fails every `route_scope`, including `[]`: that is how a missing colon shows.

A matcher object has exactly one key. A plain string in a matcher list is shorthand for `{"contains": "<string>"}`, except where this SPEC says an element is an argument pattern: there a literal string is an exact match (sections 7.4 and 7.5). `{"empty": false}` requires non-empty text. `line_count` counts `\n`-separated lines, one trailing newline not opening a new line (`""` is 0, `"a\nb\n"` is 2).

### 7.2 File checks

```json
{ "path": ".jitso/launches/{{LAUNCH_ID}}/run.sh", "exists": true, "executable": true,
  "contains": ["exec claude"], "not_contains": ["secret"], "regex": ["..."], "equals": "...",
  "text": [ { "route_scope": ["code-quickref"] }, { "line_count": 5 } ],
  "json": [ { "path": "mcpServers.dbhub.command", "equals": "fixture-dbhub" }, { "path": "mcpServers.dbhub.env", "absent": true },
            { "path": "", "keys_exact": ["mcpServers"] } ] }
{ "path": ".jitso/launches", "entries": { "count": 1 } }
{ "path": ".jitso/launches/{{LAUNCH_ID}}", "entries": [ { "equals": "brief.md\ncard.txt\ncore.md\nmcp.json\nrun.sh" } ] }
```

`path` is relative to the project dir, or starts with `{{HOME}}`/`{{PROJECT}}`. `exists` defaults to `true` (a dangling symlink exists); `exists: false` takes no other check. `executable` checks the owner execute bit (POSIX). `contains`, `not_contains` and `regex` are lists of strings; `equals` is one string, the whole content. `text` is a list of text matchers (section 7.1) on the whole content, so `route_scope`, `line_count` and the rest work on a file. `json` paths are dot paths (`a.b.0.c`, a numeric segment indexes a list; the empty path `""` is the document root); each entry holds `path` and exactly one of:

| Key | The value at `path` |
|---|---|
| `equals` | equals this JSON value, numbers by value (`0.5` equals `0.50`) |
| `regex` | matches, tested against its string form (a string as is, anything else as compact JSON) |
| `absent: true` | is missing |
| `keys_exact: [..]` | is an object whose key set is exactly these names (order free, no name twice) |
| `keys_include: [..]` | is an object that has at least these keys |

`entries` checks a directory instead of a file's content: `{"count": n}` pins the number of entries, a list of text matchers runs on the entry names, sorted by name and joined with `\n`. It takes no content check (`contains`, `regex`, `equals`, `text`, `json`) and needs `exists: true`.

### 7.3 Registry checks

```json
{ "count": 1,
  "events": [ { "at": 0, "event": "launch", "fields": { "mode": "direct", "orca.terminal": "term_fake", "id": {"regex": "^L\\d{8}T\\d{6}-[0-9a-f]{4}$"} } } ] }
```

Reads `.jitso/launches.jsonl` (one JSON event per line). A missing file is zero lines; blank lines (empty or only whitespace) are not counted. `count` is the number of the other lines, whether or not they are valid JSON, so a case can pin a corrupt line that a reader must skip. Only the line an `events[].at` check names is parsed; a line that is not JSON fails that check alone. `at` is the zero-based line; `event` compares that line's `event` field. `fields` maps dot paths to an expected JSON value, or to `{"regex": "re"}` (tested against the string form), or to `{"absent": true}`. An object with the single key `regex` or `absent` is always read as a matcher, never as an expected value.

### 7.4 Orca call checks

```json
{ "verbs": ["terminal create", "terminal wait", "terminal read", "terminal close"],
  "calls": [ { "at": 0, "argv": ["terminal", "create", "--title", {"regex": "^\\[jitso:L"}, "--command", {"regex": "run\\.sh'$"}] } ],
  "none": ["terminal send"] }
```

`verbs` is the exact ordered list of the first two argv words of every call. `calls[].argv` is compared to the whole argv of call `at`, each element an argument pattern: a literal string (exact match) or a text matcher object of section 7.1 (`{"regex": ...}`, anchor it yourself, `{"contains": ...}` and the rest). `none` lists verbs that must not appear. `{"count": n}` pins the number of calls; `{"empty": true}` means the fake was never called. The log covers the whole case so far, not only the current step; a missing log is no calls.

### 7.5 Claude dump checks

```json
{ "ran": true, "argv": ["--dangerously-skip-permissions", "--model", "claude-sonnet-5-5"], "argv_contains": ["--agents"],
  "argv_not_contains": ["--evil"], "last_arg": [{"regex": "ROUTE-SCOPE:"}], "cwd": "{{HOME}}/.jitso/dispatch-cwd",
  "env": { "CLAUDE_CODE_DISABLE_CLAUDE_MDS": "1", "JITSO_X": {"absent": true} },
  "arg_after": { "--model": "claude-haiku-5-5", "--session-id": { "regex": "^[0-9a-f]{8}-[0-9a-f]{4}-" } },
  "argv_count": { "--dangerously-skip-permissions": 1 },
  "json_arg_after": { "--agents": [ { "path": "plan.description", "regex": ".+" } ] } }
```

`ran: false` means the dump file does not exist, and takes no other check; any other check requires that the fake ran in this step (section 5.2). `argv` compares the whole list element by element, each element an argument pattern (a literal string is exact, an object is a text matcher of section 7.1). `argv_contains` and `argv_not_contains` test whole elements (an element equal to the string), not substrings. `last_arg` is a list of text matchers on the last element. `argv_count` maps an element to the exact number of times it occurs as a whole element (`0` forbids it). `arg_after` maps a flag to an argument pattern that the argument after its first occurrence must match. `json_arg_after` parses the argument that follows a flag as JSON and applies the `json` checks of section 7.2 to it. `env` values: a string (exact), `{"absent": true}`, or `{"regex": "re"}`.

## 8. Reporting

Per case (per expansion) one line: `PASS`, `FAIL` or `SKIP`. The process exits `0` only when there is no `FAIL`. A `FAIL` prints the step number, the expectation that broke, the expected and the actual value; `-v` also prints argv, stdout, stderr and the sandbox path.

A runner error (the sandbox could not be built, a step could not start, a placeholder had no value at run time) is a `FAIL` with the error printed. A case that fails to load, including one that still has a `status` key, is printed as `LOAD` with the reason; the cases that did load still run, and the process exits `2`, which wins over `1`. Exit codes: `0` green, `1` a `FAIL`, `2` a load error or a usage error. The summary line is `bin: <n> cases, <n> PASS, <n> FAIL, <n> SKIP, <n> LOAD ERROR in <t>`.

## 9. Implementation

| Flag | Meaning |
|---|---|
| `--impl bin --bin <path>` | the `jitso` executable; steps run `<path> <argv...>` with the project dir as cwd. `{{JITSO}}` is the absolute path given in `--bin`. `--impl` has no other value |

Hooks run `<bin> hook <hook>` (stdin and stdout as in 6.2). Changing that subcommand name is a one-line edit in the runner adapter, not in the cases.

Cases cite only paths their own `project` provides (the default fixture has `docs/guide.md`, `src/sync.ts`, `api/openapi.json`), never a file outside the sandbox.

## 10. Defects and intent (optional history)

`defect` and `intent` are optional metadata. A `defect` id names the row of `docs/decisions.md` ("Defect ids (history)") that explains why the case exists; an `intent` says in prose what behavior the case pins and why. A case that describes behavior a change introduces is written first, run against the built binary to see it `FAIL`, and then implemented (`AGENTS.md`, rule 3).
