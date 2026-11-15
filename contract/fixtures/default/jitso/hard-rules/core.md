# JITSO hard rules

These rules are appended to your system prompt by the launcher. They bind every launched session. The launch card at the top of your brief names your role, mode, build and who you report to.

1. Credentials are used by NAME, never by value (Critical Rule 1). Never open, print or paste a secret. Do not read `.env*` (except `.env.example` and `.env*.schema`), `.auth/**` or `.claude/settings.local.json`. A missing secret is the human's to type: name the variable and stop.
2. No AI attribution in commits. Every commit ends with two trailers, in this order: `Worktree: <name|primary>` then `Session: <label>`.
3. No repo-wide discards: never run `git restore .`, `git checkout -- .`, `git reset --hard`, an untargeted `git stash` or `git clean -f`. Discard only paths you changed in this session. Never rewrite pushed history.
4. A success code describes the call, not the outcome. Verify a write by reading it back from the destination before you report it done.
5. Context budget is 250k tokens. Past roughly 200k, stop expanding and report what you have.
6. Reporting: a session in direct mode may ask its human; a session in fleet mode never asks the human and sends questions and its final report to the launcher named on the card. The final report is a count (what was done, what failed, what is open), not a transcript.
7. Launch other sessions only with the launch command printed on the launch card, never a raw `orca` worker-start. A conductor starts each worker with that command plus `--mode fleet --run <run> --task <task>`. Never use `TaskCreate`.
8. Before any other work, load the skill the card lists under `skill`, by its path under `.agents/skills/`.
9. Repo artifacts are written in English.
