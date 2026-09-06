-- +goose Up
-- +goose StatementBegin

---------------------------------------------------------------------------------------------------
-- table idp_types
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS idp_types (
    id uuid PRIMARY KEY NOT NULL UNIQUE DEFAULT uuidv7(),

    -- The name is matched in Go (domain.IDPTypeName) to pick the claim mapping and the
    -- provider-specific quirks; it is a display name only for anything the code does not know.
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,

    -- HOW the adapter talks to the provider:
    --   'oidc'   OpenID Connect with discovery at <idps.issuer_url>/.well-known/openid-configuration,
    --            PKCE, nonce and an ID token verified against the discovered JWKS. Google, Entra ID,
    --            Okta and any other compliant provider.
    --   'github' plain OAuth2 against GitHub's fixed endpoints; GitHub has no OIDC for users, so the
    --            identity comes from /user and the primary VERIFIED address from /user/emails.
    -- It used to be inferred from the name with a switch in the adapter, which is why a provider the
    -- adapter did not know about could be created and could never sign anybody in.
    kind VARCHAR(20) NOT NULL,

    -- The scopes the adapter asks for. For 'oidc' kinds 'openid' is added by the adapter if missing.
    scopes TEXT[] NOT NULL,

    -- Where the user-info endpoint is. Only the 'github' kind needs it stated: an 'oidc' kind reads
    -- it from discovery, and this is NULL for them.
    user_info_api_url VARCHAR(255),

    -- What to show the operator in the issuer field. Google's issuer is a constant; Entra ID's and
    -- Okta's contain the tenant / org, which is why the issuer lives on the INSTANCE (idps) and
    -- this is only a hint.
    issuer_hint VARCHAR(255),

    system BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT unique_idp_type_name UNIQUE (name),
    CONSTRAINT chk_idp_types_kind CHECK (kind IN ('oidc', 'github')),
    -- a github kind must say where its user-info endpoint is; an oidc kind discovers it
    CONSTRAINT chk_idp_types_user_info CHECK (kind <> 'github' OR user_info_api_url IS NOT NULL),

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

    -- The redirect_uri registered with the provider. It is the FRONTEND's callback route
    -- (https://<frontend>/auth/idp/<idp_id>/callback): the browser lands there, and the frontend
    -- server hands state and code to this API. The API used to be the redirect target itself and
    -- set the frontend's cookies on its own host, which only worked while both shared a hostname.
    callback_url VARCHAR(255) NOT NULL,

    -- For the 'oidc' kind: the issuer whose discovery document describes the provider, and the
    -- value the ID token's iss claim must equal. Google: https://accounts.google.com; Entra ID:
    -- https://login.microsoftonline.com/<tenant-id>/v2.0 (ONE tenant per row, deliberately -- the
    -- issuer is what pins it, and a second tenant is a second row); Okta:
    -- https://<org>.okta.com/oauth2/default. NULL for the 'github' kind.
    issuer_url VARCHAR(255),

    -- the base64 encoded identity provider logo
    logo TEXT,
    -- client id
    client_id VARCHAR(255) NOT NULL,
    -- client secret, encrypted at rest with the AES key; TEXT because the ciphertext is longer than
    -- the secret and Okta's secrets are already 64 characters in the clear
    client_secret TEXT NOT NULL,

    -- A disabled provider stays configured, is listed to admins, and is NOT offered on the login
    -- page nor accepted at the callback. This is how a provider is set up before it goes live.
    enabled BOOLEAN NOT NULL DEFAULT TRUE,

    -- Whether a sign-in from an identity nobody has linked yet may CREATE an account, when the
    -- provider vouches for the email. Off means invite-only: only linked identities sign in.
    auto_provision BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- unique constraint for idp name
    CONSTRAINT unique_idp_name UNIQUE (name),

    serial_id BIGSERIAL NOT NULL UNIQUE
);

-- idps.idp_types is a foreign key with no index, so deleting an idp_types row had to scan this
-- whole table to check it.
CREATE INDEX "idx_idps_idp_types" ON idps (idp_types);

---------------------------------------------------------------------------------------------------
-- table users_identities
--
-- WHICH provider identities belong to which account. The identity is the provider's stable
-- subject (`sub`, or GitHub's numeric id), never the email: an email is a mutable attribute that
-- an Entra admin, a GitHub user or the account holder can change, and matching accounts on it let
-- anyone who controlled an email at ONE provider sign in as the account behind it -- and, until
-- this table existed, silently disable that account's password on the way in.
--
-- An IdP sign-in resolves (idps_id, subject) here first. An unknown identity whose email matches an
-- existing account is REFUSED; the account holder links the provider from their profile while
-- signed in, which is the only moment both sides of the link are proven. An unknown identity with
-- no matching account is created when the provider vouches for the email and the IdP allows
-- auto-provisioning.
---------------------------------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users_identities (
    users_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE,
    idps_id  uuid NOT NULL REFERENCES idps (id) ON DELETE CASCADE ON UPDATE CASCADE,

    -- the provider's stable identifier for the person: OIDC `sub`, GitHub's numeric user id
    subject VARCHAR(255) NOT NULL,

    -- the email the provider reported when the link was made; informational, never matched on
    email VARCHAR(255) NOT NULL,

    linked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- one person per provider identity, and one identity per provider per account
    CONSTRAINT pk_users_identities PRIMARY KEY (idps_id, subject),
    CONSTRAINT unique_users_identities_user_idp UNIQUE (users_id, idps_id)
);

-- "which identities does this account have" is the profile page's question; the primary key
-- already answers "whose identity is this".
CREATE INDEX idx_users_identities_users_id ON users_identities (users_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- idps references idp_types, so it has to go first; dropping idp_types alone failed with
-- "cannot drop table idp_types because other objects depend on it" and broke the whole down chain.
DROP TABLE IF EXISTS users_identities;
DROP TABLE IF EXISTS idps;
DROP TABLE IF EXISTS idp_types;

-- The trigger function is a schema-level object and survives the table it guarded.

-- +goose StatementEnd
