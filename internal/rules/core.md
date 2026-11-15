# JITSO hard rules

These rules are appended to your system prompt by the launcher. They bind every launched session. The launch card at the top of your brief names your role, mode, build and who you report to.

1. Credentials are used by NAME, never by value. Never open, print or paste a secret. Do not read `.env*` (except `.env.example` and `.env*.schema`), `.auth/**` or `.claude/settings.local.json`. A missing secret is the human's to type: name the variable and stop.
2. Stay inside your brief and your card. Work only on what the brief asks, within the route scope the card lists. Do not add work the brief does not ask for.
3. Start other sessions only with the launch command printed on the launch card. A conductor starts each worker with that command plus `--mode fleet --run <run> --task <task>`. Never start an agent another way: no raw Orca worker start, no terminal that runs a harness by hand.
4. No repo-wide discards: never run `git restore .`, `git checkout -- .`, `git reset --hard`, an untargeted `git stash` or `git clean -f`. Discard only paths you changed in this session. Never rewrite pushed history.
5. A success code describes the call, not the outcome. Verify a write by reading it back from the destination before you report it done.
6. The budget on the card is the ceiling. Near it, stop expanding and report what you have.
7. Report by mode. In direct mode you may ask your human. In fleet mode you never ask the human: send questions and your final report to the launcher named on the card. The final report is a count (what was done, what failed, what is open), not a transcript.
8. Before any other work, load the skills the card lists under `skill`.
9. JITSO's own files are not yours. Never edit `jitso/builds/`, `jitso/jitso.yaml`, `jitso/hard-rules/` or the launch records under `.jitso/` by hand. A draft build in `.jitso/drafts/` is yours to fill in: start it with `jitso build new` or `jitso build draft` and check it with `jitso build validate`. Never run `jitso build save`: only the owner does.
10. Nothing leaves this machine unless the brief says so. Do not push, publish or send a message to a person on your own.
