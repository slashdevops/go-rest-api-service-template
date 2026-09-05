package notifieremail

import (
	"context"
	"errors"
	"testing"

	"github.com/slashdevops/mailer"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// fakeMailer is a mailer.MailerService whose Send returns a fixed error.
type fakeMailer struct{ err error }

func (f fakeMailer) Send(_ context.Context, _ mailer.MailContent) error { return f.err }

// counterValue sums the email_send_total data points whose result label
// matches want, across the collected metrics.
func counterValue(t *testing.T, rm metricdata.ResourceMetrics, name, want string) int64 {
	t.Helper()
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %s is not an int64 Sum", name)
			}
			for _, dp := range sum.DataPoints {
				if v, present := dp.Attributes.Value("result"); present && v.AsString() == want {
					total += dp.Value
				}
			}
		}
	}
	return total
}

func TestInstrumentedMailer_Send_recordsResult(t *testing.T) {
	tests := []struct {
		name    string
		sendErr error
		want    string
	}{
		{"ok", nil, resultOK},
		{"error", errors.New("smtp down"), resultError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := metric.NewManualReader()
			mp := metric.NewMeterProvider(metric.WithReader(reader))
			meter := mp.Meter("test")

			m := NewInstrumentedMailer(fakeMailer{err: tc.sendErr}, meter)
			err := m.Send(context.Background(), mailer.MailContent{})

			if (err != nil) != (tc.sendErr != nil) {
				t.Fatalf("Send() error = %v, want error presence %v", err, tc.sendErr != nil)
			}

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Fatalf("Collect: %v", err)
			}
			if got := counterValue(t, rm, metricEmailSend, tc.want); got != 1 {
				t.Errorf("email_send_total{result=%q} = %d, want 1", tc.want, got)
			}
		})
	}
}

func TestNewInstrumentedMailer_nilMeter_returnsInner(t *testing.T) {
	inner := fakeMailer{}
	if got := NewInstrumentedMailer(inner, nil); got != mailer.MailerService(inner) {
		t.Error("with nil meter, expected the inner mailer returned unchanged")
	}
}
