Up: [Index](README.md) · Prev: [Approvals](approvals.md) · Next: [Persistence](persistence.md)

---

# Platform Feature — Networking

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel manages DNS records and TLS certificates automatically. Developers declare whether an app is public or private in `morsel.json`. The platform provisions a subdomain, obtains a certificate, and configures routing without any developer or operator action beyond the initial platform setup.

---

## URL Assignment

Every HTTP app gets a stable subdomain derived from its name and repo. The URL never changes across deploys, hibernation, or platform upgrades.

```
Named app:    https://{app-name}.{repo-slug}.apps.example.com
Unnamed app:  https://{repo-slug}.apps.example.com
```

The base domain (`apps.example.com`) is set at bootstrap and is immutable after provisioning.

Where `repo-slug` is the GitHub repository name without the org prefix — `org/my-repo` becomes `my-repo`. Examples:

```
org/my-repo  +  name: "api"    →  https://api.my-repo.apps.example.com
org/my-repo  +  (unnamed)      →  https://my-repo.apps.example.com
```

In local mode the base domain is `morsel.localhost`:

```
Named app:    https://{app-name}.{repo-slug}.morsel.localhost
Unnamed app:  https://{repo-slug}.morsel.localhost
```

Private apps (`private: true`) use the same URL structure but resolve only from within the VPC.

Worker and CronJob apps do not get URLs — they have no inbound HTTP interface.

---

## Public vs Private Apps

The `private` field in `morsel.json` controls ingress:

```json
{ "private": false }   // reachable from the public internet (default)
{ "private": true }    // reachable only from within the VPC
```

Public apps are exposed via the platform gateway's external load balancer. Private apps are exposed via an internal load balancer, reachable only from within the cluster VPC. Since all Morsel apps run within the same VPC, private apps act as internal services accessible to other Morsel apps but not to external traffic.

The `private` field can be changed freely between deploys — no approval required. Morsel updates the gateway routing and load balancer configuration on the next deploy.

---

## DNS Management

On each app deploy, Morsel creates a DNS record pointing the app's subdomain to the appropriate load balancer IP. On app deletion, the record is removed.

Morsel supports two DNS providers, selected at bootstrap:

### Cloud DNS

DNS records are managed within the Morsel cloud project. The Morsel API authenticates via its ambient cloud identity — no credential files required. All DNS operations stay within the cloud project boundary. See [platform/gcp.md](../platform/gcp.md) for GCP-specific details.

The operator must create the DNS zone for the base domain before running bootstrap. Morsel manages all records within that zone but does not create the zone itself.

### Cloudflare

The Morsel API holds a Cloudflare API token stored in the platform secret store. The token is scoped to edit a single Cloudflare zone — no other permissions. The bootstrap wizard generates token scope instructions and validates the token before provisioning.

This is the one case where a Morsel secret can modify resources outside the cloud project boundary. The token scope minimises the blast radius: it can only modify DNS records for the configured zone.

DNS provider selection is immutable after bootstrap. Changing providers requires a full platform rebuild.

---

## TLS Certificate Management

Morsel provisions and renews TLS certificates automatically using the ACME protocol against Let's Encrypt. No manual certificate management is required by the operator.

### Provisioning

When a new app is deployed:
1. Morsel calls the ACME API to request a certificate for the app's subdomain
2. ACME issues a DNS-01 challenge — Morsel creates a `_acme-challenge` TXT record via the DNS provider
3. Let's Encrypt validates the record
4. Certificate is issued and stored in a Kubernetes Secret in the app's namespace
5. Platform gateway is configured to use the certificate for TLS termination

Certificate provisioning happens asynchronously as part of the deploy operation. The deploy completes when the certificate is issued. Typical time: under 2 minutes.

### Renewal

The Morsel API background process checks certificate expiry daily. Certificates are renewed 30 days before expiry. Renewal follows the same ACME flow as provisioning. The operator does not need to intervene.

### Failure Alerting

If certificate provisioning or renewal fails, the operator is alerted via the platform status endpoint and the admin UI. The app continues to run but may show certificate errors if the certificate expires before renewal succeeds.

```json
"certs": {
  "expiring_soon": [],
  "failed": [
    { "app": "my-app", "repo": "org/my-repo", "error": "ACME challenge failed" }
  ]
}
```

---

## Platform Gateway API

Morsel uses the Kubernetes Gateway API for all ingress routing. The Morsel API manages Gateway resources directly via `client-go` — developers and operators never interact with Gateway objects.

Each HTTP app gets a dedicated `HTTPRoute` resource that routes traffic from its subdomain to the app's Kubernetes Service. Private apps use an internal Gateway class; public apps use the external Gateway class.

On hibernation, the `HTTPRoute` is updated to route to the wake-on-request proxy. On wake, it is updated back to the app's Service.

---

## Platform Internal Networking

All traffic between the Kubernetes cluster and platform-managed services (container registry, object storage, DNS, secret store) is routed via the platform's internal network — not the public internet. No platform API endpoint is reachable from the internet. See [platform/gcp.md](../platform/gcp.md) for GCP-specific details (Private Google Access).

GitHub Actions workflows run on GitHub-hosted runners and make outbound connections to the platform over the internet (OIDC exchange and staging registry push). This is accepted: the connections are outbound-only, TLS-secured, and authenticated by short-lived cryptographically signed tokens.

---

## App-to-App Communication

Apps communicate with each other via their public HTTPS URLs. There is no private inter-app channel outside of the VPC-internal routing available to private apps.

A hibernated app wakes transparently on an inbound request from another app, subject to normal cold-start latency. There is no special handling required — the calling app makes an HTTP request and the wake-on-request proxy handles the rest.

---

## Local Mode

In local mode, apps use the `*.morsel.localhost` domain, which resolves natively in modern browsers without any DNS configuration. Certificates are self-signed, generated at bootstrap time. No ACME flow, no DNS provider required.

See [platform/local.md](../platform/local.md).

---

## Component Contributions

### Morsel API
Owns DNS record management, ACME certificate provisioning and renewal, Gateway API resource management, and wake-on-request proxy routing. See [components/morsel-api.md — Networking](../components/morsel-api.md).

### CLI
Provisions the platform gateway classes and configures the DNS provider connection at bootstrap time. See [components/cli.md — Networking](../components/cli.md).

### Platform
Provides the DNS provider implementation and internal network configuration. See [platform/gcp.md](../platform/gcp.md) for GCP specifics.

---

Up: [Index](README.md) · Prev: [Approvals](approvals.md) · Next: [Persistence](persistence.md)
