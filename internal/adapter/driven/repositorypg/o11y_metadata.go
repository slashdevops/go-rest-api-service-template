package repositorypg

import "github.com/slashdevops/go-rest-api-service-template/internal/o11y"

// AppLayer is the value this package reports as app.layer.
//
// "repository" names the driven port, not this adapter: swapping PostgreSQL for
// another store must not rename a metric or invalidate a dashboard. See
// [o11y.LayerRepository].
const AppLayer = o11y.LayerRepository
