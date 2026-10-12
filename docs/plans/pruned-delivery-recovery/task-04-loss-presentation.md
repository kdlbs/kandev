---
id: "04-loss-presentation"
title: "Present historical loss and verify restart continuity"
status: in_progress
wave: 4
depends_on:
  - 03-guarded-restoration
plan: plan.md
requirements:
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.2
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.5
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.6
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.7
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
system_design:
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
---

# Task 04: Present historical loss and verify restart continuity

## Summary

Show the real missing-output cause before restoration and preserve its
historical notice after the ordinary composer becomes available. Prove the
same original conversation stays usable across another restart on desktop
and phone, with browser execution exclusively in CI.

## In scope

- Structured typed gap projection and translated recovery reason, progress,
  historical loss notice, and collapsed inspection details.
- Existing service/store/component revision fences and ordinary restored
  composer state; no stale snapshot may reactivate a resolved block.
- Seven-language localization, phone wrapping/44px actions, history/reload
  persistence, and public operations guidance through docs-maintainer.
- Synthetic desktop/phone CI scenarios for automatic and explicit recovery,
  subsequent restart, normal distinct instruction, and refusal controls.
- Recovered-event control: normal replay/projection keeps the original stream
  and shows no unavailable-output notice once actual receipt covers the interval.

## Out of scope

- New recovery actions/forms, batch recovery UI, desktop layout redesign,
  unlocalized raw backend error rendering, local browser or E2E execution,
  user-message creation during recovery, or weakened backend eligibility.

## Acceptance

- The existing inline card shows the specific gap cause instead of generic
  launch failure, with progress and existing Resume eligibility on both views.
  The historical note remains after success/reload while the composer is usable.
- Desktop/phone CI scenarios verify unchanged original identity/workspace,
  old unknown payload/outcome, cursor 5494, missing interval 5495..5499, one
  successor, no old dispatch, and a distinct new instruction after another restart.
- Stale snapshots cannot replace newer recovery state; unsupported, live, or
  unknown cases show their actual refusal without affecting eligible siblings.

## ASCII UI preview

UI-01/02 are the same structures as [the plan](plan.md#ascii-ui-preview), mapped
to AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.7.

```text
Desktop: inline card
Some output from before the restart is unavailable.
Restoring your saved conversation...
[Resume]  [Details v]

Phone: existing bounded inline card
Some earlier output is unavailable.
Restoring your saved conversation...
[             Resume             ]
[ Details v ]

After verified restoration, both views:
History: Some earlier output is unavailable.
         The previous prompt outcome is uncertain. [Details v]
[                 Ordinary composer                 ]
```

The card scrolls within its existing 50dvh maximum; chat remains the primary
scroll owner. Details wrap rather than forcing document overflow. Phone action
targets are at least 44px. Reuse `session-recovery-card.tsx` and shared domain
state; historical notice is informational and cannot disable the composer.
Resume is disabled while automatic restoration owns the attempt. Copy and
spacing are illustrative and must use translated catalog entries.

## Verification

Local targeted checks only:

```bash
(cd apps/web && pnpm exec vitest run lib/services/session-recovery-service.test.ts components/task/chat/delivery-history-notice.test.tsx components/task/chat/pruned-delivery-recovery.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

CI-only browser checks (never run these locally):

```bash
(cd apps/web && pnpm e2e:run --host e2e/tests/session/pruned-delivery-recovery.spec.ts --project=chromium)
(cd apps/web && pnpm e2e:run --host e2e/tests/session/mobile-pruned-delivery-recovery.spec.ts --project=mobile-chrome)
```

Use the existing production-build CI fixture and serialized shard limits.
Record actual new-head CI evidence; interrupted runs and filtered zero-test
commands do not count as passed. Install frozen workspace dependencies from
`apps/` only if implementing in a fresh checkout without its node_modules.

## Files likely touched

- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/lib/types/http.ts` and existing recovery WS/store projections
- `apps/web/components/task/chat/session-recovery-card.tsx`
- `apps/web/components/task/chat/session-bootstrap-recovery-card.tsx`
- `apps/web/components/task/chat/messages/action-message-details.tsx`
- `apps/web/components/task/chat/pruned-delivery-recovery.test.tsx` (new)
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/helpers/pruned-delivery-recovery.ts` (new)
- `apps/web/e2e/tests/session/pruned-delivery-recovery.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-pruned-delivery-recovery.spec.ts` (new)
- `docs/public/operations.md`

## Dependencies

03-guarded-restoration.

## Risks

A cleared current block must not hide historical loss. A historical note must
not recreate current uncertainty or imply an external tool was rolled back.
CI owns rendered verification because the user prohibits local E2E.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Design: presentation, observability, and validation.
- Plan UI-01/02, mobile-parity, shipped inline recovery card, and current
  desktop/phone recovery fixtures. Use TDD and docs-maintainer for implementation.

## Results

Localized inline Resume/Stop, collapsed range details, persistent informational archive notice, stale store fences, and public operations guidance implemented. Frontend unit, web typecheck, localization, and public docs checks pass. Desktop/phone restart scenarios and screenshots execute only in CI and remain pending.

## CI remediation in progress

Correct the intentionally partial banner fixture's TypeScript assertion. Add the
shared expandable technical-details component to the restored historical notice
so each archived stream and translated missing range survives reload and remains
inspectable. The notice remains informational with existing phone touch targets.
UI and browser validation remain CI-owned for the final fixup head.

Focused frontend tests and lint pass after moving the localized diagnostic
assembly out of JSX. The banner fixture follows the directory's describe-block
convention. No local E2E was run; final remote CI remains pending.


CI found lock contention in the mobile settings-recovery metadata fixture. Its direct SQLite write now uses a bounded busy timeout and an immediate transaction encompassing the metadata read and update, preserving concurrent backend metadata. Container recovery failures now retain backend logs as CI artifacts. These E2E scenarios are evaluated only in CI, with all existing assertions intact.


The next CI run passed backend, frontend, and container gates. Both pruned-history browser scenarios stopped at setup because the successful Go seed helper omitted the completion marker expected by the subprocess wrapper. The helper now emits the marker after its SQL/journal setup succeeds, matching the existing fixture protocol. Existing browser assertions cover that marker; recovery and subsequent-restart assertions remain unchanged and pending CI execution. No local E2E ran for this correction.
