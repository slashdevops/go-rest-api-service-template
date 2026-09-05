package driving

import (
	"context"

	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

// ResourcesLimits is the driving port consumed by the HTTP
// resources-limits handler.
//
// Read-only by design. Limits are not editable through the API: today they come
// from the database, and from the signed licence file once that lands, so an
// endpoint that wrote them would be a way around the licence.
type ResourcesLimits interface {
	List(ctx context.Context, input *domain.ListResourcesLimitsInput) (*domain.ListResourcesLimitsOutput, error)

	// StatusByScope reports every limit applying to a scope and what has been
	// consumed against it. The resource types follow from the scope type.
	StatusByScope(ctx context.Context, scope domain.ResourcesLimitsScope) (*domain.ResourcesLimitsScopeStatus, error)
}
