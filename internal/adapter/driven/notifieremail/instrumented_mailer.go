package notifieremail

import (
	"context"
	"time"

	"github.com/slashdevops/mailer"
	"go.opentelemetry.io/otel/metric"
)

// instrumentedMailer wraps a mailer.MailerService and records send count and
// latency. It lets the app observe the actual SMTP delivery performed
// asynchronously by the mail worker, which is otherwise invisible to the
// enqueue-only adapter.
type instrumentedMailer struct {
	inner   mailer.MailerService
	metrics *sendMetrics
}

// NewInstrumentedMailer wraps inner with send metrics. When meter is nil (or
// the instruments cannot be built) it returns inner unchanged, so wiring it in
// is always safe.
func NewInstrumentedMailer(inner mailer.MailerService, meter metric.Meter) mailer.MailerService {
	sm, err := newSendMetrics(meter)
	if err != nil || sm == nil {
		return inner
	}
	return &instrumentedMailer{inner: inner, metrics: sm}
}

// Send implements mailer.MailerService.
func (m *instrumentedMailer) Send(ctx context.Context, content mailer.MailContent) error {
	start := time.Now()
	err := m.inner.Send(ctx, content)

	result := resultOK
	if err != nil {
		result = resultError
	}
	m.metrics.record(ctx, start, result)

	return err
}
