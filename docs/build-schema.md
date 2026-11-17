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

Nothing outside the project is read: not a parent folder, not `$HOME/.jitso/builds/`, not `$HOME/.jitso/config.yaml`. A file at one of those places is ignored without a warning.

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

## `jitso/jitso.yaml`: the allowlists

One top-level mapping. A build may name only what it lists. Each list is read as text; a value that is not a list is an empty list.

| Key | Type | Used by |
|---|---|---|
| `version` | number | the file format; `1` |
| `models` | list of full model ids, never aliases | `model` of a build |
| `efforts` | list of effort names | `effort` of a build |
| `mcp` | list of MCP server names, as in `.mcp.json` | `mcp` of a build |
| `skills` | list of skill folder names under `.agents/skills/` | `skills_first` of a build |
| `tools` | list of harness tool names | `tools` of a build |

A project without this file has no allowlists: every catalog command exits 3 with `jitso: jitso/jitso.yaml is missing: this project has no JITSO allowlists (create it with jitso build new <id>)`. `jitso build new` writes an empty skeleton in a fresh project. Route-scope ids are not in this file: they are the `id:` line of each `.agents/instructions/agent-*.md` file.

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
| `route_scope` | non-empty list of instruction-section ids, or `[none]` | R | U | `route_scope is empty (use [none])`; `route_scope id "<id>" is not an instruction section` |
| `success` | list of mappings, each with a `check` | R | U | `success must list at least one check`; `success check "<c>" is unknown`; `success placeholder {<p>} is not a required field` |
| `budget` | mapping with `tokens` and `usd_list_cap` | O | U | none: a build without a budget validates and launches |
| `harness` | a harness with an adapter (`Adapters` in `internal/launch`) | L | U | validate: `harness "<h>" has no adapter`; launch: `no adapter for harness "<h>"` (exit 2) |
| `skills_first` | list, each in `skills` | O | E | `skill "<s>" is not in the allowlist`; `skills_first must be empty in a subagent-tier build` |
| `mcp` | list, each in `mcp`; a launch reads the project's `.mcp.json` only when it still names a server after `--mcp-exclude` | O | E | `mcp "<m>" is not in the allowlist`; `mcp must be empty in a subagent-tier build` |
| `requires` | list of field names the launch must supply | O | E | `required field "<f>" has no pattern`; `requires must be empty in a subagent-tier build` |
| `subagents` | list of kebab-case names, each a subagent-tier build | O | E | `subagents must be a list of kebab-case names`; `subagent "<s>" is not a subagent-tier build`; `a subagent-tier build cannot list subagents` |
| listed subagent status | each listed subagent is `approved`; a draft worker may also list drafts | L | n/a | launch: `JITSO REJECTED: build "<w>" lists subagent "<s>", which is <status>, not approved`; validate: `subagent "<s>" is <status>, not approved` |
| `version`, `role`, `type`, `work_kind`, `mode_default`, `agents_md` | free text, printed on the card and in the summary; `agents_md: exclude` launches `claude` with `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` | O | U | none |

A subagent-tier build is never launched alone, or listed: a worker or conductor lists it by name in `subagents`, and its `description`, `prompt`, `model` and `tools` reach `claude` as an `--agents` entry. Worker and conductor builds are validated identically; the tier changes how a build is used, not what a valid launch needs.

Value types the rules above refer to:

| Name | Values |
|---|---|
| success checks | `answer_delivered`, `jira_comment_on`, `xray_execution_with_env`, `no_rule_breaks` |
| required fields | `ticket_key` (`^[A-Z][A-Z0-9]*-[0-9]+$`), `env` (`^[a-z][a-z0-9_-]*$`); a field with no pattern cannot be required |
| placeholders | a `{name}` inside a success entry must name a field the build lists in `requires` |
| `budget.tokens`, `budget.usd_list_cap` | numbers: the token ceiling and the list-price cap in US dollars of one launch. The budget is optional. With one, the card's sixth line starts `budget <n>k tokens (100%), list cap US$ <x.xx> | ` and the rules text names the card's budget as the ceiling; without one the card's sixth line is `reports to: <whom> | log .jitso/launches.jsonl`, the rules text leaves that sentence out and the `launch` event of the log has no `budget` key. The budget is a ceiling the session is told about, not enforced by jitso |

Unknown keys are accepted and ignored by the build validator. `jitso launch --field <name>=<value>` is the one place an unknown field name is rejected: `unknown field "<f>" for build "<id>"`.

## Rules text

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
