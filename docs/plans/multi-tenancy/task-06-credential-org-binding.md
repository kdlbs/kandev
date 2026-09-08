---
id: "06-credential-org-binding"
title: "Self-authenticating credential org binding"
status: todo
wave: 2
depends_on: ["04-service-layer-org-scoping"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 06: Self-Authenticating Credential Org Binding

Every credential that authenticates *itself* rather than a session bypasses the
middleware, so each one needs its own org binding.

## Acceptance

- The GitHub credential-broker lease (`/api/v1/github/credentials/resolve`)
  carries the issuing org, and redemption against a task in another org fails
  with no token returned.
- The port-proxy capability HMAC binds the org alongside session and port;
  a capability minted in org A is rejected for org B's session even when the
  session:port pair matches.
- Automation webhook secrets, office channel HMACs, and plugin webhook secrets
  each resolve to exactly one org; a delivery is processed only in that org's
  scope.
- The public-endpoint allowlist in `internal/auth/httpmw/middleware.go` is
  re-pinned: each public route is annotated with which self-authenticating
  credential carries its org, and the pinning test fails when a route is added
  without one.
- Task shares and public share links resolve to their org and do not expose
  another org's artifact.

## Verification

- `go test ./internal/githubauth/... ./internal/github/... ./internal/gateway/websocket/... ./internal/auth/httpmw/...`
- `go test ./internal/... -run 'TestCrossOrgRedemption'`

## Files Likely Touched

- `apps/backend/internal/githubauth/`, `internal/github/` broker lease store
- `apps/backend/internal/gateway/websocket/access.go`, port-proxy handler
- `apps/backend/internal/automation/`, `internal/office/channels/`, `internal/plugins/`
- `apps/backend/internal/auth/httpmw/middleware.go`
- `apps/backend/internal/task/` share links

## Inputs

- Spec: API surface (changed contracts), Scenarios (cross-org lease, capability,
  webhook secret).
- Patterns: the port-proxy capability design and the broker-lease "opaque,
  task-scoped, hashed at rest, scope-matched on redeem" contract in
  `apps/backend/AGENTS.md` and `docs/specs/auth/spec.md`.

## Output Contract

List every self-authenticating credential, its org-binding mechanism, and its
cross-org refusal test. Report RED/GREEN commands and set this task plus its
plan checkbox to done.
