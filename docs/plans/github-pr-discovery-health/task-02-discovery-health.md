---
id: "02-discovery-health"
title: "Preserve discovery failure evidence"
status: done
wave: 2
depends_on:
  - "01-valid-pr-discovery"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.1
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.2
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.3
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.4
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.5
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.6
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.7
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 02: Preserve discovery failure evidence

## Summary

Expose discovery failures independently of quota observations. Share retry and
recovery state across background and user-triggered discovery of the same target.

## In scope

- Start with `TestPRDiscoveryHealth`: fail discovery, refresh quota to full,
  assert degradation and deadline remain, then clear only through newer target
  success. Use deterministic time and real service admission with fake transport.
- Implement the design's target key, coalescing, classified errors, bounded
  retention, partial-batch accounting, and retry rules. Cover CLI, token clients,
  and HTTP-200 GraphQL errors without selecting different credentials.
- Add mixed-target, out-of-order completion, repeated rejected-batch, stale
  generation, workspace deletion, and credential replacement tests. Assert zero
  repeated transport calls before deadlines and one coalesced due attempt.
- Add status/event health projection and revision ordering. Update Go and
  frontend wire types together, with projection and stale-event regression tests.
- Extend frontend status/store/event paths and the settings/disclosure warning.
  Add locale keys in all five languages; generate Traditional Chinese variants.
- Extend the existing desktop/mobile settings E2E fixture sequence to cover
  retained errors after quota refresh and actual recovery. Capture both views.
- Update `docs/public/integrations.md` with concise troubleshooting guidance
  distinguishing quota observations, request failures, and verified recovery.

## Out of scope

No new generic integration-health framework, new SQL tables, credential changes,
PR chat parsing, live task writes, or frontend sync-resource redesign.

## Acceptance

1. Full quota, successful auth, old HTTP responses, and unrelated successful
   targets cannot clear a current discovery failure.
2. All discovery entry points honor the design's shared deadline and attempt
   generation. Invalid/throttled batches do not immediately fan out retries.
3. Desktop and phone show persistent localized error details, preserve content
   during refresh, and clear the warning only after the corresponding recovery.

## ASCII UI preview

UI-01, excerpt from the [full preview](plan.md#ascii-ui-preview):

```text
Desktop: identity [limits] [refresh]
         ! PR discovery failed. Last failure: 12:34
         hover/focus limits -> failure, then reported quotas

Phone:   identity
         ! PR discovery failed.
           Last failure: 12:34
         [limits] [refresh]
         tap limits -> bottom drawer
           failure / retry time
           reported quotas
```

Covers health ACs .1-.3, .5-.6. Reuse `GitHubAccessHelp` and its `useTouchDrawer`
branch. The drawer is temporary detail, not navigation. Keep one internal scroll
owner for long content, safe areas, focus return, 44px touch hit areas, and no
horizontal page overflow. Copy and spacing are illustrative; structure is required.
Refreshing keeps existing content visible. Unknown health is not success.

## Verification

Run from repository root. Install dependencies once if the worktree lacks them.
Use the TDD and E2E skills for test mechanics. Update the corresponding Go and
frontend wire definitions before typecheck when wire types change.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/github -run 'TestPRDiscoveryHealth' -count=1)
(cd apps/backend && go test -race ./internal/github -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/github/github-slice.test.ts lib/ws/handlers/github.test.ts components/github/github-rate-limit.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project=chromium e2e/tests/integrations/github-workspace-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/integrations/mobile-github-workspace-settings.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/service_pr_discovery_health.go` (new)
- `apps/backend/internal/github/service_pr_discovery_health_test.go` (new)
- `apps/backend/internal/github/service.go`, `graphql.go`, `gh_client.go`
- `apps/backend/internal/github/service_pr_watch.go`, `service_pr_watch_batched.go`, `poller.go`
- `apps/backend/internal/github/ratelimit.go`, `rate_limit_fetch.go`, `controller.go`
- `apps/backend/internal/github/service_app_auth.go` and its tests (workspace status projection)
- `apps/web/lib/types/github.ts`, `apps/web/lib/types/backend.ts`, and shared Go event types
- `apps/web/lib/state/slices/github/github-slice.ts` and its test
- `apps/web/lib/ws/handlers/github.ts` and its test
- `apps/web/hooks/domains/github/use-github-status.ts`
- `apps/web/components/github/github-status.tsx`, `github-rate-limit.tsx`, `github-rate-limit.test.tsx` (new test)
- `apps/web/components/github/github-access-help.tsx` if content requires bounded scrolling
- `apps/web/src/locales/` catalogs
- `apps/web/e2e/tests/integrations/github-rate-limit-fixture.ts`
- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`
- `docs/public/integrations.md`

## Dependencies

Task 01. Shared GraphQL fields and transport coverage must pass first.

## Risks

Do not equate a successful quota refresh with successful PR discovery. Do not
hold a mutex during provider I/O. Bound target state and do not expose secrets
or a different workspace's repository identities. If evidence identifies the
brief red text as another failure, report it separately rather than expanding
this work order without a revised scope.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-discovery-health.md), health criteria.
- [Design](../../specs/integrations/system-design/github-pr-discovery-health.md), failure state through presentation.
- Existing `RateTracker`, `GitHubAccessHelp`, workspace settings tests, and task sync coordination.

## Results

- Added credential-scoped PR discovery health with bounded target retention,
  classified GraphQL/rate-limit/unavailable failures, monotonic revisions,
  coalesced admission, retry deadlines, and stale-completion protection.
- Wired health through workspace status and workspace-scoped WebSocket events;
  quota snapshots remain independent. Invalid and throttled batches do not fan
  out the same request, while other provider failures retain the existing
  per-watch fallback.
- Added localized desktop and phone warnings, retained warning details in the
  quota disclosure, bounded mobile drawer scrolling, and safe-area spacing.
- Added store and WebSocket ordering/isolation tests, focused UI tests, and
  desktop/mobile E2E fixtures for failure, full quota refresh, and recovery.
- Added public integration troubleshooting guidance and synchronized all five
  locale catalogs.
- Review remediation now groups duplicate immutable targets behind one
  provider attempt and fans persistence and feedback results to every watch.
  Immutable attempt tokens survive fork-to-upstream rebinding, live consumer
  pruning handles shared, archived, deleted, and branch-switched watches, and
  active targets are never evicted for capacity.
- Runtime epochs reject delayed events from an older backend incarnation, while
  disconnected status clears stale connection health. HTTP Retry-After/reset
  headers and HTTP-200 GraphQL rate-limit resetAt values now reach discovery
  admission instead of being replaced by synthetic deadlines.
- Verification passed: backend normal and race suites (1,769 tests each),
  backend/frontend lint, frontend typecheck, 33 focused frontend tests, i18n
  gates, backend and Vite builds, 5 Chromium E2E tests, 3 mobile Chrome E2E
  tests, public-doc validators, specification lint, and `git diff --check`.
