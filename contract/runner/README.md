# JITSO contract runner

Runs the cases under `contract/cases/` against one `jitso` binary. The case format is `../SPEC.md`; that file wins when this README and the code disagree.

Go standard library only, no cgo. The same binary is also the fake `orca` and the fake `claude`: each sandbox gets a symlink (then a hard link, then a copy) of it under those names in `bin/`, and `main` dispatches on `os.Args[0]`.

## Build and run

```sh
cd contract/runner
go build -o jitso-contract-runner .

# the binary under test (built from the repo root: go build -o <scratch>/jitso ./cmd/jitso)
./jitso-contract-runner --impl bin --bin ./path/to/jitso
```

`go run . --impl bin --bin <path>` works too, but `go run` reports every non-zero exit as `1`. Use the built binary where the exit code matters (CI): `0` green, `1` a `FAIL`, `2` a load or usage error (an unknown flag or a value of `--impl` other than `bin` included).

| Flag | Meaning |
|---|---|
| `--impl bin` | the implementation under test (required; `bin` is the only value) |
| `--bin <path>` | the `jitso` executable |
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
| `internal/engine` | one case run: sandbox build, the `Step` kinds (`run`, `hook`, `exec`, `write`, `git_worktree`), captures, the `PASS`/`FAIL` outcome |
| `internal/sandbox` | the temp root, the from-scratch environment, file copies |
| `internal/impl` | the `Implementation` interface and the `bin` adapter |
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

## The adapter

`internal/impl` holds the `Implementation` interface and its one adapter, `bin`:

| Method | Does |
|---|---|
| `Name()` | the name the summary prints |
| `Prepare(sb)` | installs what the implementation needs into a fresh sandbox (nothing for `bin`) |
| `Command(sb, args)` | the argv that runs the CLI |
| `HookCommand(sb, hook)` | the argv that runs `guard` or `dispatch-guard` |
| `Launcher(sb)` | the value of `{{JITSO}}` |

A second adapter would be a new file there plus a case in `newImplementation` in `main.go` and a row in SPEC 9.
