# After the launch: close, reap, analyze

Everything JITSO knows about a launch is in `.jitso/launches.jsonl`, an append-only log kept in the primary checkout (even when you work in a linked worktree). A launch is the fold of its lines by id, and a later line wins over an earlier one. Nothing is rewritten. The log is local and git-ignored.

| Event line | Written by | Holds |
|---|---|---|
| `launch` | `jitso launch` | build, layer, file and hash, model, effort, flags, brief path and hash, session id, transcript path, Orca terminal, run and task |
| `end` | `close`, `reap` or the SessionEnd hook (`source` says which) | when the launch ended and how long it ran |
| `analyzed` | `analyze` | turns, tokens, list cost, peak context, verdict |
| `rule_break` | `reap`, `analyze` | a kind and its evidence |

The log is schema v2 (`"v": 2` on every line) and the build id is the key `build` of a launch line. A line an older jitso wrote (`"v": 1`, with `preset`) is skipped as invalid: close every running launch with the old version before upgrading, and `jitso doctor` counts the skipped v1 lines.

The launch folder `.jitso/launches/<id>/` keeps what the session was given (`brief.md`, `card.txt`, `mcp.json`, `run.sh`) and, after `close`, `footer.txt`. Read the end of `brief.md` to see the exact rules a worker carried.

## `jitso close <launch id>`

Reads the last screen of the worker's terminal, closes that one terminal and lists terminals to confirm it is gone (a success code describes the call, not the outcome). Only then does it write `footer.txt` and the `end` event. If the close or the list fails, or the terminal is still listed, nothing is written: the launch stays running, the exit is 3, and running `close` again retries (`reap` ends it once the terminal is gone). Real output:

```
jitso: closed L<yyyymmdd>T<hhmmss>-<hex> (terminal term_fake)
```

(`term_fake` is the handle of the fake Orca this was measured with; yours is a real handle.) Running it again for an ended launch keeps the first end event, still closes the terminal and prints the same line. An id that is not in the log is `JITSO REJECTED: unknown launch <id>`. The argument must be a full launch id; anything else, including `--help`, is `JITSO REJECTED: usage: jitso close <launch_id>`.

Close a worker when its report is in and you have read it. Do not close a launch you did not start unless the human asks.

## `jitso reap [--run <run>]...`

An audit, in three phases, each reading the log again so a later phase sees what an earlier one wrote:

1. A launch with no end event whose terminal is no longer listed gets an `end` event with source `reap`. So does a launch that has no terminal handle at all (a line written by hand or by another tool; `jitso launch` always records one), with outcome `none`.
2. Every worker of the runs the log knows (plus any `--run` you name) that no launch explains becomes a `rule_break` of kind `unlogged_worker`. A worker is explained by its terminal handle, or by the `[jitso:<id>]` mark in its entry. This is how a worker started outside `jitso launch` gets caught. A worker with a terminal handle is flagged once, however many runs it shows up in and however many times you reap; a worker with no handle is flagged once per run.
3. Ended launches with a transcript on disk and no `analyzed` line are analyzed. A failure there is printed on stderr and does not stop the reap.

Real output:

```
jitso reap: 0 launch(es) marked ended, 0 unlogged worker(s), 0 analyzed
```

The unlogged-worker number of that line is how many `rule_break` lines this reap wrote, like the other two numbers, so a second reap of the same worker prints 0. Orca has no verb that lists every run, so a run the log never heard of is audited only when you name it with `--run <run>`. Run `reap` at the end of a fleet, after the workers report.

## `jitso analyze <launch id>`

Reads the worker's transcript (and its subagents' transcripts) and appends an `analyzed` line: turns, requests, peak context, the share of the token budget used, token counts, list cost, tools used, rule breaks, and a verdict. Real output of the summary line, from a hand-made two-request transcript:

```
jitso: analyzed L<yyyymmdd>T<hhmmss>-<hex>: 2 turns, peak 1100 tokens, US$ 0.001 list, verdict unknown
```

Things to know:

- It needs `jitso/prices.json` in the project, a list-price table. Without it, `analyze` exits 3 with `jitso: open <project>/jitso/prices.json: no such file or directory`. The arguments are checked first, so a bad argument is `JITSO REJECTED: usage: ...` (exit 2) whether or not the file exists. The cost is a list-price estimate for comparing runs, not what was billed.
- The launch must have a transcript on disk, else `JITSO REJECTED: launch <id> has no transcript on disk yet`. A launch whose terminal never started a session has none.
- The verdict is `fail` when a tool call touched a secret path or the launch went over its budget, `success` when every success check of the build is one that can be decided from the transcript (only `no_rule_breaks`), and otherwise `unknown`. `unknown` is the normal verdict: the other checks (a Jira comment, a test run) need the destination, so verify them there.
- A secret-path read is recorded as a `rule_break` of kind `secret_path_read` with a count, never with the path or the command. A write counts too: `Write`, `Edit`, `MultiEdit` and `NotebookEdit` of a secret-looking path are counted like a read (the counter keeps the name `secret_path_reads`; what the call writes is never looked at).
- A launch whose peak context is above its `budget.tokens`, or whose list cost is above its `budget.usd_list_cap`, gets one `rule_break` of kind `budget_over` for each limit (the evidence holds numbers only), its verdict is `fail` with the reason `the launch went over its budget`, and the summary line ends ` (over budget)`, or ` (<n> secret-path read(s), over budget)` with secret reads. The budget is detected afterwards, not enforced while the session runs.
- `jitso analyze --transcript <file.jsonl>` analyzes a transcript file and prints the analysis as JSON without writing anything.

## SessionEnd

When the guard hook is registered (the owner runs `jitso setup hook`), the hook's SessionEnd entry writes the `end` event of the closing session's launch with source `session_end_hook`. A registry problem there is swallowed so a session can always close.

## `jitso tree [<launch id>]`

Prints the launch tree from the log, read only (no write, no Orca call): one line per launch, parents first, children indented two spaces. Each line carries the rollups of that launch's branch, the launch and everything under it: list cost and tokens are summed, peak context and budget percentage are the largest of the nodes, rule breaks are counted, and the verdict is the worst of the analyzed launches (`fail`, then `unknown`, then `success`), or `pending` when none of them is analyzed yet. A launch with no `analyzed` line adds zeros. A launch whose parent is not in the log is a root. With a launch id it prints only that launch's branch. Measured on a hand-made log:

```
L20261113T140000-0001 sonnet-tester running: verdict fail, US$ 7.5 list, 7781 tokens, peak 300, budget 120%, 3 rule break(s), 4 of 4 analyzed
  L20261113T140000-000a sonnet-tester running: verdict unknown, US$ 2.5 list, 2226 tokens, peak 300, budget 120%, 1 rule break(s), 2 of 2 analyzed
    L20261113T140000-000c sonnet-tester running: verdict success, US$ 0.5 list, 4 tokens, peak 80, budget 32%, 0 rule break(s), 1 of 1 analyzed
```

With nothing in the log it prints `jitso tree: no launches`. An id that is not in the log is `JITSO REJECTED: unknown launch <id>`, and more than one argument, or one that is not a launch id, is `JITSO REJECTED: usage: jitso tree [<launch_id>]`. The cost and the numbers come from `analyze`, so run it (or `reap`) first.

## What to report to a human after a fleet

State, from the log and the reports: which launches ran, which ended and how, what `reap` flagged, the verdict and list cost per launch if `analyze` ran, and what is still open. Name what you did not verify.
