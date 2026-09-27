// Package buildinfo reports RapidGo's build and module version metadata.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Describe returns the version and any injected release metadata.
func Describe() string {
	version := Version
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok {
			version = moduleVersion(version, info)
		}
	}
	if Commit == "unknown" && Date == "unknown" {
		return version
	}
	return fmt.Sprintf("%s (commit %s, built %s)", version, Commit, Date)
}

func moduleVersion(version string, info *debug.BuildInfo) string {
	if info != nil && version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
