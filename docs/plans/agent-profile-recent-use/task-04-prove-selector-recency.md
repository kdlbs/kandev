---
id: "04-prove-selector-recency"
title: "Prove persisted selector recency"
status: done
wave: 4
depends_on:
  - "03-record-successful-launches"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
  - REQ-AGENTS-PROFILE-RECENT-USE-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-RECENT-USE-001.1
  - AC-AGENTS-PROFILE-RECENT-USE-001.3
  - AC-AGENTS-PROFILE-RECENT-USE-002.1
  - AC-AGENTS-PROFILE-RECENT-USE-003.1
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
---

# Task 04: Prove Persisted Selector Recency

## Summary

Add a focused browser flow that proves a successful agent-profile use changes
the next selector order and survives a reload. Keep the scenario narrow because
unit and integration tests own context matrices, caps, and failure paths.

## In scope

- Create or reuse two eligible agent profiles in the E2E fixture.
- Launch quick chat with the non-leading profile, reopen the picker, and assert
  it precedes unseen eligible profiles.
- Reload the app and assert the persisted order remains.
- Verify a cancelled selector interaction does not displace the last
  successfully used profile.

## Out of scope

- Repeating the browser scenario across every operational context.
- New mobile composition or geometry assertions.
- Broad E2E or manual QA sweeps.

## Acceptance

- The focused Chromium scenario proves successful-use ordering, cancellation
  exclusion, and reload persistence through real backend storage.
- The test uses user-visible selector interactions and stable test IDs rather
  than mutating the frontend store.
- Existing mobile quick-chat and handoff paths remain unchanged; no new mobile
  test is added under the data-normalization exception.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/chat/agent-profile-recent-use.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/chat/agent-profile-recent-use.spec.ts`
- `apps/web/e2e/pages/quick-chat-page.ts`
- `apps/web/e2e/fixtures/test-base.ts`

## Dependencies

- Task 03 completes all success-only recording integrations.

## Risks

- The selected option is intentionally displayed first, so assertions must
  inspect a reopened selector whose default/selection state does not mask the
  underlying recent-use order.
- The fixture must avoid relying on provider labels or source ordering that can
  change independently of this feature.

## Parallelism

`sequential`

## Inputs

- All three requirements and the complete system design.
- Existing quick-chat E2E profile setup and selector interaction helpers.

## Results

Added a focused Chromium flow that creates two eligible profiles, launches the
non-leading profile in Quick Chat, cancels a later selection, verifies the
successful profile remains first, reloads the app, and verifies the persisted
order through the real API and selector. Verified with:

```bash
cd apps/web && pnpm e2e:run --project chromium tests/chat/agent-profile-recent-use.spec.ts
```

Result: 1 test passed in 8.1 seconds.
