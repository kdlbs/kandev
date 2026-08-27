---
id: "12e-shared-task-status"
title: "Render registered provider status in exact host task chrome"
status: completed
wave: 3e
depends_on: ["12d-host-native-task-link-parity"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
decision: "../../decisions/2026-08-06-plugin-code-host-dashboard-parity.md"
---

# Task 12e: Render registered provider status in exact host task chrome

## Intent

Make status presentation impossible for a code-host plugin to visually fork. Extend the
existing review-provider data contract and have Kandev mount the exact shared topbar and
composer CI surfaces for registered providers.

## Owned paths

- `apps/web/lib/plugins/{types,registry,host-api}.ts` and focused tests
- `apps/web/components/task/review-panel-provider.ts` and tests
- `apps/web/components/integrations/change-request-status-*.tsx` and tests
- `apps/web/components/github/{pr-topbar-button,pr-status-chip,pr-ci-popover}.tsx`
- `apps/web/components/task/{task-top-bar,passthrough-toolbar}.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- plugin API/authoring docs

## Implementation

1. Add an optional normalized `taskStatus` to `ReviewItemSummary`, preserving it through
   registry validation without accepting provider-specific payloads.
2. Extract the GitHub topbar trigger, composer chip, and common CI popover/drawer body
   into provider-neutral host components. Adapt GitHub to those components first.
3. Build one registry-backed status adapter from `useNormalizedTaskReviews`; refresh via
   the provider's existing deduplicated lifecycle and background interval.
4. Mount it beside `PRTopbarButton`, beside `PRStatusChip`, and in passthrough status.
   Avoid a new plugin visual slot or second provider poller.
5. Keep optional GitHub-only automation/merge callbacks in the shared anatomy without
   requiring them from Bitbucket.

## TDD and acceptance

1. RED registry tests prove normalized status is currently discarded.
2. RED task-topbar, chat-status, and passthrough tests require registered-provider
   topbar and composer entries, exact shared popover content, refresh routing, and unload.
3. GREEN implementation keeps all existing GitHub status tests unchanged or adapts only
   their provider-neutral import names.
4. Run focused Vitest, web typecheck/lint, and desktop/mobile E2E assertions.

## Mobile contract

Touch uses the same bounded drawer as GitHub, one internal scroll owner, and controls at
least 44px tall. Desktop and mobile expose the same checks and open-Review capability.

## Risks

- Provider snapshots must remain stable for `useSyncExternalStore`.
- Background refresh must deduplicate topbar/composer consumers and abort on unload.
- Built-in GitHub behavior, automation, and multi-PR aggregation cannot regress.

## Completed verification (2026-08-06)

- Nine focused host suites passed (70 tests), plus web typecheck and lint.
- Desktop topbar and composer both rendered the shared `pr-topbar-popover-inner` anatomy.
- Touch emulation rendered the shared bounded CI drawer with no horizontal overflow.
