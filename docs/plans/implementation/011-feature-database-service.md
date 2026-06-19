# Feature 11 — Database Service

_Delivers: apps connect to Postgres via `database.morsel.internal` with no credential management._

**Direct dependencies:** [F06](006-feature-kubernetes-manifest-apply.md), [F07](007-feature-bootstrap-local-platform.md)

> Can be developed in parallel with F08, F09, F10. F12 (Queue Service) blocks on this feature for the shared Postgres StatefulSet.

## Tasks

- [ ] Shared Postgres `StatefulSet` provisioned in `morsel-services` namespace during bootstrap
- [ ] Per-app database + user provisioning on first deploy with `persistence.database` declared
- [ ] `GRANT ALL` scoped to own database only
- [ ] Real credentials stored in K8s Secret in app namespace
- [ ] PGBouncer sidecar injection — add PGBouncer container to app pod spec; configure `morsel/morsel/morsel` → real credentials mapping
- [ ] `/etc/hosts` injection so `database.morsel.internal` resolves to `127.0.0.1` in app pod
- [ ] PGBouncer sidecar removal on hibernation (scale to 0 removes the pod + sidecar)
- [ ] Idempotent re-provisioning — re-deploy with same declaration does nothing
