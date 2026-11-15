# Triage: what each runner line means and what to do

Run the failing case again with `-v` (and `--keep` to inspect the sandbox) before deciding anything. The runner prints the step, the expectation that broke, the expected and the actual value.

| Line | Against the oracle (`--impl ts`) | Against the port (`--impl bin`) |
|---|---|---|
| `PASS` | the case holds | the case holds |
| `FAIL` on a `pass` case | the case misreads the oracle, or the oracle has a defect nobody listed. Read the oracle's code and its own test for that behavior. Case wrong: fix the case. Oracle wrong: stop and bring it to the owner as a candidate defect (an `xfail` needs his OK, because it changes what JITSO is meant to do) | a regression or unported behavior: fix the Go code, not the case |
| `XFAIL` | expected for intended behavior | cannot happen: `status.go` is never `xfail` |
| `XPASS` | the oracle now does the intended thing, or the case passes for the wrong reason. The oracle is frozen, so suspect the case first: check that its expectations would fail on the old behavior | cannot happen |
| `LOAD` | the case does not parse or breaks a SPEC rule (unknown key, bad regex, unknown placeholder, `id` not equal to the folder). Fix the case; the rest of the suite still ran | same |
| `FAIL` with a runner error | the sandbox or a step could not start: an environment problem (no `bun` or `node` on PATH, a wrong `--ts-src`), not a verdict on the behavior | same, for example the binary path |

## Checks before you call it a defect

1. Run the oracle's own test for that behavior in the boilerplate checkout (read-only; do not edit it there).
2. Confirm the case cites only files its `project` provides (SPEC section 9, last paragraph): a case that reads a file of the oracle's repo passes on `ts` and fails on the port for the wrong reason.
3. A case tagged `posix` needs `/bin/sh`; skipped elsewhere with `--skip-tag posix`.
4. Timing cases (`elapsed_ms_lt`) assume `--jobs 1`; a failure under parallel jobs is not a verdict.

## A new defect, once the owner agrees

1. Write the case as the intended behavior: `status: {"ts": "xfail", "go": "pass"}`, a new `defect` id, an `intent` that says what is intended and what the oracle does today.
2. Run it on the oracle: it must be `XFAIL`. If it is `XPASS`, the case does not discriminate; tighten it.
3. Add the defect row to `docs/decisions.md` and to `contract/README.md`.
