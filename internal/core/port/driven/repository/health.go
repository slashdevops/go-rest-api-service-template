package repository

import "context"

//go:generate go tool mockgen -package=mocks -destination=../../../../../mocks/service/health.go -source=health.go Health

// Health is the driven persistence port for health checks.
type Health interface {
	DriverName() string
	PingContext(ctx context.Context) error
}
