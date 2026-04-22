// Package version exposes build-time version metadata, injected via ldflags
// in the release pipeline.
package version

import "fmt"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func String() string {
	return fmt.Sprintf("homecast %s (commit: %s, built: %s)", Version, Commit, Date)
}
