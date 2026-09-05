-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table projects_users
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS projects_users (
    projects_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    users_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- primary key to ensure unique project-user pairs
    PRIMARY KEY (projects_id, users_id)
);
-- indexes for projects_users
CREATE INDEX "idx_projects_users_users_id" ON projects_users (users_id);

-- -- trigger to ensure projects always have at least one user linked
-- CREATE OR REPLACE FUNCTION fn_ensure_project_has_users()
-- RETURNS TRIGGER AS $$
-- DECLARE
--     remaining_users_count INTEGER;
--     -- admin_users_count INTEGER;
-- BEGIN
--     -- Only check on DELETE operations
--     IF TG_OP = 'DELETE' THEN
--         -- -- First, check if there are any admin users in the system
--         -- -- Admin users have access to all projects via the view
--         -- SELECT COUNT(*) INTO admin_users_count
--         -- FROM users
--         -- WHERE admin = TRUE AND disabled = FALSE;

--         -- -- If there are active admin users, deletion is allowed
--         -- -- because admins have access to all projects
--         -- IF admin_users_count > 0 THEN
--         --     RETURN OLD;
--         -- END IF;

--         -- If no admin users exist, check remaining non-admin users for this project
--         SELECT COUNT(*) INTO remaining_users_count
--         FROM projects_users pu
--         JOIN users u ON pu.users_id = u.id
--         WHERE pu.projects_id = OLD.projects_id
--           AND pu.users_id != OLD.users_id  -- exclude the user being deleted
--           AND u.disabled = FALSE;  -- only count active users

--         -- If no remaining active users, prevent deletion
--         IF remaining_users_count = 0 THEN
--             RAISE EXCEPTION 'Cannot remove the last user from project. Project ID: %', OLD.projects_id;
--         END IF;

--         RETURN OLD;
--     END IF;

--     -- For other operations (INSERT, UPDATE), allow them
--     RETURN COALESCE(NEW, OLD);
-- END;
-- $$ LANGUAGE plpgsql;

-- CREATE TRIGGER tr_ensure_project_has_users
-- BEFORE DELETE ON projects_users
-- FOR EACH ROW
-- EXECUTE FUNCTION fn_ensure_project_has_users();

-- view for optimized user projects retrieval
CREATE OR REPLACE VIEW view_projects_users AS
-- Part 1: Get all projects for admin users.
-- This cross join is small as it only involves admins.
SELECT u.id AS user_id,
       p.id,
       p.name,
       p.description,
       p.disabled,
       p.system,
       p.created_at,
       p.updated_at,
       p.serial_id
FROM users u
CROSS JOIN projects p
WHERE u.admin = TRUE

UNION ALL

-- Part 2: Get specifically assigned projects for non-admin users.
-- This uses standard, efficient JOINs.
SELECT u.id AS user_id,
       p.id,
       p.name,
       p.description,
       p.disabled,
       p.system,
       p.created_at,
       p.updated_at,
       p.serial_id
FROM users u
JOIN projects_users pu ON u.id = pu.users_id
JOIN projects p ON pu.projects_id = p.id
WHERE u.admin = FALSE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- drop view
DROP VIEW IF EXISTS view_projects_users;

-- drop trigger and function for ensuring project has users
DROP TRIGGER IF EXISTS tr_ensure_project_has_users ON projects_users;
DROP FUNCTION IF EXISTS fn_ensure_project_has_users();

-- drop indexes for projects_users
DROP INDEX IF EXISTS "idx_projects_users_users_id";

-- drop projects_users table
DROP TABLE IF EXISTS projects_users;


-- +goose StatementEnd