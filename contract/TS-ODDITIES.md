# TS oracle: behaviors the cases pin that look accidental

The harvest of 2026-11-14 pinned the TypeScript binary as it is. The behaviors below are
real, measured, and covered by a passing case, but several look like accidents rather than
design. The Go port must either match them or change the case on purpose. Each row has a
proposed call; a row marked DECIDED carries the owner's call, every other row is still a proposal.
The end of a row's Proposal cell says what became of it: **DONE (<defect id>)** when the contract and the
Go port implement the call (`contract/README.md` has the defect), **OPEN (fix, not done)** for a `fix` row
nobody has taken up (the port keeps the oracle behavior and the case pins it), **OPEN (owner)** for an
`ask` row still waiting for the owner. A `keep` row has no marker: the port does what the oracle does.

Legend: **keep** = intended, port it as is. **fix** = change the case and the port.
**ask** = a product decision for the owner.

## Launch

| # | Behavior (case folder) | Proposal |
|---|---|---|
| L1 | `.mcp.json` is required even for a preset with no MCP servers; missing or broken file exits 3 (`launch/mcp-json/`) | DECIDED (owner, 2026-11-15): fix (reserved defect N-26): read it only when the preset lists servers **OPEN (fix, not done)**: the Go port still requires the file. |
| L2 | A failed launch leaves `.jitso/launches/<id>/` on disk with no registry line, so `reap` never sees it (`launch/`) | DECIDED (owner, 2026-11-15): fix. Go removes the launch folder when anything fails before the registry line. The failed-launch cases under `cases/launch/` change in the same step, cases first. Cases: defect N-2, `xfail` on `ts` (`launch/orca-failure/*`, `launch/trust/read-failure-closes-the-terminal`, `launch/trust/send-failure-closes-the-terminal`, `launch/fleet/worker-start-*`, `launch/mcp-json/*-exits-3`) **DONE (N-2)** |
| L3 | A trust dialog that never clears still exits 0 and writes the registry line (`launch/trust/dialog-that-never-clears-is-a-timeout`) | DECIDED (owner, 2026-11-15): keep exit 0 and register the launch, but print a warning on stderr that the dialog did not clear (option beaten: exit 3 and close the terminal). The timeout case gains a `stderr` expectation. Case: defect N-3, `xfail` on `ts` (`launch/trust/dialog-that-never-clears-is-a-timeout`) **DONE (N-3)** |
| L4 | The trust check uses the folder Orca reports, not `--worktree` (`launch/trust/`) | DECIDED (owner, 2026-11-15): superseded, check always (see docs/decisions.md, defect N-22); the earlier proposal was keep **OPEN (fix, not done)** |
| L5 | `--parent` is validated before `--dry-run`: an unknown parent exits 2 even on a dry run | keep |
| L6 | `--mcp-exclude` with an unknown or differently cased name is ignored silently | DECIDED (owner, 2026-11-15): fix (reserved defect N-26): warn on stderr **OPEN (fix, not done)** |
| L7 | An empty `jitso` script value in `package.json` counts as declared (`card/launch-line/`) | DECIDED (owner, 2026-11-15): fix (reserved defect N-26): require a non-empty string **OPEN (fix, not done)** |
| L8 | The legacy modes `directo` and `flota` are accepted by `launch` and normalized (`validation/mode`, tag `legacy-alias`; registry lines too) | DECIDED (owner, 2026-11-15): neither read nor accept. Go knows only `direct` and `fleet` everywhere, registry lines included (options beaten: read old lines but reject on input; keep both as TS). The `legacy-alias` cases and `registry/spanish-mode-alias-is-read` flip. Cases: defect N-4, `xfail` on `ts` (`validation/mode/legacy-alias-directo`, `validation/mode/legacy-alias-flota`, `registry/spanish-mode-alias-is-read`) **DONE (N-4)** |
| L9 | `--mode` has no default; omitting it is an error | keep |
| L10 | `launch --help` and `-h` are "unknown argument", exit 2 | DECIDED (owner, 2026-11-15): fix (reserved defect N-26): print usage for `launch` **OPEN (fix, not done)** |
| L11 | `--preset --dry-run` takes `--dry-run` as the preset value (the flag is `--build` in the contract since N-10, so the case is `validation/flag-takes-next-word-as-value`) | DECIDED (owner, 2026-11-15): fix (reserved defect N-26): a value may not start with `--` **OPEN (fix, not done)** |
| L12 | Every launch calls `terminal read` once even when the folder is trusted (`launch/direct/orca-call-order`) | keep, or drop if the port reads the screen only when needed |

## Brief

| # | Behavior | Proposal |
|---|---|---|
| B1 | The word count includes headings, and a cited path splits on punctuation (`src/sync.ts` counts as 3) | DECIDED (owner, 2026-11-15): fix (reserved defect N-27): count words, not tokens, so headings and cited paths that split on punctuation no longer inflate the count **OPEN (fix, not done)** |
| B2 | `Output to:` does not exempt a path; only `Output:`, `Outputs:`, `Create:`, `Creates:` do | keep |
| B3 | The `ticket_keys` check exists in code but the CLI never passes a request, so it is unreachable | DECIDED (owner, 2026-11-15): fix (reserved defect N-27): either wire it to a flag or drop it **OPEN (fix, not done)** |
| B4 | Failure order is sections, length, language, paths (all reported together) | keep |

## CLI surface

| # | Behavior | Proposal |
|---|---|---|
| C1 | `tree` and `presets` (without `list`) print "not implemented yet" and exit 0 | fix: exit 2 until implemented, or implement `tree`. The `presets` half is settled by N-10: `presets` is an unknown command, and `jitso build` alone exits 2 with the build usage (`cli/presets-is-unknown`, `build/usage`). `tree` is unchanged **DONE (N-10)** for the `presets` half; DECIDED (owner, 2026-11-15): fix (reserved defect N-30) for the `tree` half, **OPEN (fix, not done)**: `tree` still prints "not implemented yet" and exits 0 (`cli/tree-not-implemented`). |
| C2 | An empty first argument is an unknown command, not the usage | keep |
| C3 | A broken preset file: `launch` exits 3, but `presets list` crashes uncaught (exit 1, not pinned). The command is `build list` since N-10, and a broken build file there gives the same handled error as `launch` (exit 3) | fix: same handled error in both **DONE (N-10)**: `build list` loads the catalog the way `launch` does, so a broken build file gives the same handled error (exit 3); no case pins `build list` on its own. |
| C4 | Outside a git checkout the TS binary still works, because it finds its project from the script folder (`project/outside-git`, xfail N-1) | fix: project = git primary checkout of the cwd, exit 3 outside one **DONE (N-1)** |

## Registry, close, reap, analyze

| # | Behavior | Proposal |
|---|---|---|
| R1 | `close` writes the `end` event before closing the terminal; if the close then fails the launch is already ended and the exit is 3 (`close/orca-close-failure-is-exit-3`) | DECIDED (owner, 2026-11-15): fix (reserved defect N-28): close first, then write the event, or write it only on success **OPEN (fix, not done)** |
| R2 | A second `close` on an ended launch exits 0, writes no second event, still calls `terminal close` and `terminal list` | keep |
| R3 | `reap` never ends a launch that has no terminal handle (`reap/launch-without-terminal-is-never-reaped`) | DECIDED (owner, 2026-11-15): fix (reserved defect N-28): end it as `reap` with outcome `none` **OPEN (fix, not done)** |
| R4 | The unlogged-worker count repeats on every `reap`; the rule-break line is appended once; with several `--run` values the same worker is flagged once per run | DECIDED (owner, 2026-11-15): fix (reserved defect N-28): de-duplicate per worker **OPEN (fix, not done)** |
| R5 | `reap` swallows an analysis failure as exit 0 after printing `could not analyze <id>` | keep, report the count |
| R6 | `analyze` loads `prices.json` before it checks its arguments, so a malformed id exits 3 when the file is missing | DECIDED (owner, 2026-11-15): fix (reserved defect N-28): validate arguments first (exit 2) **OPEN (fix, not done)** |
| R7 | Secret-path reads count only `Read`, `Grep`, `Glob` and `Bash`; a `Write` to a secret-looking path counts 0 | DECIDED (owner, 2026-11-15): fix (reserved defect N-28): a `Write` to a secret-looking path counts in `analyze` **OPEN (fix, not done)** |
| R8 | The first `launch` line wins for a duplicate id | keep |
| R9 | A stuck registry lock makes a launch exit 3 after about 5 s and closes the terminal it just opened | keep |
| R10 | `budget_over` is a valid rule-break kind but no code emits it; only `budget_pct` is written | DECIDED (owner, 2026-11-15): fix (reserved defect N-28): emit it (the audit also lists unenforced budgets) **OPEN (fix, not done)** |

## Hooks and dispatcher

| # | Behavior | Proposal |
|---|---|---|
| H1 | `guard` fails open on malformed input (exit 0, empty stdout); `dispatch-guard` fails closed | keep, both are deliberate |
| H2 | The deny text of `guard` says `bun run jitso launch --preset <id> ...` | fix: name the launcher the target repo resolves (`jitso launch` or `bun run jitso launch`) and the flag `--build <id>` (N-10, `hooks/guard/deny-reason-names-the-build-flag`); the other cases match only the `(denied: <verb>)` tail **DONE (N-10)** |
| H3 | `dispatch-guard` allows a bare `<launcher> launch` with no flag (the flag loop is empty). Its allowed flags name `--preset`; since N-10 the contract says `--build` and the oracle denies it (`hooks/dispatch-guard/bash-allows-the-launcher`) | fix: require `--build`, `--brief`, `--mode` **DONE (N-10)** for the flag name; for requiring `--build`, `--brief` and `--mode`: DECIDED (owner, 2026-11-15): fix (reserved defect N-29), **OPEN (fix, not done)** (`hooks/dispatch-guard/bash-allows-bare-launch` still pins the oracle). |
| H4 | `guard` passes `send_instruction` and `message_worker` tools; it only denies `start_worker`, `create_worktree` with an agent and `create_terminal` with a harness | DECIDED (owner, 2026-11-15): keep; the guard keeps passing `send_instruction` and `message_worker`. The adoption verbs and `build save` are guarded since then (`docs/decisions.md`, defects N-15 and N-19); `send_instruction` and `message_worker` still pass. |
| H5 | `dispatch-guard` rejects an env prefix and trailing or double spaces; its Write check says "a new brief" but only rejects symlinks, so overwriting an existing regular file passes | DECIDED (owner, 2026-11-15): fix (reserved defect N-29): reject an existing file **OPEN (fix, not done)** |
| H6 | The dispatcher joins all argv words with a single space once the first has whitespace; an empty-string argument is an unknown command | keep |
| H7 | The oracle's hooks and dispatcher resolve the repo from their own file location (`dispatch-guard`, `dispatch.ts`, `index.ts`) | fix: resolve from the git root of the cwd or `CLAUDE_PROJECT_DIR` **DONE**: the Go hooks resolve the project from `CLAUDE_PROJECT_DIR`, else the git primary of the cwd (`docs/architecture.md` section 6); no case pins `CLAUDE_PROJECT_DIR`. |

## Known defects (xfail on `ts`, see PLAN.md)

S1 to S3 and N-1 to N-21: `README.md` lists every one with its cases and the intended behavior, and is the place to read them. Of the rows above, L2, L3, L8, C4 and the settled halves of C1, H2 and H3 are defects there.
