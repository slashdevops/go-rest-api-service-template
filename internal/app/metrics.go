package app

import (
	"log/slog"
	"time"
)

// initMetrics holds timing information for initialization phases
type initMetrics struct {
	startTime time.Time
	phases    map[string]time.Duration
}

// newInitMetrics creates a new initialization metrics tracker
func newInitMetrics() *initMetrics {
	return &initMetrics{
		startTime: time.Now(),
		phases:    make(map[string]time.Duration),
	}
}

// recordPhase records the duration of an initialization phase
func (m *initMetrics) recordPhase(name string, duration time.Duration) {
	m.phases[name] = duration
	slog.Info("initialization phase completed",
		"phase", name,
		"duration_ms", duration.Milliseconds(),
	)
}

// logSummary logs a summary of all initialization phases
func (m *initMetrics) logSummary() {
	totalDuration := time.Since(m.startTime)

	slog.Info("application initialization completed",
		"total_duration_ms", totalDuration.Milliseconds(),
		"phases", len(m.phases),
	)

	// Log individual phases sorted by duration (longest first)
	for name, duration := range m.phases {
		percentage := float64(duration.Milliseconds()) / float64(totalDuration.Milliseconds()) * 100
		slog.Debug("initialization phase detail",
			"phase", name,
			"duration_ms", duration.Milliseconds(),
			"percentage", percentage,
		)
	}
}

// logDatabasePoolStats logs database connection pool statistics
func (a *App) logDatabasePoolStats() {
	if a.dbPool == nil {
		return
	}

	stat := a.dbPool.Stat()
	slog.Info("database connection pool statistics",
		"total_conns", stat.TotalConns(),
		"idle_conns", stat.IdleConns(),
		"acquired_conns", stat.AcquiredConns(),
		"constructing_conns", stat.ConstructingConns(),
		"max_conns", stat.MaxConns(),
		"acquire_count", stat.AcquireCount(),
		"acquire_duration_ns", stat.AcquireDuration().Nanoseconds(),
		"empty_acquire_count", stat.EmptyAcquireCount(),
		"canceled_acquire_count", stat.CanceledAcquireCount(),
	)
}

// StartupMetrics holds metrics about application startup
type StartupMetrics struct {
	startTime            time.Time     // Application start time (not exported in JSON)
	TotalDuration        time.Duration `json:"total_duration_ms"`
	ConfigLoadDuration   time.Duration `json:"config_load_duration_ms"`
	TelemetryDuration    time.Duration `json:"telemetry_duration_ms"`
	DatabaseDuration     time.Duration `json:"database_duration_ms"`
	RepositoriesDuration time.Duration `json:"repositories_duration_ms"`
	MailServiceDuration  time.Duration `json:"mail_service_duration_ms"`
	ServicesDuration     time.Duration `json:"services_duration_ms"`
	HandlersDuration     time.Duration `json:"handlers_duration_ms"`
	HTTPServerDuration   time.Duration `json:"http_server_duration_ms"`
}

// GetStartupMetrics returns the startup metrics for observability
// This can be exposed via a metrics endpoint or used for monitoring
func (a *App) GetStartupMetrics() *StartupMetrics {
	// This would be populated during initialization
	// For now, return empty struct - to be enhanced in future
	return &StartupMetrics{}
}
