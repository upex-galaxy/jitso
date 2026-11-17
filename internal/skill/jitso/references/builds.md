# Builds and drafts

A build is one YAML file: the approved profile a session launches with. JITSO ships no builds of its own. Each project writes its builds, and each build is validated against that project's allowlists in `jitso/jitso.yaml` (the models, efforts, MCP servers, skills and tools a build may name). In the jitso repository the field-by-field rules are in `docs/build-schema.md`. In a consumer project, `jitso build validate` is the schema check, and it prints each problem by name.

## Look before you launch

`jitso build list` prints one line per launchable build: id, model, effort, description and what it `requires`. Example from a test fixture project:

```
- haiku-explorer: claude-haiku-5-5 low. Read-only search of the repo that returns file paths and line numbers. requires: nothing
- sonnet-tester: claude-sonnet-5-5 medium. Manual QA of one user story end to end with sprint-testing. requires: ticket_key
```

It lists only approved builds of the worker and conductor tiers. Subagent builds, candidates, retired builds and drafts are not shown.

`jitso build validate` checks every build file and prints one line each, then a summary. With an id it checks the winner of that id and the files it shadows. Exit 0 when all are fine, 2 when one is invalid or the id is unknown.

```
ok haiku-explorer repo jitso/builds/haiku-explorer.yaml
invalid my-scout draft .jitso/drafts/my-scout.yaml
  model "null" is not in the allowlist
  effort "null" is not in the allowlist
checked 1 builds: 0 ok, 1 invalid
```

(The two blocks are joined from different real runs, to show both shapes.)

## Tiers

| Tier | Launched alone | What it is for |
|---|---|---|
| `worker` (the default) | yes | one job from one brief |
| `conductor` | yes | coordinates workers; validated like a worker |
| `subagent` | never | a helper listed by a worker in `subagents`; it needs a `description` and a `prompt` and leaves `route_scope`, `skills_first`, `mcp`, `requires` and `subagents` empty |

`jitso launch --build <a subagent>` is `JITSO REJECTED: build "plan" has tier subagent: it is not launched alone, a worker lists it in its subagents`. A worker may launch only when every subagent it lists is approved.

## Where builds live

Builds come from two places only:

| Place | Where | Notes |
|---|---|---|
| `repo` | `jitso/builds/` in the primary checkout | the project's own; one invalid file here fails every catalog command with exit 3 |
| `draft` | `.jitso/drafts/` in the primary checkout | git-ignored; last in the order, and a draft never shadows a saved build |

Nothing outside the project is read: a folder of builds under the user's home or in a parent folder is not a source of builds. Two files with the same id are one build: the first one by file name wins, and the other is never used. An invalid draft file is skipped: `build list` warns on stderr, and `jitso launch --build <that id>` is rejected with the problems. Every build is validated against the project that launches it.

Statuses: `draft` (only in `.jitso/drafts/`, launches like any build), `candidate` (not approved, does not launch), `approved` (launches), `retired` (history, does not launch).

## When no build fits: draft one

A draft is a build that is not saved yet. You may write one, the owner approves it. Do it in this order:

1. Run `jitso build new <id> [--tier worker|conductor|subagent]` once to see the template for the tier (in a fresh project it first writes an empty `jitso/jitso.yaml` skeleton). It writes `.jitso/drafts/<id>.yaml` with every allowlist-bound value empty. A new draft lists as `null null. null` until it is filled.
2. Fill in the draft: edit `.jitso/drafts/<id>.yaml` (JITSO's rules allow editing a draft, and nothing else under `.jitso/`), or write the complete build in another file or on standard input and keep it with `jitso build draft`. Take every model, effort, MCP server, skill and tool from the allowlists in `jitso/jitso.yaml`. Full model ids only, never aliases. A worker needs a `harness`; a `budget` mapping (`tokens`, `usd_list_cap`) is optional.
3. Keep it: `jitso build draft <file.yaml>` or `jitso build draft -`. The command checks it with the draft rules and writes `.jitso/drafts/<id>.yaml` byte for byte, or refuses with `JITSO REJECTED: draft <path or stdin> is invalid: <problems>`. A re-draft of the same id replaces that draft. An id that any saved build already has is refused: pick another.
4. Check it: `jitso build validate <id>`, then `jitso launch --build <id> --brief ... --dry-run` to read the card. A draft card ends `(draft)`.
5. Tell the human what you drafted and why. Say that `jitso build save <id>` is theirs to run. Do not run it, and do not launch a draft the human has not seen unless they asked you to.

Real output of step 3, then of the listing:

```
drafted my-scout: .jitso/drafts/my-scout.yaml (launch it with --build my-scout; only the owner can save it)
- my-scout: claude-haiku-5-5 low. Read-only scout that returns paths and line numbers. requires: nothing (draft .jitso/drafts/my-scout.yaml)
```

`jitso build list --drafts` shows the drafts, with `shadowed by <layer> <path>` when a saved build now has the id and `invalid here: <problems>` when a draft no longer validates. `jitso build discard <id>` removes one of your drafts and `jitso build discard --all` removes every draft, so use `--all` only when the human asked.

## What `build save` does (the owner's command)

It checks the draft again, prints the exact bytes it would write with every line prefixed `+ `, and on a terminal asks `[y/N]` (`--yes` skips the question). Only `y` or `yes` saves it as `jitso/builds/<id>.yaml` with `status: approved` and removes the draft. Without a terminal it refuses unless the owner passes `--yes` after reading the file. The hook denies the command to agent sessions, and JITSO's rules forbid it. If you run it, you have approved your own build, which is the one thing the design exists to prevent.

## Subagents of a worker

A worker's `subagents` list names subagent-tier builds. They reach the session as `--agents` with their `description`, `prompt`, `model` and `tools`. To change what a helper does, change its build. The card line `subagents` shows the names.
