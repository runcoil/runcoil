package build

import "runtime"

// Set with -ldflags "-X" at build time.
var (
	Version   = "dev"
	Commit    = ""
	BuildDate = ""
)

// Info describes the running binary.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
	GoVersion string
}

// Get returns the build information of the running binary.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}
}
