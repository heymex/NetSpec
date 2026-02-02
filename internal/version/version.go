package version

// These variables are set at build time using ldflags
var (
	// Version is the semantic version (e.g., "1.0.0")
	Version = "dev"
	// Commit is the git commit hash
	Commit = "unknown"
	// BuildDate is the build timestamp
	BuildDate = "unknown"
)

// GetVersion returns the version string
func GetVersion() string {
	return Version
}

// GetCommit returns the commit hash
func GetCommit() string {
	return Commit
}

// GetBuildDate returns the build date
func GetBuildDate() string {
	return BuildDate
}

// GetFullVersion returns a formatted version string
func GetFullVersion() string {
	if Version == "dev" {
		return "dev (commit: " + Commit + ")"
	}
	return Version + " (commit: " + Commit + ")"
}
