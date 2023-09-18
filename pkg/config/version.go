package config

import (
	"fmt"
	"runtime/debug"
	"strconv"
)

// RawVersion is set via build flags.
var RawVersion = "dev"

// getVersion is a function that returns a string representation of the
// application's version. This is achieved by reading the build information from
// the binary and extracting the git short commit SHA and the modified flag. The
// version string is in the format "v<RawVersion>-<shortCommit>", where
// <RawVersion> is constant given at build time, and <shortCommit> is the first
// 7 characters of the git commit SHA with "+dirty" appended if there were
// uncommitted changes at the time of the build.
//
// The function will panic if the vcs.modified key from the build settings can't
// be parsed into a boolean value.
//
// Example: getVersion() could return 'v1.0.0-a1b2c3d+dirty', assuming
// RawVersion is '1.0.0', the commit SHA starts with 'a1b2c3d' and there were
// uncommitted changes.
func getVersion() string {
	var (
		err         error
		isDirty     bool
		shortCommit string
	)

	// read git commit sha and modified flag from go build information
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, bs := range bi.Settings {
			switch bs.Key {
			case "vcs.revision":
				shortCommit = bs.Value
				if len(bs.Value) >= 7 {
					shortCommit = bs.Value[:7]
				}
			case "vcs.modified":
				isDirty, err = strconv.ParseBool(bs.Value)
				if err != nil {
					panic(fmt.Sprintf("couldn't parse vcs.revision: %s", err))
				}
			}
		}
	}

	if isDirty {
		shortCommit += "+dirty"
	}

	return fmt.Sprintf("v%s-%s", RawVersion, shortCommit)
}
