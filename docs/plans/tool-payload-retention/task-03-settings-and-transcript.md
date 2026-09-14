---
id: "03-settings-and-transcript"
title: "Add retention settings and removed tool details"
status: completed
wave: 3
depends_on:
  - "02-guarded-cleanup"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.1
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.5
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.6
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.7
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.1
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.6
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.7
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.5
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.6
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 03: Settings and Transcript

## Summary

Deliver the analysis, activation, and cleanup flow on desktop and phone. Show
removed tool details explicitly in conversations.

## In scope

- Add the independent card, API types/client/hook, discovery target, and route save
  contributor. Preserve unsaved drafts during status refresh and handle conflicts.
- Add optional analysis, stale/partial results, explicit backup choice, preparation,
  retry/cancel, enable/disable, Run cleanup now, and last/next status.
- Keep the Office card independent. Explain payload bytes versus compaction and
  backup storage. Show engine capability and read-only admin restrictions.
- Add removed-detail rendering for all supported tool kinds. Evict stale lazy
  output when committed markers arrive. Test cached output and reload behavior.
- Localize all copy in five languages. Generate Traditional Chinese using the
  repository workflow. Add desktop, phone, and member-gating browser coverage.

## Out of scope

Daily scheduler implementation, new compaction behavior, and database audits.

## ASCII UI preview

The [combined previews](plan.md#ascii-ui-preview) are authoritative. These excerpts
retain the same labels. Sample counts and spacing are illustrative.

UI-01, desktop disabled policy:

```text
Tool payload cleanup                                  [Off]
Tasks inactive for [3] [Months v]       [Analyze savings]
Estimated payload reduction: <size> | <messages> messages
Last cleanup: Never                    Next check: Disabled
```

UI-02, phone first-cleanup review:

```text
< System                  Data & Logs
Tool payload cleanup          [On*]
Tasks inactive for
[3                                 ]
[Months                           v]
[Analyze savings                   ]
Before the first cleanup
( ) Create backup (recommended)
( ) Continue without backup
[Discard]                     [Save]
```

UI-03, shared states: show scan progress/cancel, not analyzed/stale/empty,
backup preparing/failed/retry, and cleanup partial/failed/completed status.
UI-04, shared conversation row:

```text
<original tool title>             <original status>
Tool details removed on <date>.
<retained summary>
```

Structure is required. The phone uses the route scroll owner, stacked fields,
44px touch targets, and safe-area-aware shared Save bar. No layered dialogs.
Match all referenced acceptance criteria through rendered tests.

## Acceptance

1. Users can analyze without enabling, review backup choices, save, observe
   preparation/cleanup, and reload persistent state on desktop and phone.
2. Read-only users cannot mutate or trigger analysis. Pending, stale, error, and
   unsupported states are explicit. Office retention remains independent.
3. Every supported removed tool row retains its title/status and reports removed
   details. Cached output cannot reappear after marker delivery or reload.

## Tests and verification

Use `/tdd`, `/mobile-parity`, and `/e2e`. Bootstrap workspace dependencies once
if absent. Run commands independently from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/system/tool-payload-retention-card.test.tsx hooks/domains/system/use-tool-payload-retention.test.ts)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/tool-execute-message.test.tsx hooks/domains/session/use-shell-command-output.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/system/tool-payload-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-tool-payload-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project auth tests/auth/system-data-storage-member-gating.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/auth/mobile-system-data-storage-member-gating.spec.ts)
git diff --check
```

Add equivalent removed-state tests for other supported tool renderers in their
owning test files. Record additional targeted commands with the implementation.

## Files likely touched

- `apps/web/components/settings/system/data-logs-settings.tsx`,
  `tool-payload-retention-card.tsx`, and tests.
- `apps/web/hooks/domains/system/use-tool-payload-retention.ts` and tests.
- System API client/types, job state, settings discovery, and locale catalogs.
- Tool-message renderers, `shell-output-disclosure.tsx`,
  `hooks/domains/session/use-shell-command-output.ts`, and their existing tests.
- The desktop/mobile E2E files and existing auth member-gating specs.

## Dependencies and parallelism

Depends on Task 02. Frontend work uses the agreed backend contract. Share policy/job logic across desktop and phone.

## Inputs

Read the linked design, plan UI-01 through UI-04, settings save coordinator,
`RetentionSettingsCard`, and nearest mobile data-storage tests before editing.

## Risks

A switch draft is not effective enablement. Save success with pending backup
must not display cleanup as enabled. Partial results must not resemble success.

## Results

Implemented the independent Data & Logs card, shared Save/Discard contributor,
API client and hooks, analysis states, explicit backup choice, cleanup controls,
and removed-detail transcript rendering. Office retention remains independent.

- 90 focused component, hook, renderer, and message-store tests passed. After
  lint refinements, all 29 affected tests passed again.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`: passed. Full web lint reports zero warnings/errors.
- English, Portuguese, Simplified Chinese, both generated Traditional Chinese
  catalogs, and pseudo-locale include the new states and skip reasons.
- Browser tests cover backup and explicit skip activation, actual removed
  metadata, original backup contents, protected recent activity, and persistence.
  Phone flows cover 320px and 390px, touch targets, and horizontal overflow.
- Member/admin browser tests exercise both system routes and server-side denial
  of retention analysis and mutations. Final browser totals are in the plan.
