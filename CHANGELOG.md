# Changelog

What changed, by release, for whoever runs this service or carries its
changes into a copy of it. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Every release has
a **Security** section when it has anything to say there: read that one
first if you maintain a fork. Each entry names the pull request, so that the
change can be read and ported.

Reporting a vulnerability: [`SECURITY.md`](./SECURITY.md).

## [Unreleased]

### Upgrade notes

- **The layer instruments no longer carry `successful`.** It was kept beside
  `app.outcome` for one release (`v0.0.4`), so that a saved query or an alert
  could move. A query or an alert that still reads `successful` matches
  nothing: read `app_outcome` (`failed` for an error ratio). (#179)

### Changed

- **qfv v1.1.1: a filter can carry an `ESCAPE` clause.**
  `first_name LIKE 'a!_b' ESCAPE '!'` lists, where it was a `400`; what
  PostgreSQL refuses of the clause (two characters, the clause after `~~`)
  is refused as an invalid expression. The filter grammar is PostgreSQL 18's,
  qfv's default dialect and the service's database.
  ([#180](https://github.com/slashdevops/go-rest-api-service-template/pull/180))
- **`successful` is removed from `app_calls_total` and
  `app_call_duration_seconds`**, and from the cache's and the mail's mirrors
  of them. `app.outcome` (`ok`, `refused`, `failed`) replaced it in `v0.0.4`.
  ([#179](https://github.com/slashdevops/go-rest-api-service-template/pull/179))

## [0.0.4] - 2026-10-24

Since `v0.0.3`. The fixes listed under Security are the ones advisory
[GHSA-m74g-rxpq-57hr](https://github.com/slashdevops/go-rest-api-service-template/security/advisories/GHSA-m74g-rxpq-57hr)
is about.

### Security

- **An identity is stored, and found, only under the provider it signed in
  with.** A provider may be pointed at another issuer while nobody is
  linked to it, and a first sign-in (or a link) already under way, or one
  that read the provider from the cache, stored its identity under the
  provider as it was now: a subject the old issuer proved, at a provider
  that trusts another one, which could then sign in as that account by
  asserting it. Such a sign-in also found an existing identity by the
  provider and the subject alone, and was signed in as any account linked
  to that subject under the new issuer: a takeover, by whoever answers for
  the old issuer, of an account linked after the repoint. The lookup names
  the issuer and the type the sign-in used, as the insert does.
  And a repoint did not wait for an identity being stored, and went through
  under it. The identity's insert reads the provider's row with the issuer
  and the type the sign-in used, `FOR KEY SHARE`, and stores nothing when
  they changed (`401`, "the identity provider changed while signing in";
  on a first sign-in no account either); the repoint takes the row `FOR
  UPDATE` first. A provider deleted during a sign-in is answered the same.
  ([#152](https://github.com/slashdevops/go-rest-api-service-template/pull/152))
- **A first sign-in through a provider is one transaction.** It created
  the account, then stored the identity; when the identity could not be
  stored (linked by a concurrent sign-in, the provider deleted meanwhile, a
  fault), the account stayed with no password and no identity, holding the
  address and a users slot, and the person's every later sign-in was
  refused. The account, its roles and its identity are written in one
  transaction now, and the slot is given back off the request's context, so
  a client that walks away does not leave the count one too high. An
  address taken during the sign-in is answered as an identity that is not
  linked (`401`); it mailed the address's owner as the registration form
  does, and then answered `500`. A first sign-in when the users limit is
  reached is a `409`, as `POST /users` answers it; it answered `500`.
  ([#151](https://github.com/slashdevops/go-rest-api-service-template/pull/151))
- **A provider's delete waits for a sign-in through it, and an account's
  ways in are removed one at a time.** A delete of an identity provider
  read its rule (no account whose only way in is an identity there) before
  an identity being written committed, then went ahead without reading it
  again, and its cascade removed the identity just stored. And two writes
  that each removed one of an account's two ways in -- the deletes of its
  two providers, or a delete and an unlink -- each saw the other way in and
  both went. Either way an account was left that nobody could sign in to.
  The delete is one transaction that takes the provider's row `FOR UPDATE`
  and the linked accounts' rows `FOR NO KEY UPDATE` before its rule; the
  unlink takes the account's row and holds its rule in the statement that
  deletes. The second write waits and answers `409`. A first sign-in
  stores the account and its identity together (#151), so when the delete
  comes first nothing is stored.
  ([#149](https://github.com/slashdevops/go-rest-api-service-template/pull/149))
- **A list with no project in its path is held to its caller.**
  `GET /products` answered every project's products to whoever held its
  grant, and the seeded `ProjectMember` role holds it: a member read the
  products, ids and names of projects it does not belong to.
  `GET /resources_limits` answered every project's and every user's usage
  rows, with their ids, to any holder of `GET *` (the seeded `ReadOnly`).
  The products list is now the products of the caller's projects; the
  usage list the caller's projects' rows and the caller's own. The
  administrator account (the `admin` flag, not the `Administrator` role)
  is answered every row of both, and alone the system's usage rows.
  ([#148](https://github.com/slashdevops/go-rest-api-service-template/pull/148))
- **A reset link is bound to the address it was mailed to.** A link mailed
  to an address the account no longer has set the password until it
  expired, and so did one mailed before an administrator disabled the
  account. The password is written by a statement of its own, only while
  the account has the address in the link, is a local account and is not
  disabled; anything else is one `401`, "the link is not valid for the
  account", and the link is spent. Every reset token has carried the
  address, so links already in a mailbox are bound too.
  `POST /users/{user_id}/password/reset` reads the account from the
  database, not the cache, and mails the address it has now. A link
  followed a second time answered `500`; it is the documented `401`. On
  the administrator's route a stored address that is not a valid one
  answered `500`, not the documented `400`, and a `429` now carries
  `Retry-After`.
  ([#147](https://github.com/slashdevops/go-rest-api-service-template/pull/147))
- **A role an administrator account holds is not deleted by another
  account.** `DELETE /roles/{role_id}` takes the role from every account
  that holds it, and an account with the `admin` flag was stripped of it by
  a caller without the flag, where the unlink of that one account was
  refused. While an account with the flag holds the role, its delete is a
  `403` unless the caller has the flag. (#143)
- **Go 1.27.3**, for
  [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809): `crypto/x509`
  distrusts compromised intermediate certificates. On 1.27.2 it is reached
  through the SMTP mailer's certificate check and every other TLS client
  the service runs. `make go-mod-update` raises OPA to 1.21.3 with it.
  ([#139](https://github.com/slashdevops/go-rest-api-service-template/pull/139))
- **An identity provider is not pointed elsewhere under its identities.**
  An identity is (provider, subject), and `PUT /auth/idps/{idp_id}` could
  change a provider's issuer and type while its identities stayed: whoever
  holds that route (the Administrator role, which is not the `admin` flag)
  pointed the provider at an issuer of their own, asserted a linked subject,
  and was signed in as that account. And the client and the callback, set
  to an application and a host of one's own at the real issuer, sent a
  linked user's code there. While accounts are linked, another issuer or
  type is a `409` (the update's own statement), and a change of `client_id`
  or `callback_url` is the administrator account's to make (`403` for
  anyone else). A provider nobody is linked to changes as before. The design
  is in
  [`identity-providers.md`](./docs/architecture/identity-providers.md#pointing-a-provider-elsewhere).
  ([#138](https://github.com/slashdevops/go-rest-api-service-template/pull/138))
- **go-jose 4.1.6**, which `go-oidc` uses to verify an identity provider's
  ID tokens on the OIDC sign-in path: two security releases, among them
  [GHSA-mgcc-cww5-fp2q](https://github.com/go-jose/go-jose/security/advisories/GHSA-mgcc-cww5-fp2q),
  the algorithm checked against the key's curve when verifying, and no
  panic on an empty CBC-HMAC ciphertext or an invalid key. JWK parsing now
  refuses RSA moduli over 4096 bits; providers publish 2048-bit keys.
  `make go-mod-update` had left it behind: it raised an indirect module
  only as far as a direct one required it, and now also runs
  `go get -u -t ./...`.
  ([#137](https://github.com/slashdevops/go-rest-api-service-template/pull/137))
- **The `admin` flag is granted, and taken back, only by an account that
  has it.** `PUT /users/{user_id}` accepts `admin`; it is not a grant of the
  policy engine, so the route's grant (the `UserManager`'s, the
  Administrator role's) is not enough, and a change from anyone else is a
  `403`. The last account with the flag that can sign in keeps it, and the
  seeded account keeps it by a trigger on its row. A deployment is no longer
  left with one administrator account and no way back but the database. The
  design is in
  [`docs/architecture/security.md`](./docs/architecture/security.md#the-admin-flag-is-granted-by-an-account-that-has-it).
  ([#134](https://github.com/slashdevops/go-rest-api-service-template/pull/134))
- **An identity provider's delete strands nobody, and neither its delete
  nor its disable shuts the last door.** The delete took the provider's
  identities with it and left every account that signed in through it, with
  no password, without a way in. It is a `409` while an account's only way
  in is an identity at that provider; disable the provider instead, or
  delete those accounts or make them local accounts first. Disabling or
  deleting a provider is a `403` when it would leave no administrator
  account that can sign in, and an account's own unlink keeps an identity
  at an enabled provider. The design is in
  [`docs/architecture/identity-providers.md`](./docs/architecture/identity-providers.md#deleting-or-disabling-a-provider).
  ([#132](https://github.com/slashdevops/go-rest-api-service-template/pull/132))
- **An administrator account is locked, and unlocked, only by one.** An
  account with the `admin` flag is disabled or enabled, made a federated or
  a local account, stripped of a role or deleted only by an account that
  has the flag, and the last one that can still sign in (with a password,
  or through an enabled provider) by nobody. A role
  that manages users managed that account like any other. From anyone else
  such a change is a `403`; a form that sends back the values it read is
  unaffected. The design is in
  [`docs/architecture/security.md`](./docs/architecture/security.md#an-administrator-account-is-locked-only-by-one).
  ([#131](https://github.com/slashdevops/go-rest-api-service-template/pull/131))
- **A list that fails says nothing of the cause.** A list answered an
  unexpected failure with `500` and the error's own text as the message;
  that text is the driver's or the database's, and a connection that cannot
  be opened names where the service connects to and as whom. A `500` from a list is now
  `internal server error`, with the cause on the request's log line under
  its request id. A `sort`, `filter` or `fields` that the service refuses
  is still a `400` that says what to correct.
  ([#128](https://github.com/slashdevops/go-rest-api-service-template/pull/128))
- **The three routes about a user's projects are held to the projects of
  whoever asks.** `GET /users/{user_id}/projects` answers the projects of
  that user that are the caller's too, and `POST` and `DELETE
  /users/{user_id}/projects` change a link only on a project the caller is
  linked to. For an account with the `admin` flag that is every project.
  The design is in
  [`docs/architecture/authorization.md`](./docs/architecture/authorization.md#a-route-about-a-users-projects-is-held-to-its-caller).
  ([#127](https://github.com/slashdevops/go-rest-api-service-template/pull/127))
- **An account an administrator disabled is not opened by its verification
  link.** A registration is disabled until its link is followed, and the
  link must open only that. An account created through `POST /users` is
  stored verified, and a `disabled` set through `PUT /users/{user_id}` is
  remembered in a new column, `users.disabled_by_admin`; the verification
  enables only a row without it.
  ([#125](https://github.com/slashdevops/go-rest-api-service-template/pull/125))
- **A disabled or deleted account is refused on every route.** A token that
  verifies says nothing about its account: every chain that verifies an
  access token now asks whether the account may act, and answers a disabled
  or deleted one `401` with the code `token_revoked`. The refresh and a
  sign-in through an identity provider ask the database too.
  ([#124](https://github.com/slashdevops/go-rest-api-service-template/pull/124))
- **Only the administrator account changes an account's e-mail address.**
  The address is where a password reset is sent. From any other caller a
  change of it through `PUT /users/{user_id}` is a `403`.
  ([#123](https://github.com/slashdevops/go-rest-api-service-template/pull/123))
- **A list expression over its length is refused before it is parsed.**
  `filter`, `sort` and `fields` are bounded: a query string is not covered
  by the request-body limit.
  ([#121](https://github.com/slashdevops/go-rest-api-service-template/pull/121))
- **The filter parser bounds its nesting** (`qfv` v1.0.3).
  ([#117](https://github.com/slashdevops/go-rest-api-service-template/pull/117))
- **A repository trace no longer carries an address and a password hash**
  at a level that is exported.
  ([#104](https://github.com/slashdevops/go-rest-api-service-template/pull/104))
- **Listing users needs a grant for it**, and is not every account's.
  ([#101](https://github.com/slashdevops/go-rest-api-service-template/pull/101))
- **The hardening phases**: the middleware chain and its order, response
  headers, body and content-type bounds, error hygiene, project membership,
  single-use links, the outbound address guard. The design is in
  [`docs/architecture/security.md`](./docs/architecture/security.md).
  ([#94](https://github.com/slashdevops/go-rest-api-service-template/pull/94))

### Upgrade notes

- **A request its caller abandoned is answered 499 and counted
  `refused`.** The access line of a request whose client closed the
  connection before it was answered says `status` 499 at INFO with
  `error_type` `context.Canceled`, where it said 500 at ERROR, and its calls
  leave the `app_outcome="failed"` error ratio. A dashboard or alert that
  counted those 500s counts 499s now. Slowness behind a proxy that gives up
  before the service does reads the same way; the duration histogram still
  shows it. (#177)
- **A new route, `GET /auth/idps/{idp_id}/identities`, has its `resources`
  row** in `00008_roles_policies_tables_upsert.sql`, edited in place. A
  database created from an earlier version does not list it among the
  resources until it is recreated, or the row is inserted by hand; the
  route answers whoever the policies allow either way. (#176)
- **Four 400 bodies that quoted Postgres say a fixed sentence**: "the
  request refers to a field that does not exist", "a value has the wrong
  type for its field", "the request contains a byte sequence that is not
  valid text", "the request could not be read by the database". A client
  that matched Postgres's words matches these. (#172)
- **A refused list expression's message changes.** A list's 400 for a
  `sort`, `filter` or `fields` the parser refuses says, after its
  `validation error in field '<sort|filter|fields>':` prefix, `'x' is not a
  sortable field` (filterable, selectable) or what the expression must look
  like, where it said the parser's own words ("error on field 'x': field not
  allowed for sorting"). A client that matched the parser's words matches
  the prefix, which is unchanged, or the new sentences. The status (400) is
  unchanged. (#171)
- **`DELETE /roles/{role_id}` and `DELETE /policies/{policy_id}` answer
  404 for an id that is not there**; they answered 200. A client that
  treated either delete as idempotent on its status reads 404 as "already
  gone". (#164)
- **`DELETE /projects/{project_id}` and `DELETE /projects/{project_id}/users`
  answer 404 for a project that is not there**, for every caller; they
  answered the administrator account 200 "deleted" and "unlinked". A client
  that treated a project delete as idempotent on its status reads 404 as
  "already gone". (#161)
- **A new setting, `authn.idp.timeout`** (`AUTHN_IDP_TIMEOUT`, `10s`):
  how long one request to an identity provider may take. A value outside
  `1s` to `1m` stops the process at startup. `http.client.timeout`,
  `http.client.max.retries` and `http.client.retry.strategy` no longer
  apply to the identity providers (they still apply to the mail API
  sender); the address guard (`http.client.allow.private.addresses`) and
  the connection settings still do. (#160)
- **`users.disabled`, `users.admin` and `users.local_account` are `NOT NULL`**
  (`00002_users_tables.sql`, edited in place). A database created from an
  earlier version keeps the nullable columns, and the migration tool does
  not see the edit: recreate it, or run the statements the migration's
  comment gives, which set a value never set to the column's default and
  then add `NOT NULL`. A NULL was read three ways (the sign-in, the count of
  enabled administrator accounts, the projects' view). (#144)
- **The `users` table has a trigger**, `tr_protect_bootstrap_administrator`:
  the seeded account keeps its `admin` flag. Added to the file that creates
  the table; a database created from an earlier version does not have it
  until it is recreated (the function and the trigger are in
  `00002_users_tables.sql`, to run by hand otherwise). (#134)
- **The `users` table has a new check**, `users_locked_is_disabled`: an
  account an administrator locked is a disabled one. Added to the file that
  creates the table, as the column was; a database created from an earlier
  version does not have it until it is recreated, or
  `ALTER TABLE users ADD CONSTRAINT "users_locked_is_disabled" CHECK (NOT disabled_by_admin OR disabled IS TRUE);`
  (#133)
- **`PUT /users/{user_id}` can answer `400`** for a body whose `disabled`
  changes the account against the `disabled_by_admin` it carries. (#133)
- **`DELETE /auth/idps/{idp_id}` can answer `409`**, and it and
  `PUT /auth/idps/{idp_id}` with `enabled: false` can answer `403`. A client
  that deleted providers to retire them disables them now. (#132)
- **The `users` table has a new column**, added to the file that creates
  the table (`00002_users_tables.sql`), as this template does before a first
  deploy. The migration tool does not see an edit: **a database created from
  an earlier version starts, and then fails every write to `users`.**
  Recreate it, or run
  `ALTER TABLE users ADD COLUMN disabled_by_admin BOOLEAN NOT NULL DEFAULT FALSE;`
  and set it by hand on the unverified, disabled rows an administrator
  disabled. (#125)
- **`/me` answers `401` with `token_revoked`** for a disabled or a deleted
  account, where it answered `403` and `404`. A client that treated those
  two apart has one case now, and it is "the session is over". (#124)
- **`PUT /users/{user_id}` can answer `403` for a body that carries
  `email`.** A form that sends back the address it read is unaffected. (#123)
- **Four routes can answer `403` for an account with the `admin` flag**:
  `PUT /users/{user_id}` for a change of `disabled`, `disabled_by_admin` or
  `local_account`,
  `DELETE /users/{user_id}`, `DELETE /users/{user_id}/roles` and
  `DELETE /roles/{role_id}/users`, when the caller does not have the flag,
  and for everybody when it is the last such account that can sign in.
  Deleting the seeded administrator, and taking the Administrator role from
  it through either route, is a `403` where it was a `500`. (#131)
- **An account created through `POST /users` is `verified`** in what the
  API answers. (#125)
- **`GET /users/{user_id}/projects` answers fewer rows to a caller without
  the `admin` flag**: the ones the caller is linked to too. And `POST` and
  `DELETE /users/{user_id}/projects` answer `400` for a project that is not
  one of the caller's, **and for one that does not exist**, where a link
  answered `404` "User not found" and an unlink `200`. A role that manages
  users and is a member of no project links nobody. (#127)
- `/health/detailed` answers `500`, not its `503` with the per-component
  payload, while the database is unreachable: the account check in front of
  it needs the database. A probe still sees a failure. (#124)
- Go 1.27.2. (#122)

### Added

- **`GET /auth/idps/{idp_id}/identities` lists the accounts linked to a
  provider**, each with when it was linked and `only_way_in`: true for the
  accounts with no password and an identity at no other provider, which are
  exactly the ones a 409 on the provider's delete is about. The 409 told an
  administrator to delete those accounts or make them local, and no route
  said which they were. Under the users' sort, filter and fields; a provider
  that is not there is a 404; the subject is not shown.
  ([#176](https://github.com/slashdevops/go-rest-api-service-template/pull/176))

### Changed

- **A request its caller abandoned is not a 500.** A sign-in whose tab was
  closed while the code was being exchanged was a 500 at the callback,
  counted `failed` at every layer, with an ERROR access line: nobody did
  anything wrong and nobody receives the answer. `respond.IsRefusal` knows
  `context.Canceled`; `respond.WriteCallerGone` answers 499 with no body when
  the request's own context is the one cancelled, and is asked first by
  `WriteInternalError` and by every site that writes a 500 of its own
  (`TestEvery500AsksWhetherTheCallerWentAway`); `error_type` names
  `context.Canceled`. A deadline (`context.DeadlineExceeded`) stays a
  failure. ([#177](https://github.com/slashdevops/go-rest-api-service-template/pull/177))
- **The integration suite waits a rate limiter's refusal out.** The suite
  is one address. Every request goes through `sendHTTPRequest`, which now
  sends a request the limiter refused with `RATE_LIMIT_EXCEEDED` again after
  its `Retry-After` (one to five seconds, twenty times at most), and one it
  refused with `RATE_LIMIT_UNAVAILABLE` three times at most, out of thirty
  for the whole run, so that a store that is really down still fails the
  run at once. The login and recovery throttle's 429 has no code and is
  never waited out. Test-only. ([#168](https://github.com/slashdevops/go-rest-api-service-template/pull/168))
- **A refused call is not a red span.** A span under the server span
  carries `app.outcome`, and its status follows it: a failed call is
  `Error` with an `exception` event, as before; a refused one (a 4xx) has
  its status left unset and a `refused` event with the error's type and
  text. A trace backend's `status = error` was every request a client got
  wrong; it is the failures now.
  ([#155](https://github.com/slashdevops/go-rest-api-service-template/pull/155))
- **A call says how it ended: `app.outcome` is `ok`, `refused` or
  `failed`,** on `app_calls_total` and `app_call_duration_seconds`.
  `successful="false"` counted a request the service refused (a bad field,
  a row that is not there, a limit) and a call that failed (the database
  gone) as one thing: a burst of bad requests raised every error ratio.
  The dashboards' ratios read `app_outcome="failed"`. `successful` stays
  beside it for one release: move a saved query or an alert to
  `app_outcome` before it goes. The cache and mail layers, which record on
  the pair themselves, set it too; a cache read past its timeout is a
  failure, and its `successful` is `false` with it. A login, registration, re-verification or refresh body cut by
  the size bound is answered `413`, as on every other route, instead of a
  `400` that said "failed to decode request".
  ([#154](https://github.com/slashdevops/go-rest-api-service-template/pull/154))
- **One table says which side of 500 each error is on.** `respond.StatusFor`
  maps each domain error the handlers answer, through an arm or one they
  build themselves, to its status, read off the handlers' own arms; `respond.IsRefusal` says whether an error is the
  caller's to fix (below 500, or a body that could not be decoded, which
  the decoder now marks as a `respond.DecodeError`); `respond.WriteError`
  answers with the table's status. No answer changes: the handlers keep
  their arms, and `TestEveryArmAgreesWithTheTable` holds every arm to the
  table's side of 500. A span for a body that could not be decoded names
  `*respond.DecodeError` as its exception type. A fork that adds an error
  type a handler answers below 500 adds a row.
  ([#153](https://github.com/slashdevops/go-rest-api-service-template/pull/153))
- **Dependencies updated** with `make go-mod-update`: OPA 1.21.2, goose
  3.29.0, c3e 0.0.3, qfv 1.0.5 (the parsers answer as with 1.0.4),
  valkey-go 1.0.79, and the indirect modules they bring. The target now
  ends with `go build ./...` and `go vet` on every tag, since `go mod tidy`
  accepts a graph that does not build.
  ([#135](https://github.com/slashdevops/go-rest-api-service-template/pull/135))
- **The administrator's lock is a field of its own.** `disabled_by_admin` is
  in the user answers (`GET /users`, `GET /users/{user_id}`, `/me`), the
  users lists filter, sort and select by it, and `PUT /users/{user_id}`
  writes it: `true` locks the account (disabled, and marked so that its
  verification link opens nothing) whatever `disabled` already is, which is
  how a registration that was never enabled is shut; `false` unlocks it,
  enabled if its address is verified and waiting for its link if not. A
  value the account already has changes nothing. On an account with the
  `admin` flag the lock is the administrator account's to change, as
  `disabled` is. The design is in
  [`docs/architecture/authentication.md`](./docs/architecture/authentication.md).
  ([#133](https://github.com/slashdevops/go-rest-api-service-template/pull/133))
- **The filter parser is `qfv` v1.0.4**, which reads three inputs as
  PostgreSQL does. A number or a boolean as a pattern (`name LIKE 1`,
  `SIMILAR TO 1`, the `~~` operators) is refused, as PostgreSQL has no such
  operator for any column. `YES` and `NO` are names and no boolean literals:
  `disabled = yes` is refused, where it reached PostgreSQL as a column that
  does not exist. `IS` in lower case is the keyword (`id is null`), which was
  refused in that one spelling. A `400` before and after, in other words for
  the first two. (#129)
- A failed request is one log line, at the level of its answer: `ERROR` for
  a `5xx`, `INFO` for a `4xx`. (#115)
- Logs go through OpenTelemetry as a third signal, beside the local output
  and never instead of it; every request-path log carries its trace; the
  instruments are one pair for every layer, and the dashboards are rebuilt.
  (#102, #103, #105, #106, #107)
- Authorization: the policy is compiled once and the caller's grants travel
  as input; a caller may grant only what they hold; default roles. (#96)
- `verified` is its own column, beside `disabled`; a password is 8 to 72
  bytes, and login does not apply the rule for choosing one. (#99, #108)

### Fixed

- **The contract's filter examples are filters the API accepts.** Five
  `filter` descriptions gave an example a client copies and the API
  refuses: `id=1 AND first_name='John'` on the three user lists (a number
  for a uuid column) and `'name LIKE policy%'` on the two policy lists (the
  value unquoted). They read `first_name='John' AND disabled=false` and
  `name LIKE 'policy%'`, and a test sends every example in the contract to
  a list that documents it. ([#175](https://github.com/slashdevops/go-rest-api-service-template/pull/175))
- **A failure carries its detail, and the one line about it says it.** A
  sign-in is refused, and a provider that cannot be used is answered, in
  fixed words on purpose; the reason was a second line at WARN beside the
  access line, then (since #173) a DEBUG line an operator at INFO never saw:
  an expired client secret was a 503 whose ERROR line said nothing else. The
  reason is the error's `Detail`, which its `Error()` never prints, and the
  one line about the failure carries it as `detail` (the ERROR access line
  of a 5xx, the DEBUG companion of a refusal, `operation_failed` outside a
  request), bounded to 1 KiB; the identity providers' adapter writes no line
  of its own. The callback's `error=` from the provider is no longer an INFO
  line of its own either, and the IdP use-case's log keys are
  `idp_id`/`user_id`. ([#174](https://github.com/slashdevops/go-rest-api-service-template/pull/174))
- **A sign-in logs in its request, and a provider that cannot be used is
  the service's side.** Seventeen log lines on the request path took no
  context (the identity providers' adapter, the token verifier, the
  middleware's validator): no `trace_id`, and for a failed sign-in a second
  line at WARN for a failure the access line already reports. They take the
  request's context, the adapter's at DEBUG with the provider's id. A
  provider row the service cannot sign in with (no client, no issuer for an
  OpenID Connect provider, an unknown kind: only a row written by hand)
  answered the public start routes 400 naming the provider; it is a 503 in
  the callback's sentence. A state the service cannot sign is a 500, not a
  400. ([#173](https://github.com/slashdevops/go-rest-api-service-template/pull/173))
- **A filter Postgres refuses is a 400, in our words, and no driver text
  is in a body.** A filter of the wrong type for its field (`GET
  /users?filter=id = 'nope'`, `disabled = 'maybe'`, `id = 1`, an invalid
  date or time zone, a pattern that is not one) passed the parser, which
  does not know column types, and Postgres's refusal was answered 500, on
  every list. It is a 400: "a value does not fit the type of the field it is
  compared with", or "a pattern is not a valid regular expression or LIKE
  pattern". And the four refusals every repository already answered with a
  400 (a column that does not exist, a text field used as a boolean, bad
  bytes, a malformed message) quoted Postgres, the statement's aliases
  included; each says a fixed sentence now, and the rate limits'
  repository, which mapped only the bad bytes, maps all four.
  ([#172](https://github.com/slashdevops/go-rest-api-service-template/pull/172))
- **A refused list expression is answered in our words, under one code.**
  A list's 400 for a `sort`, `filter` or `fields` it could not parse copied
  the qfv parser's error ("error on field 'x': field not allowed for
  sorting"), so a qfv release could reword every list's 400. The message is
  ours: `'x' is not a sortable field` (filterable, selectable) for a field
  the list does not accept, or what the expression must look like; and
  inside the service the refusal has one code, `INVALID_EXPRESSION`, where
  it had seven for the same failure.
  ([#171](https://github.com/slashdevops/go-rest-api-service-template/pull/171))
- **A list's expressions reach its span after they are validated.** The
  six lists that record their `sort`, `filter` and `fields` on the
  use-case's span did so before the input was validated (by the
  repository, after them), so an expression refused for its length was
  still exported to the trace backend, up to the server's header limit per
  request. Each list validates first; a test reads the package for the
  order. ([#170](https://github.com/slashdevops/go-rest-api-service-template/pull/170))
- **A request that reaches a handler without its caller is a fault of the
  service, at every site.** Only a route wired without the authentication
  middleware can produce it. The helpers that read the caller returned the
  status table's 401 type: some sites answered 500 counted as a refusal,
  others 400 or 401 with the error's text, telling the caller their token
  was at fault, and `POST /auth/password/reset` wrote "failed to get user id
  from context" into a 500's body. Every site answers the fixed 500, counted
  as a failure, with the cause on the access line. `GET
  /me/resources_limits` no longer declares a 400 it cannot write.
  ([#169](https://github.com/slashdevops/go-rest-api-service-template/pull/169))
- **`GET /rate_limits/effective` records its refusals.** Its three 400s
  (no `method`, a method that is none, no `endpoint`) were written and
  returned with nothing recorded: the call was in no count of the handler
  layer, its span had no outcome and its access line no `error_type`. Each
  is recorded as an invalid input, which the status table answers with the
  400 written; the bodies are unchanged.
  ([#166](https://github.com/slashdevops/go-rest-api-service-template/pull/166))
- **A failure a middleware answers is said to arise in it.** The access
  line of a request names where its failure arose. `CheckAuthz` (an
  authorization store fault) and `Logging`'s recover (a panic) handed their
  failure to it directly, so the line named
  `net/http.HandlerFunc.ServeHTTP` and `runtime.gopanic`; they go through
  the middleware's helper, which names the middleware, and a test holds the
  package to it. ([#165](https://github.com/slashdevops/go-rest-api-service-template/pull/165))
- **Deleting a role or a policy that is not there is a 404.** Both deletes
  answered as if they had deleted: `DELETE /roles/{role_id}` and `DELETE
  /policies/{policy_id}` said 200 "deleted" for an id that matched no row,
  twice in a row for the same id, while the user and identity-provider
  deletes answered 404. (The policy's handler also turned its not-found
  into the 200, on purpose; it answers 404.) Nothing is deleted either way.
  `DELETE /policies/{policy_id}` now declares its 404.
  ([#164](https://github.com/slashdevops/go-rest-api-service-template/pull/164))
- **A delete drops what is cached of its row before it gives the usage
  slot back.** The user, identity-provider and project deletes gave the slot
  back first, and that decrement can fail after the row is gone (a fault,
  or a counter whose signature does not verify): the call returned the
  error with the deleted row still cached, so `GET /users/{user_id}`
  answered a deleted account until the entry expired. And a project's read,
  scoped to the user by its statement, was cached under the project alone;
  it is keyed by the user too, and the project's key drops every user's
  read of it. ([#163](https://github.com/slashdevops/go-rest-api-service-template/pull/163))
- **A taken id at registration is not a taken address.** `POST
  /auth/register` takes an optional `id`. One that was already a user's was
  refused by the create and answered like a taken address: 201
  "registered", nothing created, and "you already have an account" mailed
  to the address in the body, which had no account. It is a 409 naming the
  id, and nobody is mailed, whether or not the address is taken too: the
  answer no longer depends on which unique index the database checks first,
  so a taken id does not tell a taken address from a free one. (The id is
  the caller's to choose: the route says whether an id belongs to an
  account, never whether an address does.) The use-case also refuses a
  registration whose method it does not know.
  ([#162](https://github.com/slashdevops/go-rest-api-service-template/pull/162))
- **A project that is not there is a 404 for everybody.** The `admin` flag
  admitted its account to any project id, so under a project that did not
  exist `GET /projects/{project_id}/products`, `/users` and
  `/resources_limits` answered it 200 (the last with the project scope's
  limits, for no project), `DELETE /projects/{project_id}` and the unlink
  of its users answered 200 "deleted" and "unlinked", while a create under
  the same path answered 404. The membership statement asks whether the
  project exists in the same round trip, and a missing project is the 404 a
  non-member gets, on every route under it. The Nil project id
  (`00000000-0000-0000-0000-000000000000`) is a 400, not a 500. A refused
  non-member is no longer a WARN line of its own: the access line carries
  the refusal at INFO with its `error_type`.
  ([#161](https://github.com/slashdevops/go-rest-api-service-template/pull/161))
- **The identity providers have an HTTP client of their own, and a failed
  exchange is presented once.** They used the service's shared outbound
  client, which retries a failed request ten times with backoff inside two
  minutes: a token endpoint that refused the connection was answered 503
  after about two and a half minutes, a 5xx came back after the retries as a
  transport error, and the request retried was the token POST, whose code
  is good for one exchange: a token endpoint that answered 502 was sent the
  code 22 times and answered after almost three minutes (eleven attempts,
  twice: the OAuth library, probing where the client's credentials go,
  presented a failed exchange a second time). A provider that spends the
  code and then answers 502 answers the retry `invalid_grant`, so its
  outage was a 401 "sign-in refused". The providers' client has the same transport and
  address guard, no retries, and a new setting, `authn.idp.timeout` (env
  `AUTHN_IDP_TIMEOUT`, 10 s, between 1 s and 1 m), per request; the
  credentials go where the discovery document says
  (`client_secret_basic` when listed or when nothing is, the form when only
  `client_secret_post` is; the form for GitHub). The shared client is
  unchanged and serves the mail API sender.
  ([#160](https://github.com/slashdevops/go-rest-api-service-template/pull/160))
- **A sign-in refused at the provider is not an outage, and an outage is not
  a refused sign-in.** The identity-provider callback answered one error,
  401, for both sides of a failed sign-in, and the status table counted it a
  refusal. The caller's side is a new `IDPSignInRefusedError`, still 401: the
  person cancelled or was denied, the code is not one the provider accepts
  (`invalid_grant`, GitHub's `bad_verification_code` and
  `unverified_user_email`), the ID token answers another sign-in, the
  account has no address. The provider's side is now **503**, "sign-in with
  this identity provider is not available right now", counted as a failure:
  it rejects this service's client (`invalid_client`, an expired secret),
  answers 5xx, returns no valid ID token, does not answer, or the call for
  the account's address fails. A client that read every callback 401 as
  "sign-in refused" now sees the outage. The adapter decides from the OAuth
  error code, never from text. The providers then still shared the
  service's retrying client: a 5xx or a refused connection was answered
  after the retries, as unreachable, until
  [#160](https://github.com/slashdevops/go-rest-api-service-template/pull/160).
  A sign-in cancelled at the provider spends its state: it stayed good for a
  callback with a code for the rest of its fifteen minutes, and such a
  replay signed in. `TestIDPStateIsSingleUse` runs on the suite's own
  provider instead of exchanging against github.com.
  ([#159](https://github.com/slashdevops/go-rest-api-service-template/pull/159))
- **An address the service cannot spell is a 400, not a 500.** An address
  is checked by the handler and again by the use-case, on what the first
  check returned, and `net/mail` drops the quotes of a quoted local part:
  `"a b"@example.com` passed as `a b@example.com`, which the second check
  refused, and `Ada <a@b.c>` passed the length check as an input and failed
  it as `a@b.c`. `POST /auth/register`, `POST /auth/password/recover`,
  `POST /auth/verify`, `POST /users` and `PUT /users/{user_id}` answered
  500. `ValidateEmail` refuses an address whose spelling does not parse
  back as itself and measures the spelling's length. A first sign-in
  through a provider spells the provider's address before it looks it up:
  a claim that is no address at all was the lookup's `InvalidEmailError`, a
  500, and an existing account's address in another spelling was free at the
  check (the create refused it). The spelling holds no white space either:
  `net/mail` keeps a no-break space that the trim of the input had taken
  off, so `<ab@cd.ef\u00a0>` was spelled with it and spelled again without.
  ([#158](https://github.com/slashdevops/go-rest-api-service-template/pull/158))
- **Six refusals are no longer a 500.** Each was answered `500 internal
  server error`, an ERROR line, while its call was counted `refused`:
  - `PUT /policies/{policy_id}` to a grant no endpoint serves is a `404`,
    as the create answers it;
  - `POST /roles/{role_id}/policies` naming a policy that is not there is a
    `404`;
  - `DELETE /me/identities/{idp_id}` for an account deleted meanwhile is a
    `404`;
  - the identity-provider callback of a link whose account was deleted is
    a `404`, in a fixed sentence (the error's own text names the account's
    id, and the route is public);
  - `POST /auth/register` when the system's user limit is full is a `409`,
    "registration is closed: the user limit has been reached".

  The identity-provider callback answered that limit `409` already, but in
  the limit error's own words, which name the scope, both limits and the
  usage, to an anonymous caller: it says the registration route's sentence
  now. Found by reading every `WriteInternalError` call of the handlers
  against what its use-case returns.
  ([#157](https://github.com/slashdevops/go-rest-api-service-template/pull/157))
- **The log panels show what the service writes.** The Cache dashboard's
  "Errors from the cache layer" and the Email dashboard's "Errors from the
  mail layer" filtered on a layer no log line carries and showed nothing
  through a real outage. They read the cache client's `component=cache`
  lines and the service's invalidation and health messages, and the mail
  worker's `error sending email` and `email sending cancelled`. The cache's
  "Error rate" counts timeouts. The Layers dashboard gains "Failed calls at
  $layer", from the traces; the Logs dashboard gains "Refused requests" and
  says what its error panels count. A panic writes two ERROR lines, not
  three: `Recovery` answers the 500 without logging the panic again. The
  cache layer's `successful` follows its outcome, so a timeout is
  `successful="false"` too.
  ([#156](https://github.com/slashdevops/go-rest-api-service-template/pull/156))
- **A failed COMMIT is the answer, not a log line.** Creating an account
  or a project ended its transaction in a deferred closure that logged a
  failed COMMIT and returned nil: the route answered `201` for a row that
  was never stored. Every transaction is ended by `endTx`, which reads the
  error the method returns, and a COMMIT that fails becomes that error.
  ([#150](https://github.com/slashdevops/go-rest-api-service-template/pull/150))
- An id reaches the log store as the id. The OpenTelemetry bridge exported a
  `uuid.UUID` as an array of sixteen integers, a named string type as
  "unhandled: …" and a slice of ids as arrays of integers, so a filter on
  `user_id` in Loki matched nothing (stdout showed the id). The exported
  handler gives such values as they read; a nil pointer is left as it is.
  (#146)
- A replica that stops says so. A first load before serving that failed, or a
  failure of the start from the database on, ended the process with nothing
  in the log store, not even its start line (`log.Fatalf` wrote at INFO and
  exited before the exporter's batch was sent). It writes `application
  stopped on an error` at ERROR and flushes first, and the dashboards' "Fatal"
  annotation, on a level the service never writes, is "Stopped on an error"
  on that line. A signal while starting is INFO and exit 0. (#145)
- `/health/detailed` says what its mail probe checks: "the mail transport's
  port accepts connections at <address>; delivery is not verified", not
  "mail transport reachable". (#145)
- The administrator account can correct an account whose stored address is
  not in the service's spelling (mixed case from before that spelling): its
  form sent the address back spelled, and that was taken as no change, while
  such a row cannot sign in, since a sign-in looks the address up as the
  service spells it. For the administrator account only the same bytes are
  no change; for anybody else the spelled address still is. (#142)
- An account's standing, read on every authenticated request, is one column
  (`SelectDisabled`), not the whole row with its password hash. (#142)
- A link that names an id twice links it once: `POST /projects/{project_id}/users`,
  `/users/{user_id}/roles`, `/roles/{role_id}/users` and
  `/roles/{role_id}/policies` answered `500` for `[A, A]` (a user's projects
  already did not). (#141)
- A product's `409` and `404` are in this service's words: a name that is
  taken in the project, an id that is taken, a project that is gone. They
  ended in PostgreSQL's sentence (`duplicate key value violates unique
  constraint "unique_project_product_name"`, the foreign key's), which a
  database or driver upgrade would have rewritten. (#140)
- Deleting a user or an identity provider that does not exist is a `404`
  and refunds nothing. (#119)
- `POST /users/{user_id}/projects` with a project named twice in its body
  is a `200`, where it was a `500`. (#127)
- Starting a sign-in through an identity provider that cannot be reached
  (`GET /auth/idp/{idp_id}/login`, `/register`, `/link`) answers `503`, as
  those routes document, where it answered `500`. (#128)
- A filter or a sort field is one the list can serve. (#116, #118)
- The Rego bundle passes Regal, which `make lint` now runs. (#113, #114)
- The development stack syncs the container VM's clock, which freezes while
  a laptop sleeps. (#110)

## [0.0.3] - 2026-09-06

This file starts after this release. What it contains is in
[its release notes](https://github.com/slashdevops/go-rest-api-service-template/releases/tag/v0.0.3).

[Unreleased]: https://github.com/slashdevops/go-rest-api-service-template/compare/v0.0.4...HEAD
[0.0.4]: https://github.com/slashdevops/go-rest-api-service-template/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/slashdevops/go-rest-api-service-template/releases/tag/v0.0.3
