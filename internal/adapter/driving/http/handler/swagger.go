package handler

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
)

// SwaggerHandler handles the swagger UI
type SwaggerHandler struct {
	hf http.HandlerFunc
}

// NewSwaggerHandler creates a new SwaggerHandler
func NewSwaggerHandler() *SwaggerHandler {
	return &SwaggerHandler{
		hf: httpSwagger.WrapHandler,
	}
}

// RegisterRoutes registers the routes for the handler
func (ref *SwaggerHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /swagger/", mdw.ThenFunc(ref.hf))
}
