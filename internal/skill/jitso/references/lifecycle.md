# After the launch: close, reap

Everything JITSO knows about a launch is in `.jitso/launches.jsonl`, an append-only log kept in the primary checkout (even when you work in a linked worktree). A launch is the fold of its lines by id, and a later line wins over an earlier one. Nothing is rewritten. The log is local and git-ignored.

| Event line | Written by | Holds |
|---|---|---|
| `launch` | `jitso launch` | build, layer, file and hash, model, effort, flags, brief path and hash (the hash of a pointer brief is the hash of its target card), session id, transcript path, Orca terminal, run and task |
| `end` | `close`, `reap` or the SessionEnd hook (`source` says which) | when the launch ended and how long it ran |

The log is schema v2 (`"v": 2` on every line) and the build id is the key `build` of a launch line. Only `launch` and `end` lines exist. A line an older jitso wrote (`"v": 1`, with `preset`, or an `analyzed` or `rule_break` event) is skipped as invalid: close every running launch with the old version before upgrading, and `jitso doctor` counts the skipped v1 lines.

The launch folder `.jitso/launches/<id>/` keeps what the session was given (`brief.md`, `card.txt`, `mcp.json`, `run.sh`) and, after `close`, `footer.txt`. Read the end of `brief.md` to see the exact rules a worker carried.

## `jitso close <launch id>`

Reads the last screen of the worker's terminal, closes that one terminal and lists terminals to confirm it is gone (a success code describes the call, not the outcome). Only then does it write `footer.txt` and the `end` event. If the close or the list fails, or the terminal is still listed, nothing is written: the launch stays running, the exit is 3, and running `close` again retries (`reap` ends it once the terminal is gone). Real output:

```
jitso: closed L<yyyymmdd>T<hhmmss>-<hex> (terminal term_fake)
tokens: input 84, cache write 271601, cache read 8553436, output 112274 (42 messages, 0 subagent transcripts)
```

The second line comes from the launch's `transcript_path` and every `*.jsonl` under `<transcript without .jsonl>/subagents/`: the last `message.usage` of each message id, summed per field. The end event keeps the same numbers under `tokens` (`input`, `cache_write`, `cache_read`, `output`, `messages`, `subagent_files`). A launch with no transcript, or a missing file, gets `"tokens": null` and no second line; a transcript that cannot be read gets `null` and one warning on stderr. Neither fails the close. The `end` events of the hook and of `reap` have no `tokens` key.

(`term_fake` is the handle of the fake Orca this was measured with; yours is a real handle.) Running it again for an ended launch keeps the first end event, still closes the terminal and prints the same line. An id that is not in the log is `JITSO REJECTED: unknown launch <id>`. The argument must be a full launch id; anything else, including `--help`, is `JITSO REJECTED: usage: jitso close <launch_id>`.

Close a worker when its report is in and you have read it. Do not close a launch you did not start unless the human asks.

## `jitso reap [--run <run>]...`

An audit in two parts:

1. A launch with no end event whose terminal is no longer listed gets an `end` event with source `reap`. So does a launch that has no terminal handle at all (a line written by hand or by another tool; `jitso launch` always records one), with outcome `none`.
2. Every worker of the runs the log knows (plus any `--run` you name) that no launch explains is printed as a line `unlogged worker: worker <handle> in run <run> has no launch event`. A worker is explained by its terminal handle, or by the `[jitso:<id>]` mark in its entry. This is how a worker started outside `jitso launch` gets caught. The lines are output only: nothing is written to the log, so every reap prints them again. A worker with a terminal handle is printed once however many runs it shows up in; a worker with no handle (`(no terminal handle)`) is printed once per run.

The last line is the summary:

```
jitso reap: 0 launch(es) marked ended, 0 unlogged worker(s)
```

Orca has no verb that lists every run, so a run the log never heard of is audited only when you name it with `--run <run>`. Run `reap` at the end of a fleet, after the workers report.

## SessionEnd

When the guard hook is registered (the owner runs `jitso setup hook`), the hook's SessionEnd entry writes the `end` event of the closing session's launch with source `session_end_hook`. A registry problem there is swallowed so a session can always close.

## What to report to a human after a fleet

State, from the log and the reports: which launches ran, which ended and how, what `reap` listed as an unlogged worker, and what is still open. Name what you did not verify.
