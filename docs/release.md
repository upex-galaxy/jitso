# Release procedure

How a JITSO release is built, checked and published. Everything that leaves this machine is marked **OWNER-GATED**: it waits for the owner's say in the conversation (AGENTS.md rule 4), and no agent runs it on its own. The local steps change nothing outside this repository and the scratch folder you build into.

The repository `upex-galaxy/jitso` is public (decided by the owner on 2026-11-15). Every publishing step is OWNER-GATED.

What has been published is recorded by the tags and GitHub Releases of the repository. The files this procedure uses are `scripts/package.sh`, `scripts/install.sh`, `scripts/install.ps1`, `scripts/test-install.sh`, `scripts/test-install.ps1`, `.github/workflows/ci.yml` (section 9) and `.github/workflows/release.yml`, which a pushed tag runs.

Committed prose names the source of truth, not its value (AGENTS.md rule 12), so this file has no sizes, counts or dates: it says which file or command holds each one.

## 1. What a release is

A release is a tag `v<x>` on `main` and a GitHub Release of that tag. Its assets, with no version in the names:

| Asset | What it is |
|---|---|
| `jitso-darwin-<arch>.tar.gz`, `jitso-linux-<arch>.tar.gz` | one folder `jitso-<os>-<arch>/` with `jitso`, `LICENSE` and `README.md`; `<arch>` is `amd64` or `arm64` |
| `jitso-windows-<arch>.zip` | the same folder with `jitso.exe` |
| `checksums.txt` | one line per asset above, `install.sh` and `install.ps1`, in the output format of `sha256sum` |
| `install.sh` | a copy of `scripts/install.sh` (macOS, Linux) |
| `install.ps1` | a copy of `scripts/install.ps1` (Windows) |

`scripts/package.sh` is the only definition of these names and of the build flags (`CGO_ENABLED=0`, `-trimpath`, `-ldflags '-s -w -X ...'`). The release workflow and `scripts/test-install.sh` both call it, so the local test proves the format the release ships. The tag is the version: it is stamped into the binary with `-X github.com/upex-galaxy/jitso/internal/version.Version=<tag>`, and `jitso version` prints it. The decisions behind this layout are in `docs/decisions.md` (rows on release assets, the plain `go build` matrix and the install script). Homebrew is deferred (`docs/decisions.md`, "The v0.1.0 cut").

## 2. Version policy

Decided by the owner on 2026-11-15 (`docs/decisions.md`).

| Item | Policy |
|---|---|
| Tags | SemVer `vMAJOR.MINOR.PATCH`; release candidates are `vMAJOR.MINOR.PATCH-rc.N` |
| First tag | `v0.1.0` |
| `0.x` | stay in `0.x` while the contract can still change |
| `v1.0.0` | only after an attended real Orca run |
| `v2` | never without moving the module path to `/v2`: `go install` needs it |

`package.sh` accepts any tag of the form `v<digits>.<digits>.<digits>` with an optional suffix, and the workflow treats a tag with a `-` as a prerelease.

Whatever the policy, a tag is never moved, deleted and reused, and never force-pushed. The Go module proxy and checksum database keep the first content they saw for a version for good (predicted to apply once the repository is public), so a bad tag is replaced by a new version (section 6).

## 3. Pre-flight, local

These steps run on this machine and change nothing outside the repository and your scratch folder, except the steps marked **OWNER-GATED** (they reach the network or need a download). Build into a scratch folder, never into the repo (`dist/` is ignored, but keep the habit).

1. Clean tree on `main`: `git status --porcelain` prints nothing. The scripts keep their executable bit: `git ls-files -s scripts` shows mode `100755`.
2. The verify gates in both modules, as AGENTS.md section 6 gives them: `go test ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`, from the repo root and from `contract/runner`.
3. The full contract suite against the built binary (`contract/README.md`, "Run it", prints the expected summary line).
4. Package every target: `scripts/package.sh v<x> <scratch>/dist`. Check the archives against `checksums.txt` with `shasum -a 256 -c checksums.txt` (or `sha256sum -c`) inside that folder.
5. Measure each stripped binary with `ls -l` and compare with the previous release's numbers. Those numbers live in `.session/`, not in a committed file. A noticeable growth goes to the owner with before and after labelled `measured`, as `.agents/skills/go-development/references/builds.md` asks.
6. `scripts/test-install.sh <scratch>/dist v<x>` (the archives just built) or `scripts/test-install.sh` (it builds the host target itself). Every check prints `ok`. It runs in throwaway folders and never touches your HOME.
7. **OWNER-GATED** Resolve the action SHAs in `.github/workflows/release.yml` and `.github/workflows/ci.yml`; both files pin the same SHAs. Every `uses:` line is pinned to the full commit SHA of a release tag of that action, with the tag as a comment; to bump one, resolve the new tag's SHA and review the tag. This is a network read; a command such as `gh api repos/actions/checkout/git/ref/tags/<tag> --jq .object.sha` does it (an annotated tag needs one more dereference). Check that none is left: `grep -c '0000000000000000000000000000000000000000' .github/workflows/release.yml .github/workflows/ci.yml` prints 0 for each file.
8. The toolchain pin `GO_VERSION` in the workflow matches the toolchain you verified with (`go version`), and `go.mod` says what you intend: its `go` line is a language version, not a toolchain.
9. **OWNER-GATED** The contract suite has to pass on Linux at least once before the first tag. Once `ci.yml` is on `main`, its green run on the commit you tag is that proof (section 9). A container or a VM is the owner's call, since it may mean a download. The first tag would otherwise be the suite's first Linux run.
10. Release notes: the workflow asks GitHub to generate them. Read them after the release, and say in the notes which earlier version to skip when one is bad.

## 4. Publishing, in order

Every step in this list is **OWNER-GATED**. Do not run one without the owner's say in the conversation, and stop at the first failure. His OK to push `main` for a release covers steps 3 to 6 in one go, as AGENTS.md section 7 says: the agent pushes, waits for `ci.yml` on that commit, tags, watches the workflow and verifies the release. Step 7 stays his.

| # | Step | Notes |
|---|---|---|
| 1 | **OWNER-GATED** Create the GitHub repository `upex-galaxy/jitso` | Public (decided 2026-11-15), so `go install` and the module proxy work without `GOPRIVATE`. Done with the owner present. |
| 2 | **OWNER-GATED** Add the remote | Write the push policy into AGENTS.md section 7 first (rule 4 and section 7 say it goes there before the first push). |
| 3 | **OWNER-GATED** Push `main` | No force-push, ever. |
| 4 | **OWNER-GATED** Create the annotated tag `v<x>` on the verified commit and push it | The tag push starts the workflow. |
| 5 | **OWNER-GATED** Run and watch the release workflow | The workflow first runs `ci.yml` on the tagged commit (section 9); then it verifies both modules, builds the archives, runs the contract on the Linux binary, runs the install test, then the `release` job creates the GitHub Release. If anything fails, nothing is published: the `build` job needs the `ci` job and the `release` job needs the `build` job. |
| 6 | **OWNER-GATED** Verify the published release from a clean machine or container | Download `checksums.txt` and one archive, `shasum -a 256 -c` it, run `jitso version`. Then download `install.sh` from the release, read it, and run `sh install.sh v<x>`. |
| 7 | **OWNER-GATED** Announce | Where and what is the owner's call. |

The workflow uses no configured secret: `github.token` is the automatic token, scoped by the `permissions` blocks (`contents: read` for the build job and for every CI job, `contents: write` only for the release job, which checks out no code).

## 5. How people install it

| Route | Command or action | Notes |
|---|---|---|
| Install script | `sh install.sh v<x>` from the release's own `install.sh`, or `latest` | Installs into `$HOME/.local/bin` unless `JITSO_INSTALL_DIR` says otherwise, never with `sudo`. It checks the SHA-256 of the archive against `checksums.txt` and fails closed when neither `sha256sum` nor `shasum` exists. Pin a tag in documentation. |
| `go install` | `go install github.com/upex-galaxy/jitso/cmd/jitso@v<x>` | Needs Go at the `go.mod` language version or newer. Works once the repository is public and the tag is pushed; a private repository needs `GOPRIVATE=github.com/upex-galaxy/*` and git credentials. Such a build reports the tag as its version and `commit unknown`. |
| Windows | `install.ps1` from the release, or the zip and `checksums.txt` by hand | `launch` does not run there. See section 7. |

The hook that `jitso setup hook` will register runs the bare word `jitso`, so the install folder has to be on `PATH`; the install script says so when it is not.

## 6. Recovery from a bad release

Never re-tag, never force-push, never rewrite a pushed history.

1. **OWNER-GATED** Mark the release a prerelease, or remove its assets, with `gh release edit` or `gh release delete-asset`. The release page and the install script's `latest` link then stop offering the bad version.
2. Ship a fixed patch version through the whole procedure.
3. In that patch, add `retract v<x> // <reason>` to `go.mod`, so `go install ...@latest` skips the bad version.
4. Say in the release notes of the fix which version to skip.

## 7. Windows

The release has zips for `amd64` and `arm64` and `install.ps1`. In PowerShell 5.1 or 7, download `install.ps1` from the release, read it, and run `.\install.ps1 [v<x>]`. It never needs administrator rights. It downloads the zip and `checksums.txt`, checks the SHA-256 (with .NET, so it needs no module), unpacks the zip and puts `jitso.exe` in `%LOCALAPPDATA%\Programs\jitso`, then appends that folder to the user `PATH` once. Its variables are the ones of `install.sh`, under the same names: `JITSO_VERSION`, `JITSO_INSTALL_DIR`, `JITSO_DOWNLOAD_URL`, `JITSO_ARCH`, `JITSO_NO_MODIFY_PATH=1`. To install by hand instead: check the zip against its line in `checksums.txt` (`certutil -hashfile <zip> SHA256`), unpack it and put the folder with `jitso.exe` on `PATH`. `install.sh` refuses Windows and points to `install.ps1`.

`jitso launch` needs `/bin/sh` and does not run on Windows: a real launch exits 3 with one line, and `--dry-run` still works. Every other command is expected to work. `jitso doctor` prints `warn shell` there and still exits 0 when nothing else fails. The decision and its why are in `docs/decisions.md` (section v0.3, the Windows rows).

The contract suite runs on Windows with `--skip-tag posix`: a case tagged `posix` writes POSIX path syntax into an expected value or a `PATH`, executes `run.sh`, or goes through `jitso launch`. Read `contract/SPEC.md` for the tag. The rest of the suite is what proves Windows. A case needs a Windows fix before it loses the tag.

The release workflow runs no Windows job of its own; it needs `ci.yml` (section 9), which runs Windows on every push.

## 8. Known limits

| Limit | Why |
|---|---|
| Release integrity is checksums from the same server as the archives | Decided by the owner on 2026-11-15: checksums only for release 1; GitHub artifact attestations (`actions/attest-build-provenance`, `gh attestation verify`) or minisign can come in a later release |
| The binaries are not signed or notarized | A binary downloaded through a browser on macOS may be quarantined by Gatekeeper; `curl` and the install script do not set the attribute (predicted, not measured). The install script copies the binary through a temp file and `mv`, which is what replaces a running binary atomically |
| Archives are not byte-reproducible | `tar` and `zip` write metadata; the binaries are meant to be reproducible for the same toolchain, flags and commit (predicted) |
| An old launch log is not read | The launch log is schema v2 and skips every `v: 1` line (the TypeScript-era log, with the key `preset`), so a project that still has running launches at the first run of this CLI must close or reap them with the old CLI first: the old lines stay in `.jitso/launches.jsonl`, `jitso doctor` counts them, and a worker that a still-running old launch started is reported as unlogged. Moving the old file aside (`mv .jitso/launches.jsonl .jitso/launches.v1.jsonl`) is safe. Defect N-31 |

## 9. Continuous integration

`.github/workflows/ci.yml` runs on every push to `main`, on every pull request, and as a reusable workflow (`workflow_call`) that the release workflow calls first. It has no secret and no write permission (`permissions: {}` at the top, `contents: read` per job), and one concurrency group per ref: a newer push to a pull request cancels the older run; on `main` and in a release a running job is not cancelled, though GitHub drops a run that is still pending when a newer one joins the group.

| Job | Runs on | What it checks |
|---|---|---|
| `verify` | `ubuntu-24.04`, `macos-15`, `windows-2025` (all required) | `go test` and `go vet` in both modules (`gofmt` on Linux only), builds the runner and `jitso`, runs the contract suite (on Windows with `--skip-tag posix`, section 7), and on Linux cross-builds every release target through `scripts/package.sh` |
| `install` | `ubuntu-24.04`, `macos-15` | `scripts/package.sh` into a temp folder, then `scripts/test-install.sh` against those archives, as the release does |
| `dist`, `install-windows` | `ubuntu-24.04`, then `windows-2025` | `dist` builds the archives once and uploads them; `install-windows` runs `scripts/test-install.ps1` against them in Windows PowerShell 5.1 and in PowerShell 7 |
| `no-orca` | `ubuntu-24.04` | on a machine with no `orca`, `jitso doctor` and `jitso launch` exit 3 with the install hint, and the launch writes nothing |

The release depends on it: the `build` job of `release.yml` needs the `ci` job, which runs `ci.yml` on the tagged commit, so a tag whose commit fails CI builds and publishes nothing. The CI run uses the version `v0.0.0-ci` for its archives; nothing it builds is kept.
