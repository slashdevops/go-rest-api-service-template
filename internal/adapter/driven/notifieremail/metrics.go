package notifieremail

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// Metric instrument names. Counters keep the _total suffix so the Prometheus
// OTLP receiver does not double it.
const (
	metricEmailEnqueued        = "email_enqueued_total"
	metricEmailEnqueueDuration = "email_enqueue_duration_seconds"
	metricEmailSend            = "email_send_total"
	metricEmailSendDuration    = "email_send_duration_seconds"
)

// Template label values.
const (
	templateAccountVerification = "account_verification"
	templatePasswordReset       = "password_reset"
	templateAccountExists       = "account_exists"
)

const (
	resultOK    = "ok"
	resultError = "error"
)

// enqueueMetrics instruments the adapter's Enqueue path (which can block on the
// mail queue's back-pressure). A nil *enqueueMetrics is a no-op.
type enqueueMetrics struct {
	enqueued metric.Int64Counter
	duration metric.Float64Histogram

	// layer is the shared app_calls_total / app_call_duration_seconds pair,
	// recorded in ADDITION to the two above. See [recordLayer].
	layer *o11y.LayerMetrics
}

func newEnqueueMetrics(meter metric.Meter) (*enqueueMetrics, error) {
	if meter == nil {
		return nil, nil
	}

	enqueued, err := meter.Int64Counter(
		metricEmailEnqueued,
		metric.WithDescription("Emails enqueued for delivery, by template and result."),
	)
	if err != nil {
		return nil, err
	}

	duration, err := meter.Float64Histogram(
		metricEmailEnqueueDuration,
		metric.WithDescription("Time spent enqueuing an email (includes queue back-pressure)."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	// Losing the shared pair must not take the mail-specific instruments with
	// it: one costs a panel, the other costs the reason this is instrumented.
	layer, err := o11y.NewLayerMetrics(meter, "")
	if err != nil {
		return &enqueueMetrics{enqueued: enqueued, duration: duration}, nil
	}

	return &enqueueMetrics{enqueued: enqueued, duration: duration, layer: layer}, nil
}

// recordLayer mirrors a mail operation onto the shared per-layer instruments,
// so mail appears in a layer breakdown alongside handler, usecase, repository,
// llm and cache instead of leaving a hole where a real dependency sits.
//
// The specialised instruments above keep their own vocabulary -- per template,
// enqueue separated from send, because the two fail for different reasons and
// at different times. Different instrument names, so nothing is double-counted.
func recordLayer(ctx context.Context, layer *o11y.LayerMetrics, action, domainName, result string, took time.Duration) {
	if layer == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String(o11y.AttrLayer, o11y.LayerMail),
		attribute.String(o11y.AttrDomain, domainName),
		attribute.String(o11y.AttrAction, action),
		attribute.Bool(o11y.AttrSuccessful, result != resultError),
	}

	if layer.Counter != nil {
		layer.Counter.Add(ctx, 1, metric.WithAttributes(attrs...))
	}

	if layer.Histogram != nil {
		layer.Histogram.Record(ctx, took.Seconds(), metric.WithAttributes(attrs...))
	}
}

func (m *enqueueMetrics) record(ctx context.Context, start time.Time, template, result string) {
	if m == nil {
		return
	}
	m.enqueued.Add(ctx, 1, metric.WithAttributes(
		attribute.String("template", template),
		attribute.String("result", result),
	))
	recordLayer(ctx, m.layer, "enqueue", template, result, time.Since(start))

	m.duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
		attribute.String("template", template),
	))
}

// sendMetrics instruments the actual SMTP send performed by the mail worker.
// A nil *sendMetrics is a no-op.
type sendMetrics struct {
	layer *o11y.LayerMetrics

	sent     metric.Int64Counter
	duration metric.Float64Histogram
}

func newSendMetrics(meter metric.Meter) (*sendMetrics, error) {
	if meter == nil {
		return nil, nil
	}

	sent, err := meter.Int64Counter(
		metricEmailSend,
		metric.WithDescription("Emails sent by the mail worker, by result."),
	)
	if err != nil {
		return nil, err
	}

	duration, err := meter.Float64Histogram(
		metricEmailSendDuration,
		metric.WithDescription("SMTP send duration in seconds."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	layer, err := o11y.NewLayerMetrics(meter, "")
	if err != nil {
		return &sendMetrics{sent: sent, duration: duration}, nil
	}

	return &sendMetrics{sent: sent, duration: duration, layer: layer}, nil
}

func (m *sendMetrics) record(ctx context.Context, start time.Time, result string) {
	if m == nil {
		return
	}
	m.sent.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	m.duration.Record(ctx, time.Since(start).Seconds())

	recordLayer(ctx, m.layer, "send", "Mail", result, time.Since(start))
}
