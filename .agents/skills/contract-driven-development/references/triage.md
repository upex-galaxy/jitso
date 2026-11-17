# Triage: what each runner line means and what to do

Run the failing case again with `-v` (and `--keep` to inspect the sandbox) before deciding anything. The runner prints the step, the expectation that broke, the expected and the actual value.

| Line | Meaning | What to do |
|---|---|---|
| `PASS` | the case holds | nothing |
| `FAIL` | an expectation broke | decide which side is wrong. A regression or unported behavior: fix the Go code, not the case. A case that misreads the intended behavior: fix the case and say so, because it changes what JITSO is meant to do (an owner decision when it is product behavior) |
| `LOAD` | the case does not parse or breaks a SPEC rule (unknown key, a leftover `status` key, bad regex, unknown placeholder, `id` not equal to the folder). Fix the case; the rest of the suite still ran | fix the case |
| `FAIL` with a runner error | the sandbox or a step could not start: an environment problem (a wrong `--bin` path, no `/bin/sh`), not a verdict on the behavior | fix the environment, rerun |
| `SKIP` | excluded by `--skip-tag` | nothing |

## Checks before you change a case

1. Read the case's `intent` and `source`: they say why the case exists and where the behavior came from.
2. Confirm the case cites only files its `project` provides (SPEC section 9, last paragraph).
3. A case tagged `posix` needs `/bin/sh`; skipped elsewhere with `--skip-tag posix`.
4. Timing cases (`elapsed_ms_lt`) assume `--jobs 1`; a failure under parallel jobs is not a verdict.

## A new behavior, once the owner agrees

1. Write the case as the intended behavior, with an `intent` that says what is intended and why.
2. Run it against the built binary: it must `FAIL` for the reason it pins. If it passes, the case does not discriminate; tighten it.
3. Implement, rerun, then the whole suite.
4. Record the decision as a row in `docs/decisions.md`; give it a defect id only if the change is a deliberate difference worth tracking, and add that id to the defect table there.
