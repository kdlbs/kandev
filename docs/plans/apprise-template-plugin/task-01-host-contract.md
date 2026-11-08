---
id: 01-host-contract
title: Generic host formatter contract
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-001
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-001.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.6
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.5
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
---

# Task 01: Generic host formatter contract

## Summary

Add the opt-in formatter declaration, optional RPC, owner-authorized provider
binding/action scope, and generic editor mount. Preserve the existing sender
and delivery admission. This becomes the Kandev upstream PR after discussion,
implementation, and real verification.

## In scope

- Reconcile the proposed contract against freshly fetched upstream main, record
  the SHA, and publish one immutable contract snapshot for Task 02.
- SDK/proto/manifest/runtime glue, generic binding storage/revisions, provider
  ownership and installation-generation checks, deadline/output validation.
- Separate owner-authorized preparation from active-binding management. Persist
  the selected immutable configuration revision; verify prepared receipts and
  CAS binding activation without holding SQL transactions over plugin RPC.
- Share a configuration-capture gate across binding and formatter management
  mutations so backup snapshots can resolve every selected revision coherently.
- Ordinary, inbox, and claimed runtime-batch integration; non-selected providers
  and fixed provider-test sends remain on their existing paths.
- Fixture plugin, generic selector/editor mount, ownership/conflict/lifecycle
  tests, required-store/SQLite/PostgreSQL migration coverage, author docs.

## Out of scope

Plugin product parser, template schema/editor, legacy import, and deployment.

## Acceptance

1. Optional extension leaves old SDK implementations and unbound providers
   working; invalid/unavailable formatter results use one whole default pair.
2. Owner/type/revision/installation checks protect both binding and management,
   including deletion, type change, disable, upgrade, and reinstall races.
3. The generic fixture exercises UI-01 without adding Apprise product behavior
   to core; public contract docs and focused tests match the implemented symbols.

## ASCII UI preview

UI-01 excerpt from [the plan](plan.md#ascii-ui-preview), covering
AC-PLATFORM-NOTIFICATION-FORMATTER-002.1 and .4:

```text
Notifications > Provider
Formatter [Default / Compatible formatter v]
Status: Ready / Unsupported host / Plugin unavailable
[Selected formatter's owner-bound editor mount]
State: Active / Saved but not applied / Conflict
First import: [Prepare] [Preview] [Activate prepared revision]
[Reset] [Save]
```

Use the plan's shared single-column phone composition, wrapping status, 44px
targets, accessible errors, and retained drafts. Fixture UI proves the mount;
Task 02 owns the actual product editor.

## Verification

Run from the Kandev task worktree root. New tests named in the plan are created
by this task. Supply a disposable PostgreSQL DSN separately for dialect checks;
without it, record PostgreSQL as skipped rather than passed.

```bash
(cd pluginsdk && go test -trimpath -race ./...)
(cd apps/backend && go test -trimpath -race ./internal/notifications/... ./internal/plugins/... ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/plugins/registry.test.ts lib/plugins/host-settings-save.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run e2e/tests/settings/notification-formatters.spec.ts e2e/tests/settings/mobile-notification-formatters.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `pluginsdk/`, especially optional interface/glue and `proto/kandev/plugin/v1/`.
- `apps/backend/internal/plugins/{manifest,runtime}` and provider action adapters.
- `apps/backend/internal/notifications/{service,store,handlers,controller}`.
- Backend composition, `internal/persistence/{requiredstores,storeconformance}`,
  and explicit-tag upgrade fixtures.
- `apps/web/lib/plugins/`, notification settings, localized catalogs, and
  proposed settings E2E files.
- `docs/public/plugins-authoring.md`, `plugins-manifest.md`, plugin API/wire docs.

## Dependencies

Maintainer direction discussion is satisfied by comment 6683963471. Q1-Q8 are
confirmed in the plan. Use an implementation turn after this updated design
handoff and reconcile the exact source/contract before code edits.

## Risks

Holding locks or SQL transactions across plugin RPCs can block settings and
shutdown. Snapshot selection, release locks, and fence lifecycle results instead.

## Parallelism

Contract-aligned development can overlap Task 02 after discussion. Integration
depends on this task's implementation. No delegation is authorized by the wave.

## Inputs

- Paired requirements/design and proposed ADR linked from the plan.
- Existing optional SDK extensions, notification ownership/runtime-summary tests,
  authenticated actions, and generic settings-save contributor tests.

## Results

Implemented the optional Go/protobuf contract, owner/type/revision-bound SQL
selection, scoped management envelope, owner-isolated editor mount, synthetic
preview, whole-pair fallback, and bounded configuration capture lease.

Verification on 2026-11-07: complete SDK race suite, notification package race
suite, SQL guard, persistence store conformance, frontend typecheck/full lint,
and 36 focused frontend tests passed. Changed-package Go lint passed with
Go 1.26.6 and golangci-lint 2.9.0. PostgreSQL was not configured and is not
claimed tested. The generic desktop/phone fixture and product-package browser
checks passed during implementation.

Review regressions include installation lease retention through binding commit,
management revision checks before dispatch and response, lost clear-response
readback, stale read/mutation publication, loading settlement, and formatter
capture before runtime-summary delivery claims.

The broader plugin suite hit `TestPluginExecutorPackagedRecovery` HTTPS timeout
against the host's non-loopback interface. The same test failed on unchanged
source at this worktree's base. A complete frontend Vitest run exceeded its
600-second command budget without a final summary; it is not reported passed.
Final contribution/candidate checks remain tracked with Task 04.

### PR #5141 review follow-up

Addressed the removed-editor activation race (including selection ABA), separated
capture/lifecycle fencing from dispatch admission, removed blocking process kill
from runtime lookup critical sections, and added closed-reason binding lookup
diagnostics and loaded-provider type validation. Formatter work now shares one
one-second fanout budget without shortening the sender's caller context.
Management body reads use the HTTP request deadline, reject empty formatter keys,
and forward the verified key through the additive Go/protobuf action context.

Selection-only drafts participate in shared Save/navigation guarding. The editor
prepares before explicit first activation; Save does not implicitly select an
unknown revision. Failed discovery has a Retry control, and fixture controls and
activation notices use reactive plugin translations. The rollback design now
requires preservation of later host-domain changes, not just plugin exports.

Validation: affected plugin/runtime/notification race suites passed with the
previously baseline-failing packaged executor case explicitly excluded; the
notification service suite passed after aligning its fixture with the stored
provider type. Complete SDK race tests, seven formatter hook tests, four other
formatter component/API tests, frontend typecheck/full lint/i18n checks,
changed-package Go lint, and specification validation passed. Managed desktop
and Pixel 5 E2E checks passed, including selection-only preparation, no implicit
first binding, explicit activation, preview, and shared default restoration.

### Independent shared-save follow-up

PR #5141 comment `4504572313` exposed a selection-only draft cancelling the
remaining shared-save contributors. Pending activation now defers only that
contributor while retaining dirty/navigation/Reset semantics. Ordinary explicit
save cancellation still stops subsequent contributors. The fix belongs to the
generic host; the independent Apprise editor already prepares before explicit
activation and requires no package change.

The new integration regression failed before the fix for both initial selection
and switching from an existing binding, then passed with the real save coordinator.
The formatter and coordinator suites passed 35 tests; frontend typecheck and full
lint passed. Full Vitest exceeded 600 seconds and reported two failures in
`plan-comment-migration.test.ts`; the same two failures reproduced on unchanged
stable-candidate source `c846cf102`. This is not a green full-suite result.
