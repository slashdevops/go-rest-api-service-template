-- +goose Up
-- +goose StatementBegin

INSERT INTO idp_types (id,name, description, scopes, user_info_api_url,system) VALUES
(
  '0198e2b9-7d06-7d2b-a292-413faa1c7e26',
  'Google', -- this is important because the name is used inside the code in a enumeration to detect the kind of identity provider
  'Google Oauth 2.0 Identity Provider',
  ARRAY['https://www.googleapis.com/auth/userinfo.email','https://www.googleapis.com/auth/userinfo.profile'],
  'https://www.googleapis.com/oauth2/v3/userinfo',
  TRUE
),
(
  '0198e2b9-7d06-7d2f-ae5d-73e16e71cb7f',
  'Github', -- this is important because the name is used inside the code in a enumeration to detect the kind of identity provider
  'Github Oauth 2.0 Identity Provider',
  ARRAY['github:email','github:profile'],
  'https://api.github.com/user',
  TRUE
);

-- +goose StatementEnd
--

-- +goose Down
-- +goose StatementBegin

ALTER TABLE idp_types DISABLE TRIGGER tr_restrict_delete_update_on_system_idp_types;
DELETE FROM idp_types;
ALTER TABLE idp_types ENABLE TRIGGER tr_restrict_delete_update_on_system_idp_types;

-- +goose StatementEnd
