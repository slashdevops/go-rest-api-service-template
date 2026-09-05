package cslog

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogLevelTrace(t *testing.T) {
	t.Run("trace_level_is_below_debug", func(t *testing.T) {
		assert.Less(t, LogLevelTrace, slog.LevelDebug)
	})

	t.Run("trace_level_value", func(t *testing.T) {
		assert.Equal(t, slog.Level(-8), LogLevelTrace)
	})
}

func TestLogLevelFatal(t *testing.T) {
	t.Run("fatal_level_is_above_error", func(t *testing.T) {
		assert.Greater(t, LogLevelFatal, slog.LevelError)
	})

	t.Run("fatal_level_value", func(t *testing.T) {
		assert.Equal(t, slog.Level(12), LogLevelFatal)
	})
}

func TestTrace(t *testing.T) {
	t.Run("logs_at_trace_level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: LogLevelTrace,
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Trace(ctx, "trace message", "key1", "value1")

		output := buf.String()
		require.NotEmpty(t, output)
		assert.Contains(t, output, "trace message")
		assert.Contains(t, output, "key1")
		assert.Contains(t, output, "value1")
	})

	t.Run("not_logged_when_level_is_debug", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Trace(ctx, "should not appear")

		assert.Empty(t, buf.String())
	})

	t.Run("logs_without_extra_args", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: LogLevelTrace,
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Trace(ctx, "trace no args")

		output := buf.String()
		require.NotEmpty(t, output)
		assert.Contains(t, output, "trace no args")
	})

	t.Run("logs_with_multiple_key_value_pairs", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: LogLevelTrace,
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Trace(ctx, "multi args", "k1", "v1", "k2", 42, "k3", true)

		output := buf.String()
		require.NotEmpty(t, output)
		assert.Contains(t, output, "multi args")
		assert.Contains(t, output, "k1")
		assert.Contains(t, output, "k2")
		assert.Contains(t, output, "k3")
	})
}

func TestFatal(t *testing.T) {
	t.Run("logs_at_fatal_level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: LogLevelFatal,
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Fatal(ctx, "fatal error", "error", "something broke")

		output := buf.String()
		require.NotEmpty(t, output)
		assert.Contains(t, output, "fatal error")
		assert.Contains(t, output, "something broke")
	})

	t.Run("logged_when_level_is_error", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.LevelError,
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Fatal(ctx, "fatal visible at error level")

		output := buf.String()
		require.NotEmpty(t, output)
		assert.Contains(t, output, "fatal visible at error level")
	})

	t.Run("not_logged_when_level_is_above_fatal", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
			Level: slog.Level(13),
		})
		slog.SetDefault(slog.New(handler))
		t.Cleanup(func() {
			slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
		})

		ctx := context.Background()
		Fatal(ctx, "should not appear")

		assert.Empty(t, buf.String())
	})
}
