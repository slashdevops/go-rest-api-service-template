//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	"github.com/slashdevops/c3e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valkey-io/valkey-go"

	"github.com/slashdevops/go-rest-api-service-template/internal/adapter/driven/cachevalkey"
	"github.com/slashdevops/go-rest-api-service-template/internal/config"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/domain"
	"github.com/slashdevops/go-rest-api-service-template/internal/core/port/driven/cache"
)

// The cache tests talk to Valkey directly rather than through the API, because
// the behaviour under test — stale serving, the invalidation cascade, encoder
// compatibility — is not observable from an HTTP response.
//
// They run on their own database index so they can never disturb the cache the
// running service is using. A Valkey server ships with `databases 16`, so 15 is
// the last valid index and 0 is what the service takes by default.
const cacheTestDB = 15

// 6380 is the dev environment's cleartext Valkey port. 6379 carries TLS, which
// the service uses; these tests inspect keys directly and are a harness rather
// than the service, so they take the plain side port.
const defaultCacheTestAddress = "localhost:6380"

// cacheTestAddress resolves the Valkey address the same way the service does,
// so a non-default dev environment does not need this file edited.
func cacheTestAddress() string {
	if addr := strings.TrimSpace(os.Getenv("CACHE_SERVER_ADDRESSES")); addr != "" {
		return strings.Split(addr, ",")[0]
	}

	return defaultCacheTestAddress
}

// requireValkeyAvailable skips when no Valkey is reachable, matching how the
// suite treats Ollama. A cache test that silently passes without a server would
// be worse than no test at all.
func requireValkeyAvailable(t *testing.T) {
	t.Helper()

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{cacheTestAddress()},
		SelectDB:     cacheTestDB,
		DisableRetry: true,
	})
	if err != nil {
		t.Skipf("Skipping cache test: Valkey is not reachable at %s: %v", cacheTestAddress(), err)
		return
	}
	client.Close()
}

// newTestCache builds the same stack app/services.go builds — valkey client →
// c3e.CacheManager → c3e.SafeCacheManager → cachevalkey.Adapter — so these
// tests exercise the real composition rather than a stand-in.
//
// The test database is flushed on entry, not on exit, so a failed run leaves
// its keys behind to inspect.
func newTestCache(t *testing.T, encoder c3e.CacheEncoderType, softTTL, hardTTL time.Duration) cache.Cache {
	t.Helper()

	c, _ := newTestCacheWithClient(t, encoder, softTTL, hardTTL)

	return c
}

// newTestCacheWithClient also hands back the raw client, for the few assertions
// that have to look at what actually landed in Valkey rather than at what the
// port returns.
func newTestCacheWithClient(
	t *testing.T,
	encoder c3e.CacheEncoderType,
	softTTL, hardTTL time.Duration,
) (cache.Cache, valkey.Client) {
	t.Helper()

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:      []string{cacheTestAddress()},
		SelectDB:         cacheTestDB,
		ClientName:       "go-rest-api-service-template-tests",
		DisableRetry:     true,
		ConnWriteTimeout: 2 * time.Second,
	})
	require.NoError(t, err, "connect to Valkey")
	t.Cleanup(client.Close)

	require.NoError(
		t,
		client.Do(t.Context(), client.B().Flushdb().Build()).Error(),
		"flush the cache test database",
	)

	return newCacheOn(t, client, encoder, softTTL, hardTTL), client
}

// newCacheOn builds a cache over an already-connected client and does NOT flush.
// Tests that need two managers with different TTLs over the same data depend on
// that: flushing on the second would erase what the first just wrote.
func newCacheOn(
	t *testing.T,
	client valkey.Client,
	encoder c3e.CacheEncoderType,
	softTTL, hardTTL time.Duration,
) cache.Cache {
	t.Helper()

	manager, err := c3e.NewCacheManager(client, false)
	require.NoError(t, err, "build c3e cache manager")

	safe, err := c3e.NewSafeCacheManager(manager, c3e.SafeCacheManagerConfig{
		HardTTL:       hardTTL,
		SoftTTL:       softTTL,
		JitterPercent: 0,
		EncoderType:   encoder,
		QueryTimeout:  200 * time.Millisecond,
	})
	require.NoError(t, err, "build c3e safe cache manager")

	return cachevalkey.New(safe, cachevalkey.Config{InvalidateTimeout: 5 * time.Second})
}

// defaultTestEncoder resolves the encoder the service actually ships with, so
// this test follows the configured default instead of restating it.
func defaultTestEncoder(t *testing.T) c3e.CacheEncoderType {
	t.Helper()

	switch config.DefaultCacheEncoderType {
	case domain.CacheEncoderTypeJSON:
		return c3e.CacheEncoderTypeJSON
	default:
		t.Fatalf("unknown default cache encoder %q", config.DefaultCacheEncoderType)
		return c3e.CacheEncoderTypeJSON
	}
}

// cacheEncoders is the set of encoders the service supports. It is a map rather
// than a constant because it used to hold two, and a second lossless encoder
// would go straight back in here without reshaping the tests.
func cacheEncoders() map[string]c3e.CacheEncoderType {
	return map[string]c3e.CacheEncoderType{
		"json": c3e.CacheEncoderTypeJSON,
	}
}

// assertRoundTrip caches want under a fresh key, reads it back, and asserts the
// second read never reached the fetcher. The fetcher count is the real
// assertion: a value that round-trips but is never actually stored would still
// compare equal on the way out.
func assertRoundTrip[T any](t *testing.T, c cache.Cache, entityType string, want T) {
	t.Helper()

	var calls atomic.Int32

	id := cache.Identifier{Type: entityType, ID: uuid.NewV7().String()}
	fetcher := func(context.Context) (T, []cache.Identifier, error) {
		calls.Add(1)
		return want, nil, nil
	}

	got, err := cache.GetTyped(t.Context(), c, id, fetcher)
	require.NoError(t, err, "cold read")
	assert.Equal(t, want, got, "cold read value")

	got, err = cache.GetTyped(t.Context(), c, id, fetcher)
	require.NoError(t, err, "warm read")
	assert.Equal(t, want, got, "warm read value")

	assert.Equal(t, int32(1), calls.Load(), "the warm read must be served from the cache")
}

// TestCacheRoundTripsEveryCachedTypeUnderBothEncoders is the regression test for
// the encoder bug that shipped: the default encoder at the time could not
// encode the authorization permissions map, and every authenticated request
// became a 500. It went unnoticed because nothing ran the shipped default and
// nothing tested the types actually cached.
//
// Every type below has a real cache.GetTyped call site in internal/core/usecase.
func TestCacheRoundTripsEveryCachedTypeUnderBothEncoders(t *testing.T) {
	requireValkeyAvailable(t)

	for name, encoder := range cacheEncoders() {
		t.Run(name, func(t *testing.T) {
			c := newTestCache(t, encoder, time.Minute, 5*time.Minute)

			t.Run("authz_permissions_map", func(t *testing.T) {
				// The exact shape pgx produces from the JSONB column: interface
				// values all the way down. This is what the old encoder refused.
				assertRoundTrip(t, c, "authz", map[string]any{
					"projects": []any{"read", "write"},
					"users":    []any{"read"},
					"nested":   map[string]any{"deep": []any{"x"}},
					"scalar":   "value",
					"number":   float64(42),
					"boolean":  true,
					"null":     nil,
				})
			})

			t.Run("user", func(t *testing.T) {
				assertRoundTrip(t, c, "user", &domain.User{
					ID: uuid.NewV7(), FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com",
				})
			})

			t.Run("project", func(t *testing.T) {
				assertRoundTrip(t, c, "project", &domain.Project{
					ID: uuid.NewV7(), Name: "proj", Description: "d",
				})
			})

			t.Run("role", func(t *testing.T) {
				assertRoundTrip(t, c, "role", &domain.Role{ID: uuid.NewV7(), Name: "admin"})
			})

			t.Run("policy", func(t *testing.T) {
				assertRoundTrip(t, c, "policy", &domain.Policy{ID: uuid.NewV7(), Name: "policy"})
			})

			t.Run("idp", func(t *testing.T) {
				assertRoundTrip(t, c, "idp", &domain.IDP{ID: uuid.NewV7(), Name: "idp"})
			})

			t.Run("resource", func(t *testing.T) {
				assertRoundTrip(t, c, "resource", &domain.Resource{ID: uuid.NewV7(), Name: "resource"})
			})

			t.Run("idp_collection", func(t *testing.T) {
				assertRoundTrip(t, c, "idp_collection", &domain.ListIDPsOutput{
					Items: []domain.IDP{{ID: uuid.NewV7(), Name: "one"}},
				})
			})
		})
	}
}

// TestCachePreservesOptionalFields is the regression test for a defect the
// round-trip test above did not catch, because it cached only zero-valued
// pointers.
//
// gob, which this service supported at the time, cannot represent the
// difference between a nil pointer and a pointer to a zero value: it omits any
// field holding its type's zero value, and flattens a pointer to the value
// behind it. So *bool(false) was transmitted as nothing and decoded as nil.
// domain has 40 *bool and 52 *string fields — Admin, Disabled, System,
// LocalAccount — and handlers forward them straight into responses, so the same
// endpoint answered `"admin": null` on a cache hit and `"admin": false` on a
// miss.
//
// json distinguishes the two, and is now the only encoder. This guards the
// property rather than the encoder, so it still earns its place.
func TestCachePreservesOptionalFields(t *testing.T) {
	requireValkeyAvailable(t)

	no, yes := false, true

	t.Run("default_encoder_preserves_the_tri_state", func(t *testing.T) {
		c := newTestCache(t, defaultTestEncoder(t), time.Minute, 5*time.Minute)

		want := &domain.User{
			ID: uuid.NewV7(), Email: "ada@example.com",
			Admin: &no, Disabled: &no, LocalAccount: &yes,
		}

		id := cache.Identifier{Type: "user", ID: want.ID.String()}
		fetcher := func(context.Context) (*domain.User, []cache.Identifier, error) {
			return want, nil, nil
		}

		_, err := cache.GetTyped(t.Context(), c, id, fetcher)
		require.NoError(t, err, "cold read")

		got, err := cache.GetTyped(t.Context(), c, id, fetcher)
		require.NoError(t, err, "warm read")

		require.NotNil(t, got.Admin, "Admin must survive the cache as a pointer to false, not become nil")
		assert.False(t, *got.Admin)
		require.NotNil(t, got.Disabled, "Disabled must survive as a pointer to false")
		assert.False(t, *got.Disabled)
		require.NotNil(t, got.LocalAccount)
		assert.True(t, *got.LocalAccount)
	})
}

// TestCacheDependencyTTLIsNotShortened is the standing reproduction of the
// defect that let a revoked role keep working for hours, fixed upstream in
// c3e v0.0.2.
//
// A reverse-dependency set is shared by every entry that depends on the same
// thing. c3e used to EXPIRE it unconditionally on every write, so the last
// writer won even when its TTL was shorter — and with jitter on a 12h TTL, a
// dependency set could expire more than two hours before an entry still listed
// in it. Invalidate landing in that window found an empty set, cascaded to
// nothing, and reported success.
//
// This lives here rather than only upstream because it is this service's
// authorization correctness that depends on it: the test fails if the c3e pin
// is ever moved back below v0.0.2.
func TestCacheDependencyTTLIsNotShortened(t *testing.T) {
	requireValkeyAvailable(t)

	c, client := newTestCacheWithClient(t, defaultTestEncoder(t), time.Minute, time.Hour)

	roleID := uuid.NewV7().String()

	// c3e's key layout: a reverse-dependency set is "dep:<type>:<id>". Asserting
	// on it means reading the wire format rather than a Go API, which is the
	// only way to see a TTL at all.
	depSetKey := "dep:role:" + roleID

	cacheOne := func(t *testing.T, id cache.Identifier) {
		t.Helper()

		_, err := cache.GetTyped(
			t.Context(), c, id,
			func(context.Context) (map[string]any, []cache.Identifier, error) {
				return map[string]any{"projects": []any{"read"}},
					[]cache.Identifier{{Type: "role", ID: roleID}}, nil
			},
		)
		require.NoError(t, err)
	}

	// A long-lived dependent establishes the set's TTL.
	cacheOne(t, cache.Identifier{Type: "authz", ID: uuid.NewV7().String()})

	ttlAfterLong, err := client.Do(t.Context(),
		client.B().Ttl().Key(depSetKey).Build()).AsInt64()
	require.NoError(t, err, "read dependency set TTL")
	require.Positive(t, ttlAfterLong,
		"the dependency set must carry a TTL, otherwise it leaks when entries expire naturally")

	// A second dependent written through a manager with a much shorter hard TTL.
	// Before the fix this rewrote the shared set's TTL down to a few seconds.
	shortLived := newCacheOn(t, client, defaultTestEncoder(t), time.Second, 3*time.Second)

	_, err = cache.GetTyped(
		t.Context(), shortLived,
		cache.Identifier{Type: "authz", ID: uuid.NewV7().String()},
		func(context.Context) (map[string]any, []cache.Identifier, error) {
			return map[string]any{"projects": []any{"read"}},
				[]cache.Identifier{{Type: "role", ID: roleID}}, nil
		},
	)
	require.NoError(t, err)

	ttlAfterShort, err := client.Do(t.Context(),
		client.B().Ttl().Key(depSetKey).Build()).AsInt64()
	require.NoError(t, err)

	assert.Greater(t, ttlAfterShort, int64(10),
		"a shorter-lived dependent shortened the shared dependency set to %ds (was %ds); "+
			"it would then expire before entries still listed in it, and Invalidate would "+
			"silently cascade to nothing", ttlAfterShort, ttlAfterLong)
}

// TestCacheFaultNeverFailsTheRequest is the invariant the whole layer rests on.
// A value the source of truth produced must reach the caller even when the
// cache cannot store it — this is what turns the class of bug that shipped
// (a 500 on every authenticated request) into a silent extra database read.
func TestCacheFaultNeverFailsTheRequest(t *testing.T) {
	requireValkeyAvailable(t)

	for name, encoder := range cacheEncoders() {
		t.Run(name, func(t *testing.T) {
			c := newTestCache(t, encoder, time.Minute, 5*time.Minute)

			t.Run("unencodable_value_is_still_returned", func(t *testing.T) {
				// A channel cannot be marshalled by encoding/json, so
				// the write fails while the "fetch" succeeded.
				want := map[string]any{"ok": "value", "bad": make(chan int)}

				got, err := cache.GetTyped(
					t.Context(), c,
					cache.Identifier{Type: "unencodable", ID: uuid.NewV7().String()},
					func(context.Context) (map[string]any, []cache.Identifier, error) {
						return want, nil, nil
					},
				)

				require.NoError(t, err, "a cache write failure must not fail the request")
				assert.Equal(t, "value", got["ok"], "the fetched value must be returned intact")
			})

			t.Run("typed_nil_does_not_panic", func(t *testing.T) {
				// A nil result must reach the caller and never be stored:
				// caching it would be negative caching, which this service
				// deliberately does not do.
				var nilUser *domain.User

				got, err := cache.GetTyped(
					t.Context(), c,
					cache.Identifier{Type: "user", ID: uuid.NewV7().String()},
					func(context.Context) (*domain.User, []cache.Identifier, error) {
						return nilUser, nil, nil
					},
				)

				require.NoError(t, err)
				assert.Nil(t, got)
			})
		})
	}
}

// TestCacheStalePathIsRaceFree hammers the stale-while-revalidate path, where
// c3e runs the fetcher on a detached goroutine that outlives Get. The adapter
// used to infer hit-vs-miss with an unsynchronised flag written by that
// goroutine and read by the caller, which the race detector flags.
//
// Run with -race, which the integration suite does; without it this only proves
// the path does not panic or error.
func TestCacheStalePathIsRaceFree(t *testing.T) {
	requireValkeyAvailable(t)

	// A soft TTL this short means every read after the pause below is stale, so
	// every one of them spawns a background refresh.
	c := newTestCache(t, c3e.CacheEncoderTypeJSON, 10*time.Millisecond, 5*time.Minute)

	id := cache.Identifier{Type: "authz", ID: uuid.NewV7().String()}
	fetcher := func(context.Context) (map[string]any, []cache.Identifier, error) {
		// Long enough that the refresh goroutine is still running when Get
		// returns — a sequential loop does not reproduce the race.
		time.Sleep(2 * time.Millisecond)
		return map[string]any{"projects": []any{"read"}}, nil, nil
	}

	_, err := cache.GetTyped(t.Context(), c, id, fetcher)
	require.NoError(t, err, "prime the entry")

	time.Sleep(20 * time.Millisecond) // now past the soft TTL

	const goroutines, iterations = 64, 25

	var wg sync.WaitGroup

	errs := make(chan error, goroutines*iterations)

	for range goroutines {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range iterations {
				if _, err := cache.GetTyped(t.Context(), c, id, fetcher); err != nil {
					errs <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent stale read failed: %v", err)
	}
}

var similaritySearchCacheEndpoint = newAPIEndpoint(http.MethodPost,
	"/projects/{project_id}/embedding_configs/{embedding_config_id}/similarity_search")

// serviceCacheClient connects to the database the *running service* uses, not
// the isolated one the rest of this file writes to. It never flushes: these
// keys belong to the service under test.
func serviceCacheClient(t *testing.T) valkey.Client {
	t.Helper()

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress:  []string{cacheTestAddress()},
		DisableRetry: true,
	})
	require.NoError(t, err, "connect to the service's Valkey")
	t.Cleanup(client.Close)

	return client
}

// TestCacheNeverStoresPasswordHashes is the standing guard for a credential
// leak, and it asserts on the bytes in Valkey rather than on the Go value.
//
// domain.User carries PasswordHash, and SelectByID and SelectByEmail both scan
// it. Those users are cached — so without the strip in
// UsersService.withoutCredentials, bcrypt hashes are written to a store outside
// the database, held for the full hard TTL, and (unless cache.tls.enabled is
// set) reached over a cleartext connection.
//
// Checking the Go value would prove nothing: the point is what was serialised.
//
// Not parallel: it inspects the service's live cache.
func TestCacheNeverStoresPasswordHashes(t *testing.T) {
	ctx := t.Context()

	// Logging in exercises the one path that still reads a hash, and proves the
	// uncached lookup works — if GetByEmailForAuth were reading a stripped
	// user, the compare would run against an empty string and this would fail.
	adminToken := getAdminUserTokens(t)
	authHeader := map[string]string{"Authorization": "Bearer " + adminToken.AccessToken}

	t.Cleanup(func() { deleteUserByIDFromDB(t, adminToken.UserID) })

	// GET /me reads the user by id, which is the cached path.
	resp, err := sendHTTPRequest(t, ctx, meGetEndpoint, nil, authHeader)
	require.NoError(t, err, "GET /me")

	require.Equal(t, http.StatusOK, resp.StatusCode, "GET /me: %s", readResponseBody(t, resp))
	resp.Body.Close()

	client := serviceCacheClient(t)

	var (
		cursor    uint64
		inspected int
	)

	for {
		entry, err := client.Do(
			ctx,
			client.B().Scan().Cursor(cursor).Match("cache:user:*").Count(1000).Build(),
		).AsScanEntry()
		require.NoError(t, err, "scan the cache")

		for _, key := range entry.Elements {
			raw, err := client.Do(ctx, client.B().Get().Key(key).Build()).ToString()
			if err != nil {
				continue // expired between the scan and the read
			}

			// c3e stores a CachedItem wrapper whose payload is base64 inside
			// JSON, so the hash would not be visible in the raw bytes.
			var wrapper struct {
				Data []byte `json:"Data"`
			}

			require.NoError(t, json.Unmarshal([]byte(raw), &wrapper), "decode the wrapper for %s", key)

			payload := string(wrapper.Data)
			inspected++

			// Every bcrypt hash begins with $2a$, $2b$ or $2y$.
			if strings.Contains(payload, "$2") {
				t.Errorf("cached entry %s contains what looks like a bcrypt hash: %s", key, payload)
			}

			if strings.Contains(payload, `"PasswordHash":"$`) {
				t.Errorf("cached entry %s carries a populated PasswordHash", key)
			}
		}

		if entry.Cursor == 0 {
			break
		}

		cursor = entry.Cursor
	}

	require.Positive(t, inspected,
		"no cached users were found, so this asserted nothing; GET /me should have cached one")
}
