---
id: "03-match-storage-guidance"
title: "Match chat guidance to the evidence"
status: done
wave: 3
depends_on:
  - "01-preserve-storage-evidence"
  - "02-converge-writer-recovery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002
  - REQ-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-002.16
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.1
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.3
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.5
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.6
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.8
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.9
  - AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.11
system_design:
  - ../../specs/platform/system-design/delivery-storage-diagnostics.md
---

# Match chat guidance to the evidence

## Summary

The chat notice currently falls back to free-space advice when its reason is
absent. Its desktop pressure row also adds disk usage for every cause. Use the
existing storage-health data to render the cause and its relevant measurements.

## Scope

- Show a neutral delivery-blocked headline and neutral Details when the cause
  is unavailable. Show the existing typed limit, size, and write-failure
  guidance when evidence supports it.
- Reuse the existing guidance for filesystem allocation and quota failures.
  Reserve byte cleanup advice and the pressure headline's disk percentage for
  a diagnosed low-space guard. Keep an independent high-usage sample in
  labeled Details when another cause owns the headline.
- Use the existing byte and timestamp formatters for available evidence. Use
  the current chat footer, Details, Dismiss, and eligible local Storage link.
  Add the localized fallback and guidance in every real locale.
- Update the storage section of the existing public session guide with the
  unknown-cause fallback and the meaning of the Details measurements.

## Acceptance

1. A writer failure with unknown cause and a healthy capacity sample shows
   the neutral copy. Known causes show their respective guidance.
2. The notice uses localized copy and the existing observation ages. The
   high-usage advisory remains available when its threshold is met.
3. Details and Dismiss are reachable by keyboard and touch. A phone keeps the
   same evidence and recovery actions, 44 px touch targets, and a usable
   composer while Details is open.

## ASCII UI preview

**UI-01 Missing cause** and **UI-02 Known delivery limit**, within the current
footer above the composer. [Complete preview](plan.md#ascii-ui-preview).

```text
Delivery is blocked                                         [Details] [x]
Details   Kandev could not determine the cause.
          Check the runtime logs or use session recovery.

Delivery is blocked: stream limit reached                   [Details] [x]
Details   Inspect retained output and delivery progress before resuming.
```

Details is the main action. On a phone, its tap opens the in-place disclosure.
The cause wraps beside 44 px controls in the existing footer. The preview covers
`AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.6` and
`AC-PLATFORM-DELIVERY-STORAGE-DIAGNOSTICS-003.11`.

## Verification

The temporary component case with 159.5 GB available selected the free-space
guidance key. Add that assertion to the existing notice suite. Exercise an
unknown cause and a typed stream limit with the existing status-frame fixture
on desktop and phone. Keep the explicit physical-capacity and high-usage cases.
Use the existing real writer and cancellation Playwright fixtures for the
recovery and draft behavior. Regenerate the Traditional Chinese and pseudo
catalogs when the locale keys change.

```bash
pnpm --dir apps/web test components/task/chat/session-delivery-storage-notice.test.tsx
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:pseudo
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/delivery-storage-diagnostics.spec.ts tests/session/resilient-output-delivery.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts tests/session/mobile-resilient-output-delivery.spec.ts
node scripts/validate-public-docs.mjs
```

## Likely files

- `apps/web/components/task/chat/session-delivery-storage-notice.tsx` and its component test
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/session/delivery-storage-diagnostics.spec.ts`
- `apps/web/e2e/tests/session/mobile-delivery-storage-diagnostics.spec.ts`
- `docs/public/sessions-and-review.md`

## Dependencies

[01 Preserve storage evidence](task-01-preserve-storage-evidence.md) and
[02 Bound recovery and converge cancelled sessions](task-02-converge-writer-recovery.md).

## Risks

Long translations affect the compact phone row. Verify that the labels and
control hit areas fit at the configured phone viewport.

## Parallelism

Sequential.

## Results

RED: Four component assertions failed against the previous notice: quota and
filesystem-full causes used general free-space advice, a missing cause used a
storage headline and cleanup guidance, and a stream limit showed disk usage in
the headline.

GREEN: Missing or unsupported causes use a neutral headline and runtime-log or
session-recovery guidance. Low physical-space pressure keeps the free-space
guidance and percentage label. Filesystem allocation and quota failures reuse
their localized recovery guidance. Other causes keep their existing copy, with
high disk usage labeled separately in Details while write pressure and the
backend warning state are active. The backend warning state retains the advisory
through its 85–90% hysteresis band. The local Storage link follows that warning
state or a typed local capacity issue.
The phone uses the existing disclosure and touch controls.

Verification passed:

```bash
pnpm --dir apps/web exec vitest run components/task/chat/session-delivery-storage-notice.test.tsx
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:pseudo
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/delivery-storage-diagnostics.spec.ts tests/session/resilient-output-delivery.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-delivery-storage-diagnostics.spec.ts tests/session/mobile-resilient-output-delivery.spec.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec oxlint components/task/chat/session-delivery-storage-notice.tsx components/task/chat/session-delivery-storage-notice.test.tsx
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

The component suite passed 19 tests. Desktop E2E passed 8 tests and built the
backend fixtures and Vite assets. Phone E2E passed 6 tests. Typecheck, targeted
Oxlint, localization, and public-doc validation passed. The public-doc
validator checked 56 pages.

The specification lint and final diff check passed after the review updates.

The review regression uses warning state at 95% and 88%, then normal state at
80%. Details retains the high-usage label and local Storage link at 88%; both
clear when health becomes normal. The unrelated Traditional Chinese
`identityLinkCodeInstructions` edits were reverted; the storage translations
remain in `task.json`.
