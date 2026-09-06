package payload

// Version is the struct that holds the version information.
//
//	@Description	Version is the struct that holds the version information.
type Version struct {
	Version string `json:"version" example:"1.0.0" format:"string"`
	// The build details are present only on the authenticated
	// /health/detailed answer. GET /version used to hand an anonymous caller
	// the commit, the branch and the Go version -- the same disclosure that
	// was removed from /health/status -- and it is exempt from rate limiting.
	BuildDate     string `json:"build_date,omitempty" example:"2021-01-01T00:00:00Z" format:"string"`
	GitCommit     string `json:"git_commit,omitempty" example:"abcdef123456" format:"string"`
	GitBranch     string `json:"git_branch,omitempty" example:"main" format:"string"`
	GoVersion     string `json:"go_version,omitempty" example:"go1.24.1" format:"string"`
	GoVersionArch string `json:"go_version_arch,omitempty" example:"amd64" format:"string"`
	GoVersionOS   string `json:"go_version_os,omitempty" example:"linux" format:"string"`
}
