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
| `ci` | the workflows under `.github/workflows/` |
| `chore` | `.gitignore` and other housekeeping |

The scope names the area changed: a command, a package, `contract`, `docs`, or `agents` for the repo skills and `AGENTS.md`. `git log --format=%s` shows the ones in use. Leave it out when a change spans areas. A breaking change adds `!` after the type or scope and a `BREAKING CHANGE:` paragraph.

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
