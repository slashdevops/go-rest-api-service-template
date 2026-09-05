


# Go REST API Service Template API
A production-shaped starting point for a Go HTTP REST API: hexagonal architecture, multi-tenant projects, RBAC through Open Policy Agent, database-backed rate limiting and resource limits, and OpenTelemetry traces and metrics throughout.

`products` is the worked example entity -- a project-scoped CRUD resource that exercises every convention here. Copy it when adding your own; see docs/architecture/adding-an-entity.md.

### Authentication

Four bearer credentials, each accepted by a different set of endpoints; see the security definitions below for which is which. Ordinary calls use **AccessToken**.

A logout revokes the access token it was called with, not only the refresh token, so it stops working immediately. That check can be switched off (`authn.access.token.revocation.enabled`), and where it is, a logged-out access token keeps working until it expires -- which is why the lifetime is short.

Refresh tokens are rotated: each refresh spends the token it was given and returns a successor, so a client **must** store what `POST /auth/refresh` returns. Presenting a spent one again reads as a replay and ends the session for whoever holds the live token too.

### Conventions

- **Identifiers are UUID v7.** Every `id` in this API, without exception.
- **List endpoints share one shape**: `limit`, `next_token`, `prev_token`, `sort`, `filter` and `fields`, answering `{ items, paginator }`. Paginate with the tokens rather than an offset.
- **Errors are `payload.HTTPMessage`**: a message, the method and path, and the status code. Some carry a machine-readable `code`; match on that and never on the prose, which may be reworded.
- **Timestamps are RFC 3339**, and every entity carries `created_at` and `updated_at`.

### Rate limiting

Every endpoint may answer `429`, and the two reasons are not interchangeable:

- `RATE_LIMIT_EXCEEDED` — a budget was spent. `Retry-After` says when to come back.
- `RATE_LIMIT_UNAVAILABLE` — the limiter's own shared counter is unreachable and the service is failing closed. Nobody is being limited *correctly*; this is an operator condition, not a signal to the caller to slow down.

`/health/*` and `/version` bypass the limiter, so an outage of it never makes the service look unreachable.

### Operations

- `GET /health/live` — liveness. Checks nothing else on purpose: restarting cannot fix a dependency that is down.
- `GET /health/detailed` — readiness, per component. `200` healthy, `206` degraded, `503` a hard dependency is gone.
- `GET /health/status` — a public, deliberately thin verdict for anyone who cannot present a token.
- `GET /version` — the build this instance was made from.
  
> [Architecture and operational documentation](https://github.com/slashdevops/go-rest-api-service-template/tree/main/docs)

## Informations

### Version

v1

### Contact

API Support info@goapitemplate.local https://goapitemplate.local

## Tags

  ### <span id="tag-authentication"></span>Authentication

Sign in, sign out, refresh, register, verify an address and recover a password. Also the identity-provider registry and the OAuth flows through it.

  ### <span id="tag-me"></span>Me

The calling user's own account, effective permissions and resource limits. Everything here is scoped to whoever holds the token.

  ### <span id="tag-products"></span>Products

The worked example entity: a project-scoped CRUD resource. Copy it when adding your own.

  ### <span id="tag-projects"></span>Projects

The tenant boundary. Every product, configuration and limit hangs off a project, and most paths in this API begin with one.

  ### <span id="tag-users"></span>Users

Accounts, and their links to roles and projects. Both directions of every link are available.

  ### <span id="tag-roles"></span>Roles

Named bundles of policies, and the users they are granted to.

  ### <span id="tag-policies"></span>Policies

What a role may do, expressed as actions over resources. A policy names resources from the catalogue below.

  ### <span id="tag-resources"></span>Resources

The endpoint catalogue policies are written against -- one entry per operation in this API, generated from these annotations.

  ### <span id="tag-resources-limits"></span>Resources Limits

How much a scope may create, and how much of it already exists. Read-only: limits are not editable through the API.

  ### <span id="tag-rate-limits"></span>RateLimits

The rate-limit rules, and a preview of which would actually apply to a given method and endpoint.

  ### <span id="tag-health"></span>Health

Liveness, readiness and a public status summary. All of it bypasses the rate limiter.

  ### <span id="tag-authorization"></span>Authorization

What a caller may do, resolved. `/me/authz` answers it for the caller and `/users/{id}/authz` for anyone else; both return the permission set roles and policies add up to, rather than the roles and policies themselves.

  ### <span id="tag-identity-providers-id-ps"></span>Identity Providers (IDPs)

External OAuth providers, and the login and registration flows through them. The callback is browser-driven and answers a redirect, not JSON.

  ### <span id="tag-identity-provider-types"></span>Identity Provider Types

The provider kinds an IdP can be configured as. Reference data.

  ### <span id="tag-version"></span>Version

The build this instance was made from. Public, and the quickest way to tell which deployment answered.

## Content negotiation

### URI Schemes
  * http

### Consumes
  * application/json

### Produces
  * application/json

## Access control

### Security Schemes

#### AccessToken (header: Authorization)

The ordinary credential: `Bearer <access_token>`, from `POST /auth/login` or `POST /auth/refresh`. Every endpoint outside `/auth`, `/health` and `/version` takes it. A personal access token is presented the same way and is accepted anywhere an access token is -- which is how a script authenticates without a login. Deliberately short-lived; refresh rather than lengthening it.

> **Type**: apikey

#### RefreshToken (header: Authorization)

`Bearer <refresh_token>`, and accepted **only** by `POST /auth/refresh` and `DELETE /auth/logout`. Refreshing spends it and returns a successor: store what comes back. Presenting a spent one is treated as a replay and ends the whole session, including for whoever holds the live token.

> **Type**: apikey

#### ResetPasswordToken (header: Authorization)

`Bearer <reset_password_token>`, accepted only by `POST /auth/password/reset`. Issued by `POST /auth/password/recover` and delivered by email, single use.

> **Type**: apikey

#### VerificationToken (header: Authorization)

`Bearer <verification_token>`, accepted only by `POST /auth/verify/confirm`. It proves an email address rather than exercising a permission, so it is the one credential no authorization check is applied to -- the account it belongs to is still disabled at that point. It never travels in a URL: the emailed link points at the frontend, which presents the token in this header.

> **Type**: apikey

## All endpoints

###  authentication

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| POST | /auth/login | [019822af b448 755b 92ff d167d37719c2](#019822af-b448-755b-92ff-d167d37719c2) | Authenticate user |
| DELETE | /auth/logout | [019822af b448 7562 a27f 0d02884f3477](#019822af-b448-7562-a27f-0d02884f3477) | Logout user |
| POST | /auth/refresh | [019822af b448 756a 92b2 791a0e748162](#019822af-b448-756a-92b2-791a0e748162) | Refresh access token |
| POST | /auth/register | [019822af b448 7572 a268 4c7b20a70229](#019822af-b448-7572-a268-4c7b20a70229) | Register new user |
| POST | /auth/verify | [019822af b448 7576 8a41 41b83b3239f0](#019822af-b448-7576-8a41-41b83b3239f0) | Resend verification email |
| GET | /auth/idp/{idp_id}/login | [01988e60 89e5 72ab adb4 3eef95d1afd3](#01988e60-89e5-72ab-adb4-3eef95d1afd3) | Initiate IDP login |
| GET | /auth/idp/{idp_id}/callback | [01988e60 89e5 72ee 9db4 db5cd7535717](#01988e60-89e5-72ee-9db4-db5cd7535717) | Handle IDP OAuth callback |
| GET | /auth/idp/{idp_id}/register | [019894ba 6014 79cf bff4 6668484cc7e3](#019894ba-6014-79cf-bff4-6668484cc7e3) | Initiate IDP registration |
| GET | /auth/idps | [0198e7ea 3755 7a29 90ed 13245b54f074](#0198e7ea-3755-7a29-90ed-13245b54f074) | List IDPs |
| DELETE | /auth/idps/{idp_id} | [0198e7ea 3755 7a2d 9ab0 83ccef188e37](#0198e7ea-3755-7a2d-9ab0-83ccef188e37) | Delete IDP |
| PUT | /auth/idps/{idp_id} | [0198e7ea 3755 7a35 9e30 6a9392e8e7a1](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1) | Update IDP |
| POST | /auth/idps | [0198e7ea 3755 7a39 9dfc 717d83facf02](#0198e7ea-3755-7a39-9dfc-717d83facf02) | Create IDP |
| GET | /auth/idps/{idp_id} | [0198e7ea 3755 7a3d 8baa 36126e5d1c48](#0198e7ea-3755-7a3d-8baa-36126e5d1c48) | Get IDP |
| GET | /auth/idp_types | [0198f1e2 14ff 7678 afbe 9a627b0eaabd](#0198f1e2-14ff-7678-afbe-9a627b0eaabd) | List IDP types |
| GET | /auth/idp_types/{idp_type_id} | [0198f1e2 14ff 767c 971c 3904e0f2c484](#0198f1e2-14ff-767c-971c-3904e0f2c484) | Get IDP type |
| GET | /auth/idp/available | [0198fb33 7333 76f9 bcb4 1af086de3e10](#0198fb33-7333-76f9-bcb4-1af086de3e10) | List identity providers |
| POST | /auth/password/recover | [01991917 2720 7589 971b cce23bf8a74b](#01991917-2720-7589-971b-cce23bf8a74b) | Initiate password recovery |
| POST | /auth/password/reset | [01991917 2720 758d 8104 94a0368acecb](#01991917-2720-758d-8104-94a0368acecb) | Reset password |
| POST | /auth/verify/confirm | [01a02dbb bc41 7287 9cfd 7ac08bf882ae](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae) | Confirm email verification |
  


###  health

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /health/status | [01982303 f0f9 7eec 8bf3 84f51fd09b73](#01982303-f0f9-7eec-8bf3-84f51fd09b73) | Health summary (diagnostic, not a probe) |
| GET | /health/detailed | [01982304 a1b2 7eec 8bf3 84f51fd09b74](#01982304-a1b2-7eec-8bf3-84f51fd09b74) | Readiness probe and detailed health |
| GET | /health/live | [01a02ec9 ac6c 77c2 81ad d6e2f23bcd92](#01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92) | Liveness probe |
  


###  me

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /me | [0199489b f2f0 718a a0cb de8752ea864f](#0199489b-f2f0-718a-a0cb-de8752ea864f) | Get authenticated user |
| PUT | /me | [0199489b f2f0 718e a94d b05a296eb818](#0199489b-f2f0-718e-a94d-b05a296eb818) | Update authenticated user |
| GET | /me/authz | [0199489b f2f0 719e b860 3b7ea6a86a1a](#0199489b-f2f0-719e-b860-3b7ea6a86a1a) | Get authorization info |
  


###  policies

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| POST | /policies/{policy_id}/roles | [01982303 f0f9 7e0d ab27 f75b3a03ef46](#01982303-f0f9-7e0d-ab27-f75b3a03ef46) | Link roles to policy |
| GET | /policies/{policy_id} | [01982303 f0f9 7e30 ab3f 9220a73b02eb](#01982303-f0f9-7e30-ab3f-9220a73b02eb) | Get policy |
| POST | /policies | [01982303 f0f9 7e38 ab68 486c8a2e819b](#01982303-f0f9-7e38-ab68-486c8a2e819b) | Create policy |
| PUT | /policies/{policy_id} | [01982303 f0f9 7ec1 8f39 98e77141c05c](#01982303-f0f9-7ec1-8f39-98e77141c05c) | Update policy |
| DELETE | /policies/{policy_id}/roles | [01982303 f0f9 7ed4 9630 b9af3e3b6f17](#01982303-f0f9-7ed4-9630-b9af3e3b6f17) | Unlink roles from policy |
| GET | /policies | [01982303 f0f9 7ee4 968d ba2078a272fc](#01982303-f0f9-7ee4-968d-ba2078a272fc) | List policies |
| DELETE | /policies/{policy_id} | [01982303 f0f9 7f13 a03b ed306ff7d06b](#01982303-f0f9-7f13-a03b-ed306ff7d06b) | Delete policy |
| GET | /roles/{role_id}/policies | [01982303 f0fa 7036 9474 482fc8e5843d](#01982303-f0fa-7036-9474-482fc8e5843d) | List policies by role |
  


###  products

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| POST | /projects/{project_id}/products | [01982303 f0f9 7e63 92ba 141813745b01](#01982303-f0f9-7e63-92ba-141813745b01) | Create product |
| GET | /projects/{project_id}/products/{product_id} | [01982303 f0f9 7e63 92ba 141813745b02](#01982303-f0f9-7e63-92ba-141813745b02) | Get product |
| PUT | /projects/{project_id}/products/{product_id} | [01982303 f0f9 7e63 92ba 141813745b03](#01982303-f0f9-7e63-92ba-141813745b03) | Update product |
| DELETE | /projects/{project_id}/products/{product_id} | [01982303 f0f9 7e63 92ba 141813745b04](#01982303-f0f9-7e63-92ba-141813745b04) | Delete product |
| GET | /projects/{project_id}/products | [01982303 f0f9 7e63 92ba 141813745b05](#01982303-f0f9-7e63-92ba-141813745b05) | List products by project |
| GET | /products | [01982303 f0f9 7e63 92ba 141813745b06](#01982303-f0f9-7e63-92ba-141813745b06) | List products |
  


###  projects

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| PUT | /projects/{project_id} | [01982303 f0f9 7db3 991f 2b7943b5328c](#01982303-f0f9-7db3-991f-2b7943b5328c) | Update project |
| GET | /projects | [01982303 f0f9 7dbf 9688 6ef0150502e9](#01982303-f0f9-7dbf-9688-6ef0150502e9) | List projects |
| GET | /projects/{project_id} | [01982303 f0f9 7dfa 966c 4b9ce4133a33](#01982303-f0f9-7dfa-966c-4b9ce4133a33) | Get project |
| POST | /projects | [01982303 f0f9 7e63 92ba 141813745a7d](#01982303-f0f9-7e63-92ba-141813745a7d) | Create project |
| DELETE | /projects/{project_id} | [01982303 f0f9 7e9f 9bb9 81d42a9eb30a](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a) | Delete project |
| DELETE | /projects/{project_id}/users | [01986f44 3a65 7a19 a92d e6100dd80807](#01986f44-3a65-7a19-a92d-e6100dd80807) | Unlink users from project |
| POST | /projects/{project_id}/users | [01986f44 3a65 7a21 9c2b 392f2b0eacf7](#01986f44-3a65-7a21-9c2b-392f2b0eacf7) | Link users to project |
| GET | /users/{user_id}/projects | [019870ff 37f6 737e 8efb e39730ef6952](#019870ff-37f6-737e-8efb-e39730ef6952) | List projects by user |
  


###  rate_limits

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /rate_limits | [01a03a46 16d4 7831 9c94 a7975a9c4334](#01a03a46-16d4-7831-9c94-a7975a9c4334) | List rate limits |
| POST | /rate_limits | [01a03a46 16d4 7ad9 b646 0bc67824b38c](#01a03a46-16d4-7ad9-b646-0bc67824b38c) | Create rate limit |
| GET | /rate_limits/{rate_limit_id} | [01a03a46 16d4 7af9 9f96 d9dc094afd80](#01a03a46-16d4-7af9-9f96-d9dc094afd80) | Get rate limit |
| PUT | /rate_limits/{rate_limit_id} | [01a03a46 16d4 7b0a af95 6805d68a37d3](#01a03a46-16d4-7b0a-af95-6805d68a37d3) | Update rate limit |
| DELETE | /rate_limits/{rate_limit_id} | [01a03a46 16d4 7b1a 913f e9e50f9acfa7](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7) | Delete rate limit |
| GET | /rate_limits/effective | [01a03a46 16d4 7b2b 8932 ef9694d8f940](#01a03a46-16d4-7b2b-8932-ef9694d8f940) | Effective rate limits |
  


###  resources

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /resources/{resource_id} | [019822c9 9775 71b1 a2c6 deac83cf2519](#019822c9-9775-71b1-a2c6-deac83cf2519) | Get resource |
| GET | /resources/matches | [01982303 f0f9 7e44 aeb1 63934913e601](#01982303-f0f9-7e44-aeb1-63934913e601) | Find resources by action and pattern |
| GET | /resources | [01982303 f0f9 7ee0 aa66 0f756c3c8bec](#01982303-f0f9-7ee0-aa66-0f756c3c8bec) | List resources |
  


###  resources_limits

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /resources_limits | [01994754 5db8 7904 80f3 91417f2a4003](#01994754-5db8-7904-80f3-91417f2a4003) | List resource limits |
| GET | /me/resources_limits | [01a01117 dba9 74da bd70 d1acc3842ffa](#01a01117-dba9-74da-bd70-d1acc3842ffa) | Get my resource limits |
| GET | /projects/{project_id}/resources_limits | [01a01117 dba9 763d 8f4e 968072dbdb52](#01a01117-dba9-763d-8f4e-968072dbdb52) | Get project resource limits |
  


###  roles

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /roles/{role_id} | [01982303 f0f9 7dde ab91 1ab138a8b6c5](#01982303-f0f9-7dde-ab91-1ab138a8b6c5) | Get role |
| DELETE | /roles/{role_id}/users | [01982303 f0f9 7e02 8bf7 c240927de056](#01982303-f0f9-7e02-8bf7-c240927de056) | Unlink users from role |
| POST | /roles/{role_id}/users | [01982303 f0f9 7e2c a026 5aec9fbbe375](#01982303-f0f9-7e2c-a026-5aec9fbbe375) | Link users to role |
| GET | /users/{user_id}/roles | [01982303 f0f9 7e6b 9d17 9b9076785bd6](#01982303-f0f9-7e6b-9d17-9b9076785bd6) | List roles by user |
| PUT | /roles/{role_id} | [01982303 f0f9 7e92 bd69 076fc1cd4a6e](#01982303-f0f9-7e92-bd69-076fc1cd4a6e) | Update role |
| POST | /roles/{role_id}/policies | [01982303 f0f9 7edc bbff c8fc5dcba075](#01982303-f0f9-7edc-bbff-c8fc5dcba075) | Link policies to role |
| GET | /policies/{policy_id}/roles | [01982303 f0f9 7ef0 ad95 3cb214216ef1](#01982303-f0f9-7ef0-ad95-3cb214216ef1) | List roles by policy |
| DELETE | /roles/{role_id}/policies | [01982303 f0f9 7efb a786 89d7d9db40ee](#01982303-f0f9-7efb-a786-89d7d9db40ee) | Unlink policies from role |
| POST | /roles | [01982303 f0f9 7eff 825d 622a05ef4435](#01982303-f0f9-7eff-825d-622a05ef4435) | Create role |
| DELETE | /roles/{role_id} | [01982303 f0fa 7007 aaf6 462c0b8702ec](#01982303-f0fa-7007-aaf6-462c0b8702ec) | Delete role |
| GET | /roles | [01982303 f0fa 703a 92fa be272044b2e3](#01982303-f0fa-703a-92fa-be272044b2e3) | List roles |
  


###  users

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /users | [01982303 f0f9 7daf 809b 4f7880ca9e40](#01982303-f0f9-7daf-809b-4f7880ca9e40) | List users |
| DELETE | /users/{user_id} | [01982303 f0f9 7dda 84da cedd68bca775](#01982303-f0f9-7dda-84da-cedd68bca775) | Delete user |
| GET | /users/{user_id} | [01982303 f0f9 7e25 85f4 4d9d47622702](#01982303-f0f9-7e25-85f4-4d9d47622702) | Get user |
| PUT | /users/{user_id} | [01982303 f0f9 7e3c a186 f186a3418768](#01982303-f0f9-7e3c-a186-f186a3418768) | Update user |
| POST | /users | [01982303 f0f9 7e78 bddf 389144c4beaf](#01982303-f0f9-7e78-bddf-389144c4beaf) | Create user |
| DELETE | /users/{user_id}/roles | [01982303 f0f9 7f23 b203 7079222718d0](#01982303-f0f9-7f23-b203-7079222718d0) | Unlink roles from user |
| POST | /users/{user_id}/roles | [01982303 f0fa 700f a042 0a487ed3c9fb](#01982303-f0fa-700f-a042-0a487ed3c9fb) | Link roles to user |
| GET | /roles/{role_id}/users | [01982303 f0fa 7027 9b77 197b693d0e5a](#01982303-f0fa-7027-9b77-197b693d0e5a) | List users by role |
| GET | /users/{user_id}/authz | [01982303 f0fa 7089 9875 cd42f8e1a3d6](#01982303-f0fa-7089-9875-cd42f8e1a3d6) | Get user authorization |
| DELETE | /users/{user_id}/projects | [01986f44 3a65 7a25 afe8 fdd6ae4572c4](#01986f44-3a65-7a25-afe8-fdd6ae4572c4) | Unlink projects from user |
| POST | /users/{user_id}/projects | [01986f44 3a65 7a2d 8c68 8c579be0aae7](#01986f44-3a65-7a2d-8c68-8c579be0aae7) | Link projects to user |
| GET | /projects/{project_id}/users | [01987096 b4a1 7e8a 8a38 98148daa27a2](#01987096-b4a1-7e8a-8a38-98148daa27a2) | List users by project |
  


###  version

| Method  | URI     | Name   | Summary |
|---------|---------|--------|---------|
| GET | /version | [01982303 f0f9 7dee 90c1 ca400b3d7b91](#01982303-f0f9-7dee-90c1-ca400b3d7b91) | Get service version |
  


## Paths

### <span id="019822af-b448-755b-92ff-d167d37719c2"></span> Authenticate user (*019822af-b448-755b-92ff-d167d37719c2*)

```
POST /auth/login
```

Authenticate with email and password to obtain access and refresh tokens.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadLoginUserRequest](#payload-login-user-request) | `models.PayloadLoginUserRequest` | | ✓ | | Email and password credentials |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019822af-b448-755b-92ff-d167d37719c2-200) | OK | Authentication successful - returns access and refresh tokens |  | [schema](#019822af-b448-755b-92ff-d167d37719c2-200-schema) |
| [400](#019822af-b448-755b-92ff-d167d37719c2-400) | Bad Request | Invalid request body or missing required fields |  | [schema](#019822af-b448-755b-92ff-d167d37719c2-400-schema) |
| [401](#019822af-b448-755b-92ff-d167d37719c2-401) | Unauthorized | Invalid email or password. The same answer is given for an unknown address, a wrong password, and a disabled account |  | [schema](#019822af-b448-755b-92ff-d167d37719c2-401-schema) |
| [429](#019822af-b448-755b-92ff-d167d37719c2-429) | Too Many Requests | Too many failed login attempts for this account; see Retry-After | ✓ | [schema](#019822af-b448-755b-92ff-d167d37719c2-429-schema) |
| [500](#019822af-b448-755b-92ff-d167d37719c2-500) | Internal Server Error | Internal server error during authentication |  | [schema](#019822af-b448-755b-92ff-d167d37719c2-500-schema) |

#### Responses


##### <span id="019822af-b448-755b-92ff-d167d37719c2-200"></span> 200 - Authentication successful - returns access and refresh tokens
Status: OK

###### <span id="019822af-b448-755b-92ff-d167d37719c2-200-schema"></span> Schema
   
  

[PayloadLoginUserResponse](#payload-login-user-response)

##### <span id="019822af-b448-755b-92ff-d167d37719c2-400"></span> 400 - Invalid request body or missing required fields
Status: Bad Request

###### <span id="019822af-b448-755b-92ff-d167d37719c2-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-755b-92ff-d167d37719c2-401"></span> 401 - Invalid email or password. The same answer is given for an unknown address, a wrong password, and a disabled account
Status: Unauthorized

###### <span id="019822af-b448-755b-92ff-d167d37719c2-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-755b-92ff-d167d37719c2-429"></span> 429 - Too many failed login attempts for this account; see Retry-After
Status: Too Many Requests

###### <span id="019822af-b448-755b-92ff-d167d37719c2-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Retry-After | integer | `int64` |  |  | Seconds until an attempt is possible again |

##### <span id="019822af-b448-755b-92ff-d167d37719c2-500"></span> 500 - Internal server error during authentication
Status: Internal Server Error

###### <span id="019822af-b448-755b-92ff-d167d37719c2-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019822af-b448-7562-a27f-0d02884f3477"></span> Logout user (*019822af-b448-7562-a27f-0d02884f3477*)

```
DELETE /auth/logout
```

End the session. The access token this request was authorised with is revoked immediately and stops working. The refresh token is revoked too when it is supplied in the body -- without it, that token stays valid until it expires and can still mint new access tokens, so pass it.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadLogoutUserRequest](#payload-logout-user-request) | `models.PayloadLogoutUserRequest` | |  | | Refresh token to revoke. Omitting it leaves the refresh token valid |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019822af-b448-7562-a27f-0d02884f3477-200) | OK | Logout successful |  | [schema](#019822af-b448-7562-a27f-0d02884f3477-200-schema) |
| [400](#019822af-b448-7562-a27f-0d02884f3477-400) | Bad Request | Malformed request or invalid stored session data |  | [schema](#019822af-b448-7562-a27f-0d02884f3477-400-schema) |
| [401](#019822af-b448-7562-a27f-0d02884f3477-401) | Unauthorized | Invalid or missing access token. An ALREADY-REVOKED access token is accepted here, unlike everywhere else: logging out twice must succeed, because two tabs logging out at once is ordinary |  | [schema](#019822af-b448-7562-a27f-0d02884f3477-401-schema) |
| [403](#019822af-b448-7562-a27f-0d02884f3477-403) | Forbidden | Insufficient permissions |  | [schema](#019822af-b448-7562-a27f-0d02884f3477-403-schema) |
| [429](#019822af-b448-7562-a27f-0d02884f3477-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#019822af-b448-7562-a27f-0d02884f3477-429-schema) |
| [500](#019822af-b448-7562-a27f-0d02884f3477-500) | Internal Server Error | Internal server error during logout |  | [schema](#019822af-b448-7562-a27f-0d02884f3477-500-schema) |

#### Responses


##### <span id="019822af-b448-7562-a27f-0d02884f3477-200"></span> 200 - Logout successful
Status: OK

###### <span id="019822af-b448-7562-a27f-0d02884f3477-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7562-a27f-0d02884f3477-400"></span> 400 - Malformed request or invalid stored session data
Status: Bad Request

###### <span id="019822af-b448-7562-a27f-0d02884f3477-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7562-a27f-0d02884f3477-401"></span> 401 - Invalid or missing access token. An ALREADY-REVOKED access token is accepted here, unlike everywhere else: logging out twice must succeed, because two tabs logging out at once is ordinary
Status: Unauthorized

###### <span id="019822af-b448-7562-a27f-0d02884f3477-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7562-a27f-0d02884f3477-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="019822af-b448-7562-a27f-0d02884f3477-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7562-a27f-0d02884f3477-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="019822af-b448-7562-a27f-0d02884f3477-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7562-a27f-0d02884f3477-500"></span> 500 - Internal server error during logout
Status: Internal Server Error

###### <span id="019822af-b448-7562-a27f-0d02884f3477-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019822af-b448-756a-92b2-791a0e748162"></span> Refresh access token (*019822af-b448-756a-92b2-791a0e748162*)

```
POST /auth/refresh
```

Obtain new access and refresh tokens using valid refresh token. The token spent is the one in the Authorization header; the body is optional and, if it carries a token, it must be the same one.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * RefreshToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadRefreshTokenRequest](#payload-refresh-token-request) | `models.PayloadRefreshTokenRequest` | |  | | Optional, and must match the Authorization header when present |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019822af-b448-756a-92b2-791a0e748162-200) | OK | New access and refresh tokens issued |  | [schema](#019822af-b448-756a-92b2-791a0e748162-200-schema) |
| [400](#019822af-b448-756a-92b2-791a0e748162-400) | Bad Request | Malformed body, or a refresh_token that disagrees with the Authorization header |  | [schema](#019822af-b448-756a-92b2-791a0e748162-400-schema) |
| [401](#019822af-b448-756a-92b2-791a0e748162-401) | Unauthorized | Refresh token is invalid or expired, or the account it was issued for is disabled or no longer exists |  | [schema](#019822af-b448-756a-92b2-791a0e748162-401-schema) |
| [403](#019822af-b448-756a-92b2-791a0e748162-403) | Forbidden | Insufficient permissions |  | [schema](#019822af-b448-756a-92b2-791a0e748162-403-schema) |
| [429](#019822af-b448-756a-92b2-791a0e748162-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#019822af-b448-756a-92b2-791a0e748162-429-schema) |
| [500](#019822af-b448-756a-92b2-791a0e748162-500) | Internal Server Error | Internal server error during token refresh |  | [schema](#019822af-b448-756a-92b2-791a0e748162-500-schema) |

#### Responses


##### <span id="019822af-b448-756a-92b2-791a0e748162-200"></span> 200 - New access and refresh tokens issued
Status: OK

###### <span id="019822af-b448-756a-92b2-791a0e748162-200-schema"></span> Schema
   
  

[PayloadRefreshTokenResponse](#payload-refresh-token-response)

##### <span id="019822af-b448-756a-92b2-791a0e748162-400"></span> 400 - Malformed body, or a refresh_token that disagrees with the Authorization header
Status: Bad Request

###### <span id="019822af-b448-756a-92b2-791a0e748162-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-756a-92b2-791a0e748162-401"></span> 401 - Refresh token is invalid or expired, or the account it was issued for is disabled or no longer exists
Status: Unauthorized

###### <span id="019822af-b448-756a-92b2-791a0e748162-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-756a-92b2-791a0e748162-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="019822af-b448-756a-92b2-791a0e748162-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-756a-92b2-791a0e748162-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="019822af-b448-756a-92b2-791a0e748162-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-756a-92b2-791a0e748162-500"></span> 500 - Internal server error during token refresh
Status: Internal Server Error

###### <span id="019822af-b448-756a-92b2-791a0e748162-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019822af-b448-7572-a268-4c7b20a70229"></span> Register new user (*019822af-b448-7572-a268-4c7b20a70229*)

```
POST /auth/register
```

Create a new user account and send verification email.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadRegisterUserRequest](#payload-register-user-request) | `models.PayloadRegisterUserRequest` | | ✓ | | User registration details including email, password, and profile |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#019822af-b448-7572-a268-4c7b20a70229-201) | Created | Registration accepted. Answered the same way whether or not the address already has an account — deliberately, so this endpoint cannot be used to discover which addresses are registered. If the address was already taken, its owner is told by email instead and no second account is created | ✓ | [schema](#019822af-b448-7572-a268-4c7b20a70229-201-schema) |
| [400](#019822af-b448-7572-a268-4c7b20a70229-400) | Bad Request | Invalid request body or validation error |  | [schema](#019822af-b448-7572-a268-4c7b20a70229-400-schema) |
| [500](#019822af-b448-7572-a268-4c7b20a70229-500) | Internal Server Error | Internal server error during registration |  | [schema](#019822af-b448-7572-a268-4c7b20a70229-500-schema) |

#### Responses


##### <span id="019822af-b448-7572-a268-4c7b20a70229-201"></span> 201 - Registration accepted. Answered the same way whether or not the address already has an account — deliberately, so this endpoint cannot be used to discover which addresses are registered. If the address was already taken, its owner is told by email instead and no second account is created
Status: Created

###### <span id="019822af-b448-7572-a268-4c7b20a70229-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | /users/{id}"	"URI of the created user resource |

##### <span id="019822af-b448-7572-a268-4c7b20a70229-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="019822af-b448-7572-a268-4c7b20a70229-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7572-a268-4c7b20a70229-500"></span> 500 - Internal server error during registration
Status: Internal Server Error

###### <span id="019822af-b448-7572-a268-4c7b20a70229-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019822af-b448-7576-8a41-41b83b3239f0"></span> Resend verification email (*019822af-b448-7576-8a41-41b83b3239f0*)

```
POST /auth/verify
```

Request a new verification email for unverified account.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadReVerifyUserRequest](#payload-re-verify-user-request) | `models.PayloadReVerifyUserRequest` | | ✓ | | Email address for verification |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019822af-b448-7576-8a41-41b83b3239f0-200) | OK | Verification email sent if account exists |  | [schema](#019822af-b448-7576-8a41-41b83b3239f0-200-schema) |
| [400](#019822af-b448-7576-8a41-41b83b3239f0-400) | Bad Request | Invalid request body or email format |  | [schema](#019822af-b448-7576-8a41-41b83b3239f0-400-schema) |
| [401](#019822af-b448-7576-8a41-41b83b3239f0-401) | Unauthorized | Invalid or expired token |  | [schema](#019822af-b448-7576-8a41-41b83b3239f0-401-schema) |
| [500](#019822af-b448-7576-8a41-41b83b3239f0-500) | Internal Server Error | Internal server error during email send |  | [schema](#019822af-b448-7576-8a41-41b83b3239f0-500-schema) |

#### Responses


##### <span id="019822af-b448-7576-8a41-41b83b3239f0-200"></span> 200 - Verification email sent if account exists
Status: OK

###### <span id="019822af-b448-7576-8a41-41b83b3239f0-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7576-8a41-41b83b3239f0-400"></span> 400 - Invalid request body or email format
Status: Bad Request

###### <span id="019822af-b448-7576-8a41-41b83b3239f0-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7576-8a41-41b83b3239f0-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="019822af-b448-7576-8a41-41b83b3239f0-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822af-b448-7576-8a41-41b83b3239f0-500"></span> 500 - Internal server error during email send
Status: Internal Server Error

###### <span id="019822af-b448-7576-8a41-41b83b3239f0-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019822c9-9775-71b1-a2c6-deac83cf2519"></span> Get resource (*019822c9-9775-71b1-a2c6-deac83cf2519*)

```
GET /resources/{resource_id}
```

Retrieve detailed information about a specific system resource configuration using its unique identifier.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| resource_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique resource identifier (UUID v7) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019822c9-9775-71b1-a2c6-deac83cf2519-200) | OK | Resource details retrieved successfully |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-200-schema) |
| [400](#019822c9-9775-71b1-a2c6-deac83cf2519-400) | Bad Request | Invalid resource ID format or malformed request |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-400-schema) |
| [401](#019822c9-9775-71b1-a2c6-deac83cf2519-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-401-schema) |
| [403](#019822c9-9775-71b1-a2c6-deac83cf2519-403) | Forbidden | Insufficient permissions |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-403-schema) |
| [404](#019822c9-9775-71b1-a2c6-deac83cf2519-404) | Not Found | Resource not found |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-404-schema) |
| [429](#019822c9-9775-71b1-a2c6-deac83cf2519-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-429-schema) |
| [500](#019822c9-9775-71b1-a2c6-deac83cf2519-500) | Internal Server Error | Internal server error |  | [schema](#019822c9-9775-71b1-a2c6-deac83cf2519-500-schema) |

#### Responses


##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-200"></span> 200 - Resource details retrieved successfully
Status: OK

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-200-schema"></span> Schema
   
  

[PayloadResourceResponse](#payload-resource-response)

##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-400"></span> 400 - Invalid resource ID format or malformed request
Status: Bad Request

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-404"></span> 404 - Resource not found
Status: Not Found

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="019822c9-9775-71b1-a2c6-deac83cf2519-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40"></span> List users (*01982303-f0f9-7daf-809b-4f7880ca9e40*)

```
GET /users
```

Retrieve paginated list of users with optional filtering and sorting.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Fields to return (comma-separated). Example: id,first_name,last_name |
| filter | `query` | string | `string` |  |  |  | Filter expression. Example: id=1 AND first_name='John' |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of results per page |
| next_token | `query` | string | `string` |  |  |  | Next page cursor for pagination |
| prev_token | `query` | string | `string` |  |  |  | Previous page cursor for pagination |
| sort | `query` | string | `string` |  |  |  | Sort fields (comma-separated). Example: first_name ASC, created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7daf-809b-4f7880ca9e40-200) | OK | Paginated list of users |  | [schema](#01982303-f0f9-7daf-809b-4f7880ca9e40-200-schema) |
| [400](#01982303-f0f9-7daf-809b-4f7880ca9e40-400) | Bad Request | Invalid query parameters or filter expression |  | [schema](#01982303-f0f9-7daf-809b-4f7880ca9e40-400-schema) |
| [401](#01982303-f0f9-7daf-809b-4f7880ca9e40-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7daf-809b-4f7880ca9e40-401-schema) |
| [403](#01982303-f0f9-7daf-809b-4f7880ca9e40-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7daf-809b-4f7880ca9e40-403-schema) |
| [429](#01982303-f0f9-7daf-809b-4f7880ca9e40-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7daf-809b-4f7880ca9e40-429-schema) |
| [500](#01982303-f0f9-7daf-809b-4f7880ca9e40-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7daf-809b-4f7880ca9e40-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-200"></span> 200 - Paginated list of users
Status: OK

###### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-200-schema"></span> Schema
   
  

[PayloadListUsersResponse](#payload-list-users-response)

##### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-400"></span> 400 - Invalid query parameters or filter expression
Status: Bad Request

###### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7daf-809b-4f7880ca9e40-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7db3-991f-2b7943b5328c"></span> Update project (*01982303-f0f9-7db3-991f-2b7943b5328c*)

```
PUT /projects/{project_id}
```

Update existing project details by ID.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project unique identifier |
| body | `body` | [PayloadUpdateProjectRequest](#payload-update-project-request) | `models.PayloadUpdateProjectRequest` | | ✓ | | Project update details including name, description, or settings |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7db3-991f-2b7943b5328c-200) | OK | Project updated successfully |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-200-schema) |
| [400](#01982303-f0f9-7db3-991f-2b7943b5328c-400) | Bad Request | Invalid project ID or request body |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-400-schema) |
| [401](#01982303-f0f9-7db3-991f-2b7943b5328c-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-401-schema) |
| [403](#01982303-f0f9-7db3-991f-2b7943b5328c-403) | Forbidden | System projects cannot be modified |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-403-schema) |
| [404](#01982303-f0f9-7db3-991f-2b7943b5328c-404) | Not Found | Project or owning user not found |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-404-schema) |
| [409](#01982303-f0f9-7db3-991f-2b7943b5328c-409) | Conflict | Project name already in use |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-409-schema) |
| [429](#01982303-f0f9-7db3-991f-2b7943b5328c-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-429-schema) |
| [500](#01982303-f0f9-7db3-991f-2b7943b5328c-500) | Internal Server Error | Internal server error during update |  | [schema](#01982303-f0f9-7db3-991f-2b7943b5328c-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-200"></span> 200 - Project updated successfully
Status: OK

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-400"></span> 400 - Invalid project ID or request body
Status: Bad Request

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-403"></span> 403 - System projects cannot be modified
Status: Forbidden

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-404"></span> 404 - Project or owning user not found
Status: Not Found

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-409"></span> 409 - Project name already in use
Status: Conflict

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-500"></span> 500 - Internal server error during update
Status: Internal Server Error

###### <span id="01982303-f0f9-7db3-991f-2b7943b5328c-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9"></span> List projects (*01982303-f0f9-7dbf-9688-6ef0150502e9*)

```
GET /projects
```

Retrieve paginated list of accessible projects for authenticated user.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Fields to return (comma-separated). Example: id,name,description |
| filter | `query` | string | `string` |  |  |  | Filter expression. Example: name LIKE '%test%' |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of results per page |
| next_token | `query` | string | `string` |  |  |  | Next page cursor for pagination |
| prev_token | `query` | string | `string` |  |  |  | Previous page cursor for pagination |
| sort | `query` | string | `string` |  |  |  | Sort fields (comma-separated). Example: name ASC, created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7dbf-9688-6ef0150502e9-200) | OK | Paginated list of projects |  | [schema](#01982303-f0f9-7dbf-9688-6ef0150502e9-200-schema) |
| [400](#01982303-f0f9-7dbf-9688-6ef0150502e9-400) | Bad Request | Invalid query parameters or filter expression |  | [schema](#01982303-f0f9-7dbf-9688-6ef0150502e9-400-schema) |
| [401](#01982303-f0f9-7dbf-9688-6ef0150502e9-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7dbf-9688-6ef0150502e9-401-schema) |
| [403](#01982303-f0f9-7dbf-9688-6ef0150502e9-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7dbf-9688-6ef0150502e9-403-schema) |
| [429](#01982303-f0f9-7dbf-9688-6ef0150502e9-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7dbf-9688-6ef0150502e9-429-schema) |
| [500](#01982303-f0f9-7dbf-9688-6ef0150502e9-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7dbf-9688-6ef0150502e9-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-200"></span> 200 - Paginated list of projects
Status: OK

###### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-200-schema"></span> Schema
   
  

[PayloadListProjectsResponse](#payload-list-projects-response)

##### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-400"></span> 400 - Invalid query parameters or filter expression
Status: Bad Request

###### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7dbf-9688-6ef0150502e9-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7dda-84da-cedd68bca775"></span> Delete user (*01982303-f0f9-7dda-84da-cedd68bca775*)

```
DELETE /users/{user_id}
```

Permanently remove user account by ID.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7dda-84da-cedd68bca775-200) | OK | User deleted successfully |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-200-schema) |
| [400](#01982303-f0f9-7dda-84da-cedd68bca775-400) | Bad Request | Invalid user ID format |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-400-schema) |
| [401](#01982303-f0f9-7dda-84da-cedd68bca775-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-401-schema) |
| [403](#01982303-f0f9-7dda-84da-cedd68bca775-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-403-schema) |
| [404](#01982303-f0f9-7dda-84da-cedd68bca775-404) | Not Found | User not found |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-404-schema) |
| [429](#01982303-f0f9-7dda-84da-cedd68bca775-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-429-schema) |
| [500](#01982303-f0f9-7dda-84da-cedd68bca775-500) | Internal Server Error | Internal server error during deletion |  | [schema](#01982303-f0f9-7dda-84da-cedd68bca775-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-200"></span> 200 - User deleted successfully
Status: OK

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-400"></span> 400 - Invalid user ID format
Status: Bad Request

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-404"></span> 404 - User not found
Status: Not Found

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dda-84da-cedd68bca775-500"></span> 500 - Internal server error during deletion
Status: Internal Server Error

###### <span id="01982303-f0f9-7dda-84da-cedd68bca775-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5"></span> Get role (*01982303-f0f9-7dde-ab91-1ab138a8b6c5*)

```
GET /roles/{role_id}
```

Retrieve role configuration by unique identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-200) | OK | Role retrieved successfully |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-200-schema) |
| [400](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-400) | Bad Request | Invalid role ID format or malformed request |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-400-schema) |
| [401](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-401-schema) |
| [403](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-403-schema) |
| [404](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-404) | Not Found | Role not found |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-404-schema) |
| [429](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-429-schema) |
| [500](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7dde-ab91-1ab138a8b6c5-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-200"></span> 200 - Role retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-200-schema"></span> Schema
   
  

[PayloadRoleResponse](#payload-role-response)

##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-400"></span> 400 - Invalid role ID format or malformed request
Status: Bad Request

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7dde-ab91-1ab138a8b6c5-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7dee-90c1-ca400b3d7b91"></span> Get service version (*01982303-f0f9-7dee-90c1-ca400b3d7b91*)

```
GET /version
```

Retrieve the current version and build information of the service

#### Produces
  * application/json

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7dee-90c1-ca400b3d7b91-200) | OK | Service version information retrieved successfully |  | [schema](#01982303-f0f9-7dee-90c1-ca400b3d7b91-200-schema) |
| [500](#01982303-f0f9-7dee-90c1-ca400b3d7b91-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7dee-90c1-ca400b3d7b91-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7dee-90c1-ca400b3d7b91-200"></span> 200 - Service version information retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7dee-90c1-ca400b3d7b91-200-schema"></span> Schema
   
  

[PayloadVersion](#payload-version)

##### <span id="01982303-f0f9-7dee-90c1-ca400b3d7b91-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7dee-90c1-ca400b3d7b91-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33"></span> Get project (*01982303-f0f9-7dfa-966c-4b9ce4133a33*)

```
GET /projects/{project_id}
```

Retrieve project details by unique identifier.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7dfa-966c-4b9ce4133a33-200) | OK | Project details retrieved successfully |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-200-schema) |
| [400](#01982303-f0f9-7dfa-966c-4b9ce4133a33-400) | Bad Request | Invalid project ID format |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-400-schema) |
| [401](#01982303-f0f9-7dfa-966c-4b9ce4133a33-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-401-schema) |
| [403](#01982303-f0f9-7dfa-966c-4b9ce4133a33-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-403-schema) |
| [404](#01982303-f0f9-7dfa-966c-4b9ce4133a33-404) | Not Found | Project not found |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-404-schema) |
| [429](#01982303-f0f9-7dfa-966c-4b9ce4133a33-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-429-schema) |
| [500](#01982303-f0f9-7dfa-966c-4b9ce4133a33-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7dfa-966c-4b9ce4133a33-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-200"></span> 200 - Project details retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-200-schema"></span> Schema
   
  

[PayloadProjectResponse](#payload-project-response)

##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-400"></span> 400 - Invalid project ID format
Status: Bad Request

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-404"></span> 404 - Project not found
Status: Not Found

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7dfa-966c-4b9ce4133a33-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e02-8bf7-c240927de056"></span> Unlink users from role (*01982303-f0f9-7e02-8bf7-c240927de056*)

```
DELETE /roles/{role_id}/users
```

Remove user associations from role

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |
| body | `body` | [PayloadUnlinkUsersFromRoleRequest](#payload-unlink-users-from-role-request) | `models.PayloadUnlinkUsersFromRoleRequest` | | ✓ | | User IDs to unlink from role |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e02-8bf7-c240927de056-200) | OK | Users unlinked from role successfully |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-200-schema) |
| [400](#01982303-f0f9-7e02-8bf7-c240927de056-400) | Bad Request | Invalid request payload or role ID format |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-400-schema) |
| [401](#01982303-f0f9-7e02-8bf7-c240927de056-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-401-schema) |
| [403](#01982303-f0f9-7e02-8bf7-c240927de056-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-403-schema) |
| [404](#01982303-f0f9-7e02-8bf7-c240927de056-404) | Not Found | Role not found |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-404-schema) |
| [429](#01982303-f0f9-7e02-8bf7-c240927de056-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-429-schema) |
| [500](#01982303-f0f9-7e02-8bf7-c240927de056-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e02-8bf7-c240927de056-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-200"></span> 200 - Users unlinked from role successfully
Status: OK

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-400"></span> 400 - Invalid request payload or role ID format
Status: Bad Request

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e02-8bf7-c240927de056-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e02-8bf7-c240927de056-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46"></span> Link roles to policy (*01982303-f0f9-7e0d-ab27-f75b3a03ef46*)

```
POST /policies/{policy_id}/roles
```

Associate multiple roles with a specific policy for authorization

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policy_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique policy identifier |
| body | `body` | [PayloadLinkRolesToPolicyRequest](#payload-link-roles-to-policy-request) | `models.PayloadLinkRolesToPolicyRequest` | | ✓ | | Roles linking request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-200) | OK | Roles linked to policy successfully"	{Location: "/policies/{policy_id}/roles/{policy_id}"} |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-200-schema) |
| [400](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-400-schema) |
| [401](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-401-schema) |
| [403](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-403-schema) |
| [404](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-404) | Not Found | Policy not found |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-404-schema) |
| [429](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-429-schema) |
| [500](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e0d-ab27-f75b3a03ef46-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-200"></span> 200 - Roles linked to policy successfully"	{Location: "/policies/{policy_id}/roles/{policy_id}"}
Status: OK

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-404"></span> 404 - Policy not found
Status: Not Found

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e0d-ab27-f75b3a03ef46-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e25-85f4-4d9d47622702"></span> Get user (*01982303-f0f9-7e25-85f4-4d9d47622702*)

```
GET /users/{user_id}
```

Retrieve user account by unique identifier.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e25-85f4-4d9d47622702-200) | OK | User details retrieved successfully |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-200-schema) |
| [400](#01982303-f0f9-7e25-85f4-4d9d47622702-400) | Bad Request | Invalid user ID format |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-400-schema) |
| [401](#01982303-f0f9-7e25-85f4-4d9d47622702-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-401-schema) |
| [403](#01982303-f0f9-7e25-85f4-4d9d47622702-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-403-schema) |
| [404](#01982303-f0f9-7e25-85f4-4d9d47622702-404) | Not Found | User not found |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-404-schema) |
| [429](#01982303-f0f9-7e25-85f4-4d9d47622702-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-429-schema) |
| [500](#01982303-f0f9-7e25-85f4-4d9d47622702-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e25-85f4-4d9d47622702-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-200"></span> 200 - User details retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-200-schema"></span> Schema
   
  

[PayloadUserResponse](#payload-user-response)

##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-400"></span> 400 - Invalid user ID format
Status: Bad Request

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-404"></span> 404 - User not found
Status: Not Found

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e25-85f4-4d9d47622702-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375"></span> Link users to role (*01982303-f0f9-7e2c-a026-5aec9fbbe375*)

```
POST /roles/{role_id}/users
```

Associate multiple users with role for authorization

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |
| body | `body` | [PayloadLinkUsersToRoleRequest](#payload-link-users-to-role-request) | `models.PayloadLinkUsersToRoleRequest` | | ✓ | | User IDs to link with role |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e2c-a026-5aec9fbbe375-200) | OK | Users linked to role successfully |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-200-schema) |
| [400](#01982303-f0f9-7e2c-a026-5aec9fbbe375-400) | Bad Request | Invalid request payload or role ID format |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-400-schema) |
| [401](#01982303-f0f9-7e2c-a026-5aec9fbbe375-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-401-schema) |
| [403](#01982303-f0f9-7e2c-a026-5aec9fbbe375-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-403-schema) |
| [404](#01982303-f0f9-7e2c-a026-5aec9fbbe375-404) | Not Found | Role not found |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-404-schema) |
| [429](#01982303-f0f9-7e2c-a026-5aec9fbbe375-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-429-schema) |
| [500](#01982303-f0f9-7e2c-a026-5aec9fbbe375-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e2c-a026-5aec9fbbe375-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-200"></span> 200 - Users linked to role successfully
Status: OK

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-400"></span> 400 - Invalid request payload or role ID format
Status: Bad Request

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e2c-a026-5aec9fbbe375-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb"></span> Get policy (*01982303-f0f9-7e30-ab3f-9220a73b02eb*)

```
GET /policies/{policy_id}
```

Retrieve a specific authorization policy by its unique identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policy_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique policy identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e30-ab3f-9220a73b02eb-200) | OK | Policy retrieved successfully |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-200-schema) |
| [400](#01982303-f0f9-7e30-ab3f-9220a73b02eb-400) | Bad Request | Invalid policy ID format |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-400-schema) |
| [401](#01982303-f0f9-7e30-ab3f-9220a73b02eb-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-401-schema) |
| [403](#01982303-f0f9-7e30-ab3f-9220a73b02eb-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-403-schema) |
| [404](#01982303-f0f9-7e30-ab3f-9220a73b02eb-404) | Not Found | Policy not found |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-404-schema) |
| [429](#01982303-f0f9-7e30-ab3f-9220a73b02eb-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-429-schema) |
| [500](#01982303-f0f9-7e30-ab3f-9220a73b02eb-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e30-ab3f-9220a73b02eb-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-200"></span> 200 - Policy retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-200-schema"></span> Schema
   
  

[PayloadPolicyResponse](#payload-policy-response)

##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-400"></span> 400 - Invalid policy ID format
Status: Bad Request

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-404"></span> 404 - Policy not found
Status: Not Found

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e30-ab3f-9220a73b02eb-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b"></span> Create policy (*01982303-f0f9-7e38-ab68-486c8a2e819b*)

```
POST /policies
```

Create a new authorization policy with specified permissions

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadCreatePolicyRequest](#payload-create-policy-request) | `models.PayloadCreatePolicyRequest` | | ✓ | | Policy creation request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#01982303-f0f9-7e38-ab68-486c8a2e819b-201) | Created | Policy created successfully"	{Location: "/policies/{policy_id}"} |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-201-schema) |
| [400](#01982303-f0f9-7e38-ab68-486c8a2e819b-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-400-schema) |
| [401](#01982303-f0f9-7e38-ab68-486c8a2e819b-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-401-schema) |
| [403](#01982303-f0f9-7e38-ab68-486c8a2e819b-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-403-schema) |
| [404](#01982303-f0f9-7e38-ab68-486c8a2e819b-404) | Not Found | One or more referenced resources not found |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-404-schema) |
| [409](#01982303-f0f9-7e38-ab68-486c8a2e819b-409) | Conflict | Policy already exists |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-409-schema) |
| [429](#01982303-f0f9-7e38-ab68-486c8a2e819b-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-429-schema) |
| [500](#01982303-f0f9-7e38-ab68-486c8a2e819b-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e38-ab68-486c8a2e819b-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-201"></span> 201 - Policy created successfully"	{Location: "/policies/{policy_id}"}
Status: Created

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-404"></span> 404 - One or more referenced resources not found
Status: Not Found

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-409"></span> 409 - Policy already exists
Status: Conflict

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e38-ab68-486c8a2e819b-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e3c-a186-f186a3418768"></span> Update user (*01982303-f0f9-7e3c-a186-f186a3418768*)

```
PUT /users/{user_id}
```

Update existing user account details by ID.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| body | `body` | [PayloadUpdateUserRequest](#payload-update-user-request) | `models.PayloadUpdateUserRequest` | | ✓ | | User update details including name, email, or profile changes |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e3c-a186-f186a3418768-200) | OK | User updated successfully |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-200-schema) |
| [400](#01982303-f0f9-7e3c-a186-f186a3418768-400) | Bad Request | Invalid user ID or request body |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-400-schema) |
| [401](#01982303-f0f9-7e3c-a186-f186a3418768-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-401-schema) |
| [403](#01982303-f0f9-7e3c-a186-f186a3418768-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-403-schema) |
| [404](#01982303-f0f9-7e3c-a186-f186a3418768-404) | Not Found | User not found |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-404-schema) |
| [409](#01982303-f0f9-7e3c-a186-f186a3418768-409) | Conflict | Email already in use by another user |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-409-schema) |
| [429](#01982303-f0f9-7e3c-a186-f186a3418768-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-429-schema) |
| [500](#01982303-f0f9-7e3c-a186-f186a3418768-500) | Internal Server Error | Internal server error during update |  | [schema](#01982303-f0f9-7e3c-a186-f186a3418768-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-200"></span> 200 - User updated successfully
Status: OK

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-400"></span> 400 - Invalid user ID or request body
Status: Bad Request

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-404"></span> 404 - User not found
Status: Not Found

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-409"></span> 409 - Email already in use by another user
Status: Conflict

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e3c-a186-f186a3418768-500"></span> 500 - Internal server error during update
Status: Internal Server Error

###### <span id="01982303-f0f9-7e3c-a186-f186a3418768-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e44-aeb1-63934913e601"></span> Find resources by action and pattern (*01982303-f0f9-7e44-aeb1-63934913e601*)

```
GET /resources/matches
```

Retrieve a paginated list of resources that match the specified action and resource policy patterns.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| action | `query` | string | `string` |  | ✓ |  | Action pattern to match (e.g., 'read', 'write', 'delete' or wildcard patterns) |
| fields | `query` | string | `string` |  |  |  | Comma-separated list of fields to include in response. Example: id,name,action,resource |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items to return per page (default: varies by configuration) |
| next_token | `query` | string | `string` |  |  |  | Pagination cursor for fetching the next page of results |
| prev_token | `query` | string | `string` |  |  |  | Pagination cursor for fetching the previous page of results |
| resource | `query` | string | `string` |  | ✓ |  | Resource pattern to match (e.g., 'users/*', 'projects/123' or wildcard patterns) |
| sort | `query` | string | `string` |  |  |  | Sort order: comma-separated fields with ASC/DESC. Example: name ASC,created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e44-aeb1-63934913e601-200) | OK | Matching resources retrieved successfully |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-200-schema) |
| [400](#01982303-f0f9-7e44-aeb1-63934913e601-400) | Bad Request | Invalid action/resource pattern or malformed query parameters |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-400-schema) |
| [401](#01982303-f0f9-7e44-aeb1-63934913e601-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-401-schema) |
| [403](#01982303-f0f9-7e44-aeb1-63934913e601-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-403-schema) |
| [404](#01982303-f0f9-7e44-aeb1-63934913e601-404) | Not Found | No resources found matching the specified patterns |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-404-schema) |
| [429](#01982303-f0f9-7e44-aeb1-63934913e601-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-429-schema) |
| [500](#01982303-f0f9-7e44-aeb1-63934913e601-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e44-aeb1-63934913e601-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-200"></span> 200 - Matching resources retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-200-schema"></span> Schema
   
  

[PayloadListResourcesResponse](#payload-list-resources-response)

##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-400"></span> 400 - Invalid action/resource pattern or malformed query parameters
Status: Bad Request

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-404"></span> 404 - No resources found matching the specified patterns
Status: Not Found

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e44-aeb1-63934913e601-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e44-aeb1-63934913e601-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745a7d"></span> Create project (*01982303-f0f9-7e63-92ba-141813745a7d*)

```
POST /projects
```

Create a new project with specified configuration.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadCreateProjectRequest](#payload-create-project-request) | `models.PayloadCreateProjectRequest` | | ✓ | | Project configuration including name, description, and settings |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#01982303-f0f9-7e63-92ba-141813745a7d-201) | Created | Project created successfully | ✓ | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-201-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745a7d-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745a7d-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745a7d-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-403-schema) |
| [404](#01982303-f0f9-7e63-92ba-141813745a7d-404) | Not Found | Owning user not found |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-404-schema) |
| [409](#01982303-f0f9-7e63-92ba-141813745a7d-409) | Conflict | Project with name already exists |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-409-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745a7d-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745a7d-500) | Internal Server Error | Internal server error during project creation |  | [schema](#01982303-f0f9-7e63-92ba-141813745a7d-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-201"></span> 201 - Project created successfully
Status: Created

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | /projects/{id}"	"URI of the created project resource |

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-404"></span> 404 - Owning user not found
Status: Not Found

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-409"></span> 409 - Project with name already exists
Status: Conflict

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745a7d-500"></span> 500 - Internal server error during project creation
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745a7d-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745b01"></span> Create product (*01982303-f0f9-7e63-92ba-141813745b01*)

```
POST /projects/{project_id}/products
```

Create a new product inside a project. The name must be unique within the project.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | string | `string` |  | ✓ |  | The project id in UUID format |
| body | `body` | [PayloadCreateProductRequest](#payload-create-product-request) | `models.PayloadCreateProductRequest` | | ✓ | | Product configuration including name and description |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#01982303-f0f9-7e63-92ba-141813745b01-201) | Created | Product created successfully | ✓ | [schema](#01982303-f0f9-7e63-92ba-141813745b01-201-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745b01-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745b01-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745b01-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-403-schema) |
| [404](#01982303-f0f9-7e63-92ba-141813745b01-404) | Not Found | Project not found, or the caller has no access to it |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-404-schema) |
| [409](#01982303-f0f9-7e63-92ba-141813745b01-409) | Conflict | Product with that name already exists in the project, or the project's product limit is reached |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-409-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745b01-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745b01-500) | Internal Server Error | Internal server error during product creation |  | [schema](#01982303-f0f9-7e63-92ba-141813745b01-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745b01-201"></span> 201 - Product created successfully
Status: Created

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | /projects/{project_id}/products/{id}"	"URI of the created product resource |

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-404"></span> 404 - Project not found, or the caller has no access to it
Status: Not Found

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-409"></span> 409 - Product with that name already exists in the project, or the project's product limit is reached
Status: Conflict

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b01-500"></span> 500 - Internal server error during product creation
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745b01-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745b02"></span> Get product (*01982303-f0f9-7e63-92ba-141813745b02*)

```
GET /projects/{project_id}/products/{product_id}
```

Retrieve a product by its unique identifier within a project.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| product_id | `path` | string | `string` |  | ✓ |  | The product id in UUID format |
| project_id | `path` | string | `string` |  | ✓ |  | The project id in UUID format |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e63-92ba-141813745b02-200) | OK | Product found |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-200-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745b02-400) | Bad Request | Invalid identifier |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745b02-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745b02-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-403-schema) |
| [404](#01982303-f0f9-7e63-92ba-141813745b02-404) | Not Found | Product not found, or the caller has no access to the project |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-404-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745b02-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745b02-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e63-92ba-141813745b02-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745b02-200"></span> 200 - Product found
Status: OK

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-200-schema"></span> Schema
   
  

[PayloadProductResponse](#payload-product-response)

##### <span id="01982303-f0f9-7e63-92ba-141813745b02-400"></span> 400 - Invalid identifier
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b02-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b02-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b02-404"></span> 404 - Product not found, or the caller has no access to the project
Status: Not Found

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b02-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b02-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745b02-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745b03"></span> Update product (*01982303-f0f9-7e63-92ba-141813745b03*)

```
PUT /projects/{project_id}/products/{product_id}
```

Update a product's name or description. Both fields are optional; at least one is required.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| product_id | `path` | string | `string` |  | ✓ |  | The product id in UUID format |
| project_id | `path` | string | `string` |  | ✓ |  | The project id in UUID format |
| body | `body` | [PayloadUpdateProductRequest](#payload-update-product-request) | `models.PayloadUpdateProductRequest` | | ✓ | | Fields to update |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e63-92ba-141813745b03-200) | OK | Product updated successfully |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-200-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745b03-400) | Bad Request | Invalid request body or identifier |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745b03-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745b03-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-403-schema) |
| [404](#01982303-f0f9-7e63-92ba-141813745b03-404) | Not Found | Product not found, or the caller has no access to the project |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-404-schema) |
| [409](#01982303-f0f9-7e63-92ba-141813745b03-409) | Conflict | Another product in the project already has that name |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-409-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745b03-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745b03-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e63-92ba-141813745b03-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745b03-200"></span> 200 - Product updated successfully
Status: OK

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-400"></span> 400 - Invalid request body or identifier
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-404"></span> 404 - Product not found, or the caller has no access to the project
Status: Not Found

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-409"></span> 409 - Another product in the project already has that name
Status: Conflict

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b03-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745b03-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745b04"></span> Delete product (*01982303-f0f9-7e63-92ba-141813745b04*)

```
DELETE /projects/{project_id}/products/{product_id}
```

Delete a product from a project.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| product_id | `path` | string | `string` |  | ✓ |  | The product id in UUID format |
| project_id | `path` | string | `string` |  | ✓ |  | The project id in UUID format |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e63-92ba-141813745b04-200) | OK | Product deleted successfully |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-200-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745b04-400) | Bad Request | Invalid identifier |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745b04-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745b04-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-403-schema) |
| [404](#01982303-f0f9-7e63-92ba-141813745b04-404) | Not Found | Product not found, or the caller has no access to the project |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-404-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745b04-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745b04-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e63-92ba-141813745b04-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745b04-200"></span> 200 - Product deleted successfully
Status: OK

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b04-400"></span> 400 - Invalid identifier
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b04-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b04-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b04-404"></span> 404 - Product not found, or the caller has no access to the project
Status: Not Found

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b04-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b04-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745b04-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745b05"></span> List products by project (*01982303-f0f9-7e63-92ba-141813745b05*)

```
GET /projects/{project_id}/products
```

List the products of one project, with filtering, sorting, partial fields and pagination.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | string | `string` |  | ✓ |  | The project id in UUID format |
| fields | `query` | string | `string` |  |  |  | Comma-separated fields to return |
| filter | `query` | string | `string` |  |  |  | Filter expression, e.g. name = 'widget' |
| limit | `query` | integer | `int64` |  |  |  | Maximum items to return |
| next_token | `query` | string | `string` |  |  |  | Token for the next page |
| prev_token | `query` | string | `string` |  |  |  | Token for the previous page |
| sort | `query` | string | `string` |  |  |  | Comma-separated sort fields, e.g. name ASC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e63-92ba-141813745b05-200) | OK | Products found |  | [schema](#01982303-f0f9-7e63-92ba-141813745b05-200-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745b05-400) | Bad Request | Invalid query parameters |  | [schema](#01982303-f0f9-7e63-92ba-141813745b05-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745b05-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745b05-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745b05-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745b05-403-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745b05-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745b05-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745b05-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e63-92ba-141813745b05-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745b05-200"></span> 200 - Products found
Status: OK

###### <span id="01982303-f0f9-7e63-92ba-141813745b05-200-schema"></span> Schema
   
  

[PayloadListProductsResponse](#payload-list-products-response)

##### <span id="01982303-f0f9-7e63-92ba-141813745b05-400"></span> 400 - Invalid query parameters
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745b05-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b05-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745b05-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b05-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745b05-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b05-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745b05-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b05-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745b05-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e63-92ba-141813745b06"></span> List products (*01982303-f0f9-7e63-92ba-141813745b06*)

```
GET /products
```

List products across every project, with filtering, sorting, partial fields and pagination.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated fields to return |
| filter | `query` | string | `string` |  |  |  | Filter expression, e.g. name = 'widget' |
| limit | `query` | integer | `int64` |  |  |  | Maximum items to return |
| next_token | `query` | string | `string` |  |  |  | Token for the next page |
| prev_token | `query` | string | `string` |  |  |  | Token for the previous page |
| sort | `query` | string | `string` |  |  |  | Comma-separated sort fields, e.g. name ASC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e63-92ba-141813745b06-200) | OK | Products found |  | [schema](#01982303-f0f9-7e63-92ba-141813745b06-200-schema) |
| [400](#01982303-f0f9-7e63-92ba-141813745b06-400) | Bad Request | Invalid query parameters |  | [schema](#01982303-f0f9-7e63-92ba-141813745b06-400-schema) |
| [401](#01982303-f0f9-7e63-92ba-141813745b06-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e63-92ba-141813745b06-401-schema) |
| [403](#01982303-f0f9-7e63-92ba-141813745b06-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e63-92ba-141813745b06-403-schema) |
| [429](#01982303-f0f9-7e63-92ba-141813745b06-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e63-92ba-141813745b06-429-schema) |
| [500](#01982303-f0f9-7e63-92ba-141813745b06-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e63-92ba-141813745b06-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e63-92ba-141813745b06-200"></span> 200 - Products found
Status: OK

###### <span id="01982303-f0f9-7e63-92ba-141813745b06-200-schema"></span> Schema
   
  

[PayloadListProductsResponse](#payload-list-products-response)

##### <span id="01982303-f0f9-7e63-92ba-141813745b06-400"></span> 400 - Invalid query parameters
Status: Bad Request

###### <span id="01982303-f0f9-7e63-92ba-141813745b06-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b06-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e63-92ba-141813745b06-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b06-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e63-92ba-141813745b06-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b06-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e63-92ba-141813745b06-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e63-92ba-141813745b06-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e63-92ba-141813745b06-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6"></span> List roles by user (*01982303-f0f9-7e6b-9d17-9b9076785bd6*)

```
GET /users/{user_id}/roles
```

Retrieve paginated list of roles assigned to user

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| fields | `query` | string | `string` |  |  |  | Comma-separated field names to include in response |
| filter | `query` | string | `string` |  |  |  | Filter expression (e.g., name='admin' AND system=true) |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items per page |
| next_token | `query` | string | `string` |  |  |  | Pagination token for next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination token for previous page |
| sort | `query` | string | `string` |  |  |  | Comma-separated sort fields with direction (e.g., name ASC, created_at DESC) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e6b-9d17-9b9076785bd6-200) | OK | Paginated list of user roles retrieved successfully |  | [schema](#01982303-f0f9-7e6b-9d17-9b9076785bd6-200-schema) |
| [400](#01982303-f0f9-7e6b-9d17-9b9076785bd6-400) | Bad Request | Invalid user ID or query parameters |  | [schema](#01982303-f0f9-7e6b-9d17-9b9076785bd6-400-schema) |
| [401](#01982303-f0f9-7e6b-9d17-9b9076785bd6-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e6b-9d17-9b9076785bd6-401-schema) |
| [403](#01982303-f0f9-7e6b-9d17-9b9076785bd6-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e6b-9d17-9b9076785bd6-403-schema) |
| [429](#01982303-f0f9-7e6b-9d17-9b9076785bd6-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e6b-9d17-9b9076785bd6-429-schema) |
| [500](#01982303-f0f9-7e6b-9d17-9b9076785bd6-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e6b-9d17-9b9076785bd6-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-200"></span> 200 - Paginated list of user roles retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-200-schema"></span> Schema
   
  

[PayloadListRolesResponse](#payload-list-roles-response)

##### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-400"></span> 400 - Invalid user ID or query parameters
Status: Bad Request

###### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e6b-9d17-9b9076785bd6-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e78-bddf-389144c4beaf"></span> Create user (*01982303-f0f9-7e78-bddf-389144c4beaf*)

```
POST /users
```

Create a new user account with specified details.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadCreateUserRequest](#payload-create-user-request) | `models.PayloadCreateUserRequest` | | ✓ | | User creation details including email, name, and profile |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#01982303-f0f9-7e78-bddf-389144c4beaf-201) | Created | User account created successfully | ✓ | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-201-schema) |
| [400](#01982303-f0f9-7e78-bddf-389144c4beaf-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-400-schema) |
| [401](#01982303-f0f9-7e78-bddf-389144c4beaf-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-401-schema) |
| [403](#01982303-f0f9-7e78-bddf-389144c4beaf-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-403-schema) |
| [409](#01982303-f0f9-7e78-bddf-389144c4beaf-409) | Conflict | User with email already exists |  | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-409-schema) |
| [429](#01982303-f0f9-7e78-bddf-389144c4beaf-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-429-schema) |
| [500](#01982303-f0f9-7e78-bddf-389144c4beaf-500) | Internal Server Error | Internal server error during user creation |  | [schema](#01982303-f0f9-7e78-bddf-389144c4beaf-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-201"></span> 201 - User account created successfully
Status: Created

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | /users/{id}"	"URI of the created user resource |

##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-409"></span> 409 - User with email already exists
Status: Conflict

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-500"></span> 500 - Internal server error during user creation
Status: Internal Server Error

###### <span id="01982303-f0f9-7e78-bddf-389144c4beaf-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e"></span> Update role (*01982303-f0f9-7e92-bd69-076fc1cd4a6e*)

```
PUT /roles/{role_id}
```

Update existing role configuration by identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |
| body | `body` | [PayloadUpdateRoleRequest](#payload-update-role-request) | `models.PayloadUpdateRoleRequest` | | ✓ | | Role update payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-200) | OK | Role updated successfully |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-200-schema) |
| [400](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-400) | Bad Request | Invalid request payload or validation failed |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-400-schema) |
| [401](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-401-schema) |
| [403](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-403-schema) |
| [404](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-404) | Not Found | Role not found |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-404-schema) |
| [409](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-409) | Conflict | Role with same name already exists |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-409-schema) |
| [429](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-429-schema) |
| [500](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7e92-bd69-076fc1cd4a6e-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-200"></span> 200 - Role updated successfully
Status: OK

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-400"></span> 400 - Invalid request payload or validation failed
Status: Bad Request

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-409"></span> 409 - Role with same name already exists
Status: Conflict

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7e92-bd69-076fc1cd4a6e-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a"></span> Delete project (*01982303-f0f9-7e9f-9bb9-81d42a9eb30a*)

```
DELETE /projects/{project_id}
```

Permanently remove project and all associated data.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-200) | OK | Project deleted successfully |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-200-schema) |
| [400](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-400) | Bad Request | Invalid project ID format |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-400-schema) |
| [401](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-401-schema) |
| [403](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-403) | Forbidden | System projects cannot be deleted |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-403-schema) |
| [404](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-404) | Not Found | Project not found |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-404-schema) |
| [429](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-429-schema) |
| [500](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-500) | Internal Server Error | Internal server error during deletion |  | [schema](#01982303-f0f9-7e9f-9bb9-81d42a9eb30a-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-200"></span> 200 - Project deleted successfully
Status: OK

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-400"></span> 400 - Invalid project ID format
Status: Bad Request

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-403"></span> 403 - System projects cannot be deleted
Status: Forbidden

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-404"></span> 404 - Project not found
Status: Not Found

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-500"></span> 500 - Internal server error during deletion
Status: Internal Server Error

###### <span id="01982303-f0f9-7e9f-9bb9-81d42a9eb30a-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c"></span> Update policy (*01982303-f0f9-7ec1-8f39-98e77141c05c*)

```
PUT /policies/{policy_id}
```

Update an existing authorization policy by its unique identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policy_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique policy identifier |
| body | `body` | [PayloadUpdatePolicyRequest](#payload-update-policy-request) | `models.PayloadUpdatePolicyRequest` | | ✓ | | Policy update request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7ec1-8f39-98e77141c05c-200) | OK | Policy updated successfully"	{Location: "/policies/{policy_id}"} |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-200-schema) |
| [400](#01982303-f0f9-7ec1-8f39-98e77141c05c-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-400-schema) |
| [401](#01982303-f0f9-7ec1-8f39-98e77141c05c-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-401-schema) |
| [403](#01982303-f0f9-7ec1-8f39-98e77141c05c-403) | Forbidden | System policies cannot be modified |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-403-schema) |
| [404](#01982303-f0f9-7ec1-8f39-98e77141c05c-404) | Not Found | Policy not found |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-404-schema) |
| [409](#01982303-f0f9-7ec1-8f39-98e77141c05c-409) | Conflict | Policy name already in use |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-409-schema) |
| [429](#01982303-f0f9-7ec1-8f39-98e77141c05c-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-429-schema) |
| [500](#01982303-f0f9-7ec1-8f39-98e77141c05c-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7ec1-8f39-98e77141c05c-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-200"></span> 200 - Policy updated successfully"	{Location: "/policies/{policy_id}"}
Status: OK

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-403"></span> 403 - System policies cannot be modified
Status: Forbidden

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-404"></span> 404 - Policy not found
Status: Not Found

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-409"></span> 409 - Policy name already in use
Status: Conflict

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7ec1-8f39-98e77141c05c-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17"></span> Unlink roles from policy (*01982303-f0f9-7ed4-9630-b9af3e3b6f17*)

```
DELETE /policies/{policy_id}/roles
```

Remove role associations from a specific policy

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policy_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique policy identifier |
| body | `body` | [PayloadUnlinkRolesFromPolicyRequest](#payload-unlink-roles-from-policy-request) | `models.PayloadUnlinkRolesFromPolicyRequest` | | ✓ | | Roles unlinking request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-200) | OK | Roles unlinked from policy successfully"	{Location: "/policies/{policy_id}/roles/{policy_id}"} |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-200-schema) |
| [400](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-400) | Bad Request | Invalid request body or validation error |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-400-schema) |
| [401](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-401-schema) |
| [403](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-403-schema) |
| [404](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-404) | Not Found | Policy not found |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-404-schema) |
| [429](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-429-schema) |
| [500](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7ed4-9630-b9af3e3b6f17-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-200"></span> 200 - Roles unlinked from policy successfully"	{Location: "/policies/{policy_id}/roles/{policy_id}"}
Status: OK

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-400"></span> 400 - Invalid request body or validation error
Status: Bad Request

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-404"></span> 404 - Policy not found
Status: Not Found

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7ed4-9630-b9af3e3b6f17-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075"></span> Link policies to role (*01982303-f0f9-7edc-bbff-c8fc5dcba075*)

```
POST /roles/{role_id}/policies
```

Associate multiple policies with role for authorization

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |
| body | `body` | [PayloadLinkPoliciesToRoleRequest](#payload-link-policies-to-role-request) | `models.PayloadLinkPoliciesToRoleRequest` | | ✓ | | Policy IDs to link with role |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7edc-bbff-c8fc5dcba075-200) | OK | Policies linked to role successfully |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-200-schema) |
| [400](#01982303-f0f9-7edc-bbff-c8fc5dcba075-400) | Bad Request | Invalid request payload or role ID format |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-400-schema) |
| [401](#01982303-f0f9-7edc-bbff-c8fc5dcba075-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-401-schema) |
| [403](#01982303-f0f9-7edc-bbff-c8fc5dcba075-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-403-schema) |
| [404](#01982303-f0f9-7edc-bbff-c8fc5dcba075-404) | Not Found | Role not found |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-404-schema) |
| [429](#01982303-f0f9-7edc-bbff-c8fc5dcba075-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-429-schema) |
| [500](#01982303-f0f9-7edc-bbff-c8fc5dcba075-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7edc-bbff-c8fc5dcba075-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-200"></span> 200 - Policies linked to role successfully
Status: OK

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-400"></span> 400 - Invalid request payload or role ID format
Status: Bad Request

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7edc-bbff-c8fc5dcba075-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec"></span> List resources (*01982303-f0f9-7ee0-aa66-0f756c3c8bec*)

```
GET /resources
```

Retrieve a paginated list of all available system resources.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated list of fields to include in response. Example: id,name,action,resource |
| filter | `query` | string | `string` |  |  |  | Filter expression using SQL-like syntax. Example: action='read' AND system=true |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items to return per page (default: varies by configuration) |
| next_token | `query` | string | `string` |  |  |  | Pagination cursor for fetching the next page of results |
| prev_token | `query` | string | `string` |  |  |  | Pagination cursor for fetching the previous page of results |
| sort | `query` | string | `string` |  |  |  | Sort order: comma-separated fields with ASC/DESC. Example: name ASC,created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-200) | OK | Paginated list of resources retrieved successfully |  | [schema](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-200-schema) |
| [400](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-400) | Bad Request | Invalid query parameters or malformed filter/sort expression |  | [schema](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-400-schema) |
| [401](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-401-schema) |
| [403](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-403-schema) |
| [429](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-429-schema) |
| [500](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7ee0-aa66-0f756c3c8bec-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-200"></span> 200 - Paginated list of resources retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-200-schema"></span> Schema
   
  

[PayloadListResourcesResponse](#payload-list-resources-response)

##### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-400"></span> 400 - Invalid query parameters or malformed filter/sort expression
Status: Bad Request

###### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7ee0-aa66-0f756c3c8bec-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc"></span> List policies (*01982303-f0f9-7ee4-968d-ba2078a272fc*)

```
GET /policies
```

Retrieve a paginated list of all authorization policies in the system

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated fields to return (e.g., 'id,name,description') |
| filter | `query` | string | `string` |  |  |  | Filter conditions (e.g., 'name LIKE policy%') |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items per page |
| next_token | `query` | string | `string` |  |  |  | Token for next page of results |
| prev_token | `query` | string | `string` |  |  |  | Token for previous page of results |
| sort | `query` | string | `string` |  |  |  | Sort by fields (e.g., 'name ASC, created_at DESC') |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7ee4-968d-ba2078a272fc-200) | OK | Policies retrieved successfully |  | [schema](#01982303-f0f9-7ee4-968d-ba2078a272fc-200-schema) |
| [400](#01982303-f0f9-7ee4-968d-ba2078a272fc-400) | Bad Request | Invalid query parameters |  | [schema](#01982303-f0f9-7ee4-968d-ba2078a272fc-400-schema) |
| [401](#01982303-f0f9-7ee4-968d-ba2078a272fc-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7ee4-968d-ba2078a272fc-401-schema) |
| [403](#01982303-f0f9-7ee4-968d-ba2078a272fc-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7ee4-968d-ba2078a272fc-403-schema) |
| [429](#01982303-f0f9-7ee4-968d-ba2078a272fc-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7ee4-968d-ba2078a272fc-429-schema) |
| [500](#01982303-f0f9-7ee4-968d-ba2078a272fc-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7ee4-968d-ba2078a272fc-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-200"></span> 200 - Policies retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-200-schema"></span> Schema
   
  

[PayloadListPoliciesResponse](#payload-list-policies-response)

##### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-400"></span> 400 - Invalid query parameters
Status: Bad Request

###### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7ee4-968d-ba2078a272fc-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7eec-8bf3-84f51fd09b73"></span> Health summary (diagnostic, not a probe) (*01982303-f0f9-7eec-8bf3-84f51fd09b73*)

```
GET /health/status
```

A human-readable summary including database connectivity and runtime metrics. NOT a probe: it pings the database inside a five second budget, so it hangs as long as the database does, and when the ping fails it answers 500 and discards the summary — no verdict a probe could act on. Point liveness at /health/live and readiness at /health/detailed.

#### Produces
  * application/json

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7eec-8bf3-84f51fd09b73-200) | OK | Service health status retrieved successfully |  | [schema](#01982303-f0f9-7eec-8bf3-84f51fd09b73-200-schema) |
| [500](#01982303-f0f9-7eec-8bf3-84f51fd09b73-500) | Internal Server Error | The check could not be completed -- currently, the database ping failed. The body carries a fixed message and never the underlying error: this endpoint is public, and the driver's text names the database user, the database and the addresses tried. The reason is on the span and in an ERROR log |  | [schema](#01982303-f0f9-7eec-8bf3-84f51fd09b73-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7eec-8bf3-84f51fd09b73-200"></span> 200 - Service health status retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7eec-8bf3-84f51fd09b73-200-schema"></span> Schema
   
  

[PayloadHealth](#payload-health)

##### <span id="01982303-f0f9-7eec-8bf3-84f51fd09b73-500"></span> 500 - The check could not be completed -- currently, the database ping failed. The body carries a fixed message and never the underlying error: this endpoint is public, and the driver's text names the database user, the database and the addresses tried. The reason is on the span and in an ERROR log
Status: Internal Server Error

###### <span id="01982303-f0f9-7eec-8bf3-84f51fd09b73-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1"></span> List roles by policy (*01982303-f0f9-7ef0-ad95-3cb214216ef1*)

```
GET /policies/{policy_id}/roles
```

Retrieve paginated list of roles associated with policy

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policy_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Policy unique identifier |
| fields | `query` | string | `string` |  |  |  | Comma-separated field names to include in response |
| filter | `query` | string | `string` |  |  |  | Filter expression (e.g., name='admin' AND system=true) |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items per page |
| next_token | `query` | string | `string` |  |  |  | Pagination token for next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination token for previous page |
| sort | `query` | string | `string` |  |  |  | Comma-separated sort fields with direction (e.g., name ASC, created_at DESC) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7ef0-ad95-3cb214216ef1-200) | OK | Paginated list of policy roles retrieved successfully |  | [schema](#01982303-f0f9-7ef0-ad95-3cb214216ef1-200-schema) |
| [400](#01982303-f0f9-7ef0-ad95-3cb214216ef1-400) | Bad Request | Invalid policy ID or query parameters |  | [schema](#01982303-f0f9-7ef0-ad95-3cb214216ef1-400-schema) |
| [401](#01982303-f0f9-7ef0-ad95-3cb214216ef1-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7ef0-ad95-3cb214216ef1-401-schema) |
| [403](#01982303-f0f9-7ef0-ad95-3cb214216ef1-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7ef0-ad95-3cb214216ef1-403-schema) |
| [429](#01982303-f0f9-7ef0-ad95-3cb214216ef1-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7ef0-ad95-3cb214216ef1-429-schema) |
| [500](#01982303-f0f9-7ef0-ad95-3cb214216ef1-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7ef0-ad95-3cb214216ef1-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-200"></span> 200 - Paginated list of policy roles retrieved successfully
Status: OK

###### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-200-schema"></span> Schema
   
  

[PayloadListRolesResponse](#payload-list-roles-response)

##### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-400"></span> 400 - Invalid policy ID or query parameters
Status: Bad Request

###### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7ef0-ad95-3cb214216ef1-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee"></span> Unlink policies from role (*01982303-f0f9-7efb-a786-89d7d9db40ee*)

```
DELETE /roles/{role_id}/policies
```

Remove policy associations from role

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |
| body | `body` | [PayloadUnlinkPoliciesFromRoleRequest](#payload-unlink-policies-from-role-request) | `models.PayloadUnlinkPoliciesFromRoleRequest` | | ✓ | | Policy IDs to unlink from role |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7efb-a786-89d7d9db40ee-200) | OK | Policies unlinked from role successfully |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-200-schema) |
| [400](#01982303-f0f9-7efb-a786-89d7d9db40ee-400) | Bad Request | Invalid request payload or role ID format |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-400-schema) |
| [401](#01982303-f0f9-7efb-a786-89d7d9db40ee-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-401-schema) |
| [403](#01982303-f0f9-7efb-a786-89d7d9db40ee-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-403-schema) |
| [404](#01982303-f0f9-7efb-a786-89d7d9db40ee-404) | Not Found | Role not found |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-404-schema) |
| [429](#01982303-f0f9-7efb-a786-89d7d9db40ee-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-429-schema) |
| [500](#01982303-f0f9-7efb-a786-89d7d9db40ee-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7efb-a786-89d7d9db40ee-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-200"></span> 200 - Policies unlinked from role successfully
Status: OK

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-400"></span> 400 - Invalid request payload or role ID format
Status: Bad Request

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7efb-a786-89d7d9db40ee-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7eff-825d-622a05ef4435"></span> Create role (*01982303-f0f9-7eff-825d-622a05ef4435*)

```
POST /roles
```

Create new role with specified permissions and access levels

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadCreateRoleRequest](#payload-create-role-request) | `models.PayloadCreateRoleRequest` | | ✓ | | Role creation payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#01982303-f0f9-7eff-825d-622a05ef4435-201) | Created | Role created successfully | ✓ | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-201-schema) |
| [400](#01982303-f0f9-7eff-825d-622a05ef4435-400) | Bad Request | Invalid request payload or validation failed |  | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-400-schema) |
| [401](#01982303-f0f9-7eff-825d-622a05ef4435-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-401-schema) |
| [403](#01982303-f0f9-7eff-825d-622a05ef4435-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-403-schema) |
| [409](#01982303-f0f9-7eff-825d-622a05ef4435-409) | Conflict | Role already exists |  | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-409-schema) |
| [429](#01982303-f0f9-7eff-825d-622a05ef4435-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-429-schema) |
| [500](#01982303-f0f9-7eff-825d-622a05ef4435-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7eff-825d-622a05ef4435-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-201"></span> 201 - Role created successfully
Status: Created

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | URL of created role resource |

##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-400"></span> 400 - Invalid request payload or validation failed
Status: Bad Request

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-409"></span> 409 - Role already exists
Status: Conflict

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7eff-825d-622a05ef4435-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7eff-825d-622a05ef4435-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b"></span> Delete policy (*01982303-f0f9-7f13-a03b-ed306ff7d06b*)

```
DELETE /policies/{policy_id}
```

Permanently remove an authorization policy by its unique identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| policy_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique policy identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7f13-a03b-ed306ff7d06b-200) | OK | Policy deleted successfully |  | [schema](#01982303-f0f9-7f13-a03b-ed306ff7d06b-200-schema) |
| [400](#01982303-f0f9-7f13-a03b-ed306ff7d06b-400) | Bad Request | Invalid policy ID format |  | [schema](#01982303-f0f9-7f13-a03b-ed306ff7d06b-400-schema) |
| [401](#01982303-f0f9-7f13-a03b-ed306ff7d06b-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7f13-a03b-ed306ff7d06b-401-schema) |
| [403](#01982303-f0f9-7f13-a03b-ed306ff7d06b-403) | Forbidden | System policies cannot be deleted |  | [schema](#01982303-f0f9-7f13-a03b-ed306ff7d06b-403-schema) |
| [429](#01982303-f0f9-7f13-a03b-ed306ff7d06b-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7f13-a03b-ed306ff7d06b-429-schema) |
| [500](#01982303-f0f9-7f13-a03b-ed306ff7d06b-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0f9-7f13-a03b-ed306ff7d06b-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-200"></span> 200 - Policy deleted successfully
Status: OK

###### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-400"></span> 400 - Invalid policy ID format
Status: Bad Request

###### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-403"></span> 403 - System policies cannot be deleted
Status: Forbidden

###### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0f9-7f13-a03b-ed306ff7d06b-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0f9-7f23-b203-7079222718d0"></span> Unlink roles from user (*01982303-f0f9-7f23-b203-7079222718d0*)

```
DELETE /users/{user_id}/roles
```

Remove role associations from user within project.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| body | `body` | [PayloadUnlinkRolesFromUserRequest](#payload-unlink-roles-from-user-request) | `models.PayloadUnlinkRolesFromUserRequest` | | ✓ | | Role IDs to unlink with project context |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0f9-7f23-b203-7079222718d0-200) | OK | Roles unlinked from user successfully |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-200-schema) |
| [400](#01982303-f0f9-7f23-b203-7079222718d0-400) | Bad Request | Invalid user ID or request body |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-400-schema) |
| [401](#01982303-f0f9-7f23-b203-7079222718d0-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-401-schema) |
| [403](#01982303-f0f9-7f23-b203-7079222718d0-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-403-schema) |
| [404](#01982303-f0f9-7f23-b203-7079222718d0-404) | Not Found | User not found |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-404-schema) |
| [409](#01982303-f0f9-7f23-b203-7079222718d0-409) | Conflict | Conflict |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-409-schema) |
| [429](#01982303-f0f9-7f23-b203-7079222718d0-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-429-schema) |
| [500](#01982303-f0f9-7f23-b203-7079222718d0-500) | Internal Server Error | Internal server error during role unlinking |  | [schema](#01982303-f0f9-7f23-b203-7079222718d0-500-schema) |

#### Responses


##### <span id="01982303-f0f9-7f23-b203-7079222718d0-200"></span> 200 - Roles unlinked from user successfully
Status: OK

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-400"></span> 400 - Invalid user ID or request body
Status: Bad Request

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-404"></span> 404 - User not found
Status: Not Found

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-409"></span> 409 - Conflict
Status: Conflict

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0f9-7f23-b203-7079222718d0-500"></span> 500 - Internal server error during role unlinking
Status: Internal Server Error

###### <span id="01982303-f0f9-7f23-b203-7079222718d0-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec"></span> Delete role (*01982303-f0fa-7007-aaf6-462c0b8702ec*)

```
DELETE /roles/{role_id}
```

Permanently remove role and all associations

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0fa-7007-aaf6-462c0b8702ec-200) | OK | Role deleted successfully |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-200-schema) |
| [400](#01982303-f0fa-7007-aaf6-462c0b8702ec-400) | Bad Request | Invalid role ID format |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-400-schema) |
| [401](#01982303-f0fa-7007-aaf6-462c0b8702ec-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-401-schema) |
| [403](#01982303-f0fa-7007-aaf6-462c0b8702ec-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-403-schema) |
| [404](#01982303-f0fa-7007-aaf6-462c0b8702ec-404) | Not Found | Role not found |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-404-schema) |
| [429](#01982303-f0fa-7007-aaf6-462c0b8702ec-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-429-schema) |
| [500](#01982303-f0fa-7007-aaf6-462c0b8702ec-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0fa-7007-aaf6-462c0b8702ec-500-schema) |

#### Responses


##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-200"></span> 200 - Role deleted successfully
Status: OK

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-400"></span> 400 - Invalid role ID format
Status: Bad Request

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-404"></span> 404 - Role not found
Status: Not Found

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0fa-7007-aaf6-462c0b8702ec-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb"></span> Link roles to user (*01982303-f0fa-700f-a042-0a487ed3c9fb*)

```
POST /users/{user_id}/roles
```

Associate multiple roles with user within project.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| user | `body` | [PayloadLinkRolesToUserRequest](#payload-link-roles-to-user-request) | `models.PayloadLinkRolesToUserRequest` | | ✓ | | Role IDs to link with project context |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0fa-700f-a042-0a487ed3c9fb-200) | OK | Roles linked to user successfully |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-200-schema) |
| [400](#01982303-f0fa-700f-a042-0a487ed3c9fb-400) | Bad Request | Invalid user ID or request body |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-400-schema) |
| [401](#01982303-f0fa-700f-a042-0a487ed3c9fb-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-401-schema) |
| [403](#01982303-f0fa-700f-a042-0a487ed3c9fb-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-403-schema) |
| [404](#01982303-f0fa-700f-a042-0a487ed3c9fb-404) | Not Found | User not found |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-404-schema) |
| [409](#01982303-f0fa-700f-a042-0a487ed3c9fb-409) | Conflict | Role already linked to user |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-409-schema) |
| [429](#01982303-f0fa-700f-a042-0a487ed3c9fb-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-429-schema) |
| [500](#01982303-f0fa-700f-a042-0a487ed3c9fb-500) | Internal Server Error | Internal server error during role linking |  | [schema](#01982303-f0fa-700f-a042-0a487ed3c9fb-500-schema) |

#### Responses


##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-200"></span> 200 - Roles linked to user successfully
Status: OK

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-400"></span> 400 - Invalid user ID or request body
Status: Bad Request

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-404"></span> 404 - User not found
Status: Not Found

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-409"></span> 409 - Role already linked to user
Status: Conflict

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-500"></span> 500 - Internal server error during role linking
Status: Internal Server Error

###### <span id="01982303-f0fa-700f-a042-0a487ed3c9fb-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0fa-7027-9b77-197b693d0e5a"></span> List users by role (*01982303-f0fa-7027-9b77-197b693d0e5a*)

```
GET /roles/{role_id}/users
```

Retrieve paginated list of users assigned to specific role.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Role unique identifier |
| fields | `query` | string | `string` |  |  |  | Fields to return (comma-separated). Example: id,first_name,last_name |
| filter | `query` | string | `string` |  |  |  | Filter expression. Example: id=1 AND first_name='John' |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of results per page |
| next_token | `query` | string | `string` |  |  |  | Next page cursor for pagination |
| prev_token | `query` | string | `string` |  |  |  | Previous page cursor for pagination |
| sort | `query` | string | `string` |  |  |  | Sort fields (comma-separated). Example: first_name ASC, created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0fa-7027-9b77-197b693d0e5a-200) | OK | Paginated list of users with specified role |  | [schema](#01982303-f0fa-7027-9b77-197b693d0e5a-200-schema) |
| [400](#01982303-f0fa-7027-9b77-197b693d0e5a-400) | Bad Request | Invalid role ID or query parameters |  | [schema](#01982303-f0fa-7027-9b77-197b693d0e5a-400-schema) |
| [401](#01982303-f0fa-7027-9b77-197b693d0e5a-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0fa-7027-9b77-197b693d0e5a-401-schema) |
| [403](#01982303-f0fa-7027-9b77-197b693d0e5a-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0fa-7027-9b77-197b693d0e5a-403-schema) |
| [429](#01982303-f0fa-7027-9b77-197b693d0e5a-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0fa-7027-9b77-197b693d0e5a-429-schema) |
| [500](#01982303-f0fa-7027-9b77-197b693d0e5a-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0fa-7027-9b77-197b693d0e5a-500-schema) |

#### Responses


##### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-200"></span> 200 - Paginated list of users with specified role
Status: OK

###### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-200-schema"></span> Schema
   
  

[PayloadListUsersResponse](#payload-list-users-response)

##### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-400"></span> 400 - Invalid role ID or query parameters
Status: Bad Request

###### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0fa-7027-9b77-197b693d0e5a-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0fa-7036-9474-482fc8e5843d"></span> List policies by role (*01982303-f0fa-7036-9474-482fc8e5843d*)

```
GET /roles/{role_id}/policies
```

Retrieve a paginated list of policies associated with a specific role

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| role_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique role identifier |
| fields | `query` | string | `string` |  |  |  | Comma-separated fields to return (e.g., 'id,name,description') |
| filter | `query` | string | `string` |  |  |  | Filter conditions (e.g., 'name LIKE policy%') |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items per page |
| next_token | `query` | string | `string` |  |  |  | Token for next page of results |
| prev_token | `query` | string | `string` |  |  |  | Token for previous page of results |
| sort | `query` | string | `string` |  |  |  | Sort by fields (e.g., 'name ASC, created_at DESC') |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0fa-7036-9474-482fc8e5843d-200) | OK | Policies retrieved successfully |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-200-schema) |
| [400](#01982303-f0fa-7036-9474-482fc8e5843d-400) | Bad Request | Invalid query parameters |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-400-schema) |
| [401](#01982303-f0fa-7036-9474-482fc8e5843d-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-401-schema) |
| [403](#01982303-f0fa-7036-9474-482fc8e5843d-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-403-schema) |
| [404](#01982303-f0fa-7036-9474-482fc8e5843d-404) | Not Found | No policies found for the given role |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-404-schema) |
| [429](#01982303-f0fa-7036-9474-482fc8e5843d-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-429-schema) |
| [500](#01982303-f0fa-7036-9474-482fc8e5843d-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0fa-7036-9474-482fc8e5843d-500-schema) |

#### Responses


##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-200"></span> 200 - Policies retrieved successfully
Status: OK

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-200-schema"></span> Schema
   
  

[PayloadListPoliciesResponse](#payload-list-policies-response)

##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-400"></span> 400 - Invalid query parameters
Status: Bad Request

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-404"></span> 404 - No policies found for the given role
Status: Not Found

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7036-9474-482fc8e5843d-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0fa-7036-9474-482fc8e5843d-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0fa-703a-92fa-be272044b2e3"></span> List roles (*01982303-f0fa-703a-92fa-be272044b2e3*)

```
GET /roles
```

Retrieve paginated list of roles with filtering and sorting

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated field names to include in response |
| filter | `query` | string | `string` |  |  |  | Filter expression (e.g., name='admin' AND system=true) |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items per page |
| next_token | `query` | string | `string` |  |  |  | Pagination token for next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination token for previous page |
| sort | `query` | string | `string` |  |  |  | Comma-separated sort fields with direction (e.g., name ASC, created_at DESC) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0fa-703a-92fa-be272044b2e3-200) | OK | Paginated list of roles retrieved successfully |  | [schema](#01982303-f0fa-703a-92fa-be272044b2e3-200-schema) |
| [400](#01982303-f0fa-703a-92fa-be272044b2e3-400) | Bad Request | Invalid query parameters or pagination tokens |  | [schema](#01982303-f0fa-703a-92fa-be272044b2e3-400-schema) |
| [401](#01982303-f0fa-703a-92fa-be272044b2e3-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0fa-703a-92fa-be272044b2e3-401-schema) |
| [403](#01982303-f0fa-703a-92fa-be272044b2e3-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0fa-703a-92fa-be272044b2e3-403-schema) |
| [429](#01982303-f0fa-703a-92fa-be272044b2e3-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0fa-703a-92fa-be272044b2e3-429-schema) |
| [500](#01982303-f0fa-703a-92fa-be272044b2e3-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0fa-703a-92fa-be272044b2e3-500-schema) |

#### Responses


##### <span id="01982303-f0fa-703a-92fa-be272044b2e3-200"></span> 200 - Paginated list of roles retrieved successfully
Status: OK

###### <span id="01982303-f0fa-703a-92fa-be272044b2e3-200-schema"></span> Schema
   
  

[PayloadListRolesResponse](#payload-list-roles-response)

##### <span id="01982303-f0fa-703a-92fa-be272044b2e3-400"></span> 400 - Invalid query parameters or pagination tokens
Status: Bad Request

###### <span id="01982303-f0fa-703a-92fa-be272044b2e3-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-703a-92fa-be272044b2e3-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0fa-703a-92fa-be272044b2e3-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-703a-92fa-be272044b2e3-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0fa-703a-92fa-be272044b2e3-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-703a-92fa-be272044b2e3-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0fa-703a-92fa-be272044b2e3-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-703a-92fa-be272044b2e3-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0fa-703a-92fa-be272044b2e3-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6"></span> Get user authorization (*01982303-f0fa-7089-9875-cd42f8e1a3d6*)

```
GET /users/{user_id}/authz
```

Retrieve user authorization permissions and roles for access control decisions.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982303-f0fa-7089-9875-cd42f8e1a3d6-200) | OK | The user's effective permissions, keyed by category. Note the extra nesting compared with /me/authz, which strips the outer \"permissions\" level |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-200-schema) |
| [400](#01982303-f0fa-7089-9875-cd42f8e1a3d6-400) | Bad Request | Invalid user ID format |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-400-schema) |
| [401](#01982303-f0fa-7089-9875-cd42f8e1a3d6-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-401-schema) |
| [403](#01982303-f0fa-7089-9875-cd42f8e1a3d6-403) | Forbidden | Insufficient permissions |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-403-schema) |
| [404](#01982303-f0fa-7089-9875-cd42f8e1a3d6-404) | Not Found | User not found |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-404-schema) |
| [429](#01982303-f0fa-7089-9875-cd42f8e1a3d6-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-429-schema) |
| [500](#01982303-f0fa-7089-9875-cd42f8e1a3d6-500) | Internal Server Error | Internal server error |  | [schema](#01982303-f0fa-7089-9875-cd42f8e1a3d6-500-schema) |

#### Responses


##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-200"></span> 200 - The user's effective permissions, keyed by category. Note the extra nesting compared with /me/authz, which strips the outer \"permissions\" level
Status: OK

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-200-schema"></span> Schema
   
  

[PayloadUserAuthzResponse](#payload-user-authz-response)

##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-400"></span> 400 - Invalid user ID format
Status: Bad Request

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-404"></span> 404 - User not found
Status: Not Found

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01982303-f0fa-7089-9875-cd42f8e1a3d6-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74"></span> Readiness probe and detailed health (*01982304-a1b2-7eec-8bf3-84f51fd09b74*)

```
GET /health/detailed
```

Per-component health, database pool stats and startup metrics, with the status code carrying the verdict: 200 healthy, 206 degraded, 503 a hard dependency is unreachable. This is the READINESS target — it answers whether this instance should receive traffic. Point liveness at /health/live, which deliberately checks nothing.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01982304-a1b2-7eec-8bf3-84f51fd09b74-200) | OK | Every component is healthy |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-200-schema) |
| [206](#01982304-a1b2-7eec-8bf3-84f51fd09b74-206) | Partial Content | One or more components are DEGRADED; the same payload is returned with the per-component details. A client polling this endpoint must treat 206 as a reachable service, not as a failure. Note that ratelimit_store degraded under the default fail-closed mode means the service is refusing every request with 429 -- reachable, but serving nothing |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-206-schema) |
| [401](#01982304-a1b2-7eec-8bf3-84f51fd09b74-401) | Unauthorized | Invalid or expired token. This endpoint names every component, its configuration and its timings, so it requires authentication -- unlike /health/live and /health/status, which an orchestrator must be able to reach without a token |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-401-schema) |
| [403](#01982304-a1b2-7eec-8bf3-84f51fd09b74-403) | Forbidden | Not authorized |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-403-schema) |
| [429](#01982304-a1b2-7eec-8bf3-84f51fd09b74-429) | Too Many Requests | Too many requests |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-429-schema) |
| [500](#01982304-a1b2-7eec-8bf3-84f51fd09b74-500) | Internal Server Error | The check could not be completed. The body carries a fixed message and never the underlying error. The reason is on the span and in an ERROR log |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-500-schema) |
| [503](#01982304-a1b2-7eec-8bf3-84f51fd09b74-503) | Service Unavailable | A HARD dependency is unreachable -- currently the database -- so this instance cannot serve. A load balancer or readiness probe should take it out of rotation. Two components are deliberately excluded from this code and can only reach 206: the cache, because it is fail-open and a request still succeeds without it; and the rate-limit store, because health and version bypass the limiter precisely so its outage cannot evict a replica, and failing readiness would reintroduce that eviction by the other door -- on every replica at once, since they share the store |  | [schema](#01982304-a1b2-7eec-8bf3-84f51fd09b74-503-schema) |

#### Responses


##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-200"></span> 200 - Every component is healthy
Status: OK

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-200-schema"></span> Schema
   
  

[PayloadDetailedHealth](#payload-detailed-health)

##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-206"></span> 206 - One or more components are DEGRADED; the same payload is returned with the per-component details. A client polling this endpoint must treat 206 as a reachable service, not as a failure. Note that ratelimit_store degraded under the default fail-closed mode means the service is refusing every request with 429 -- reachable, but serving nothing
Status: Partial Content

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-206-schema"></span> Schema
   
  

[PayloadDetailedHealth](#payload-detailed-health)

##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-401"></span> 401 - Invalid or expired token. This endpoint names every component, its configuration and its timings, so it requires authentication -- unlike /health/live and /health/status, which an orchestrator must be able to reach without a token
Status: Unauthorized

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-403"></span> 403 - Not authorized
Status: Forbidden

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-500"></span> 500 - The check could not be completed. The body carries a fixed message and never the underlying error. The reason is on the span and in an ERROR log
Status: Internal Server Error

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-503"></span> 503 - A HARD dependency is unreachable -- currently the database -- so this instance cannot serve. A load balancer or readiness probe should take it out of rotation. Two components are deliberately excluded from this code and can only reach 206: the cache, because it is fail-open and a request still succeeds without it; and the rate-limit store, because health and version bypass the limiter precisely so its outage cannot evict a replica, and failing readiness would reintroduce that eviction by the other door -- on every replica at once, since they share the store
Status: Service Unavailable

###### <span id="01982304-a1b2-7eec-8bf3-84f51fd09b74-503-schema"></span> Schema
   
  

[PayloadDetailedHealth](#payload-detailed-health)

### <span id="01986f44-3a65-7a19-a92d-e6100dd80807"></span> Unlink users from project (*01986f44-3a65-7a19-a92d-e6100dd80807*)

```
DELETE /projects/{project_id}/users
```

Remove user associations from project.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project unique identifier |
| body | `body` | [PayloadUnlinkUsersFromProjectRequest](#payload-unlink-users-from-project-request) | `models.PayloadUnlinkUsersFromProjectRequest` | | ✓ | | User IDs to unlink from project |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01986f44-3a65-7a19-a92d-e6100dd80807-200) | OK | Users unlinked from project successfully |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-200-schema) |
| [400](#01986f44-3a65-7a19-a92d-e6100dd80807-400) | Bad Request | Invalid project ID or request body |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-400-schema) |
| [401](#01986f44-3a65-7a19-a92d-e6100dd80807-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-401-schema) |
| [403](#01986f44-3a65-7a19-a92d-e6100dd80807-403) | Forbidden | Insufficient permissions |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-403-schema) |
| [404](#01986f44-3a65-7a19-a92d-e6100dd80807-404) | Not Found | Project or user not found |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-404-schema) |
| [429](#01986f44-3a65-7a19-a92d-e6100dd80807-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-429-schema) |
| [500](#01986f44-3a65-7a19-a92d-e6100dd80807-500) | Internal Server Error | Internal server error during user unlinking |  | [schema](#01986f44-3a65-7a19-a92d-e6100dd80807-500-schema) |

#### Responses


##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-200"></span> 200 - Users unlinked from project successfully
Status: OK

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-400"></span> 400 - Invalid project ID or request body
Status: Bad Request

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-404"></span> 404 - Project or user not found
Status: Not Found

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-500"></span> 500 - Internal server error during user unlinking
Status: Internal Server Error

###### <span id="01986f44-3a65-7a19-a92d-e6100dd80807-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7"></span> Link users to project (*01986f44-3a65-7a21-9c2b-392f2b0eacf7*)

```
POST /projects/{project_id}/users
```

Associate multiple users with project.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project unique identifier |
| body | `body` | [PayloadLinkUsersToProjectRequest](#payload-link-users-to-project-request) | `models.PayloadLinkUsersToProjectRequest` | | ✓ | | User IDs to link to project |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-200) | OK | Users linked to project successfully |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-200-schema) |
| [400](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-400) | Bad Request | Invalid project ID or request body |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-400-schema) |
| [401](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-401-schema) |
| [403](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-403) | Forbidden | Insufficient permissions |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-403-schema) |
| [404](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-404) | Not Found | Project not found |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-404-schema) |
| [409](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-409) | Conflict | One or more users are already linked to the project |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-409-schema) |
| [429](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-429-schema) |
| [500](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-500) | Internal Server Error | Internal server error during user linking |  | [schema](#01986f44-3a65-7a21-9c2b-392f2b0eacf7-500-schema) |

#### Responses


##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-200"></span> 200 - Users linked to project successfully
Status: OK

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-400"></span> 400 - Invalid project ID or request body
Status: Bad Request

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-404"></span> 404 - Project not found
Status: Not Found

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-409"></span> 409 - One or more users are already linked to the project
Status: Conflict

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-500"></span> 500 - Internal server error during user linking
Status: Internal Server Error

###### <span id="01986f44-3a65-7a21-9c2b-392f2b0eacf7-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4"></span> Unlink projects from user (*01986f44-3a65-7a25-afe8-fdd6ae4572c4*)

```
DELETE /users/{user_id}/projects
```

Remove project associations from user.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| body | `body` | [PayloadUnlinkProjectsFromUserRequest](#payload-unlink-projects-from-user-request) | `models.PayloadUnlinkProjectsFromUserRequest` | | ✓ | | Project IDs to unlink |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-200) | OK | Projects unlinked from user successfully |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-200-schema) |
| [400](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-400) | Bad Request | Invalid user ID or request body |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-400-schema) |
| [401](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-401-schema) |
| [403](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-403) | Forbidden | Insufficient permissions |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-403-schema) |
| [404](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-404) | Not Found | User not found |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-404-schema) |
| [429](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-429-schema) |
| [500](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-500) | Internal Server Error | Internal server error during project unlinking |  | [schema](#01986f44-3a65-7a25-afe8-fdd6ae4572c4-500-schema) |

#### Responses


##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-200"></span> 200 - Projects unlinked from user successfully
Status: OK

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-400"></span> 400 - Invalid user ID or request body
Status: Bad Request

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-404"></span> 404 - User not found
Status: Not Found

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-500"></span> 500 - Internal server error during project unlinking
Status: Internal Server Error

###### <span id="01986f44-3a65-7a25-afe8-fdd6ae4572c4-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7"></span> Link projects to user (*01986f44-3a65-7a2d-8c68-8c579be0aae7*)

```
POST /users/{user_id}/projects
```

Associate multiple projects with user.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| user | `body` | [PayloadLinkProjectsToUserRequest](#payload-link-projects-to-user-request) | `models.PayloadLinkProjectsToUserRequest` | | ✓ | | Project IDs to link |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01986f44-3a65-7a2d-8c68-8c579be0aae7-200) | OK | Projects linked to user successfully |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-200-schema) |
| [400](#01986f44-3a65-7a2d-8c68-8c579be0aae7-400) | Bad Request | Invalid user ID or request body |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-400-schema) |
| [401](#01986f44-3a65-7a2d-8c68-8c579be0aae7-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-401-schema) |
| [403](#01986f44-3a65-7a2d-8c68-8c579be0aae7-403) | Forbidden | Insufficient permissions |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-403-schema) |
| [404](#01986f44-3a65-7a2d-8c68-8c579be0aae7-404) | Not Found | User not found |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-404-schema) |
| [409](#01986f44-3a65-7a2d-8c68-8c579be0aae7-409) | Conflict | One or more projects already linked to user |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-409-schema) |
| [429](#01986f44-3a65-7a2d-8c68-8c579be0aae7-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-429-schema) |
| [500](#01986f44-3a65-7a2d-8c68-8c579be0aae7-500) | Internal Server Error | Internal server error during project linking |  | [schema](#01986f44-3a65-7a2d-8c68-8c579be0aae7-500-schema) |

#### Responses


##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-200"></span> 200 - Projects linked to user successfully
Status: OK

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-400"></span> 400 - Invalid user ID or request body
Status: Bad Request

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-404"></span> 404 - User not found
Status: Not Found

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-409"></span> 409 - One or more projects already linked to user
Status: Conflict

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-500"></span> 500 - Internal server error during project linking
Status: Internal Server Error

###### <span id="01986f44-3a65-7a2d-8c68-8c579be0aae7-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2"></span> List users by project (*01987096-b4a1-7e8a-8a38-98148daa27a2*)

```
GET /projects/{project_id}/users
```

Retrieve paginated list of users assigned to specific project.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project unique identifier |
| fields | `query` | string | `string` |  |  |  | Fields to return (comma-separated). Example: id,first_name,last_name |
| filter | `query` | string | `string` |  |  |  | Filter expression. Example: id=1 AND first_name='John' |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of results per page |
| next_token | `query` | string | `string` |  |  |  | Next page cursor for pagination |
| prev_token | `query` | string | `string` |  |  |  | Previous page cursor for pagination |
| sort | `query` | string | `string` |  |  |  | Sort fields (comma-separated). Example: first_name ASC, created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01987096-b4a1-7e8a-8a38-98148daa27a2-200) | OK | Paginated list of users in specified project |  | [schema](#01987096-b4a1-7e8a-8a38-98148daa27a2-200-schema) |
| [400](#01987096-b4a1-7e8a-8a38-98148daa27a2-400) | Bad Request | Invalid project ID or query parameters |  | [schema](#01987096-b4a1-7e8a-8a38-98148daa27a2-400-schema) |
| [401](#01987096-b4a1-7e8a-8a38-98148daa27a2-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01987096-b4a1-7e8a-8a38-98148daa27a2-401-schema) |
| [403](#01987096-b4a1-7e8a-8a38-98148daa27a2-403) | Forbidden | Insufficient permissions |  | [schema](#01987096-b4a1-7e8a-8a38-98148daa27a2-403-schema) |
| [429](#01987096-b4a1-7e8a-8a38-98148daa27a2-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01987096-b4a1-7e8a-8a38-98148daa27a2-429-schema) |
| [500](#01987096-b4a1-7e8a-8a38-98148daa27a2-500) | Internal Server Error | Internal server error |  | [schema](#01987096-b4a1-7e8a-8a38-98148daa27a2-500-schema) |

#### Responses


##### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-200"></span> 200 - Paginated list of users in specified project
Status: OK

###### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-200-schema"></span> Schema
   
  

[PayloadListUsersResponse](#payload-list-users-response)

##### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-400"></span> 400 - Invalid project ID or query parameters
Status: Bad Request

###### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01987096-b4a1-7e8a-8a38-98148daa27a2-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019870ff-37f6-737e-8efb-e39730ef6952"></span> List projects by user (*019870ff-37f6-737e-8efb-e39730ef6952*)

```
GET /users/{user_id}/projects
```

Retrieve paginated list of projects accessible to specific user.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| user_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | User unique identifier |
| fields | `query` | string | `string` |  |  |  | Fields to return (comma-separated). Example: id,name,description |
| filter | `query` | string | `string` |  |  |  | Filter expression. Example: name LIKE '%test%' |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of results per page |
| next_token | `query` | string | `string` |  |  |  | Next page cursor for pagination |
| prev_token | `query` | string | `string` |  |  |  | Previous page cursor for pagination |
| sort | `query` | string | `string` |  |  |  | Sort fields (comma-separated). Example: name ASC, created_at DESC |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019870ff-37f6-737e-8efb-e39730ef6952-200) | OK | Paginated list of user's projects |  | [schema](#019870ff-37f6-737e-8efb-e39730ef6952-200-schema) |
| [400](#019870ff-37f6-737e-8efb-e39730ef6952-400) | Bad Request | Invalid user ID or query parameters |  | [schema](#019870ff-37f6-737e-8efb-e39730ef6952-400-schema) |
| [401](#019870ff-37f6-737e-8efb-e39730ef6952-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#019870ff-37f6-737e-8efb-e39730ef6952-401-schema) |
| [403](#019870ff-37f6-737e-8efb-e39730ef6952-403) | Forbidden | Insufficient permissions |  | [schema](#019870ff-37f6-737e-8efb-e39730ef6952-403-schema) |
| [429](#019870ff-37f6-737e-8efb-e39730ef6952-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#019870ff-37f6-737e-8efb-e39730ef6952-429-schema) |
| [500](#019870ff-37f6-737e-8efb-e39730ef6952-500) | Internal Server Error | Internal server error |  | [schema](#019870ff-37f6-737e-8efb-e39730ef6952-500-schema) |

#### Responses


##### <span id="019870ff-37f6-737e-8efb-e39730ef6952-200"></span> 200 - Paginated list of user's projects
Status: OK

###### <span id="019870ff-37f6-737e-8efb-e39730ef6952-200-schema"></span> Schema
   
  

[PayloadListProjectsResponse](#payload-list-projects-response)

##### <span id="019870ff-37f6-737e-8efb-e39730ef6952-400"></span> 400 - Invalid user ID or query parameters
Status: Bad Request

###### <span id="019870ff-37f6-737e-8efb-e39730ef6952-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019870ff-37f6-737e-8efb-e39730ef6952-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="019870ff-37f6-737e-8efb-e39730ef6952-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019870ff-37f6-737e-8efb-e39730ef6952-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="019870ff-37f6-737e-8efb-e39730ef6952-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019870ff-37f6-737e-8efb-e39730ef6952-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="019870ff-37f6-737e-8efb-e39730ef6952-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019870ff-37f6-737e-8efb-e39730ef6952-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="019870ff-37f6-737e-8efb-e39730ef6952-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3"></span> Initiate IDP login (*01988e60-89e5-72ab-adb4-3eef95d1afd3*)

```
GET /auth/idp/{idp_id}/login
```

Initiate authentication with specified Identity Provider and returns redirect URL for OAuth flow.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Identity Provider unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01988e60-89e5-72ab-adb4-3eef95d1afd3-200) | OK | Login URL generated successfully. RedirectURL and RedirectCode are fields of the JSON body — this endpoint returns 200, it does not itself redirect. |  | [schema](#01988e60-89e5-72ab-adb4-3eef95d1afd3-200-schema) |
| [400](#01988e60-89e5-72ab-adb4-3eef95d1afd3-400) | Bad Request | Invalid IDP ID format or malformed request |  | [schema](#01988e60-89e5-72ab-adb4-3eef95d1afd3-400-schema) |
| [500](#01988e60-89e5-72ab-adb4-3eef95d1afd3-500) | Internal Server Error | Internal server error during URL generation. NOTE: an unknown Identity Provider currently surfaces here rather than as a 404 — the handler has no not-found branch. Tracked as a behaviour defect; this annotation documents what the endpoint does today, not what it should do. |  | [schema](#01988e60-89e5-72ab-adb4-3eef95d1afd3-500-schema) |

#### Responses


##### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3-200"></span> 200 - Login URL generated successfully. RedirectURL and RedirectCode are fields of the JSON body — this endpoint returns 200, it does not itself redirect.
Status: OK

###### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3-200-schema"></span> Schema
   
  

[PayloadIDPLoginResponse](#payload-id-p-login-response)

##### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3-400"></span> 400 - Invalid IDP ID format or malformed request
Status: Bad Request

###### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3-500"></span> 500 - Internal server error during URL generation. NOTE: an unknown Identity Provider currently surfaces here rather than as a 404 — the handler has no not-found branch. Tracked as a behaviour defect; this annotation documents what the endpoint does today, not what it should do.
Status: Internal Server Error

###### <span id="01988e60-89e5-72ab-adb4-3eef95d1afd3-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01988e60-89e5-72ee-9db4-db5cd7535717"></span> Handle IDP OAuth callback (*01988e60-89e5-72ee-9db4-db5cd7535717*)

```
GET /auth/idp/{idp_id}/callback
```

Process OAuth callback from Identity Provider, validates state and authorization code.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Identity Provider unique identifier |
| code | `query` | string | `string` |  | ✓ |  | OAuth authorization code from IDP |
| state | `query` | string | `string` |  | ✓ |  | OAuth state parameter for CSRF protection |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [302](#01988e60-89e5-72ee-9db4-db5cd7535717-302) | Found | Callback processed successfully. Authentication cookies are set and the caller is redirected to the IDP's configured login or register redirect URL. This endpoint never returns a JSON body on success — it always issues a redirect. | ✓ | [schema](#01988e60-89e5-72ee-9db4-db5cd7535717-302-schema) |
| [400](#01988e60-89e5-72ee-9db4-db5cd7535717-400) | Bad Request | Invalid parameters, missing state/code, or invalid IDP ID format |  | [schema](#01988e60-89e5-72ee-9db4-db5cd7535717-400-schema) |
| [404](#01988e60-89e5-72ee-9db4-db5cd7535717-404) | Not Found | Identity Provider not found |  | [schema](#01988e60-89e5-72ee-9db4-db5cd7535717-404-schema) |
| [409](#01988e60-89e5-72ee-9db4-db5cd7535717-409) | Conflict | User already exists during registration |  | [schema](#01988e60-89e5-72ee-9db4-db5cd7535717-409-schema) |
| [500](#01988e60-89e5-72ee-9db4-db5cd7535717-500) | Internal Server Error | Internal server error during callback processing |  | [schema](#01988e60-89e5-72ee-9db4-db5cd7535717-500-schema) |

#### Responses


##### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-302"></span> 302 - Callback processed successfully. Authentication cookies are set and the caller is redirected to the IDP's configured login or register redirect URL. This endpoint never returns a JSON body on success — it always issues a redirect.
Status: Found

###### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-302-schema"></span> Schema
   
  



###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | The IDP's configured LoginRedirectURL or RegisterRedirectURL |

##### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-400"></span> 400 - Invalid parameters, missing state/code, or invalid IDP ID format
Status: Bad Request

###### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-404"></span> 404 - Identity Provider not found
Status: Not Found

###### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-409"></span> 409 - User already exists during registration
Status: Conflict

###### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-500"></span> 500 - Internal server error during callback processing
Status: Internal Server Error

###### <span id="01988e60-89e5-72ee-9db4-db5cd7535717-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="019894ba-6014-79cf-bff4-6668484cc7e3"></span> Initiate IDP registration (*019894ba-6014-79cf-bff4-6668484cc7e3*)

```
GET /auth/idp/{idp_id}/register
```

Initiate user registration with specified Identity Provider and returns redirect URL for OAuth registration flow.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Identity Provider unique identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#019894ba-6014-79cf-bff4-6668484cc7e3-200) | OK | Registration URL generated successfully. RedirectURL and RedirectCode are fields of the JSON body — this endpoint returns 200, it does not itself redirect. |  | [schema](#019894ba-6014-79cf-bff4-6668484cc7e3-200-schema) |
| [400](#019894ba-6014-79cf-bff4-6668484cc7e3-400) | Bad Request | Invalid IDP ID format or malformed request |  | [schema](#019894ba-6014-79cf-bff4-6668484cc7e3-400-schema) |
| [500](#019894ba-6014-79cf-bff4-6668484cc7e3-500) | Internal Server Error | Internal server error during URL generation. NOTE: an unknown Identity Provider currently surfaces here rather than as a 404 — the handler has no not-found branch. Tracked as a behaviour defect; this annotation documents what the endpoint does today, not what it should do. |  | [schema](#019894ba-6014-79cf-bff4-6668484cc7e3-500-schema) |

#### Responses


##### <span id="019894ba-6014-79cf-bff4-6668484cc7e3-200"></span> 200 - Registration URL generated successfully. RedirectURL and RedirectCode are fields of the JSON body — this endpoint returns 200, it does not itself redirect.
Status: OK

###### <span id="019894ba-6014-79cf-bff4-6668484cc7e3-200-schema"></span> Schema
   
  

[PayloadIDPRegisterResponse](#payload-id-p-register-response)

##### <span id="019894ba-6014-79cf-bff4-6668484cc7e3-400"></span> 400 - Invalid IDP ID format or malformed request
Status: Bad Request

###### <span id="019894ba-6014-79cf-bff4-6668484cc7e3-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="019894ba-6014-79cf-bff4-6668484cc7e3-500"></span> 500 - Internal server error during URL generation. NOTE: an unknown Identity Provider currently surfaces here rather than as a 404 — the handler has no not-found branch. Tracked as a behaviour defect; this annotation documents what the endpoint does today, not what it should do.
Status: Internal Server Error

###### <span id="019894ba-6014-79cf-bff4-6668484cc7e3-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198e7ea-3755-7a29-90ed-13245b54f074"></span> List IDPs (*0198e7ea-3755-7a29-90ed-13245b54f074*)

```
GET /auth/idps
```

Retrieve paginated list of Identity Providers with optional filtering and sorting

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated fields to return (e.g., id,name,idp_type) |
| filter | `query` | string | `string` |  |  |  | Filter expression (e.g., idp_type='oauth2' AND name LIKE 'Google%') |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items per page (default: system-defined) |
| next_token | `query` | string | `string` |  |  |  | Pagination token for next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination token for previous page |
| sort | `query` | string | `string` |  |  |  | Sort order (e.g., name ASC, created_at DESC) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198e7ea-3755-7a29-90ed-13245b54f074-200) | OK | List of IDPs with pagination metadata |  | [schema](#0198e7ea-3755-7a29-90ed-13245b54f074-200-schema) |
| [400](#0198e7ea-3755-7a29-90ed-13245b54f074-400) | Bad Request | Invalid query parameters, filter syntax, or sort fields |  | [schema](#0198e7ea-3755-7a29-90ed-13245b54f074-400-schema) |
| [401](#0198e7ea-3755-7a29-90ed-13245b54f074-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198e7ea-3755-7a29-90ed-13245b54f074-401-schema) |
| [403](#0198e7ea-3755-7a29-90ed-13245b54f074-403) | Forbidden | Insufficient permissions |  | [schema](#0198e7ea-3755-7a29-90ed-13245b54f074-403-schema) |
| [429](#0198e7ea-3755-7a29-90ed-13245b54f074-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198e7ea-3755-7a29-90ed-13245b54f074-429-schema) |
| [500](#0198e7ea-3755-7a29-90ed-13245b54f074-500) | Internal Server Error | Internal server error |  | [schema](#0198e7ea-3755-7a29-90ed-13245b54f074-500-schema) |

#### Responses


##### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-200"></span> 200 - List of IDPs with pagination metadata
Status: OK

###### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-200-schema"></span> Schema
   
  

[PayloadListIDPsResponse](#payload-list-id-ps-response)

##### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-400"></span> 400 - Invalid query parameters, filter syntax, or sort fields
Status: Bad Request

###### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198e7ea-3755-7a29-90ed-13245b54f074-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37"></span> Delete IDP (*0198e7ea-3755-7a2d-9ab0-83ccef188e37*)

```
DELETE /auth/idps/{idp_id}
```

Permanently remove an Identity Provider configuration

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Identity Provider UUID |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-200) | OK | IDP deleted successfully |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-200-schema) |
| [400](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-400) | Bad Request | Invalid UUID format or malformed request |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-400-schema) |
| [401](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-401-schema) |
| [403](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-403) | Forbidden | Insufficient permissions |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-403-schema) |
| [404](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-404) | Not Found | IDP not found |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-404-schema) |
| [429](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-429-schema) |
| [500](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-500) | Internal Server Error | Internal server error |  | [schema](#0198e7ea-3755-7a2d-9ab0-83ccef188e37-500-schema) |

#### Responses


##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-200"></span> 200 - IDP deleted successfully
Status: OK

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-400"></span> 400 - Invalid UUID format or malformed request
Status: Bad Request

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-404"></span> 404 - IDP not found
Status: Not Found

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198e7ea-3755-7a2d-9ab0-83ccef188e37-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1"></span> Update IDP (*0198e7ea-3755-7a35-9e30-6a9392e8e7a1*)

```
PUT /auth/idps/{idp_id}
```

Modify an existing Identity Provider configuration

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Identity Provider UUID |
| body | `body` | [PayloadUpdateIDPRequest](#payload-update-id-p-request) | `models.PayloadUpdateIDPRequest` | | ✓ | | Updated IDP configuration |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-200) | OK | IDP updated successfully |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-200-schema) |
| [400](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-400) | Bad Request | Invalid UUID format, request body, or validation failure |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-400-schema) |
| [401](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-401-schema) |
| [403](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-403) | Forbidden | Insufficient permissions |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-403-schema) |
| [404](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-404) | Not Found | IDP not found |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-404-schema) |
| [409](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-409) | Conflict | IDP name already exists |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-409-schema) |
| [429](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-429-schema) |
| [500](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-500) | Internal Server Error | Internal server error |  | [schema](#0198e7ea-3755-7a35-9e30-6a9392e8e7a1-500-schema) |

#### Responses


##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-200"></span> 200 - IDP updated successfully
Status: OK

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-400"></span> 400 - Invalid UUID format, request body, or validation failure
Status: Bad Request

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-404"></span> 404 - IDP not found
Status: Not Found

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-409"></span> 409 - IDP name already exists
Status: Conflict

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198e7ea-3755-7a35-9e30-6a9392e8e7a1-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02"></span> Create IDP (*0198e7ea-3755-7a39-9dfc-717d83facf02*)

```
POST /auth/idps
```

Register a new Identity Provider with authentication configuration

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadCreateIDPRequest](#payload-create-id-p-request) | `models.PayloadCreateIDPRequest` | | ✓ | | IDP configuration details |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#0198e7ea-3755-7a39-9dfc-717d83facf02-201) | Created | IDP created successfully | ✓ | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-201-schema) |
| [400](#0198e7ea-3755-7a39-9dfc-717d83facf02-400) | Bad Request | Invalid request body, validation failure, or malformed UUID |  | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-400-schema) |
| [401](#0198e7ea-3755-7a39-9dfc-717d83facf02-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-401-schema) |
| [403](#0198e7ea-3755-7a39-9dfc-717d83facf02-403) | Forbidden | Insufficient permissions |  | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-403-schema) |
| [409](#0198e7ea-3755-7a39-9dfc-717d83facf02-409) | Conflict | IDP with this name or configuration already exists |  | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-409-schema) |
| [429](#0198e7ea-3755-7a39-9dfc-717d83facf02-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-429-schema) |
| [500](#0198e7ea-3755-7a39-9dfc-717d83facf02-500) | Internal Server Error | Internal server error |  | [schema](#0198e7ea-3755-7a39-9dfc-717d83facf02-500-schema) |

#### Responses


##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-201"></span> 201 - IDP created successfully
Status: Created

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

###### Response headers

| Name | Type | Go type | Separator | Default | Description |
|------|------|---------|-----------|---------|-------------|
| Location | string | `string` |  |  | URL of the newly created IDP resource (/auth/idps/{idp_id}) |

##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-400"></span> 400 - Invalid request body, validation failure, or malformed UUID
Status: Bad Request

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-409"></span> 409 - IDP with this name or configuration already exists
Status: Conflict

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198e7ea-3755-7a39-9dfc-717d83facf02-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48"></span> Get IDP (*0198e7ea-3755-7a3d-8baa-36126e5d1c48*)

```
GET /auth/idps/{idp_id}
```

Retrieve a specific Identity Provider configuration by its unique identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Identity Provider UUID |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-200) | OK | IDP configuration retrieved successfully |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-200-schema) |
| [400](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-400) | Bad Request | Invalid UUID format or malformed request |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-400-schema) |
| [401](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-401-schema) |
| [403](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-403) | Forbidden | Insufficient permissions |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-403-schema) |
| [404](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-404) | Not Found | IDP not found |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-404-schema) |
| [429](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-429-schema) |
| [500](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-500) | Internal Server Error | Internal server error |  | [schema](#0198e7ea-3755-7a3d-8baa-36126e5d1c48-500-schema) |

#### Responses


##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-200"></span> 200 - IDP configuration retrieved successfully
Status: OK

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-200-schema"></span> Schema
   
  

[PayloadIDPResponse](#payload-id-p-response)

##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-400"></span> 400 - Invalid UUID format or malformed request
Status: Bad Request

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-404"></span> 404 - IDP not found
Status: Not Found

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198e7ea-3755-7a3d-8baa-36126e5d1c48-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd"></span> List IDP types (*0198f1e2-14ff-7678-afbe-9a627b0eaabd*)

```
GET /auth/idp_types
```

Retrieve a paginated list of all Identity Provider Types available for authentication.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated list of fields to return |
| filter | `query` | string | `string` |  |  |  | Filter expression for querying results |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items to return (default: 20, max: 100) |
| next_token | `query` | string | `string` |  |  |  | Pagination cursor for next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination cursor for previous page |
| sort | `query` | string | `string` |  |  |  | Sort by fields (comma-separated with ASC/DESC) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-200) | OK | Paginated list of IDP types retrieved successfully |  | [schema](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-200-schema) |
| [400](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-400) | Bad Request | Invalid request - malformed query parameters |  | [schema](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-400-schema) |
| [401](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-401-schema) |
| [403](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-403) | Forbidden | Insufficient permissions |  | [schema](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-403-schema) |
| [429](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-429-schema) |
| [500](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-500) | Internal Server Error | Internal server error |  | [schema](#0198f1e2-14ff-7678-afbe-9a627b0eaabd-500-schema) |

#### Responses


##### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-200"></span> 200 - Paginated list of IDP types retrieved successfully
Status: OK

###### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-200-schema"></span> Schema
   
  

[PayloadListIDPTypesResponse](#payload-list-id-p-types-response)

##### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-400"></span> 400 - Invalid request - malformed query parameters
Status: Bad Request

###### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198f1e2-14ff-7678-afbe-9a627b0eaabd-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484"></span> Get IDP type (*0198f1e2-14ff-767c-971c-3904e0f2c484*)

```
GET /auth/idp_types/{idp_type_id}
```

Retrieve a specific Identity Provider Type by its unique identifier

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| idp_type_id | `path` | string | `string` |  | ✓ |  | IDP type unique identifier (UUID v7) |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198f1e2-14ff-767c-971c-3904e0f2c484-200) | OK | IDP type details retrieved successfully |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-200-schema) |
| [400](#0198f1e2-14ff-767c-971c-3904e0f2c484-400) | Bad Request | Invalid request - malformed UUID or invalid parameters |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-400-schema) |
| [401](#0198f1e2-14ff-767c-971c-3904e0f2c484-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-401-schema) |
| [403](#0198f1e2-14ff-767c-971c-3904e0f2c484-403) | Forbidden | Insufficient permissions |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-403-schema) |
| [404](#0198f1e2-14ff-767c-971c-3904e0f2c484-404) | Not Found | IDP type not found |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-404-schema) |
| [429](#0198f1e2-14ff-767c-971c-3904e0f2c484-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-429-schema) |
| [500](#0198f1e2-14ff-767c-971c-3904e0f2c484-500) | Internal Server Error | Internal server error |  | [schema](#0198f1e2-14ff-767c-971c-3904e0f2c484-500-schema) |

#### Responses


##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-200"></span> 200 - IDP type details retrieved successfully
Status: OK

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-200-schema"></span> Schema
   
  

[PayloadIDPTypesResponse](#payload-id-p-types-response)

##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-400"></span> 400 - Invalid request - malformed UUID or invalid parameters
Status: Bad Request

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-404"></span> 404 - IDP type not found
Status: Not Found

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0198f1e2-14ff-767c-971c-3904e0f2c484-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10"></span> List identity providers (*0198fb33-7333-76f9-bcb4-1af086de3e10*)

```
GET /auth/idp/available
```

Retrieve all identity providers configured and available for user authentication and registration.

#### Consumes
  * application/json

#### Produces
  * application/json

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0198fb33-7333-76f9-bcb4-1af086de3e10-200) | OK | List of available Identity Providers retrieved successfully |  | [schema](#0198fb33-7333-76f9-bcb4-1af086de3e10-200-schema) |
| [400](#0198fb33-7333-76f9-bcb4-1af086de3e10-400) | Bad Request | Malformed request |  | [schema](#0198fb33-7333-76f9-bcb4-1af086de3e10-400-schema) |
| [500](#0198fb33-7333-76f9-bcb4-1af086de3e10-500) | Internal Server Error | Internal server error retrieving IDPs |  | [schema](#0198fb33-7333-76f9-bcb4-1af086de3e10-500-schema) |

#### Responses


##### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10-200"></span> 200 - List of available Identity Providers retrieved successfully
Status: OK

###### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10-200-schema"></span> Schema
   
  

[PayloadListIDPAvailableResponse](#payload-list-id-p-available-response)

##### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10-400"></span> 400 - Malformed request
Status: Bad Request

###### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10-500"></span> 500 - Internal server error retrieving IDPs
Status: Internal Server Error

###### <span id="0198fb33-7333-76f9-bcb4-1af086de3e10-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01991917-2720-7589-971b-cce23bf8a74b"></span> Initiate password recovery (*01991917-2720-7589-971b-cce23bf8a74b*)

```
POST /auth/password/recover
```

Request a password reset email with secure token.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadRecoverPasswordRequest](#payload-recover-password-request) | `models.PayloadRecoverPasswordRequest` | | ✓ | | Email address for password recovery |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01991917-2720-7589-971b-cce23bf8a74b-200) | OK | Accepted. Answered the same way whether or not an account exists, is disabled, or authenticates through an identity provider — deliberately, so this endpoint cannot be used to discover which addresses have accounts |  | [schema](#01991917-2720-7589-971b-cce23bf8a74b-200-schema) |
| [400](#01991917-2720-7589-971b-cce23bf8a74b-400) | Bad Request | Invalid request body or email format |  | [schema](#01991917-2720-7589-971b-cce23bf8a74b-400-schema) |
| [429](#01991917-2720-7589-971b-cce23bf8a74b-429) | Too Many Requests | This address has been asked about too often; Retry-After says when. Keyed on the submitted address, so an address with no account is throttled exactly like a real one |  | [schema](#01991917-2720-7589-971b-cce23bf8a74b-429-schema) |
| [500](#01991917-2720-7589-971b-cce23bf8a74b-500) | Internal Server Error | Internal server error during password recovery |  | [schema](#01991917-2720-7589-971b-cce23bf8a74b-500-schema) |

#### Responses


##### <span id="01991917-2720-7589-971b-cce23bf8a74b-200"></span> 200 - Accepted. Answered the same way whether or not an account exists, is disabled, or authenticates through an identity provider — deliberately, so this endpoint cannot be used to discover which addresses have accounts
Status: OK

###### <span id="01991917-2720-7589-971b-cce23bf8a74b-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-7589-971b-cce23bf8a74b-400"></span> 400 - Invalid request body or email format
Status: Bad Request

###### <span id="01991917-2720-7589-971b-cce23bf8a74b-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-7589-971b-cce23bf8a74b-429"></span> 429 - This address has been asked about too often; Retry-After says when. Keyed on the submitted address, so an address with no account is throttled exactly like a real one
Status: Too Many Requests

###### <span id="01991917-2720-7589-971b-cce23bf8a74b-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-7589-971b-cce23bf8a74b-500"></span> 500 - Internal server error during password recovery
Status: Internal Server Error

###### <span id="01991917-2720-7589-971b-cce23bf8a74b-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01991917-2720-758d-8104-94a0368acecb"></span> Reset password (*01991917-2720-758d-8104-94a0368acecb*)

```
POST /auth/password/reset
```

Set new password using the token from password recovery email.

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * ResetPasswordToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadResetPasswordRequest](#payload-reset-password-request) | `models.PayloadResetPasswordRequest` | | ✓ | | New password to set |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01991917-2720-758d-8104-94a0368acecb-200) | OK | Password reset successful |  | [schema](#01991917-2720-758d-8104-94a0368acecb-200-schema) |
| [400](#01991917-2720-758d-8104-94a0368acecb-400) | Bad Request | Invalid request body or password validation error |  | [schema](#01991917-2720-758d-8104-94a0368acecb-400-schema) |
| [401](#01991917-2720-758d-8104-94a0368acecb-401) | Unauthorized | Invalid, expired, or already used reset token |  | [schema](#01991917-2720-758d-8104-94a0368acecb-401-schema) |
| [403](#01991917-2720-758d-8104-94a0368acecb-403) | Forbidden | Insufficient permissions |  | [schema](#01991917-2720-758d-8104-94a0368acecb-403-schema) |
| [429](#01991917-2720-758d-8104-94a0368acecb-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01991917-2720-758d-8104-94a0368acecb-429-schema) |
| [500](#01991917-2720-758d-8104-94a0368acecb-500) | Internal Server Error | Internal server error during password reset |  | [schema](#01991917-2720-758d-8104-94a0368acecb-500-schema) |

#### Responses


##### <span id="01991917-2720-758d-8104-94a0368acecb-200"></span> 200 - Password reset successful
Status: OK

###### <span id="01991917-2720-758d-8104-94a0368acecb-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-758d-8104-94a0368acecb-400"></span> 400 - Invalid request body or password validation error
Status: Bad Request

###### <span id="01991917-2720-758d-8104-94a0368acecb-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-758d-8104-94a0368acecb-401"></span> 401 - Invalid, expired, or already used reset token
Status: Unauthorized

###### <span id="01991917-2720-758d-8104-94a0368acecb-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-758d-8104-94a0368acecb-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01991917-2720-758d-8104-94a0368acecb-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-758d-8104-94a0368acecb-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01991917-2720-758d-8104-94a0368acecb-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01991917-2720-758d-8104-94a0368acecb-500"></span> 500 - Internal server error during password reset
Status: Internal Server Error

###### <span id="01991917-2720-758d-8104-94a0368acecb-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01994754-5db8-7904-80f3-91417f2a4003"></span> List resource limits (*01994754-5db8-7904-80f3-91417f2a4003*)

```
GET /resources_limits
```

Retrieve a paginated list of resource limits in the system with optional filtering, sorting, and field selection

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated list of fields to return |
| filter | `query` | string | `string` |  |  |  | Filter expression for querying resources limits |
| limit | `query` | integer | `int64` |  |  |  | Maximum number of items to return |
| next_token | `query` | string | `string` |  |  |  | Pagination token for next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination token for previous page |
| sort | `query` | string | `string` |  |  |  | Comma-separated list of fields to sort by |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01994754-5db8-7904-80f3-91417f2a4003-200) | OK | Resources limits list retrieved successfully |  | [schema](#01994754-5db8-7904-80f3-91417f2a4003-200-schema) |
| [400](#01994754-5db8-7904-80f3-91417f2a4003-400) | Bad Request | Invalid request - malformed parameters or invalid filter syntax |  | [schema](#01994754-5db8-7904-80f3-91417f2a4003-400-schema) |
| [401](#01994754-5db8-7904-80f3-91417f2a4003-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#01994754-5db8-7904-80f3-91417f2a4003-401-schema) |
| [403](#01994754-5db8-7904-80f3-91417f2a4003-403) | Forbidden | Insufficient permissions |  | [schema](#01994754-5db8-7904-80f3-91417f2a4003-403-schema) |
| [429](#01994754-5db8-7904-80f3-91417f2a4003-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01994754-5db8-7904-80f3-91417f2a4003-429-schema) |
| [500](#01994754-5db8-7904-80f3-91417f2a4003-500) | Internal Server Error | Internal server error |  | [schema](#01994754-5db8-7904-80f3-91417f2a4003-500-schema) |

#### Responses


##### <span id="01994754-5db8-7904-80f3-91417f2a4003-200"></span> 200 - Resources limits list retrieved successfully
Status: OK

###### <span id="01994754-5db8-7904-80f3-91417f2a4003-200-schema"></span> Schema
   
  

[PayloadListResourcesLimitsResponse](#payload-list-resources-limits-response)

##### <span id="01994754-5db8-7904-80f3-91417f2a4003-400"></span> 400 - Invalid request - malformed parameters or invalid filter syntax
Status: Bad Request

###### <span id="01994754-5db8-7904-80f3-91417f2a4003-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01994754-5db8-7904-80f3-91417f2a4003-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="01994754-5db8-7904-80f3-91417f2a4003-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01994754-5db8-7904-80f3-91417f2a4003-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01994754-5db8-7904-80f3-91417f2a4003-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01994754-5db8-7904-80f3-91417f2a4003-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01994754-5db8-7904-80f3-91417f2a4003-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01994754-5db8-7904-80f3-91417f2a4003-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01994754-5db8-7904-80f3-91417f2a4003-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f"></span> Get authenticated user (*0199489b-f2f0-718a-a0cb-de8752ea864f*)

```
GET /me
```

Retrieve the profile information for the currently authenticated user

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0199489b-f2f0-718a-a0cb-de8752ea864f-200) | OK | User information retrieved successfully |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-200-schema) |
| [400](#0199489b-f2f0-718a-a0cb-de8752ea864f-400) | Bad Request | Invalid request format or parameters |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-400-schema) |
| [401](#0199489b-f2f0-718a-a0cb-de8752ea864f-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-401-schema) |
| [403](#0199489b-f2f0-718a-a0cb-de8752ea864f-403) | Forbidden | Insufficient permissions |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-403-schema) |
| [404](#0199489b-f2f0-718a-a0cb-de8752ea864f-404) | Not Found | User not found |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-404-schema) |
| [429](#0199489b-f2f0-718a-a0cb-de8752ea864f-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-429-schema) |
| [500](#0199489b-f2f0-718a-a0cb-de8752ea864f-500) | Internal Server Error | Internal server error |  | [schema](#0199489b-f2f0-718a-a0cb-de8752ea864f-500-schema) |

#### Responses


##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-200"></span> 200 - User information retrieved successfully
Status: OK

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-200-schema"></span> Schema
   
  

[PayloadUserResponse](#payload-user-response)

##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-400"></span> 400 - Invalid request format or parameters
Status: Bad Request

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-404"></span> 404 - User not found
Status: Not Found

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0199489b-f2f0-718a-a0cb-de8752ea864f-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0199489b-f2f0-718e-a94d-b05a296eb818"></span> Update authenticated user (*0199489b-f2f0-718e-a94d-b05a296eb818*)

```
PUT /me
```

Update the profile information for the currently authenticated user

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadUpdateMeRequest](#payload-update-me-request) | `models.PayloadUpdateMeRequest` | | ✓ | | User update request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0199489b-f2f0-718e-a94d-b05a296eb818-200) | OK | User updated successfully |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-200-schema) |
| [400](#0199489b-f2f0-718e-a94d-b05a296eb818-400) | Bad Request | Invalid request format or validation failed |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-400-schema) |
| [401](#0199489b-f2f0-718e-a94d-b05a296eb818-401) | Unauthorized | Missing or invalid authentication token |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-401-schema) |
| [403](#0199489b-f2f0-718e-a94d-b05a296eb818-403) | Forbidden | Insufficient permissions |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-403-schema) |
| [404](#0199489b-f2f0-718e-a94d-b05a296eb818-404) | Not Found | User not found |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-404-schema) |
| [409](#0199489b-f2f0-718e-a94d-b05a296eb818-409) | Conflict | User already exists (duplicate email) |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-409-schema) |
| [429](#0199489b-f2f0-718e-a94d-b05a296eb818-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-429-schema) |
| [500](#0199489b-f2f0-718e-a94d-b05a296eb818-500) | Internal Server Error | Internal server error |  | [schema](#0199489b-f2f0-718e-a94d-b05a296eb818-500-schema) |

#### Responses


##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-200"></span> 200 - User updated successfully
Status: OK

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-400"></span> 400 - Invalid request format or validation failed
Status: Bad Request

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-401"></span> 401 - Missing or invalid authentication token
Status: Unauthorized

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-404"></span> 404 - User not found
Status: Not Found

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-409"></span> 409 - User already exists (duplicate email)
Status: Conflict

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0199489b-f2f0-718e-a94d-b05a296eb818-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a"></span> Get authorization info (*0199489b-f2f0-719e-b860-3b7ea6a86a1a*)

```
GET /me/authz
```

Retrieve the authorization details and permissions for the currently authenticated user

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-200) | OK | Authorization information retrieved successfully |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-200-schema) |
| [400](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-400) | Bad Request | Invalid request format or parameters |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-400-schema) |
| [401](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-401) | Unauthorized | Unauthorized - invalid or missing authentication |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-401-schema) |
| [403](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-403) | Forbidden | Insufficient permissions |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-403-schema) |
| [404](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-404) | Not Found | User not found |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-404-schema) |
| [429](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-429-schema) |
| [500](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-500) | Internal Server Error | Internal server error |  | [schema](#0199489b-f2f0-719e-b860-3b7ea6a86a1a-500-schema) |

#### Responses


##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-200"></span> 200 - Authorization information retrieved successfully
Status: OK

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-200-schema"></span> Schema
   
  

[PayloadGetAuthenticatedUserResponse](#payload-get-authenticated-user-response)

##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-400"></span> 400 - Invalid request format or parameters
Status: Bad Request

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-401"></span> 401 - Unauthorized - invalid or missing authentication
Status: Unauthorized

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-404"></span> 404 - User not found
Status: Not Found

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="0199489b-f2f0-719e-b860-3b7ea6a86a1a-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa"></span> Get my resource limits (*01a01117-dba9-74da-bd70-d1acc3842ffa*)

```
GET /me/resources_limits
```

Retrieve the limits that apply to the calling user and how much of each has been consumed. Read-only: limits are not editable through the API.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a01117-dba9-74da-bd70-d1acc3842ffa-200) | OK | Limits and usage for the calling user |  | [schema](#01a01117-dba9-74da-bd70-d1acc3842ffa-200-schema) |
| [400](#01a01117-dba9-74da-bd70-d1acc3842ffa-400) | Bad Request | Missing or malformed user context |  | [schema](#01a01117-dba9-74da-bd70-d1acc3842ffa-400-schema) |
| [401](#01a01117-dba9-74da-bd70-d1acc3842ffa-401) | Unauthorized | Missing or invalid authentication |  | [schema](#01a01117-dba9-74da-bd70-d1acc3842ffa-401-schema) |
| [403](#01a01117-dba9-74da-bd70-d1acc3842ffa-403) | Forbidden | Insufficient permissions |  | [schema](#01a01117-dba9-74da-bd70-d1acc3842ffa-403-schema) |
| [429](#01a01117-dba9-74da-bd70-d1acc3842ffa-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01a01117-dba9-74da-bd70-d1acc3842ffa-429-schema) |
| [500](#01a01117-dba9-74da-bd70-d1acc3842ffa-500) | Internal Server Error | Internal server error |  | [schema](#01a01117-dba9-74da-bd70-d1acc3842ffa-500-schema) |

#### Responses


##### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-200"></span> 200 - Limits and usage for the calling user
Status: OK

###### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-200-schema"></span> Schema
   
  

[PayloadResourcesLimitsStatusResponse](#payload-resources-limits-status-response)

##### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-400"></span> 400 - Missing or malformed user context
Status: Bad Request

###### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-401"></span> 401 - Missing or invalid authentication
Status: Unauthorized

###### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a01117-dba9-74da-bd70-d1acc3842ffa-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a01117-dba9-763d-8f4e-968072dbdb52"></span> Get project resource limits (*01a01117-dba9-763d-8f4e-968072dbdb52*)

```
GET /projects/{project_id}/resources_limits
```

Retrieve the limits that apply to a project and how much of each has been consumed. Read-only: limits are not editable through the API.

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| project_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Project ID |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a01117-dba9-763d-8f4e-968072dbdb52-200) | OK | Limits and usage for the project |  | [schema](#01a01117-dba9-763d-8f4e-968072dbdb52-200-schema) |
| [400](#01a01117-dba9-763d-8f4e-968072dbdb52-400) | Bad Request | Invalid project ID format |  | [schema](#01a01117-dba9-763d-8f4e-968072dbdb52-400-schema) |
| [401](#01a01117-dba9-763d-8f4e-968072dbdb52-401) | Unauthorized | Missing or invalid authentication |  | [schema](#01a01117-dba9-763d-8f4e-968072dbdb52-401-schema) |
| [403](#01a01117-dba9-763d-8f4e-968072dbdb52-403) | Forbidden | Insufficient permissions |  | [schema](#01a01117-dba9-763d-8f4e-968072dbdb52-403-schema) |
| [429](#01a01117-dba9-763d-8f4e-968072dbdb52-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01a01117-dba9-763d-8f4e-968072dbdb52-429-schema) |
| [500](#01a01117-dba9-763d-8f4e-968072dbdb52-500) | Internal Server Error | Internal server error |  | [schema](#01a01117-dba9-763d-8f4e-968072dbdb52-500-schema) |

#### Responses


##### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-200"></span> 200 - Limits and usage for the project
Status: OK

###### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-200-schema"></span> Schema
   
  

[PayloadResourcesLimitsStatusResponse](#payload-resources-limits-status-response)

##### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-400"></span> 400 - Invalid project ID format
Status: Bad Request

###### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-401"></span> 401 - Missing or invalid authentication
Status: Unauthorized

###### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-403"></span> 403 - Insufficient permissions
Status: Forbidden

###### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a01117-dba9-763d-8f4e-968072dbdb52-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae"></span> Confirm email verification (*01a02dbb-bc41-7287-9cfd-7ac08bf882ae*)

```
POST /auth/verify/confirm
```

Activate a user account with the verification token from the email. The token travels in the Authorization header, never in the URL.

#### Produces
  * application/json

#### Security Requirements
  * VerificationToken

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-200) | OK | Email verified - account activated |  | [schema](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-200-schema) |
| [400](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-400) | Bad Request | Invalid or malformed token format |  | [schema](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-400-schema) |
| [401](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-401) | Unauthorized | Token missing, expired, or not an email verification token |  | [schema](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-401-schema) |
| [404](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-404) | Not Found | User not found |  | [schema](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-404-schema) |
| [429](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-429) | Too Many Requests | Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store |  | [schema](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-429-schema) |
| [500](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-500) | Internal Server Error | Internal server error during verification |  | [schema](#01a02dbb-bc41-7287-9cfd-7ac08bf882ae-500-schema) |

#### Responses


##### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-200"></span> 200 - Email verified - account activated
Status: OK

###### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-400"></span> 400 - Invalid or malformed token format
Status: Bad Request

###### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-401"></span> 401 - Token missing, expired, or not an email verification token
Status: Unauthorized

###### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-404"></span> 404 - User not found
Status: Not Found

###### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-429"></span> 429 - Too many requests -- RATE_LIMIT_EXCEEDED is the budget, RATE_LIMIT_UNAVAILABLE the limiter's own store
Status: Too Many Requests

###### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-500"></span> 500 - Internal server error during verification
Status: Internal Server Error

###### <span id="01a02dbb-bc41-7287-9cfd-7ac08bf882ae-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92"></span> Liveness probe (*01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92*)

```
GET /health/live
```

Answers 200 whenever this process can serve HTTP. It checks NOTHING else — no database, no cache — on purpose: a liveness probe decides whether to RESTART the process, and restarting cannot fix a dependency that is down. Point a readiness probe at /health/detailed instead, which does reflect dependencies.

#### Produces
  * application/json

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92-200) | OK | The process is alive and serving requests |  | [schema](#01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92-200-schema) |

#### Responses


##### <span id="01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92-200"></span> 200 - The process is alive and serving requests
Status: OK

###### <span id="01a02ec9-ac6c-77c2-81ad-d6e2f23bcd92-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334"></span> List rate limits (*01a03a46-16d4-7831-9c94-a7975a9c4334*)

```
GET /rate_limits
```

List the rate-limit rules, with filtering, sorting, partial fields and pagination

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| fields | `query` | string | `string` |  |  |  | Comma-separated fields to return |
| filter | `query` | string | `string` |  |  |  | Filter expression |
| limit | `query` | integer | `int64` |  |  |  | Maximum items per page |
| next_token | `query` | string | `string` |  |  |  | Pagination token for the next page |
| prev_token | `query` | string | `string` |  |  |  | Pagination token for the previous page |
| sort | `query` | string | `string` |  |  |  | Sort expression, for example 'name ASC' |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a03a46-16d4-7831-9c94-a7975a9c4334-200) | OK | Rate limits retrieved successfully |  | [schema](#01a03a46-16d4-7831-9c94-a7975a9c4334-200-schema) |
| [400](#01a03a46-16d4-7831-9c94-a7975a9c4334-400) | Bad Request | Invalid query parameters |  | [schema](#01a03a46-16d4-7831-9c94-a7975a9c4334-400-schema) |
| [401](#01a03a46-16d4-7831-9c94-a7975a9c4334-401) | Unauthorized | Invalid or expired token |  | [schema](#01a03a46-16d4-7831-9c94-a7975a9c4334-401-schema) |
| [403](#01a03a46-16d4-7831-9c94-a7975a9c4334-403) | Forbidden | Not authorized |  | [schema](#01a03a46-16d4-7831-9c94-a7975a9c4334-403-schema) |
| [429](#01a03a46-16d4-7831-9c94-a7975a9c4334-429) | Too Many Requests | Too many requests |  | [schema](#01a03a46-16d4-7831-9c94-a7975a9c4334-429-schema) |
| [500](#01a03a46-16d4-7831-9c94-a7975a9c4334-500) | Internal Server Error | Internal server error |  | [schema](#01a03a46-16d4-7831-9c94-a7975a9c4334-500-schema) |

#### Responses


##### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-200"></span> 200 - Rate limits retrieved successfully
Status: OK

###### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-200-schema"></span> Schema
   
  

[PayloadListRateLimitsResponse](#payload-list-rate-limits-response)

##### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-400"></span> 400 - Invalid query parameters
Status: Bad Request

###### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-403"></span> 403 - Not authorized
Status: Forbidden

###### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a03a46-16d4-7831-9c94-a7975a9c4334-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c"></span> Create rate limit (*01a03a46-16d4-7ad9-b646-0bc67824b38c*)

```
POST /rate_limits
```

Create a rate-limit rule. The target is validated against the endpoint catalogue, so a rule for a route this service does not serve is refused rather than silently protecting nothing

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| body | `body` | [PayloadCreateRateLimitRequest](#payload-create-rate-limit-request) | `models.PayloadCreateRateLimitRequest` | | ✓ | | Rate limit creation request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [201](#01a03a46-16d4-7ad9-b646-0bc67824b38c-201) | Created | Rate limit created successfully |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-201-schema) |
| [400](#01a03a46-16d4-7ad9-b646-0bc67824b38c-400) | Bad Request | Invalid request body, unknown strategy, or a target no route matches |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-400-schema) |
| [401](#01a03a46-16d4-7ad9-b646-0bc67824b38c-401) | Unauthorized | Invalid or expired token |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-401-schema) |
| [403](#01a03a46-16d4-7ad9-b646-0bc67824b38c-403) | Forbidden | Not authorized |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-403-schema) |
| [409](#01a03a46-16d4-7ad9-b646-0bc67824b38c-409) | Conflict | A rate limit with that name already exists |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-409-schema) |
| [429](#01a03a46-16d4-7ad9-b646-0bc67824b38c-429) | Too Many Requests | Too many requests |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-429-schema) |
| [500](#01a03a46-16d4-7ad9-b646-0bc67824b38c-500) | Internal Server Error | Internal server error |  | [schema](#01a03a46-16d4-7ad9-b646-0bc67824b38c-500-schema) |

#### Responses


##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-201"></span> 201 - Rate limit created successfully
Status: Created

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-201-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-400"></span> 400 - Invalid request body, unknown strategy, or a target no route matches
Status: Bad Request

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-403"></span> 403 - Not authorized
Status: Forbidden

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-409"></span> 409 - A rate limit with that name already exists
Status: Conflict

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a03a46-16d4-7ad9-b646-0bc67824b38c-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80"></span> Get rate limit (*01a03a46-16d4-7af9-9f96-d9dc094afd80*)

```
GET /rate_limits/{rate_limit_id}
```

Retrieve a rate-limit rule and its windows by unique identifier

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| rate_limit_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique rate limit identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a03a46-16d4-7af9-9f96-d9dc094afd80-200) | OK | Rate limit retrieved successfully |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-200-schema) |
| [400](#01a03a46-16d4-7af9-9f96-d9dc094afd80-400) | Bad Request | Invalid rate limit ID format |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-400-schema) |
| [401](#01a03a46-16d4-7af9-9f96-d9dc094afd80-401) | Unauthorized | Invalid or expired token |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-401-schema) |
| [403](#01a03a46-16d4-7af9-9f96-d9dc094afd80-403) | Forbidden | Not authorized |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-403-schema) |
| [404](#01a03a46-16d4-7af9-9f96-d9dc094afd80-404) | Not Found | Rate limit not found |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-404-schema) |
| [429](#01a03a46-16d4-7af9-9f96-d9dc094afd80-429) | Too Many Requests | Too many requests |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-429-schema) |
| [500](#01a03a46-16d4-7af9-9f96-d9dc094afd80-500) | Internal Server Error | Internal server error |  | [schema](#01a03a46-16d4-7af9-9f96-d9dc094afd80-500-schema) |

#### Responses


##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-200"></span> 200 - Rate limit retrieved successfully
Status: OK

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-200-schema"></span> Schema
   
  

[PayloadRateLimitResponse](#payload-rate-limit-response)

##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-400"></span> 400 - Invalid rate limit ID format
Status: Bad Request

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-403"></span> 403 - Not authorized
Status: Forbidden

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-404"></span> 404 - Rate limit not found
Status: Not Found

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a03a46-16d4-7af9-9f96-d9dc094afd80-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3"></span> Update rate limit (*01a03a46-16d4-7b0a-af95-6805d68a37d3*)

```
PUT /rate_limits/{rate_limit_id}
```

Replace a rate-limit rule. The window set is replaced in full, not merged

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| rate_limit_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique rate limit identifier |
| body | `body` | [PayloadUpdateRateLimitRequest](#payload-update-rate-limit-request) | `models.PayloadUpdateRateLimitRequest` | | ✓ | | Rate limit update request payload |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a03a46-16d4-7b0a-af95-6805d68a37d3-200) | OK | Rate limit updated successfully |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-200-schema) |
| [400](#01a03a46-16d4-7b0a-af95-6805d68a37d3-400) | Bad Request | Invalid request body, unknown strategy, or a target no route matches |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-400-schema) |
| [401](#01a03a46-16d4-7b0a-af95-6805d68a37d3-401) | Unauthorized | Invalid or expired token |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-401-schema) |
| [403](#01a03a46-16d4-7b0a-af95-6805d68a37d3-403) | Forbidden | Not authorized, or the rule is system-managed |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-403-schema) |
| [404](#01a03a46-16d4-7b0a-af95-6805d68a37d3-404) | Not Found | Rate limit not found |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-404-schema) |
| [409](#01a03a46-16d4-7b0a-af95-6805d68a37d3-409) | Conflict | A rate limit with that name already exists |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-409-schema) |
| [429](#01a03a46-16d4-7b0a-af95-6805d68a37d3-429) | Too Many Requests | Too many requests |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-429-schema) |
| [500](#01a03a46-16d4-7b0a-af95-6805d68a37d3-500) | Internal Server Error | Internal server error |  | [schema](#01a03a46-16d4-7b0a-af95-6805d68a37d3-500-schema) |

#### Responses


##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-200"></span> 200 - Rate limit updated successfully
Status: OK

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-400"></span> 400 - Invalid request body, unknown strategy, or a target no route matches
Status: Bad Request

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-403"></span> 403 - Not authorized, or the rule is system-managed
Status: Forbidden

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-404"></span> 404 - Rate limit not found
Status: Not Found

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-409"></span> 409 - A rate limit with that name already exists
Status: Conflict

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-409-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a03a46-16d4-7b0a-af95-6805d68a37d3-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7"></span> Delete rate limit (*01a03a46-16d4-7b1a-913f-e9e50f9acfa7*)

```
DELETE /rate_limits/{rate_limit_id}
```

Delete a rate-limit rule. Its windows are removed with it

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| rate_limit_id | `path` | uuid (formatted string) | `strfmt.UUID` |  | ✓ |  | Unique rate limit identifier |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-200) | OK | Rate limit deleted successfully |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-200-schema) |
| [400](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-400) | Bad Request | Invalid rate limit ID format |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-400-schema) |
| [401](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-401) | Unauthorized | Invalid or expired token |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-401-schema) |
| [403](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-403) | Forbidden | Not authorized, or the rule is system-managed |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-403-schema) |
| [404](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-404) | Not Found | Rate limit not found |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-404-schema) |
| [429](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-429) | Too Many Requests | Too many requests |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-429-schema) |
| [500](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-500) | Internal Server Error | Internal server error |  | [schema](#01a03a46-16d4-7b1a-913f-e9e50f9acfa7-500-schema) |

#### Responses


##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-200"></span> 200 - Rate limit deleted successfully
Status: OK

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-200-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-400"></span> 400 - Invalid rate limit ID format
Status: Bad Request

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-403"></span> 403 - Not authorized, or the rule is system-managed
Status: Forbidden

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-404"></span> 404 - Rate limit not found
Status: Not Found

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-404-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a03a46-16d4-7b1a-913f-e9e50f9acfa7-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940"></span> Effective rate limits (*01a03a46-16d4-7b2b-8932-ef9694d8f940*)

```
GET /rate_limits/effective
```

Resolve which rules apply to a method and endpoint, one per scope, most specific first. Resolved with the same function the limiter uses, so it cannot disagree with what is enforced

#### Consumes
  * application/json

#### Produces
  * application/json

#### Security Requirements
  * AccessToken

#### Parameters

| Name | Source | Type | Go type | Separator | Required | Default | Description |
|------|--------|------|---------|-----------| :------: |---------|-------------|
| authenticated | `query` | boolean | `bool` |  |  |  | Whether to resolve as an authenticated caller. Defaults to true |
| endpoint | `query` | string | `string` |  | ✓ |  | Route template, for example /projects/{project_id}/generate |
| method | `query` | string | `string` |  | ✓ |  | HTTP method, uppercase |

#### All responses
| Code | Status | Description | Has headers | Schema |
|------|--------|-------------|:-----------:|--------|
| [200](#01a03a46-16d4-7b2b-8932-ef9694d8f940-200) | OK | Effective rules resolved |  | [schema](#01a03a46-16d4-7b2b-8932-ef9694d8f940-200-schema) |
| [400](#01a03a46-16d4-7b2b-8932-ef9694d8f940-400) | Bad Request | method or endpoint missing or invalid |  | [schema](#01a03a46-16d4-7b2b-8932-ef9694d8f940-400-schema) |
| [401](#01a03a46-16d4-7b2b-8932-ef9694d8f940-401) | Unauthorized | Invalid or expired token |  | [schema](#01a03a46-16d4-7b2b-8932-ef9694d8f940-401-schema) |
| [403](#01a03a46-16d4-7b2b-8932-ef9694d8f940-403) | Forbidden | Not authorized |  | [schema](#01a03a46-16d4-7b2b-8932-ef9694d8f940-403-schema) |
| [429](#01a03a46-16d4-7b2b-8932-ef9694d8f940-429) | Too Many Requests | Too many requests |  | [schema](#01a03a46-16d4-7b2b-8932-ef9694d8f940-429-schema) |
| [500](#01a03a46-16d4-7b2b-8932-ef9694d8f940-500) | Internal Server Error | Internal server error |  | [schema](#01a03a46-16d4-7b2b-8932-ef9694d8f940-500-schema) |

#### Responses


##### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-200"></span> 200 - Effective rules resolved
Status: OK

###### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-200-schema"></span> Schema
   
  

[PayloadEffectiveRateLimitsResponse](#payload-effective-rate-limits-response)

##### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-400"></span> 400 - method or endpoint missing or invalid
Status: Bad Request

###### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-400-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-401"></span> 401 - Invalid or expired token
Status: Unauthorized

###### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-401-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-403"></span> 403 - Not authorized
Status: Forbidden

###### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-403-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-429"></span> 429 - Too many requests
Status: Too Many Requests

###### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-429-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

##### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-500"></span> 500 - Internal server error
Status: Internal Server Error

###### <span id="01a03a46-16d4-7b2b-8932-ef9694d8f940-500-schema"></span> Schema
   
  

[PayloadHTTPMessage](#payload-http-message)

## Models

### <span id="domain-check"></span> domain.Check


> One component's health verdict. Status only: this endpoint is public
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| kind | string (formatted string)| `string` |  | |  | `database` |
| name | string (formatted string)| `string` |  | |  | `database` |
| status | string (formatted boolean)| `string` |  | | Health status of a service. | `true` |



### <span id="domain-component-health"></span> domain.ComponentHealth


> Health status of an individual component.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| details | map of any | `map[string]any` |  | |  |  |
| message | string (formatted string)| `string` |  | |  | `Database is reachable` |
| response_time | string (formatted string)| `string` |  | |  | `2.5ms` |
| status | string (formatted string)| `string` |  | |  | `healthy` |



### <span id="domain-database-pool-stats"></span> domain.DatabasePoolStats


> Database connection pool statistics.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| acquire_count | integer| `int64` |  | |  | `150` |
| acquire_duration_ns | integer| `int64` |  | |  | `5000000` |
| acquired_connections | integer| `int64` |  | |  | `5` |
| canceled_acquire_count | integer| `int64` |  | |  | `2` |
| constructing_connections | integer| `int64` |  | |  | `0` |
| empty_acquire_count | integer| `int64` |  | |  | `100` |
| idle_connections | integer| `int64` |  | |  | `10` |
| max_connections | integer| `int64` |  | |  | `20` |
| total_connections | integer| `int64` |  | |  | `15` |



### <span id="domain-paginator"></span> domain.Paginator


> Paginator represents a paginator.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| limit | int (formatted integer)| `int64` |  | | Maximum number of items per page | `10` |
| next_page | string (formatted string)| `string` |  | | URL for the next page | `http://localhost:8080/users?next_token=ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=\u0026limit=10` |
| next_token | string (formatted string)| `string` |  | | Base64 encoded next page token | `ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=` |
| prev_page | string (formatted string)| `string` |  | | URL for the previous page | `http://localhost:8080/users?prev_token=ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=\u0026limit=10` |
| prev_token | string (formatted string)| `string` |  | | Base64 encoded previous page token | `ZmZmZmZmZmYtZmZmZi0tZmZmZmZmZmY=` |
| size | int (formatted integer)| `int64` |  | | Number of items in the current page | `10` |



### <span id="domain-project"></span> domain.Project


> Project represents a project.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the project was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Detailed description of the project's purpose | `This is my main project` |
| disabled | boolean| `bool` |  | | Disabled indicates if the project is disabled.</br>Pointer to distinguish between false and unset values in partial responses. | `false` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the project | `019b4b0d-a682-7e34-a20c-c71a7147d7e7` |
| name | string (formatted string)| `string` |  | | Project display name | `My Project` |
| system | boolean| `bool` |  | | Indicates if this is a system-managed project (cannot be deleted) | `false` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the project was last updated | `2021-01-01T00:00:00Z` |



### <span id="domain-startup-metrics"></span> domain.StartupMetrics


> Application startup timing metrics.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| phase_percentage | map of any | `map[string]any` |  | |  |  |
| phase_timings | map of any | `map[string]any` |  | |  |  |
| total_time | string (formatted string)| `string` |  | |  | `1.5s` |



### <span id="domain-token-type"></span> domain.TokenType


  

| Name | Type | Go type | Default | Description | Example |
|------|------|---------| ------- |-------------|---------|
| domain.TokenType | string| string | |  |  |



### <span id="payload-authz-paths"></span> payload.AuthzPaths


> API path to the methods granted on it. A key of "*" grants every path
  



[PayloadAuthzPaths](#payload-authz-paths)

### <span id="payload-authz-permissions"></span> payload.AuthzPermissions


> Permission set by category; today the only category is "users"
  



[PayloadAuthzPermissions](#payload-authz-permissions)

### <span id="payload-authz-subjects"></span> payload.AuthzSubjects


> Subject id to the paths granted to it
  



[PayloadAuthzSubjects](#payload-authz-subjects)

### <span id="payload-create-id-p-request"></span> payload.CreateIDPRequest


> Request payload for creating a new identity provider configuration
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| callback_url | uri (formatted string)| `strfmt.URI` | ✓ | | OAuth callback URL | `http://localhost:8080/api/v1/auth/idp/019b4b0d-a682-7e19-a524-866cfffef121/callback` |
| client_id | string (formatted string)| `string` | ✓ | | OAuth client ID | `367082405970-example` |
| client_secret | string (formatted string)| `string` | ✓ | | OAuth client secret | `GOCSPX-example_secret_key` |
| description | string (formatted string)| `string` | ✓ | | Description | `Google OAuth2 Identity Provider` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom ID | `019b4b0d-a682-7e19-a524-866cfffef121` |
| idp_type_id | uuid (formatted string)| `strfmt.UUID` | ✓ | | Identity provider type ID | `019b4b0d-a682-7e1d-bd83-3864c7d5aa43` |
| login_redirect_url | uri (formatted string)| `strfmt.URI` | ✓ | | Login redirect URL | `https://example.com/login` |
| logo | uri (formatted string)| `strfmt.URI` |  | | Logo URL | `https://example.com/logo.png` |
| name | string (formatted string)| `string` | ✓ | | Display name | `Google` |
| register_redirect_url | uri (formatted string)| `strfmt.URI` | ✓ | | Registration redirect URL | `https://example.com/register` |



### <span id="payload-create-policy-request"></span> payload.CreatePolicyRequest


> Request payload for creating a new authorization policy
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| allowed_action | string (formatted string)| `string` | ✓ | | HTTP method allowed by this policy | `POST` |
| allowed_resource | string (formatted string)| `string` | ✓ | | Resource path pattern (supports wildcards and UUIDs) | `/projects/*/tokens` |
| description | string (formatted string)| `string` |  | | Detailed description of the policy's purpose and scope | `Allows managing API tokens for specific projects` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom policy ID (auto-generated if not provided) | `019b4b0d-a682-7e38-b235-3dfcb59f4d9e` |
| name | string (formatted string)| `string` | ✓ | | Policy display name | `Project Tokens Manager` |



### <span id="payload-create-product-request"></span> payload.CreateProductRequest


> Request payload for creating a new product within a project
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` | ✓ | | Detailed description of the product | `This is a product` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom product ID (auto-generated if not provided) | `01980434-b7ff-7abe-a45d-7311bc7011f5` |
| name | string (formatted string)| `string` | ✓ | | Product name, unique within its project | `Product Name` |



### <span id="payload-create-project-request"></span> payload.CreateProjectRequest


> Request payload for creating a new project workspace
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` | ✓ | | Detailed description of the project's purpose | `A workspace for team collaboration` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom project ID (auto-generated if not provided) | `019b4b0d-a682-7dea-9751-9b2bb20b0132` |
| name | string (formatted string)| `string` | ✓ | | Project display name | `My New Project` |



### <span id="payload-create-rate-limit-request"></span> payload.CreateRateLimitRequest


> Request payload for creating a rate-limit rule
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| audience | string (formatted string)| `string` |  | | Defaults to any | `auth` |
| description | string (formatted string)| `string` | ✓ | | What the rule is for | `One expensive call, seconds of wall clock, real money` |
| enabled | boolean| `bool` |  | | Defaults to true | `true` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom ID | `019b4b0d-a682-7e38-b235-3dfcb59f4d9e` |
| methods | []array (formatted string)| `[]string` | ✓ | | Verbs the rule covers. ["*"] means any verb and ONE shared budget across them | `["POST"]` |
| name | string (formatted string)| `string` | ✓ | | Rule display name | `Generate per project` |
| scope | string (formatted string)| `string` | ✓ | | What the bucket is keyed on | `project` |
| strategy | string (formatted string)| `string` |  | | Defaults to token_bucket | `leaky_bucket` |
| target | string (formatted string)| `string` | ✓ | | Route template as registered. A prefix must end with /; a global rule must use * | `/projects/{project_id}/generate` |
| target_kind | string (formatted string)| `string` | ✓ | | How target is matched | `endpoint` |
| windows | [][PayloadRateLimitWindowRequest](#payload-rate-limit-window-request)| `[]*PayloadRateLimitWindowRequest` | ✓ | | At least one. All apply; the shortest period is evaluated first |  |



### <span id="payload-create-role-request"></span> payload.CreateRoleRequest


> Request payload for creating a new role with permissions
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` | ✓ | | Description of the role's responsibilities and scope | `Content editor role` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom role ID | `019b4b0d-a682-7f0b-a592-7bc362bae397` |
| name | string (formatted string)| `string` | ✓ | | Role display name | `Editor` |



### <span id="payload-create-user-request"></span> payload.CreateUserRequest


> Request payload for creating a new user account
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| email | email (formatted string)| `strfmt.Email` | ✓ | | User's email address (must be unique) | `john.doe@example.com` |
| first_name | string (formatted string)| `string` | ✓ | | User's first name | `John` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom user ID | `019b4b0d-a682-7013-91b4-d452c93dfa47` |
| last_name | string (formatted string)| `string` | ✓ | | User's last name | `Doe` |
| password | password (formatted string)| `strfmt.Password` | ✓ | | User's password (minimum 8 characters) | `SecureP@ssw0rd123` |



### <span id="payload-detailed-health"></span> payload.DetailedHealth


> Comprehensive health information including component status and metrics.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| components | map of [DomainComponentHealth](#domain-component-health)| `map[string]DomainComponentHealth` |  | |  |  |
| database_pool | [DomainDatabasePoolStats](#domain-database-pool-stats)| `DomainDatabasePoolStats` |  | |  |  |
| startup_metrics | [DomainStartupMetrics](#domain-startup-metrics)| `DomainStartupMetrics` |  | |  |  |
| status | string (formatted string)| `string` |  | |  | `healthy` |
| uptime | string (formatted string)| `string` |  | |  | `2h30m45s` |
| version | string (formatted string)| `string` |  | |  | `1.0.0` |



### <span id="payload-effective-rate-limit-entry"></span> payload.EffectiveRateLimitEntry


> A rule that applies to a request, with the reason it won its scope
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| name | string (formatted string)| `string` |  | |  | `Generate per project` |
| rule_id | uuid (formatted string)| `strfmt.UUID` |  | |  | `019b4b0d-a682-7e34-a20c-c71a7147d7e7` |
| scope | string (formatted string)| `string` |  | | What this rule buckets on | `project` |
| strategy | string (formatted string)| `string` |  | |  | `leaky_bucket` |
| why | string (formatted string)| `string` |  | | Why is prose on purpose. The ladder is what operators get wrong, and a</br>tier number answers "which rung" when the question being asked is "why</br>not the other rule". | `exact endpoint and a named verb — the most specific match there is` |
| windows | [][PayloadRateLimitWindowResponse](#payload-rate-limit-window-response)| `[]*PayloadRateLimitWindowResponse` |  | |  |  |



### <span id="payload-effective-rate-limits-response"></span> payload.EffectiveRateLimitsResponse


> The rules that apply to a method and endpoint, one per scope, most specific first
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| bucket_key | string (formatted string)| `string` |  | | BucketKey states what a budget is keyed on, because "10 per minute" alone</br>does not say per what. A rule's verbs share one budget; each of its windows</br>gets its own. | `(rule_id, window_id, scope_key) — one budget per window, shared across the rule's verbs` |
| effective | [][PayloadEffectiveRateLimitEntry](#payload-effective-rate-limit-entry)| `[]*PayloadEffectiveRateLimitEntry` |  | | Effective carries ONE entry per scope. That is the shape of the design: an</br>IP rule and a project rule both apply, and neither substitutes for the</br>other. |  |
| endpoint | string (formatted string)| `string` |  | |  | `/projects/{project_id}/generate` |
| enforcing | boolean| `bool` |  | | Enforcing is false when ratelimit.enabled=false: the rules below are real</br>and editable, and nothing is applying them. A client that renders the</br>rules without rendering this tells an operator a limit is in place that</br>is not. | `true` |
| method | string (formatted string)| `string` |  | |  | `POST` |



### <span id="payload-get-authenticated-user-response"></span> payload.GetAuthenticatedUserResponse


> Response containing authenticated user details and their complete permission set
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| account | [PayloadGetAuthenticatedUserResponse](#payload-get-authenticated-user-response)| `PayloadGetAuthenticatedUserResponse` |  | | Authenticated user's account information |  |
| permissions | [PayloadGetAuthenticatedUserResponse](#payload-get-authenticated-user-response)| `PayloadGetAuthenticatedUserResponse` |  | | Complete set of permissions granted to the user through roles and policies |  |



### <span id="payload-http-message"></span> payload.HTTPMessage


> HTTPMessage represents a message to be sent to the client trough HTTP REST API.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| code | string (formatted string)| `string` |  | | Code is a stable, machine-readable reason, present only where a client</br>has to BRANCH on it. Message is prose and may be reworded; this is not.</br></br>It exists because a 401 has two meanings a client must treat differently:</br>an expired access token should be refreshed and the request retried,</br>while a revoked one must not be — the refresh token was revoked in the</br>same breath, so the retry burns two requests and fails anyway. Nothing in</br>the old response distinguished them, and a client cannot be asked to</br>match on English.</br></br>Empty on every response that does not need one, so no existing client</br>sees a change. | `token_revoked` |
| message | string (formatted string)| `string` |  | |  | `success` |
| method | string (formatted string)| `string` |  | |  | `GET` |
| path | string (formatted string)| `string` |  | |  | `/api/v1/users` |
| status_code | int32 (formatted integer)| `int32` |  | |  | `200` |
| timestamp | date-time (formatted string)| `strfmt.DateTime` |  | |  | `2021-07-01T00:00:00Z` |



### <span id="payload-health"></span> payload.Health


> Health check of the service.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| checks | [][DomainCheck](#domain-check)| `[]*DomainCheck` |  | |  |  |
| status | string (formatted boolean)| `string` |  | | Health status of a service. | `true` |



### <span id="payload-id-p-available-response"></span> payload.IDPAvailableResponse


> Available identity provider information including type and branding details
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` |  | | Detailed description | `Google Identity Provider` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier | `019b4b0d-a682-7d88-9ce1-12d63815e879` |
| idp_type | [PayloadIDPAvailableResponse](#payload-id-p-available-response)| `PayloadIDPAvailableResponse` |  | | Type information for this identity provider |  |
| logo | uri (formatted string)| `strfmt.URI` |  | | URL to the identity provider's logo image | `https://example.com/logo.png` |
| name | string (formatted string)| `string` |  | | Display name of the identity provider | `Google` |



### <span id="payload-id-p-login-response"></span> payload.IDPLoginResponse


> Response containing OAuth redirect information for initiating login flow
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| idp_id | uuid (formatted string)| `strfmt.UUID` |  | |  | `019b4b0d-a682-7e25-b4c8-afa26aa7d1dc` |
| redirect_code | integer| `int64` |  | |  | `302` |
| redirect_url | uri (formatted string)| `strfmt.URI` |  | |  |  |



### <span id="payload-id-p-register-response"></span> payload.IDPRegisterResponse


> Response containing OAuth redirect information for initiating registration flow
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| idp_id | uuid (formatted string)| `strfmt.UUID` |  | |  | `019b4b0d-a682-7e29-b6e2-a4cc6db87ea8` |
| redirect_code | integer| `int64` |  | |  | `302` |
| redirect_url | uri (formatted string)| `strfmt.URI` |  | |  |  |



### <span id="payload-id-p-response"></span> payload.IDPResponse


> Complete identity provider configuration including OAuth credentials and redirect URLs
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| callback_url | uri (formatted string)| `strfmt.URI` |  | | OAuth callback URL | `https://example.com/callback` |
| client_id | string (formatted string)| `string` |  | | OAuth client ID | `367082405970-example` |
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the identity provider was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Description | `Google Identity Provider` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier | `019b4b0d-a682-7e11-90b0-c94f29b8483a` |
| idp_type | [PayloadIDPResponse](#payload-id-p-response)| `PayloadIDPResponse` |  | | Type |  |
| login_redirect_url | uri (formatted string)| `strfmt.URI` |  | | Login redirect URL | `https://example.com/login` |
| logo | uri (formatted string)| `strfmt.URI` |  | | Logo URL | `https://example.com/logo.png` |
| name | string (formatted string)| `string` |  | | Display name | `Google` |
| register_redirect_url | uri (formatted string)| `strfmt.URI` |  | | Registration redirect URL | `https://example.com/register` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the identity provider was last updated | `2021-01-01T00:00:00Z` |



### <span id="payload-id-p-types-available"></span> payload.IDPTypesAvailable


> Available identity provider types that can be configured in the system
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` |  | | Detailed description of the identity provider type | `Google OAuth2.0 Provider` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the identity provider type | `019b4b0d-a682-7e2c-86fa-05e7237c0c5b` |
| name | string (formatted string)| `string` |  | | Display name of the identity provider type (e.g., Google, Github) | `Google` |



### <span id="payload-id-p-types-response"></span> payload.IDPTypesResponse


> Complete identity provider type information including OAuth scopes and API endpoints
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the identity provider type was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Detailed description of the identity provider type and its capabilities | `Google OAuth2.0 Identity Provider` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the identity provider type | `019b4b0d-a682-7e30-8b33-650caa6446c7` |
| name | string (formatted string)| `string` |  | | Display name of the identity provider type (e.g., Google, Github, Microsoft) | `Google` |
| scopes | []csv (formatted string)| `[]string` |  | | OAuth scopes required for this identity provider type | `["openid","email","profile"]` |
| serial_id | integer| `int64` |  | | Sequential identifier for ordering | `1` |
| system | boolean| `bool` |  | | Indicates if this is a system-managed identity provider type (cannot be deleted) | `true` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the identity provider type was last updated | `2021-01-01T00:00:00Z` |
| user_info_api_url | uri (formatted string)| `strfmt.URI` |  | | API endpoint URL to retrieve user information after authentication | `https://www.googleapis.com/oauth2/v3/userinfo` |



### <span id="payload-link-policies-to-role-request"></span> payload.LinkPoliciesToRoleRequest


> Request payload for attaching multiple policies to a role
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| policy_ids | []array (formatted string)| `[]string` | ✓ | | Array of policy IDs to attach to the role | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-link-projects-to-user-request"></span> payload.LinkProjectsToUserRequest


> Request payload for associating multiple projects with a user
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| project_ids | []array (formatted string)| `[]string` | ✓ | | Array of project IDs to associate with the user | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-link-roles-to-policy-request"></span> payload.LinkRolesToPolicyRequest


> Request payload for associating multiple roles with a policy
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| role_ids | []array (formatted string)| `[]string` |  | | Array of role IDs to associate with the policy | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-link-roles-to-user-request"></span> payload.LinkRolesToUserRequest


> Request payload for assigning multiple roles to a user
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| role_ids | []array (formatted string)| `[]string` | ✓ | | Array of role IDs to assign to the user | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-link-users-to-project-request"></span> payload.LinkUsersToProjectRequest


> Request payload for adding users to a project workspace
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| user_ids | []array (formatted string)| `[]string` |  | | Array of user IDs to add to the project | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-link-users-to-role-request"></span> payload.LinkUsersToRoleRequest


> Request payload for assigning multiple users to a role
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| user_ids | []array (formatted string)| `[]string` | ✓ | | Array of user IDs to assign to the role | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-list-id-p-available-response"></span> payload.ListIDPAvailableResponse


> List of identity providers available for configuration in the system
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadIDPAvailableResponse](#payload-id-p-available-response)| `[]*PayloadIDPAvailableResponse` |  | | Array of available IDP options |  |



### <span id="payload-list-id-p-types-response"></span> payload.ListIDPTypesResponse


> Paginated list of identity provider types available in the system
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadIDPTypesResponse](#payload-id-p-types-response)| `[]*PayloadIDPTypesResponse` |  | | Array of identity provider type configurations |  |
| paginator | [PayloadListIDPTypesResponse](#payload-list-id-p-types-response)| `PayloadListIDPTypesResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-list-id-ps-response"></span> payload.ListIDPsResponse


> Paginated list of identity provider configurations
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadIDPResponse](#payload-id-p-response)| `[]*PayloadIDPResponse` |  | | Array of IDP configurations |  |
| paginator | [PayloadListIDPsResponse](#payload-list-id-ps-response)| `PayloadListIDPsResponse` |  | | Pagination information |  |



### <span id="payload-list-policies-response"></span> payload.ListPoliciesResponse


> Paginated list of authorization policies with access rules
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadPolicyResponse](#payload-policy-response)| `[]*PayloadPolicyResponse` |  | | Array of policy configurations with authorization rules |  |
| paginator | [PayloadListPoliciesResponse](#payload-list-policies-response)| `PayloadListPoliciesResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-list-products-response"></span> payload.ListProductsResponse


> Paginated list of products
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadProductResponse](#payload-product-response)| `[]*PayloadProductResponse` |  | | Array of products |  |
| paginator | [PayloadListProductsResponse](#payload-list-products-response)| `PayloadListProductsResponse` |  | | Pagination information including page tokens |  |



### <span id="payload-list-projects-response"></span> payload.ListProjectsResponse


> Paginated list of project workspaces
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadProjectResponse](#payload-project-response)| `[]*PayloadProjectResponse` |  | | Array of project configurations |  |
| paginator | [PayloadListProjectsResponse](#payload-list-projects-response)| `PayloadListProjectsResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-list-rate-limits-response"></span> payload.ListRateLimitsResponse


> Paginated list of rate-limit rules
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| enforcing | boolean| `bool` |  | | Enforcing is false when ratelimit.enabled=false. The rules above are then</br>real, listable and editable, and applying to nothing.</br></br>It belongs on the LIST because the list is where an operator looks at</br>rules. Reading it from /rate_limits/effective instead would mean inventing</br>a method and an endpoint to ask about, purely to learn a property of the</br>deployment. | `true` |
| items | [][PayloadRateLimitResponse](#payload-rate-limit-response)| `[]*PayloadRateLimitResponse` |  | | Rate-limit rules |  |
| paginator | [PayloadListRateLimitsResponse](#payload-list-rate-limits-response)| `PayloadListRateLimitsResponse` |  | | Pagination information |  |



### <span id="payload-list-resources-limits-response"></span> payload.ListResourcesLimitsResponse


> Paginated list of resource limit configurations across different scopes and resource types
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadResourcesLimitsResponse](#payload-resources-limits-response)| `[]*PayloadResourcesLimitsResponse` |  | | Array of resource limit configurations |  |
| paginator | [PayloadListResourcesLimitsResponse](#payload-list-resources-limits-response)| `PayloadListResourcesLimitsResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-list-resources-response"></span> payload.ListResourcesResponse


> Paginated list of API resource permission definitions
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadResourceResponse](#payload-resource-response)| `[]*PayloadResourceResponse` |  | | Array of resource permission definitions |  |
| paginator | [PayloadListResourcesResponse](#payload-list-resources-response)| `PayloadListResourcesResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-list-roles-response"></span> payload.ListRolesResponse


> Paginated list of roles with their permissions and configurations
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadRoleResponse](#payload-role-response)| `[]*PayloadRoleResponse` |  | | Array of role configurations |  |
| paginator | [PayloadListRolesResponse](#payload-list-roles-response)| `PayloadListRolesResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-list-users-response"></span> payload.ListUsersResponse


> Paginated list of user accounts in the system
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| items | [][PayloadUserResponse](#payload-user-response)| `[]*PayloadUserResponse` |  | | Array of user account details |  |
| paginator | [PayloadListUsersResponse](#payload-list-users-response)| `PayloadListUsersResponse` |  | | Pagination information including total count and page details |  |



### <span id="payload-login-user-request"></span> payload.LoginUserRequest


> Request payload for user authentication via email and password
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| email | email (formatted string)| `strfmt.Email` | ✓ | | User's email address | `admin@goapitemplate.local` |
| password | password (formatted string)| `strfmt.Password` | ✓ | | User's password | `ThisIsApassw0rd.,` |



### <span id="payload-login-user-response"></span> payload.LoginUserResponse


> Response containing authentication tokens and user information after successful login
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| access_token | string (formatted string)| `string` |  | | JWT access token | `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...` |
| permissions | [PayloadLoginUserResponse](#payload-login-user-response)| `PayloadLoginUserResponse` |  | | User's permissions and accessible resources |  |
| refresh_token | string (formatted string)| `string` |  | | JWT refresh token | `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...` |
| token_type | string (formatted string)| `string` |  | | Token type for Authorization header | `Bearer` |
| user_id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier of the authenticated user | `019b4b0d-a682-7e40-a1c3-d5e8f9a2b4c6` |



### <span id="payload-logout-user-request"></span> payload.LogoutUserRequest


  



**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| refresh_token | string| `string` |  | |  | `eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9...` |



### <span id="payload-policy-response"></span> payload.PolicyResponse


> Response containing policy details with authorization rules
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| allowed_action | string (formatted string)| `string` |  | | HTTP method allowed by this policy | `GET` |
| allowed_resource | string (formatted string)| `string` |  | | Resource path pattern that this policy grants access to | `/projects/*/tokens` |
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the policy was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Detailed description of the policy's purpose | `Allows read access to project resources` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the policy | `019b4b0d-a682-7e34-a20c-c71a7147d7e7` |
| name | string (formatted string)| `string` |  | | Policy display name | `Project Viewer` |
| resource | [PayloadPolicyResponse](#payload-policy-response)| `PayloadPolicyResponse` |  | | Resource that this policy applies to |  |
| system | boolean| `bool` |  | | Indicates if this is a system-managed policy (cannot be modified) | `false` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the policy was last updated | `2021-01-01T00:00:00Z` |



### <span id="payload-product-response"></span> payload.ProductResponse


> Response containing a product
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Description | `This is a product` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier | `01980434-b7ff-7abe-a45d-7311bc7011f5` |
| name | string (formatted string)| `string` |  | | Product name, unique within its project | `Product Name` |
| project | uuid (formatted string)| `strfmt.UUID` |  | | Project that owns this product |  |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp updated | `2021-01-01T00:00:00Z` |



### <span id="payload-project-response"></span> payload.ProjectResponse


> Project represents a project.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the project was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Detailed description of the project's purpose | `This is my main project` |
| disabled | boolean| `bool` |  | | Disabled indicates if the project is disabled.</br>Pointer to distinguish between false and unset values in partial responses. | `false` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the project | `019b4b0d-a682-7e34-a20c-c71a7147d7e7` |
| name | string (formatted string)| `string` |  | | Project display name | `My Project` |
| system | boolean| `bool` |  | | Indicates if this is a system-managed project (cannot be deleted) | `false` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the project was last updated | `2021-01-01T00:00:00Z` |



### <span id="payload-rate-limit-response"></span> payload.RateLimitResponse


> A rate-limit rule: where it applies, who to, what it buckets on, and how it is enforced
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| audience | string (formatted string)| `string` |  | | Which callers the rule applies to | `auth` |
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | When the rule was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | What the rule is for | `One expensive call, seconds of wall clock` |
| enabled | boolean| `bool` |  | | Whether the rule is applied | `true` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the rule | `019b4b0d-a682-7e34-a20c-c71a7147d7e7` |
| methods | []array (formatted string)| `[]string` |  | | HTTP verbs the rule covers, or ["*"] for any | `["POST"]` |
| name | string (formatted string)| `string` |  | | Rule display name | `Generate per project` |
| scope | string (formatted string)| `string` |  | | What the bucket is keyed on | `project` |
| strategy | string (formatted string)| `string` |  | | How the budget is enforced | `leaky_bucket` |
| system | boolean| `bool` |  | | System-managed rules cannot be modified or deleted | `false` |
| target | string (formatted string)| `string` |  | | Route template, prefix, or * for the global rule | `/projects/{project_id}/generate` |
| target_kind | string (formatted string)| `string` |  | | How target is matched | `endpoint` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | When the rule was last updated | `2021-01-01T00:00:00Z` |
| windows | [][PayloadRateLimitWindowResponse](#payload-rate-limit-window-response)| `[]*PayloadRateLimitWindowResponse` |  | | Budgets, all of which apply |  |



### <span id="payload-rate-limit-window-request"></span> payload.RateLimitWindowRequest


> One window of a rate-limit rule
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| burst | integer| `int64` |  | | Capacity at once. Omit or 0 to mean the same as requests | `300` |
| period | string (formatted string)| `string` | ✓ | | Window as a duration string, between 1s and 24h | `1m0s` |
| requests | integer| `int64` | ✓ | | Requests allowed within the period | `300` |



### <span id="payload-rate-limit-window-response"></span> payload.RateLimitWindowResponse


> One window of a rate-limit rule: a request budget over a period
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| burst | integer| `int64` |  | | Capacity available at once. 0 means the same as requests | `300` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the window | `019b4b0d-a682-7e34-a20c-c71a7147d7e7` |
| period | string (formatted string)| `string` |  | | Period is rendered as a Go duration string ("1m0s"), not as seconds.</br>Seconds are what the column holds; a duration is what an operator reads</br>and what every other duration in this API uses. | `1m0s` |
| requests | integer| `int64` |  | | Requests allowed within the period | `300` |



### <span id="payload-re-verify-user-request"></span> payload.ReVerifyUserRequest


> Request payload for resending email verification to an unverified user
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| email | email (formatted string)| `strfmt.Email` | ✓ | | Email address of the user to re-verify | `user@example.com` |



### <span id="payload-recover-password-request"></span> payload.RecoverPasswordRequest


> Request payload for initiating password recovery via email
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| email | email (formatted string)| `strfmt.Email` | ✓ | | Email address of the account to recover | `user@example.com` |



### <span id="payload-refresh-token-request"></span> payload.RefreshTokenRequest


> Request payload for obtaining a new access token using a refresh token
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| refresh_token | string (formatted string)| `string` |  | | Optional. The token actually spent is the one in the Authorization</br>header, which is what the middleware verified. This field is kept because</br>every existing client sends the token in both places, but it may not</br>disagree with the header: a request authorised with one token and asking</br>to spend another is refused rather than resolved by picking one. | `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...` |



### <span id="payload-refresh-token-response"></span> payload.RefreshTokenResponse


> Response containing new authentication tokens after successful refresh
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| access_token | string (formatted string)| `string` |  | | New JWT access token | `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...` |
| refresh_token | string (formatted string)| `string` |  | | New JWT refresh token | `eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...` |
| token_type | string (formatted string)| `string` |  | | Token type (always "Bearer") | `Bearer` |



### <span id="payload-register-user-request"></span> payload.RegisterUserRequest


> Request payload for creating a new user account with email verification
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| email | email (formatted string)| `strfmt.Email` | ✓ | | User's email address | `john.doe@example.com` |
| first_name | string (formatted string)| `string` | ✓ | | User's first name | `John` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Optional custom user ID | `019b4b0d-a682-7e41-8c2d-f3a4b5c6d7e8` |
| last_name | string (formatted string)| `string` | ✓ | | User's last name | `Doe` |
| password | password (formatted string)| `strfmt.Password` | ✓ | | User's password (minimum 8 characters) | `SecureP@ssw0rd123` |



### <span id="payload-reset-password-request"></span> payload.ResetPasswordRequest


> Request payload for setting a new password using a reset token
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| password | password (formatted string)| `strfmt.Password` | ✓ | | New password to set (minimum 8 characters) | `NewSecureP@ssw0rd` |



### <span id="payload-resource-response"></span> payload.ResourceResponse


> API resource permission definition for authorization control
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| action | string (formatted string)| `string` |  | | HTTP method or action type | `GET` |
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the resource was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Detailed description of what this permission grants | `Allows reading of user data` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the resource permission | `019b4b0d-a682-7e48-b818-261829e39f76` |
| name | string (formatted string)| `string` |  | | Human-readable name of the permission | `Read Users` |
| resource | string (formatted string)| `string` |  | | API resource path or identifier | `/api/v1/users` |
| system | boolean| `bool` |  | | Indicates if this is a system-managed resource (cannot be deleted) | `false` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the resource was last updated | `2021-01-01T00:00:00Z` |



### <span id="payload-resource-usage-status-response"></span> payload.ResourceUsageStatusResponse


> A single resource's limit and how much of it is used
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| can_create | boolean| `bool` |  | | Whether another one may be created right now | `true` |
| hard_limit | integer| `int64` |  | | Creation is refused at or above this. -1 means no limit is configured | `12` |
| resource_type | string| `string` |  | | Type of resource being limited | `projects` |
| soft_limit | integer| `int64` |  | | Warning threshold. -1 means no limit is configured | `10` |
| soft_limit_reached | boolean| `bool` |  | | Whether usage has reached the warning threshold | `false` |
| tamper_detected | boolean| `bool` |  | | The stored counter failed its integrity check; creation is refused until it is reconciled | `false` |
| usage | integer| `int64` |  | | How many currently exist in this scope | `3` |



### <span id="payload-resources-limits-response"></span> payload.ResourcesLimitsResponse


> Resource limit configuration defining usage constraints and current consumption
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the limit was created | `2021-01-01T00:00:00Z` |
| hard_limit | integer| `int64` |  | | Hard limit threshold (maximum allowed) | `20` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for this resource limit | `019b4b0d-a682-7e38-b235-3dfcb59f4d9e` |
| resource_type | string| `string` |  | | Type of resource being limited | `projects` |
| scope_id | uuid (formatted string)| `strfmt.UUID` |  | | ID of the scope entity (null for system-wide limits) | `019b4b0d-a682-7e3c-aca0-dd93b3229ff7` |
| scope_type | string| `string` |  | | Scope level: system (global), user (per user), or project (per project) | `user` |
| soft_limit | integer| `int64` |  | | Soft limit threshold (warning level) | `10` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the limit was last updated | `2021-01-01T00:00:00Z` |
| usage | integer| `int64` |  | | Current number of resources in use | `5` |



### <span id="payload-resources-limits-status-response"></span> payload.ResourcesLimitsStatusResponse


> Limits and consumption for a single scope, such as the calling user or one project
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| resources | [][PayloadResourceUsageStatusResponse](#payload-resource-usage-status-response)| `[]*PayloadResourceUsageStatusResponse` |  | | One entry per resource type this scope governs |  |
| scope_id | uuid (formatted string)| `strfmt.UUID` |  | | The scope's identifier | `019b4b0d-a682-7e3c-aca0-dd93b3229ff7` |
| scope_type | string| `string` |  | | Scope these limits belong to | `user` |



### <span id="payload-role-response"></span> payload.RoleResponse


> RoleResponse represents a role.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| auto_assign | boolean| `bool` |  | | Indicates if this role is automatically assigned to new users | `false` |
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the role was created | `2021-01-01T00:00:00Z` |
| description | string (formatted string)| `string` |  | | Detailed description of the role's permissions and purpose | `Administrator role with full access` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the role | `019b4b0d-a682-7033-b6af-5e7f9a689613` |
| name | string (formatted string)| `string` |  | | Role display name | `Admin` |
| system | boolean| `bool` |  | | Indicates if this is a system-managed role (cannot be deleted) | `false` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the role was last updated | `2021-01-01T00:00:00Z` |



### <span id="payload-unlink-policies-from-role-request"></span> payload.UnlinkPoliciesFromRoleRequest


> Request payload for attaching multiple policies to a role
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| policy_ids | []array (formatted string)| `[]string` | ✓ | | Array of policy IDs to attach to the role | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-unlink-projects-from-user-request"></span> payload.UnlinkProjectsFromUserRequest


> Request payload for associating multiple projects with a user
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| project_ids | []array (formatted string)| `[]string` | ✓ | | Array of project IDs to associate with the user | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-unlink-roles-from-policy-request"></span> payload.UnlinkRolesFromPolicyRequest


> Request payload for associating multiple roles with a policy
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| role_ids | []array (formatted string)| `[]string` |  | | Array of role IDs to associate with the policy | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-unlink-roles-from-user-request"></span> payload.UnlinkRolesFromUserRequest


> Request payload for assigning multiple roles to a user
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| role_ids | []array (formatted string)| `[]string` | ✓ | | Array of role IDs to assign to the user | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-unlink-users-from-project-request"></span> payload.UnlinkUsersFromProjectRequest


> Request payload for adding users to a project workspace
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| user_ids | []array (formatted string)| `[]string` |  | | Array of user IDs to add to the project | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-unlink-users-from-role-request"></span> payload.UnlinkUsersFromRoleRequest


> Request payload for assigning multiple users to a role
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| user_ids | []array (formatted string)| `[]string` | ✓ | | Array of user IDs to assign to the role | `["019b4b0d-a682-7e34-a20c-c71a7147d7e7","019b4b0d-a682-7e38-b235-3dfcb59f4d9e"]` |



### <span id="payload-update-id-p-request"></span> payload.UpdateIDPRequest


> Request payload for updating an existing identity provider configuration (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| callback_url | uri (formatted string)| `strfmt.URI` |  | | Updated callback URL | `https://example.com/callback` |
| client_id | string (formatted string)| `string` |  | | Updated OAuth client ID | `367082405970-new` |
| client_secret | string (formatted string)| `string` |  | | Updated OAuth client secret | `GOCSPX-new_secret` |
| description | string (formatted string)| `string` |  | | Updated description | `Updated Google Identity Provider` |
| idp_type_id | uuid (formatted string)| `strfmt.UUID` |  | | Updated IDP type ID | `019b4b0d-a682-7e21-9f5c-725b8be59cd5` |
| login_redirect_url | uri (formatted string)| `strfmt.URI` |  | | Updated login redirect URL | `https://example.com/login` |
| logo | uri (formatted string)| `strfmt.URI` |  | | Updated logo URL | `https://example.com/logo-new.png` |
| name | string (formatted string)| `string` |  | | Updated display name | `Google Updated` |
| register_redirect_url | uri (formatted string)| `strfmt.URI` |  | | Updated registration redirect URL | `https://example.com/register` |



### <span id="payload-update-me-request"></span> payload.UpdateMeRequest


> Request payload for updating the authenticated user's profile information (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| first_name | string (formatted string)| `string` |  | | Updated first name | `John` |
| last_name | string (formatted string)| `string` |  | | Updated last name | `Doe` |
| password | password (formatted string)| `strfmt.Password` |  | | New password (must meet security requirements) | `NewSecureP@ssw0rd` |



### <span id="payload-update-policy-request"></span> payload.UpdatePolicyRequest


> Request payload for updating an existing authorization policy (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| allowed_action | string (formatted string)| `string` |  | | Updated HTTP method | `PUT` |
| allowed_resource | string (formatted string)| `string` |  | | Updated resource path pattern | `/projects/*/tokens` |
| description | string (formatted string)| `string` |  | | Updated description | `Updated policy description` |
| name | string (formatted string)| `string` |  | | Updated policy name | `Updated Policy Name` |



### <span id="payload-update-product-request"></span> payload.UpdateProductRequest


> Request payload for updating an existing product (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` |  | | Updated product description | `Updated description` |
| name | string (formatted string)| `string` |  | | Updated product name | `Updated Product Name` |



### <span id="payload-update-project-request"></span> payload.UpdateProjectRequest


> Request payload for updating an existing project (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` |  | | Updated project description | `Updated project description` |
| disabled | boolean| `bool` |  | | Set to true to disable the project, false to enable | `false` |
| name | string (formatted string)| `string` |  | | Updated project name | `Updated Project Name` |



### <span id="payload-update-rate-limit-request"></span> payload.UpdateRateLimitRequest


> Request payload for replacing a rate-limit rule. The window set is replaced in full
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| audience | string (formatted string)| `string` |  | |  | `auth` |
| description | string (formatted string)| `string` | ✓ | |  | `One expensive call, seconds of wall clock, real money` |
| enabled | boolean| `bool` |  | |  | `true` |
| methods | []array (formatted string)| `[]string` | ✓ | |  | `["POST"]` |
| name | string (formatted string)| `string` | ✓ | |  | `Generate per project` |
| scope | string (formatted string)| `string` | ✓ | |  | `project` |
| strategy | string (formatted string)| `string` |  | |  | `leaky_bucket` |
| target | string (formatted string)| `string` | ✓ | |  | `/projects/{project_id}/generate` |
| target_kind | string (formatted string)| `string` | ✓ | |  | `endpoint` |
| windows | [][PayloadRateLimitWindowRequest](#payload-rate-limit-window-request)| `[]*PayloadRateLimitWindowRequest` | ✓ | |  |  |



### <span id="payload-update-role-request"></span> payload.UpdateRoleRequest


> Request payload for updating an existing role (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| description | string (formatted string)| `string` |  | | Updated role description | `Updated role description` |
| name | string (formatted string)| `string` |  | | Updated role name | `Updated Editor` |



### <span id="payload-update-user-request"></span> payload.UpdateUserRequest


> Request payload for updating an existing user account (all fields optional)
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| disabled | boolean| `bool` |  | | Set to true to disable the account, false to enable | `false` |
| email | email (formatted string)| `strfmt.Email` |  | | Updated email address | `john.doe@example.com` |
| first_name | string (formatted string)| `string` |  | | Updated first name | `John` |
| last_name | string (formatted string)| `string` |  | | Updated last name | `Doe` |
| local_account | boolean| `bool` |  | | Set account type (local vs federated) | `true` |
| password | password (formatted string)| `strfmt.Password` |  | | New password | `NewSecureP@ssw0rd` |



### <span id="payload-user-authz-response"></span> payload.UserAuthzResponse


> A user's effective permissions, by category
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| permissions | [PayloadUserAuthzResponse](#payload-user-authz-response)| `PayloadUserAuthzResponse` |  | | Effective permissions, keyed by category |  |



### <span id="payload-user-response"></span> payload.UserResponse


> UserResponse represents a user entity.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| admin | boolean| `bool` |  | | Indicates if the user has administrative privileges | `false` |
| created_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the user account was created | `2021-01-01T00:00:00Z` |
| disabled | boolean| `bool` |  | | Indicates if the user account is currently disabled | `false` |
| email | email (formatted string)| `strfmt.Email` |  | | User's email address | `john.doe@example.com` |
| first_name | string (formatted string)| `string` |  | | User's first name | `John` |
| id | uuid (formatted string)| `strfmt.UUID` |  | | Unique identifier for the user | `019b4b0d-a682-7e82-a25a-b0671dc354c2` |
| last_name | string (formatted string)| `string` |  | | User's last name | `Doe` |
| local_account | boolean| `bool` |  | | Indicates if this is a locally managed account (vs SSO/federated) | `true` |
| updated_at | date-time (formatted string)| `strfmt.DateTime` |  | | Timestamp when the user account was last updated | `2021-01-01T00:00:00Z` |



### <span id="payload-version"></span> payload.Version


> Version is the struct that holds the version information.
  





**Properties**

| Name | Type | Go type | Required | Default | Description | Example |
|------|------|---------|:--------:| ------- |-------------|---------|
| build_date | string (formatted string)| `string` |  | |  | `2021-01-01T00:00:00Z` |
| git_branch | string (formatted string)| `string` |  | |  | `main` |
| git_commit | string (formatted string)| `string` |  | |  | `abcdef123456` |
| go_version | string (formatted string)| `string` |  | |  | `go1.24.1` |
| go_version_arch | string (formatted string)| `string` |  | |  | `amd64` |
| go_version_os | string (formatted string)| `string` |  | |  | `linux` |
| version | string (formatted string)| `string` |  | |  | `1.0.0` |


