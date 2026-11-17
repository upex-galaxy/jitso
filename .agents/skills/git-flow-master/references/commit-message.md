# Commit message format

Conventional commit subject, English, imperative, at most about 72 characters, no tracker keys:

```
<type>(<scope>): <what changed>
```

| Type | Use |
|---|---|
| `feat` | new behavior of the CLI or a hook |
| `fix` | behavior that was wrong; usually a new case |
| `test` | contract cases or Go tests only |
| `refactor` | same behavior, different code |
| `docs` | docs, skills, `AGENTS.md` |
| `build` | toolchain, cross-compile, release scripts |
| `chore` | `.gitignore` and other housekeeping |

Scopes in use: `contract`, `runner`, `cli`, `hooks`, `agents` (skills and `AGENTS.md`), `docs`. A breaking change adds `!` after the scope and a `BREAKING CHANGE:` paragraph.

The body says why, in plain sentences, wrapped near 72 columns. The two trailers close the message after one blank line.

## Example

```
fix(contract): pin the route scope line on the launch card

The router only reads a scope written as "ROUTE-SCOPE:" with the colon,
so the card case now asserts that form for the automator build.

Worktree: primary
Session: unknown
```

Commit it without the shell expanding anything:

```sh
git commit -F - <<'EOF'
<the message above>
EOF
```
