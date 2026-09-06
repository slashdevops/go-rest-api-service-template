package domain

// Status represents the health status of a service.
//
//	@Description	Health status of a service.
type Status bool

// String returns the string representation of the status.
func (val Status) String() string {
	if val == StatusUp {
		return "UP"
	}
	return "DOWN"
}

// Health statuses enumeration.
//
//	@Description	Health statuses enumeration.
const (
	StatusUp   Status = true
	StatusDown Status = false
)

// Check is one component's verdict on the PUBLIC status endpoint.
//
// Status and nothing else, deliberately. This carried a `data` map until
// go-rest-api-service-template#391 moved the runtime detail to /health/detailed, which
// requires a token; the field survived that change unpopulated and without
// `omitempty`, so every check answered `"data": null` forever and the published
// schema advertised a free-form object no caller could ever receive.
//
// Do not add it back. /health/status is unauthenticated AND exempt from the
// rate limiter, which is the pair that makes anything carried here a cheap
// amplifier as well as a disclosure -- the reason the detail moved in the first
// place. Per-component detail belongs on [ComponentHealth], which is only
// reachable with a token and marks its Details `omitempty`.
//
//	@Description	One component's health verdict. Status only: this endpoint is public
type Check struct {
	Name   string `json:"name" example:"database" format:"string"`
	Kind   string `json:"kind" example:"database" format:"string"`
	Status Status `json:"status" example:"True" format:"string"`
}

// Health represents a health check.
//
//	@Description	Health check of the service.
type Health struct {
	Checks []Check `json:"checks"`
	Status Status  `json:"status" example:"True" format:"string"`
}

// ComponentHealth represents the health status of an individual component.
//
//	@Description	Health status of an individual component.
type ComponentHealth struct {
	Details      map[string]any `json:"details,omitempty"`
	Status       string         `json:"status" example:"healthy" format:"string"`
	Message      string         `json:"message,omitempty" example:"Database is reachable" format:"string"`
	ResponseTime string         `json:"response_time,omitempty" example:"2.5ms" format:"string"`
}

// DatabasePoolStats represents database connection pool statistics.
//
//	@Description	Database connection pool statistics.
type DatabasePoolStats struct {
	AcquiredConnections     int32 `json:"acquired_connections" example:"5"`
	IdleConnections         int32 `json:"idle_connections" example:"10"`
	MaxConnections          int32 `json:"max_connections" example:"20"`
	TotalConnections        int32 `json:"total_connections" example:"15"`
	ConstructingConnections int32 `json:"constructing_connections" example:"0"`
	EmptyAcquireCount       int64 `json:"empty_acquire_count" example:"100"`
	AcquireCount            int64 `json:"acquire_count" example:"150"`
	AcquireDuration         int64 `json:"acquire_duration_ns" example:"5000000"`
	CanceledAcquireCount    int64 `json:"canceled_acquire_count" example:"2"`
}

// StartupMetrics represents application startup timing metrics.
//
//	@Description	Application startup timing metrics.
type StartupMetrics struct {
	PhaseTimings    map[string]any `json:"phase_timings"`
	PhasePercentage map[string]any `json:"phase_percentage"`
	TotalTime       string         `json:"total_time" example:"1.5s" format:"string"`
}

// DetailedHealth represents comprehensive health information.
//
//	@Description	Comprehensive health information including component status and metrics.
//
// BuildInfo identifies the running binary. It is reported on the
// authenticated detailed health and nowhere anonymous: the commit, branch and
// Go version let a caller match the build against published advisories, which
// is why they left GET /version.
type BuildInfo struct {
	Version       string `json:"version" example:"1.0.0" format:"string"`
	BuildDate     string `json:"build_date" example:"2021-01-01T00:00:00Z" format:"string"`
	GitCommit     string `json:"git_commit" example:"abcdef123456" format:"string"`
	GitBranch     string `json:"git_branch" example:"main" format:"string"`
	GoVersion     string `json:"go_version" example:"go1.27.1" format:"string"`
	GoVersionArch string `json:"go_version_arch" example:"arm64" format:"string"`
	GoVersionOS   string `json:"go_version_os" example:"linux" format:"string"`
}

type DetailedHealth struct {
	Components     map[string]ComponentHealth `json:"components"`
	Build          BuildInfo                  `json:"build"`
	DatabasePool   *DatabasePoolStats         `json:"database_pool,omitempty"`
	StartupMetrics *StartupMetrics            `json:"startup_metrics,omitempty"`
	Status         string                     `json:"status" example:"healthy" format:"string"`
	Version        string                     `json:"version" example:"1.0.0" format:"string"`
	Uptime         string                     `json:"uptime" example:"2h30m45s" format:"string"`
}
