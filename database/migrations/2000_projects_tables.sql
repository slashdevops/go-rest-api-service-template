-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table projects
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),
    name VARCHAR(70) NOT NULL,
    description TEXT NOT NULL,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    system BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- serial_id is used for pagination
    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- indexes for projects
CREATE INDEX "idx_projects_name" ON projects (name);
CREATE INDEX "idx_projects_pagination" ON projects (serial_id, id);

-- trigger to restrict delete and update on system projects
CREATE TRIGGER tr_restrict_delete_update_on_system_projects
BEFORE DELETE OR UPDATE ON projects
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- CASCADE because these tables reference one another and the drop order would otherwise
-- have to be maintained by hand; dropping a table takes its indexes and triggers with it.
-- The trigger FUNCTIONS are schema-level objects and survive the table, so they are named.

DROP TABLE IF EXISTS projects CASCADE;


-- +goose StatementEnd
