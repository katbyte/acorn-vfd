// Package version exposes the acornvfd version, set at build time or derived from module build info.
package version

import "runtime/debug"

// Version is set by the linker at release time, or taken from module build info.
var Version = "dev"

// GitCommit is set by the linker at build time.
var GitCommit string

func init() {
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" && info.Main.Version != "" {
			Version = info.Main.Version
		}
	}
}
