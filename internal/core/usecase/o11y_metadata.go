package usecase

import "github.com/slashdevops/go-rest-api-service-template/internal/o11y"

// AppLayer is the value this package reports as app.layer.
//
// It is "usecase", not "service", and not the package's own name by accident:
// the layer names a ROLE in the hexagon, so a dashboard keeps working when the
// package behind the role is replaced. See [o11y.LayerUsecase].
//
// The metric names that used to live here are gone. Both instruments are now
// shared across every layer and built by [o11y.NewLayerMetrics], with the layer
// travelling as an attribute rather than as part of the name.
const AppLayer = o11y.LayerUsecase
