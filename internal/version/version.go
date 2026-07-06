package version

import (
	"fmt"
	"runtime/debug"
)

// Version is injected at build time via -ldflags; defaults to "dev" for local builds.
var Version = "dev"

// Info holds version and VCS metadata for a binary.
type Info struct {
	Version   string
	Commit    string
	BuildTime string
	Dirty     bool
}

// Get returns version and build info for the running binary.
func Get() Info {
	info := Info{Version: Version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			commit := s.Value
			if len(commit) > 8 {
				commit = commit[:8]
			}
			info.Commit = commit
		case "vcs.time":
			info.BuildTime = s.Value
		case "vcs.modified":
			info.Dirty = s.Value == "true"
		}
	}
	return info
}

func (i Info) String() string {
	if i.Commit == "" {
		return i.Version
	}
	dirty := ""
	if i.Dirty {
		dirty = "-dirty"
	}
	return fmt.Sprintf("%s-%s%s (%s)", i.Version, i.Commit, dirty, i.BuildTime)
}
