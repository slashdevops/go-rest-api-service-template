package o11y

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	otelMetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// OpenTelemetryMeterConfig represents the configuration of the OpenTelemetry meter.
type OpenTelemetryMeterConfig struct {
	Resources      *resource.Resource
	Name           string
	MetricEndpoint string
	MetricExporter string
	MetricPort     int
	MetricInterval time.Duration
}

// OpenTelemetryMeter represents the metrics of the service.
// It is used to collect and export metrics using OpenTelemetry.
// It is initialized with the OpenTelemetryMeterConfig and provides methods to set up and shutdown the metrics.
type OpenTelemetryMeter struct {
	ctx context.Context

	// Meter is the OpenTelemetry metric meter.
	Meter otelMetric.Meter

	// Resource is the OpenTelemetry resource.
	res *resource.Resource

	// MeterProvider is the OpenTelemetry metric meter provider.
	mp *metric.MeterProvider

	name string

	metricEndpoint string
	metricExporter string
	metricPort     int
	metricInterval time.Duration
}

func NewOpenTelemetryMeter(ctx context.Context, conf *OpenTelemetryMeterConfig) *OpenTelemetryMeter {
	return &OpenTelemetryMeter{
		ctx:  ctx,
		name: conf.Name,

		metricEndpoint: conf.MetricEndpoint,
		metricPort:     conf.MetricPort,
		metricExporter: conf.MetricExporter,
		metricInterval: conf.MetricInterval,

		res: conf.Resources,

		Meter: otel.Meter(conf.Name),
	}
}

func (ref *OpenTelemetryMeter) SetupMetrics() error {
	// when testing, use the noop exporter
	if ref.metricExporter == config.ExporterNoop {
		slog.Warn("No metric exporter configured, use 'noop' for testing purposes only")

		mp := noop.NewMeterProvider()

		otel.SetMeterProvider(mp)
		ref.Meter = mp.Meter(ref.name)
		return nil
	}

	// Set up metric exporter.
	mExp, err := ref.newMetricExporter(ref.ctx)
	if err != nil {
		return err
	}

	// Set up meter provider.
	mp, err := ref.newMeterProvider(mExp)
	if err != nil {
		return err
	}
	ref.mp = mp

	// Register the meter provider with the global provider.
	otel.SetMeterProvider(mp)
	ref.Meter = mp.Meter(ref.name)

	return nil
}

func (ref *OpenTelemetryMeter) Shutdown() {
	if ref.mp != nil {
		if err := ref.mp.Shutdown(ref.ctx); err != nil {
			slog.Error("failed to shutdown meter provider", "error", err)
		}
	}
}

// newMetricExporter creates a new metric exporter based on the configuration.
func (ref *OpenTelemetryMeter) newMetricExporter(ctx context.Context) (metric.Exporter, error) {
	var exporter metric.Exporter
	var err error

	switch ref.metricExporter {
	case "console":
		exporter, err = stdoutmetric.New(stdoutmetric.WithPrettyPrint())
		if err != nil {
			return nil, err
		}

	case "otlp-http":
		insecureOpt := otlpmetrichttp.WithInsecure()
		WithCompression := otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression)
		endpointOpt := otlpmetrichttp.WithEndpointURL(
			fmt.Sprintf(
				"http://%s:%d/api/v1/otlp/v1/metrics",
				ref.metricEndpoint,
				ref.metricPort,
			),
		)
		exporter, err = otlpmetrichttp.New(ctx, insecureOpt, endpointOpt, WithCompression)
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("unknown metric exporter: %s", ref.metricExporter)
	}

	return exporter, nil
}

// newMeterProvider creates a new MeterProvider with the given exporter.
//
// Note: dot-to-underscore normalization of attribute keys (e.g. app.layer ->
// app_layer) is intentionally left to the exporter/backend, which applies the
// Prometheus naming rules. A previous SDK View attempted this via an
// AttributeFilter, but AttributeFilter is a keep/drop predicate and mutating
// its by-value argument had no effect, so it was removed.
func (ref *OpenTelemetryMeter) newMeterProvider(exp metric.Exporter) (*metric.MeterProvider, error) {
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(ref.res),
		metric.WithReader(
			metric.NewPeriodicReader(exp, metric.WithInterval(ref.metricInterval)),
		),
	)

	return meterProvider, nil
}
