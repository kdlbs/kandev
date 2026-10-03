---
id: "02-header-recovery"
title: "Configuration Chat header recovery flow"
status: done
wave: 2
depends_on:
  - "01-backend-restart"
plan: "plan.md"
requirements:
  - REQ-TASKS-CONFIG-CHAT-RESTART-001
acceptance_criteria:
  - AC-TASKS-CONFIG-CHAT-RESTART-001.1
  - AC-TASKS-CONFIG-CHAT-RESTART-001.2
  - AC-TASKS-CONFIG-CHAT-RESTART-001.3
  - AC-TASKS-CONFIG-CHAT-RESTART-001.4
  - AC-TASKS-CONFIG-CHAT-RESTART-001.5
  - AC-TASKS-CONFIG-CHAT-RESTART-001.6
  - AC-TASKS-CONFIG-CHAT-RESTART-001.7
  - AC-TASKS-CONFIG-CHAT-RESTART-001.8
  - AC-TASKS-CONFIG-CHAT-RESTART-001.9
system_design:
  - ../../specs/tasks/system-design/configuration-chat-restart.md
---

# Task 02: Header recovery flow

## Summary

Connect the backend operation to a localized header control and shared
Configuration Chat state. Deliver desktop and phone confirmation, busy/error
feedback, and a usable blank replacement, with unit and browser evidence.

## In scope

- Typed restart client, deletion preflight ticket, stage-aware errors, and
  uncertain-outcome reconciliation without blind POST retries.
- Shared start/restart admission, stale-result protection, cache/descriptor
  replacement, suppression of retiring-session effects, and safe close/workspace
  changes. Accepted restart must not use abandoned-setup task cleanup.
- Consume the list's pending/retiring-session projection through
  `use-quick-chat-resync` so a lost response or another browser's restart cannot
  turn the temporary absence of a session into permission to create another.
- Header control and shared responsive confirmation, including fixed header,
  phone touch sizes, disabled setup state, progress and mounted-session errors.
- Six real locales, generated pseudo/Traditional Chinese, desktop/mobile E2E.
- Update Configuration Chat recovery guidance in `docs/public/developer-tools.md`
  and its feature-status row. Reconcile the no-new-session-action wording in
  `quick-chat-expiration.md` with replacement-only recovery. Do not claim more
  than one configuration conversation is supported.

## Out of scope

An expanded-dialog restart button, broad Quick Chat redesign, new confirmation
primitives, automatic retries of destructive requests, and unrelated settings.

## Acceptance

1. Header confirmation and Cancel work without losing the existing draft.
   Confirmed restart shows progress, prevents old-session sends/resume, replaces
   only the captured config chat, and clears every old prompt/history input.
2. Deferred hook tests cover duplicate clicks and cross-hook starts, stop/delete
   errors with the old view present, creation/start errors, response loss,
   close/workspace changes, late deletion events, and pre-hydrated replacement
   deduplication. Expanded Quick Chat/reload use the same replacement.
3. Fresh-build E2E proves a new prompt succeeds after restart, old history stays
   deleted, cancel and retry work, and phone bottom confirmation and controls
   are contained, accessible, and at least 44px. Inspect rendered phone output
   against UI-01/UI-02 and test desktop sizing and breakpoint boundaries.

## ASCII UI preview

UI-01/UI-02 excerpt, Settings panel with an existing conversation (AC .1-.2,
.5-.9). See the [combined previews](plan.md#ascii-ui-preview).

```text
Desktop: Configuration Chat  [Restart] [Expand] [Close]
Phone:   Configuration...    [R]       [Expand] [Close]

Desktop anchored / phone bottom confirmation:
+------------------------------------------------+
| Restart session?                               |
| Delete this conversation and start a new one.   |
| [Cancel]                     [Restart session] |
+------------------------------------------------+

During restart: fixed header, Restart/Expand disabled,
one "Restarting session..." status, no active composer.
Failure: visible cause + recovery action in the panel.
```

R represents the restart icon; use a stable localized accessible name. Same
control order on both viewports, 28px fine-pointer desktop and at least 44px
phone/coarse-pointer hitboxes. Phone confirmation uses the shared inset Drawer
and its safe-area/internal-scroll behavior. Prose/spacing are illustrative.

## Verification

Run from the repository root. Install once if workspace dependencies are absent.
Use `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test components/config-chat/use-config-chat.test.ts components/config-chat/config-chat-panel.test.tsx components/config-chat/config-chat-copy.test.ts lib/api/domains/workspace-api.test.ts components/quick-chat/quick-chat-session-view.test.tsx hooks/use-quick-chat-resync.test.ts lib/state/slices/ui/quick-chat-sync.test.ts lib/state/slices/ui/quick-chat-actions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/config-chat lib/api/domains/workspace-api.ts lib/api/domains/workspace-api.test.ts components/quick-chat/quick-chat-session-view.tsx hooks/use-quick-chat-resync.ts lib/state/slices/ui/quick-chat-sync.ts lib/state/slices/ui/quick-chat-actions.ts lib/state/slices/ui/types.ts lib/state/slices/ui/ui-slice.ts lib/state/app-state-types.ts e2e/helpers/config-chat-restart.ts e2e/tests/settings/config-chat-restart.spec.ts e2e/tests/settings/mobile-config-chat-restart.spec.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/config-chat-restart.spec.ts tests/settings/config-chat-popover.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-config-chat-restart.spec.ts tests/settings/mobile-configuration-chat.spec.ts tests/settings/mobile-config-chat-popover.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run desktop and mobile managed builds sequentially; preserve one worker per
shard and verify discovered counts. If the implementation changes an additional
suite, add its exact command before marking results complete. After all checks,
promote the paired requirement/design and mark the plan implemented.

## Files likely touched

- `apps/web/components/config-chat/config-chat-panel.tsx`
- `apps/web/components/config-chat/config-chat-header.tsx` (new if extracted)
- `apps/web/components/config-chat/config-chat-restart-confirmation.tsx` (new)
- `apps/web/components/config-chat/use-config-chat.ts` and its test
- `apps/web/components/config-chat/use-config-chat-restart.ts`,
  `config-chat-recovery.ts`, and `config-chat-operations.ts` (new)
- `apps/web/components/config-chat/config-chat-panel.test.tsx` (new)
- `apps/web/components/config-chat/config-chat-copy.test.ts`
- `apps/web/lib/api/domains/workspace-api.ts` and its test
- `apps/web/components/quick-chat/quick-chat-session-view.tsx` and its test
- `apps/web/hooks/use-quick-chat-resync.ts` and its test
- `apps/web/lib/state/slices/ui/quick-chat-sync.ts` and its test
- `apps/web/lib/state/slices/ui/` only for shared transient retirement state,
  with adjacent tests if needed; no second durable chat store
- `apps/web/lib/state/app-state-types.ts` and `apps/web/AGENTS.md`
- `apps/web/e2e/helpers/config-chat-restart.ts` (new)
- `apps/web/src/locales/*/configChat.json`
- New `apps/web/e2e/tests/settings/config-chat-restart.spec.ts` and
  `mobile-config-chat-restart.spec.ts`
- `docs/public/developer-tools.md`, `docs/public/feature-status.md`
- Paired specs and this plan's completion records

## Dependencies

Task 01 must provide the restart endpoint and stable stage/identity response.

## Risks

`reset()` currently cleans up superseded starts; accepted restart needs different
ownership. Existing panel errors render only in setup. Adding a button without
moving error/progress ownership would hide failures while the old session exists.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/configuration-chat-restart.md)
- `apps/web/AGENTS.md`, `components/confirmation/AGENTS.md`
- Current config-chat start tests and desktop/mobile Settings chat E2E.

## Results

Completed on 2026-10-01 in the primary session.

- RED: restart API, shared admission, confirmation and recovery tests initially
  lacked implementation. Additional response-loss tests exposed retained old
  drafts/cache, and browser assertions exposed an unsolicited startup response.
- GREEN: all eight listed frontend suites passed (123 tests). The replacement
  clears retired local storage, messages, queued prompts and pending questions,
  even when the deletion event is missed. Uncertain outcomes reconcile through
  GET without replaying the destructive POST or allowing another start.
- Type checking, scoped ESLint, all six real locale catalogs, generated
  Traditional Chinese/pseudo catalogs, i18n checks and the copy ratchet passed.
- Fresh managed browser builds/runs passed sequentially with one worker:
  desktop 12/12 and mobile 4/4. Coverage includes blank startup, a new prompt,
  running-agent stop, disabled auto-start, passthrough launch, failure/retry,
  expansion, reload, draft-preserving Cancel and breakpoint changes.
- UI-01/UI-02 visual inspection passed: desktop anchored confirmation and 28px
  controls; phone inset bottom confirmation, 44px controls/actions, focus return,
  viewport containment and no horizontal overflow. Screenshots are retained
  in the ignored `apps/web/.pr-assets/` directory and mirrored under
  `/tmp/config-chat-restart-evidence/`.
- Public docs, specification statuses and work-package results were updated.
  Public-document checks (62 tests, 47 pages), specification checks (330
  decisions, 1248 specs, 36 linter tests), delivery-package coverage and
  whitespace checks passed. The successful phone command used an isolated Go
  cache and a 2048MB Node heap after build-only host/cache failures; see plan.
