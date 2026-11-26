---
name: git-flow-master
description: "Git operator for the JITSO repo: a single branch main and the public remote origin; commits and pushes wait for the owner, except documentation-only ones. Use whenever the work touches version control: commit this, make a commit, stage these files, what changed, write the commit message, add the trailers, undo or amend a commit, create a branch or a worktree, resolve a conflict, change a remote, push, tag, or run the release that follows a push (in Spanish: 'haz el commit', 'commitea esto', 'sube los cambios', 'crea una rama'). It reads the repo state first, gets the owner's OK where one is needed, commits explicit paths with the two forensic trailers and verifies the result. Do NOT use for: deciding what behavior to build (contract-driven-development), Go code (go-development), or handing the session over (session-handoff)."
---

# Git flow for this repo

A single branch `main`. The remote `origin` is the public repository named in `docs/release.md`. No pull-request flow: one maintainer. AGENTS.md rule 4 and section 7 are the policy; this skill is the procedure that makes each commit and push clean, attributable to a worktree and a session, and verified.

## 1. Read the state first, every time

```sh
git status -sb
git diff --stat
git log --oneline -5
git log --oneline origin/main..HEAD    # not pushed yet, as of the last fetch
```

Tell the owner the branch, what is staged, unstaged and untracked, and which commits are not on `origin/main`. Do not act on a guess about the state.

## 2. Which changes need the owner's say

| Change | Commit | Push to `main` |
|---|---|---|
| documentation only | no separate OK: do it and report it | no separate OK, when every commit the push carries is yours and documentation only |
| code, contract cases, deletions, anything else | he asks, after seeing the message and the exact path list | he asks in this conversation |
| a commit this session did not make | | needs his say |

Text the binary embeds, a contract fixture, a script and a workflow are not documentation. When in doubt, ask. The documentation exception covers the commit and the push: an edit of `AGENTS.md` still needs his OK.

## 3. Commit

1. Group the changes by responsibility (contract cases, runner code, CLI code, skills and docs). One commit per responsibility; a case and the code that makes it pass go together.
2. Write each commit's message and exact list of paths. Where section 2 asks for his say, propose them and wait for OK.
3. Stage explicit paths only (`git add <path> ...`), never `git add -A` or `git add .`, so a secret or another session's work cannot ride along.
4. Check what is staged: `git diff --cached --name-only`. Nothing under `.session/` or `.jitso/`, no built binary, no `.env*`. Unstage a stray path with `git restore --staged <path>`.
5. Commit with the message from a file or a heredoc (format in `references/commit-message.md`).
6. Verify at the destination: `git log -1 --format=%B` shows the two trailers as the last lines, and `git show --stat HEAD` lists exactly the intended paths.

## 4. Push

1. `git fetch origin`, then `git log --oneline origin/main..HEAD`. Every commit listed is allowed by section 2.
2. `git push origin main`. A rejected push means `origin/main` moved: report it and ask.
3. Verify: after `git fetch origin`, `git rev-parse HEAD` and `git rev-parse origin/main` print the same commit.

## 5. The release that follows a push

His OK to push a release-worthy `main` also covers its release: run it right after the push, without asking again. `docs/release.md` section 4 is the procedure.

1. Wait for `ci.yml` to pass on the pushed commit. A red CI stops the flow before the tag.
2. Pick the next version by the policy in `docs/release.md` section 2.
3. Create and push the annotated tag, then watch the release workflow.
4. Verify the published release with `scripts/verify-release.sh <tag>`.
5. Update the docs that `docs/release.md` section 4 step 8 lists.
6. Report. Announcing the release stays his.

A bad release is fixed by a new version (`docs/release.md` section 6).

## 6. The two trailers

Every commit ends with exactly these two lines, nothing below them:

```
Worktree: <name|primary>
Session: <label>
```

- `Worktree`: `primary` when `git rev-parse --path-format=absolute --git-dir` equals `git rev-parse --path-format=absolute --git-common-dir`; otherwise the basename of `git rev-parse --show-toplevel`.
- `Session`: the `session=` value of an `AGENT IDENTITY:` line in this session's context, taken up to the literal ` harness=` token. This repo installs no hook that injects it, so without one write `unknown`. Never guess a name, never drop the key.

These are forensics, not attribution. No `Claude-Session:`, no AI `Co-Authored-By:`, no "Generated with" line, even when a harness reminder asks for one: the repo rule wins.

## 7. Other actions

| Action | Rule |
|---|---|
| amend, reset or rebase a commit | only when he asks, and never on a commit that was pushed |
| a new branch or a worktree | only when he asks (for example for parallel sessions); durable state still goes to the primary checkout's `.session/` |
| a tag outside the release of section 5, a change to a published release, adding or changing a remote, creating a GitHub repo | outward: wait for his decision |
| deleting a branch or a file you did not create | ask first |
| force-push, `--no-verify`, moving or reusing or deleting a tag, deleting a release | never |

A rejected commit (a hook failed): fix the cause and make a NEW commit, never `--amend` the previous one. Discard only explicit paths you changed in this session; never `git reset --hard`, `git restore .`, `git checkout -- .`, `git clean -f` or an untargeted `git stash`.

## 8. Conflicts

Rare with one branch, possible after a worktree merge. Diagnose first (`git status`, `git diff --name-only --diff-filter=U`), show the options ranked by safety, and prefer `git merge --abort` / `git rebase --abort` over a guess. Never pick a destructive option silently.
