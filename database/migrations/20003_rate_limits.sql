-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table rate_limits
--
-- One row is one RULE: where it applies (target_kind + target + methods), who it applies to
-- (audience), what it buckets on (scope), and how it is enforced (strategy). How MUCH it allows
-- lives in rate_limit_windows, because a rule may carry several windows at once -- "10 per second
-- AND 300 per minute".
--
-- NO FOREIGN KEY TO resources, deliberately. Those rows are system = TRUE and are regenerated
-- wholesale whenever swagger changes; an FK would make a rate-limit row BLOCK that regeneration.
-- It also could not express the '*' global default. The target is validated against the endpoint
-- catalogue on write instead, so a rule for a route that does not exist is a 400 at write time
-- rather than a rule that silently matches nothing.
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rate_limits (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,

    -- how `target` is matched. 'tag' is deliberately absent: it is the one kind that cannot be
    -- validated against the resources catalogue, and prefix covers most of the same ground.
    target_kind VARCHAR(20) NOT NULL,
    -- '/projects/{project_id}/generate' | '/projects/' | '*'
    target VARCHAR(255) NOT NULL,

    -- a SET of verbs, or {'*'} for any. Naming a verb beats '*' at the same tier of the ladder.
    -- The bucket key does NOT expand by verb: a {GET,POST} rule is ONE budget shared across both.
    methods TEXT[] NOT NULL,

    -- 'ip' | 'user' | 'token' | 'project' | 'global' -- what the bucket is keyed on
    scope VARCHAR(20) NOT NULL,
    -- 'any' | 'guest' | 'auth' -- orthogonal to scope
    audience VARCHAR(20) NOT NULL DEFAULT 'any',

    -- 'token_bucket' | 'leaky_bucket'
    --
    -- DEFAULT matters more than it looks: it is what keeps every existing rule, and every row a
    -- migration seeds, behaving exactly as it does today.
    --
    -- The two admit IDENTICALLY at equal parameters -- they are duals, measured. This column
    -- records which question the operator was asking (a budget, or a pace) and picks the
    -- parameterisation that says it. It is not a behavioural switch, and the UI must not sell it
    -- as one.
    strategy VARCHAR(20) NOT NULL DEFAULT 'token_bucket',

    enabled BOOLEAN NOT NULL DEFAULT TRUE,

    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT unique_rate_limit_name UNIQUE (name),

    -- The enumerations are CHECKed in the database as well as in domain.Validate(). The Go check
    -- is the one that produces a good error message; this one is what stops a hand-edited row, a
    -- restored backup or a future migration putting a value in that the limiter cannot build.
    CONSTRAINT chk_rate_limits_target_kind CHECK (target_kind IN ('endpoint', 'prefix', 'global')),
    CONSTRAINT chk_rate_limits_scope CHECK (scope IN ('ip', 'user', 'token', 'project', 'global')),
    CONSTRAINT chk_rate_limits_audience CHECK (audience IN ('any', 'guest', 'auth')),
    CONSTRAINT chk_rate_limits_strategy CHECK (strategy IN ('token_bucket', 'leaky_bucket')),

    -- '*' is only meaningful for the global kind, and a global rule cannot target anything else.
    -- Without this a row can exist that matches nothing and looks correct in a list.
    CONSTRAINT chk_rate_limits_global_target CHECK (
        (target_kind = 'global' AND target = '*') OR (target_kind <> 'global' AND target <> '*')
    ),

    -- methods is either exactly {'*'} or a list that does not contain it. Allowing {'*','GET'}
    -- would make the precedence ladder ambiguous at the same tier.
    CONSTRAINT chk_rate_limits_methods CHECK (
        cardinality(methods) > 0
        AND (methods = ARRAY['*']::TEXT[] OR NOT ('*' = ANY (methods)))
    ),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- Pagination, the same shape every other table uses.
CREATE INDEX "idx_rate_limits_pagination" ON rate_limits (serial_id, id);

-- No index on enabled, target or scope. The mirror loads the whole table once per reload and
-- answers every request from memory, so there is no per-request query to serve -- and the table is
-- expected to hold tens of rows, not thousands. Indexing them would be the speculative indexing
-- this schema removed 130 of.

CREATE TRIGGER tr_restrict_delete_update_on_system_rate_limits
BEFORE DELETE OR UPDATE ON rate_limits
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

---------------------------------------------------------------------------------------------------
-- table rate_limit_windows
--
-- The budget, one row per window. A rule with three windows -- 10/s, 300/min, 1000/h -- has three
-- rows here, and all three are evaluated SHORTEST PERIOD FIRST so a long window's budget is not
-- spent on requests a short one would have refused.
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS rate_limit_windows (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    rate_limits_id uuid NOT NULL REFERENCES rate_limits (id) ON DELETE CASCADE ON UPDATE CASCADE,

    -- the budget, and the window it applies to. requests + period rather than a float rate: it is
    -- how an operator states it, it is what a form collects, and the rate is derived. Storing the
    -- float makes every UI guess at the window it came from.
    requests INTEGER NOT NULL,
    period_seconds INTEGER NOT NULL,

    -- capacity. 0 means "same as requests", which is the sensible default for a token bucket and
    -- the reason a leaky-bucket rule usually sets it to 1.
    burst INTEGER NOT NULL DEFAULT 0,

    -- The shared trigger function reads OLD.system by name, so a table carrying that trigger MUST
    -- have this column. Without it the trigger raises `record "old" has no field "system"` -- and
    -- because the cascade from rate_limits fires it, DELETING ANY RULE THAT HAS WINDOWS FAILS.
    -- Verified: it does, with exactly that error.
    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Two windows on the same period is a contradiction, not a preference. In the database and not
    -- only in a validator, because a validator is not what a restored backup goes through.
    CONSTRAINT unique_rate_limit_window_period UNIQUE (rate_limits_id, period_seconds),

    CONSTRAINT chk_rate_limit_windows_requests CHECK (requests > 0),
    CONSTRAINT chk_rate_limit_windows_period CHECK (period_seconds > 0 AND period_seconds <= 86400),
    CONSTRAINT chk_rate_limit_windows_burst CHECK (burst >= 0),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- The foreign key's leading column. Without it, deleting a rate_limits row scans this whole table
-- to check the cascade.
CREATE INDEX "idx_rate_limit_windows_rate_limits_id" ON rate_limit_windows (rate_limits_id);

CREATE TRIGGER tr_restrict_delete_update_on_system_rate_limit_windows
BEFORE DELETE OR UPDATE ON rate_limit_windows
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

---------------------------------------------------------------------------------------------------
-- The default rule.
--
-- system = FALSE, deliberately, and this is a reversal worth stating.
--
-- The obvious move is system = TRUE so a deployment cannot delete its way to having no rule at all.
-- But the shared trigger refuses UPDATE as well as DELETE, so a system default rule would be
-- permanently UN-TUNABLE: an operator who wants a different global ceiling could neither edit this
-- row nor add a second global rule without making the global tier ambiguous.
--
-- The flags are the better floor. `http.server.ip.rate.limiter.*` applies when no rule matches, when
-- no rule can be loaded, and when the database is unreachable -- three cases, where a seeded row
-- covers only the first and cannot cover the last at all. So this row is an editable starting point
-- whose numbers happen to equal the flag defaults, not a guarantee; the guarantee lives in code.
---------------------------------------------------------------------------------------------------
INSERT INTO rate_limits (id, name, description, target_kind, target, methods, scope, audience, strategy, enabled, system)
VALUES (
    '01a03000-0000-7000-8000-000000000001',
    'Default per-IP limit',
    'Applies to every endpoint that no more specific rule matches. Mirrors the shipped http.server.ip.rate.limiter.* defaults, so a deployment with no other rules behaves as it did before rules existed. Editable: the flags, not this row, are what guarantees a limit exists.',
    'global', '*', ARRAY['*']::TEXT[], 'ip', 'any', 'token_bucket', TRUE, FALSE
);

INSERT INTO rate_limit_windows (id, rate_limits_id, requests, period_seconds, burst)
VALUES (
    '01a03000-0000-7000-8000-000000000002',
    '01a03000-0000-7000-8000-000000000001',
    100, 1, 300
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- The child table references rate_limits, so it goes first. Dropping the parent alone fails with
-- "cannot drop table rate_limits because other objects depend on it" and breaks the whole down
-- chain -- which is how the previous generation of down migrations in this repo were fiction.
--
-- The triggers and indexes go with their tables; the shared function is not ours to drop.
--
-- Nothing seeded here is system = TRUE, so there is no trigger to work around. DROP TABLE is not a
-- DELETE and would not be blocked by a row-level trigger in any case -- unlike a down migration
-- that removes seed rows from a table it keeps, which must ALTER TABLE ... DISABLE TRIGGER first.
DROP TABLE IF EXISTS rate_limit_windows;
DROP TABLE IF EXISTS rate_limits;

-- +goose StatementEnd
