-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table resources_limits
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS resources_limits (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    -- scope_type is the resource type (e.g. "system", "user", "project")
    scope_type    VARCHAR(255) NOT NULL,

    -- scope_id is the specific ID for user/project/etc level limits (the zero UUID marks the
    -- default row for a scope type)
    scope_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,

    -- resource_type is the specific resource being limited (e.g. "users", "projects", "products")
    resource_type    VARCHAR(255) NOT NULL,

    -- soft_limit is the maximum limit that can be exceeded temporarily
    soft_limit INT NOT NULL,

    -- hard_limit is the absolute maximum limit that cannot be exceeded
    hard_limit INT NOT NULL,

    -- system represent it is a system create record
    system BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- unique resource limit constraint (includes scope_id for ID-based scoping)
    CONSTRAINT unique_resource_limit UNIQUE (scope_type, scope_id, resource_type),

    -- -1 is the "no limit configured" sentinel (domain.ResourcesLimitsUnlimited) and is allowed, so
    -- the bounds check has to admit it explicitly rather than simply requiring non-negative values.
    -- A soft limit above its hard limit is a state nothing in the service can produce, which is
    -- exactly why it is worth asserting: if one appears it came from outside the service.
    CONSTRAINT chk_resources_limits_bounds CHECK (
        soft_limit >= -1
        AND hard_limit >= -1
        AND (soft_limit = -1 OR hard_limit = -1 OR soft_limit <= hard_limit)
    ),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- Every lookup in the repository keys on (scope_type, resource_type, scope_id) — the resolution
-- CTE, the counter lock, the recount. The UNIQUE constraint is on (scope_type, scope_id,
-- resource_type), a different column order, so it serves a prefix of that predicate rather than all
-- of it. This index is the one the queries actually use.
CREATE INDEX idx_resources_limits_lookup ON resources_limits (scope_type, resource_type, scope_id);

-- trigger to restrict delete and update on system resources_limits
CREATE TRIGGER tr_restrict_delete_update_on_system_resources_limits
BEFORE DELETE OR UPDATE ON resources_limits
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();


---------------------------------------------------------------------------------------------------
-- table resources_usage
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS resources_usage (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    -- scope_type is the resource type (e.g. "system", "user", "project")
    scope_type    VARCHAR(255) NOT NULL,

    -- scope_id is the specific ID for user/project level usage (the zero UUID for system level)
    scope_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'::uuid,

    -- resource_type is the specific resource being used (e.g. "users", "projects", "products")
    resource_type VARCHAR(255) NOT NULL,

    -- usage is the current usage of the resource
    usage INT NOT NULL DEFAULT 0,

    -- signature is the signature of the record to ensure the integrity of the resource limits
    -- this is combination of inputs in the source code signed with a private key
    signature BYTEA,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- unique constraint for resources_usage (includes scope_id for ID-based scoping)
    CONSTRAINT unique_resources_usage UNIQUE (scope_type, scope_id, resource_type),

    -- A counter below zero is a state nothing in the service can produce. Better to fail the write
    -- than to discover the drift later.
    CONSTRAINT chk_resources_usage_not_negative CHECK (usage >= 0),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- Same reasoning as resources_limits above: one index in the column order the queries use.
CREATE INDEX idx_resources_usage_lookup ON resources_usage (scope_type, resource_type, scope_id);


---------------------------------------------------------------------------------------------------
-- Remove a scope's usage counters when the scope itself is deleted.
--
-- resources_usage.scope_id is polymorphic — it holds a user id, a project id, or the zero UUID for
-- the system scope, depending on scope_type — so it cannot carry a foreign key. Without one,
-- deleting a user or a project would leave its counters behind forever.
--
-- This is done with triggers rather than in the use-cases on purpose. The counters drift precisely
-- because things remove resources without going through the service — direct SQL, an operator
-- cleanup, an ON DELETE CASCADE from some other table. A trigger holds for all of those; an
-- application-level cleanup only holds for the paths someone remembered to change.
--
-- AFTER DELETE, so it runs only once the delete has actually succeeded — in particular after the
-- BEFORE DELETE trigger that refuses to remove system rows.
---------------------------------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION fn_delete_resources_usage_for_user()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM resources_usage
    WHERE scope_type = 'user' AND scope_id = OLD.id;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_delete_resources_usage_for_project()
RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM resources_usage
    WHERE scope_type = 'project' AND scope_id = OLD.id;

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tr_delete_resources_usage_for_user
AFTER DELETE ON users
FOR EACH ROW
EXECUTE FUNCTION fn_delete_resources_usage_for_user();

CREATE TRIGGER tr_delete_resources_usage_for_project
AFTER DELETE ON projects
FOR EACH ROW
EXECUTE FUNCTION fn_delete_resources_usage_for_project();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TRIGGER IF EXISTS tr_delete_resources_usage_for_user ON users;
DROP TRIGGER IF EXISTS tr_delete_resources_usage_for_project ON projects;
DROP FUNCTION IF EXISTS fn_delete_resources_usage_for_user();
DROP FUNCTION IF EXISTS fn_delete_resources_usage_for_project();

-- resources_usage table
DROP INDEX IF EXISTS idx_resources_usage_lookup;
DROP TABLE IF EXISTS resources_usage;

-- resources_limits table
-- NOTE: PostgreSQL takes `DROP INDEX <name>`; the `... ON <table>` suffix is MySQL
-- syntax and made this whole Down block a syntax error, so `goose down` could never run.
DROP TRIGGER IF EXISTS tr_restrict_delete_update_on_system_resources_limits ON resources_limits;

DROP INDEX IF EXISTS idx_resources_limits_lookup;
DROP TABLE IF EXISTS resources_limits;

-- +goose StatementEnd
