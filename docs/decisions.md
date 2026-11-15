# Decisions

What was decided for JITSO and this repo, the options each decision beat, and why. How the contract itself is built (runner, case format, status model) is recorded in `contract/PLAN.md`; per-behavior calls for the port are in `contract/TS-ODDITIES.md`.

To add a decision, append a row with the options it beat and the why. A reversed decision gets a new row that names the row it supersedes; the old row stays as history.

## Product and repo decisions

Decided by the owner on 2026-11-14.

| # | Decision | Options beaten | Why |
|---|---|---|---|
| D1 | The CLI is written in Go, from the new design, not translated line by line | a compiled TypeScript binary; TypeScript now and Go later; Rust | binary size (the compiled TypeScript CLI measured about 85 MB, a stdlib-only Go probe about 3 MB stripped), distribution as one file, hooks that need no Node or Bun, and the design changes anyway |
| D2 | JITSO has its own repository, kept local for now with no remote | create a private or public remote now | creating a remote is an outward step the owner has not taken yet |
| D3 | Contract tests first, then the port; the TypeScript implementation is the frozen oracle | finish the builds work in TypeScript first; write Go from scratch with no oracle | a contract the old and new binaries both run stops fixed bugs from coming back |
| D4 | Distribution through GitHub Releases, an install script and a Homebrew tap; an npm shim later | `go install` only | users who do not have Go |
| D5 | JITSO ships no builds: only patterns, a schema with validators, the hard-rules core, an authoring skill and empty templates; each project defines its own builds | ship generic builds | builds are specific to each project |
| D6 | The term is `build`, not `preset`. Builds are looked up in layers (the repo, then trusted parent folders, then the user), nearest wins, and are validated against the allowlists of the repo where they launch | keep `preset`; no parent-folder layer | monorepos and reuse across repos |
| D7 | Just-in-time builds are drafts validated by code, kept in a git-ignored folder; `jitso build save` promotes one only with the owner's OK | builds at user level only; never save a draft | keeps the allowlist guarantee for anything that launches |
| D8 | Adoption commands: `install skill --scope user\|project`, `setup hook`, `setup instructions` (a delimited, reversible block in the target's `AGENTS.md`) and `doctor`, each showing a diff and writing a backup | install the skill and the hook only | adoption in one command per step |
| D9 | Everything moves here first (code, the catalog patterns as templates, hooks, docs, the local history); only then is JITSO removed from the repo it came from, which becomes an ordinary consumer of the CLI; that repo's installer and updater do nothing with JITSO | keep a vendored copy in the original repo | less noise and an independent release cycle |

## Other decisions

| Decision | Options beaten | Why |
|---|---|---|
| The project history lives in the git-ignored `.session/` folder and is never committed | commit it as is | it is private: other languages, tracker keys, raw notes |
| Release 1 targets macOS, Linux and Windows; Windows is marked "not tested" until the owner tests it | macOS only | cross-compiling is one environment variable |
| A subagent is a build with `tier: subagent`; Conductor, Worker and Subagent are the three tiers of the same Build, and a worker lists its subagents by name (decided 2026-11-15). Each project chooses the names (for example planner, implementer, tester, verifier). JITSO resolves them with the same layered lookup and allowlists and passes them to `claude` as `--agents`. A subagent-tier build cannot be launched alone with `jitso launch` | a separate `jitso/subagents/<name>.md` file format; inline definitions inside the worker's YAML; dropping the `subagents` field | one concept instead of two, matches the owner's model of tiers, and answers where each helper's `description` and `prompt` come from (defect S3) |
| The repo is licensed MIT (decided 2026-11-15) | Apache-2.0; no license until the model is decided | the same license the original repo declares, permissive and simple for adoption |
| The rename from `preset` to `build` (D6) happens in this repo, not in the original | rename in the original first | avoids renaming twice. The contract still says `--preset` and `presets/` because it pins the oracle; the rename is a contract change made with the port, cases first |

## The contract-first rule

A behavior change starts as a case under `contract/cases/`. Behavior the oracle already has is pinned as `pass` for both implementations. Behavior the oracle lacks is written as the intended behavior with `status: {"ts": "xfail", "go": "pass"}`, a `defect` id and an `intent`. The Go port must pass every case. When the oracle and a case disagree, the oracle wins unless the behavior is a listed defect or a `TS-ODDITIES.md` row the owner decided. The procedure is the `contract-driven-development` skill; the format is `contract/SPEC.md`.

## Known defects of the oracle

Measured against the TypeScript implementation on 2026-11-14. Each is encoded as intended behavior, `xfail` on `ts` and `pass` on `go`; the cases are the ones that declare `status.ts: xfail` (`grep -rl '"xfail"' contract/cases`). N-2 to N-4 are decided TS-ODDITIES rows (L2, L3, L8, owner, 2026-11-15). N-5 to N-8 come from the owner's decisions of the same day (the tier model, the dispatcher project handoff, the launcher word of the dispatcher prompt).

| Id | Intended behavior | The oracle today |
|---|---|---|
| S1 `route_scope` | the launch card has six lines, with `ROUTE-SCOPE: <ids>` (the colon the router's regex needs) on a line of its own after the subagents line, and in `direct` mode the first prompt carries the same line | the card has five lines and writes `ROUTE-SCOPE` without the colon, as the tail of the subagents line; the first prompt only says to read the brief |
| S2 `agents_md` | a build with `agents_md: exclude` launches `claude` with `CLAUDE_CODE_DISABLE_CLAUDE_MDS=1` in its environment | the field is only logged. Measured on Claude Code 2.1.325 in a headless canary test: the variable stops both the native read of a lone `AGENTS.md` and a `CLAUDE.md` that imports `@AGENTS.md`. Not covered: user-level instruction files, nested or parent folders, other versions or models |
| S3 `subagents` | a worker build's `subagents` reach `claude` as `--agents <json>` with exactly those keys, each entry carrying the `description`, `prompt`, `model` and `tools` of the build (tier subagent) of that name | no `--agents` flag is ever passed |
| N-1 project root | the project is the git primary checkout of the working directory; outside a git checkout the command exits 3 | the oracle finds its project from its own script folder |
| N-2 failed launch | a launch that fails before its registry line leaves no `.jitso/launches/<id>/` folder behind (TS-ODDITIES L2) | the folder (brief, card, core, mcp.json, run.sh) stays on disk with no registry line, so `reap` never sees it |
| N-3 trust timeout | a trust dialog that never clears still exits 0 and registers the launch, and prints a warning on stderr that names the trust dialog and says it did not clear (TS-ODDITIES L3; beaten: exit 3 and close the terminal) | exit 0 and the registry line, with nothing on stderr |
| N-4 modes | only `direct` and `fleet` exist, on the command line and in registry lines; `directo` and `flota` are rejected (exit 2) and a registry line with either is an invalid line (TS-ODDITIES L8; beaten: read old lines but reject on input, keep both) | both aliases are accepted, normalized to the English mode, and read from old registry lines |
| N-5 tiers: launch and listing | a subagent-tier build is not launchable alone (`launch --preset plan` exits 2 with `JITSO REJECTED:` naming the tier) and is not shown by `presets list` or offered in the dispatcher catalog | the oracle has no tiers: it launches and lists every approved preset |
| N-6 tiers: subagent names | a worker whose `subagents` names a missing build, or a build that is not subagent tier, is a catalog error (exit 3, stderr names the file and the subagent) | the oracle only checks that each name is kebab-case |
| N-7 dispatcher project handoff | `JITSO_PROJECT` names the project: set to a git checkout it lets a launch run from outside git, set to a folder that is not a git checkout it exits 3 naming the folder, and the dispatcher starts `claude` with it set | the oracle finds its project from its own script folder and never reads the variable |
| N-8 dispatcher launcher word | the dispatcher prompt's launch line starts with the launcher itself (no `bun` word) and is the line the dispatch guard allows | the oracle's guard allows only `bun <index.ts> launch`, so the line the new prompt template tells the model to write is denied |
