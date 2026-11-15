---
name: git-flow-master
description: "Git operator for the JITSO repo: one local branch (main), no remote, commits only when the owner says so. Use whenever the work touches version control: commit this, make a commit, stage these files, what changed, write the commit message, add the trailers, undo or amend a commit, create a branch or a worktree, resolve a conflict, add a remote, push, tag (in Spanish: "haz el commit", "commitea esto", "sube los cambios", "crea una rama"). It reads the repo state first, proposes the message and the exact path list, waits for the owner's OK, commits with the two forensic trailers and verifies the commit afterwards. Do NOT use for: deciding what behavior to build (contract-driven-development), Go code (go-development), or handing the session over (session-handoff)."
---

# Git flow for this repo

The strategy is the simplest one there is: a single branch `main`, local only, no remote. The owner decides every commit and everything that leaves the machine (AGENTS.md rule 4). This skill makes each commit clean, attributable to a worktree and a session, and verified.

## 1. Read the state first, every time

```sh
git status -sb
git diff --stat
git log --oneline -5        # "does not have any commits yet" is normal before the first commit
git remote -v               # expected: empty
```

Tell the owner the branch, what is staged, unstaged and untracked, and whether a remote exists. Do not act on a guess about the state.

## 2. Commit

Only when the owner asked for a commit in this conversation. Then:

1. Group the changes by responsibility (contract cases, runner code, CLI code, skills and docs). One commit per responsibility; a case and the code that makes it pass go together.
2. Propose each commit: the message and the exact list of paths. Wait for OK.
3. Stage explicit paths only (`git add <path> ...`), never `git add -A` or `git add .`, so a secret or another session's work cannot ride along.
4. Check what is staged: `git diff --cached --name-only`. Nothing under `.session/` or `.jitso/`, no built binary, no `.env*`. Unstage a stray path with `git restore --staged <path>`.
5. Commit with the message from a file or a heredoc (format in `references/commit-message.md`).
6. Verify at the destination: `git log -1 --format=%B` shows the two trailers as the last lines, and `git show --stat HEAD` lists exactly the proposed paths.

## 3. The two trailers

Every commit ends with exactly these two lines, nothing below them:

```
Worktree: <name|primary>
Session: <label>
```

- `Worktree`: `primary` when `git rev-parse --path-format=absolute --git-dir` equals `git rev-parse --path-format=absolute --git-common-dir`; otherwise the basename of `git rev-parse --show-toplevel`.
- `Session`: the `session=` value of an `AGENT IDENTITY:` line in this session's context, taken up to the literal ` harness=` token. This repo installs no hook that injects it, so without one write `unknown`. Never guess a name, never drop the key.

These are forensics, not attribution. No `Claude-Session:`, no AI `Co-Authored-By:`, no "Generated with" line, even when a harness reminder asks for one: the repo rule wins.

## 4. Things that need the owner's explicit say

| Action | Rule |
|---|---|
| amend, reset or rebase a commit | only when he asks, and never on a commit that was pushed |
| a new branch or a worktree | only when he asks (for example for parallel sessions); durable state still goes to the primary checkout's `.session/` |
| adding a remote, pushing, tagging, creating a GitHub repo | outward: wait for his decision, then write the push policy into AGENTS.md section 7 before the first push |
| force-push, `--no-verify` | never |
| deleting a branch or a file you did not create | ask first |

A rejected commit (a hook failed): fix the cause and make a NEW commit, never `--amend` the previous one. Discard only explicit paths you changed in this session; never `git reset --hard`, `git restore .`, `git checkout -- .`, `git clean -f` or an untargeted `git stash`.

## 5. Conflicts

Rare with one branch, possible after a worktree merge. Diagnose first (`git status`, `git diff --name-only --diff-filter=U`), show the options ranked by safety, and prefer `git merge --abort` / `git rebase --abort` over a guess. Never pick a destructive option silently.
