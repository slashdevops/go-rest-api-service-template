//go:build unit

package app

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/server"
	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// The two components these tests cover were nil checks: they answered "healthy"
// on the strength of a non-nil pointer. Every assertion here is one the nil
// check passed, so each of them fails if the probe is reverted.

func testApp(t *testing.T) *App {
	t.Helper()

	return &App{
		configs: &Configs{
			Telemetry: config.NewOpenTelemetryConfig("test-app", "0.0.0-test"),
			Mail:      config.NewMailConfig(),
		},
	}
}

func TestTimeCheckMeasuresEveryComponent(t *testing.T) {
	got := timeCheck(func() ComponentHealth {
		return ComponentHealth{Name: "structural", Status: ComponentStatusHealthy}
	})

	if got.ResponseTime == nil {
		t.Fatal("ResponseTime = nil; every component's check is measured now")
	}
	if *got.ResponseTime <= 0 {
		t.Errorf("ResponseTime = %v, want a positive duration", *got.ResponseTime)
	}
}

func TestTimeCheckKeepsAMoreSpecificMeasurement(t *testing.T) {
	// A check that measured the thing an operator is judging -- the database
	// ping, not the struct building around it -- keeps its own number.
	probe := 42 * time.Millisecond

	got := timeCheck(func() ComponentHealth {
		return ComponentHealth{Name: "database", ResponseTime: &probe}
	})

	if got.ResponseTime == nil || *got.ResponseTime != probe {
		t.Errorf("ResponseTime = %v, want the check's own %v", got.ResponseTime, probe)
	}
}

func TestEveryHealthyComponentCarriesAResponseTime(t *testing.T) {
	// The regression this pins: the column was populated for some components
	// and blank for others, which reads as missing data rather than as a
	// distinction, and was reported as a bug three times.
	//
	// Structural components are included deliberately. Their number is small,
	// and their message is what says why -- see the note below.
	a := testApp(t)
	a.configs.Telemetry.TraceExporter.Value = config.ExporterNoop
	a.configs.Telemetry.MetricExporter.Value = config.ExporterNoop
	a.telemetry = &o11y.OpenTelemetry{Errors: &o11y.ExportErrors{}}
	a.httpServer = &server.HTTPServer{}
	a.repositories = &Repositories{}
	a.services = &Services{}
	a.handlers = &Handlers{}

	health := a.getDetailedHealth(context.Background())

	for name, comp := range health.Components {
		if comp.Status == ComponentStatusUnknown {
			// An absent component reports no number, because no check ran.
			continue
		}
		if comp.ResponseTime == nil {
			t.Errorf("component %q has no response time", name)
		}
	}

	// A number next to a structural component must not be readable as a probe.
	// The message is the only thing that prevents that, so it is load-bearing.
	for _, name := range []string{"http_server", "repositories", "services", "handlers"} {
		comp, ok := health.Components[name]
		if !ok {
			t.Errorf("component %q missing", name)
			continue
		}
		if !strings.Contains(comp.Message, "nothing is probed") {
			t.Errorf("component %q message = %q, want it to say nothing is probed",
				name, comp.Message)
		}
	}
}

func TestMailHealthReportsAnUnreachableTransport(t *testing.T) {
	a := testApp(t)
	a.configs.Mail.MailSender.Value = config.MailSenderSMTP
	a.configs.Mail.SMTPHost.Value = "127.0.0.1"
	// Port 1 is reserved and nothing listens on it, so the dial fails fast
	// without depending on a name server or on the network being down.
	a.configs.Mail.SMTPPort.Value = 1

	ctx, cancel := context.WithTimeout(context.Background(), mailProbeTimeout)
	defer cancel()

	got := a.checkMailHealth(ctx)

	if got.Status != ComponentStatusDegraded {
		t.Errorf("status = %q, want %q -- an unreachable mail host must not report healthy",
			got.Status, ComponentStatusDegraded)
	}

	// The message has to say the consequence, not just the fact. A dropped send
	// is not retried, and an operator reading "mail_service: degraded" has no
	// way to know that from the status alone.
	if !strings.Contains(got.Message, "not retried") {
		t.Errorf("message = %q, want it to say sends are not retried", got.Message)
	}

	if got.ResponseTime == nil {
		t.Error("ResponseTime = nil, want the failed dial to still be timed")
	}
}

func TestMailHealthReportsAReachableTransport(t *testing.T) {
	// A real listener, so this asserts the probe connects rather than that it
	// tolerates a failure.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}

	a := testApp(t)
	a.configs.Mail.MailSender.Value = config.MailSenderSMTP
	a.configs.Mail.SMTPHost.Value = host
	if _, err := net.LookupPort("tcp", port); err != nil {
		t.Fatalf("port: %v", err)
	}
	a.configs.Mail.SMTPPort.Value = ln.Addr().(*net.TCPAddr).Port

	got := a.checkMailHealth(context.Background())

	if got.Status != ComponentStatusHealthy {
		t.Errorf("status = %q, want healthy (message: %q)", got.Status, got.Message)
	}

	// The probe's limits belong in the payload: a reachable host says nothing
	// about whether the credentials work.
	if probe, _ := got.Details["probe"].(string); !strings.Contains(probe, "tcp connect only") {
		t.Errorf("details[probe] = %q, want it to state what was NOT verified", probe)
	}
}

func TestMailHealthBoundsItsOwnDial(t *testing.T) {
	// 192.0.2.0/24 is TEST-NET-1 (RFC 5737): routable nowhere, so the connect
	// neither succeeds nor is refused -- it hangs until something stops it.
	// That is the case the probe's own timeout exists for, and the case a
	// health endpoint must not inherit from its caller: this test passes a
	// context with NO deadline, which is what a health poll actually carries.
	a := testApp(t)
	a.configs.Mail.MailSender.Value = config.MailSenderSMTP
	a.configs.Mail.SMTPHost.Value = "192.0.2.1"
	a.configs.Mail.SMTPPort.Value = 25

	start := time.Now()
	got := a.checkMailHealth(context.Background())
	elapsed := time.Since(start)

	if got.Status != ComponentStatusDegraded {
		t.Errorf("status = %q, want degraded", got.Status)
	}

	// Generous, because a slow CI box is not the failure being tested. What
	// would fail here is a probe with no bound at all, which hangs for the
	// platform's TCP connect timeout -- 75 seconds on Linux.
	if elapsed > mailProbeTimeout+2*time.Second {
		t.Errorf("probe took %s, want it bounded by mailProbeTimeout (%s)", elapsed, mailProbeTimeout)
	}
}

func TestMailTransportAddressForMailgun(t *testing.T) {
	tests := map[string]struct {
		url  string
		want string
	}{
		"https gets 443":     {"https://api.mailgun.net/v3/x", "api.mailgun.net:443"},
		"http gets 80":       {"http://localhost/v3/x", "localhost:80"},
		"explicit port wins": {"https://api.mailgun.net:8443/v3/x", "api.mailgun.net:8443"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a := testApp(t)
			a.configs.Mail.MailSender.Value = config.MailSenderMailgun
			a.configs.Mail.APIURL.Value = tc.url

			got, err := a.mailTransportAddress()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("address = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTelemetryHealthSaysDisabledRatherThanActive(t *testing.T) {
	a := testApp(t)
	a.telemetry = &o11y.OpenTelemetry{Errors: &o11y.ExportErrors{}}
	a.configs.Telemetry.TraceExporter.Value = config.ExporterNoop
	a.configs.Telemetry.MetricExporter.Value = config.ExporterNoop

	got := a.checkTelemetryHealth(context.Background())

	// Not a fault -- an operator chose this -- so the status stays healthy.
	if got.Status != ComponentStatusHealthy {
		t.Errorf("status = %q, want healthy: a deliberately disabled exporter is not a fault", got.Status)
	}

	// But it must not claim to be exporting. This is the assertion the old nil
	// check failed: it said "telemetry active" with both exporters off.
	if !strings.Contains(got.Message, "disabled") {
		t.Errorf("message = %q, want it to say telemetry is disabled", got.Message)
	}
	if strings.Contains(got.Message, "active") {
		t.Errorf("message = %q, must not claim telemetry is active", got.Message)
	}
}

func TestTelemetryHealthReportsAFailingExporter(t *testing.T) {
	a := testApp(t)
	errs := &o11y.ExportErrors{}
	a.telemetry = &o11y.OpenTelemetry{Errors: errs}
	a.configs.Telemetry.TraceExporter.Value = "otlp-http"
	a.configs.Telemetry.MetricExporter.Value = "otlp-http"

	errs.Handle(errors.New("connection refused"))

	got := a.checkTelemetryHealth(context.Background())

	if got.Status != ComponentStatusDegraded {
		t.Errorf("status = %q, want degraded after an export failure", got.Status)
	}
	if got.Details["last_export_error"] != "connection refused" {
		t.Errorf("details[last_export_error] = %v, want the SDK's message", got.Details["last_export_error"])
	}
}

func TestTelemetryHealthReportsAnUnreachableCollector(t *testing.T) {
	// The blind spot this closes: with no traffic there is nothing to export,
	// so a collector that has been refusing connections since startup produces
	// ZERO export errors. Before the dial, that reported "telemetry exporting"
	// -- and a quiet service is exactly when nobody is watching a dashboard to
	// notice it has gone flat.
	a := testApp(t)
	a.telemetry = &o11y.OpenTelemetry{Errors: &o11y.ExportErrors{}}
	a.configs.Telemetry.TraceExporter.Value = config.ExporterOTLPHTTP
	a.configs.Telemetry.MetricExporter.Value = config.ExporterOTLPHTTP
	a.configs.Telemetry.TraceEndpoint.Value = "127.0.0.1"
	a.configs.Telemetry.TracePort.Value = 1 // reserved; nothing listens

	got := a.checkTelemetryHealth(context.Background())

	if got.Status != ComponentStatusDegraded {
		t.Errorf("status = %q, want degraded (message: %q)", got.Status, got.Message)
	}
	if got.ResponseTime == nil {
		t.Error("ResponseTime = nil, want the failed dial to still be timed")
	}
	if got.Details["collector"] != "127.0.0.1:1" {
		t.Errorf("details[collector] = %v, want the address that was tried", got.Details["collector"])
	}
}

func TestTelemetryHealthTimesAReachableCollector(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	a := testApp(t)
	a.telemetry = &o11y.OpenTelemetry{Errors: &o11y.ExportErrors{}}
	a.configs.Telemetry.TraceExporter.Value = config.ExporterOTLPHTTP
	a.configs.Telemetry.MetricExporter.Value = config.ExporterOTLPHTTP
	a.configs.Telemetry.TraceEndpoint.Value = "127.0.0.1"
	a.configs.Telemetry.TracePort.Value = ln.Addr().(*net.TCPAddr).Port

	got := a.checkTelemetryHealth(context.Background())

	if got.Status != ComponentStatusHealthy {
		t.Errorf("status = %q, want healthy (message: %q)", got.Status, got.Message)
	}

	// This is the whole point of the change: the component now carries a number.
	if got.ResponseTime == nil {
		t.Fatal("ResponseTime = nil, want the collector round trip to be timed")
	}
}

func TestTelemetryHealthDoesNotDialWhenThereIsNoCollector(t *testing.T) {
	// A noop exporter ships to nothing, so there is no host to reach for and no
	// honest number to report. Dialling anyway would time a connection the
	// service never makes.
	a := testApp(t)
	a.telemetry = &o11y.OpenTelemetry{Errors: &o11y.ExportErrors{}}
	a.configs.Telemetry.TraceExporter.Value = config.ExporterNoop
	a.configs.Telemetry.MetricExporter.Value = config.ExporterNoop
	a.configs.Telemetry.TraceEndpoint.Value = "127.0.0.1"
	a.configs.Telemetry.TracePort.Value = 1

	got := a.checkTelemetryHealth(context.Background())

	if got.Status != ComponentStatusHealthy {
		t.Errorf("status = %q, want healthy: a disabled exporter is not a fault", got.Status)
	}
	if got.ResponseTime != nil {
		t.Errorf("ResponseTime = %v, want nil: nothing was dialled", got.ResponseTime)
	}
	if _, dialled := got.Details["collector"]; dialled {
		t.Error("details carries a collector address for an exporter that has none")
	}
}

func TestTelemetryHealthWhenNotInitialized(t *testing.T) {
	a := testApp(t)

	got := a.checkTelemetryHealth(context.Background())

	if got.Status != ComponentStatusUnknown {
		t.Errorf("status = %q, want unknown", got.Status)
	}
}
