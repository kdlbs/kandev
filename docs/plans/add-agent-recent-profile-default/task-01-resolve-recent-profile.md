---
id: "01-resolve-recent-profile"
title: "Resolve the recent Add Agent profile"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
  - REQ-AGENTS-PROFILE-RECENT-USE-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-RECENT-USE-001.2
  - AC-AGENTS-PROFILE-RECENT-USE-001.4
  - AC-AGENTS-PROFILE-RECENT-USE-001.6
  - AC-AGENTS-PROFILE-RECENT-USE-001.7
  - AC-AGENTS-PROFILE-RECENT-USE-001.8
  - AC-AGENTS-PROFILE-RECENT-USE-002.1
  - AC-AGENTS-PROFILE-RECENT-USE-003.3
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
---

# Task 01: Resolve the Recent Add Agent Profile

## Summary

Add a pure initial-profile resolver for the New Agent dialog. Use the recent
`task_session` profile before the current session. Preserve handoff, manual
choice, eligibility, and fallback behavior.

## In scope

- Write the failing resolver test before production code.
- Resolve compatible profiles by handoff, recency, current session, and source
  order.
- Track whether the selection came from handoff, recency, fallback, or manual
  input.
- Send `profile_explicit: true` for the recent-use default.
- Prevent later store and compatibility updates from replacing manual input.
- Keep recency recording success-only.

## Out of scope

- Backend request or routing changes.
- Changes to non-`task_session` selectors.
- Dialog presentation or user-facing copy.
- Playwright coverage.

## Acceptance

- A compatible recent profile becomes the selected and explicit launch profile.
- A handoff target or manual input remains authoritative.
- Empty or ineligible history keeps the existing fallback and does not block
  the dialog.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/new-session-profile-selection.test.ts components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts lib/agent-profile-recent-use.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/new-session-profile-selection.ts`
- `apps/web/components/task/new-session-profile-selection.test.ts`
- `apps/web/components/task/new-session-dialog.tsx`
- `apps/web/components/task/new-session-dialog.test.tsx`
- `apps/web/components/task/new-session-form-actions.test.ts`

## Dependencies

None.

## Risks

- `profile_explicit: true` lets the recent default override workflow-step
  profile resolution.
- An effect-based reconciliation can replace manual input after the dialog
  opens.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-PROFILE-RECENT-USE-001`
- Frontend ordering and failure sections in the profile recent-use design.
- Existing selection and launch logic in `new-session-dialog.tsx`.

## Results

- Added `new-session-profile-selection.ts` with handoff, compatible recent-use,
  current-session, and source-order resolution plus provenance.
- Wired the resolver into `NewSessionDialog`; recent defaults are explicit,
  manual choices are protected, and invalid selections still fall back safely.
- Added focused resolver and dialog regressions, including unavailable and
  empty recent-use state, invalid manual selections, and incompatible handoff
  fallback.
- `cd apps && pnpm --filter @kandev/web test -- --run components/task/new-session-profile-selection.test.ts components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts lib/agent-profile-recent-use.test.ts` passed (34 tests).
- `cd apps/web && pnpm run typecheck` passed.
