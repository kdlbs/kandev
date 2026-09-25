---
id: "01-wire-adapter"
title: "camelCase/snake_case adapter and cron-reconcile control flow"
status: done
wave: 0
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-WIRE-001
  - REQ-OFFICE-ROUTINE-WIRE-002
  - REQ-OFFICE-ROUTINE-WIRE-003
  - REQ-OFFICE-ROUTINE-WIRE-004
acceptance_criteria:
  - AC-OFFICE-ROUTINE-WIRE-001.1
  - AC-OFFICE-ROUTINE-WIRE-001.2
  - AC-OFFICE-ROUTINE-WIRE-001.3
  - AC-OFFICE-ROUTINE-WIRE-001.4
  - AC-OFFICE-ROUTINE-WIRE-001.5
  - AC-OFFICE-ROUTINE-WIRE-001.6
  - AC-OFFICE-ROUTINE-WIRE-001.7
  - AC-OFFICE-ROUTINE-WIRE-001.8
  - AC-OFFICE-ROUTINE-WIRE-001.9
  - AC-OFFICE-ROUTINE-WIRE-001.10
  - AC-OFFICE-ROUTINE-WIRE-001.11
  - AC-OFFICE-ROUTINE-WIRE-002.1
  - AC-OFFICE-ROUTINE-WIRE-002.2
  - AC-OFFICE-ROUTINE-WIRE-002.3
  - AC-OFFICE-ROUTINE-WIRE-002.4
  - AC-OFFICE-ROUTINE-WIRE-002.5
  - AC-OFFICE-ROUTINE-WIRE-002.6
  - AC-OFFICE-ROUTINE-WIRE-002.7
  - AC-OFFICE-ROUTINE-WIRE-002.8
  - AC-OFFICE-ROUTINE-WIRE-002.9
  - AC-OFFICE-ROUTINE-WIRE-002.10
  - AC-OFFICE-ROUTINE-WIRE-002.11
  - AC-OFFICE-ROUTINE-WIRE-002.12
  - AC-OFFICE-ROUTINE-WIRE-002.13
  - AC-OFFICE-ROUTINE-WIRE-002.14
  - AC-OFFICE-ROUTINE-WIRE-003.1
  - AC-OFFICE-ROUTINE-WIRE-003.2
  - AC-OFFICE-ROUTINE-WIRE-003.3
  - AC-OFFICE-ROUTINE-WIRE-003.4
  - AC-OFFICE-ROUTINE-WIRE-003.5
  - AC-OFFICE-ROUTINE-WIRE-003.6
  - AC-OFFICE-ROUTINE-WIRE-003.7
  - AC-OFFICE-ROUTINE-WIRE-003.8
  - AC-OFFICE-ROUTINE-WIRE-003.9
  - AC-OFFICE-ROUTINE-WIRE-003.10
  - AC-OFFICE-ROUTINE-WIRE-003.11
  - AC-OFFICE-ROUTINE-WIRE-003.12
  - AC-OFFICE-ROUTINE-WIRE-003.13
  - AC-OFFICE-ROUTINE-WIRE-003.14
  - AC-OFFICE-ROUTINE-WIRE-003.15
  - AC-OFFICE-ROUTINE-WIRE-003.16
  - AC-OFFICE-ROUTINE-WIRE-003.17
  - AC-OFFICE-ROUTINE-WIRE-003.18
  - AC-OFFICE-ROUTINE-WIRE-003.19
  - AC-OFFICE-ROUTINE-WIRE-004.1
  - AC-OFFICE-ROUTINE-WIRE-004.2
  - AC-OFFICE-ROUTINE-WIRE-004.3
  - AC-OFFICE-ROUTINE-WIRE-004.4
  - AC-OFFICE-ROUTINE-WIRE-004.5
system_design:
  - ../../specs/office/system-design/routine-wire-contract.md
---

# Task 01: camelCase/snake_case adapter and cron-reconcile control flow

## Summary

The routines UI sent and read camelCase against a snake_case backend
contract, so multi-word fields (assignee, policies, task template, cron
schedule, declared variables) were silently discarded on write and rendered
as blanks, placeholders, or one character per line on read, and a cron
trigger could not be armed at all. This task adds a single adapter pair at
the client's API boundary — a request builder that emits the contract's
exact snake_case keys and a response normalizer that produces camelCase
models with no snake_case fallback and no unchecked cast — plus the
cron-reconcile control flow that replaces an armed trigger correctly when
its schedule changes.

## In scope

- `apps/web/lib/api/domains/office-routine-api.ts`: request builders for
  create/update routine and create routine trigger, emitting only the keys
  and shapes `apps/backend/internal/office/routines/dto.go` binds.
- `apps/web/lib/api/domains/office-routine-normalize.ts`: the response-side
  adapter — envelope unwrap, JSON-string decode for `task_template` and
  `variables`, and the routine/trigger/run camelCase mapping.
- `apps/web/lib/api/domains/office-api.ts`: wires the two modules above into
  the existing exported API surface.
- `apps/web/app/office/routines/[id]/cron-reconcile.ts`: create-before-delete
  trigger replacement, the two-cron-trigger warning, and the failed-relist
  message.
- `apps/web/app/office/routines/{routine-row.tsx, routines-content.tsx,
  create-routine-dialog.tsx, [id]/routine-detail-view.tsx, [id]/page.tsx}`,
  `apps/web/app/office/agents/[id]/layout.tsx`,
  `apps/web/app/office/lib/catch-up-max.ts`,
  `apps/web/src/office-routine-client-routes.tsx`: consume model-shape
  fields exclusively; `create-routine-dialog.tsx`'s `onSubmit` returns
  `Promise<boolean>` so a rejected create leaves the dialog open.
- `apps/web/app/office/routines/routine-wire-scan.test.ts`: the static
  no-snake_case / no-`as unknown as` scan required by AC-003.10, and the
  closed-set assertion required by AC-003.18.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/office.json`: the
  new trigger-replacement and stale-schedule toast copy (AC-002.12).
- `apps/web/e2e/tests/office/routines-ui.spec.ts` and the routine
  catch-up/fire/agentctl-cli E2E suites touched by the model-shape change.

## Acceptance

Every acceptance criterion under REQ-OFFICE-ROUTINE-WIRE-001 through
REQ-OFFICE-ROUTINE-WIRE-004 in
[the requirements](../../specs/office/requirements/routine-wire-contract.md),
listed in this file's frontmatter.

## Verification

```bash
cd apps/web
pnpm run typecheck
pnpm run lint
pnpm run test -- app/office/routines lib/api/domains/office-routine-api.test.ts lib/api/domains/office-routine-normalize.test.ts
pnpm run i18n:ratchet
pnpm run i18n:check
pnpm run build
pnpm run e2e:run -- routines-ui routine-catch-up-policy-ui routine-fire
```

## Files likely touched

See `## In scope` above; full diff stat against the branch's merge-base:
`git diff --stat $(git merge-base HEAD origin/main)..HEAD`.

## Dependencies

None — single task, single wave.

## Parallelism

Not applicable: the request builder, response adapter, and the components
that consume them change together behind one contract.

## Results

Shipped on `feature/routines-ui-camelcas-rf9`. Four independent review
rounds (code-reviewer, security-reviewer, test-supervisor, and a
cross-vendor outside voice) converged with no residual defect. The final
round's one verified new finding — a React crash on a non-string
`task_template.title`/`.description`, reachable only because this branch is
the first time that field's content reaches JSX — was fixed with a
`templateText` guard and regression test. A second, pre-existing
finding surfaced by that same round's test work — the create dialog
resetting and losing the operator's input on a rejected `createRoutine` call
— was fixed by making `onSubmit` report success/failure and only
closing/resetting on success, with regression coverage in
`routines-content.test.tsx`.
