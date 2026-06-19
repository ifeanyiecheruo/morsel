# Feature 09 — Networking

_Delivers: HTTP apps get stable URLs with TLS; HTTPS works in-browser._

**Direct dependencies:** [F06](006-feature-kubernetes-manifest-apply.md), [F07](007-feature-bootstrap-local-platform.md)

> Can be developed in parallel with F08, F10, F11.

## Tasks

- [ ] Gateway class + Gateway resource provisioned during `LocalPlatform.Bootstrap().Provision()`
- [ ] `HTTPRoute` apply on deploy — route subdomain to app Service
- [ ] `HTTPRoute` routing decision — external Gateway class for `private: false`; internal for `private: true`
- [ ] `LocalPlatform.DNS()` — no-op; `*.morsel.localhost` resolves natively in modern browsers
- [ ] `LocalPlatform.Certs()` — generate self-signed wildcard cert for `*.morsel.localhost` at bootstrap; store in K8s Secret
- [ ] Cert storage helper — write `*tls.Certificate` to K8s Secret in app namespace
- [ ] Certificate renewal background goroutine — check expiry daily; renew 30 days before expiry
- [ ] Certificate alert in `GET /api/operator/status` — expiring soon, failed
- [ ] `HTTPRoute` delete on app deletion
