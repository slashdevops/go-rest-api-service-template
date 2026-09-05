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
--

-- +goose Down
-- +goose StatementBegin

-- CASCADE because these tables reference one another and the drop order would otherwise
-- have to be maintained by hand; dropping a table takes its indexes and triggers with it.
-- The trigger FUNCTIONS are schema-level objects and survive the table, so they are named.

DROP TABLE IF EXISTS users_roles, roles_policies, policies, resources, roles CASCADE;


-- +goose StatementEnd
