-- +goose Up
-- +goose StatementBegin

-- The names are matched in Go (domain.IDPTypeName): the adapter picks the claim mapping and the
-- provider quirks from them. The kind says how to talk to the provider; see 024_idps.sql.
INSERT INTO idp_types (id, name, description, kind, scopes, user_info_api_url, issuer_hint, system) VALUES
(
  '0198e2b9-7d06-7d2b-a292-413faa1c7e26',
  'Google',
  'Google, through OpenID Connect. The issuer is a constant.',
  'oidc',
  ARRAY['openid', 'email', 'profile'],
  NULL,
  'https://accounts.google.com',
  TRUE
),
(
  '0198e2b9-7d06-7d2f-ae5d-73e16e71cb7f',
  'Github',
  'GitHub, through OAuth 2.0. GitHub has no OpenID Connect for users, so the identity is the numeric user id from /user and the email is the primary VERIFIED address from /user/emails.',
  'github',
  -- read:user for the profile, user:email for /user/emails. The old seed carried 'github:email'
  -- and 'github:profile', which are not GitHub scopes at all.
  ARRAY['read:user', 'user:email'],
  'https://api.github.com/user',
  NULL,
  TRUE
),
(
  '01a07340-5f2e-7b0e-8f0d-3a1c2b9e4d51',
  'EntraID',
  'Microsoft Entra ID, through OpenID Connect. One tenant per provider row: the issuer pins the tenant, and emails are trusted as the tenant''s own directory attribute.',
  'oidc',
  ARRAY['openid', 'email', 'profile'],
  NULL,
  'https://login.microsoftonline.com/<tenant-id>/v2.0',
  TRUE
),
(
  '01a07340-5f2e-7b7a-9c42-7e8d1f6a0b23',
  'Okta',
  'Okta, through OpenID Connect against an authorization server. The issuer names the org and the server.',
  'oidc',
  ARRAY['openid', 'email', 'profile'],
  NULL,
  'https://<org>.okta.com/oauth2/default',
  TRUE
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE idp_types DISABLE TRIGGER tr_restrict_delete_update_on_system_idp_types;
DELETE FROM idp_types;
ALTER TABLE idp_types ENABLE TRIGGER tr_restrict_delete_update_on_system_idp_types;

-- +goose StatementEnd
