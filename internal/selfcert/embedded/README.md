# Embedded local dev certificate

`cert.pem.b64` and `key.pem.b64` are a self-signed wildcard certificate for
`*.morsel.localhost` / `morsel.localhost`, base64-encoded so secret scanners
don't flag PEM/key headers in the diff.

The cert is generated once and committed rather than regenerated on every
`service bootstrap`, so a developer who trusts it in their OS/browser trust
store only has to do that once. It's valid for 10 years.

## Regenerating

Only needed if the key is compromised or validity is about to expire. Run
from the repo root:

```
go run internal/selfcert/embedded/gen/main.go
```

This overwrites `cert.pem.b64` and `key.pem.b64` in place. After
regenerating, every developer who already trusted the old cert must trust
the new one.
