---
id: "02-composer-customization"
title: "Move session MCP controls into composer plug"
status: done
wave: 2
depends_on:
  - "01-stabilize-selection-refresh"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.12
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 02: Composer MCP Customization

## Summary

Use the existing plug's explorer for both connection inspection and session
customization. Remove the transcript bar and keep unapplied state visible on
the closed plug, including compact and touch compositions.

## In scope

Extract one session selection controller above responsive toolbar branches;
pass its result into the plug/explorer. Reuse the picker content and application
notice in a Customize view, preserve Connections, and add a clock/next-start/
failure presentation derived only from authoritative selection state. Share
query/action logic across desktop/mobile and preserve ordinary/handled errors.

## Out of scope

Backend contracts, marketplace or built-in pruning, quick-chat customization,
new global selection storage, and external forwarding configuration.

## Acceptance

1. The conversation has no session MCP bar. Plug click or keyboard opens the
   desktop dialog; phone/coarse-pointer tap opens the single existing explorer
   drawer. Both allow changing session additions with names and inherited
   summaries and retain connection/tool inspection. The compact toolbar keeps
   the plug accessible without first opening overflow.
2. Pending changes display a clock while the overlay is closed and a translated
   waiting notice inside; applied removes it, deferred/failed are distinct,
   and Retry remains usable. No pending or connection-health state is invented
   for rejected writes. Closed-overlay polling still reaches terminal state.
3. One controller/timer outlives responsive toolbar branches. Loaded rows,
   search, focus, and scroll survive refresh and view changes. Context switching
   isolates state. Ordinary errors remain local; handled recovery closes the
   whole dialog/drawer. Touch controls meet 44px and are viewport-contained.

## Regression sequence

Change existing task MCP journeys to enter via `mcp-status-trigger` and Customize,
then assert no `task-session-mcp-settings` bar in the transcript. These must fail
before integration. Move the Task 01 held-response tests into Customize without
dropping their mount/focus assertions. Extend the rejection helper to target
this surface, including closed overlays for shared reload recovery on desktop.

Add a pure presentation helper regression for pending/applied/deferred/failed
markers and invalid context. Add hook coverage for one sequential pending poll,
refresh failure recovery, unmount/context cancellation, and save guards.
Use existing explorer tests for view/navigation integration; do not add markup
snapshot tests. Preserve connection/tool E2E suites and add pending transitions
with the overlay closed, compact toolbar visibility, and a coarse-pointer tablet
drawer case. Do not simulate a DB write just to test a pending marker: control
the real GET/PUT response boundary and keep provider attachment evidence separate.

## ASCII UI preview

UI-01/UI-02/UI-03 from [the combined preview](plan.md#ascii-ui-preview):

```text
Closed composer: [plan] [plug+clock] [attach] ... [send]
No separate bar above the conversation.

Desktop dialog:
+-------------------------------------------------+
| MCP servers                             [Close] |
| [Connections] [Customize]                       |
| Waiting for idle                                |
| Inherited: Documentation / Profile: Sandbox     |
| [Search.......................................] |
| [ ] Issue tracker                               |
+-------------------------------------------------+

Phone drawer:
+------------------------------------+
| MCP servers                [Close] | fixed
| [Connections] [Customize]          | fixed
|------------------------------------|
| Waiting for idle                   |
| Inherited names                    |
| [Search..........................] |
| [ ] Issue tracker                  | scroll
+------------------------------------+
```

One overlay and a pending clock are structural requirements (AC-006.9/006.10).
Phone uses the existing full-height explorer; tablet uses its bounded drawer.
Connection view remains a focused server/tools hierarchy on touch. Labels and
spacing are illustrative. Reuse tokens, translated copy, focus/dismiss behavior,
safe areas, and 28px desktop/44px touch controls. Refresh/error views follow UI-04.

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`: remove the old bar/import.
- `apps/web/components/task/task-session-mcp-settings.tsx`: extract controller
  and retire the existing-session bar; retain only shared pieces actually used.
- New `apps/web/hooks/domains/session/use-task-session-mcp-selection.ts` and test.
- `apps/web/components/task/mcp-session-selector.tsx`: reuse/export the state
  notice/body without changing creation-form composition.
- `apps/web/components/task/chat/chat-input-container.tsx`, `chat-input-body.tsx`,
  `chat-input-toolbar.tsx`, `chat-input-toolbar-desktop.tsx`, and
  `chat-input-toolbar-mobile.tsx`: pass one controller, pin the plug.
- `apps/web/components/task/chat/mcp-explorer/mcp-indicator.tsx`,
  `mcp-server-explorer.tsx`, `mcp-status-presentation.tsx`, `mcp-explorer-header.tsx`,
  `mcp-explorer-views.tsx`, and presentation helper/test: overlay views and markers.
- Existing task MCP/status/toolbar suites and `e2e/helpers/mcp-selection-feedback.ts`.
- `apps/web/src/locales/*/task.json` and `settings.json` as needed.
- `docs/public/agents-and-profiles.md`: composer entry instructions.

## Verification

Run from repository root. Build once after final production edits, then run
guarded browser suites sequentially; never overlap full runs or add workers:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-task-session-mcp-selection.test.ts hooks/domains/workspace/use-mcp-selection-editor.test.ts hooks/domains/workspace/use-mcp-workspace-settings.test.ts components/mcp/mcp-origin-labels.test.ts lib/api/client.test.ts components/task/chat/mcp-explorer/mcp-selection-presentation.test.ts components/task/chat/mcp-explorer/mcp-server-explorer.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/task/task-mcp-selection.spec.ts tests/chat/mcp-status.spec.ts tests/chat/toolbar-overflow.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-task-mcp-selection.spec.ts tests/chat/mobile-mcp-status.spec.ts --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint with `--max-warnings 0` on every changed TS/TSX path,
including the new controller/helper and all changed suites. Keep that exact
path list in Results. If production changes after the build, rebuild before
using `--no-build`. Verify every newly referenced test path exists before running.

After checks pass, back up and retain the owned :48593 SQLite DB, install the
tested backend/web artifacts, and visually verify desktop/phone plug behavior.
Record ordinary forwarded 403 separately from direct positive save evidence.
Leave main :9998 and fixture ownership unchanged; close diagnostic browsers.

## Dependencies and risks

Task 01 must pass first. Responsive toolbar branches mount independently; create
the controller above their branch. Close overlays on context changes/handled recovery.
Do not confuse desired selections with provider connection evidence. Sequential
work; no delegation. Preserve historical verification counts in companion plans.

## Inputs and results

Owning requirements/design, current MCP explorer, prior feedback regressions,
and Task 01 stable-refresh results. Implemented and deployed on 2026-10-05.

RED: presentation and controller regressions each reproduced three failures;
the browser integration regression found the old transcript bar still present.
GREEN after the final component extraction: 102 unit tests across 10 files,
8 desktop/tablet browser tests, and 6 phone browser tests passed. Browser projects
ran sequentially, with one worker and zero retries. Typecheck, targeted ESLint,
localization completeness/ratchet, 62 public-documentation tests, 47-page docs
validation, the 348-decision/1334-specification catalog, 36 specification-linter
tests, specification lint, and whitespace checks passed. Both work orders passed
the local PR-documentation package-reference preflight.

The held-response journeys kept the same focused search input and checkbox
mounted through three pending refreshes, then observed the closed plug clear its
marker on Applied. Rejection cases retained authoritative choices without false
reload for ordinary 403; handled recovery closed desktop and phone overlays.
Existing connection/tool exploration and compact toolbar access passed. Touch
tablet used one bounded drawer, and touch plug/Close controls measured 44px.
The controller regressions cover sequential polling, failed refresh recovery,
unmount and invalid-context cancellation, and handled recovery stopping polling.

The final unit command also included `chat-input-container.test.tsx`,
`chat-input-toolbar.test.tsx`, and `chat-input-toolbar-composer.test.tsx` beside
the seven test paths listed above. Final browser logs are
`/tmp/composer-final-desktop.log` and `/tmp/composer-final-phone.log`.

Deployment installed the tested artifacts into the owned :48593 runtime after
SQLite backup to `/tmp/kandev-pr3293-test-runtime/composer-before-deploy.db`.
Backend PID 2107007 is healthy; main :9998 remains PID 3009187. Startup health
became available before the selection API; an initial post-start read returned
503, then the ready API returned 200 with saved IDs retained. The deploy manifest
is `/tmp/kandev-pr3293-test-runtime/composer-build/manifest.json`.

Live desktop inspection found no transcript bar, a 28px plug, named inherited
origins, and no horizontal overflow. Direct browser selection save and restore
both returned 200 with truthful deferred-next-start state; original IDs were
restored (desired revision advanced from 1 to 3). Phone inspection found one
viewport-contained drawer, a 44px Close control, and no horizontal overflow.
Screenshots: `mcp-composer-desktop.png` and `mcp-composer-phone.png` under the
owned runtime directory. The fresh external forwarder request still returned
403 `forbidden`, retained choices, showed one local error, and produced zero
reload alerts. Its credential injection was not changed. Browser closed.

No commits, pushes, PR changes, merge, delegation, or main-instance mutation.
The original PR CI/review work and built-in tool policy remain separate pending
phases; these passing targeted checks are not a merge-readiness verdict.


Exact final ESLint path list (run from `apps/web`, `--max-warnings 0`):

```bash
pnpm exec eslint --max-warnings 0 \
  "app/settings/agents/[agentId]/profile-mcp-selection-card.tsx" \
  components/mcp/mcp-selection-picker.tsx \
  components/settings/repository-mcp-selection.tsx \
  components/task-create-dialog.tsx \
  components/task/chat/chat-input-body.tsx \
  components/task/chat/chat-input-container.test.tsx \
  components/task/chat/chat-input-container.tsx \
  components/task/chat/chat-input-toolbar-desktop.tsx \
  components/task/chat/chat-input-toolbar-mobile.tsx \
  components/task/chat/chat-input-toolbar.tsx \
  components/task/chat/mcp-explorer/mcp-explorer-header.tsx \
  components/task/chat/mcp-explorer/mcp-indicator.tsx \
  components/task/chat/mcp-explorer/mcp-server-explorer.tsx \
  components/task/chat/mcp-explorer/mcp-status-presentation.tsx \
  components/task/mcp-session-selector.tsx \
  components/task/new-session-dialog.tsx \
  components/task/task-chat-panel.tsx \
  e2e/tests/chat/toolbar-overflow.spec.ts \
  e2e/tests/task/mobile-task-mcp-selection.spec.ts \
  e2e/tests/task/task-mcp-selection.spec.ts \
  hooks/domains/workspace/use-mcp-selection-editor.ts \
  hooks/domains/workspace/use-mcp-workspace-settings.ts \
  lib/api/client.test.ts \
  components/mcp/mcp-origin-labels.test.ts \
  components/mcp/mcp-origin-labels.ts \
  components/task/chat/mcp-explorer/mcp-explorer-views.tsx \
  components/task/chat/mcp-explorer/mcp-selection-presentation.test.ts \
  components/task/chat/mcp-explorer/mcp-selection-presentation.ts \
  e2e/helpers/mcp-composer.ts \
  e2e/helpers/mcp-selection-feedback.ts \
  e2e/helpers/mcp-selection-refresh.ts \
  hooks/domains/session/use-task-session-mcp-selection.test.ts \
  hooks/domains/session/use-task-session-mcp-selection.ts \
  hooks/domains/workspace/use-mcp-selection-editor.test.ts \
  hooks/domains/workspace/use-mcp-workspace-settings.test.ts
```
