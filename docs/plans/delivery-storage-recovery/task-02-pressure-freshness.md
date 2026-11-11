---
id: "02-pressure-freshness"
title: "Preserve independent pressure freshness"
status: complete
wave: 2
depends_on:
  - 01-post-turn-observation
plan: plan.md
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.2
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.17
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.4
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.7
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.8
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Task 02: Preserve independent pressure freshness

## Summary

Give pressure observations their own receipt age through the existing backend and browser projection.
Fresh capacity must not renew an old blocked-output claim.

## In scope

- Separate capacity and pressure receipts in `RuntimeStorageObservation`, including copies and equality/refresh handling.
- Add optional `pressure_observed_at` and `pressure_age_seconds` to DTOs, event serialization, TypeScript types, and normalization.
- Refresh pressure receipt after every accepted supported status, including unchanged values. Bound nonhealthy event refresh to 30 seconds.
- Preserve epoch/revision ordering across boot, detail, subscription, and live-event merges.
- Show independent observation times in Details and last-reported pressure after 90 seconds or unknown pressure age.
- Preserve existing notice composition, draft, focus, recovery controls, and independent capacity thresholds.
- Add translated time labels and update the low-disk-space how-to when behavior is implemented.

## Out of scope

Session-state mutation, new controls, automatic Resume, new browser polling, or persistent health records.

## Acceptance

1. RED: `TestDeliveryStorageCapacityDoesNotRefreshPressureAge` and a component regression expose renewed old pressure. GREEN: both clocks round-trip independently, including zero, unknown, failed, and unchanged samples.
2. Desktop and phone show last-reported pressure while capacity stays fresh. A supported healthy delivery clears the live row at normal capacity and preserves a warning-only disk advisory.
3. Reload and late snapshots preserve newer health and historical failure evidence. Drafts, focus, recovery eligibility, locale support, and touch targets remain correct.

## ASCII UI preview

Full structural contract: [UI-01 and UI-02](plan.md#ascii-ui-preview).
The same shared view model serves task chat and Quick Chat.

```text
UI-01, desktop:
  ! Last reported: Delivery is blocked on storage  [Storage] [Details]
  Details: Pressure observed <time>; capacity observed <time>

UI-01, phone:
  ! Last reported: Delivery is blocked
    on storage                    [Storage] [Details]

UI-02, healthy delivery and normal capacity:
  [ Existing composer; no live pressure row ]
```

Use the shipped notice and recovery card as mobile exemplars.
Keep 44px phone/coarse-pointer controls, the current scroll owner, and explicit disclosure.
Spacing is illustrative. The disclosure and independent time meanings are requirements.
The preview maps to criteria 002.2, 002.16, 003.5, 003.6, and 003.7.

## Verification

Install workspace dependencies once before the first frontend command in this worktree.
Run from the repository root, sequentially:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/task/dto ./internal/orchestrator ./internal/backendapp -run 'DeliveryStorage|RuntimeStorage|BootSessionRuntime|DeliveryHealth' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/runtime_storage.go apps/backend/internal/task/dto/runtime_storage.go apps/backend/internal/orchestrator/runtime_storage_health.go
(cd apps/web && pnpm exec vitest run lib/session-delivery-storage.test.ts lib/ws/handlers/session-delivery-storage.test.ts lib/state/slices/session/session-merge-runtime-storage.test.ts components/task/chat/session-delivery-storage-notice.test.tsx)
(cd apps/web && pnpm exec eslint lib/session-delivery-storage.ts lib/types/http.ts lib/ws/handlers/session-delivery-storage.ts components/task/chat/session-delivery-storage-notice.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/session/delivery-storage-diagnostics.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.
Use fake time for 89, 90, and 91-second pressure ages and future source timestamps.
Verify missing legacy fields, zero-age serialization, delayed idle responses, and repeated capacity revisions.
Managed browser runs must build fresh artifacts and execute sequentially with the configured worker limits.
Browser status frames prove presentation only. Backend tests must prove actual source-to-projection behavior.
Include every additional changed Go file in changed-file lint.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/runtime_storage.go`, `runtime_storage_events.go`, `runtime_storage_test.go`, and `delivery_health_monitor.go`.
- `apps/backend/internal/task/dto/runtime_storage.go` and its tests.
- `apps/backend/internal/orchestrator/runtime_storage_health.go` and its tests.
- `apps/backend/internal/backendapp/boot_session_runtime_test.go`.
- `apps/web/lib/types/http.ts`, `lib/session-delivery-storage.ts`, and its tests.
- `apps/web/lib/ws/handlers/session-delivery-storage.ts` and its tests.
- `apps/web/lib/state/slices/session/session-merge-runtime-storage.test.ts`.
- `apps/web/components/task/chat/session-delivery-storage-notice.tsx` and its tests.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- `apps/web/e2e/tests/session/delivery-storage-diagnostics.spec.ts` and `mobile-delivery-storage-diagnostics.spec.ts`.
- `docs/public/sessions-and-review.md`, low-disk-space how-to section.

## Dependencies

[Task 01](task-01-post-turn-observation.md).

## Risks

- Reusing capacity age for an older backend silently recreates the defect.
- Updating every pressure receipt can create event churn unless publication remains bounded.
- Source time or browser wall-clock skew cannot decide freshness.

## Parallelism

`sequential`

## Inputs

- [Storage requirements](../../specs/platform/requirements/delivery-storage-diagnostics.md), requirements 002 and 003.
- [Freshness design](../../specs/platform/system-design/delivery-storage-diagnostics.md#independent-receipt-ages).
- Existing runtime-storage DTO, boot enrichment, shared merge, notice, and desktop/phone fixture patterns.

## Results

Implemented independent pressure receipts in `RuntimeStorageObservation`, the task DTO, boot/detail projection, and browser normalization, merge, and notice presentation.
Capacity samples preserve the pressure receipt while renewing capacity age.
Accepted supported pressure refreshes its receipt, including unchanged values, and sustained nonhealthy events remain bounded to 30 seconds.
Desktop and phone notices show last-reported pressure after 90 seconds or when an older backend omits pressure age.
Review follow-up resets the local monotonic pressure baseline from each accepted snapshot age identity so rising `pressure_age_seconds` is not counted twice, while capacity and pressure clocks stay independent.
The component, merge, normalization, and DTO regressions passed, along with typecheck, targeted ESLint, the full i18n gate, and both storage diagnostics end-to-end specs.
