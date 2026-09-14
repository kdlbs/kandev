---
id: "01-hydrate-shared-status"
title: "Hydrate shared task executor status"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 01: Hydrate shared task executor status

## Acceptance

- A valid rendered remote-executor task scope begins its authoritative status
  read before hover; Kubernetes uses the exact executor/task/session REST path
  and compatible executors retain `task.session.status`.
- Exact duplicate consumers and explicit refreshes join one in-flight promise.
  Successful results are reusable for 90 seconds, failures remain visible but
  retryable, and the cache has bounded LRU growth.
- Scope changes and missing identities cannot publish stale data, issue malformed
  requests, or synthesize a ready state. The external `status` prop remains an
  authoritative no-fetch path.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/remote-executor-status-resource.test.ts components/task/remote-cloud-tooltip.test.tsx && cd web && pnpm run typecheck && cd ../.. && git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/session/remote-executor-status-resource.ts` (new)
- `apps/web/hooks/domains/session/remote-executor-status-resource.test.ts` (new)
- `apps/web/hooks/domains/session/use-remote-executor-status.ts` (new)
- `apps/web/components/task/remote-cloud-tooltip.tsx`
- `apps/web/components/task/remote-cloud-tooltip.test.tsx`
- `apps/web/lib/api/domains/kubernetes-api.ts` (only if an AbortSignal needs to be threaded through the existing options argument)

## Dependencies

None.

## Parallelism

`sequential`. Task 02 consumes the resource snapshot and refresh contract.

## Inputs

- Spec: eager indicator hydration, exact-scope deduplication, invalid-identity,
  and stale-response scenarios.
- Plan: Eager, shared exact-status resource.
- Existing patterns: `pr-commits-resource.ts`, `usePRCommits`,
  `getKubernetesTaskSession`, `getWebSocketClient().request`, and
  `useSyncExternalStore`.

## Output contract

Record the RED first-hover assertion, GREEN cache/deduplication/freshness
commands, request counts for Kubernetes and compatibility sources, files
changed, blockers/risks, and synchronize this task plus `plan.md`.

## Results

- RED proved a mounted valid scope issued zero requests before hover, while two
  exact duplicate consumers issued two requests. Separate REDs covered the
  90-second boundary, retryable thrown and sanitized failures, malformed scope
  guards, and the 128-entry inactive LRU bound.
- GREEN introduced `remote-executor-status-resource.ts` and
  `use-remote-executor-status.ts`. Kubernetes uses the exact REST inventory
  read; compatible remote executors retain `task.session.status`; concurrent
  consumers and forced refreshes join one promise.
- A resolved payload containing `remote_status_error` is classified as failed,
  remains visible, and retries on the next load instead of being cached as a
  successful result.
- Focused final Vitest contributed to the 5-file, 49-test passing suite.
  Typecheck and full zero-warning lint passed. No blocker remains.
