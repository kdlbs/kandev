---
id: "01-capability-and-broker"
title: "Add credential reissue capability"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/git-credential-lease-reissue.md"
---

# Task 01: Add credential reissue capability

- **Acceptance:** Capability claims have an exact scope and expiry, reject
  forged/expired/mismatched requests, and reissue only through live broker
  authorization and binding validation.
- **Acceptance:** The HTTP reissue endpoint returns only a lease and remains
  self-authenticating under the existing public-route middleware boundary.
- **Verification:** `cd apps/backend && go test -count=1 ./internal/gitcredentials ./internal/github ./internal/backendapp`

Likely files: `internal/gitcredentials`, `internal/github`,
`internal/backendapp`, and `internal/auth/httpmw`.

Risk: no old lease may be treated as reissue authority.
