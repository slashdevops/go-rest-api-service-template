package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"uuid"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/middleware"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/respond"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driving"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// ProductsHandlerConf represents the handler for the products.
type ProductsHandlerConf struct {
	Service       driving.Products
	OT            *o11y.OpenTelemetry
	MetricsPrefix string
}

// ProductsHandler represents the handler for the products.
type ProductsHandler struct {
	service         driving.Products
	ot              *o11y.OpenTelemetry
	metrics         *o11y.LayerMetrics
	metricsMetadata o11y.Metadata
	metricsPrefix   string
}

// NewProductsHandler creates a new ProductsHandler.
func NewProductsHandler(conf ProductsHandlerConf) (*ProductsHandler, error) {
	if conf.Service == nil {
		return nil, &domain.InvalidServiceError{Message: "driving.Products is required"}
	}

	if conf.OT == nil {
		return nil, &domain.InvalidOTConfigurationError{Message: "OpenTelemetry is required"}
	}

	ref := &ProductsHandler{
		service:       conf.Service,
		ot:            conf.OT,
		metricsPrefix: conf.MetricsPrefix,
		metricsMetadata: o11y.Metadata{
			Layer:  AppLayer,
			Domain: "Products",
			Action: "NewProductsHandler",
		},
	}

	if conf.MetricsPrefix != "" {
		ref.metricsPrefix = strings.ReplaceAll(conf.MetricsPrefix, "-", "_")
		ref.metricsPrefix += "_"
	}

	callsCounter, err := ref.ot.Metrics.Meter.Int64Counter(
		fmt.Sprintf("%s%s", ref.metricsPrefix, MetricCallsCounterName),
		metric.WithDescription(fmt.Sprintf("Total number of %s calls", AppLayer)),
	)
	if err != nil {
		return nil, err
	}

	callsDuration, err := ref.ot.Metrics.Meter.Float64Histogram(
		fmt.Sprintf("%s%s", ref.metricsPrefix, MetricDurationHistogramName),
		metric.WithDescription(fmt.Sprintf("Duration of %s calls", AppLayer)),
		metric.WithUnit("s"), // Seconds
	)
	if err != nil {
		return nil, err
	}

	ref.metrics = &o11y.LayerMetrics{
		Counter:   callsCounter,
		Histogram: callsDuration,
	}

	return ref, nil
}

// RegisterRoutes registers the routes on the mux.
func (ref *ProductsHandler) RegisterRoutes(mux *http.ServeMux, middlewares ...middleware.Middleware) {
	mdw := middleware.Chain(middlewares...)

	mux.Handle("GET /products", mdw.ThenFunc(ref.list))
	mux.Handle("GET /projects/{project_id}/products", mdw.ThenFunc(ref.listByProjectID))
	mux.Handle("POST /projects/{project_id}/products", mdw.ThenFunc(ref.createByProjectID))
	mux.Handle("GET /projects/{project_id}/products/{product_id}", mdw.ThenFunc(ref.getByIDByProjectID))
	mux.Handle("PUT /projects/{project_id}/products/{product_id}", mdw.ThenFunc(ref.updateByIDByProjectID))
	mux.Handle("DELETE /projects/{project_id}/products/{product_id}", mdw.ThenFunc(ref.deleteByIDByProjectID))
}

// writeProductError maps a service error to a status code.
//
// Six handlers need the same mapping, and the layered implementation repeated
// it inline at each one -- which is how two of them ended up answering 500 for
// a not-found.
//
// It deliberately does NOT map the 409 cases. Only create and update can
// conflict -- on the unique (projects_id, name), or on the project's product
// limit -- and routing every handler through a mapper that could emit 409 made
// GET and DELETE declare a status they can never return.
// TestEverySwaggerStatusIsDeclared catches exactly that, and the honest fix is
// for the two write paths to handle their own conflicts (see writeConflict)
// rather than for the read paths to document a lie.
func (ref *ProductsHandler) writeProductError(
	w http.ResponseWriter, r *http.Request, ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) {
	// A caller outside the project gets the same 404 as a caller asking for a
	// product that does not exist. Distinguishing them would confirm the
	// project's existence to someone with no access to it.
	_, isProductMissing := errors.AsType[*domain.ProductNotFoundError](err)
	_, isProjectMissing := errors.AsType[*domain.ProjectNotFoundError](err)

	if isProductMissing || isProjectMissing {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusNotFound, e.Error())

		return
	}

	_, isInvalidByteSeq := errors.AsType[*domain.InvalidByteSequenceError](err)
	_, isInvalidMsgFmt := errors.AsType[*domain.InvalidMessageFormatError](err)
	_, isUndefCol := errors.AsType[*domain.UndefinedColumnError](err)
	_, isDtMismatch := errors.AsType[*domain.DatatypeMismatchError](err)
	_, isValidation := errors.AsType[*domain.ValidationErrors](err)

	if isInvalidByteSeq || isInvalidMsgFmt || isUndefCol || isDtMismatch || isValidation {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	respond.WriteInternalError(w, r, e)
}

// writeConflict answers the two errors only a write can produce, and reports
// whether it handled the error. Callers that get false fall through to
// [ProductsHandler.writeProductError].
func (ref *ProductsHandler) writeConflict(
	w http.ResponseWriter, r *http.Request, ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue, err error,
) bool {
	_, isLimit := errors.AsType[*domain.ResourcesLimitsHardLimitReachedError](err)
	_, isDuplicate := errors.AsType[*domain.ProductAlreadyExistsError](err)

	if !isLimit && !isDuplicate {
		return false
	}

	e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
	respond.WriteJSONMessage(w, r, http.StatusConflict, e.Error())

	return true
}

// productPathParams pulls the caller and the path UUIDs out of a request.
func (ref *ProductsHandler) productPathParams(
	w http.ResponseWriter, r *http.Request, ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue, withProductID bool,
) (userID, projectID, productID uuid.UUID, ok bool) {
	userID, err := getUserIDFromContext(ctx)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return userID, projectID, productID, false
	}

	projectID, err = parseUUIDQueryParams(r.PathValue("project_id"))
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return userID, projectID, productID, false
	}

	if withProductID {
		productID, err = parseUUIDQueryParams(r.PathValue("product_id"))
		if err != nil {
			e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
			respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

			return userID, projectID, productID, false
		}
	}

	return userID, projectID, productID, true
}

// createByProjectID Create a product
//
//	@ID				01982303-f0f9-7e63-92ba-141813745b01
//	@Summary		Create product
//	@Description	Create a new product inside a project. The name must be unique within the project.
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Param			project_id	path		string							true	"The project id in UUID format"
//	@Param			body		body		payload.CreateProductRequest	true	"Product configuration including name and description"
//	@Success		201			{object}	payload.HTTPMessage				"Product created successfully"
//	@Header			201			{string}	Location						"/projects/{project_id}/products/{id}"	"URI of the created product resource"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid request body or validation error"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage				"Project not found, or the caller has no access to it"
//	@Failure		409			{object}	payload.HTTPMessage				"Product with that name already exists in the project, or the project's product limit is reached"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		413			{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error during product creation"
//	@Router			/projects/{project_id}/products [post]
//	@Security		AccessToken
func (ref *ProductsHandler) createByProjectID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "createByProjectID")
	defer span.End()

	userID, projectID, _, ok := ref.productPathParams(w, r, ctx, span, start, attrs, false)
	if !ok {
		return
	}

	var req payload.CreateProductRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)

		return
	}

	var err error

	req.ID, err = domain.EnsureUUIDV7(req.ID)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	input := &domain.CreateProductInput{
		ID:          req.ID,
		ProjectID:   projectID,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := ref.service.CreateByProjectID(ctx, input); err != nil {
		if ref.writeConflict(w, r, ctx, span, start, attrs, err) {
			return
		}

		ref.writeProductError(w, r, ctx, span, start, attrs, err)

		return
	}

	// Location header is required for RESTful APIs
	respond.SetLocation(w, r, input.ID.String())

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Product created",
		attribute.String("product.id", input.ID.String()))
	respond.WriteJSONMessage(w, r, http.StatusCreated, domain.ProductsProductCreatedSuccessfully)
}

// getByIDByProjectID Get a product
//
//	@ID				01982303-f0f9-7e63-92ba-141813745b02
//	@Summary		Get product
//	@Description	Retrieve a product by its unique identifier within a project.
//	@Tags			Products
//	@Produce		json
//	@Param			project_id	path		string					true	"The project id in UUID format"
//	@Param			product_id	path		string					true	"The product id in UUID format"
//	@Success		200			{object}	payload.ProductResponse	"Product found"
//	@Failure		400			{object}	payload.HTTPMessage		"Invalid identifier"
//	@Failure		401			{object}	payload.HTTPMessage		"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage		"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage		"Product not found, or the caller has no access to the project"
//	@Failure		429			{object}	payload.HTTPMessage		"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage		"Internal server error"
//	@Router			/projects/{project_id}/products/{product_id} [get]
//	@Security		AccessToken
func (ref *ProductsHandler) getByIDByProjectID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "getByIDByProjectID")
	defer span.End()

	userID, projectID, productID, ok := ref.productPathParams(w, r, ctx, span, start, attrs, true)
	if !ok {
		return
	}

	out, err := ref.service.GetByIDByProjectID(ctx, productID, projectID, userID)
	if err != nil {
		ref.writeProductError(w, r, ctx, span, start, attrs, err)
		return
	}

	if err := respond.WriteJSONData(w, http.StatusOK, payload.ProductResponse(*out)); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "get product",
		attribute.String("product.id", out.ID.String()))
}

// updateByIDByProjectID Update a product
//
//	@ID				01982303-f0f9-7e63-92ba-141813745b03
//	@Summary		Update product
//	@Description	Update a product's name or description. Both fields are optional; at least one is required.
//	@Tags			Products
//	@Accept			json
//	@Produce		json
//	@Param			project_id	path		string							true	"The project id in UUID format"
//	@Param			product_id	path		string							true	"The product id in UUID format"
//	@Param			body		body		payload.UpdateProductRequest	true	"Fields to update"
//	@Success		200			{object}	payload.HTTPMessage				"Product updated successfully"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid request body or identifier"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage				"Product not found, or the caller has no access to the project"
//	@Failure		409			{object}	payload.HTTPMessage				"Another product in the project already has that name"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		413			{object}	payload.HTTPMessage				"Request body larger than http.server.max.body.bytes"
//	@Failure		415			{object}	payload.HTTPMessage				"Body not declared as application/json"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/projects/{project_id}/products/{product_id} [put]
//	@Security		AccessToken
func (ref *ProductsHandler) updateByIDByProjectID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "updateByIDByProjectID")
	defer span.End()

	userID, projectID, productID, ok := ref.productPathParams(w, r, ctx, span, start, attrs, true)
	if !ok {
		return
	}

	var req payload.UpdateProductRequest
	if err := decodeJSONBody(r, &req); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteDecodeError(w, r, e)

		return
	}

	if err := req.Validate(); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return
	}

	input := &domain.UpdateProductInput{
		ID:          productID,
		ProjectID:   projectID,
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
	}

	if err := ref.service.UpdateByIDByProjectID(ctx, input); err != nil {
		if ref.writeConflict(w, r, ctx, span, start, attrs, err) {
			return
		}

		ref.writeProductError(w, r, ctx, span, start, attrs, err)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Product updated",
		attribute.String("product.id", productID.String()))
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.ProductsProductUpdatedSuccessfully)
}

// deleteByIDByProjectID Delete a product
//
//	@ID				01982303-f0f9-7e63-92ba-141813745b04
//	@Summary		Delete product
//	@Description	Delete a product from a project.
//	@Tags			Products
//	@Produce		json
//	@Param			project_id	path		string				true	"The project id in UUID format"
//	@Param			product_id	path		string				true	"The product id in UUID format"
//	@Success		200			{object}	payload.HTTPMessage	"Product deleted successfully"
//	@Failure		400			{object}	payload.HTTPMessage	"Invalid identifier"
//	@Failure		401			{object}	payload.HTTPMessage	"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage	"Insufficient permissions"
//	@Failure		404			{object}	payload.HTTPMessage	"Product not found, or the caller has no access to the project"
//	@Failure		429			{object}	payload.HTTPMessage	"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage	"Internal server error"
//	@Router			/projects/{project_id}/products/{product_id} [delete]
//	@Security		AccessToken
func (ref *ProductsHandler) deleteByIDByProjectID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "deleteByIDByProjectID")
	defer span.End()

	userID, projectID, productID, ok := ref.productPathParams(w, r, ctx, span, start, attrs, true)
	if !ok {
		return
	}

	input := &domain.DeleteProductInput{
		ID:        productID,
		ProjectID: projectID,
		UserID:    userID,
	}

	if err := ref.service.DeleteByIDByProjectID(ctx, input); err != nil {
		ref.writeProductError(w, r, ctx, span, start, attrs, err)
		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "Product deleted",
		attribute.String("product.id", productID.String()))
	respond.WriteJSONMessage(w, r, http.StatusOK, domain.ProductsProductDeletedSuccessfully)
}

// listByProjectID List products of a project
//
//	@ID				01982303-f0f9-7e63-92ba-141813745b05
//	@Summary		List products by project
//	@Description	List the products of one project, with filtering, sorting, partial fields and pagination.
//	@Tags			Products
//	@Produce		json
//	@Param			project_id	path		string							true	"The project id in UUID format"
//	@Param			sort		query		string							false	"Comma-separated sort fields, e.g. name ASC"
//	@Param			filter		query		string							false	"Filter expression, e.g. name = 'widget'"
//	@Param			fields		query		string							false	"Comma-separated fields to return"
//	@Param			next_token	query		string							false	"Token for the next page"
//	@Param			prev_token	query		string							false	"Token for the previous page"
//	@Param			limit		query		int								false	"Maximum items to return"
//	@Success		200			{object}	payload.ListProductsResponse	"Products found"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		404			{object}	payload.HTTPMessage				"Project not found, or the caller is not a member of it"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/projects/{project_id}/products [get]
//	@Security		AccessToken
func (ref *ProductsHandler) listByProjectID(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "listByProjectID")
	defer span.End()

	userID, projectID, _, ok := ref.productPathParams(w, r, ctx, span, start, attrs, false)
	if !ok {
		return
	}

	input, ok := ref.listInput(w, r, ctx, span, start, attrs)
	if !ok {
		return
	}

	out, err := ref.service.ListByProjectID(ctx, projectID, userID, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())

		return
	}

	ref.writeList(w, r, ctx, span, start, attrs, out)
}

// list List all products
//
//	@ID				01982303-f0f9-7e63-92ba-141813745b06
//	@Summary		List products
//	@Description	List products across every project, with filtering, sorting, partial fields and pagination.
//	@Tags			Products
//	@Produce		json
//	@Param			sort		query		string							false	"Comma-separated sort fields, e.g. name ASC"
//	@Param			filter		query		string							false	"Filter expression, e.g. name = 'widget'"
//	@Param			fields		query		string							false	"Comma-separated fields to return"
//	@Param			next_token	query		string							false	"Token for the next page"
//	@Param			prev_token	query		string							false	"Token for the previous page"
//	@Param			limit		query		int								false	"Maximum items to return"
//	@Success		200			{object}	payload.ListProductsResponse	"Products found"
//	@Failure		400			{object}	payload.HTTPMessage				"Invalid query parameters"
//	@Failure		401			{object}	payload.HTTPMessage				"Missing or invalid authentication token"
//	@Failure		403			{object}	payload.HTTPMessage				"Insufficient permissions"
//	@Failure		429			{object}	payload.HTTPMessage				"Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store"
//	@Failure		500			{object}	payload.HTTPMessage				"Internal server error"
//	@Router			/products [get]
//	@Security		AccessToken
func (ref *ProductsHandler) list(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, span, attrs := o11y.SetupTraceHTTP(r, ref.ot.Traces.Tracer, ref.metricsMetadata, "list")
	defer span.End()

	input, ok := ref.listInput(w, r, ctx, span, start, attrs)
	if !ok {
		return
	}

	out, err := ref.service.List(ctx, input)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, httpStatusForListError(err), e.Error())

		return
	}

	ref.writeList(w, r, ctx, span, start, attrs, out)
}

// listInput parses the shared list query parameters.
func (ref *ProductsHandler) listInput(
	w http.ResponseWriter, r *http.Request, ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue,
) (*domain.ListProductsInput, bool) {
	params := map[string]any{
		"sort":      r.URL.Query().Get("sort"),
		"filter":    r.URL.Query().Get("filter"),
		"fields":    r.URL.Query().Get("fields"),
		"nextToken": r.URL.Query().Get("next_token"),
		"prevToken": r.URL.Query().Get("prev_token"),
		"limit":     r.URL.Query().Get("limit"),
	}

	sort, filter, fields, nextToken, prevToken, limit, err := parseListQueryParams(params)
	if err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteJSONMessage(w, r, http.StatusBadRequest, e.Error())

		return nil, false
	}

	return &domain.ListProductsInput{
		Sort:   sort,
		Filter: filter,
		Fields: fields,
		Paginator: domain.Paginator{
			NextToken: nextToken,
			PrevToken: prevToken,
			Limit:     limit,
		},
	}, true
}

// writeList renders a product page.
func (ref *ProductsHandler) writeList(
	w http.ResponseWriter, r *http.Request, ctx context.Context, span trace.Span, start time.Time, attrs []attribute.KeyValue,
	out *domain.ListProductsOutput,
) {
	outResponse := &payload.ListProductsResponse{
		Items:     make([]payload.ProductResponse, len(out.Items)),
		Paginator: out.Paginator,
	}

	for i, product := range out.Items {
		outResponse.Items[i] = payload.ProductResponse(product)
	}

	// Generate the next and previous pages
	location := fmt.Sprintf("http://%s%s", r.Host, r.URL.Path)
	outResponse.Paginator.GeneratePages(location)

	if err := respond.WriteJSONData(w, http.StatusOK, outResponse); err != nil {
		e := o11y.RecordError(ctx, span, start, err, ref.metrics, attrs)
		respond.WriteInternalError(w, r, e)

		return
	}

	o11y.RecordSuccess(ctx, span, start, ref.metrics, attrs, "list products",
		attribute.Int("product.count", len(outResponse.Items)))
}
