-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table revoked_tokens
--
-- The missing primitive. Until now nothing in the service could revoke anything: logout invalidated
-- two cache entries and returned success while both tokens kept working, and the refresh endpoint's
-- own swagger documented a "revoked" 401 that could not happen.
--
-- WHY POSTGRES AND NOT VALKEY. `cache.enabled=false` is a supported mode, so a cache-backed
-- denylist would silently not exist in a valid configuration — logout would go back to doing
-- nothing. And docs/architecture/caching.md states that a cache fault must never fail a request,
-- which for a revocation check means granting a revoked token. Postgres is a hard dependency of the
-- service, so putting the denylist here gives it the same availability as the service itself, and a
-- cache in front of it can only ever be an optimisation: a miss falls through to the truth.
--
-- IT IS A DENYLIST, NOT A SESSION TABLE. A token that is not named here is valid. That is the
-- invariant the whole design rests on: absence means valid, so losing this table cannot lock anyone
-- out, and a token issued before the table existed still works.
--
-- ROTATION CHANGED THE VOLUME, NOT THE MODEL. Every refresh now revokes the token it consumed and
-- records its successor in `replaced_by`, so the table holds a row per refresh rather than a row
-- per logout. Bounded by the refresh lifetime and swept at `expires_at`, that is roughly
-- (active sessions x refreshes per session) rows -- with a 5m access token and a 24h refresh
-- token, up to ~288 rows per session per day. If that volume ever stops being acceptable, the
-- answer is a session table keyed by a `sid` claim (one row per session, updated in place), which
-- buys row economy by giving up the absence-means-valid invariant above. It was not worth that
-- trade at this size.
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS revoked_tokens (
    -- the jti claim of the revoked token; the whole point of minting one per token
    jti uuid PRIMARY KEY NOT NULL,

    -- the subject the token was issued for. Deliberately NOT a foreign key to users: deleting a
    -- user must not delete their revocations, or removing an account would quietly re-validate
    -- every token it had ever been issued.
    users_id uuid NOT NULL,

    -- when the token would have expired on its own. After this instant the row is dead weight —
    -- the token is refused for being expired, not for being revoked — so it can be swept.
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,

    revoked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- The successor. NULL means the token was revoked outright -- a logout -- and nothing replaced
    -- it. Non-NULL means it was rotated: it was spent on a refresh that issued the token named
    -- here, and the two are links in one chain.
    --
    -- The distinction is what makes reuse detection possible AND safe. Presenting a token that was
    -- revoked outright is just a logged-out client, and answers 401. Presenting one that was
    -- ROTATED means two parties hold the same token -- the legitimate client already spent it, so
    -- whoever is presenting it now copied it. Following replaced_by to the end of the chain finds
    -- the token that is still live, and revoking that ends the session for both of them, because
    -- there is no way to tell the thief from the victim.
    --
    -- Deliberately not a foreign key to revoked_tokens(jti): the successor has no row until it is
    -- itself revoked, so a self-reference would reject every rotation.
    replaced_by uuid
);

-- Supports the sweep of expired rows. The lookup by jti uses the primary key.
CREATE INDEX idx_revoked_tokens_expires_at ON revoked_tokens (expires_at);

-- No index on replaced_by: the chain walk follows it INTO jti, which is the primary key. Indexing
-- the column the walk reads FROM would serve no query this service makes.

-- Answers "end every session for this user", and makes the table readable during an incident.
CREATE INDEX idx_revoked_tokens_users_id ON revoked_tokens (users_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS revoked_tokens CASCADE;

-- +goose StatementEnd
