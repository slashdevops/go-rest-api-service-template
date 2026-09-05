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

Or you can run the following command to test the policy:

```sh
opa eval -i input.json -d data.json -d bundle/authorization/policy.rego "data.authorization.allow"
```
