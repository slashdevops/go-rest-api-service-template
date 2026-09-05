//go:build unit

package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeCache stands in for the Valkey adapter. Each test supplies the exact
// Cache.Get behaviour it wants to exercise, including the ones the real
// implementation only reaches on a fault.
type fakeCache struct {
	get func(ctx context.Context, id Identifier, dest any, fetcher Fetcher) error
}

func (f *fakeCache) Get(ctx context.Context, id Identifier, dest any, fetcher Fetcher) error {
	return f.get(ctx, id, dest, fetcher)
}

func (f *fakeCache) Invalidate(context.Context, Identifier) error { return nil }

type entity struct {
	Name string
}

// notFoundError stands in for a domain.*NotFoundError: handlers match on the
// concrete type to choose a status code, so GetTyped must not flatten it.
type notFoundError struct{ id string }

func (e *notFoundError) Error() string { return "entity not found: " + e.id }

const testID = "0198f1d2-0000-7000-8000-000000000001"

func testIdentifier() Identifier {
	return Identifier{Type: "entity", ID: testID}
}

func TestGetTypedCacheDisabled(t *testing.T) {
	t.Parallel()

	t.Run("fetches_directly", func(t *testing.T) {
		t.Parallel()

		calls := 0
		got, err := GetTyped(t.Context(), nil, testIdentifier(),
			func(context.Context) (*entity, []Identifier, error) {
				calls++
				return &entity{Name: "acme"}, nil, nil
			})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got == nil || got.Name != "acme" {
			t.Errorf("got %+v, want &entity{Name: acme}", got)
		}

		if calls != 1 {
			t.Errorf("fetcher called %d times, want 1", calls)
		}
	})

	t.Run("propagates_fetcher_error", func(t *testing.T) {
		t.Parallel()

		want := &notFoundError{id: testID}
		_, err := GetTyped(t.Context(), nil, testIdentifier(),
			func(context.Context) (*entity, []Identifier, error) {
				return nil, nil, want
			})

		if got, ok := errors.AsType[*notFoundError](err); !ok || got != want {
			t.Errorf("got %v, want the fetcher's *notFoundError", err)
		}
	})
}

// TestGetTypedNeverFailsOnCacheFault is the guarantee the package comment
// makes: a fault in the cache costs latency, never a request. Each case is a
// fault the real stack can produce.
func TestGetTypedNeverFailsOnCacheFault(t *testing.T) {
	t.Parallel()

	cacheFault := errors.New("failed to encode data: json: unsupported type: chan int")

	t.Run("encode_failure_still_serves_the_fetched_value", func(t *testing.T) {
		t.Parallel()

		c := &fakeCache{get: func(ctx context.Context, _ Identifier, _ any, fetcher Fetcher) error {
			// The fetcher runs and succeeds; serializing its result fails.
			if _, _, err := fetcher(ctx); err != nil {
				t.Fatalf("fetcher should have succeeded: %v", err)
			}

			return cacheFault
		}}

		got, err := GetTyped(t.Context(), c, testIdentifier(),
			func(context.Context) (*entity, []Identifier, error) {
				return &entity{Name: "acme"}, nil, nil
			})
		if err != nil {
			t.Fatalf("cache fault reached the caller: %v", err)
		}

		if got == nil || got.Name != "acme" {
			t.Errorf("got %+v, want the value the fetcher produced", got)
		}
	})

	t.Run("undecodable_payload_refetches_from_source", func(t *testing.T) {
		t.Parallel()

		// A cache *hit* whose payload no longer decodes — what an encoder or
		// schema change does to a warm cache. The fetcher never runs, so
		// GetTyped has to go to the source itself.
		c := &fakeCache{get: func(context.Context, Identifier, any, Fetcher) error {
			return errors.New("unexpected EOF")
		}}

		calls := 0
		got, err := GetTyped(t.Context(), c, testIdentifier(),
			func(context.Context) (*entity, []Identifier, error) {
				calls++
				return &entity{Name: "acme"}, nil, nil
			})
		if err != nil {
			t.Fatalf("cache fault reached the caller: %v", err)
		}

		if got == nil || got.Name != "acme" {
			t.Errorf("got %+v, want the value the fetcher produced", got)
		}

		if calls != 1 {
			t.Errorf("fetcher called %d times, want 1", calls)
		}
	})

	t.Run("fetcher_error_is_preserved_not_swallowed", func(t *testing.T) {
		t.Parallel()

		want := &notFoundError{id: testID}
		c := &fakeCache{get: func(ctx context.Context, _ Identifier, _ any, fetcher Fetcher) error {
			_, _, err := fetcher(ctx)
			return err
		}}

		_, err := GetTyped(t.Context(), c, testIdentifier(),
			func(context.Context) (*entity, []Identifier, error) {
				return nil, nil, want
			})

		got, ok := errors.AsType[*notFoundError](err)
		if !ok || got != want {
			t.Fatalf("got %v, want the fetcher's *notFoundError unwrapped", err)
		}
	})
}

// TestGetTypedRefusesToCacheNilPointer covers the nil-result guard: a fetcher
// that yields a typed nil must have it returned to the caller and never handed
// to the encoder.
//
// It began as protection against gob, which panics on a nil pointer instead of
// returning an error — and on the stale-while-revalidate path that panic runs
// in a detached goroutine with no recover, killing the process rather than the
// request. gob is gone, but the guard stays: storing a nil is negative caching,
// which this service deliberately does not do.
func TestGetTypedRefusesToCacheNilPointer(t *testing.T) {
	t.Parallel()

	var offered any
	var offeredErr error

	c := &fakeCache{get: func(ctx context.Context, _ Identifier, _ any, fetcher Fetcher) error {
		offered, _, offeredErr = fetcher(ctx)
		return offeredErr
	}}

	got, err := GetTyped(t.Context(), c, testIdentifier(),
		func(context.Context) (*entity, []Identifier, error) {
			return nil, nil, nil // a fetcher that reports "no error, no value"
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}

	if offered != nil {
		t.Errorf("nil pointer was handed to the encoder as %#v; a nil result must not be cached", offered)
	}

	if !errors.Is(offeredErr, errUncacheableNil) {
		t.Errorf("cache was told %v, want errUncacheableNil so it skips the write", offeredErr)
	}
}

// TestGetTypedFetcherIsRaceFree runs the fetcher from a detached goroutine
// while Get returns, which is what the stale-while-revalidate path does. Run
// under -race; an unsynchronised provenance flag fails here.
func TestGetTypedFetcherIsRaceFree(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	c := &fakeCache{get: func(ctx context.Context, _ Identifier, _ any, fetcher Fetcher) error {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = fetcher(context.WithoutCancel(ctx))
		}()

		// Serve a stale entry whose payload does not decode, so GetTyped has
		// to inspect the fetch outcome while the goroutine above writes it.
		return errors.New("unexpected EOF")
	}}

	for range 200 {
		got, err := GetTyped(t.Context(), c, testIdentifier(),
			func(context.Context) (*entity, []Identifier, error) {
				return &entity{Name: "acme"}, nil, nil
			})
		if err != nil {
			t.Fatalf("cache fault reached the caller: %v", err)
		}

		if got == nil || got.Name != "acme" {
			t.Fatalf("got %+v, want the value the fetcher produced", got)
		}
	}

	wg.Wait()
}
