-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table authn_token_lifetimes
--
-- ONE ROW: how long an access token and a refresh token issued from now on will live. It used to
-- be two startup flags (authn.access.token.duration, authn.refresh.token.duration), which meant a
-- change was a redeploy and nothing checked that the refresh token outlived the access token.
--
-- THE INITIAL VALUES COME FROM HERE. There is no Go fallback and no flag: this migration seeds the
-- row with the shipped defaults, a replica loads it synchronously at startup and refuses to start
-- if it cannot. The authz resource rows for GET/PUT /auth/token_lifetimes are in the roles and
-- policies upsert with every other endpoint's. Same invariant as rate_limits -- if the service is serving, it has lifetimes. The
-- Go constants domain.DefaultAuthn{Access,Refresh}TokenDuration exist for the API's `defaults`
-- field and the docs, and TestSeedTokenLifetimesMatchDomainDefaults fails if they drift from the
-- INSERT below.
--
-- SECONDS, NOT INTERVAL, for the same reason rate_limit_windows.period_seconds is: the API speaks
-- Go duration strings, Go holds a time.Duration, and an integer column round-trips both without a
-- codec in between.
--
-- NO `system` COLUMN. The shared trigger refuses UPDATE on system rows, and this row exists to be
-- updated. The singleton column is what stops a second row: it is always TRUE and UNIQUE, so a
-- second INSERT fails on the constraint rather than leaving two rows for the loader to choose from.
--
-- The CHECK constraints repeat domain.ValidAuthn*Duration in seconds. The Go validator is what
-- produces a readable error; these are what stop a hand-edited row, a restored backup or a future
-- migration putting in a value the service would refuse to start on.
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS authn_token_lifetimes (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    -- 2 minutes .. 48 hours
    access_token_seconds BIGINT NOT NULL,
    -- 12 hours .. 168 hours (7 days)
    refresh_token_seconds BIGINT NOT NULL,

    -- who last changed it; NULL for the seeded row. NOT a foreign key to users: deleting the admin
    -- who set a lifetime must not touch the lifetime.
    updated_by uuid,

    singleton BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_authn_token_lifetimes_singleton CHECK (singleton = TRUE),
    CONSTRAINT unique_authn_token_lifetimes_singleton UNIQUE (singleton),

    CONSTRAINT chk_authn_token_lifetimes_access CHECK (
        access_token_seconds BETWEEN 120 AND 172800
    ),
    CONSTRAINT chk_authn_token_lifetimes_refresh CHECK (
        refresh_token_seconds BETWEEN 43200 AND 604800
    ),
    -- strictly greater: an equal pair leaves no moment at which refreshing is both possible and
    -- useful.
    CONSTRAINT chk_authn_token_lifetimes_order CHECK (
        refresh_token_seconds > access_token_seconds
    )
);

-- The shipped defaults: a 5 minute access token, a 24 hour refresh token. The same numbers the
-- removed flags defaulted to, so an upgrade changes nothing until an operator does.
INSERT INTO authn_token_lifetimes (id, access_token_seconds, refresh_token_seconds)
VALUES ('01a0730a-b5c1-7c7c-9d16-42e9e2f2c3d0', 300, 86400)
ON CONFLICT (singleton) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS authn_token_lifetimes;

-- +goose StatementEnd
