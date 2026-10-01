// Package buildinfo reports RapidGo's build and module version metadata.
package buildinfo

import (
	"runtime/debug"

	"github.com/hangxie/rapidgo/internal/i18n"
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
	return i18n.Format("msg_s_commit_s_built_s", version, Commit, Date)
}

func moduleVersion(version string, info *debug.BuildInfo) string {
	if info != nil && version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}
