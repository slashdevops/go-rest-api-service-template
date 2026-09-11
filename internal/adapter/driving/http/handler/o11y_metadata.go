package handler

import "github.com/slashdevops/go-rest-api-service-template/internal/o11y"

// AppLayer is the value this package reports as app.layer.
//
// "handler" names the driving port. A gRPC handler would report the same layer;
// the protocol is an attribute on the span (http.route and friends), not a
// layer of its own. See [o11y.LayerHandler].
const AppLayer = o11y.LayerHandler
