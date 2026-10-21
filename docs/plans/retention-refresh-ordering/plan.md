---
created: 2026-10-21
status: done
requirements:
  - REQ-OFFICE-RUN-HISTORY-RETENTION-004
system_design:
  - ../../specs/office/system-design/run-history-retention-operations.md
legacy_specs: []
---

# Implementation Plan: Retention refresh ordering

## Overview

An old route instance's GET unconditionally replaces shared status after a newer
PUT acknowledgement, rewinding the baseline and making Reset restore obsolete
settings. Parent reproduced the real component/store/API/coordinator race on
2530afe4. Implement one bounded sequential work order under the explicit
autonomous design-package/TDD exception, then stop at parent validation.

## Scope

Fence Office retention reads and preserve normalized acknowledgements and full
status. Exclude backend/schema/security changes, tool-payload retention, sweeps,
deletion, generic resources, dependencies, push and PR creation.

## Technical approach

Use store-scoped read tokens and hook-lifetime/local read ownership in
`use-retention-settings.ts`, following nearby per-store weak-map conventions.
Preserve admitted mutation publication after unmount and coordinator semantics.
Reconcile the submitted card draft with the normalized acknowledgement only if
no later edit replaced it.
Confirmed assumptions: whole-document PUT, last-writer-wins server, no new
server ordering protocol. No material unresolved design questions or ADR.

## ASCII UI preview

UI-01: Storage > Office retention, Advanced expanded, desktop and phone.
Composition and copy remain shipped; only reconciliation changes.

```text
Office retention
  Policy and sweep status
  Advanced settings [expanded]
    Batch limit [7000]
  [Reset] [Save changes]   (only for a dirty draft)
```

Before: late GET 6000 makes saved 7000 falsely dirty; Reset displays 6000.
After: saved 7000 stays clean; edit 8000 then Reset displays 7000.
Phone reuses the shipped one-column card, touch controls and safe-area-aware
floating Save surface; desktop retains its field grid. No layout, navigation,
copy, or control-size changes. AC-OFFICE-RUN-HISTORY-RETENTION-004.10 is checked
through the real card/coordinator and guarded desktop/phone navigation tests.

## Tests

`retention-refresh-ordering.test.tsx` exercises real card/store/coordinator/API
with fetch-only deferred responses. Hook tests cover read success/failure orders,
loading/error ownership, isolation, unmount and normalized acknowledgements.
Existing card and coordinator/API suites retain validation/failure/revision coverage.

## E2E tests

Desktop and phone share a held real-backend GET helper, leave/reenter Storage
through client navigation, save twice, settle stale GET, verify clean UI, Reset,
fresh save, reload, and actual backend readback. Restore full original policy in
finally. No sweep or deletion is invoked.

## Work orders

- [x] [Task 01: Fence retention refresh publication](task-01-fence-retention-refresh.md)

## Verification

Run from repository root with installed Node 24 on PATH:

```bash
(cd apps/web && pnpm exec vitest run components/settings/system/retention-refresh-ordering.test.tsx components/settings/system/retention-settings-card.test.tsx components/settings/settings-save-provider.test.tsx hooks/domains/system/use-retention-settings.test.tsx lib/api/domains/system-api.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/system/use-retention-settings.ts hooks/domains/system/use-retention-settings.test.tsx components/settings/system/retention-refresh-ordering.test.tsx components/settings/system/retention-settings-card.tsx e2e/helpers/retention-refresh-ordering.ts e2e/tests/system/retention-settings.spec.ts e2e/tests/system/mobile-retention-settings.spec.ts)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project chromium tests/system/retention-settings.spec.ts --grep 'acknowledged policy' --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --no-build --project mobile-chrome tests/system/mobile-retention-settings.spec.ts --grep 'acknowledged policy' --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff 2530afe4e53c481c937bf39eb2b10c5e6382988e HEAD --check
```

Run exact-head PR documentation coverage with the repository validator before
handoff. Retain every process handle through terminal completion. Run browsers
sequentially with one managed worker, no retries, and disposable backend data.

## Verification results

Component RED: both stale-success completion orders rewound baseline 7000 to
6000; hook RED exposed old loading finalization and cross-reader replacement.
A normalized-response control also failed (submitted 6001 remained after
acknowledged 6000); adopting that response only for the submitted draft fixes it.
Final guarded GREEN: 74 tests across five targeted suites (11.46s), including
all new regressions plus unchanged card, coordinator and API contracts.
Typecheck passed with Node 24 and 8 GiB heap. Targeted ESLint passed.
Managed fresh production build and desktop flow passed, one test (8.9s runner);
phone flow passed sequentially against the same build, one test (6.8s runner).
Both used a real isolated backend, held real GET snapshots, actual client
navigation and controls, persisted readback, reload, and full policy restoration.
Component tests are the causal RED gate; browser evidence is post-fix only.
No sweep/deletion was invoked, and no user's running instance was used.
Catalog validation (396 decisions / 1691 specifications), full spec lint,
working-tree documentation coverage and whitespace checks passed. Delivery adds
normal hook receipts and exact committed-head coverage/diff checks in the parent
handoff; no broad audit or publication is part of this work order.

## Documentation impact

Public operations docs already describe the policy/settings surface accurately.
This repairs the existing Save/Reset contract without changing commands, API,
copy or navigation; internal requirements/design/plan updated only.

## Risks

Rejecting obsolete full snapshots must preserve newer sweep/count fields. A
retired read must not suppress an admitted write acknowledgement. Browser route
navigation must preserve app store while replacing the pathname-keyed provider.
