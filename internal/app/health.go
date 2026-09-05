package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
	"github.com/slashdevops/go-rest-api-service-template/internal/version"
)

// ComponentStatus represents the health status of a component
type ComponentStatus string

const (
	ComponentStatusHealthy      ComponentStatus = "healthy"
	ComponentStatusDegraded     ComponentStatus = "degraded"
	ComponentStatusUnhealthy    ComponentStatus = "unhealthy"
	ComponentStatusInitializing ComponentStatus = "initializing"
	ComponentStatusUnknown      ComponentStatus = "unknown"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	LastChecked time.Time      `json:"last_checked"`
	Details     map[string]any `json:"details,omitempty"`
	// nil means "not measured", which is NOT the same as "took no time".
	//
	// This was `ResponseTimeMs int64`, and the converter blanked the field when
	// it was zero -- so a check that genuinely took under a millisecond was
	// reported as unmeasured. Locally every check does, which is why
	// /health/detailed never carried a single response_time.
	ResponseTime *time.Duration  `json:"response_time_ns,omitempty"`
	Name         string          `json:"name"`
	Status       ComponentStatus `json:"status"`
	Message      string          `json:"message,omitempty"`
}

// AppHealth represents the overall health of the application
type AppHealth struct {
	Timestamp     time.Time                  `json:"timestamp"`
	Components    map[string]ComponentHealth `json:"components"`
	Startup       *StartupMetrics            `json:"startup_metrics,omitempty"`
	DatabaseStats *DatabasePoolStats         `json:"database_stats,omitempty"`
	Status        ComponentStatus            `json:"status"`
	Version       string                     `json:"version"`
	Uptime        time.Duration              `json:"uptime_seconds"`
}

// DatabasePoolStats holds database connection pool statistics
type DatabasePoolStats struct {
	TotalConns           int32 `json:"total_conns"`
	IdleConns            int32 `json:"idle_conns"`
	AcquiredConns        int32 `json:"acquired_conns"`
	ConstructingConns    int32 `json:"constructing_conns"`
	MaxConns             int32 `json:"max_conns"`
	AcquireCount         int64 `json:"acquire_count"`
	AcquireDurationNs    int64 `json:"acquire_duration_ns"`
	EmptyAcquireCount    int64 `json:"empty_acquire_count"`
	CanceledAcquireCount int64 `json:"canceled_acquire_count"`
}

// GetHealth returns the current health status of the application and all components
func (a *App) GetHealth(ctx context.Context) payload.DetailedHealth {
	appHealth := a.getDetailedHealth(ctx)
	return a.convertToModelHealth(appHealth)
}

// failModeConsequence puts the operator-visible effect of a store outage into
// the health message, because ratelimit.store.fail.mode changes it completely
// and the mode alone does not say which.
func failModeConsequence(mode string) string {
	if mode == config.RateLimitFailModeLocal {
		return "falling back to per-replica limits, so the effective limit is " +
			"multiplied by the number of replicas"
	}

	return "refusing requests with 429 RATE_LIMIT_UNAVAILABLE"
}

// overallForDatabase returns the overall status a database in this state
// implies.
//
// The distinction it draws is the whole point: a SLOW database is degraded and
// still worth traffic, while an UNREACHABLE one means this service cannot
// answer anything. Collapsing both into degraded made a service with no
// database answer 206 -- a success code -- so anything treating 2xx as healthy
// kept sending it work.
//
// Note what is deliberately absent: the cache. A cache fault never reaches the
// overall status, because the cache layer is fail-open and a request survives
// losing it. See the call site.
func overallForDatabase(db ComponentStatus) ComponentStatus {
	switch db {
	case ComponentStatusHealthy:
		return ComponentStatusHealthy
	case ComponentStatusUnhealthy:
		return ComponentStatusUnhealthy
	default:
		// Degraded, unknown, or anything added later: worth reporting, still
		// worth traffic.
		return ComponentStatusDegraded
	}
}

// getDetailedHealth is the internal implementation that returns the internal AppHealth type
func (a *App) getDetailedHealth(ctx context.Context) *AppHealth {
	startTime := time.Now()

	health := &AppHealth{
		Status:     ComponentStatusHealthy,
		Timestamp:  startTime,
		Version:    version.Version,
		Components: make(map[string]ComponentHealth),
		Startup:    a.startupMetrics,
	}

	// Guard the field that is actually dereferenced.
	//
	// This read `if a.configs != nil` and then dereferenced a.startupMetrics --
	// two different fields, so the check protected nothing about the pointer it
	// was guarding. Unreachable today, because the builder always sets both,
	// but only by coincidence: it is one construction path away from a nil
	// dereference inside the endpoint an operator reaches for when things are
	// already going wrong. Found by a test that built an App directly.
	if a.startupMetrics != nil {
		health.Uptime = time.Since(a.startupMetrics.startTime)
	}

	// Check database health.
	//
	// The database is a HARD dependency: without it this service cannot answer
	// a single request, so its state has to reach the overall status honestly.
	// A slow database is degraded and still worth traffic; an unreachable one
	// is not, and saying so is the only way a load balancer or a readiness
	// probe can act on it.
	//
	// This used to collapse both into "degraded", which meant a service with no
	// database at all answered 206 — a SUCCESS code — so anything treating 2xx
	// as healthy kept sending it work it could not do. The 503 the endpoint
	// documented was unreachable, which is how this was found.
	if a.dbPool != nil {
		dbHealth := a.checkDatabaseHealth(ctx)
		health.Components["database"] = dbHealth

		if overall := overallForDatabase(dbHealth.Status); overall != ComponentStatusHealthy {
			health.Status = overall
		}

		// Add database pool stats
		health.DatabaseStats = a.getDatabasePoolStats()
	} else {
		// No pool at all. The service is running but cannot serve, which is the
		// same thing as an unreachable database from a caller's point of view.
		health.Components["database"] = ComponentHealth{
			Name:        "database",
			Status:      ComponentStatusUnknown,
			Message:     "database not initialized",
			LastChecked: time.Now(),
		}
		health.Status = ComponentStatusUnhealthy
	}

	// Check cache health. Reported only when caching is switched on — with
	// cache.enabled=false there is no component to be unhealthy about.
	//
	// A cache fault never degrades the overall status. The layer is fail-open
	// by design: a read that times out or errors falls through to the database
	// and the request still succeeds, so failing readiness here would take a
	// working service out of rotation over an optimisation. The component
	// exists so the outage is *visible* — previously a total Valkey failure
	// showed up as nothing but higher database load and p99.
	//
	// This exemption is about THE CACHE, and only the cache. The same Valkey
	// usually also backs the rate-limit store, which fails CLOSED by default —
	// see the ratelimit_store component below, which is why losing that server
	// no longer reports an unqualified "healthy".
	if a.cacheClient != nil {
		health.Components["cache"] = a.checkCacheHealth(ctx)
	}

	// Check the rate limiter's shared counter. Reported only when there is one:
	// with rules off, or with no Valkey configured, each replica limits from
	// memory and there is no shared store to be down.
	//
	// # Why this is degraded and never unhealthy
	//
	// The store fails CLOSED by default, so when it is unreachable the service
	// refuses every request with 429 RATE_LIMIT_UNAVAILABLE. That is not a
	// working service, and reporting a bare "healthy" is what this component
	// exists to stop: measured with Valkey stopped, /health/detailed answered
	// 200 healthy while every other endpoint answered 429.
	//
	// But it must not reach 503 either. Health and version bypass the limiter
	// precisely so a store outage cannot evict a replica from the load
	// balancer; letting the same outage fail readiness would reintroduce that
	// eviction by the other door, and every replica would go at once, since
	// they share the store. Degraded says "working, but something is wrong",
	// which is the honest answer and the one a human can act on.
	//
	// The state comes from the limiter's own gauge rather than a fresh ping —
	// see [middleware.RateLimitMetrics.StoreUp] for why a second probe would be
	// asking a different question.
	if a.rateLimitShared != nil && !a.rateLimitMetrics.StoreUp() {
		health.Components["ratelimit_store"] = timeCheck(func() ComponentHealth {
			return ComponentHealth{
				Name:   "ratelimit_store",
				Status: ComponentStatusDegraded,
				Message: "the shared rate-limit counter is unreachable; with " +
					"ratelimit.store.fail.mode=" + a.configs.RateLimit.StoreFailMode.Value +
					" this replica is " + failModeConsequence(a.configs.RateLimit.StoreFailMode.Value),
				LastChecked: time.Now(),
			}
		})

		// Never worse than degraded, and never better than what the database
		// already decided.
		if health.Status == ComponentStatusHealthy {
			health.Status = ComponentStatusDegraded
		}
	} else if a.rateLimitShared != nil {
		// No response time here, and not because nothing was measured -- because
		// it was measured under another name.
		//
		// The shared counter runs on the SAME Valkey client as the cache, owned
		// by initCacheClient (see App.Shutdown, which deliberately does not
		// close it twice). So a ping here would time the identical connection
		// the `cache` component already reports, and publishing one round trip
		// as two numbers invites an operator to compare them and conclude
		// something from the difference, which would be noise.
		//
		// The status still comes from the limiter's own gauge rather than a
		// fresh ping: the gauge answers what the limiter actually experiences,
		// which is the question this component is for.
		health.Components["ratelimit_store"] = timeCheck(func() ComponentHealth {
			// A real round trip, on the client the counter actually uses.
			//
			// This is the SAME Valkey client as the cache (owned by
			// initCacheClient -- App.Shutdown deliberately does not close it
			// twice), so the two components measure one connection at two
			// moments. Both numbers are real, and neither is the other: a
			// difference between them is scheduling noise, not a fact about the
			// store, which is why `details` says so rather than leaving an
			// operator to infer it.
			//
			// The STATUS still comes from the limiter's own gauge, above. A
			// ping answers "is it reachable"; the gauge answers "is the limiter
			// succeeding", and the second is the question this component is for
			// -- a store reachable by ping while every INCR times out is
			// exactly the case the gauge exists to catch.
			pingCtx, cancel := context.WithTimeout(ctx, a.configs.RateLimit.StoreTimeout.Value)
			defer cancel()

			_ = a.cacheClient.Do(pingCtx, a.cacheClient.B().Ping().Build()).Error()

			return ComponentHealth{
				Name:        "ratelimit_store",
				Status:      ComponentStatusHealthy,
				Message:     "the shared rate-limit counter is answering",
				LastChecked: time.Now(),
				Details: map[string]any{
					"shares_connection_with": "cache",
					"status_source":          "the limiter's own gauge, not this ping",
				},
			}
		})
	}

	// Telemetry. A real assessment, not a nil check -- see
	// checkTelemetryHealth for what the nil check was hiding.
	//
	// Deliberately absent from the overall status: losing telemetry costs
	// visibility, not service.
	health.Components["telemetry"] = timeCheck(func() ComponentHealth {
		return a.checkTelemetryHealth(ctx)
	})

	// Mail. Also a real probe now: a send that fails is dropped rather than
	// retried, so an unreachable transport destroys verification email
	// silently. See checkMailHealth.
	//
	// Like the cache and telemetry, it never reaches the overall status -- no
	// request depends on mail.
	if a.mailServer != nil {
		health.Components["mail_service"] = a.checkMailHealth(ctx)
	} else {
		health.Components["mail_service"] = ComponentHealth{
			Name:        "mail_service",
			Status:      ComponentStatusUnknown,
			Message:     "mail service not initialized",
			LastChecked: time.Now(),
		}
	}

	// # The four structural components below
	//
	// http_server, repositories, services and handlers are not probes: there is
	// nothing to reach. Each asserts that a startup phase completed, and every
	// one of them is implied by this request being answered at all -- the
	// handler serving /health/detailed IS one of the handlers it reports on.
	//
	// They are kept because a MISSING component is a real signal (a phase that
	// did not run), and dropped from a payload they cannot be missing from.
	//
	// They DO carry a response time, and it is the honest one: how long the
	// check took, which for a pointer comparison is a few hundred nanoseconds.
	// Leaving it blank was reported as missing data three times, which is the
	// stronger argument -- an empty cell communicates "we failed to measure
	// this", and that is not what was happening. Their messages still say
	// "structural check, nothing is probed", and that sentence is what stops a
	// small number being read as a round trip. Do not remove it.
	//
	// If one of these ever gains something to measure, it stops being
	// structural and gets a probe like database, cache and mail_service have.
	if a.httpServer != nil {
		health.Components["http_server"] = timeCheck(func() ComponentHealth {
			return ComponentHealth{
				Name:        "http_server",
				Status:      ComponentStatusHealthy,
				Message:     "http server started; structural check, nothing is probed",
				LastChecked: time.Now(),
			}
		})
	} else {
		health.Components["http_server"] = ComponentHealth{
			Name:        "http_server",
			Status:      ComponentStatusUnknown,
			Message:     "http server not initialized",
			LastChecked: time.Now(),
		}
	}

	// Check repositories
	if a.repositories != nil {
		health.Components["repositories"] = timeCheck(func() ComponentHealth {
			return ComponentHealth{
				Name:        "repositories",
				Status:      ComponentStatusHealthy,
				Message:     "repositories wired at startup; structural check, nothing is probed",
				LastChecked: time.Now(),
			}
		})
	}

	// Check services
	if a.services != nil {
		health.Components["services"] = timeCheck(func() ComponentHealth {
			return ComponentHealth{
				Name:        "services",
				Status:      ComponentStatusHealthy,
				Message:     "services wired at startup; structural check, nothing is probed",
				LastChecked: time.Now(),
			}
		})
	}

	// Check handlers
	if a.handlers != nil {
		health.Components["handlers"] = timeCheck(func() ComponentHealth {
			return ComponentHealth{
				Name:        "handlers",
				Status:      ComponentStatusHealthy,
				Message:     "handlers registered at startup; structural check, nothing is probed",
				LastChecked: time.Now(),
			}
		})
	}

	// Add database pool stats if available
	if a.dbPool != nil {
		health.DatabaseStats = a.getDatabasePoolStats()
	}

	return health
}

// mailProbeTimeout bounds the mail reachability probe.
//
// Deliberately not mail.worker.timeout, which is the budget a real send gets
// and may be tens of seconds. /health/detailed is polled, and a health endpoint
// that can block for the length of an SMTP timeout turns a mail outage into a
// monitoring outage.
const mailProbeTimeout = 3 * time.Second

// timeCheck runs a component's check and records how long it took.
//
// # One definition for the whole column
//
// `response_time` means the same thing for every component: how long that
// component's check took. Where the check is a round trip -- database, cache,
// mail, the telemetry collector -- the round trip dominates, so the number is
// the round trip. Where the check reaches nothing, the number is the
// sub-microsecond cost of not reaching anything.
//
// This replaces a column that was populated for some components and blank for
// others, which read as missing data rather than as a distinction. It was
// reported as a bug three times.
//
// # The number is measured, never invented -- and it is not a probe
//
// A structural component's few hundred nanoseconds is a real measurement of a
// real (trivial) check. It is NOT evidence that anything was contacted, and
// nothing here should be read as claiming otherwise: the `message` on each of
// those components says "structural check, nothing is probed", and that
// sentence is what stops the number being misread. Do not remove it.
//
// The honest tell is the magnitude. A component whose check is a network round
// trip cannot report 200ns, and one that reaches nothing cannot report 3ms.
func timeCheck(build func() ComponentHealth) ComponentHealth {
	start := time.Now()
	component := build()
	elapsed := time.Since(start)

	// A check that already measured something more specific than itself keeps
	// its own number -- the database records the ping alone, which is the part
	// an operator is judging, not the microseconds of struct building around it.
	if component.ResponseTime == nil {
		component.ResponseTime = &elapsed
	}

	return component
}

// dialProbe opens a TCP connection to address and reports how long it took.
//
// Shared by the mail and telemetry components because they ask the same narrow
// question -- is this host accepting connections -- and must answer it under the
// same bound. It returns the error rather than a status so each caller can
// decide what an unreachable host means for it; they do not agree.
//
// # The budget is the probe's own, never the request's
//
// A health poll usually carries no deadline at all, so inheriting the caller's
// context means an unreachable host hangs the endpoint for the platform's TCP
// connect timeout. Measured without this: 75 seconds against a blackholed
// address, on an endpoint whose entire purpose is to be polled.
//
// Nothing is written to the connection, so there is nothing to close politely.
func (a *App) dialProbe(ctx context.Context, address string) (time.Duration, error) {
	dialCtx, cancel := context.WithTimeout(ctx, mailProbeTimeout)
	defer cancel()

	start := time.Now()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "tcp", address)
	elapsed := time.Since(start)

	if err != nil {
		return elapsed, err
	}

	_ = conn.Close()

	return elapsed, nil
}

// checkMailHealth dials the configured mail transport.
//
// # Why this is a real probe now
//
// It used to be a nil check on a.mailServer, which said "mail service running"
// whenever the object had been constructed — true of a service whose SMTP host
// has been unreachable since it started.
//
// That mattered more than it looks, because a failed send is LOST.
// mailer.MailService.deliver logs the error and returns; there is no retry and
// no dead-letter queue. So an unreachable mail host does not delay a
// verification email, it destroys it — and nothing in the request path notices,
// because sending is asynchronous. The user is told to check their inbox, the
// mail never arrives, and every health signal the service publishes says
// healthy.
//
// # Why reachability, and only reachability
//
// A TCP connect is enough to see that outage. The probe deliberately stops
// short of a full SMTP conversation or an authenticated API call: those spend
// credentials and provider quota on every poll, and they fail for reasons — a
// rejected password, a rate limit at the provider — that would be reported here
// as the transport being down. What this answers is the narrow question the
// component name implies, and the message says so rather than claiming more.
//
// # Why degraded and never unhealthy
//
// No request depends on mail. Failing readiness over an unreachable SMTP host
// would evict a replica that is serving every API call correctly. Same shape as
// the cache exemption above.
func (a *App) checkMailHealth(ctx context.Context) ComponentHealth {
	sender := a.configs.Mail.MailSender.Value

	address, err := a.mailTransportAddress()
	if err != nil {
		// Not a fault: a sender this function does not know how to reach is a
		// gap here, not a broken deployment. Say which, rather than reporting a
		// working transport as down.
		return ComponentHealth{
			Name:        "mail_service",
			Status:      ComponentStatusUnknown,
			Message:     "mail service running; no reachability probe for sender " + sender,
			LastChecked: time.Now(),
		}
	}

	responseTime, err := a.dialProbe(ctx, address)
	if err != nil {
		// The address is in the payload but the error is not. Same rule as the
		// database check: the dial error can carry resolver detail and internal
		// hostnames, and this endpoint's audience is an operator who already
		// knows the configured host.
		slog.Error(
			"mail transport unreachable, verification emails will be dropped",
			"sender", sender,
			"address", address,
			"error", err,
			"response_time", formatHealthDuration(responseTime),
		)

		return ComponentHealth{
			Name:   "mail_service",
			Status: ComponentStatusDegraded,
			Message: "cannot reach the mail transport at " + address +
				"; sends are not retried, so mail queued while this lasts is lost",
			LastChecked:  time.Now(),
			ResponseTime: &responseTime,
			Details:      map[string]any{"sender": sender, "address": address},
		}
	}

	return ComponentHealth{
		Name:         "mail_service",
		Status:       ComponentStatusHealthy,
		Message:      "mail transport reachable at " + address,
		LastChecked:  time.Now(),
		ResponseTime: &responseTime,
		Details: map[string]any{
			"sender": sender,
			// Stated because the probe's limits are the interesting part: a
			// reachable host says nothing about whether credentials are valid.
			"probe": "tcp connect only; credentials and delivery are not verified",
		},
	}
}

// mailTransportAddress returns the host:port the configured sender talks to.
func (a *App) mailTransportAddress() (string, error) {
	switch a.configs.Mail.MailSender.Value {
	case config.MailSenderSMTP:
		return net.JoinHostPort(
			a.configs.Mail.SMTPHost.Value,
			strconv.Itoa(a.configs.Mail.SMTPPort.Value),
		), nil

	case config.MailSenderMailgun:
		u, err := url.Parse(a.configs.Mail.APIURL.Value)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("mail.api.url is not a usable URL")
		}

		if u.Port() != "" {
			return u.Host, nil
		}

		port := "443"
		if u.Scheme == "http" {
			port = "80"
		}

		return net.JoinHostPort(u.Hostname(), port), nil

	default:
		return "", fmt.Errorf("unknown mail sender %q", a.configs.Mail.MailSender.Value)
	}
}

// telemetryErrorWindow is how recently an export must have failed for the
// pipeline to count as failing NOW.
//
// An exporter retries on its own schedule, so a failure is only evidence of a
// current outage if it is newer than the interval the exporter works on.
// Anything older is history: a collector that was restarted an hour ago should
// not leave this component degraded forever.
func telemetryErrorWindow(conf *config.OpenTelemetryConfig) time.Duration {
	window := max(
		2*conf.TraceExporterBatchTimeout.Value,
		2*conf.MetricInterval.Value,
		time.Minute,
	)

	return window
}

// checkTelemetryHealth reports whether telemetry is actually leaving the
// process.
//
// # What it replaces
//
// A nil check on a.telemetry, which answered "telemetry active, healthy" for
// three different situations: exporting normally; exporting to a collector that
// has been refusing connections for a week; and not exporting at all, because
// both exporters are noop. Only the first deserves that message.
//
// The second is the one an operator needs, and it is the hardest to notice from
// anywhere else — every dashboard is simply stale, and nothing says why. It is
// visible here because [o11y.ExportErrors] is installed as the SDK's global
// error handler; the batched pipelines cannot report a failure any other way.
//
// The third is not a fault at all, but reporting it as "active" is a lie an
// operator would act on. It says "disabled" now, and names the setting.
//
// # Why a failing exporter is degraded and never unhealthy
//
// Losing telemetry costs visibility, not service: every request still succeeds
// and no caller can tell. Failing readiness over it would take a working
// replica out of rotation because a collector is down — and since replicas
// share a collector, all of them at once.
func (a *App) checkTelemetryHealth(ctx context.Context) ComponentHealth {
	var responseTime *time.Duration

	if a.telemetry == nil {
		return ComponentHealth{
			Name:        "telemetry",
			Status:      ComponentStatusUnknown,
			Message:     "telemetry not initialized",
			LastChecked: time.Now(),
		}
	}

	traces := a.configs.Telemetry.TraceExporter.Value
	metrics := a.configs.Telemetry.MetricExporter.Value

	details := map[string]any{
		"trace_exporter":  traces,
		"metric_exporter": metrics,
	}

	// Reach for the collector, when there is one to reach for.
	//
	// # What this catches that the export errors do not
	//
	// With no traffic there is nothing to export, so a collector that has been
	// refusing connections since startup produces ZERO export errors and the
	// component reported "exporting". A quiet service is exactly when nobody is
	// watching a dashboard closely enough to notice it has gone flat.
	//
	// # What it does NOT catch, and must not be read as claiming
	//
	// A TCP connect proves something is LISTENING, not that a collector is
	// working. Anything that accepts and then fails -- a port forwarder in
	// front of a stopped container, a proxy, a sidecar, a collector that
	// answers the handshake and rejects every payload -- passes this and is
	// caught only by the export errors below.
	//
	// Measured, and worth recording because it is easy to mistake for the probe
	// working: with Tempo stopped, podman's gvproxy still held :4318, so the
	// dial succeeded and this reported healthy. The export-error path caught it
	// once traffic flowed.
	//
	// So the two are complements, not redundancy -- one sees an absent
	// listener without needing traffic, the other sees a broken collector but
	// only once something has been sent. Neither alone is enough.
	//
	// The dial is also the only honest response time this component can carry:
	// everything else it reports is a read of recorded state.
	var collectorErr error
	if traces == config.ExporterOTLPHTTP {
		address := net.JoinHostPort(
			a.configs.Telemetry.TraceEndpoint.Value,
			strconv.Itoa(a.configs.Telemetry.TracePort.Value),
		)

		var elapsed time.Duration
		elapsed, collectorErr = a.dialProbe(ctx, address)
		responseTime = &elapsed
		details["collector"] = address
	}

	if a.telemetry.Errors != nil {
		count, last, lastErr := a.telemetry.Errors.Snapshot()
		if count > 0 {
			details["export_errors"] = strconv.FormatUint(count, 10)
			details["last_export_error"] = lastErr
			details["last_export_error_at"] = last.UTC().Format(time.RFC3339)

			if a.telemetry.Errors.Failing(telemetryErrorWindow(a.configs.Telemetry)) {
				return ComponentHealth{
					Name:   "telemetry",
					Status: ComponentStatusDegraded,
					Message: "the telemetry exporter is failing; traces and metrics " +
						"from this replica are not reaching the collector",
					LastChecked:  time.Now(),
					ResponseTime: responseTime,
					Details:      details,
				}
			}
		}
	}

	// The collector refused the connection, and no export has failed yet --
	// which is the quiet-service case: nothing has been sent, so nothing has
	// had the chance to fail. Degraded, not healthy: the next batch will be
	// lost, and reporting "exporting" here is how a flat dashboard goes
	// unexplained.
	if collectorErr != nil {
		slog.Warn(
			"telemetry collector unreachable",
			"collector", details["collector"],
			"error", collectorErr,
		)

		return ComponentHealth{
			Name:   "telemetry",
			Status: ComponentStatusDegraded,
			Message: "the telemetry collector is not accepting connections; " +
				"traces and metrics from this replica will be dropped",
			LastChecked:  time.Now(),
			ResponseTime: responseTime,
			Details:      details,
		}
	}

	// Both off. Not a fault, but "active" would be false.
	if traces == config.ExporterNoop && metrics == config.ExporterNoop {
		return ComponentHealth{
			Name:   "telemetry",
			Status: ComponentStatusHealthy,
			Message: "telemetry is disabled; opentelemetry.trace.exporter and " +
				"opentelemetry.metric.exporter are both " + config.ExporterNoop +
				", so nothing is exported",
			LastChecked:  time.Now(),
			ResponseTime: responseTime,
			Details:      details,
		}
	}

	message := "telemetry exporting"
	if traces == config.ExporterNoop || metrics == config.ExporterNoop {
		// Half on is a real and easily-unnoticed configuration: one pipeline
		// feeding dashboards while the other silently produces nothing.
		message = "telemetry partially exporting; one exporter is " + config.ExporterNoop
	}

	return ComponentHealth{
		Name:         "telemetry",
		Status:       ComponentStatusHealthy,
		Message:      message,
		LastChecked:  time.Now(),
		ResponseTime: responseTime,
		Details:      details,
	}
}

// checkDatabaseHealth checks if the database is healthy by pinging it
func (a *App) checkDatabaseHealth(ctx context.Context) ComponentHealth {
	start := time.Now()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := a.dbPool.Ping(pingCtx)
	responseTime := time.Since(start)

	if err != nil {
		// The reason goes to the log, not into the payload. /health/detailed is
		// public and unauthenticated -- a probe cannot hold a token -- and the
		// pgx failure text names the database user, the database and every
		// address the pool tried. See handler.healthCheckFailedMessage.
		slog.Error(
			"database health check failed",
			"error", err,
			"response_time_ms", responseTime.Milliseconds(),
		)

		return ComponentHealth{
			Name:         "database",
			Status:       ComponentStatusUnhealthy,
			Message:      "database ping failed",
			LastChecked:  time.Now(),
			ResponseTime: &responseTime,
		}
	}

	status := ComponentStatusHealthy
	message := "database responding"

	// Check if response time is degraded (> 1 second)
	if responseTime > time.Second {
		status = ComponentStatusDegraded
		message = "database responding slowly"
	}

	return ComponentHealth{
		Name:         "database",
		Status:       status,
		Message:      message,
		LastChecked:  time.Now(),
		ResponseTime: &responseTime,
	}
}

// checkCacheHealth pings Valkey within the same budget a real cache read gets.
//
// Using cache.max.query.timeout rather than a generous health-check timeout is
// the point: a Valkey that cannot answer inside that budget is one every read
// will abandon in favour of the database, so from the service's perspective it
// is already down even though it is technically reachable.
//
// The worst status this returns is degraded. See the call site for why an
// unreachable cache must not fail readiness.
func (a *App) checkCacheHealth(ctx context.Context) ComponentHealth {
	start := time.Now()

	pingCtx, cancel := context.WithTimeout(ctx, a.configs.Cache.MaxQueryTimeout.Value)
	defer cancel()

	err := a.cacheClient.Do(pingCtx, a.cacheClient.B().Ping().Build()).Error()
	responseTime := time.Since(start)

	if err != nil {
		slog.Warn(
			"cache health check failed, reads are falling through to the database",
			"error", err,
			"response_time_ms", responseTime.Milliseconds(),
		)

		return ComponentHealth{
			Name:         "cache",
			Status:       ComponentStatusDegraded,
			Message:      "cache ping failed, reads are falling through to the database",
			LastChecked:  time.Now(),
			ResponseTime: &responseTime,
		}
	}

	return ComponentHealth{
		Name:         "cache",
		Status:       ComponentStatusHealthy,
		Message:      "cache responding",
		LastChecked:  time.Now(),
		ResponseTime: &responseTime,
		Details: map[string]any{
			"encoder":       a.configs.Cache.EncoderType.Value,
			"query_timeout": a.configs.Cache.MaxQueryTimeout.Value.String(),
			"hard_ttl":      a.configs.Cache.EntitiesHardTTL.Value.String(),
			"soft_ttl":      a.configs.Cache.EntitiesSoftTTL.Value.String(),
		},
	}
}

// getDatabasePoolStats returns current database pool statistics
func (a *App) getDatabasePoolStats() *DatabasePoolStats {
	if a.dbPool == nil {
		return nil
	}

	stat := a.dbPool.Stat()
	return &DatabasePoolStats{
		TotalConns:           stat.TotalConns(),
		IdleConns:            stat.IdleConns(),
		AcquiredConns:        stat.AcquiredConns(),
		ConstructingConns:    stat.ConstructingConns(),
		MaxConns:             stat.MaxConns(),
		AcquireCount:         stat.AcquireCount(),
		AcquireDurationNs:    stat.AcquireDuration().Nanoseconds(),
		EmptyAcquireCount:    stat.EmptyAcquireCount(),
		CanceledAcquireCount: stat.CanceledAcquireCount(),
	}
}

// formatHealthDuration renders a check's duration at a resolution that can
// actually represent it.
//
// The published example for this field is "2.5ms", which is fractional --
// milliseconds alone cannot express it. A healthy local database pings in a few
// hundred microseconds, so truncating to whole milliseconds reported every
// check as zero, and zero was then read as "not measured".
func formatHealthDuration(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	case d >= time.Microsecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	default:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
}

// convertToModelHealth converts internal AppHealth to payload.DetailedHealth
func (a *App) convertToModelHealth(ah *AppHealth) payload.DetailedHealth {
	// Convert components
	components := make(map[string]payload.ComponentHealth)
	for name, comp := range ah.Components {
		responseTime := ""
		if comp.ResponseTime != nil {
			responseTime = formatHealthDuration(*comp.ResponseTime)
		}
		components[name] = payload.ComponentHealth{
			Status:       string(comp.Status),
			Message:      comp.Message,
			ResponseTime: responseTime,
			Details:      comp.Details,
		}
	}

	// Convert database pool stats
	var dbPool *payload.DatabasePoolStats
	if ah.DatabaseStats != nil {
		dbPool = &payload.DatabasePoolStats{
			AcquiredConnections:     ah.DatabaseStats.AcquiredConns,
			IdleConnections:         ah.DatabaseStats.IdleConns,
			MaxConnections:          ah.DatabaseStats.MaxConns,
			TotalConnections:        ah.DatabaseStats.TotalConns,
			ConstructingConnections: ah.DatabaseStats.ConstructingConns,
			EmptyAcquireCount:       ah.DatabaseStats.EmptyAcquireCount,
			AcquireCount:            ah.DatabaseStats.AcquireCount,
			AcquireDuration:         ah.DatabaseStats.AcquireDurationNs,
			CanceledAcquireCount:    ah.DatabaseStats.CanceledAcquireCount,
		}
	}

	// Convert startup metrics
	var startupMetrics *payload.StartupMetrics
	if ah.Startup != nil && ah.Startup.TotalDuration > 0 {
		phaseTimings := make(map[string]any)
		phasePercentage := make(map[string]any)

		if ah.Startup.ConfigLoadDuration > 0 {
			phaseTimings["config_load"] = ah.Startup.ConfigLoadDuration.String()
			phasePercentage["config_load"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.ConfigLoadDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.TelemetryDuration > 0 {
			phaseTimings["telemetry"] = ah.Startup.TelemetryDuration.String()
			phasePercentage["telemetry"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.TelemetryDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.DatabaseDuration > 0 {
			phaseTimings["database"] = ah.Startup.DatabaseDuration.String()
			phasePercentage["database"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.DatabaseDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.RepositoriesDuration > 0 {
			phaseTimings["repositories"] = ah.Startup.RepositoriesDuration.String()
			phasePercentage["repositories"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.RepositoriesDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.MailServiceDuration > 0 {
			phaseTimings["mail_service"] = ah.Startup.MailServiceDuration.String()
			phasePercentage["mail_service"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.MailServiceDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.ServicesDuration > 0 {
			phaseTimings["services"] = ah.Startup.ServicesDuration.String()
			phasePercentage["services"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.ServicesDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.HandlersDuration > 0 {
			phaseTimings["handlers"] = ah.Startup.HandlersDuration.String()
			phasePercentage["handlers"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.HandlersDuration)/float64(ah.Startup.TotalDuration)*100)
		}
		if ah.Startup.HTTPServerDuration > 0 {
			phaseTimings["http_server"] = ah.Startup.HTTPServerDuration.String()
			phasePercentage["http_server"] = fmt.Sprintf("%.1f%%", float64(ah.Startup.HTTPServerDuration)/float64(ah.Startup.TotalDuration)*100)
		}

		startupMetrics = &payload.StartupMetrics{
			TotalTime:       ah.Startup.TotalDuration.String(),
			PhaseTimings:    phaseTimings,
			PhasePercentage: phasePercentage,
		}
	}

	// The runtime picture an operator wants, behind the authentication this
	// endpoint now requires. It used to be on the PUBLIC /health/status.
	//
	// This one is timed, unlike the four structural components, because
	// runtime.ReadMemStats is real work rather than a pointer comparison -- it
	// can stop the world -- so the number means something: a collection that
	// takes noticeably longer than usual is a symptom of heap pressure, which
	// is precisely what an operator opened this page to see.
	runtimeStart := time.Now()
	runtimeInfo := usecase.RuntimeInfo()
	runtimeElapsed := time.Since(runtimeStart)

	components["runtime"] = payload.ComponentHealth{
		Status:       "healthy",
		Message:      "go runtime",
		ResponseTime: formatHealthDuration(runtimeElapsed),
		Details:      runtimeInfo,
	}

	return payload.DetailedHealth{
		Status:         string(ah.Status),
		Version:        ah.Version,
		Uptime:         ah.Uptime.String(),
		Components:     components,
		DatabasePool:   dbPool,
		StartupMetrics: startupMetrics,
	}
}
