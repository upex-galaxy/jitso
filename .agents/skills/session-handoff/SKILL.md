---
name: session-handoff
description: "Compact this whole agent session into a handoff file so a NEW session resumes exactly where this one stopped, as if the context window had been extended rather than reset. Use when the context window is getting high, when work must continue past the end of this session, or when the owner asks to hand over, continue in a fresh session, pass the context on, or write a handoff (the owner also says it in Spanish: "hagamos el handoff", "pasa el contexto a otra sesion", "continua esto en otra sesion"). Writes <PRIMARY_ROOT>/.session/handoffs/<session-name>-handoff-NN.md in the primary checkout (also from a worktree) and launches the successor in the same worktree and the same harness, through Orca when it is reachable, otherwise by printing the line for the owner. Do NOT use for: delegating a scoped task to a subagent or worker while this session keeps working, or saving durable facts across sessions (that is memory, the repo or docs/decisions.md)."
---

# Session handoff

A handoff is context transplanted, not summarized. The successor IS this session with a new window: everything it needs must be on disk, addressable, and true when it reads it.

## When

Manual is the trigger. Write one when:

- the context is past the threshold the owner set (he states it per session; if he did not, ask before assuming one)
- the session is about to end with work in flight
- the next step (a big harvest, a long read) would eat the budget the work after it needs
- the owner asks

Do not write one when the remaining work fits. A handoff costs a real slice of context, and an early successor pays the startup cost for nothing. Do not promise an automatic trigger: none is built here.

## The three steps

1. **Capture.** Walk `references/capture-contract.md` section by section. All ten sections are mandatory; an empty one is written as `none` plus the reason, never dropped.
2. **Write.** Fill `templates/handoff.md` into `<PRIMARY_ROOT>/.session/handoffs/<session-name>-handoff-NN.md`.
3. **Launch the successor.** Follow `references/successor-launch.md`.

## Paths and names

`<PRIMARY_ROOT>` is the primary checkout, the same from the primary and from every linked worktree:

```sh
dirname "$(git rev-parse --path-format=absolute --git-common-dir)"
```

- `.session/` is gitignored. A handoff is disposable state, not a project record; it never goes into a commit.
- It lives in the primary checkout even when the session runs in a worktree: a file inside a worktree dies with it, and two worktrees would each count `NN` from an empty folder. The successor still launches in the SAME worktree.
- `NN` is two digits from `01`, counted across the lineage. List `<PRIMARY_ROOT>/.session/handoffs/` before choosing it.
- The successor's session name is the handoff basename without `.md`, so the lineage reads from the file list: `<base>`, `<base>-handoff-01`, `<base>-handoff-01-handoff-02`.
- `<session-name>` is the `session=` value of an `AGENT IDENTITY:` line when a hook injects one (spaces and parentheses become `-`). This repo installs no such hook, so usually there is none: ask the owner for the name rather than inventing one.

## Hard rules

1. Label every claim `measured`, `read` or `predicted`.
2. Mark live state `PERISHABLE` with the wall-clock time it was measured and the command that re-verifies it.
3. Perishable beats priority: if a live item needs attention first, say so in that item.
4. Copy ids verbatim in backticks (terminal handle, session id, SHA, run id). "The worker from earlier" is not an id.
5. Every path is absolute.
6. Name what is NOT done and why.
7. Write the file before launching the successor; the order is not reversible.
8. Same harness for the successor.
9. English and no AI attribution (AGENTS.md section 5 and rule 5), even though the file is never committed.
10. Never delete an earlier handoff; the lineage is the audit trail.
11. No secrets: name the variable, never the value; no tokens, cookies or `.auth/**` paths.

## Files

| File | Holds |
|---|---|
| `references/capture-contract.md` | the ten sections, what each is for, the failure it prevents |
| `references/successor-launch.md` | launching with and without Orca, and why this is not orchestration |
| `templates/handoff.md` | the skeleton to fill |
