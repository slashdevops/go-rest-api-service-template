//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/repositorypg"
	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driving/http/payload"
	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/usecase"
	"github.com/slashdevops/go-rest-api-service-template/internal/o11y"
)

// The resource-limits subsystem had no test of any kind, which is how two
// silent defects survived: a hardcoded UUID in the default-limit lookup, and
// three resources that checked a limit and then never incremented the counter.
// Both were invisible from the outside — the API kept returning 201 — so the
// tests below assert the counter and the resolved limit directly, not just the
// status code.

var resourcesLimitsListEndpoint = newAPIEndpoint(http.MethodGet, "/resources_limits")

// noopSigner stands in for the service's ECDSA signer in tests that exercise
// the counter arithmetic rather than the integrity check. Signature behaviour
// under concurrency is covered end-to-end through the API instead, where the
// real key is in play.
func noopSigner(int) ([]byte, error) { return nil, nil }

// newTestResourcesLimitsRepository builds the real repository against the test
// database. The scope-resolution SQL is the part that was broken, and it cannot
// be reached over HTTP with an arbitrary scope, so these tests call it directly.
// OpenTelemetry runs with the noop exporters so nothing leaves the process.
func newTestResourcesLimitsRepository(t *testing.T) *repositorypg.ResourcesLimitsRepository {
	t.Helper()

	return newTestResourcesLimitsRepositoryWithOT(t, newTestOpenTelemetry(t))
}

// newTestOpenTelemetry builds OTEL with the noop exporters, so nothing leaves
// the process while tests construct real adapters.
func newTestOpenTelemetry(t *testing.T) *o11y.OpenTelemetry {
	t.Helper()

	otConf := config.NewOpenTelemetryConfig("integration-test", "test")
	otConf.TraceExporter.Value = "noop"
	otConf.MetricExporter.Value = "noop"

	ot, err := o11y.New(t.Context(), otConf)
	require.NoError(t, err, "Failed to build OpenTelemetry for the test")
	require.NoError(t, ot.Start(), "Failed to start OpenTelemetry for the test")
	t.Cleanup(ot.Shutdown)

	return ot
}

func newTestResourcesLimitsRepositoryWithOT(t *testing.T, ot *o11y.OpenTelemetry) *repositorypg.ResourcesLimitsRepository {
	t.Helper()

	repo, err := repositorypg.NewResourcesLimitsRepository(repositorypg.ResourcesLimitsRepositoryConfig{
		DB:              testDBPool,
		OT:              ot,
		MetricsPrefix:   "integration_test",
		MaxPingTimeout:  5 * time.Second,
		MaxQueryTimeout: 10 * time.Second,
	})
	require.NoError(t, err, "Failed to build the resources limits repository")

	return repo
}

// newTestResourcesLimitsService builds the real service on top of the real
// repository, using the same signing keys the running server uses.
//
// Reconciliation cannot be exercised through the API — it has no endpoint — and
// testing it with a stub signer would prove nothing about the property that
// matters: that a repaired counter carries a signature the *server* accepts.
// Sharing the key files is what makes that assertion real.
func newTestResourcesLimitsService(t *testing.T) *usecase.ResourcesLimitsService {
	t.Helper()

	return newTestResourcesLimitsServiceWithKeys(
		t,
		filepath.Join("..", "..", "certs", "jwt.key"),
		filepath.Join("..", "..", "certs", "jwt.pub"),
	)
}

// newTestResourcesLimitsServiceWithKeys builds the service on a named key pair.
func newTestResourcesLimitsServiceWithKeys(t *testing.T, privateKeyFile, publicKeyFile string) *usecase.ResourcesLimitsService {
	t.Helper()

	privateKey, err := os.ReadFile(privateKeyFile)
	require.NoError(t, err, "Failed to read the signing private key")

	publicKey, err := os.ReadFile(publicKeyFile)
	require.NoError(t, err, "Failed to read the signing public key")

	return newTestResourcesLimitsServiceWithPEM(t, privateKey, publicKey)
}

// generateECKeyPairPEM mints a throwaway EC P-256 pair in PEM, the same shape
// the OpenSSL commands in docs/certificates/certificates.md produce.
//
// Generated rather than read from certs/, which is gitignored: a test that
// depends on a file a fresh checkout does not have is a test that fails for
// everyone but its author.
func generateECKeyPairPEM(t *testing.T) (privateKeyPEM, publicKeyPEM []byte) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err, "Failed to generate an EC key")

	privateDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err, "Failed to marshal the private key")

	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err, "Failed to marshal the public key")

	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
}

// newTestResourcesLimitsServiceWithPEM builds the service on the given key
// material, so a test can hold a key the running server does not.
func newTestResourcesLimitsServiceWithPEM(t *testing.T, privateKey, publicKey []byte) *usecase.ResourcesLimitsService {
	t.Helper()

	ot := newTestOpenTelemetry(t)

	service, err := usecase.NewResourcesLimitsService(usecase.ResourcesLimitsServiceConf{
		Repository:    newTestResourcesLimitsRepositoryWithOT(t, ot),
		OT:            ot,
		MetricsPrefix: "integration_test_service",
		PrivateKey:    privateKey,
		PublicKey:     publicKey,
	})
	require.NoError(t, err, "Failed to build the resources limits service")

	return service
}

// countRowsInDB returns a scalar count, for asserting a recount against reality.
func countRowsInDB(t *testing.T, query string, args ...any) int {
	t.Helper()

	var count int
	err := testDBPool.QueryRow(context.Background(), query, args...).Scan(&count)
	require.NoError(t, err, "Failed to count rows")

	return count
}

// setUsageInDB forces a counter to a value, leaving its signature stale. This is
// what drift looks like from the outside: a number nothing in the service put
// there.
func setUsageInDB(t *testing.T, scopeType string, scopeID uuid.UUID, resourceType string, usage int) {
	t.Helper()

	query := `
        INSERT INTO resources_usage (scope_type, scope_id, resource_type, usage)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (scope_type, scope_id, resource_type) DO UPDATE SET usage = EXCLUDED.usage;
    `

	_, err := testDBPool.Exec(context.Background(), query, scopeType, scopeID, resourceType, usage)
	require.NoError(t, err, "Failed to force the usage counter")
}

// insertLimitInDB inserts a limit row and removes it when the test finishes.
func insertLimitInDB(t *testing.T, scopeType string, scopeID uuid.UUID, resourceType string, soft, hard int) {
	t.Helper()

	id := uuid.NewV7()

	query := `
        INSERT INTO resources_limits (id, scope_type, scope_id, resource_type, soft_limit, hard_limit, system)
        VALUES ($1, $2, $3, $4, $5, $6, FALSE);
    `

	_, err := testDBPool.Exec(context.Background(), query, id, scopeType, scopeID, resourceType, soft, hard)
	require.NoError(t, err, "Failed to insert resource limit into database")

	t.Cleanup(func() {
		if _, err := testDBPool.Exec(context.Background(), `DELETE FROM resources_limits WHERE id = $1;`, id); err != nil {
			t.Errorf("Failed to delete resource limit from database: %v", err)
		}
	})
}

// insertUsageInDB inserts a usage row and removes it when the test finishes.
func insertUsageInDB(t *testing.T, scopeType string, scopeID uuid.UUID, resourceType string, usage int) {
	t.Helper()

	query := `
        INSERT INTO resources_usage (scope_type, scope_id, resource_type, usage)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (scope_type, scope_id, resource_type) DO UPDATE SET usage = EXCLUDED.usage;
    `

	_, err := testDBPool.Exec(context.Background(), query, scopeType, scopeID, resourceType, usage)
	require.NoError(t, err, "Failed to insert resource usage into database")

	t.Cleanup(func() { deleteUsageFromDB(t, scopeType, scopeID, resourceType) })
}

func deleteUsageFromDB(t *testing.T, scopeType string, scopeID uuid.UUID, resourceType string) {
	t.Helper()

	query := `DELETE FROM resources_usage WHERE scope_type = $1 AND scope_id = $2 AND resource_type = $3;`
	if _, err := testDBPool.Exec(context.Background(), query, scopeType, scopeID, resourceType); err != nil {
		t.Errorf("Failed to delete resource usage from database: %v", err)
	}
}

// getUsageFromDB returns the stored counter, or -1 when no row exists.
func getUsageFromDB(t *testing.T, scopeType string, scopeID uuid.UUID, resourceType string) int {
	t.Helper()

	query := `SELECT usage FROM resources_usage WHERE scope_type = $1 AND scope_id = $2 AND resource_type = $3;`

	var usage int
	err := testDBPool.QueryRow(context.Background(), query, scopeType, scopeID, resourceType).Scan(&usage)
	if err != nil {
		return -1
	}

	return usage
}

// TestResourcesLimitsScopeResolution covers the three-priority resolution in
// ResourcesLimitsRepository.CheckUsage. Every case uses a synthetic scope_type
// so it cannot collide with a real scope or with another test running in
// parallel.
func TestResourcesLimitsScopeResolution(t *testing.T) {
	t.Run("default_limit_applies_the_callers_own_usage", func(t *testing.T) {
		t.Parallel()

		repo := newTestResourcesLimitsRepository(t)
		scopeType := "test_default_" + mustUUIDString(t)[:8]
		scopeID := uuid.NewV7()

		// A default row for the scope type, and NO row for this specific scope,
		// so resolution must fall through to priority 2.
		insertLimitInDB(t, scopeType, uuid.Nil(), "projects", 10, 12)
		insertUsageInDB(t, scopeType, scopeID, "projects", 7)

		check, err := repo.CheckUsage(
			t.Context(),
			domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeType(scopeType), ID: &scopeID},
			domain.ResourcesLimitsResourceTypeProjects,
		)
		require.NoError(t, err, "CheckUsage should resolve the default limit")

		// This is the regression assertion. The query used to join usage on a
		// hardcoded UUID left over from debugging, so it returned 0 here and no
		// scope relying on a default limit was ever enforced.
		assert.Equal(t, 7, check.Usage, "Default-limit resolution must report the caller's own usage, not zero")
		assert.Equal(t, 10, check.SoftLimit, "Expected the default soft limit")
		assert.Equal(t, 12, check.HardLimit, "Expected the default hard limit")
	})

	t.Run("specific_limit_overrides_the_default", func(t *testing.T) {
		t.Parallel()

		repo := newTestResourcesLimitsRepository(t)
		scopeType := "test_specific_" + mustUUIDString(t)[:8]
		scopeID := uuid.NewV7()

		insertLimitInDB(t, scopeType, uuid.Nil(), "projects", 10, 12)
		insertLimitInDB(t, scopeType, scopeID, "projects", 50, 60)
		insertUsageInDB(t, scopeType, scopeID, "projects", 7)

		check, err := repo.CheckUsage(
			t.Context(),
			domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeType(scopeType), ID: &scopeID},
			domain.ResourcesLimitsResourceTypeProjects,
		)
		require.NoError(t, err, "CheckUsage should resolve the specific limit")

		assert.Equal(t, 7, check.Usage, "Expected the caller's usage")
		assert.Equal(t, 50, check.SoftLimit, "Priority 1 must win over the default row")
		assert.Equal(t, 60, check.HardLimit, "Priority 1 must win over the default row")
	})

	t.Run("no_limit_configured_falls_back_to_unlimited", func(t *testing.T) {
		t.Parallel()

		repo := newTestResourcesLimitsRepository(t)
		scopeType := "test_absent_" + mustUUIDString(t)[:8]
		scopeID := uuid.NewV7()

		check, err := repo.CheckUsage(
			t.Context(),
			domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeType(scopeType), ID: &scopeID},
			domain.ResourcesLimitsResourceTypeProjects,
		)
		require.NoError(t, err, "CheckUsage should return the fallback")

		// Documents today's fail-open behaviour so the change is visible when
		// the license work replaces it with a free tier.
		assert.Equal(t, -1, check.HardLimit, "Expected the unlimited sentinel when nothing is configured")
	})

	t.Run("usage_of_another_scope_does_not_leak", func(t *testing.T) {
		t.Parallel()

		repo := newTestResourcesLimitsRepository(t)
		scopeType := "test_isolation_" + mustUUIDString(t)[:8]
		mine := uuid.NewV7()
		theirs := uuid.NewV7()

		insertLimitInDB(t, scopeType, uuid.Nil(), "projects", 10, 12)
		insertUsageInDB(t, scopeType, theirs, "projects", 9)

		check, err := repo.CheckUsage(
			t.Context(),
			domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeType(scopeType), ID: &mine},
			domain.ResourcesLimitsResourceTypeProjects,
		)
		require.NoError(t, err, "CheckUsage should resolve the default limit")

		assert.Equal(t, 0, check.Usage, "A scope with no usage row must read zero, not another scope's counter")
	})
}

// TestResourcesLimitsIncrementDecrement checks the counter arithmetic directly.
func TestResourcesLimitsIncrementDecrement(t *testing.T) {
	t.Run("increment_then_decrement_returns_to_zero", func(t *testing.T) {
		t.Parallel()

		repo := newTestResourcesLimitsRepository(t)
		scopeType := "test_counter_" + mustUUIDString(t)[:8]
		scopeID := uuid.NewV7()
		scope := domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeType(scopeType), ID: &scopeID}

		t.Cleanup(func() { deleteUsageFromDB(t, scopeType, scopeID, "projects") })

		first, err := repo.IncrementUsage(t.Context(), scope, domain.ResourcesLimitsResourceTypeProjects, noopSigner)
		require.NoError(t, err, "Failed to increment usage")
		assert.Equal(t, 1, first, "First increment should create the row at 1")

		second, err := repo.IncrementUsage(t.Context(), scope, domain.ResourcesLimitsResourceTypeProjects, noopSigner)
		require.NoError(t, err, "Failed to increment usage")
		assert.Equal(t, 2, second, "Second increment should return 2")

		_, err = repo.DecrementUsage(t.Context(), scope, domain.ResourcesLimitsResourceTypeProjects, noopSigner)
		require.NoError(t, err, "Failed to decrement usage")
		_, err = repo.DecrementUsage(t.Context(), scope, domain.ResourcesLimitsResourceTypeProjects, noopSigner)
		require.NoError(t, err, "Failed to decrement usage")

		assert.Equal(t, 0, getUsageFromDB(t, scopeType, scopeID, "projects"), "Expected the counter back at zero")
	})

	t.Run("decrement_does_not_go_negative", func(t *testing.T) {
		t.Parallel()

		repo := newTestResourcesLimitsRepository(t)
		scopeType := "test_floor_" + mustUUIDString(t)[:8]
		scopeID := uuid.NewV7()
		scope := domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeType(scopeType), ID: &scopeID}

		insertUsageInDB(t, scopeType, scopeID, "projects", 0)

		_, err := repo.DecrementUsage(t.Context(), scope, domain.ResourcesLimitsResourceTypeProjects, noopSigner)
		require.NoError(t, err, "Decrementing at zero should not be an error")

		assert.Equal(t, 0, getUsageFromDB(t, scopeType, scopeID, "projects"), "Decrementing at zero must not produce a negative counter")
	})
}

// TestResourcesLimitsEnforcement drives the real API and asserts that a hard
// limit actually blocks creation. The limit is attached to the freshly created
// user's own scope so the test cannot interfere with any other test.
func TestResourcesLimitsEnforcement(t *testing.T) {
	t.Run("failed_creation_releases_the_reservation", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken.AccessToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		userScope := domain.ResourcesLimitsScopeTypeUser.String()
		projectsType := domain.ResourcesLimitsResourceTypeProjects.String()

		t.Cleanup(func() {
			deleteUsageFromDB(t, userScope, adminToken.UserID, projectsType)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		projectName := generateRandomName(t, "release_test")
		projectID := mustUUIDString(t)
		first := map[string]any{
			"id":          projectID,
			"name":        projectName,
			"description": "first project, should succeed",
		}

		firstResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, first, accessTokenHeader)
		require.NoError(t, err, "Failed to create the first project")
		defer firstResponse.Body.Close()
		require.Equal(t, http.StatusCreated, firstResponse.StatusCode,
			"Expected the first project to be created. Body: %s", readResponseBody(t, firstResponse))

		t.Cleanup(func() { deleteProjectFromDBByName(t, projectName) })

		require.Equal(t, 1, getUsageFromDB(t, userScope, adminToken.UserID, projectsType),
			"The successful creation should be counted")

		// Same id: the insert violates the primary key and fails. The slot was
		// already reserved by then, so it has to be handed back or the counter
		// leaks upward on every failed attempt. (Project *names* are not unique
		// — only the id is — so a duplicate name would not fail here.)
		duplicate := map[string]any{
			"id":          projectID,
			"name":        generateRandomName(t, "release_dup"),
			"description": "duplicate id, insert must fail",
		}

		duplicateResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, duplicate, accessTokenHeader)
		require.NoError(t, err, "Failed to send the duplicate creation")
		defer duplicateResponse.Body.Close()

		require.NotEqual(t, http.StatusCreated, duplicateResponse.StatusCode,
			"A duplicate project name must be refused. Body: %s", readResponseBody(t, duplicateResponse))

		assert.Equal(t, 1, getUsageFromDB(t, userScope, adminToken.UserID, projectsType),
			"A failed creation must not leave its reservation behind")
	})

	t.Run("deleting_a_nonexistent_resource_does_not_decrement", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken.AccessToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		userScope := domain.ResourcesLimitsScopeTypeUser.String()
		projectsType := domain.ResourcesLimitsResourceTypeProjects.String()

		t.Cleanup(func() {
			deleteUsageFromDB(t, userScope, adminToken.UserID, projectsType)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		projectName := generateRandomName(t, "phantom_delete")
		project := map[string]any{
			"id":          mustUUIDString(t),
			"name":        projectName,
			"description": "one real project",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to create the project")
		defer createResponse.Body.Close()
		require.Equal(t, http.StatusCreated, createResponse.StatusCode,
			"Expected the project to be created. Body: %s", readResponseBody(t, createResponse))

		t.Cleanup(func() { deleteProjectFromDBByName(t, projectName) })

		require.Equal(t, 1, getUsageFromDB(t, userScope, adminToken.UserID, projectsType),
			"The real creation should be counted")

		// Delete a project that does not exist. Nothing is removed, so nothing
		// may be given back. If the counter drops here, any caller can reset
		// their own usage to zero by deleting random ids and then create past
		// their limit indefinitely.
		for range 3 {
			phantom := projectsDeleteEndpoint.Clone().RewriteSlugs(mustUUIDString(t))

			deleteResponse, err := sendHTTPRequest(t, ctx, phantom, nil, accessTokenHeader)
			require.NoError(t, err, "Failed to send the phantom delete")
			deleteResponse.Body.Close()
		}

		assert.Equal(t, 1, getUsageFromDB(t, userScope, adminToken.UserID, projectsType),
			"Deleting resources that do not exist must not lower the counter")
	})
}

// TestResourcesLimitsUsageTracking is the regression test for the three
// resources that checked a limit and then never incremented their counter, and
// for the two that checked one scope and decremented another.
func TestResourcesLimitsUsageTracking(t *testing.T) {
	t.Run("project_creation_tracks_user_usage", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken.AccessToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		userScope := domain.ResourcesLimitsScopeTypeUser.String()
		projectsType := domain.ResourcesLimitsResourceTypeProjects.String()

		t.Cleanup(func() {
			deleteUsageFromDB(t, userScope, adminToken.UserID, projectsType)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		projectID := uuid.NewV7()
		project := map[string]any{
			"id":          projectID.String(),
			"name":        generateRandomName(t, "limit_project"),
			"description": "project created to assert usage tracking",
		}

		createResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to send request to create project")
		defer createResponse.Body.Close()

		require.Equal(t, http.StatusCreated, createResponse.StatusCode,
			"Expected the project to be created. Body: %s", readResponseBody(t, createResponse))

		assert.Equal(t, 1, getUsageFromDB(t, userScope, adminToken.UserID, projectsType),
			"Creating a project must increment the owner's counter")

		deleteEndpoint := projectsDeleteEndpoint.Clone().RewriteSlugs(projectID.String())
		deleteResponse, err := sendHTTPRequest(t, ctx, deleteEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request to delete project")
		defer deleteResponse.Body.Close()

		assert.Equal(t, 0, getUsageFromDB(t, userScope, adminToken.UserID, projectsType),
			"Deleting a project must decrement the owner's counter")
	})
}

// TestResourcesLimitsReconciliation covers recovery from drift — the only way
// back once a counter has strayed from the resources it is supposed to count.
func TestResourcesLimitsReconciliation(t *testing.T) {
	t.Run("a_repaired_counter_is_usable_by_the_running_server", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		service := newTestResourcesLimitsService(t)

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		userScope := domain.ResourcesLimitsScopeTypeUser.String()
		projectsType := domain.ResourcesLimitsResourceTypeProjects.String()

		t.Cleanup(func() {
			deleteUsageFromDB(t, userScope, adminToken.UserID, projectsType)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		// Force the counter by hand. Its signature is now stale, which is what
		// the server refuses writes on.
		setUsageInDB(t, userScope, adminToken.UserID, projectsType, 7)

		tamperedName := generateRandomName(t, "after_tamper")
		tampered := map[string]any{
			"id":          mustUUIDString(t),
			"name":        tamperedName,
			"description": "should be refused while the signature is stale",
		}

		tamperedResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, tampered, accessTokenHeader)
		require.NoError(t, err, "Failed to send the creation")
		defer tamperedResponse.Body.Close()

		require.NotEqual(t, http.StatusCreated, tamperedResponse.StatusCode,
			"A hand-edited counter must not be accepted. Body: %s", readResponseBody(t, tamperedResponse))

		// Repair. Only the service can do this, because only it can mint a
		// signature the server will accept — no SQL fix exists.
		_, err = service.RecountUsage(
			t.Context(),
			domain.ResourcesLimitsScope{Type: domain.ResourcesLimitsScopeTypeUser, ID: &adminToken.UserID},
			domain.ResourcesLimitsResourceTypeProjects,
		)
		require.NoError(t, err, "Recount should succeed")

		// The real assertion: the *server*, holding its own copy of the key,
		// accepts what the recount wrote. A repair that produced a signature
		// only the repairing process trusted would be no repair at all.
		repairedName := generateRandomName(t, "after_repair")
		repaired := map[string]any{
			"id":          mustUUIDString(t),
			"name":        repairedName,
			"description": "should be accepted once the counter is repaired",
		}

		repairedResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, repaired, accessTokenHeader)
		require.NoError(t, err, "Failed to send the creation")
		defer repairedResponse.Body.Close()

		assert.Equal(t, http.StatusCreated, repairedResponse.StatusCode,
			"Creation must work again after reconciliation. Body: %s", readResponseBody(t, repairedResponse))

		t.Cleanup(func() { deleteProjectFromDBByName(t, repairedName) })
	})
}

// TestResourcesLimitsSignature covers the integrity check: what it catches, and
// what it is allowed to break when it does.
func TestResourcesLimitsSignature(t *testing.T) {
	t.Run("a_zeroed_counter_is_still_verified", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		userScope := domain.ResourcesLimitsScopeTypeUser.String()
		projectsType := domain.ResourcesLimitsResourceTypeProjects.String()

		t.Cleanup(func() {
			deleteUsageFromDB(t, userScope, adminToken.UserID, projectsType)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		// Zero is the value an attacker writes: it buys back the whole quota.
		// Verification used to be skipped whenever the counter was zero, which
		// made this the cheapest possible tamper.
		setUsageInDB(t, userScope, adminToken.UserID, projectsType, 0)

		project := map[string]any{
			"id":          mustUUIDString(t),
			"name":        generateRandomName(t, "zeroed_counter"),
			"description": "should be refused: the zero counter has no valid signature",
		}

		response, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, project, accessTokenHeader)
		require.NoError(t, err, "Failed to send the creation")
		defer response.Body.Close()

		assert.NotEqual(t, http.StatusCreated, response.StatusCode,
			"A zeroed counter must still be verified. Body: %s", readResponseBody(t, response))
	})

	t.Run("a_tampered_counter_blocks_writes_but_not_reads", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		userScope := domain.ResourcesLimitsScopeTypeUser.String()
		projectsType := domain.ResourcesLimitsResourceTypeProjects.String()

		t.Cleanup(func() {
			deleteUsageFromDB(t, userScope, adminToken.UserID, projectsType)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		setUsageInDB(t, userScope, adminToken.UserID, projectsType, 4)

		// Writes are refused …
		blocked := map[string]any{
			"id":          mustUUIDString(t),
			"name":        generateRandomName(t, "tamper_blocked"),
			"description": "should be refused while the counter is untrusted",
		}

		blockedResponse, err := sendHTTPRequest(t, ctx, projectsCreateEndpoint, blocked, accessTokenHeader)
		require.NoError(t, err, "Failed to send the creation")
		defer blockedResponse.Body.Close()

		require.NotEqual(t, http.StatusCreated, blockedResponse.StatusCode,
			"A tampered counter must refuse creation. Body: %s", readResponseBody(t, blockedResponse))

		// … but the tenant can still see their own data. Hard-failing reads on a
		// bad row is what turned a single racy write into a tenant-wide outage
		// before; a bad counter means "cannot be trusted to create", not
		// "cannot be trusted to look".
		listResponse, err := sendHTTPRequest(t, ctx, projectsListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to list projects")
		defer listResponse.Body.Close()

		assert.Equal(t, http.StatusOK, listResponse.StatusCode,
			"Reads must keep working with a tampered counter. Body: %s", readResponseBody(t, listResponse))

		limitsResponse, err := sendHTTPRequest(t, ctx, resourcesLimitsListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to list resource limits")
		defer limitsResponse.Body.Close()

		assert.Equal(t, http.StatusOK, limitsResponse.StatusCode,
			"The limits listing must keep working too. Body: %s", readResponseBody(t, limitsResponse))
	})
}

// TestResourcesLimitsListContract covers what the listing endpoint puts on the
// wire, which the frontend renders directly.
func TestResourcesLimitsListContract(t *testing.T) {
	t.Run("a_zero_counter_is_present_in_the_json", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		scopeID := uuid.NewV7()
		userScope := domain.ResourcesLimitsScopeTypeUser.String()

		insertLimitInDB(t, userScope, scopeID, "projects", 0, 0)
		insertUsageInDB(t, userScope, scopeID, "projects", 0)

		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })

		response, err := sendHTTPRequest(t, ctx, resourcesLimitsListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to list resource limits")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode, "Expected 200 for the listing")

		body := readResponseBody(t, response)

		// `omitempty` on these three dropped a genuine zero from the payload, so
		// the client saw `undefined` where the answer was `0` — "unknown"
		// instead of "none". A hard limit of zero is meaningful too: it means
		// this scope may create nothing at all.
		var parsed struct {
			Items []map[string]any `json:"items"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &parsed), "Failed to parse the listing")

		var found bool
		for _, item := range parsed.Items {
			if item["scope_id"] == scopeID.String() {
				found = true

				assert.Contains(t, item, "usage", "usage must be present even when it is zero")
				assert.Contains(t, item, "soft_limit", "soft_limit must be present even when it is zero")
				assert.Contains(t, item, "hard_limit", "hard_limit must be present even when it is zero")
			}
		}

		assert.True(t, found, "Expected the seeded scope in the listing. Body: %s", body)
	})

	t.Run("usage_is_a_selectable_field", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })

		// The repository could always scan `usage`, but it was missing from the
		// field lists, so the parser rejected the one column a caller is most
		// likely to ask for on its own.
		endpoint := resourcesLimitsListEndpoint.Clone()
		endpoint.SetQueryParam("fields", "scope_id,resource_type,usage")

		response, err := sendHTTPRequest(t, ctx, endpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to list resource limits")
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode,
			"?fields=usage must be accepted. Body: %s", readResponseBody(t, response))
	})
}

var (
	resourcesLimitsMeEndpoint      = newAPIEndpoint(http.MethodGet, "/me/resources_limits")
	resourcesLimitsProjectEndpoint = newAPIEndpoint(http.MethodGet, "/projects/{project_id}/resources_limits")
)

// TestResourcesLimitsStatus covers the read endpoints that tell a caller what
// their limits are and how much they have used.
func TestResourcesLimitsStatus(t *testing.T) {
	t.Run("project_limits_cover_the_project_scoped_resources", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		project := createProjectInDB(t, generateRandomName(t, "status_project"), "project for the status endpoint")

		t.Cleanup(func() {
			deleteProjectByIDFromDB(t, project.ID)
			deleteUserByIDFromDB(t, adminToken.UserID)
		})

		endpoint := resourcesLimitsProjectEndpoint.Clone().RewriteSlugs(project.ID.String())

		response, err := sendHTTPRequest(t, ctx, endpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to request the project resource limits")
		defer response.Body.Close()

		require.Equal(t, http.StatusOK, response.StatusCode,
			"Expected 200. Body: %s", readResponseBody(t, response))

		status, err := parserResponseBody[payload.ResourcesLimitsStatusResponse](t, response)
		require.NoError(t, err, "Failed to parse the status response")

		assert.Equal(t, domain.ResourcesLimitsScopeTypeProject.String(), status.ScopeType)
		assert.Equal(t, project.ID, status.ScopeID)

		types := make([]string, 0, len(status.Resources))
		for _, resource := range status.Resources {
			types = append(types, resource.ResourceType)
		}

		assert.ElementsMatch(t, []string{"models", "embedding_config", "generate_config"}, types,
			"A project scope governs exactly these three")
	})

	t.Run("a_malformed_project_id_is_refused", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })

		endpoint := resourcesLimitsProjectEndpoint.Clone().RewriteSlugs("not-a-uuid")

		response, err := sendHTTPRequest(t, ctx, endpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send the request")
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadRequest, response.StatusCode,
			"A malformed project id must be a 400. Body: %s", readResponseBody(t, response))
	})
}

// TestResourcesLimitsList covers the listing endpoint.
func TestResourcesLimitsList(t *testing.T) {
	t.Run("list_returns_one_row_per_scope", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()

		adminToken := getAdminUserTokens(t)
		require.NotEmpty(t, adminToken.AccessToken, "Admin token should not be empty")

		accessTokenHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

		scopeID := uuid.NewV7()
		userScope := domain.ResourcesLimitsScopeTypeUser.String()

		// A specific limit alongside the seeded default for the same
		// (scope_type, resource_type). The list join used to ignore scope_id,
		// so this one usage row came back twice — once carrying the default
		// row's limits as if they belonged to this scope.
		insertLimitInDB(t, userScope, scopeID, "projects", 50, 60)
		insertUsageInDB(t, userScope, scopeID, "projects", 3)

		t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })

		listResponse, err := sendHTTPRequest(t, ctx, resourcesLimitsListEndpoint, nil, accessTokenHeader)
		require.NoError(t, err, "Failed to send request to list resources limits")
		defer listResponse.Body.Close()

		require.Equal(t, http.StatusOK, listResponse.StatusCode,
			"Expected status code 200 for list. Body: %s", readResponseBody(t, listResponse))

		// The handler answers with the snake_case payload DTO, not the domain type.
		listAPIResp, err := parserResponseBody[payload.ListResourcesLimitsResponse](t, listResponse)
		require.NoError(t, err, "Failed to parse list response body")

		matches := 0
		for _, item := range listAPIResp.Items {
			if item.ScopeID == scopeID && item.ResourceType == "projects" {
				matches++
				assert.Equal(t, 3, item.Usage, "Expected the scope's own usage")
				assert.Equal(t, 50, item.SoftLimit, "Expected the scope's own soft limit, not the default row's")
				assert.Equal(t, 60, item.HardLimit, "Expected the scope's own hard limit, not the default row's")
			}
		}

		assert.Equal(t, 1, matches, "A scope with one usage row must appear exactly once")
	})
}
