# Builds per OS and the size budget

## Cross-compiling

Release 1 targets macOS, Linux and Windows (`docs/decisions.md`, "Other decisions"). Pure Go cross-compiles with two environment variables; keep cgo off so it stays that way:

```sh
out=<scratch dir>
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o "$out/jitso-darwin-arm64" .
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o "$out/jitso-linux-amd64" .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$out/jitso-windows-amd64.exe" .
```

A successful cross-compile proves that the build constraints are complete (every OS has an implementation of each OS-specific function). It does not prove the binary works there: Windows stays "not tested" until the owner runs it. `go tool dist list` prints every valid `GOOS/GOARCH` pair.

## Size budget

JITSO exists partly because the compiled TypeScript CLI measured about 85 MB. The reference point for Go is a stdlib-only probe (json, regexp, os/exec) measured on 2026-11-14: about 4.2 MB with default flags and about 3 MB stripped, per OS. Those numbers are history, not a live value: measure again rather than quoting them.

Measure on every new dependency and before a release:

```sh
go build -ldflags='-s -w' -o <scratch dir>/jitso . && ls -l <scratch dir>/jitso
```

`-s -w` drops the symbol table and DWARF data. Report a change that grows the stripped binary noticeably to the owner with the before and after numbers, labelled `measured`.

## Where builds go

Into a scratch directory outside the repo, or a path `.gitignore` covers (`dist/` is ignored). Never commit a binary.
