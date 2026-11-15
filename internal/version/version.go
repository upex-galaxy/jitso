// Package version names the build of the running binary: the release version
// stamped at link time, the commit the Go toolchain recorded, and the Go
// version and platform it was built for.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Version is set at release time:
//
//	-ldflags "-X github.com/upex-galaxy/jitso/internal/version.Version=v0.1.0"
//
// When it is empty, the module version of the build is used (a `go install
// module@tag` build carries the tag), and when that is empty or "(devel)" too,
// the version is "dev".
var Version = ""

// unknownCommit is printed when the build carries no VCS stamp (a build
// outside a git checkout, or with -buildvcs=false).
const unknownCommit = "unknown"

// Line is the one line `jitso version` prints, without the newline:
//
//	jitso <version> (commit <12 hex>[-dirty]|unknown, <go version>, <goos>/<goarch>)
func Line() string {
	info, ok := debug.ReadBuildInfo()
	return line(Version, info, ok, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// line is Line over explicit inputs, so a test can give it any build info.
func line(stamped string, info *debug.BuildInfo, ok bool, goVersion, goos, goarch string) string {
	v, commit := resolve(stamped, info, ok)
	return fmt.Sprintf("jitso %s (commit %s, %s, %s/%s)", v, commit, goVersion, goos, goarch)
}

func resolve(stamped string, info *debug.BuildInfo, ok bool) (version, commit string) {
	version, commit = stamped, unknownCommit
	if !ok || info == nil {
		if version == "" {
			version = "dev"
		}
		return version, commit
	}
	if version == "" {
		version = info.Main.Version
		if version == "" || version == "(devel)" {
			version = "dev"
		}
	}
	var revision string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		commit = revision
		if modified {
			commit += "-dirty"
		}
	}
	return version, commit
}
