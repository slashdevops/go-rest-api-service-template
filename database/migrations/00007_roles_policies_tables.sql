-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table roles
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS roles (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    system BOOLEAN NOT NULL DEFAULT FALSE,
    auto_assign BOOLEAN NOT NULL DEFAULT FALSE, -- this is used to set the auto_assign role for new users
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- name are unique
    CONSTRAINT "roles_name" UNIQUE (name),

    -- serial_id is used for pagination
    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- indexes for roles
CREATE INDEX "idx_roles_pagination" ON roles (serial_id, id);

-- trigger to restrict delete and update on system roles
CREATE TRIGGER tr_restrict_delete_update_on_system_roles
BEFORE DELETE OR UPDATE ON roles
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

---------------------------------------------------------------------------------------------------
-- table resources
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS resources (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    action VARCHAR(255) NOT NULL,
    resource VARCHAR(512) NOT NULL,
    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- name, action and resource are unique
    CONSTRAINT "resources_name_action_resource" UNIQUE (name, action, resource),

    -- serial_id is used for pagination
    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- indexes for resources
CREATE INDEX "idx_resources_action" ON resources (action);
CREATE INDEX "idx_resources_resource" ON resources (resource);
CREATE INDEX "idx_resources_pagination" ON resources (serial_id, id);

-- trigger to restrict delete and update on system resources
CREATE TRIGGER tr_restrict_delete_update_on_system_resources
BEFORE DELETE OR UPDATE ON resources
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

---------------------------------------------------------------------------------------------------
-- table policies
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS policies (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    resources_id uuid NOT NULL REFERENCES resources (id) ON DELETE CASCADE ON UPDATE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    allowed_action VARCHAR(255) NOT NULL,
    allowed_resource VARCHAR(512) NOT NULL,
    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- name, allowed_action and allowed_resource are unique
    CONSTRAINT "policies_name_allowed_action_allowed_resource" UNIQUE (name, allowed_action, allowed_resource),

    -- serial_id is used for pagination
    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- indexes for policies
CREATE INDEX "idx_policies_resources_id" ON policies (resources_id);
CREATE INDEX "idx_policies_allowed_action" ON policies (allowed_action);
CREATE INDEX "idx_policies_allowed_resource" ON policies (allowed_resource);
CREATE INDEX "idx_policies_pagination" ON policies (serial_id, id);

-- trigger to restrict delete and update on system policies
CREATE TRIGGER tr_restrict_delete_update_on_system_policies
BEFORE DELETE OR UPDATE ON policies
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

---------------------------------------------------------------------------------------------------
-- table roles_policies
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS roles_policies (
    roles_id uuid NOT NULL REFERENCES roles (id) ON DELETE CASCADE ON UPDATE CASCADE,
    policies_id uuid NOT NULL REFERENCES policies (id) ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- PRIMARY KEY
    PRIMARY KEY (roles_id, policies_id)
);

-- indexes for roles_policies
CREATE INDEX "idx_roles_policies_policies_id" ON roles_policies (policies_id);


-- table users_roles
CREATE TABLE IF NOT EXISTS users_roles (
    users_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    roles_id uuid NOT NULL REFERENCES roles (id) ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- PRIMARY KEY
    PRIMARY KEY (users_id, roles_id)
);

-- indexes for users_roles
CREATE INDEX "idx_users_roles_roles_id" ON users_roles (roles_id);

-- +goose StatementEnd

-- The two links that make the bootstrap administrator an administrator:
-- Administrator -> Full Access, and the seeded admin -> Administrator. The
-- rows on either side are system rows, but the LINK never was, so one
-- DELETE /roles/<Administrator>/policies severed every administrator from
-- its only grant, irrecoverably through the API. A link between the seeded
-- ids cannot be deleted; anything else can.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION fn_protect_bootstrap_links()
RETURNS TRIGGER AS $$
BEGIN
    -- Nested, not one AND: PL/pgSQL evaluates the whole condition, and
    -- OLD.users_id on a roles_policies row is an error, not false.
    IF TG_TABLE_NAME = 'roles_policies' THEN
        IF OLD.roles_id = '019822af-b448-750c-ae0d-edaf3aaafc41'
           AND OLD.policies_id = '019822c9-9775-7678-b6ea-5c4701531a00' THEN
            RAISE EXCEPTION 'The Administrator role cannot be unlinked from the Full Access policy.';
        END IF;
    ELSIF TG_TABLE_NAME = 'users_roles' THEN
        IF OLD.users_id = '019822af-b448-73fb-89a1-447e8f8d1cde'
           AND OLD.roles_id = '019822af-b448-750c-ae0d-edaf3aaafc41' THEN
            RAISE EXCEPTION 'The bootstrap administrator cannot be removed from the Administrator role.';
        END IF;
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER tr_protect_bootstrap_roles_policies
BEFORE DELETE ON roles_policies
FOR EACH ROW EXECUTE FUNCTION fn_protect_bootstrap_links();

CREATE TRIGGER tr_protect_bootstrap_users_roles
BEFORE DELETE ON users_roles
FOR EACH ROW EXECUTE FUNCTION fn_protect_bootstrap_links();

--

-- +goose Down
-- +goose StatementBegin

-- CASCADE because these tables reference one another and the drop order would otherwise
-- have to be maintained by hand; dropping a table takes its indexes and triggers with it.
-- The trigger FUNCTIONS are schema-level objects and survive the table, so they are named.

DROP TABLE IF EXISTS users_roles, roles_policies, policies, resources, roles CASCADE;
DROP FUNCTION IF EXISTS fn_protect_bootstrap_links();


-- +goose StatementEnd
