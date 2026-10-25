---
id: "10-cloud-dc-domain-auth"
title: "Cloud/Data Center domain and authentication"
status: completed
wave: 3
depends_on: ["01-design-package", "02-plugin-repository-bootstrap", "03-protocol-manifest-actions"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 10: Cloud/Data Center domain, HTTP clients, and authentication

## Intent

Implement the plugin-owned common Bitbucket domain and separate Cloud/Data Center
adapters with secure workspace-scoped connection/auth lifecycle.

## Owned paths

- Attached `kdlbs/kandev-plugin-bitbucket` worktree: domain models, Cloud adapter, Data
  Center adapter, HTTP clients, encrypted secret/state store, OAuth/token flows, health
  probe, redaction, and their fixture tests.

## Dependencies

Tasks 01, 02, and 03.

## Acceptance

1. Cloud uses `https://api.bitbucket.org/2.0` cursor pagination and API-token/OAuth
   authentication; Data Center normalizes configured API/clone bases and supports
   `start`/`limit`, PAT/HTTP access token, and OAuth when application link permits.
2. Adapters map into one domain with explicit capabilities; UI/business code has no
   scattered Cloud/DC checks. Unsupported Data Center versions/capabilities are
   visible, not implied supported.
3. Secrets use workspace/generation encryption, PKCE/signed one-time OAuth state,
   rotation singleflight, redaction, HTTPS/redirect/timeout/response-cap policy, and
   ~90-second health polling with jitter/backoff.

## Verification

```sh
make test
make fmt-check
make vet
make build
```

## Risks

Data Center context paths, private networks, proxies, and supported releases vary.
Never store URL credentials or ship OAuth client secrets; test capability probes and
origin-locked redirect behavior with fixtures.
