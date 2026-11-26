# The launch card

The card is six lines. The launcher prints it, saves it as `.jitso/launches/<id>/card.txt` and puts it at the top of the worker's `brief.md`. A worker reads its own card to learn its role, its limits and who it reports to. A launcher reads it to check what it just started.

Example from a test fixture project (your build, model and numbers differ):

```
JITSO L<yyyymmdd>T<hhmmss>-<hex> | operator/automator | mode direct (asks you) | build sonnet-automator v1
model claude-sonnet-5-5 effort medium | bypass | MCP dbhub,openapi | skill test-automation
subagents plan,code,verifier (suggested; others allowed)
ROUTE-SCOPE: code-quickref,ticket-work
launch: jitso launch
budget 400k tokens (100%), list cap US$ 6.00 | reports to: you | log .jitso/launches.jsonl
```

A launch into another repository has one more line right after the header, `target <path> | home <path>`: the worker runs in the first, and the builds, briefs and registry are in the second.

A build without a `budget` prints the sixth line without its first segment: `reports to: you | log .jitso/launches.jsonl`.

## Line by line

| Line | Fields | What it limits or tells |
|---|---|---|
| 1 header | launch id, `role/type`, mode and who it asks, build id and version | The id is what `close` and the registry use. `mode direct (asks you)` means the worker may ask its human. `mode fleet (asks your conductor)` means it never does. A draft ends with `(draft)`: it is not saved. A role, type or version the build does not give is left out. |
| 2 model | model, effort, `bypass`, MCP servers, skills | `bypass` means the session runs with permission prompts skipped, so what holds it is the build, the rules at the end of `brief.md` and the guard (which blocks launches only). `MCP` lists the only servers the session has (others are stripped). `skill` is what it must load before any other work (`none` when empty). |
| 3 subagents | helper names, or `unrestricted (harness default)` | A listed set is a suggestion: `(suggested; others allowed)`. The names are the builds the worker was given as `--agents`. |
| 4 `ROUTE-SCOPE:` | instruction-section ids, or `none` (also what a build with no `route_scope` prints) | The sections of the project's `.agents/instructions/` the session is routed to. It starts its own line so the instruction router can read it. Work outside this scope is outside the brief. |
| 5 `launch:` | the command to start another session | `jitso launch`, or `bun run jitso launch` in a project whose `package.json` has a `jitso` script that is not blank. It is bare: a conductor adds `--mode fleet --run <run>` itself, and JITSO creates the Task from the brief. Only this command may start a session. |
| 6 budget | token ceiling, list-price cap (only when the build has a budget), who the report goes to, the registry path | When there is a budget it is the ceiling: near it, stop expanding and report. `(100%)` is the ceiling itself, not progress. The cap is a list-price estimate, not what was billed. `reports to: you` is direct mode, `your conductor` is fleet mode (read as "the launcher"). |

## Reading a card back to a human

Say what the numbers mean, not just what they are. For the example: this is a direct launch of the automator build, it may ask the human, it runs with permissions bypassed and two MCP servers, it must load one skill first, it is routed to two instruction sections, and, if line six has a budget, its ceiling is the token figure there. If a field you expected is `none`, say so: an empty `MCP none` means the session has no MCP servers at all.

## What differs by mode

- Direct: the worker's first prompt is `Read <path to brief.md> and follow it exactly.` followed by the same `ROUTE-SCOPE:` line.
- Fleet: no first prompt. The card at the top of `brief.md` carries the scope and the launcher.

## A dry run adds one line

```
dry run: nothing was written and Orca was not called
```

Into another repository it reads `dry run: nothing was written; Orca was only asked for its repository list`.

The id on a dry run is a preview. A real launch prints the card again with its own id, and that id is the one to keep.

## When a card looks wrong

- Wrong build or layer: `jitso build list` and `jitso build validate <id>` show which file wins an id.
- A missing MCP server or skill: the build does not list it. A launch may only drop servers (`--mcp-exclude`), never add them.
- A model or effort you did not expect: it is the build's. A launch cannot override it. Use another build or compose one.
