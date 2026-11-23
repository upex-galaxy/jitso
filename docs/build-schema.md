# Build schema

The schema of a JITSO build and of the files around it: where a build lives, what its fields mean, which fields each tier needs and the exact problem text a validator prints. The validators in `internal/catalog` are the source of truth; this page documents them. Decisions and the options they beat are in `docs/decisions.md`.

There is no machine-readable JSON Schema. What a build may name depends on each project's allowlists and instruction sections, which a static schema cannot express, and a second copy of the rules would drift. `jitso build validate` is the schema check.

A build is one YAML file. The YAML subset the files use is in `docs/architecture.md` section 8: full-line `#` comments, one top-level block mapping, block and flow sequences of plain scalars, one-line flow mappings, plain and single-quoted scalars. Nothing else is promised: no anchors, no block scalars, no inline comments, no tabs.

## Where builds live

Every file is `<folder>/<id>.yaml`. Only `*.yaml` files directly inside a layer folder count, sorted by file name. The primary checkout is the project: the git primary checkout of the working directory. Every layer is computed from its real path.

| Layer | Folder | Shown as | Notes |
|---|---|---|---|
| `repo` | `<primary>/jitso/builds/` | `jitso/builds/<file>` | a missing folder is an empty layer |
| `draft` | `<primary>/.jitso/drafts/` | `.jitso/drafts/<file>` | git-ignored through its own `.gitignore` (`*`); the only layer where `status` is `draft` |

No build is read from outside the project: not a parent folder, not `$HOME/.jitso/builds/`, not `$HOME/.jitso/config.yaml`. A file at one of those places is ignored without a warning. The one user file read is the user allowlist `~/.jitso/jitso.yaml` (below), and only for `user:` names.

### Precedence

1. A file's id is its `id` value when the file decodes to a mapping with a kebab-case string `id`. A file without one has no id, shadows nothing and is reported invalid.
2. For each id the build is the file of the nearest layer, in the order `repo`, `draft`. Inside one layer the first file by name wins.
3. The winner is the build of that id whether or not it is valid in this project. If it is invalid, the id is unusable here and the farther files of the same id are not used.
4. A draft never shadows a saved build. `jitso build draft` and `jitso build new` refuse an id that a saved build already has. When a saved build with the id of a draft appears later, the saved build is the build of that id and the draft stays on disk, shown as shadowed by `jitso build list --drafts`.

### What an invalid file does

| Layer | Effect |
|---|---|
| `repo` | the catalog is validated as a whole: one invalid file is exit 3 for every command that loads it, `build <file> is invalid: <problems>` |
| `draft` | the file is skipped: `jitso build list` warns on stderr, `jitso launch --build <id>` is rejected (exit 2) with the problems, every other command is silent |

Every build is validated against the launching project: its `jitso/jitso.yaml`, its instruction sections and its subagent names, resolved with the same lookup.

### Statuses

| Status | Where | Meaning |
|---|---|---|
| `draft` | `.jitso/drafts/` only | being written; launches like any build, marked `(draft)` |
| `candidate` | `jitso/builds/` | not approved: it does not launch |
| `approved` | `jitso/builds/` | launchable (subagent tier: usable by a worker) |
| `retired` | `jitso/builds/` | kept for history: it does not launch |

`jitso build save <id>` turns a draft into `jitso/builds/<id>.yaml` with the `status` scalar changed from `draft` to `approved` and every other byte kept. Only the owner saves a build, and the draft is removed.

## `jitso/jitso.yaml`: the allowlists and the field patterns

One top-level mapping. A build may name only what it lists. Each list is read as text; a value that is not a list is an empty list.

| Key | Type | Used by |
|---|---|---|
| `version` | number | the file format; `1` |
| `models` | list of full model ids, never aliases | `model` of a build |
| `efforts` | list of effort names | `effort` of a build |
| `mcp` | list of MCP server names, as in `.mcp.json` | `mcp` of a build |
| `skills` | list of skill folder names under `.agents/skills/` | `skills_first` of a build |
| `tools` | list of harness tool names | `tools` of a build |
| `fields` | optional mapping of field name to pattern (see below) | the `--field` values of a launch, which also reach the launch's `brief.md` and the `fields` object of its launch event |

A project without this file has no allowlists: every catalog command exits 3 with `jitso: jitso/jitso.yaml is missing: this project has no JITSO allowlists (create it with jitso build new <id>)`. `jitso build new` writes an empty skeleton in a fresh project. The `fields` key is optional: absent or null means no patterns. Each key is a field name (`^[a-z][a-z_]*$`) and each value a non-empty Go `regexp` (RE2) pattern, which must match the whole value: the loader compiles `^(?:<pattern>)$`. A field a launch supplies is checked against its pattern; a field with no entry accepts any non-empty value with no control character (U+0000 to U+001F, U+007F). A build may require any well-formed field name, with or without an entry, and an entry no build requires is accepted. A problem in the key is reported with all the others, in key order, as `jitso/jitso.yaml is invalid: <problems joined by "; ">` (exit 3, and `fail catalog:` in `jitso doctor`):

| Input | Problem text |
|---|---|
| `fields` is not a mapping | `fields must be a mapping of field name to pattern` |
| a key that is not a field name | `field name "<n>" must be lowercase letters and _` |
| a value that is not a string, or is empty | `field "<n>" pattern must be a non-empty string` |
| a pattern that does not compile | `field "<n>" pattern does not compile: <regexp error text>` |

A launch that gives a bad value is `JITSO REJECTED: field <name> has an invalid value`.

Route-scope ids are not in this file: they are the `id:` line of each `.agents/instructions/agent-*.md` file.

## `~/.jitso/jitso.yaml`: the user allowlist

Same YAML subset as `jitso/jitso.yaml`. A build names an item of this file with the scope `user:` in `skills_first` or `mcp` (`user:humanizer`); a plain name never resolves here, and a `user:` name never resolves in the project file. Keys: `version` (`1`), `skills` (folder names under `~/.claude/skills/`), `mcp` (server names of the top-level `mcpServers` of `~/.claude.json`), `literal_env` (`<server>/<KEY>` entries). Any other key is ignored. A missing file is an empty user layer, silent. An invalid file fails only a command whose builds name a `user:` item, with exit 3 and `jitso: ~/.jitso/jitso.yaml is invalid: <problems>`.

| Input | Problem text |
|---|---|
| `user:<n>` skill not in `skills` | `skill "user:<n>" is not in the user allowlist (~/.jitso/jitso.yaml)` |
| `user:<n>` skill without `~/.claude/skills/<n>/SKILL.md` | `user skill "<n>" has no SKILL.md under ~/.claude/skills/<n>` |
| `user:<n>` skill that is also a project skill (in `skills`, or a folder under `.claude/skills/` or `.agents/skills/`) | `user skill "<n>" has the name of a project skill` |
| `user:<n>` server not in `mcp` | `mcp "user:<n>" is not in the user allowlist (~/.jitso/jitso.yaml)` |
| `user:<n>` server that is also in the project's `mcp` | `user mcp "<n>" has the name of a project mcp server` |
| `user:<n>` server not under `mcpServers` of `~/.claude.json` | `user mcp "<n>" is not defined in ~/.claude.json` |
| `user:<n>` server with a literal `env` or `headers` value whose `<n>/<KEY>` is not in `literal_env` | `user mcp "<n>" has a literal value in env <KEY>: use ${VAR}, or list <n>/<KEY> in literal_env of ~/.jitso/jitso.yaml` (`in headers <KEY>` for a header) |
| `user:<n>` server with an `oauth` key | `user mcp "<n>" uses OAuth, which a launch cannot carry` |
| `cloud:<n>` server | `mcp "cloud:<n>": cloud connectors cannot be given to a launched session yet` |
| any other scope | `skill "<scope>:<n>": unknown scope (use user:)`, `mcp "<scope>:<n>": unknown scope (use user:)` |

A launch copies each `user:` skill into its own folder, `.jitso/launches/<id>/pkg/.claude/skills/<n>/`, and passes that folder to the session, which lists the skill by its plain name; a skill over 5 MiB or 500 files is `JITSO REJECTED: user skill "<n>" is larger than a launch carries`. A `user:` server is written into the launch's `mcp.json` under its plain name, with variable references only (never a literal value unless `literal_env` lists its key). A variable a server refers to that is unset in jitso's environment is one stderr warning that names the variable, never a value. The jitso skill needs no allowlist entry: it travels in the package for a conductor build or a build that names `jitso`, unless the checkout holds its own copy. `--mcp-exclude` takes the scoped name (`user:<n>`). `jitso arsenal` lists what a build may name, with these names as a build writes them.

## The build file

Key: R required, O optional (validated when present), U optional and unused by that tier, E must be empty or absent, L checked at launch and also reported by `jitso build validate`.

| Field | Type and rule | worker / conductor | subagent | Problem text |
|---|---|---|---|---|
| `id` | kebab-case string | R | R | `id must be kebab-case` |
| `status` | `candidate`, `approved`, `retired`; in `.jitso/drafts/` exactly `draft` | R | R | `status must be one of candidate, approved, retired`; in a draft `status must be draft in a draft` |
| `tier` | `worker`, `conductor`, `subagent`; absent means worker | O | R | `tier must be one of worker, conductor, subagent` |
| `model` | in `models` | R | R | `model "<m>" is not in the allowlist` |
| `effort` | in `efforts` | R | R | `effort "<e>" is not in the allowlist` |
| `description` | text; it is the summary a launching session reads | O | R, non-empty | `description must be a non-empty string in a subagent-tier build` |
| `prompt` | text; the subagent's instructions | U | R, non-empty | `prompt must be a non-empty string in a subagent-tier build` |
| `tools` | list, each in `tools`; absent means the harness default (a subagent inherits its parent's) | O | O | `tool "<t>" is not in the allowlist` |
| `route_scope` | list of instruction-section ids, or `[none]`; absent or empty means none (the card prints `ROUTE-SCOPE: none`) | O | U | `route_scope id "<id>" is not an instruction section` |
| `budget` | mapping with `tokens` and `usd_list_cap` | O | U | none: a build without a budget validates and launches |
| `harness` | a harness with an adapter (`Adapters` in `internal/launch`): `claude`, `codex`, `opencode`, `pi`; a field the harness cannot honor is a problem (table below) | L | U | validate: `harness "<h>" has no adapter`; launch: `no adapter for harness "<h>"` (exit 2) |
| `skills_first` | list, each in `skills`, or `user:<name>` in the user allowlist, or `jitso` (JITSO's own skill, no entry needed) | O | E | `skill "<s>" is not in the allowlist`; `skills_first must be empty in a subagent-tier build` |
| `mcp` | list, each in `mcp`, or `user:<name>` in the user allowlist; a launch reads the project's `.mcp.json` only when it still names a server after `--mcp-exclude` | O | E | `mcp "<m>" is not in the allowlist`; `mcp must be empty in a subagent-tier build` |
| `requires` | list of field names the launch must supply; each matches `^[a-z][a-z_]*$` | O | E | `required field "<f>" is not a field name`; `requires must be empty in a subagent-tier build` |
| `subagents` | list of kebab-case names, each a subagent-tier build | O | E | `subagents must be a list of kebab-case names`; `subagent "<s>" is not a subagent-tier build`; `a subagent-tier build cannot list subagents` |
| listed subagent status | each listed subagent is `approved`; a draft worker may also list drafts | L | n/a | launch: `JITSO REJECTED: build "<w>" lists subagent "<s>", which is <status>, not approved`; validate: `subagent "<s>" is <status>, not approved` |
| `version`, `role`, `type`, `work_kind`, `mode_default`, `agents_md` | free text, printed on the card and in the summary; `agents_md: exclude` launches `claude` with `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` | O | U | none |

### What each harness honors

An adapter never drops a field it cannot honor: `jitso build validate` reports it and `jitso launch` rejects it (exit 2, `JITSO REJECTED: <problems joined by "; ">`). The source of truth is `Support()` of each package under `internal/harness/` and `launch.SupportProblems`.

| Field | claude | codex | opencode | pi | Problem text |
|---|---|---|---|---|---|
| `subagents` | `--agents` | no | no | no | `harness "<h>" cannot give a session subagents` |
| `tools` | `--tools=` | no | no | `--tools` / `--no-tools` | `harness "<h>" cannot limit the tools of a session` |
| `agents_md: exclude` | `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` | `-c project_doc_max_bytes=0` | no | `--no-context-files` | `harness "<h>" cannot keep AGENTS.md out of a session` |
| `mcp` (project servers) | strict `--mcp-config` | `-c mcp_servers.<n>=...`, added to the owner's Codex servers (a note after the card says so) | `mcp.servers` of the launch's config folder | no server at all (`--no-mcp`) | pi: `harness "pi" cannot give a session MCP servers` |
| `mcp` `user:<n>` | yes | no | no | no | `harness "<h>" cannot give a session the user mcp server "user:<n>"` |
| `skills_first` `user:<n>` | `--add-dir` package | no | `skills` of the launch's config | one `--skill` per skill | `harness "<h>" cannot load the user skill "user:<n>" from the launch package` |
| jitso skill (conductor) | in the package | from the session's own `.agents/skills` (`jitso install skill`) | in the package | in the package | none |
| effort | `--effort` | `-c model_reasoning_effort=` | the model variant `<model>#<effort>` | `--thinking` | none |

A subagent-tier build is never launched alone, or listed: a worker or conductor lists it by name in `subagents`, and its `description`, `prompt`, `model` and `tools` reach `claude` as an `--agents` entry. Worker and conductor builds are validated identically; the tier changes how a build is used, not what a valid launch needs.

Value types the rules above refer to:

| Name | Values |
|---|---|
| required fields | any name that matches `^[a-z][a-z_]*$`; its value is checked against the pattern in the `fields` key of `jitso/jitso.yaml`, or generically when it has none |
| `budget.tokens`, `budget.usd_list_cap` | numbers: the token ceiling and the list-price cap in US dollars of one launch. The budget is optional. With one, the card's sixth line starts `budget <n>k tokens (100%), list cap US$ <x.xx> | ` and the rules text names the card's budget as the ceiling; without one the card's sixth line is `reports to: <whom> | log .jitso/launches.jsonl`, the rules text leaves that sentence out and the `launch` event of the log has no `budget` key. The budget is a ceiling the session is told about, not enforced by jitso |

Unknown keys are accepted and ignored by the build validator. That includes `success`, a key that builds used to carry as documentation and that no longer exists. `jitso launch --field <name>=<value>` is the one place an unknown field name is rejected: `unknown field "<f>" for build "<id>"`.

## Rules text

The `--field` values of a launch are written for the session. `brief.md` gets a block `## Fields` with one `- <key>: <value>` line per field, sorted by key, after the brief text and before the rules text, so the rules stay last; a launch with no field has no block. The launch event of `.jitso/launches.jsonl` gets a `fields` object of key to value, and has no `fields` key when the launch has none. A value is checked as before, and no field value may hold a control character, so a value stays on its one line: a project pattern does not lift that ban. The contract cases under `launch/fields` pin the block, the event and the ban.

A project has no rules files. Every launch appends JITSO's short rules text to the end of the session's `brief.md`, after one blank line; the text is `card.Rules` in `internal/card`, and the contract case `launch/direct/brief-md-ends-with-the-direct-rules` pins the direct wording (`launch/fleet/brief-md-ends-with-the-fleet-rules` the fleet one).

## Starting a build

`jitso build new <id> [--tier worker|conductor|subagent]` writes a draft, `.jitso/drafts/<id>.yaml`, from the embedded template of the tier. A template is a commented draft: `status: draft`, the tier set, and every value that must come from an allowlist left empty, so the validator names each one until the author fills it in. No template names a real model, skill or server. The template stays out of `jitso/builds/` on purpose: an invalid file there would fail every command.

| Aspect | Rule |
|---|---|
| id | kebab-case, at most 64 characters (`^[a-z][a-z0-9-]{0,63}$`); anything else is `JITSO REJECTED: build id must be kebab-case`. No id is `JITSO REJECTED: build new needs a build id` |
| tier | `worker` (the default), `conductor` or `subagent`; anything else is `JITSO REJECTED: --tier must be worker, conductor or subagent`; `--tier` with no value is `JITSO REJECTED: --tier needs a value` |
| refuses | the id of a saved build (`JITSO REJECTED: build "<id>" already exists (<layer> <path>): a draft needs an id no build has`) and the id of an existing draft (`JITSO REJECTED: draft "<id>" already exists: edit .jitso/drafts/<id>.yaml or discard it`); both are checked before anything is written, and a draft is never overwritten |
| fresh project | with no `jitso/jitso.yaml` and no `*.yaml` in `jitso/builds/`, it first writes the `jitso/jitso.yaml` skeleton (the allowlists, all empty; it never overwrites a file) and prints `wrote jitso/jitso.yaml (empty allowlists: fill them first)`. With builds and no `jitso/jitso.yaml` it exits 3 with the missing-allowlists message of this page and writes nothing |
| stdout | `wrote .jitso/drafts/<id>.yaml (tier <tier>, status draft): fill every empty value, check it with jitso build validate <id>, and ask the owner to run jitso build save <id>` |

The author fills the empty values, checks the build with `jitso build validate <id>`, tries it with `jitso launch --build <id>`, and asks the owner to run `jitso build save <id>`. `jitso build draft <file.yaml|->` keeps an existing build as a draft after the same checks. `jitso build discard <id>` and `jitso build discard --all` delete drafts.

## Drafts

A draft is a build that is not saved yet: the file `<primary>/.jitso/drafts/<id>.yaml`, a layer of its own (the last one) that no session needs the owner's OK to write to and that launches like any build.

| Aspect | Rule |
|---|---|
| folder | `.jitso/drafts/` of the primary checkout, even from a linked worktree. It holds its own `.gitignore` with the single line `*`, created when missing and never rewritten, so a draft is never committed |
| input of `jitso build draft` | a path ending in `.yaml` or `.yml`, or `-` for standard input; at most 64 KiB; anything else is `JITSO REJECTED: a draft is a .yaml file (or - for stdin) of at most 64 KiB`. A file that cannot be read is `JITSO REJECTED: cannot read <path>: <reason>`; a second word or a flag is `JITSO REJECTED: unknown argument "<x>"` |
| checks | the rules of this page for the draft layer (`status` exactly `draft`), the launch rules marked L (a worker needs a harness with an adapter, a listed subagent must exist), and an id of at most 64 characters (`id must be at most 64 characters`). A problem is `JITSO REJECTED: draft <path or stdin> is invalid: <problems joined by "; ">`, and nothing is written |
| id | an id no saved build has: `JITSO REJECTED: build "<id>" already exists (<layer> <path>): a draft needs an id no build has`. An id that already has a draft is replaced |
| written | the input bytes, unchanged, as `.jitso/drafts/<id>.yaml`; stdout is `drafted <id>: .jitso/drafts/<id>.yaml (launch it with --build <id>; only the owner can save it)` |
| listing | `jitso build list` never lists a draft (an invalid one is a warning on stderr). `jitso build list --drafts` prints one line per draft file, by file name: the summary line, ` (draft .jitso/drafts/<file>)`, ` shadowed by <layer> <path>` when a saved build has its id, ` invalid here: <problems>` when it is invalid. |
| launching | `jitso launch --build <id>` launches a draft: the card header ends ` (draft)` and the registry line records `build_layer: draft`, `build_file` and the `build_sha256` of the bytes. A draft is the only file that launches with a status other than `approved`. A draft worker may list draft subagents; a saved worker may list only approved ones |
| saving | `jitso build save <id> [--yes]` writes `jitso/builds/<id>.yaml` and removes the draft; the id must be kebab-case (`JITSO REJECTED: build save needs a draft id`, `JITSO REJECTED: build id must be kebab-case`), and a missing draft is `JITSO REJECTED: no draft "<id>"`. The draft is checked again against the current allowlists and the launch rules (`JITSO REJECTED: draft "<id>" is invalid: <problems>`), its `id` must be its file name (`the id "<x>" must be the file name "<id>"`), and the target must be free: `JITSO REJECTED: jitso/builds/<id>.yaml already exists: save never overwrites a build`, or `build "<id>" already exists (<layer> <path>): a draft needs an id no build has` when a build of another layer has the id. The content is the draft bytes with the top-level `status` scalar `draft` replaced by `approved` and nothing else changed (a quoted value keeps its quotes); a status written with a tag, an anchor, an escape or over several lines is `JITSO REJECTED: draft "<id>" cannot be saved: write its status as the plain word draft, or in quotes`. Stdout shows the content, every line prefixed `+ ` (a control character as `\xNN`). On a terminal it then asks `Save it as jitso/builds/<id>.yaml with status approved? [y/N] ` and only `y` or `yes` (any case) saves; anything else is `JITSO REJECTED: not saved`. `--yes` saves without asking, on a terminal or not. Without a terminal and without `--yes` it refuses with `JITSO REJECTED: saving a build needs the owner: read the file above, then run it again with --yes, or in a terminal to answer the prompt; nothing saved`. Success prints `saved <id>: jitso/builds/<id>.yaml (status approved); the draft is removed`. `jitso hook guard` denies `jitso build save` to a session that has it installed; `--yes` does not stop a session that can write the file itself |
| discarding | `jitso build discard <id>` deletes one draft (`discarded <id>`; no draft is `JITSO REJECTED: no draft "<id>"`; an id that is not kebab-case is `JITSO REJECTED: build id must be kebab-case`); `jitso build discard --all` deletes every `*.yaml` of the folder (`discarded <n> drafts`) and leaves the `.gitignore` and any other file. Drafts never expire |

## Checking builds

`jitso build validate` checks every build file of the project against this project's allowlists and instruction sections, and also reports what only a launch would reject (a harness with no adapter, a listed subagent that is not approved). `jitso build validate <id>` checks the winner of one id and the files it shadows. It lists every problem of every file instead of stopping at the first, and an invalid file of the project is a finding here, not the exit 3 that other commands give. Exit codes: 0 when every checked build is ok, 2 when one is invalid or the id is unknown, 3 when the environment is wrong (no `jitso/jitso.yaml`, outside a git checkout).
