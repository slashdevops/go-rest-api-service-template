-- +goose Up
-- +goose StatementBegin

-- table users
INSERT INTO users (id, first_name, last_name, email, password_hash, disabled, admin, local_account) VALUES
-- user Administrator
('019822af-b448-73fb-89a1-447e8f8d1cde',
 'Administrator',
 'Default',
 'admin@goapitemplate.local',
 '$2a$10$IqIoI8R.vDCRQw5Pceq6w..qKdeklXJYCR5U0nJSvN4jTIaXzm8Gm', -- password is 'ThisIsApassw0rd.,' hashed with bcrypt and salt
 FALSE,
 TRUE,
 TRUE);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- `user` is a reserved word and the table is `users`; `DELETE FROM user` was a syntax error,
-- so this down migration could never run.
DELETE FROM users;

-- +goose StatementEnd
