# OPA Authorization

This is the OPA policy used in the application.

## Prerequisites

- [opa](https://www.openpolicyagent.org/docs/latest/#running-opa)

## Validation

```bash
opa check --strict bundle/authorization
```

## Testing

```sh
opa test -v .
```

## Linting

```sh
regal lint bundle/authorization
```

[Regal](https://www.openpolicyagent.org/projects/regal) is the Rego linter;
`make tools` installs it. The bundle has no violations and `make lint` keeps
it so. If a rule has to be turned off, it is turned off in
`.regal/config.yaml` with the reason beside it, never by rewording the policy
around the linter.

Or you can run the following command to test the policy:

```sh
opa eval -i input.json -d data.json -d bundle/authorization/policy.rego "data.authorization.allow"
```
