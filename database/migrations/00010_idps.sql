-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table idp_types
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS idp_types (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    -- name this is important because the name is used inside the code in a enumeration to detect the kind of identity provider
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    scopes TEXT[] NOT NULL,

    -- user info api url:  this is from where we will fetch user information
    user_info_api_url VARCHAR(255) NOT NULL,

    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT unique_idp_type_name UNIQUE (name),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- indexes for idp_types

-- trigger to restrict delete and update on system idp_types
CREATE TRIGGER tr_restrict_delete_update_on_system_idp_types
BEFORE DELETE OR UPDATE ON idp_types
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

---------------------------------------------------------------------------------------------------
-- table idps
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS idps (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    idp_types uuid NOT NULL REFERENCES idp_types (id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,

    -- the callback url defined in the idp oauth2 configuration
    callback_url VARCHAR(255) NOT NULL,
    -- the login url to redirect to after authentication
    login_redirect_url VARCHAR(255) NOT NULL,
    -- the register url to redirect to after registration
    register_redirect_url VARCHAR(255) NOT NULL,
    -- the base64 encoded identity provider logo
    logo TEXT,
    -- client id
    client_id VARCHAR(255) NOT NULL,
    -- client secret
    client_secret VARCHAR(255) NOT NULL,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- unique constraint for idp name
    CONSTRAINT unique_idp_name UNIQUE (name),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- idps.idp_types is a foreign key with no index, so deleting an idp_types row had to scan this
-- whole table to check it.
CREATE INDEX "idx_idps_idp_types" ON idps (idp_types);

-- indexes for idps

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- idps references idp_types, so it has to go first; dropping idp_types alone failed with
-- "cannot drop table idp_types because other objects depend on it" and broke the whole down chain.
DROP TABLE IF EXISTS idps;
DROP TABLE IF EXISTS idp_types;

-- The trigger function is a schema-level object and survives the table it guarded.

-- +goose StatementEnd
