-- +goose Up
-- +goose StatementBegin

-- table projects
INSERT INTO projects (id, name, description, system, disabled) VALUES
('019822af-b448-7505-9611-a289cbdc4e57',
'Default Project',
'Project created by the system to hold all the default data.',
TRUE,
FALSE);

-- table projects_users
INSERT INTO projects_users (projects_id, users_id) VALUES
('019822af-b448-7505-9611-a289cbdc4e57',
'019822af-b448-73fb-89a1-447e8f8d1cde'); -- Administrator user

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- projects_users first: it references projects. The seeded project is system = TRUE and
-- fn_restrict_delete_update_on_system rejects DELETE as well as UPDATE on such rows.
DELETE FROM projects_users;

ALTER TABLE projects DISABLE TRIGGER tr_restrict_delete_update_on_system_projects;
DELETE FROM projects;
ALTER TABLE projects ENABLE TRIGGER tr_restrict_delete_update_on_system_projects;

-- +goose StatementEnd
