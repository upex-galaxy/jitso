# JITSO contract runner

Runs the cases under `contract/cases/` against one `jitso` implementation. The case format is `../SPEC.md`; that file wins when this README and the code disagree.

Go standard library only, no cgo. The same binary is also the fake `orca` and the fake `claude`: each sandbox gets a symlink (then a hard link, then a copy) of it under those names in `bin/`, and `main` dispatches on `os.Args[0]`.

## Build and run

```sh
cd contract/runner
go build -o jitso-contract-runner .

# the frozen TypeScript oracle (needs bun and node on PATH)
./jitso-contract-runner --impl ts --ts-src ../../../agentic-qa-boilerplate

# any binary, judged by the `go` status of each case
./jitso-contract-runner --impl bin --bin ./path/to/jitso
```

`go run . --impl ts --ts-src <checkout>` works too, but `go run` reports every non-zero exit as `1`. Use the built binary where the exit code matters (CI): `0` green, `1` a `FAIL` or `XPASS`, `2` a load or usage error.

| Flag | Meaning |
|---|---|
| `--impl ts\|bin` | implementation under test (required) |
| `--ts-src <dir>` | boilerplate checkout holding `cli/jitso/` (with `--impl ts`) |
| `--bin <path>` | the executable (with `--impl bin`) |
| `--cases <dir>` | cases folder; default `contract/cases`, found by walking up from the cwd, then from the executable |
| `--fixtures <dir>` | fixtures folder; default the sibling `fixtures` of the cases folder |
| `--filter <regex>` | run only cases whose expanded id (`area/slug[2]`) matches |
| `--skip-tag <tag>` | report cases with this tag as `SKIP` (repeatable), for example `posix` |
| `-v` | print argv, cwd, stdout and stderr of every step, and the sandbox |
| `--keep` | keep each sandbox (under the system temp folder) and print its path |
| `--jobs <n>` | cases in parallel, default `1`; `elapsed_ms_lt` cases assume `1` |

Self checks: `go vet ./... && gofmt -l . && go test ./...`.

## Layout

| Package | Holds |
|---|---|
| `main.go` | flags, case discovery, the multicall switch |
| `internal/spec` | loading and validating `case.json`, placeholders, `for_each`, file maps, step parsing |
| `internal/expect` | the expectation keys and text matchers (SPEC 7), each a registered builder |
| `internal/engine` | one case run: sandbox build, the `Step` kinds (`run`, `hook`, `exec`, `write`, `git_worktree`), captures, status logic |
| `internal/sandbox` | the temp root, the from-scratch environment, file copies |
| `internal/impl` | the `Implementation` interface and the `ts` and `bin` adapters |
| `internal/fakes` | the fake `orca` (stateless, replays its own call log) and the fake `claude` |
| `internal/report` | the text reporter |

A case is validated at load time by building every expansion with stub values, so an unknown key, a bad regex or an unknown placeholder is a `LOAD` error naming the case, never a failure in the middle of a run.

## Adding a text matcher

Create one file in `internal/expect`, for example `text_starts_with.go`:

```go
package expect

import (
	"fmt"
	"strings"

	"jitso-contract-runner/internal/jsonx"
)

func init() {
	registerText("starts_with", func(v any, what string) (TextMatcher, error) {
		s, err := jsonx.String(v, what)
		if err != nil {
			return nil, err
		}
		return startsWith(s), nil
	})
}

type startsWith string

func (m startsWith) Name() string { return "starts_with" }

func (m startsWith) Match(text string) *Mismatch {
	if strings.HasPrefix(text, string(m)) {
		return nil
	}
	return &Mismatch{Expected: fmt.Sprintf("starts with %q", string(m))}
}
```

It is then valid everywhere a text matcher is (`stdout`, `stderr`, `reason`, `last_arg`, file `text` and `entries`, and as an argument pattern in `argv` and `arg_after`). Document it in SPEC 7.1 in the same change and add a row to `TestTextMatchers`. A new expectation key works the same way with `register(name, kinds, builder)`, the kinds being the step kinds it is valid on.

## Adding an implementation adapter

Implement `impl.Implementation` in a new file of `internal/impl`:

| Method | Does |
|---|---|
| `Name()` | the name the summary prints |
| `StatusKey()` | which `status` key of a case judges it (`ts` or `go`; a new key also goes in `spec.StatusKeys` and SPEC 2) |
| `Prepare(sb)` | installs what the implementation needs into a fresh sandbox: files under `sb.Project`, programs through `sb.LinkTool` (never a real folder on PATH), variables through `sb.SetEnv` |
| `Command(sb, args)` | the argv that runs the CLI |
| `HookCommand(sb, hook)` | the argv that runs `guard` or `dispatch-guard` |
| `AroundStep(sb)` | called before every `run` and `hook` step (not `exec`, `write` or the git steps); returns the function the engine calls right after the step's process ends, before captures and expectations, even when the process failed to run. `bin` returns a no-op. `ts` maps the registry file between the case's schema and the oracle's (`internal/impl/registrymap.go`, SPEC 9). Keep the state in the returned closure, never in the adapter value: sandboxes run in parallel |

Then add its case to `newImplementation` in `main.go` and a row to SPEC 9. Return an error from the constructor when a prerequisite is missing, so the run stops before any case starts.
