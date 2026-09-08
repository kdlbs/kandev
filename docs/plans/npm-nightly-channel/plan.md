---
spec: docs/specs/release/requirements/npm-nightly-channel.md
created: 2026-07-31
status: implemented
---

# Implementation Plan: npm nightly channel

## Overview

First make versioning and npm publication deterministic and testable, then add the scheduled branch
without allowing it into stable release jobs. Next add channel-aware backend discovery and exact
apply semantics, then the responsive setting, E2E proof, and public operating docs.

## Release automation

### Version and publisher

`scripts/release/nightly-version.mjs` validates a stable baseline plus full SHA and emits the next
patch prerelease. `scripts/release/publish-npm.sh` accepts explicit version, dist-tag, and either a
GitHub release tag or local assets. It sets the runner checkout's launcher version, pins all five
optional dependencies, publishes runtimes first, and publishes the launcher last.

### Workflow

`.github/workflows/release.yml` gains cron `0 12 * * *`, a Stable/Nightly manual channel selector,
a read-only nightly metadata job, shared web/runtime builds, and an OIDC npm publish job. Manual
Nightly runs are limited to `main`; their dry-run mode performs preflight without building or
publishing. All stable-only jobs require the Stable channel. Non-cancelling concurrency serializes
complete Stable and Nightly runs so a pending Stable tag cannot race npm, and Nightly rechecks the
stable Git/npm baseline plus the starting `nightly` tag before publication.

## Backend

`apps/backend/internal/system/updates/channel.go` owns typed channel validation, the install-wide
`updates_channel` setting, and capability checks. `npm.go` resolves and validates the public npm
metadata. `Service` selects a resolver and isolated cache for `Get`, `Check`, poller, notification,
and apply paths. `PATCH /updates/channel` is mounted on the existing admin group.

`apps/backend/internal/persistence/meta.go` retains stable keys and adds explicit nightly keys.
Channel-aware availability treats npm's dist-tag as authoritative between nightlies while keeping
ordinary stable comparisons and avoiding downgrade notifications.

## Frontend

`apps/web/lib/types/system.ts`, `system-api.ts`, and `use-updates.ts` carry the channel contract and
save mutation. `updates-card.tsx` registers a settings-save contributor and renders full-row Stable
and Nightly choices with server-owned capability reasons.

### Mobile design contract

The entry point remains the direct Settings > System > Updates route. The selector is a short,
one-dimensional setting, so both desktop and phone use an inline card rather than a drawer. The
page remains the single scroll owner. Rows follow
`mcp-task-agent-profile-default-settings.tsx`, use at least 44px height, wrap the immutable version,
and expose no hover-only action. Shared hook/state logic owns both viewports. A `mobile-chrome`
Pixel 5 test proves select, save, reload, and zero horizontal overflow.

## Tests

- **Version identity:** valid/invalid baselines, numeric SHA prefixes, and deterministic repeat.
  **File:** `scripts/release/nightly-version.test.mjs`. **How:** Node unit tests.
- **Publisher/workflow contract:** source exclusivity, exact package order, dist-tags, schedule,
  event gates, and no stable side effects. **File:** `apps/cli/src/release-config.test.ts` and
  `.github/scripts/release-workflow-contract_test.py`. **How:** fixture/static contract tests.
- **Registry resolver:** valid tag, missing tag/version, malformed SemVer, HTTP failures.
  **File:** `apps/backend/internal/system/updates/npm_test.go`. **How:** `httptest.Server`.
- **Persistence and service:** default/invalid channel, isolated cache, restart, comparisons,
  polling, notification, and apply exactness. **Files:** update and persistence `*_test.go`.
  **How:** real in-memory SQLite plus injected resolvers.
- **HTTP:** admin PATCH success, invalid `400`, unsupported `409`, and resolver failure.
  **File:** `apps/backend/internal/system/updates/handler_test.go`. **How:** Gin integration tests.
- **Frontend contract:** request shape, draft/save/discard, disabled reason, and exact target.
  **Files:** `system-api.test.ts`, `updates-card.test.tsx`. **How:** Vitest with complete response
  fixtures.

## E2E Tests

- **Scenario:** a managed npm service selects Nightly, saves, reloads, and applies the resolved exact
  version. **File:** `apps/web/e2e/tests/system/updates-channel.spec.ts`.
- **Scenario:** the same selection and persistence work at Pixel 5 width with 44px rows and no
  horizontal overflow. **File:** `apps/web/e2e/tests/system/mobile-updates-page.spec.ts`.
- **Scenario:** an unsupported install stays Stable and explains why. **Files:** both specs where
  the fixture can expose the install kind.

## Implementation waves

Wave 1:

- [x] [Task 01 — Version and npm publisher](task-01-version-and-publisher.md)

Wave 2:

- [x] [Task 02 — Scheduled release workflow](task-02-release-workflow.md)
- [x] [Task 03 — Backend channel foundation](task-03-backend-channel-foundation.md)

Wave 3:

- [x] [Task 04 — Backend API and apply semantics](task-04-backend-api-apply.md)

Wave 4:

- [x] [Task 05 — Responsive Updates setting](task-05-updates-channel-ui.md)

Wave 5:

- [x] [Task 06 — Desktop and mobile E2E](task-06-updates-channel-e2e.md)

Wave 6:

- [x] [Task 07 — Public docs and final records](task-07-docs-and-records.md)
