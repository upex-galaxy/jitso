# After the launch: close, reap

Everything JITSO knows about a launch is in `.jitso/launches.jsonl`, an append-only log kept in the primary checkout (even when you work in a linked worktree). A launch is the fold of its lines by id, and a later line wins over an earlier one. Nothing is rewritten. The log is local and git-ignored.

| Event line | Written by | Holds |
|---|---|---|
| `launch` | `jitso launch` | build, layer, file and hash, model, effort, flags, package, brief path and hash (the hash of a pointer brief is the hash of its target card), session id, transcript path, Orca terminal, run and task |
| `end` | `close`, `reap` or the SessionEnd hook (`source` says which) | when the launch ended and how long it ran |
| `link` | `jitso link` | `from` and `to`, each a launch id and an Orca handle (`null` when unknown), and when. It belongs to no launch |

The launch event keeps the Orca ids under `orca`: `orca.run`, `orca.task`, `orca.terminal` and `orca.title`. There is no top-level `task`, `terminal` or `dispatch` key, and the dispatch id is not recorded: read it from `orca orchestration worker-list`.

The log is schema v2 (`"v": 2` on every line) and the build id is the key `build` of a launch line. Only `launch`, `end` and `link` lines exist; a jitso from before `link` skips those lines and its `doctor` counts them as invalid. A line an older jitso wrote (`"v": 1`, with `preset`, or an `analyzed` or `rule_break` event) is skipped as invalid: close every running launch with the old version before upgrading, and `jitso doctor` counts the skipped v1 lines.

The launch event also carries `package` when something travelled with the launch: the skills copied in (`name`, `scope`, `sha256` of the copied tree), whether the guard and the jitso skill travelled (`guard`, `jitso_skill`) and how the folder reached the harness (`flag`: `add-dir`, or `none`). The key is absent when nothing travelled.

The launch folder `.jitso/launches/<id>/` keeps what the session was given (`brief.md`, `card.txt`, `mcp.json`, `run.sh`, and `pkg/` when the launch carried a package) and, after `close`, `footer.txt`. Read the end of `brief.md` to see the exact rules a worker carried.

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

## `jitso fleet [--json] [--all]`

Read only: it writes nothing to the log and makes one Orca call, `terminal list`. It joins the launches of this project's log with every terminal Orca lists, to answer who else is running and how to reach them. This project comes first: its launches, then the unlaunched terminals of its checkouts, then those of other folders.

```
ID                     ROLE/TYPE            HARNESS  STATE       HANDLE  PARENT  REPO       TITLE
L20261124T100000-a1a1  conductor/conductor  claude   live        term_a  -       my-app     [jitso:L20261124T100000-a1a1] sonnet-conductor
-                      -                    -        unlaunched  term_x  -       other-app  zsh
jitso fleet: 1 live, 1 unlaunched
```

| State | Meaning |
|---|---|
| `live` | the launch has no end event and its terminal is listed |
| `gone` | no end event, terminal not listed (`jitso reap` ends it) |
| `unknown` | no end event, and Orca could not be asked |
| `closed` | the launch has an end event; shown only with `--all` |
| `unlaunched` | a listed terminal that no launch explains, by handle or by the `[jitso:<id>]` mark in its title |

Orca lists every terminal it manages, shells included, so an `unlaunched` row is a terminal to look at, not proof of an agent session. Your own row is the one whose id is in your card. The title is also the name of a Claude Code or Pi session (they start with `--name`); Codex and OpenCode have no launch flag for a name, so address those by handle. `REPO` is the repository a launch recorded, or that of an unlaunched terminal's folder. `--json` prints `{"orca": <bool>, "sessions": [...]}`, with `null` for what is not known. A truncated terminal list is exit 3 and prints nothing. With no `orca` on PATH the registry rows still print, with state `unknown`, and the command exits 3 with the install line.

## `jitso link <from> <to>`

Records that one session talked to another, from the first to the second. Run it after a Claude Code `SendMessage`, which leaves no trace jitso can read. An Orca message needs no link: `jitso graph` reads it from the inbox, and a link would count it twice.

```
jitso link: L20261124T100000-a1a1 -> term_x recorded
```

Each side is a launch id, an Orca terminal handle or an exact session title (the title the launch gave, or the tab title Orca shows now). Launch ids and recorded handles need no Orca call. A title, or a handle the log does not hold, asks Orca for the terminal list; without Orca only what the log holds resolves, and the rest is exit 3 with the install line. Rejected (exit 2), with nothing written: an unknown reference, a title that names several sessions (the text lists them), two references to one session.

## `jitso graph [--json] [--port <n>]`

Read only. Without `--json` it serves a page on `127.0.0.1` (a free port unless `--port` names one), prints the URL and runs until Ctrl-C; it opens no browser. It is the human's view: give them the command, and do not leave it running in your own session. `jitso graph --json` prints one snapshot and exits:

| Key | Holds |
|---|---|
| `orca`, `notices` | whether the terminals could be listed, and what is missing from the snapshot |
| `project_empty` | why no node is of a project (not a git checkout, no registry, no session open), else `null` |
| `nodes` | one per session: `key`, `id`, `name`, `title`, `handle`, `role`, `type`, `build`, `harness`, `mode`, `state`, `listed`, `worktree`, `folder` (the last element of the worktree path), `branch`, `parent`, `project`. A closed launch appears only when a running one descends from it |
| `hierarchy` | `parent` and `child` keys |
| `synapses` | `from`, `to`, `direction` (`one-way` or `two-way`), `forward`, `back` (the count of each direction) and `last` |

Synapses add the `link` lines of this project's log to the messages of Orca's orchestration inbox, of which only sender, recipient, type, run and time are read. A missing or failing Orca still prints the snapshot, from the log, and exits 3. So does a call Orca has not answered after 10 seconds; the notice names it.

It runs from any folder. Outside a git checkout, or in a repository with no `.jitso/launches.jsonl`, there is no project fleet: the snapshot holds the Orca sessions and their synapses, `project_empty` says why, nothing is written and the page opens on every session.

## SessionEnd

When the guard hook is registered (a launch carries it in its `--settings`, or `jitso setup hook` registered it), the hook's SessionEnd entry writes the `end` event of the closing session's launch with source `session_end_hook`. A registry problem there is swallowed so a session can always close.

## What to report to a human after a fleet

State, from the log and the reports: which launches ran, which ended and how, what `reap` listed as an unlogged worker, and what is still open. Name what you did not verify.
