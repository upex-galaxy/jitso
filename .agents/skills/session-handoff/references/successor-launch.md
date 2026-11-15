# Launching the successor

The handoff file is written first, in full, and only then does the successor start. That order is not reversible.

## This is not orchestration

A handoff transfers a whole session. There is no task, no dispatch, no mailbox and nothing comes back, because the successor IS the predecessor. Orchestration verbs (runs, tasks, supervised workers) are the wrong family: they create a worker the owner then has to release and close for nothing.

| | Handoff (this skill) | Orchestration |
|---|---|---|
| what moves | the whole session | one scoped task |
| the predecessor | ends | keeps working and supervises |
| channel afterwards | none, by design | messages, questions, replies |
| runtime objects | one terminal | run, task, worker |

## Is Orca reachable?

Check, and stay silent about Orca when it is not there:

```sh
command -v orca >/dev/null && orca status --json </dev/null
```

No binary, or a status that fails: use the path without a runtime. Always pass `</dev/null` to `orca` commands. Do not copy Orca's command grammar into this repo; ask the binary (`orca --help`, `orca terminal --help`) or load the user-level `orca-cli` skill when it is installed.

## With Orca

Preconditions, all three verified first:

1. the runtime is reachable (the check above)
2. the handoff file exists at its final path and is complete
3. the target is the SAME worktree the predecessor runs in, and the SAME harness

Create one terminal in that worktree whose command is the harness's own launch line, carrying the session name (the handoff basename without `.md`) and the first prompt below. For Claude Code that is `claude -n <name> "<first prompt>"`; confirm the flag with `claude --help` on the machine before using it. Announce the launch to the owner before firing it and say which terminal will carry it.

## Without Orca

Identical except for who opens the terminal: print the launch line and the handoff path, and the owner pastes it. The prompt is byte-identical to the Orca path; the path nobody exercises is the one that breaks silently.

## The first prompt, both paths

One sentence:

> Read `<absolute path to the handoff>` top to bottom and continue that session exactly where it stopped.

If section 0 of the handoff names an order the successor must respect, say it in the prompt too. A file pointer reads as reference material; an instruction in the prompt reads as an instruction.

## After the successor is up

1. Confirm on screen that the successor started and has the file.
2. Save anything durable to memory, because the handoff is disposable and gitignored.
3. Stop.

Do not close the predecessor's own terminal: ending it is the owner's call, and a session that closes itself cannot report that the handoff failed.
