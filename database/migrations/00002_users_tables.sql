-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table for users
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    first_name VARCHAR(25) NOT NULL,
    last_name VARCHAR(25) NOT NULL,
    -- RFC 5321's cap, the same one ValidateEmail enforces. At 50 an address the
    -- validator had accepted was refused by the column, which the API could only
    -- report as a 500.
    email VARCHAR(254) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    -- disabled says whether the account may sign in; verified says whether the
    -- address has been proven. They used to be one flag: a registration was
    -- disabled until the verification link was followed, and the service could
    -- not tell that account from one an administrator had switched off. Password
    -- recovery therefore refused both, silently. Registration sets verified FALSE
    -- and disabled TRUE; following the link sets verified TRUE and disabled
    -- FALSE; an administrator only ever touches disabled.
    disabled BOOLEAN DEFAULT TRUE,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    admin BOOLEAN DEFAULT FALSE,
    local_account BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- email is unique
    CONSTRAINT "users_email" UNIQUE (email),

    -- serial_id is used for pagination
    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- indexes for users
CREATE INDEX "idx_users_pagination" ON users (serial_id, id);

-- +goose StatementEnd
--

-- +goose Down
-- +goose StatementBegin

-- CASCADE because these tables reference one another and the drop order would otherwise
-- have to be maintained by hand; dropping a table takes its indexes and triggers with it.
-- The trigger FUNCTIONS are schema-level objects and survive the table, so they are named.

DROP TABLE IF EXISTS users CASCADE;

-- +goose StatementEnd
