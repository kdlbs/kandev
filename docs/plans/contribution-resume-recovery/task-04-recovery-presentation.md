---
id: "04-recovery-presentation"
title: "Unify recovery presentation"
status: completed
wave: 4
depends_on:
  - "03-bootstrap-projection"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 04: Unify recovery presentation

## Summary

Create one shared recovery view model using existing durable projection and request state. Make task detail, preview, Quick Chat, transcript, and composer agree on the current owner. Reuse existing recovery handlers and valid secondary choices. Localize all new copy.

## In scope

Create one shared recovery view model using existing durable projection and request state. Make task detail, preview, Quick Chat, transcript, and composer agree on the current owner. Reuse existing recovery handlers and valid secondary choices. Localize all new copy.

## Out of scope

Unrelated historical error suppression, hidden recovery actions, initial creation redesign, new global error store, and changes to recovery authorization.

## Acceptance

- UI-01/UI-02 show one correlated card and one action owner; UI-03 has no recovery error. Unrelated historical/provider failures remain visible.
- UI-04 updates the same card across pending, dual failure, workspace-only success, and successful resume, with navigation/archive generation guards.
- Desktop uses compact controls; phone uses stacked touch actions, semantic details, existing scroll ownership, and safe-area clearance.

## ASCII UI preview

### UI-01: Task Chat, blocked resume
Entry: selected task/session, automatic or manual resume failed.
The supplied screenshot shows three presentations of one failure:
```text
BEFORE
[Top: Session recovery failed                 Retry]
[Transcript: Agent has encountered an error + Git output]
[Composer: same Git output       Resume | Start fresh]

AFTER / DESKTOP
[Task header and session tabs]
[Transcript history ...]
[Could not resume the session]
[Source repository access could not be verified.]
[Retry resume] [Restore workspace] [More options]
[> Details]
[Composer reflects stopped state, no duplicate warning]
```
The cause above illustrates an access failure; confirmed history-only rejection
instead follows UI-03. Never claim access denial without matching evidence.

### UI-02: Phone Chat, blocked resume
```text
[Back] [Task / session]
[Transcript history ...]
[Could not resume the session]
[Short cause, wrapping]
[ Retry resume            ]
[ Restore workspace       ]
[ More options            ]
[ > Details               ]
[Safe-area clearance]
```
More options retains valid existing fresh-start and branch-loss recovery flows;
fresh start is secondary and keeps its existing confirmation. No new drawer is
needed for the card. Use the existing responsive menu for secondary choices.

### UI-03: Successful resume after remote history update
```text
[Task / session]
[Transcript and active composer]
[Changes: existing provider/local history state and version actions]
```
No agent-error card or recovery banner is produced by the history-only rejection.

### UI-04: Pending, details, and fallback states
```text
PENDING: [Resuming...] [Restore workspace disabled] [> Details]
EXPANDED:
  Resume: <safe cause>
  Workspace restore: <safe cause, only if attempted>
RESTORE SUCCEEDED:
  [Workspace available. Agent remains stopped.]
  [Retry resume] [> Details]
RESUME SUCCEEDED:
  [Normal transcript and composer; active card removed]
```
The card stays inline in the transcript scroll owner. Details wrap without
their own scroller. Existing phone layout owns dynamic viewport and safe-area
spacing. Desktop buttons measure 28px; phone/coarse-pointer targets are at least
44px. Control order, one-owner behavior, scroll ownership, and state changes
are required. Exact wording and spacing are illustrative and must use localized
keys. UI-01/02/04 map to recovery requirement 006; UI-03 maps to contribution
requirement 002.

Full combined preview: [plan.md](plan.md#ascii-ui-preview).

## Regression and verification

Write selector/component regressions first: same stamp across three inputs, missing correlation, wrong session, prior runtime error, reverse arrival, pending retry, navigation away/back, archive, dual failure, and success. Update affected existing tests, including Quick Chat and preview. Details contain sanitized causes; no collapsed summary contains raw nested transport output.

Run from the repository root. Use the listed test names for new regressions.
If any existing test is changed beyond this list, add its exact command here
before marking results complete.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-recovery-presentation.test.ts lib/session-last-agent-error.test.ts lib/types/task-status-summary.test.ts components/task/task-launch-error-context.test.tsx components/task/task-chat-panel.launch-error.test.tsx components/task/ensure-session-error.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/simple/components/task-launch-error-entry.test.tsx components/task/preview-session-tabs.test.tsx components/quick-chat/quick-chat-session-view.test.tsx hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.archive.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

## Files likely touched

- `apps/web/components/task/task-launch-error-context.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/preview-session-tabs.tsx`
- `apps/web/components/task/ensure-session-error.tsx`
- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/components/task/simple/components/task-launch-error-entry.tsx`
- `apps/web/components/quick-chat/quick-chat-session-view.tsx`
- `apps/web/hooks/domains/session/use-session-resumption-operations.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/lib/session-last-agent-error.ts`
- `apps/web/lib/session-recovery-presentation.ts (new shared pure selector, with tests)`
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`

## Dependencies

[Task 03](task-03-bootstrap-projection.md).
Execution is sequential.

## Inputs

- [Package evidence and design](plan.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- [System design](../../specs/tasks/system-design/task-launch-failure-recovery.md).
- Applicable REQ and AC identifiers in frontmatter.
- Read scoped AGENTS.md and the existing adjacent tests before implementation.

## Risks

Multiple recovery hooks can own separate busy state. Deduplicate actions by current attempt, not text. Old errors need safe fallback without hiding history.

## Parallelism

`sequential`

## Results

Completed. Task detail, preview, Quick Chat, transcript, and composer now use
one correlated bootstrap recovery-card owner. The card exposes localized safe
causes, bounded optional details, resume, read-only workspace restore, and
fresh-session actions while preserving unrelated runtime errors and existing
archive and branch-loss flows. Mobile controls are stacked with touch-safe
targets and the existing scroll owner.

Verification passed:

- Focused Vitest coverage: 13 files, 143 tests passed.
- `pnpm run typecheck`
- `pnpm --filter @kandev/web lint`
- `pnpm run i18n:check`
- `pnpm run i18n:ratchet`
- Traditional Chinese generation via
  `pnpm exec node scripts/convert-zh-cn-to-zh-hant.mjs --locale all --write --namespace task`
- Changed Playwright-file ESLint checks.

Review remediation verification covers the automatic hook outcomes and the
manual retry path in the shared owner:

- Focused recovery Vitest run: 5 files, 72 tests passed, including detail,
  preview, Quick Chat, workspace-only success, dual automatic failure, and
  repeated manual restore failure.
- Full web Vitest sweep: 1,956 files, 16,767 tests passed, 4 skipped.
- `pnpm run lint` with zero warnings, `pnpm run typecheck`,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet`
- `pnpm --filter @kandev/web build:vite` and `pnpm run build:e2e`
