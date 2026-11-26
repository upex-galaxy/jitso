# Builds and drafts

A build is one YAML file: the profile a session launches with. JITSO ships no builds of its own. A project saves the builds it wants to keep, and a session composes the others when it needs them. Each build is validated against the project that launches it. In the jitso repository the field-by-field rules are in `docs/build-schema.md`; in a consumer project `jitso build validate` is the schema check, and it prints each problem by name.

## Compose a build just in time

1. `jitso arsenal` lists what a build may name here, one line per item, as a build writes it:

   ```
   model   claude-sonnet-5-5
   skill   sprint-testing   <first sentence of its description>
   skill   user:humanizer   <first sentence of its description>
   mcp     user:engram      stdio
   ```

   An `(open)` line means nothing restricts that kind: any model id, effort name or tool name; any skill folder of the project; any server of `.mcp.json`. `jitso arsenal --all` adds what exists on this machine but a list leaves out, each marked `not allowlisted (edit <file>)`: a build may not name it until the list does. It prints key names such as `literal env: KEY`, never a value.

2. Keep the build as a draft, from standard input or a `.yaml` file:

   ```
   jitso build draft -
   ```

   `id`, `model` and `effort` are enough. A draft needs no `status`, and with no `harness` it is a `claude` build. Add what the job needs: `skills_first`, `mcp`, `tools`, `subagents`, `requires`, `route_scope`, `budget` (`tokens`, `usd_list_cap`), `description`, `role`, `type`, `version`. Full model ids only, never aliases. The command checks the draft and writes `.jitso/drafts/<id>.yaml` byte for byte, or refuses with `JITSO REJECTED: draft <path or stdin> is invalid: <problems>`. A re-draft of the same id replaces it. An id a saved build has is refused.

3. Launch it: `jitso launch --build <id> --brief ...`, with `--dry-run` first. A draft card ends `(draft)`.
4. Optional: `jitso build save <id>`.

`jitso build new <id> [--tier worker|conductor|subagent]` writes a commented template as a draft instead, with the model and effort left empty (in a project without `jitso/jitso.yaml` it also writes that skeleton). Fill it, then `jitso build validate <id>`.

Real output of a draft, then of the listing:

```
drafted my-scout: .jitso/drafts/my-scout.yaml (launch it with --build my-scout; keep it with jitso build save my-scout)
- my-scout: claude-haiku-5-5 low. Read-only scout that returns paths and line numbers. requires: nothing (draft .jitso/drafts/my-scout.yaml)
```

`jitso build list --drafts` shows the drafts, with `shadowed by <layer> <path>` when a saved build now has the id and `invalid here: <problems>` when a draft no longer validates. `jitso build discard <id>` removes a draft and `jitso build discard --all` removes every draft, so use `--all` only when the human asked.

## Save a build

`jitso build save <id>` checks the draft again, prints the bytes it writes (each line prefixed `+ `), writes `jitso/builds/<id>.yaml` with `status: approved` and removes the draft. It asks nothing and never overwrites. `--yes` is accepted and changes nothing. Save a build the project will use again, and tell the human: the new file is theirs to review and commit.

A saved worker launches only when every subagent it lists is approved, so save its draft subagents too.

## Look before you compose

`jitso build list` prints one line per saved launchable build (worker and conductor tiers, status `approved`): id, model, effort, description and what it `requires`. Parts a build does not give are left out. Example:

```
- haiku-explorer: claude-haiku-5-5 low. Read-only search of the repo that returns file paths and line numbers. requires: nothing
- sonnet-tester: claude-sonnet-5-5 medium. Manual QA of one user story end to end with sprint-testing. requires: ticket_key
```

`jitso build validate` checks every build file and prints one line each, then a summary; with an id it checks the winner of that id and the files it shadows. Exit 0 when all are fine, 2 when one is invalid or the id is unknown.

```
invalid my-scout draft .jitso/drafts/my-scout.yaml
  model "null" is not in the allowlist
checked 1 builds: 0 ok, 1 invalid
```

## Allowlists

`jitso/jitso.yaml` marks the project for the guard, and may restrict builds with the lists `models`, `efforts`, `mcp`, `skills` and `tools`. A key that is absent or `[]` is open; a list with entries restricts every build to them. Without the file everything is open and the guard is off. The optional `fields` key gives a pattern per `--field`; a field without one takes any non-empty value on one line. The lists are the human's restrictions: change them only when the human asks.

## Tiers

| Tier | Launched alone | What it is for |
|---|---|---|
| `worker` (the default) | yes | one job from one brief |
| `conductor` | yes | coordinates workers; validated like a worker |
| `subagent` | never | a helper listed by a worker in `subagents`; it needs a `description` and a `prompt` and leaves `skills_first`, `mcp`, `requires` and `subagents` empty |

A worker's `subagents` reach the session as `--agents` with their `description`, `prompt`, `model` and `tools`. To change what a helper does, change its build.

## Where builds live

| Place | Where | Notes |
|---|---|---|
| `repo` | `jitso/builds/` in the primary checkout | saved builds; one invalid file here fails every catalog command with exit 3 |
| `draft` | `.jitso/drafts/` in the primary checkout | git-ignored; last in the order, and a draft never shadows a saved build |

Builds are read only from the project. Two files with the same id are one build: the first one by file name wins. An invalid draft is skipped: `build list` warns on stderr, and a launch of that id is rejected with the problems.

Statuses: `draft` (a draft, with the key written or left out; launches), `candidate` (does not launch), `approved` (launches), `retired` (history, does not launch). A saved file always carries one.

## User items: `user:<name>`

`skills_first` and `mcp` accept `user:<name>` for a skill under `~/.claude/skills/` or a server of `~/.claude.json`; a plain name resolves only in the project. The lists `skills` and `mcp` of `~/.jitso/jitso.yaml` restrict them the way the project's lists do. A launch copies each user skill into its package folder and writes each user server into its `mcp.json`. `--mcp-exclude user:<name>` drops one for a launch. The jitso skill needs no entry: name `jitso` in `skills_first`.

Refused, with the text in `references/troubleshooting.md`: `cloud:` connectors, a user server that uses OAuth, a literal secret value in a user server's `env`, `headers` or `args` (use `${VAR}`), and a user skill over 5 MiB or 500 files.
