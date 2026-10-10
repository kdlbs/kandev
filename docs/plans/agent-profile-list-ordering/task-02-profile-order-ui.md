---
id: "02-profile-order-ui"
title: "Profile reorder UI without sorting"
status: in_progress
wave: 2
depends_on:
  - "01-profile-order-backend"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-001
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.3
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.4
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.6
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.7
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.8
  - AC-AGENTS-PROFILE-LIST-ORDERING-001.9
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.3
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.7
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.11
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.13
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.14
system_design:
  - ../../specs/agents/system-design/profile-list-ordering.md
---

# Task 02: Profile reorder UI without sorting

## Summary

Keep manual drag-and-drop ordering for each installed agent's profile list on
Settings > Agents. Remove the automatic profile-sorting action, its helper,
locale label, tests, and public-doc instruction. Preserve the per-agent save
queue, membership freshness guard, profile-prepend behavior, cross-client sync,
and pointer, touch, and keyboard reorder coverage. Issue #4307's maintainer
clarification limits the saved order to Settings: selectors must retain their
current order, recency, and default-selection behavior.

## In scope

- `agent-profile-order.ts` reorder helpers, `reorderAgentProfilesAction`, slice
  state `ProfileOrderSync` with `acceptServerOrder`, `reconcileAgentOrders`
  applied only to the Settings projection, membership-epoch guarded
  `applyAgentListSnapshot`, `setAgentProfileOrder`, `profile-order-queue.ts`,
  the `use-profile-order` accessor, and WS handler.
- Capture and enforce the client profile mutation epoch for every fetched
  `GET /agents` snapshot, including direct browser reads outside
  `AgentListResourceScope`; preserve stable-epoch route hydration and the
  rehydration guard.
- Keep persisted order in the Settings profile list and Settings navigation
  tree. Restore selector-facing profiles to the previous per-agent
  `created_at DESC, id ASC` order, retaining existing context-specific
  recency/default selection behavior.
- Audit selectors sourced from `agentProfiles`, `settingsAgents`, and direct
  `GET /agents` flattening; do not add new recency or default-selection behavior.
- Remove `sortProfileIdsByName`, the sort button and handler from
  `InstalledAgentsHeader`, the sorter-only test, and `sortProfilesByName` from
  all locale catalogs.
- Update desktop and mobile layout/order E2E specs and the auth reorder spec.
- Remove the automatic-sorting instruction from the public profile guide while
  retaining its manual reorder and Settings-only scope.

## Out of scope

- Backend changes (Task 01). Agent-card order, Dynamic agent profiles, and the
  Office list.
- Any automatic profile sorting, including by profile name or agent label.
- New selector ordering, recency, or default-selection behavior.

## Acceptance

- Dragging a profile by its handle with a pointer, touch, or keyboard saves the
  new order, which survives reload and appears on a second open Settings page
  and in the Settings navigation tree. A newly created or duplicated profile
  appears first within its own agent.
- The Installed agents toolbar exposes no automatic profile-sorting action to
  any user. Its existing Terminal, Rescan, and agent-creation actions remain.
- Profile selectors outside Settings > Agents retain their pre-change option
  order, context-specific recency ranking, and default selection after an order
  is saved. In particular, changing the Settings order does not change task
  creation, in-task agent selection, or handoff selector behavior. Do not add
  recency or selection behavior that is not already present.
- Non-administrators see no drag handle; a drop onto another agent's row changes
  nothing. When a save fails with no newer queued intent, restore the saved order
  and show a toast.
- If a newer drag arrives while a save is in flight, keep showing and submit the
  latest drag. On `409`, accept the refetch only through the membership-epoch
  guard; reconcile new IDs first in refreshed server order, then surviving
  queued IDs in their queued order. A profile created after the queued drag
  stays first, and deleted IDs are omitted. Opening, duplicating, and deleting
  profiles still work.
- An agent-list snapshot, cached response, or event with an older order revision
  never replaces a newer known order or a pending drag (settings load, Settings
  route re-hydration, and task-page hydration included).
- After a profile create/delete event, a delayed pre-event agent-list response,
  including the refetch for a `409`, cannot replace membership even when
  `profile_order_revision` is unchanged. A created profile remains visible
  first and a deleted profile stays absent. The `409` path captures the client
  epoch, rejects a mismatched response atomically, obtains a fresh resource
  result, then reconciles and replays surviving queued intent against current
  membership.
- `app/settings/agents/page.agent-list-snapshot.test.tsx` and
  `app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.test.tsx`
  hold each direct list GET response, apply create and delete events before it
  resolves, then assert the pre-event response cannot replace membership in
  either mirrored list.
- `pnpm test`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` pass.

## ASCII UI preview

Views UI-01, UI-02, and UI-03 in [plan.md](plan.md#ascii-ui-preview). UI-01
excerpt:

```text
Installed agents       [Terminal] [Rescan] [Add TUI agent]
| Claude
| [::] * Work profile      [sonnet] [No fallback]        [dup] [del]
| [::] * Personal          [opus]   [Next model]         [dup] [del]
```

Applies to `AC-AGENTS-PROFILE-LIST-ORDERING-001.1`, `001.2`, `001.7`, and
`001.9`.

## Verification

Run these commands with the Node version pinned in `apps/.node-version` (24).

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/settings/agent-profile-order.test.ts lib/settings/agent-profile-selector-order.test.ts lib/settings/profile-order-queue.test.ts lib/state/slices/settings/settings-slice.test.ts hooks/domains/settings/agent-list-resource.test.ts lib/state/hydration/hydrator.test.ts hooks/domains/settings/use-profile-duplicate.test.ts hooks/domains/settings/use-profile-enabled-toggle.test.ts lib/ws/handlers/agents.test.ts "app/settings/agents/[agentId]/agent-save-helpers.test.ts" "app/settings/agents/[agentId]/agent-save-helpers-provider.test.ts" "app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.test.tsx" app/office/setup/agent-profile-setup-controls.test.tsx "app/settings/agents/page.agent-list-snapshot.test.tsx" components/task-create-dialog-options.test.tsx components/quick-chat/quick-chat-setup.test.tsx components/settings/agents src/settings-routes.test.ts)
(cd apps/web && pnpm test)
(cd apps/web && pnpm run typecheck && pnpm run i18n:pseudo && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm e2e:run -- tests/settings/agent-profile-order.spec.ts tests/settings/agent-profile-layout.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/settings/mobile-agent-profile-order.spec.ts tests/settings/mobile-agent-profile-layout.spec.ts)
(cd apps/web && pnpm e2e:run --project auth -- tests/auth/agent-profile-order-member.spec.ts)
```

## Files likely touched

- `apps/web/lib/settings/agent-profile-order.ts` and `.test.ts`
- `apps/web/lib/settings/profile-order-queue.ts` and `.test.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts` and `types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.test.ts`
- `apps/web/app/actions/agents.ts`
- `apps/web/hooks/domains/settings/use-profile-order.ts`
- `apps/web/hooks/domains/settings/use-profile-duplicate.ts` and `.test.ts`
- `apps/web/lib/ws/handlers/agents.ts` and `.test.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.ts`, `agent-save-helpers.test.ts`, and `agent-save-helpers-provider.test.ts`
- `apps/web/app/office/setup/agent-profile-setup-controls.tsx` and `.test.tsx`
- `apps/web/lib/state/hydration/hydrator.ts` and `hydrator.test.ts`
- `apps/web/hooks/domains/settings/agent-list-resource.ts` and `.test.ts`
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/use-agent-profile-settings.ts` and `.test.tsx`
- `apps/web/lib/state/slices/settings/types.ts` and related tests
- `apps/web/components/task-create-dialog-options.tsx` and `.test.tsx`
- `apps/web/components/quick-chat/quick-chat-setup.tsx` and `.test.tsx`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/agent-profile-order.spec.ts`
- `apps/web/e2e/tests/auth/agent-profile-order-member.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-order.spec.ts`
- `apps/web/e2e/tests/settings/agent-profile-layout.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-layout.spec.ts`
- `docs/public/agents-and-profiles.md`
- Audit and update any other selector consumer that reads `settingsAgents` or a
  direct `GET /agents` response.

## Dependencies

Task 01 (endpoint, event, stored order).

## Risks

- The overlay link in `ProfileRowCard` can swallow handle clicks; keep the
  handle in the `z-10` action layer.
- Optimistic order, the echoed WS event, and rollback must not flip-flop; the
  save queue owns that behavior.
- The e2e backend is worker-scoped: new specs must use dedicated profiles and
  restore order, or later specs that use `profiles[0]` see a leaked order.
- Keep the drag-row component within the size limits in
  `apps/web/AGENTS.md`; do not expand `ProfileRowCard`.

## Parallelism

`sequential`

## Inputs

- System design sections Frontend, Store updates, Save coordination, and Failure
  and recovery.
- `components/task/sidebar-filter/automatic-color-rule-list.tsx` and
  `automatic-color-rule-card.tsx` for dnd-kit setup and the translated
  role description.
- `docs/i18n.md` for locale rules.

## Results

The results below are historical contributor reports from before integration
with current main. They are not verification of the combined review head.

The revised implementation removes the automatic name-sort action and limits
saved ordering to Settings > Agents and its navigation tree. Profile selectors
retain their creation baseline, context-specific recent-use ranking, defaults,
and workspace-scoped option slots.

The focused profile-order suite passed 20 files and 233 tests under pinned
Node 24.19.0. Full web lint, typecheck, i18n checks and ratchet, formatting,
desktop/mobile E2E, public-doc validation, and specification validation passed.
The implementation subtask reported the auth E2E flow passing. The Settings
route fallback assertion now checks only `items` and `version`.

`mise exec -- pnpm test` under Node 24 failed: 44 files, 70 tests, 21,543
passed, and 4 skipped. Reported failures did not include the changed
profile-order test files; they included timeouts and test-fixture connection
errors. Keep this broad-suite failure visible; the focused profile-order suite,
selector E2E, and reorder E2E passed.

## Integration review validation

The review fixup preserves current-main catalogue publication, enabled-intent
persistence, capability/runtime metadata, and Settings model-picker behavior.
It adds regression coverage for creation baselines during pending drags and
late acknowledgements, terminal rollback, the 15-second request timeout,
malformed timestamp handling, Office option preservation, and localized drag
announcements. Desktop handles use the shared 28 px size; phone and coarse
pointer handles retain 44 px hit targets. Backend regressions cover canceled
repository reads, equal/missing ranks, and create/delete after saved ordering.

Focused frontend tests run in a credential-free, nonroot, network-disabled
container. Broad frontend/backend, race, lint, build, typecheck, and rendered
desktop/mobile checks remain assigned to hosted CI. Host hooks and contributor
tooling are not executed during this review. The historical broad-suite
failures above remain visible; they are not attributed to unrelated tests
without exact source and log evidence.


## Corrective validation after PR #4373

Original-head hosted frontend tests found accepted new owners missing after an
independent profile event, obsolete Settings row-index expectations, and a page
fixture missing the actual store API. PR #4399 restores the new-owner create
publication, including accepted partial MCP results, while retaining the
missing-existing-owner guard. The owning app store observes owner removal during
save and prevents a late response from reinserting that observed identity,
including after the editor unmounts. Observations are scoped to each store.
Accepted local deletion advances the snapshot epoch; a held pre-deletion GET
must fail its epoch guard rather than reintroducing the deleted choice.

Settings-only prepend expectations are updated by identity, retaining catalogue
counts, capability metadata, independent options, rejection state and picker
assertions. The selector E2E trace reached Quick Chat and waited for a Cancel
button absent from that surface. It now dismisses the dialog with Escape;
selector equality, defaults and recent-use assertions remain unchanged.

Focused isolated hook regression: observed-removal RED (1 failed, 12 passed),
then GREEN (13 passed), plus 17 real MCP component cases passed. The larger
creation/deletion/page React suites are locally unavailable because the minimal
private cache lacks terminal/floating-popup dependencies. Hosted frontend and
selector E2E checks remain required before this work order can be completed.
Original-head desktop/mobile captures are historical evidence, not proof for
corrective source. No copy, layout, touch or locale changes are introduced.

The recovered desktop capture came from retry 1. Its first attempt received a
null bounding box after visibility succeeded, before either 28 px dimension
could be checked. The fixture now polls actual rendered height and width with
the same 28 px values and precision. It retains the existing test timeout,
retry policy, keyboard reorder, Escape cancellation, persistence, and cursor
assertions. Fresh hosted geometry and interaction evidence remains required.

An additional real-provider lifecycle regression reproduced owner resurrection
after editor unmount: RED (1 failed, 14 passed), then GREEN (15 passed). The
removal observation now starts in `createAppStore`, uses store-lifetime weak
keys, and retains the current setters, snapshot epochs, hydration payloads, and
catalogue metadata. A separate store remains eligible to publish its new owner.

Additional corrective files:

- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.test.tsx`
- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`
- `apps/web/app/settings/agents/[agentId]/agent-create-catalogue.test.tsx`
- `apps/web/app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx`
- `apps/web/app/settings/agents/page.test.tsx`
- `apps/web/components/settings/custom-tui-mcp-card.test.tsx`
- `apps/web/components/settings/agents/agent-profiles-section-delete-inventory.test.tsx`
- `apps/web/e2e/tests/settings/agent-profile-order-selectors.spec.ts`
- `apps/web/e2e/tests/settings/agent-profile-order.spec.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/settings-agent-removals.ts`
