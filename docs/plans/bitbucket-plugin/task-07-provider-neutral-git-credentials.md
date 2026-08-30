---
id: "07-provider-neutral-git-credentials"
title: "Provider-neutral Git credential broker"
status: completed
wave: 2
depends_on: ["01-design-package", "03-protocol-manifest-actions"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 07: Provider-neutral Git credential broker

## Intent

Extract generic broker lease mechanics, retain GitHub via adapter, and route plugin
credential resolution through scoped opaque helper leases for clone/fetch/push.

## Owned paths

- `apps/backend/internal/gitcredentials/`
- `apps/backend/internal/github/credential_broker.go`
- `apps/backend/internal/repoclone/clone.go`
- `apps/backend/internal/repoclone/protocol.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_github_env.go`
- `apps/backend/internal/agent/runtime/lifecycle/broker_reachability.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- Focused `internal/gitcredentials`, clone, executor, and lifecycle tests.

## Dependencies

Tasks 01 and 03.

## Acceptance

1. Composite resolver supports arbitrary manifest-declared provider IDs and optional
   plugin credential RPCs while retaining current GitHub behavior unchanged.
2. Leases bind exact workspace/task/session/repository/host/path, expire/revoke on all
   required lifecycle changes, re-resolve after OAuth refresh, and use HTTPS for remote
   executor broker transport.
3. Clone/fetch/push preserve `RepositoryCloneURL` precedence and leave no credential in
   URLs, task state, logs, environment, command arguments, or executor payloads.
4. Initial host clone requests carry exact task, active-session, and persisted-repository
   scope through the orchestrator and workspace-source materializer. Plugin providers
   reject missing scope; GitHub legacy behavior remains unchanged.
5. Pre-worktree refresh for plugin providers uses the same exact scope, is strict, and
   suppresses every later unauthenticated fetch/pull while consuming refreshed refs.

## Verification

```sh
cd apps/backend && go test ./internal/gitcredentials ./internal/github ./internal/repoclone ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle
cd apps/backend && go test ./internal/backendapp ./internal/orchestrator/executor ./internal/repoclone
make -C apps/backend lint
```

## Risks

Credential leakage and scope confusion are security failures. Treat host/path mismatch,
disabled plugin, expired lease, and unsupported resolver as deny-by-default.

## Completed verification (2026-08-06)

- Focused clone, worktree, executor, lifecycle, and backend-app suites passed, including
  strict exact-scope refresh and no-network-after-refresh regressions.
- `make -C apps/backend lint` passed (with only the known stale external-worktree filter
  warning).
- A live Bitbucket Cloud task prepared in 16 ms with only `git checkout`, performed no
  second unauthenticated fetch, launched Codex, and reached `WAITING_FOR_INPUT` with `OK`.
