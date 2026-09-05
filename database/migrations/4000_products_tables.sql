-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table products
--
-- The worked example entity. It is deliberately the simplest thing that is still realistic: a
-- project-scoped row with a name, a description and nothing else. Everything a new entity needs --
-- tenant scoping, uniqueness inside the tenant, pagination, the system-row guard, a resource limit
-- -- is here and nowhere else is required to understand it.
--
-- `price` and `currency` are NOT columns, and never were. The layered implementation this was
-- ported from advertised both in its filter and sort field lists, so `?filter=price gt 10` parsed
-- and then generated SQL against a column that does not exist. The field lists in
-- internal/core/domain/products.go now name only what is here.
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS products (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    -- ON DELETE CASCADE: a product cannot outlive its project. The resource-limit counter for the
    -- project is removed by the same cascade (see 20000_limits.sql), so a deleted project does not
    -- leave a counter behind claiming its products still exist.
    projects_id uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE ON UPDATE CASCADE,

    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,

    -- system marks rows that ship with the product. The shared trigger function reads OLD.system by
    -- name, so a table carrying that trigger MUST have this column -- without it the trigger raises
    -- `record "old" has no field "system"` and every DELETE on the table fails.
    system BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Uniqueness is per project, not global: two tenants may both have a "Starter" product, and
    -- making the name globally unique would leak one tenant's naming to another as a 409.
    CONSTRAINT unique_project_product_name UNIQUE (projects_id, name),

    -- serial_id is used for pagination
    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- The foreign key's leading column, and the column every project-scoped query filters on. Without
-- it, deleting a project scans this whole table to check the cascade.
CREATE INDEX "idx_products_projects_id" ON products (projects_id);

-- Pagination, the same shape every other table uses.
CREATE INDEX "idx_products_pagination" ON products (serial_id, id);

-- No index on name, created_at or updated_at. The layered schema created one per column on the
-- assumption that a sortable field needs an index; sorting a tenant's products -- tens of rows,
-- already narrowed by projects_id -- does not. Add one when a query plan asks for it.

CREATE TRIGGER tr_restrict_delete_update_on_system_products
BEFORE DELETE OR UPDATE ON products
FOR EACH ROW
EXECUTE FUNCTION fn_restrict_delete_update_on_system();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- The trigger and indexes go with the table. Nothing seeded here is system = TRUE, so there is no
-- row-level guard to work around -- and DROP TABLE is not a DELETE, so the trigger would not fire
-- in any case.
DROP TABLE IF EXISTS products;

-- +goose StatementEnd
