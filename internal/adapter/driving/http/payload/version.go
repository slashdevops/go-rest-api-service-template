package payload

// Version is the struct that holds the version information.
//
//	@Description	Version is the struct that holds the version information.
type Version struct {
	Version       string `json:"version" example:"1.0.0" format:"string"`
	BuildDate     string `json:"build_date" example:"2021-01-01T00:00:00Z" format:"string"`
	GitCommit     string `json:"git_commit" example:"abcdef123456" format:"string"`
	GitBranch     string `json:"git_branch" example:"main" format:"string"`
	GoVersion     string `json:"go_version" example:"go1.24.1" format:"string"`
	GoVersionArch string `json:"go_version_arch" example:"amd64" format:"string"`
	GoVersionOS   string `json:"go_version_os" example:"linux" format:"string"`
}
