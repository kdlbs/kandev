---
id: "05-continuity-feedback"
title: "Expose explicit continuity recovery"
status: complete
wave: 5
depends_on: ["04-workspace-identity"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-HARNESS-SESSION-CONTINUITY-005
acceptance_criteria:
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1
  - AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.2
system_design:
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 05: Expose explicit continuity recovery

## Summary

Deliver the interactive portion of the continuity milestone with persistent recovery outcomes and an explicit context-continuation action.

## In scope

- Wire the authorized continuation action through the existing recovery handler, service, and session ownership checks.
- Extend SessionRecoveryFeedback and status messages with native, context, blocked, and truncation outcomes.
- Reuse the chat scroll owner on mobile. Add accessible status, focus behavior, wrapping actions, and coarse-pointer touch targets.
- Preserve stable action names and one translated role=status region. Route autonomous notices to this recovery surface.
- Add all five locales and desktop/mobile end-to-end coverage. Keep existing branch/archive recovery flows intact.

- Update the continuity explanation in docs/public/sessions-and-review.md without claiming durable transport is available.

## Out of scope

Durable transport claims, a new recovery page, and automatic continuation policy.

## Acceptance

- A reload retains the typed outcome. A duplicate or stale action cannot create another generation.
- Desktop and mobile users can complete the same recovery actions without horizontal overflow.
- The interface explains lost private harness state before context continuation and shows the target directory.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/web && rtk pnpm test lib/services/session-recovery-service.test.ts)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/session/harness-session-continuity.spec.ts tests/session/session-resume-recovery.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-harness-session-continuity.spec.ts tests/session/mobile-session-resume-recovery.spec.ts)
rtk make -C apps/backend lint
(cd apps/web && rtk pnpm run lint)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/web/lib/services/session-recovery-service.test.ts`:

- `persists typed recovery outcomes across reload`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.1`.
- `rejects stale continuation actions`: stale-action regression.
- `mobile-harness-session-continuity.spec.ts`: `AC-AGENTS-HARNESS-SESSION-CONTINUITY-005.2` rendered parity.

## Files likely touched

- `apps/web/components/task/ensure-session-error.tsx`
- `apps/web/components/task/chat/messages/status-message.tsx`
- `apps/web/components/task/chat/messages/action-message-recovery.tsx`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/hooks/domains/session/`
- `apps/web/src/locales/`
- `apps/backend/internal/orchestrator/`
- `docs/public/sessions-and-review.md`
- `apps/web/lib/services/session-recovery-service.test.ts` (new tests or extensions).

## Dependencies

[Task 04](task-04-workspace-identity.md).

## Risks

- A notice must not imply that a fresh conversation restored private harness state.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/agents/system-design/harness-session-continuity.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented typed recovery service actions, localized desktop/mobile feedback, stale-action handling, and touch/overflow E2E coverage. Focused web tests and i18n/lint gates pass.
