# Configuration rules

How a setting is declared, named, defaulted and validated, and the mistakes
each rule was written against. The long form of the "Configuration" bullets in
`CLAUDE.md`; the operator's view is in
[`docs/operations/running-the-service.md`](../operations/running-the-service.md).


**Every setting's env var is the flag with dots replaced by underscores,
uppercased.** `http.server.*` used to be the one exception, mapping to
`SERVER_*` while its sibling `http.client.*` mapped to `HTTP_CLIENT_*`; it is
`HTTP_SERVER_*` now. Keep the rule mechanical — an operator should never have to
look one up.

**The two token lifetimes are not settings.** `authn.access.token.duration`
and `authn.refresh.token.duration` were removed on 2026-09-05; the values are
one database row, seeded by migration `00016` with the same defaults (5m /
24h), read into a per-replica mirror, and edited through
`GET`/`PUT /auth/token_lifetimes`. The only flag left is
`authn.token.lifetimes.reload.interval`. Do not add a fallback constant in Go:
a replica that cannot read the row refuses to start, which is the same
invariant the rate limiter keeps. The full design, including why
`revoked_tokens` carries a `token_type` column for it, is in
[`docs/architecture/token-lifetimes.md`](../architecture/token-lifetimes.md).

**A switch is `.enabled`, never `.enable`.** A wrong guess is not a warning, it
is a stopped process: `flag provided but not defined: -database.migration.enabled`.

**A default is written twice** — in `NewField` (the env path) and in
`setupFlags` (the flag path) — and nothing connects them.
`TestNoTwoSettingsShareADefaultConstant` catches the mistake that follows:
borrowing a neighbouring field's constant, so the same setting answers
differently depending on how it was set. That had already happened to
`http.client.max.idle.conns.per.host`, invisibly, because both constants were
`100`.

**Validation must key on the setting that selects the behaviour.**
`MailConfig.Validate` used to require "either `mail.smtp.host` or
`mail.api.url`", which accepted a configuration the service could not run:
setting only the API URL passed, and startup then failed with `SMTPHost must be
between 1 and 255 characters` — an error naming a setting the operator
deliberately did not set. It keys on `mail.sender` now, so the message names the
setting to fix.

**A mail TRANSPORT belongs in `slashdevops/mailer`, not here.** `MailerMailgun`
was added there (v1.1.0) rather than in `notifieremail`, for the same reason the
rate limiter has one source of budgets: a second copy is a second thing to keep
in step.

**`log.level` accepts `ctrace` and `cfatal`** beyond the four it advertises, and
**`.air.toml` runs on `ctrace`**. Both are named in `ValidLogLevelHidden` and in
the flag help, so the dev stack no longer uses a value the help calls invalid.


## Every config Field needs a flag registered by hand

`internal/config` declares a setting as a `Field` carrying its flag name and env
var name, but `setupFlags` in `internal/app/configs.go` registers the flag **one
line per setting**. Add a `Field` and forget the line and the setting works
through the environment while *stopping the process* when passed as a flag:
`flag provided but not defined: -authn.refresh.token.rotation.enabled`.

Six settings had drifted this way, including the switch that turns refresh-token
rotation off — the one an operator reaches for in a hurry, documented as
available, and guaranteed to refuse to start.
`TestEveryConfigFieldHasAFlag` walks the config by reflection and now fails on
any new gap; it recognises a `Field` structurally, so a new config group is
covered without being told about it.

When a caller genuinely needs to tell "expired, refresh" from "revoked, sign in",
design a deliberate signal — do not reach back for a library string.

