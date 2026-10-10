---
id: "01-restore-new-agent-publication"
title: "Restore new-agent publication and CI compatibility"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CREATION-CATALOGUE-001
acceptance_criteria:
  - AC-AGENTS-CREATION-CATALOGUE-001.1
  - AC-AGENTS-CREATION-CATALOGUE-001.2
  - AC-AGENTS-CREATION-CATALOGUE-001.3
  - AC-AGENTS-CREATION-CATALOGUE-001.4
  - AC-AGENTS-CREATION-CATALOGUE-001.5
  - AC-AGENTS-CREATION-CATALOGUE-001.6
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
---

# Task 01: Restore new-agent publication and CI compatibility

## Summary and gates

Mark new-agent accepted publication explicitly so an unrelated profile-version
advance cannot discard it. Reconcile the ten exact stale fixture expectations,
preserving current profile-order policy and genuine membership/configuration
assertions. DESIGN ONLY until ROOT full actual-file review and LATER release,
including explicit source-integration choice described by [manifest](plan.md).
SAME primary/task/session, one sequential worker/no file parallelism/delegates.

## Inputs

- [Unchanged requirement](../../specs/agents/requirements/creation-catalogue.md), all six creation ACs.
- [Design](../../specs/agents/system-design/creation-catalogue.md#new-agent-publication-across-profile-order-reconciliation).
- [Existing-owner guard](../../specs/agents/system-design/target-profile-creation-catalogue.md#accepted-creation-publication).
- Exact saved tested/main/candidate source deltas and full final Frontend log,
  corrected `frontend-failure-classification.json`; original hosted actual joins.

## Files and ownership

Only production edits after release:

- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`.
- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.ts`.

Affected existing tests:

- `apps/web/app/settings/agents/[agentId]/agent-create-catalogue.test.tsx` (four failures).
- `apps/web/app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx` (one).
- `apps/web/components/settings/custom-tui-mcp-card.test.tsx` (two).
- `apps/web/components/settings/agents/agent-profiles-section-delete-inventory.test.tsx` (three).
- `apps/web/app/settings/agents/page.test.tsx` (two).
- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.test.tsx` and
  `apps/web/app/settings/agents/[agentId]/agent-save-helpers.test.ts`: changed callback/guard compatibility.
- Read-only integrated `apps/web/app/settings/agents/[agentId]/agent-save-store-sync.test.tsx`
  (confirm actual integrated path; checkpoint a mismatch rather than omit its guards).

Artifacts: owning design, this order/manifest; requirement reused unchanged.
No edits to ordinary save-sync, page version admission, state actions, handlers,
profile-order helpers or Global/scroll ownership. ROOT-authorized normal base
integration is recorded separately from owned fix paths; never hide its actual
tracked/cached/untracked NUL inventory or foreign edits. Preserve worktrees/deps/
caches/refs/processes/protected sources and all previous native receipts.

## Acceptance

1. Two exact new-owner cases fail causally against the integrated tested/main
   contract while no-event/rejection controls pass; GREEN retains accepted owner,
   normalized profiles/MCP draft/independent selectable choice, unique persisted
   identities and existing shared-save/navigation behavior.
2. Only two production files change. Explicit new-agent provenance is distinct
   from ordinary save/existing-owner creation. Existing missing-owner/version
   guards and current ordering/metadata/option retention remain effective.
3. All twelve CI cases plus bounded changed-helper/guard controls pass with no
   skip/assertion weakening. Ten fixture changes express current contracts, not
   product changes. Record full actual checks/joins/current owned cleanup; batch
   with reviewed scroll fix for one normal publication only after all required work.

## Verification

Only after ROOT review/release/authorized source integration, Node24/retained deps:

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx' -t 'publishes a newly created agent|publishes a new agent partial MCP result|accepts profile creation without an intervening event|rejects creation while preserving the live choice and newer draft')
(cd apps/web && corepack pnpm@9.15.9 exec vitest run 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx' 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx' components/settings/custom-tui-mcp-card.test.tsx components/settings/agents/agent-profiles-section-delete-inventory.test.tsx app/settings/agents/page.test.tsx hooks/domains/settings/use-agent-creation-store-sync.test.tsx 'app/settings/agents/[agentId]/agent-save-helpers.test.ts' 'app/settings/agents/[agentId]/agent-save-store-sync.test.tsx')
(cd apps/web && corepack pnpm@9.15.9 exec eslint 'app/settings/agents/[agentId]/agent-save-helpers.ts' hooks/domains/settings/use-agent-creation-store-sync.ts 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx' 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx' components/settings/custom-tui-mcp-card.test.tsx components/settings/agents/agent-profiles-section-delete-inventory.test.tsx app/settings/agents/page.test.tsx hooks/domains/settings/use-agent-creation-store-sync.test.tsx 'app/settings/agents/[agentId]/agent-save-helpers.test.ts' --max-warnings 0)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

First command is RED selection before production. Confirm actual existing control
names after integrated source identity; add the independently missing no-event
new-owner control before RED if needed. No new browser check for data-only scope.
Run every actually changed test suite; preflight all actual paths/refs/coverage
before delivery. Retain full original native results/every yielded session_id/
actual joins. Normal hooks, no bypass; stage any reviewed new TS helper before
final i18n ratchet. Batch necessary fixes, never push with unresolved material loss.

## Out of scope

Global/state/handler/profile-order/ordinary-save redesign, broad suites/typecheck/
build/mobile browser, installs or cleanup, auth/API/backend, layout/copy/operator
steps, old-head reruns, manual FULL BOT per SHA. No new task/session/model.

## Mobile exception

State/data publication and fixture compatibility only, shared existing page/Save/
picker; no interaction/composition change. Existing phone controls remain. Expand
the package before any layout, touch, navigation or scroll change.

## Dependencies

Explicit reviewed source-integration choice and later implementation release.
Candidate lacks the tested version gate; do not claim candidate-only RED proves it.

## Risks

New-agent provenance must not allow existing-owner resurrection. Respect existing
ordinary save/version guards and accepted new-first order. Resolve only actual
owned integration conflicts; no silent all-main source transplantation.

## Parallelism

`sequential`

## Results

DESIGN_READY; lightweight checks and ROOT full actual-file review complete; see
[manifest results](plan.md#verification-results). LATER explicit release and
heavy admission pending, including the reviewed exact02ff057 normal merge.
No Agents production/tests/integration/runtime changed.
