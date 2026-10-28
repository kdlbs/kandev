---
id: "05-shared-viewer"
title: "Reuse the shared desktop and phone viewer"
status: done
wave: 5
depends_on:
  - "04-activity-and-input"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
  - REQ-AGENTS-BACKGROUND-WORK-009
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-007.4
  - AC-AGENTS-BACKGROUND-WORK-008.4
  - AC-AGENTS-BACKGROUND-WORK-008.5
  - AC-AGENTS-BACKGROUND-WORK-009.1
  - AC-AGENTS-BACKGROUND-WORK-009.2
  - AC-AGENTS-BACKGROUND-WORK-009.3
  - AC-AGENTS-BACKGROUND-WORK-009.4
  - AC-AGENTS-BACKGROUND-WORK-009.5
  - AC-AGENTS-BACKGROUND-WORK-009.6
system_design:
  - ../../specs/agents/system-design/background-work-opencode.md
---

# Task 05: Reuse the shared desktop and phone viewer

## Summary

Use the existing background-work UI for OpenCode through normalized state.
Complete generic history, capability, and phone-detail behavior without adding
provider-specific branches.

## In scope

- Use BackgroundWorkChip, BackgroundWorkPanel, useBackgroundWork, and current
  shared API/WebSocket/store paths. Add no OpenCode viewer or store.
- Distinguish running, waiting, unknown, terminal, and unavailable states.
  Keep completed history reachable when totalCount reaches zero.
- Hide unsupported controls. Do not infer Stop from a missing capability map.
  Preserve existing explicitly supported Codex actions.
- Treat output capability none as unavailable. Do not label the background
  launch banner as live output. Show final retained output and exit status.
- Preserve draft, focus, selected job, and existing Send/Queue semantics across
  observations, tab changes, disconnect, and completion.
- Desktop: compact chip, summary popover, named central details and overview.
  Phone: inset summary drawer, focused full-height detail, Back, and dismissal.
- Localize new copy, plurals, and accessibility labels in every supported
  catalog. Generate the Traditional Chinese pair with the repository command.

## Out of scope

New job controls, a child-agent composer, same-turn steering, task workflow
changes, and redesign of unrelated chat/status controls.

## Acceptance

1. Shared state renders the same OpenCode job outcomes on desktop and phone,
   with history at zero active jobs and no unsupported controls.
2. Typing, unsent attachments, focus, Send/Queue behavior, and job selection
   survive background updates and opening/closing details.
3. The rendered phone flow has one scroll owner, no page horizontal overflow,
   safe-area/keyboard clearance, and at least 44px action targets.

## ASCII UI preview

Use [OC-UI-01 through OC-UI-03](plan.md#ascii-ui-preview). Relevant excerpt:

~~~text
OC-UI-01 desktop:
  Working in background
  [2 Background jobs v] [Background history]
  [Message...                                ] [Send]
  Popover -> [CI poller tab] / [Background work tab]

OC-UI-02 phone:
  [2 Background jobs]
       |
       v
  Inset summary drawer -> full-height job detail
  [Back] CI poller [Close]
  [one scrolling output region]

OC-UI-03 terminal/unknown:
  Unknown: Activity unavailable. Unsupported controls absent.
  Failed: retained output + actual exit status.
  All terminal: history entry; active animation removed.
~~~

Structural requirements: use the existing mobile drawer entry point, a focused
detail surface, fixed detail header, and one internal output scroll region.
Keep draft state in the shared composer owner. Desktop control density remains
unchanged. Map the rendered evidence to AC-009.1-.6.

## Verification

Install dependencies once if this worktree has no apps/node_modules:

~~~bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web exec vitest run components/task/chat/background-work/background-work.test.tsx hooks/domains/session/use-background-work.test.ts hooks/domains/session/session-input-mode.test.ts
pnpm --dir apps/web exec eslint --max-warnings 0 components/task/chat/background-work hooks/domains/session/use-background-work.ts hooks/domains/session/session-input-mode.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/opencode-background-work.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-opencode-background-work.spec.ts
git diff --check
~~~

Create the two deterministic browser suites in this work order. Task 06 extends
their executor and recovery evidence. Run desktop and phone commands sequentially.
Inspect the rendered phone view against OC-UI-02 and record any difference.

## Files likely touched

- Existing components/task/chat/background-work/background-work-chip.tsx,
  background-work-panel.tsx, background-work.test.tsx, and chat-status-bar.tsx.
- Existing hooks/domains/session/use-background-work.ts and its tests.
- Existing lib/types/background-work.ts, normalized state selectors, and
  session-input-mode tests only where the current shared model needs wiring.
- Proposed e2e/tests/task/opencode-background-work.spec.ts and
  mobile-opencode-background-work.spec.ts.
- Existing mock-agent fixture handlers for an OpenCode-identity producer.
- src/locales/en, ja, ko, pt-pt, zh-cn, zh-hk, zh-tw, and pseudo catalogs as needed.

## Dependencies

Task 04 provides truthful background activity and prompt admission.
Source, not the older draft shared plan, determines which viewer gaps remain.

## Risks

The current chip disappears when active count is zero. Missing action metadata
currently permits fallback controls in parts of the viewer. Fix those generic
paths without disabling explicit provider capabilities.

## Parallelism

sequential

## Inputs

- [Shared presentation design](../../specs/agents/system-design/background-work-opencode.md#shared-desktop-and-phone-experience).
- Existing BackgroundWorkChip drawer, mobile-parked-background-work.spec.ts,
  and useResponsiveBreakpoint.
- /mobile-parity, /e2e, apps/web/AGENTS.md, and chat/AGENTS.md.

## Results

Implemented on 2026-10-27.

Generic viewer gaps closed without provider branches:

- `useBackgroundWork` now exposes `unknownCount`, `totalJobCount`, and
  `hasHistory` from one pure summarizer. `totalCount` remains the active count
  (running + waiting + unknown).
- `BackgroundWorkChip` keeps a completed/failed history entry
  (`background-work-history-chip`) once the active count reaches zero; an empty
  session that never had jobs still renders nothing. The summary shows separate
  running, waiting, and unknown counts and localized state labels.
- Unsupported actions are no longer inferred from a missing capability map.
  The detail header renders Stop/Interrupt only for an explicitly supported,
  available capability and otherwise shows a localized unavailable notice.
  Explicit Codex capabilities keep working.
- Native Codex subagent frames now carry the adapter's verified declaration
  (`codexSubagentCapabilities`), so a live child exposes Interrupt without a
  snapshot reload. The declaration is persisted and re-emitted through the
  committed projection.
- Action rendering is state-gated: a completed, failed, ended, or unknown job
  never shows an enabled Stop, Interrupt, or input control even when an earlier
  declaration said available. The store preserves static support while clearing
  dynamic availability on those transitions.
- Phone detail reuses the retained exit-status badge and output, so a failed
  job shows `Exit <code>` on both viewports.
- Output capability `none` shows a distinct "output is not available" message
  and hides the search/autoscroll/clear toolbar; the launch banner is never
  treated as live output. Retained exit status renders as `Exit <code>`, and a
  failed state uses the destructive badge.
- Phone job detail is a full-height surface with a fixed header, one internal
  scroll owner, coarse-pointer 44px Back/Close/Open targets, and safe-area
  bottom padding. The summary stays an inset bottom drawer; dismissal restores
  focus to the chip.
- New copy is translated in `en`, `pt-pt`, `zh-cn`, `ja`, `ko`; the Traditional
  Chinese pair was generated with `pnpm run i18n:zh-hant`; the pseudo catalog
  was regenerated. `pnpm run i18n:check` passes.
- Store merges preserve adapter-declared capabilities when a committed
  projection cannot carry them, and never reopen terminal work.

Tests: extended `background-work.test.tsx` and `use-background-work.test.ts`,
added `background-work-state.test.ts`, plus the two deterministic browser
suites (`e2e/tests/task/opencode-background-work.spec.ts` and
`e2e/tests/task/mobile-opencode-background-work.spec.ts`) and the
`/opencode-background-fixture` mock producer.

The suites cover launch identity, unknown-after-settlement, no unsupported
controls, unavailable output, history after the active count clears, an
ordinary prompt while jobs remain, phone drawer/detail/Back, 44px targets,
focus return, draft preservation, and no horizontal overflow. Automatic
follow-up delivery is not asserted because it requires Task 02.

Executed on 2026-10-27:

~~~bash
pnpm --dir apps/web exec vitest run \
  components/task/chat/background-work/background-work.test.tsx \
  lib/state/slices/session-runtime \
  lib/state/slices/features/features-contract.test.ts \
  hooks/domains/session/use-background-work.test.ts \
  hooks/domains/session/session-input-mode.test.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium \
  tests/task/opencode-background-work.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome \
  tests/task/mobile-opencode-background-work.spec.ts
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
~~~

All passed. The rendered phone detail measured full viewport height, the
Back/Close targets measured at least 44px, the unsent draft survived
open/close, focus returned to the chip, and the document had no horizontal
overflow.
