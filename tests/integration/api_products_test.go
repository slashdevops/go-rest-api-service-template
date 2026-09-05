//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
)

var (
	productsListAllEndpoint = newAPIEndpoint(http.MethodGet, "/products")
	productsListEndpoint    = newAPIEndpoint(http.MethodGet, "/projects/{project_id}/products")
	productsCreateEndpoint  = newAPIEndpoint(http.MethodPost, "/projects/{project_id}/products")
	productsGetEndpoint     = newAPIEndpoint(http.MethodGet, "/projects/{project_id}/products/{product_id}")
	productsUpdateEndpoint  = newAPIEndpoint(http.MethodPut, "/projects/{project_id}/products/{product_id}")
	productsDeleteEndpoint  = newAPIEndpoint(http.MethodDelete, "/projects/{project_id}/products/{product_id}")
)

// createProduct creates a product over the API and returns its id.
func createProduct(t *testing.T, accessToken string, projectID uuid.UUID, name, description string) uuid.UUID {
	t.Helper()

	productID := uuid.NewV7()

	endpoint := productsCreateEndpoint.RewriteSlugs(projectID.String())

	response, err := sendHTTPRequest(t, t.Context(), endpoint, map[string]any{
		"id":          productID.String(),
		"name":        name,
		"description": description,
	}, map[string]string{"Authorization": "Bearer " + accessToken})
	require.NoError(t, err, "Error creating the product")

	defer response.Body.Close()

	require.Equal(t, http.StatusCreated, response.StatusCode,
		"Expected the product to be created. Got %d. Message: %s", response.StatusCode, readResponseBody(t, response))

	return productID
}

// loginAndGetAccessToken enables a freshly created user, gives it the ordinary
// authenticated role, and logs it in over the API.
func loginAndGetAccessToken(t *testing.T, userID uuid.UUID, email, password string) string {
	t.Helper()

	enableUserByEmailFromDB(t, email)
	assignRoleToUserInDB(t, userID, "AuthenticatedUser")

	response, err := sendHTTPRequest(t, t.Context(), newAPIEndpoint(http.MethodPost, "/auth/login"), map[string]any{
		"email":    email,
		"password": password,
	})
	require.NoError(t, err, "Error logging in the test user")

	defer response.Body.Close()

	require.Equal(t, http.StatusOK, response.StatusCode,
		"Expected the login to succeed. Message: %s", readResponseBody(t, response))

	out, err := parserResponseBody[payload.LoginUserResponse](t, response)
	require.NoError(t, err, "Error parsing the login response")

	return out.AccessToken
}

// TestProductsCRUD covers the ordinary lifecycle of the worked-example entity.
func TestProductsCRUD(t *testing.T) {
	adminToken := getAdminUserTokens(t).AccessToken
	accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken}

	project := createProjectInDB(t, generateRandomName(t, "products_crud"), "project for the products CRUD test")

	t.Run("create_then_get_returns_what_was_written", func(t *testing.T) {
		name := generateRandomName(t, "product")
		productID := createProduct(t, adminToken, project.ID, name, "a product for the CRUD test")

		endpoint := productsGetEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Error getting the product")

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected the product to be found")

		got, err := parserResponseBody[payload.ProductResponse](t, response)
		require.NoError(t, err, "Error parsing the get response")

		assert.Equal(t, productID, got.ID, "The returned id must be the one created")
		assert.Equal(t, name, got.Name, "The returned name must be the one created")
		assert.Equal(t, "a product for the CRUD test", got.Description)
		assert.Equal(t, project.ID, got.Project.ID, "The product must report its owning project")
	})

	t.Run("update_changes_only_the_named_fields", func(t *testing.T) {
		productID := createProduct(t, adminToken, project.ID, generateRandomName(t, "product"), "before")

		endpoint := productsUpdateEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint,
			map[string]any{"description": "after"}, accessTokenHeader)
		require.NoError(t, err, "Error updating the product")

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode,
			"Expected the product to be updated. Message: %s", readResponseBody(t, response))

		getEndpoint := productsGetEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		getResponse, err := sendHTTPRequest(t, t.Context(), getEndpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer getResponse.Body.Close()

		got, err := parserResponseBody[payload.ProductResponse](t, getResponse)
		require.NoError(t, err)

		assert.Equal(t, "after", got.Description, "The description must have been updated")
		assert.NotEmpty(t, got.Name, "The name must be untouched by a description-only update")
	})

	t.Run("delete_removes_it_and_a_second_delete_is_404", func(t *testing.T) {
		productID := createProduct(t, adminToken, project.ID, generateRandomName(t, "product"), "to be deleted")

		endpoint := productsDeleteEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Error deleting the product")

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected the product to be deleted")

		// The second delete must report not-found rather than succeeding
		// silently: the use-case decrements the resource counter on a
		// successful delete, so a no-op that answered 200 would hand back a
		// slot nothing had taken.
		second, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer second.Body.Close()

		assert.Equal(t, http.StatusNotFound, second.StatusCode,
			"Deleting an already-deleted product must be 404, not a silent success")
	})

	t.Run("duplicate_name_in_the_same_project_is_409", func(t *testing.T) {
		name := generateRandomName(t, "product")
		createProduct(t, adminToken, project.ID, name, "the first one")

		endpoint := productsCreateEndpoint.RewriteSlugs(project.ID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, map[string]any{
			"id":          uuid.NewV7().String(),
			"name":        name,
			"description": "the second one",
		}, accessTokenHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		assert.Equal(t, http.StatusConflict, response.StatusCode,
			"A duplicate name within one project must conflict. Message: %s", readResponseBody(t, response))
	})

	t.Run("the_same_name_in_another_project_is_allowed", func(t *testing.T) {
		name := generateRandomName(t, "product")
		createProduct(t, adminToken, project.ID, name, "in the first project")

		other := createProjectInDB(t, generateRandomName(t, "products_other"), "a second project")

		// Uniqueness is (projects_id, name), so this must succeed. If it ever
		// starts failing, the constraint has been widened to the name alone and
		// one tenant's naming has become visible to another as a 409.
		createProduct(t, adminToken, other.ID, name, "in the second project")
	})
}

// TestProductsProjectIsolation is the tenant check.
//
// OPA authorises the PATH, and a policy granting `/projects/*/products` matches
// every project -- so without the membership predicate in the repository, a user
// holding that policy could read and write another project's products. This is
// the test that fails if that predicate is ever dropped.
func TestProductsProjectIsolation(t *testing.T) {
	adminToken := getAdminUserTokens(t).AccessToken

	project := createProjectInDB(t, generateRandomName(t, "products_isolated"), "a project the outsider is not in")
	productID := createProduct(t, adminToken, project.ID, generateRandomName(t, "product"), "not yours")

	// An ordinary user who is a member of no project at all.
	firstName, lastName, email := generateUserData(t)
	password := generatePassword(t)
	outsiderID := createUserInDB(t, firstName, lastName, email, password)
	t.Cleanup(func() { deleteUserByEmailFromDB(t, email) })

	outsiderToken := loginAndGetAccessToken(t, outsiderID, email, password)
	outsiderHeader := map[string]string{"Authorization": "Bearer " + outsiderToken}

	t.Run("an_outsider_cannot_read_the_product", func(t *testing.T) {
		endpoint := productsGetEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, outsiderHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		// 403 from the policy layer or 404 from the membership predicate are
		// both acceptable; 200 is not, and neither is a 500.
		assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, response.StatusCode,
			"An outsider must not be able to read another project's product. Got %d: %s",
			response.StatusCode, readResponseBody(t, response))
	})

	t.Run("an_outsider_cannot_create_in_the_project", func(t *testing.T) {
		endpoint := productsCreateEndpoint.RewriteSlugs(project.ID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, map[string]any{
			"id":          uuid.NewV7().String(),
			"name":        generateRandomName(t, "product"),
			"description": "an outsider should not be able to write this",
		}, outsiderHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, response.StatusCode,
			"An outsider must not be able to create a product in another project. Got %d: %s",
			response.StatusCode, readResponseBody(t, response))
	})

	t.Run("an_outsider_cannot_delete_the_product", func(t *testing.T) {
		endpoint := productsDeleteEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, outsiderHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		assert.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, response.StatusCode,
			"An outsider must not be able to delete another project's product. Got %d: %s",
			response.StatusCode, readResponseBody(t, response))

		// And the product must still be there.
		getEndpoint := productsGetEndpoint.RewriteSlugs(project.ID.String(), productID.String())

		check, err := sendHTTPRequest(t, t.Context(), getEndpoint, nil,
			map[string]string{"Authorization": "Bearer " + adminToken})
		require.NoError(t, err)

		defer check.Body.Close()

		assert.Equal(t, http.StatusOK, check.StatusCode, "The product must survive an outsider's delete attempt")
	})
}

// TestProductsListing covers the shared list contract: filtering, sorting,
// partial fields and pagination.
func TestProductsListing(t *testing.T) {
	adminToken := getAdminUserTokens(t).AccessToken
	accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken}

	project := createProjectInDB(t, generateRandomName(t, "products_list"), "project for the products list test")

	const created = 5
	for i := range created {
		createProduct(t, adminToken, project.ID, generateRandomName(t, fmt.Sprintf("product_%d", i)), "listed")
	}

	t.Run("list_by_project_returns_the_projects_products", func(t *testing.T) {
		endpoint := productsListEndpoint.RewriteSlugs(project.ID.String())

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode)

		out, err := parserResponseBody[payload.ListProductsResponse](t, response)
		require.NoError(t, err)

		assert.Len(t, out.Items, created, "Every created product must be listed")

		for _, item := range out.Items {
			assert.Equal(t, project.ID, item.Project.ID, "A project-scoped list must not leak another project's rows")
		}
	})

	t.Run("limit_paginates_and_the_next_token_advances", func(t *testing.T) {
		endpoint := productsListEndpoint.RewriteSlugs(project.ID.String())
		endpoint.SetQueryParam("limit", "2")

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode)

		first, err := parserResponseBody[payload.ListProductsResponse](t, response)
		require.NoError(t, err)

		require.Len(t, first.Items, 2, "The page must honour the limit")
		require.NotEmpty(t, first.Paginator.NextToken, "There are more rows, so a next token is required")

		next := productsListEndpoint.RewriteSlugs(project.ID.String())
		next.SetQueryParam("limit", "2")
		next.SetQueryParam("next_token", first.Paginator.NextToken)

		nextResponse, err := sendHTTPRequest(t, t.Context(), next, nil, accessTokenHeader)
		require.NoError(t, err)

		defer nextResponse.Body.Close()

		second, err := parserResponseBody[payload.ListProductsResponse](t, nextResponse)
		require.NoError(t, err)

		require.NotEmpty(t, second.Items, "The second page must not be empty")
		assert.NotEqual(t, first.Items[0].ID, second.Items[0].ID, "The second page must not repeat the first")
	})

	t.Run("partial_fields_returns_only_what_was_asked_for", func(t *testing.T) {
		endpoint := productsListEndpoint.RewriteSlugs(project.ID.String())
		endpoint.SetQueryParam("fields", "id,name")

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode)

		out, err := parserResponseBody[payload.ListProductsResponse](t, response)
		require.NoError(t, err)

		require.NotEmpty(t, out.Items)

		for _, item := range out.Items {
			assert.NotEmpty(t, item.Name, "name was requested and must be present")
			assert.Empty(t, item.Description, "description was not requested and must be absent")
		}
	})

	t.Run("price_is_not_a_filterable_field", func(t *testing.T) {
		// The layered implementation advertised `price` and `currency` in its
		// filter and sort lists, but the table has never had either column, so
		// this request parsed and then failed in the database. It must be
		// refused by the parser instead.
		endpoint := productsListEndpoint.RewriteSlugs(project.ID.String())
		endpoint.SetQueryParam("filter", "price gt 10")

		response, err := sendHTTPRequest(t, t.Context(), endpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode,
			"Filtering on a column that does not exist must be a 400 from the parser, not a 500 from the database")
	})

	t.Run("list_all_spans_projects", func(t *testing.T) {
		response, err := sendHTTPRequest(t, t.Context(), productsListAllEndpoint, nil, accessTokenHeader)
		require.NoError(t, err)

		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode)

		out, err := parserResponseBody[payload.ListProductsResponse](t, response)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(out.Items), created, "The unscoped list must see at least this project's products")
	})
}

// TestProductsResourceLimit checks that products are counted against the
// project scope, which is the scope the seeded `products` limit is written for.
func TestProductsResourceLimit(t *testing.T) {
	adminToken := getAdminUserTokens(t).AccessToken

	project := createProjectInDB(t, generateRandomName(t, "products_limit"), "project for the products limit test")

	productsType := domain.ResourcesLimitsResourceTypeProducts.String()

	// A fresh project has no resources_usage row until the first increment
	// creates it, and getUsageFromDB reports "no row" as -1. That is zero
	// usage, not minus one.
	before := max(getUsageFromDB(t, string(domain.ResourcesLimitsScopeTypeProject), project.ID, productsType), 0)

	createProduct(t, adminToken, project.ID, generateRandomName(t, "product"), "counted")

	after := getUsageFromDB(t, string(domain.ResourcesLimitsScopeTypeProject), project.ID, productsType)

	assert.Equal(t, before+1, after, "Creating a product must increment the project's products usage")
}
