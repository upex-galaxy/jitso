# Release procedure

How a JITSO release is built, checked and published. Everything that leaves this machine is marked **OWNER-GATED**: it waits for the owner's say in the conversation (AGENTS.md rule 4), and no agent runs it on its own. The local steps change nothing outside this repository and the scratch folder you build into.

Nothing here has been published. The files this procedure uses exist and have been run locally: `scripts/package.sh`, `scripts/install.sh`, `scripts/test-install.sh`, `scripts/fill-formula.sh`, `packaging/homebrew/jitso.rb`. `.github/workflows/release.yml` is written and has never run.

Committed prose names the source of truth, not its value (AGENTS.md rule 12), so this file has no sizes, counts or dates: it says which file or command holds each one.

## 1. What a release is

A release is a tag `v<x>` on `main` and a GitHub Release of that tag. Its assets, with no version in the names:

| Asset | What it is |
|---|---|
| `jitso-darwin-<arch>.tar.gz`, `jitso-linux-<arch>.tar.gz` | one folder `jitso-<os>-<arch>/` with `jitso`, `LICENSE` and `README.md`; `<arch>` is `amd64` or `arm64` |
| `jitso-windows-<arch>.zip` | the same folder with `jitso.exe` |
| `checksums.txt` | one line per asset above and `install.sh`, in the output format of `sha256sum` |
| `install.sh` | a copy of `scripts/install.sh` |

`scripts/package.sh` is the only definition of these names and of the build flags (`CGO_ENABLED=0`, `-trimpath`, `-ldflags '-s -w -X ...'`). The release workflow and `scripts/test-install.sh` both call it, so the local test proves the format the release ships. The tag is the version: it is stamped into the binary with `-X github.com/upex-galaxy/jitso/internal/version.Version=<tag>`, and `jitso version` prints it. The decisions behind this layout are in `docs/decisions.md` (rows on release assets, the plain `go build` matrix, the install script and the Homebrew formula).

## 2. Version policy

| # | Question | Recommendation |
|---|---|---|
| OWNER | Which version numbers does JITSO use? | SemVer tags `vMAJOR.MINOR.PATCH`, release candidates as `-rc.N`. Start at `v0.1.0` and stay in `0.x` while the contract can still change. Move to `v1.0.0` after the attended real Orca run. Never reach `v2` without moving the module path to `/v2`: `go install` needs it. |

This is the owner's call; until it is answered, `package.sh` accepts any tag of the form `v<digits>.<digits>.<digits>` with an optional suffix, and the workflow treats a tag with a `-` as a prerelease.

Whatever the policy, a tag is never moved, deleted and reused, and never force-pushed. The Go module proxy and checksum database keep the first content they saw for a version for good (predicted to apply once the repository is public), so a bad tag is replaced by a new version (section 6).

## 3. Pre-flight, local

These steps run on this machine and change nothing outside the repository and your scratch folder, except the steps marked **OWNER-GATED** (they reach the network or need a download). Build into a scratch folder, never into the repo (`dist/` is ignored, but keep the habit).

1. Clean tree on `main`: `git status --porcelain` prints nothing. The scripts keep their executable bit: `git ls-files -s scripts` shows mode `100755`.
2. The verify gates in both modules, as AGENTS.md section 6 gives them: `go test ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`, from the repo root and from `contract/runner`.
3. The full contract suite against the built binary (`contract/README.md`, "Run it", prints the expected summary line), and against the oracle while its checkout exists.
4. Package every target: `scripts/package.sh v<x> <scratch>/dist`. Check the archives against `checksums.txt` with `shasum -a 256 -c checksums.txt` (or `sha256sum -c`) inside that folder.
5. Measure each stripped binary with `ls -l` and compare with the previous release's numbers. Those numbers live in `.session/`, not in a committed file. A noticeable growth goes to the owner with before and after labelled `measured`, as `.agents/skills/go-development/references/builds.md` asks.
6. `scripts/test-install.sh <scratch>/dist v<x>` (the archives just built) or `scripts/test-install.sh` (it builds the host target itself). Every check prints `ok`. It runs in throwaway folders and never touches your HOME.
7. `scripts/fill-formula.sh <x without v> <scratch>/dist/checksums.txt <scratch>/jitso.rb`, then `ruby -c <scratch>/jitso.rb`. **OWNER-GATED**: `brew style` and `brew audit` need Homebrew and may reach the network. Whether `brew audit` flags the explicit `version` line as redundant with the URLs is predicted, not measured.
8. **OWNER-GATED** Resolve the action SHAs in `.github/workflows/release.yml`. Every `uses:` line carries a placeholder (forty zeros) and a `PLACEHOLDER` comment; replace each with the full commit SHA of a release tag of that action and review the tag. This is a network read; a command such as `gh api repos/actions/checkout/git/ref/tags/<tag> --jq .object.sha` does it (an annotated tag needs one more dereference). Check that none is left: `grep -c '0000000000000000000000000000000000000000' .github/workflows/release.yml` prints 0.
9. The toolchain pin `GO_VERSION` in the workflow matches the toolchain you verified with (`go version`), and `go.mod` says what you intend: its `go` line is a language version, not a toolchain.
10. **OWNER-GATED** The contract suite has to pass on Linux at least once before the first tag. A container or a VM is the owner's call, since it may mean a download. The first tag would otherwise be the suite's first Linux run.
11. Release notes: the workflow asks GitHub to generate them. Read them after the release, and say in the notes which earlier version to skip when one is bad.

## 4. Publishing, in order

Every step in this list is **OWNER-GATED**. Do not run one without the owner's say in the conversation, and stop at the first failure.

| # | Step | Notes |
|---|---|---|
| 1 | **OWNER-GATED** Create the GitHub repository `upex-galaxy/jitso` | Visibility is the owner's call. Recommendation: public, so `go install` and the module proxy work without `GOPRIVATE`. |
| 2 | **OWNER-GATED** Add the remote | Write the push policy into AGENTS.md section 7 first (rule 4 and section 7 say it goes there before the first push). |
| 3 | **OWNER-GATED** Push `main` | No force-push, ever. |
| 4 | **OWNER-GATED** Create the annotated tag `v<x>` on the verified commit and push it | The tag push starts the workflow. |
| 5 | **OWNER-GATED** Run and watch the release workflow | The workflow verifies both modules, builds the archives, runs the contract on the Linux binary, runs the install test, then the `release` job creates the GitHub Release. If anything fails, nothing is published: the `release` job needs the `build` job. |
| 6 | **OWNER-GATED** Verify the published release from a clean machine or container | Download `checksums.txt` and one archive, `shasum -a 256 -c` it, run `jitso version`. Then download `install.sh` from the release, read it, and run `sh install.sh v<x>`. |
| 7 | **OWNER-GATED** Create the tap repository `upex-galaxy/homebrew-tap` and publish the formula | Fill the formula from the release's own `checksums.txt` (section 3, step 7), commit it as `Formula/jitso.rb`, push. |
| 8 | **OWNER-GATED** `brew install upex-galaxy/tap/jitso` and `brew test jitso` on macOS | Proves the formula, including the single top folder of the archive (predicted to stage correctly). |
| 9 | **OWNER-GATED** Announce | Where and what is the owner's call. |

The workflow uses no configured secret: `github.token` is the automatic token, scoped by the `permissions` blocks (`contents: read` for the build job, `contents: write` only for the release job, which checks out no code).

## 5. How people install it

| Route | Command or action | Notes |
|---|---|---|
| Install script | `sh install.sh v<x>` from the release's own `install.sh`, or `latest` | Installs into `$HOME/.local/bin` unless `JITSO_INSTALL_DIR` says otherwise, never with `sudo`. It checks the SHA-256 of the archive against `checksums.txt` and fails closed when neither `sha256sum` nor `shasum` exists. Pin a tag in documentation. |
| Homebrew | `brew install upex-galaxy/tap/jitso` | After step 7 of section 4. |
| `go install` | `go install github.com/upex-galaxy/jitso/cmd/jitso@v<x>` | Needs Go at the `go.mod` language version or newer. Works once the repository is public and the tag is pushed; a private repository needs `GOPRIVATE=github.com/upex-galaxy/*` and git credentials. Such a build reports the tag as its version and `commit unknown`. |
| Windows | download the zip and `checksums.txt` by hand, check the SHA-256, put `jitso.exe` on `PATH` | See section 7. |

The hook that `jitso setup hook` will register runs the bare word `jitso`, so the install folder has to be on `PATH`; the install script says so when it is not.

## 6. Recovery from a bad release

Never re-tag, never force-push, never rewrite a pushed history.

1. **OWNER-GATED** Mark the release a prerelease, or remove its assets, with `gh release edit` or `gh release delete-asset`. The release page and the install script's `latest` link then stop offering the bad version.
2. Ship a fixed patch version through the whole procedure.
3. In that patch, add `retract v<x> // <reason>` to `go.mod`, so `go install ...@latest` skips the bad version.
4. **OWNER-GATED** Revert the tap formula commit, or push the fixed formula.
5. Say in the release notes of the fix which version to skip.

## 7. Windows

The release has zips for `amd64` and `arm64`, marked "not tested" until the owner runs one (`docs/decisions.md`, "Other decisions"). The install script refuses Windows with a message that points to the zip: its job needs `uname`, `tar` and a POSIX shell. `jitso launch` and the dispatcher need `/bin/sh` and do not run on Windows. `jitso version` and the commands that only read files are what a Windows build can be expected to do.

The workflow does not run any Windows job. A `windows-latest` job that runs `jitso.exe version` would cost runner minutes and is the owner's call.

## 8. Known limits

| Limit | Why |
|---|---|
| Release integrity is checksums from the same server as the archives | OWNER question, recommendation: accept for the first release; add GitHub artifact attestations (`actions/attest-build-provenance`, `gh attestation verify`) or minisign in a later one |
| The binaries are not signed or notarized | A binary downloaded through a browser on macOS may be quarantined by Gatekeeper; `curl` and the install script do not set the attribute (predicted, not measured). The install script copies the binary through a temp file and `mv`, which is what replaces a running binary atomically |
| Archives are not byte-reproducible | `tar` and `zip` write metadata; the binaries are meant to be reproducible for the same toolchain, flags and commit (predicted) |
| The workflow has never run | the SHAs are placeholders, and `gh` and `zip` being present on the runner image is predicted, not measured |
