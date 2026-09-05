package notifieremail

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
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

	return &enqueueMetrics{enqueued: enqueued, duration: duration}, nil
}

func (m *enqueueMetrics) record(ctx context.Context, start time.Time, template, result string) {
	if m == nil {
		return
	}
	m.enqueued.Add(ctx, 1, metric.WithAttributes(
		attribute.String("template", template),
		attribute.String("result", result),
	))
	m.duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
		attribute.String("template", template),
	))
}

// sendMetrics instruments the actual SMTP send performed by the mail worker.
// A nil *sendMetrics is a no-op.
type sendMetrics struct {
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

	return &sendMetrics{sent: sent, duration: duration}, nil
}

func (m *sendMetrics) record(ctx context.Context, start time.Time, result string) {
	if m == nil {
		return
	}
	m.sent.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	m.duration.Record(ctx, time.Since(start).Seconds())
}
